package tgc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"sync"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/types"
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"
)

var (
	ErrInValidChannelId       = errors.New("invalid channel id")
	ErrInvalidChannelMessages = errors.New("invalid channel messages")
)

func GetChannelById(ctx context.Context, client *tg.Client, channelId int64) (*tg.InputChannel, error) {
	inputChannel := &tg.InputChannel{
		ChannelID: channelId,
	}
	channels, err := client.ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})

	if err != nil {
		return nil, err
	}

	if len(channels.GetChats()) == 0 {
		return nil, ErrInValidChannelId
	}
	return channels.GetChats()[0].(*tg.Channel).AsInput(), nil
}

func DeleteMessages(ctx context.Context, client *telegram.Client, channelId int64, ids []int) error {

	return RunWithAuth(ctx, client, "", func(ctx context.Context) error {
		channel, err := GetChannelById(ctx, client.API(), channelId)

		if err != nil {
			return err
		}

		batchSize := 100

		batchCount := int(math.Ceil(float64(len(ids)) / float64(batchSize)))

		g, _ := errgroup.WithContext(ctx)

		g.SetLimit(runtime.NumCPU())

		for i := 0; i < batchCount; i++ {
			start := i * batchSize
			end := min((i+1)*batchSize, len(ids))
			batchIds := ids[start:end]
			g.Go(func() error {
				messageDeleteRequest := tg.ChannelsDeleteMessagesRequest{Channel: channel, ID: batchIds}
				_, err = client.API().ChannelsDeleteMessages(ctx, &messageDeleteRequest)
				return err
			})
		}
		return g.Wait()
	})
}

func getTGMessagesBatch(ctx context.Context, client *tg.Client, channel *tg.InputChannel, ids []int) (tg.MessagesMessagesClass, error) {

	messageRequest := tg.ChannelsGetMessagesRequest{
		Channel: channel,
		ID: utils.Map(ids, func(id int) tg.InputMessageClass {
			return &tg.InputMessageID{ID: id}
		}),
	}

	res, err := client.ChannelsGetMessages(ctx, &messageRequest)

	if err != nil {
		return nil, err
	}

	return res, nil

}

func GetMessages(ctx context.Context, client *tg.Client, ids []int, channelId int64) ([]tg.MessageClass, error) {

	channel, err := GetChannelById(ctx, client, channelId)

	if err != nil {
		return nil, err
	}

	batchSize := 100

	batchCount := int(math.Ceil(float64(len(ids)) / float64(batchSize)))

	g, _ := errgroup.WithContext(ctx)

	g.SetLimit(runtime.NumCPU())

	messageMap := make(map[int]*tg.MessagesChannelMessages)

	var mapMu sync.Mutex

	for i := range batchCount {
		g.Go(func() error {
			splitIds := ids[i*batchSize : min((i+1)*batchSize, len(ids))]
			res, err := getTGMessagesBatch(ctx, client, channel, splitIds)
			if err != nil {
				return err
			}
			messages, ok := res.(*tg.MessagesChannelMessages)
			if !ok {
				return ErrInvalidChannelMessages
			}
			mapMu.Lock()
			messageMap[i] = messages
			mapMu.Unlock()
			return nil
		})

	}

	if err = g.Wait(); err != nil {
		return nil, err
	}

	allMessages := []tg.MessageClass{}

	for i := range batchCount {
		allMessages = append(allMessages, messageMap[i].Messages...)
	}

	return allMessages, nil
}

func GetChunk(ctx context.Context, client *tg.Client, location tg.InputFileLocationClass, offset int64, limit int64) ([]byte, error) {
	req := &tg.UploadGetFileRequest{
		Offset:   offset,
		Limit:    int(limit),
		Location: location,
		Precise:  true,
	}

	r, err := client.UploadGetFile(ctx, req)

	if err != nil {
		return nil, err
	}

	switch result := r.(type) {
	case *tg.UploadFile:
		return result.Bytes, nil
	default:
		return nil, fmt.Errorf("unexpected type %T", r)
	}
}

func GetMediaContent(ctx context.Context, client *tg.Client, location tg.InputFileLocationClass) (*bytes.Buffer, error) {
	offset := int64(0)
	limit := int64(1024 * 1024)
	buff := &bytes.Buffer{}
	for {
		r, err := GetChunk(ctx, client, location, offset, limit)
		if err != nil {
			return buff, err
		}
		if len(r) == 0 {
			break
		}
		buff.Write(r)
		offset += int64(limit)
	}
	return buff, nil
}

func GetBotInfo(ctx context.Context, boltdb *bbolt.DB, config *config.TGConfig, token string) (*types.BotInfo, error) {
	var user *tg.User
	middlewares := NewMiddleware(config, WithFloodWait(), WithRateLimit())
	client, _ := BotClient(ctx, boltdb, config, token, middlewares...)
	err := RunWithAuth(ctx, client, token, func(ctx context.Context) error {
		user, _ = client.Self(ctx)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &types.BotInfo{Id: user.ID, UserName: user.Username, Token: token}, nil
}

func GetLocation(ctx context.Context, client *tg.Client, channelId int64, partId int64) (location *tg.InputDocumentFileLocation, err error) {

	channel, err := GetChannelById(ctx, client, channelId)

	if err != nil {
		return nil, err
	}
	messageRequest := tg.ChannelsGetMessagesRequest{
		Channel: channel,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: int(partId)}},
	}

	res, err := client.ChannelsGetMessages(ctx, &messageRequest)
	if err != nil {
		return nil, err
	}

	messages, _ := res.(*tg.MessagesChannelMessages)

	if len(messages.Messages) == 0 {
		return nil, errors.New("no messages found")
	}

	switch item := messages.Messages[0].(type) {
	case *tg.MessageEmpty:
		return nil, errors.New("no messages found")
	case *tg.Message:
		media := item.Media.(*tg.MessageMediaDocument)
		document := media.Document.(*tg.Document)
		location = document.AsInputDocumentFileLocation()

	}

	return location, nil
}

func CalculateChunkSize(start, end int64) int64 {
	chunkSize := int64(1024 * 1024)

	for chunkSize > 1024 && chunkSize > (end-start) {
		chunkSize /= 2
	}
	return chunkSize
}

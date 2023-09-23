package services

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"strconv"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/schemas"
	"github.com/divyam234/teldrive/types"
	"github.com/divyam234/teldrive/utils"
	"github.com/divyam234/teldrive/utils/kv"
	"github.com/divyam234/teldrive/utils/tgc"
	"github.com/gin-gonic/gin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/pkg/errors"
	"github.com/thoas/go-funk"
)

func getChunk(ctx context.Context, tgClient *telegram.Client, location tg.InputFileLocationClass, offset int64, limit int64) ([]byte, error) {

	req := &tg.UploadGetFileRequest{
		Offset:   offset,
		Limit:    int(limit),
		Location: location,
	}

	r, err := tgClient.API().UploadGetFile(ctx, req)

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

func iterContent(ctx context.Context, tgClient *telegram.Client, location tg.InputFileLocationClass) (*bytes.Buffer, error) {
	offset := int64(0)
	limit := int64(1024 * 1024)
	buff := &bytes.Buffer{}
	for {
		r, err := getChunk(ctx, tgClient, location, offset, limit)
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

func getUserAuth(c *gin.Context) (int64, string) {
	val, _ := c.Get("jwtUser")
	jwtUser := val.(*types.JWTClaims)
	userId, _ := strconv.ParseInt(jwtUser.Subject, 10, 64)
	return userId, jwtUser.TgSession
}

func getBotInfo(ctx context.Context, token string) (*BotInfo, error) {
	client, _ := tgc.BotLogin(token)
	var user *tg.User
	err := tgc.RunWithAuth(ctx, client, token, func(ctx context.Context) error {
		user, _ = client.Self(ctx)
		return nil
	})

	if err != nil {
		return nil, err
	}
	return &BotInfo{Id: user.ID, UserName: user.Username, Token: token}, nil
}

func getParts(ctx context.Context, client *telegram.Client, file *schemas.FileOutFull, userID string) ([]types.Part, error) {

	ids := funk.Map(*file.Parts, func(part models.Part) tg.InputMessageClass {
		return tg.InputMessageClass(&tg.InputMessageID{ID: int(part.ID)})
	})

	channel, err := GetChannelById(ctx, client, *file.ChannelID, userID)

	if err != nil {
		return nil, err
	}

	messageRequest := tg.ChannelsGetMessagesRequest{Channel: channel, ID: ids.([]tg.InputMessageClass)}

	res, err := client.API().ChannelsGetMessages(ctx, &messageRequest)

	if err != nil {
		return nil, err
	}

	messages := res.(*tg.MessagesChannelMessages)

	parts := []types.Part{}

	for _, message := range messages.Messages {
		item := message.(*tg.Message)
		media := item.Media.(*tg.MessageMediaDocument)
		document := media.Document.(*tg.Document)
		location := document.AsInputDocumentFileLocation()
		parts = append(parts, types.Part{Location: location, Start: 0, End: document.Size - 1, Size: document.Size})
	}
	return parts, nil
}

func rangedParts(parts []types.Part, start, end int64) []types.Part {

	chunkSize := parts[0].Size

	startPartNumber := utils.Max(int64(math.Ceil(float64(start)/float64(chunkSize)))-1, 0)

	endPartNumber := int64(math.Ceil(float64(end) / float64(chunkSize)))

	partsToDownload := parts[startPartNumber:endPartNumber]
	partsToDownload[0].Start = start % chunkSize
	partsToDownload[len(partsToDownload)-1].End = end % chunkSize

	for i, part := range partsToDownload {
		partsToDownload[i].Length = part.End - part.Start + 1
	}

	return partsToDownload
}

func GetChannelById(ctx context.Context, client *telegram.Client, channelID int64, userID string) (*tg.InputChannel, error) {

	channel := &tg.InputChannel{}
	inputChannel := &tg.InputChannel{
		ChannelID: channelID,
	}
	channels, err := client.API().ChannelsGetChannels(ctx, []tg.InputChannelClass{inputChannel})

	if err != nil {
		return nil, err
	}

	if len(channels.GetChats()) == 0 {
		return nil, errors.New("no channels found")
	}

	channel = channels.GetChats()[0].(*tg.Channel).AsInput()
	return channel, nil
}

func GetDefaultChannel(ctx context.Context, userID int64) (int64, error) {

	var channelID int64

	key := kv.Key("users", strconv.FormatInt(userID, 10), "channel")

	err := kv.GetValue(database.KV, key, &channelID)

	if err != nil {
		var channelIds []int64
		database.DB.Model(&models.Channel{}).Where("user_id = ?", userID).Where("selected = ?", true).
			Pluck("channel_id", &channelIds)

		if len(channelIds) == 1 {
			channelID = channelIds[0]
			kv.SetValue(database.KV, key, &channelID)
		}
	}

	if channelID == 0 {
		return channelID, errors.New("default channel not set")
	}

	return channelID, nil
}

func GetBotsToken(userID int64) ([]string, error) {
	var bots []string

	key := kv.Key("users", strconv.FormatInt(userID, 10), "bots")

	err := kv.GetValue(database.KV, key, &bots)

	if err != nil {
		if err := database.DB.Model(&models.Bot{}).Where("user_id = ?", userID).Pluck("token", &bots).Error; err != nil {
			return nil, err
		}
		kv.SetValue(database.KV, key, &bots)
	}

	return bots, nil

}

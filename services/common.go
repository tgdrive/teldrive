package services

import (
	"bytes"
	"context"
	"fmt"
	"strconv"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/schemas"
	"github.com/divyam234/teldrive/types"
	"github.com/divyam234/teldrive/utils/cache"
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
	client, _ := tgc.BotLogin(ctx, token)
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

func getTGMessages(ctx context.Context, client *telegram.Client, parts models.Parts, channelId int64, userID string) (*tg.MessagesChannelMessages, error) {

	ids := funk.Map(parts, func(part models.Part) tg.InputMessageClass {
		return tg.InputMessageClass(&tg.InputMessageID{ID: int(part.ID)})
	})

	channel, err := GetChannelById(ctx, client, channelId, userID)

	if err != nil {
		return nil, err
	}

	messageRequest := tg.ChannelsGetMessagesRequest{Channel: channel, ID: ids.([]tg.InputMessageClass)}

	res, err := client.API().ChannelsGetMessages(ctx, &messageRequest)

	if err != nil {
		return nil, err
	}

	messages := res.(*tg.MessagesChannelMessages)

	return messages, nil
}

func getParts(ctx context.Context, client *telegram.Client, file *schemas.FileOutFull, userID string) ([]types.Part, error) {

	parts := []types.Part{}

	key := fmt.Sprintf("messages:%s:%s", file.ID, userID)

	err := cache.GetCache().Get(key, &parts)

	if err == nil {
		return parts, nil
	}

	messages, err := getTGMessages(ctx, client, *file.Parts, *file.ChannelID, userID)

	if err != nil {
		return nil, err
	}

	for _, message := range messages.Messages {
		item := message.(*tg.Message)
		media := item.Media.(*tg.MessageMediaDocument)
		document := media.Document.(*tg.Document)
		location := document.AsInputDocumentFileLocation()
		parts = append(parts, types.Part{Location: location, Start: 0, End: document.Size - 1})
	}
	cache.GetCache().Set(key, &parts, 3600)
	return parts, nil
}

func rangedParts(parts []types.Part, startByte, endByte int64) []types.Part {

	chunkSize := parts[0].End + 1

	numParts := int64(len(parts))

	validParts := []types.Part{}

	firstChunk := max(startByte/chunkSize, 0)

	lastChunk := min(endByte/chunkSize, numParts)

	startInFirstChunk := startByte % chunkSize

	endInLastChunk := endByte % chunkSize

	if firstChunk == lastChunk {
		validParts = append(validParts, types.Part{
			Location: parts[firstChunk].Location,
			Start:    startInFirstChunk,
			End:      endInLastChunk,
		})
	} else {
		validParts = append(validParts, types.Part{
			Location: parts[firstChunk].Location,
			Start:    startInFirstChunk,
			End:      parts[firstChunk].End,
		})

		// Add valid parts from any chunks in between.
		for i := firstChunk + 1; i < lastChunk; i++ {
			validParts = append(validParts, types.Part{
				Location: parts[i].Location,
				Start:    0,
				End:      parts[i].End,
			})
		}

		// Add valid parts from the last chunk.
		validParts = append(validParts, types.Part{
			Location: parts[lastChunk].Location,
			Start:    0,
			End:      endInLastChunk,
		})
	}

	return validParts
}

func GetChannelById(ctx context.Context, client *telegram.Client, channelId int64, userID string) (*tg.InputChannel, error) {

	channel := &tg.InputChannel{}
	inputChannel := &tg.InputChannel{
		ChannelID: channelId,
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

	var channelId int64

	key := fmt.Sprintf("users:channel:%d", userID)

	err := cache.GetCache().Get(key, &channelId)

	if err == nil {
		return channelId, nil
	}

	var channelIds []int64
	database.DB.Model(&models.Channel{}).Where("user_id = ?", userID).Where("selected = ?", true).
		Pluck("channel_id", &channelIds)

	if len(channelIds) == 1 {
		channelId = channelIds[0]
		cache.GetCache().Set(key, channelId, 0)
	}

	if channelId == 0 {
		return channelId, errors.New("default channel not set")
	}

	return channelId, nil
}

func GetBotsToken(ctx context.Context, userID, channelId int64) ([]string, error) {
	var bots []string

	key := fmt.Sprintf("users:bots:%d:%d", userID, channelId)

	err := cache.GetCache().Get(key, &bots)

	if err == nil {
		return bots, nil
	}

	if err := database.DB.Model(&models.Bot{}).Where("user_id = ?", userID).
		Where("channel_id = ?", channelId).Pluck("token", &bots).Error; err != nil {
		return nil, err
	}

	cache.GetCache().Set(key, &bots, 0)
	return bots, nil

}

func GetSessionByHash(hash string) (*models.Session, error) {

	var session models.Session

	key := fmt.Sprintf("sessions:%s", hash)

	err := cache.GetCache().Get(key, &session)

	if err == nil {
		return &session, nil
	}

	if err := database.DB.Model(&models.Session{}).Where("hash = ?", hash).First(&session).Error; err != nil {
		return nil, err
	}

	cache.GetCache().Set(key, &session, 0)

	return &session, nil

}

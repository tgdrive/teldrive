package services

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	cnf "github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/mapper"
	"github.com/divyam234/teldrive/pkg/schemas"

	"github.com/divyam234/teldrive/pkg/types"

	"github.com/divyam234/teldrive/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"gorm.io/gorm"
)

type UploadService struct {
	Db *gorm.DB
}

func NewUploadService(db *gorm.DB) *UploadService {
	return &UploadService{Db: db}
}

func (us *UploadService) GetUploadFileById(c *gin.Context) (*schemas.UploadOut, *types.AppError) {
	uploadId := c.Param("id")
	parts := []schemas.UploadPartOut{}
	config := cnf.GetConfig()
	if err := us.Db.Model(&models.Upload{}).Order("part_no").Where("upload_id = ?", uploadId).
		Where("created_at >= ?", time.Now().UTC().AddDate(0, 0, -config.UploadRetention)).
		Find(&parts).Error; err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return &schemas.UploadOut{Parts: parts}, nil
}

func (us *UploadService) DeleteUploadFile(c *gin.Context) (*schemas.Message, *types.AppError) {
	uploadId := c.Param("id")
	if err := us.Db.Where("upload_id = ?", uploadId).Delete(&models.Upload{}).Error; err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return &schemas.Message{Message: "upload deleted"}, nil
}

func (us *UploadService) CreateUploadPart(c *gin.Context) (*schemas.UploadPartOut, *types.AppError) {

	userId, _ := getUserAuth(c)

	var payload schemas.UploadPart

	if err := c.ShouldBindJSON(&payload); err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	partUpload := &models.Upload{
		Name:      payload.Name,
		UploadId:  payload.UploadId,
		PartId:    payload.PartId,
		ChannelID: payload.ChannelID,
		Size:      payload.Size,
		PartNo:    payload.PartNo,
		UserId:    userId,
	}

	if err := us.Db.Create(partUpload).Error; err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	out := mapper.ToUploadOut(partUpload)

	return out, nil
}

func (us *UploadService) UploadFile(c *gin.Context) (*schemas.UploadPartOut, *types.AppError) {

	var (
		uploadQuery schemas.UploadQuery
		channelId   int64
		err         error
		client      *telegram.Client
		token       string
		channelUser string
		out         *schemas.UploadPartOut
	)

	uploadQuery.PartNo = 1

	if err := c.ShouldBindQuery(&uploadQuery); err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	userId, session := getUserAuth(c)

	uploadId := c.Param("id")

	file := c.Request.Body

	fileSize := c.Request.ContentLength

	fileName := uploadQuery.Filename

	if uploadQuery.ChannelID == 0 {
		channelId, err = GetDefaultChannel(c, userId)
		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
		}
	} else {
		channelId = uploadQuery.ChannelID
	}

	tokens, err := getBotsToken(c, userId, channelId)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	if len(tokens) == 0 {
		client, _ = tgc.UserLogin(c, session)
		channelUser = strconv.FormatInt(userId, 10)
	} else {
		tgc.Workers.Set(tokens, channelId)
		token = tgc.Workers.Next(channelId)
		client, _ = tgc.BotLogin(c, token)
		channelUser = strings.Split(token, ":")[0]
	}

	err = tgc.RunWithAuth(c, client, token, func(ctx context.Context) error {

		channel, err := GetChannelById(ctx, client, channelId, channelUser)

		if err != nil {
			return err
		}

		api := client.API()

		u := uploader.NewUploader(api).WithThreads(16).WithPartSize(512 * 1024)

		upload, err := u.Upload(c, uploader.NewUpload(fileName, file, fileSize))

		if err != nil {
			return err
		}

		document := message.UploadedDocument(upload).Filename(fileName).ForceFile(true)

		sender := message.NewSender(client.API())

		target := sender.To(&tg.InputPeerChannel{ChannelID: channel.ChannelID,
			AccessHash: channel.AccessHash})

		res, err := target.Media(c, document)

		if err != nil {
			return err
		}

		updates := res.(*tg.Updates)

		var message *tg.Message

		for _, update := range updates.Updates {
			channelMsg, ok := update.(*tg.UpdateNewChannelMessage)
			if ok {
				message = channelMsg.Message.(*tg.Message)
				break
			}

		}

		if message.ID == 0 {
			return errors.New("failed to upload part")
		}

		partUpload := &models.Upload{
			Name:      fileName,
			UploadId:  uploadId,
			PartId:    message.ID,
			ChannelID: channelId,
			Size:      fileSize,
			PartNo:    uploadQuery.PartNo,
			UserId:    userId,
		}

		if err := us.Db.Create(partUpload).Error; err != nil {
			return err
		}

		out = mapper.ToUploadOut(partUpload)

		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return out, nil
}

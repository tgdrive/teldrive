package services

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/divyam234/teldrive/mapper"
	"github.com/divyam234/teldrive/schemas"
	"github.com/divyam234/teldrive/utils/tgc"

	"github.com/divyam234/teldrive/types"

	"github.com/divyam234/teldrive/models"
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

func (us *UploadService) GetUploadFileById(c *gin.Context) (*schemas.UploadOut, *types.AppError) {
	uploadId := c.Param("id")
	parts := []schemas.UploadPartOut{}
	if err := us.Db.Model(&models.Upload{}).Order("part_no").Where("upload_id = ?", uploadId).Find(&parts).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to fetch from db"), Code: http.StatusInternalServerError}
	}

	return &schemas.UploadOut{Parts: parts}, nil
}

func (us *UploadService) DeleteUploadFile(c *gin.Context) *types.AppError {
	uploadId := c.Param("id")
	if err := us.Db.Where("upload_id = ?", uploadId).Delete(&models.Upload{}).Error; err != nil {
		return &types.AppError{Error: errors.New("failed to delete upload"), Code: http.StatusInternalServerError}
	}

	return nil
}

func (us *UploadService) UploadFile(c *gin.Context) (*schemas.UploadPartOut, *types.AppError) {

	var uploadQuery schemas.UploadQuery

	uploadQuery.PartNo = 1
	uploadQuery.TotalParts = 1

	if err := c.ShouldBindQuery(&uploadQuery); err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusBadRequest}
	}

	if uploadQuery.Filename == "" {
		return nil, &types.AppError{Error: errors.New("filename missing"), Code: http.StatusBadRequest}
	}

	uploadId := c.Param("id")

	var uploadPart []models.Upload

	us.Db.Model(&models.Upload{}).Where("upload_id = ?", uploadId).Where("part_no = ?", uploadQuery.PartNo).Find(&uploadPart)

	if len(uploadPart) == 1 {
		out := mapper.MapUploadSchema(&uploadPart[0])
		return out, nil
	}

	file := c.Request.Body

	fileSize := c.Request.ContentLength

	fileName := uploadQuery.Filename

	var msgId int

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		cancel()
	}()

	tokens, err := GetBotsToken(c)

	if err != nil {
		return nil, &types.AppError{Error: errors.New("failed to fetch bots"), Code: http.StatusInternalServerError}
	}

	var client *telegram.Client

	var token string

	var channelUser string

	userId, session := getUserAuth(c)

	if len(tokens) == 0 {
		client, _ = tgc.UserLogin(session)
		channelUser = strconv.FormatInt(userId, 10)
	} else {
		tgc.Workers.Set(tokens)
		token = tgc.Workers.Next()
		client, _ = tgc.BotLogin(token)
		channelUser = strings.Split(token, ":")[0]
	}

	var out *schemas.UploadPartOut

	err = tgc.RunWithAuth(ctx, client, token, func(ctx context.Context) error {

		channelId, err := GetDefaultChannel(ctx, userId)

		if err != nil {
			return err
		}

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

		msgId = updates.Updates[0].(*tg.UpdateMessageID).ID

		if msgId == 0 {
			return errors.New("failed to upload part")
		}

		partUpload := &models.Upload{
			Name:       fileName,
			UploadId:   uploadId,
			PartId:     msgId,
			ChannelID:  channelId,
			Size:       fileSize,
			PartNo:     uploadQuery.PartNo,
			TotalParts: uploadQuery.TotalParts,
		}

		if err := us.Db.Create(partUpload).Error; err != nil {
			return errors.New("failed to upload part")
		}

		out = mapper.MapUploadSchema(partUpload)

		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	return out, nil
}

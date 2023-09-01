package services

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/divyam234/teldrive/cache"
	"github.com/divyam234/teldrive/schemas"
	"github.com/divyam234/teldrive/utils"

	"github.com/divyam234/teldrive/types"

	"github.com/divyam234/teldrive/models"
	"github.com/gin-gonic/gin"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"gorm.io/gorm"
)

type UploadService struct {
	Db        *gorm.DB
	ChannelID int64
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
		out := mapSchema(&uploadPart[0])
		return out, nil
	}

	client, idx := utils.GetUploadClient(c)

	file := c.Request.Body

	fileSize := c.Request.ContentLength

	fileName := uploadQuery.Filename

	var msgId int

	ctx := context.Background()

	ctx, cancel := context.WithCancel(ctx)

	defer func() {
		if idx != -1 {
			utils.GetClientWorkload().Dec(idx)
		}
		cancel()
	}()

	err := client.Run(ctx, func(ctx context.Context) error {

		api := client.API()

		u := uploader.NewUploader(api).WithThreads(8).WithPartSize(512 * 1024)

		upload, err := u.Upload(c, uploader.NewUpload(fileName, file, fileSize))

		if err != nil {
			return err
		}

		document := message.UploadedDocument(upload).Filename(fileName).ForceFile(true)

		res, err := cache.CachedFunction(utils.GetChannelById, fmt.Sprintf("channels:%d", us.ChannelID))(c, client.API(), us.ChannelID)

		if err != nil {
			return err
		}

		channel := res.(*tg.Channel)

		sender := message.NewSender(client.API())

		target := sender.To(&tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash})

		res, err = target.Media(c, document)

		if err != nil {
			return err
		}

		updates := res.(*tg.Updates)

		msgId = updates.Updates[0].(*tg.UpdateMessageID).ID

		return nil
	})

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	if msgId == 0 {
		return nil, &types.AppError{Error: errors.New("failed to upload part"), Code: http.StatusInternalServerError}
	}

	partUpload := &models.Upload{
		Name:       fileName,
		UploadId:   uploadId,
		PartId:     msgId,
		ChannelID:  us.ChannelID,
		Size:       fileSize,
		PartNo:     uploadQuery.PartNo,
		TotalParts: uploadQuery.TotalParts,
	}

	if err := us.Db.Create(partUpload).Error; err != nil {
		return nil, &types.AppError{Error: errors.New("failed to upload part"), Code: http.StatusInternalServerError}
	}

	out := mapSchema(partUpload)

	return out, nil
}

func mapSchema(in *models.Upload) *schemas.UploadPartOut {
	out := &schemas.UploadPartOut{
		ID:         in.ID,
		Name:       in.Name,
		PartId:     in.PartId,
		ChannelID:  in.ChannelID,
		PartNo:     in.PartNo,
		TotalParts: in.TotalParts,
		Size:       in.Size,
	}
	return out
}

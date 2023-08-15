package services

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/divyam234/teldrive-go/cache"
	"github.com/divyam234/teldrive-go/schemas"
	"github.com/divyam234/teldrive-go/utils"

	"github.com/divyam234/teldrive-go/types"

	"github.com/divyam234/teldrive-go/models"
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

	config := utils.GetConfig()

	var tgClient *utils.Client

	var err error
	if config.MultiClient {
		tgClient = utils.GetBotClient()
		tgClient.Workload++

	} else {
		val, _ := c.Get("jwtUser")
		jwtUser := val.(*types.JWTClaims)
		userId, _ := strconv.Atoi(jwtUser.Subject)
		tgClient, _, err = utils.GetAuthClient(jwtUser.TgSession, userId)
		if err != nil {
			return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
		}
	}

	file := c.Request.Body

	fileSize := c.Request.ContentLength

	api := tgClient.Tg.API()

	u := uploader.NewUploader(api).WithThreads(16).WithPartSize(512 * 1024)

	sender := message.NewSender(api).WithUploader(u)

	fileName := uploadQuery.Filename

	if uploadQuery.TotalParts > 1 {
		fileName = fmt.Sprintf("%s.part.%03d", fileName, uploadQuery.PartNo)
	}

	upload, err := u.Upload(c, uploader.NewUpload(fileName, file, fileSize))

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	document := message.UploadedDocument(upload).Filename(fileName).ForceFile(true)

	res, err := cache.CachedFunction(utils.GetChannelById, fmt.Sprintf("channels:%d", us.ChannelID))(c, api, us.ChannelID)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	channel := res.(*tg.Channel)

	target := sender.To(&tg.InputPeerChannel{ChannelID: channel.ID, AccessHash: channel.AccessHash})

	res, err = target.Media(c, document)

	if err != nil {
		return nil, &types.AppError{Error: err, Code: http.StatusInternalServerError}
	}

	updates := res.(*tg.Updates)

	msgId := updates.Updates[0].(*tg.UpdateMessageID).ID

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

	out := &schemas.UploadPartOut{
		ID:         partUpload.ID,
		Name:       partUpload.Name,
		PartId:     partUpload.PartId,
		ChannelID:  partUpload.ChannelID,
		PartNo:     partUpload.PartNo,
		TotalParts: partUpload.TotalParts,
	}

	return out, nil
}

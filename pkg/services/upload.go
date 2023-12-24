package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/crypt"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/mapper"
	"github.com/divyam234/teldrive/pkg/schemas"
	"go.uber.org/zap"

	"github.com/divyam234/teldrive/pkg/types"

	"github.com/divyam234/teldrive/pkg/models"
	"github.com/gin-gonic/gin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"gorm.io/gorm"
)

const saltLength = 32

type UploadService struct {
	Db     *gorm.DB
	log    *zap.Logger
	worker *tgc.UploadWorker
}

func NewUploadService(db *gorm.DB, logger *zap.Logger) *UploadService {
	return &UploadService{Db: db, log: logger.Named("uploads"),
		worker: &tgc.UploadWorker{}}
}

func generateRandomSalt() (string, error) {
	randomBytes := make([]byte, saltLength)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}

	hasher := sha256.New()
	hasher.Write(randomBytes)
	hashedSalt := base64.URLEncoding.EncodeToString(hasher.Sum(nil))

	return hashedSalt, nil
}

func (us *UploadService) logAndReturn(context string, err error, errCode int) *types.AppError {
	us.log.Error(context, zap.Error(err))
	return &types.AppError{Error: err, Code: errCode}
}

func (us *UploadService) GetUploadFileById(c *gin.Context) (*schemas.UploadOut, *types.AppError) {
	uploadId := c.Param("id")
	parts := []schemas.UploadPartOut{}
	if err := us.Db.Model(&models.Upload{}).Order("part_no").Where("upload_id = ?", uploadId).
		Where("created_at >= ?", time.Now().UTC().AddDate(0, 0, -config.GetConfig().UploadRetention)).
		Find(&parts).Error; err != nil {
		return nil, us.logAndReturn("get upload", err, http.StatusInternalServerError)
	}

	return &schemas.UploadOut{Parts: parts}, nil
}

func (us *UploadService) DeleteUploadFile(c *gin.Context) (*schemas.Message, *types.AppError) {
	uploadId := c.Param("id")
	if err := us.Db.Where("upload_id = ?", uploadId).Delete(&models.Upload{}).Error; err != nil {
		return nil, us.logAndReturn("delete upload", err, http.StatusInternalServerError)
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
		index       int
		channelUser string
		out         *schemas.UploadPartOut
	)

	uploadQuery.PartNo = 1

	if err := c.ShouldBindQuery(&uploadQuery); err != nil {
		return nil, us.logAndReturn("UploadFile", err, http.StatusBadRequest)
	}

	var encryptedKey string

	if uploadQuery.Encrypted {
		encryptedKey = config.GetConfig().EncryptionKey

		if encryptedKey == "" {
			return nil, us.logAndReturn("UploadFile", errors.New("encryption key not set"), http.StatusInternalServerError)
		}
	}

	userId, session := getUserAuth(c)

	uploadId := c.Param("id")

	fileStream := c.Request.Body

	fileSize := c.Request.ContentLength

	defer c.Request.Body.Close()

	if uploadQuery.ChannelID == 0 {
		channelId, err = GetDefaultChannel(c, userId)
		if err != nil {
			return nil, us.logAndReturn("uploadFile", err, http.StatusInternalServerError)
		}
	} else {
		channelId = uploadQuery.ChannelID
	}

	tokens, err := getBotsToken(c, userId, channelId)

	if err != nil {
		return nil, us.logAndReturn("uploadFile", err, http.StatusInternalServerError)
	}

	if len(tokens) == 0 {
		client, _ = tgc.UserLogin(c, session)
		channelUser = strconv.FormatInt(userId, 10)
	} else {
		us.worker.Set(tokens, channelId)
		token, index = us.worker.Next(channelId)
		client, _ = tgc.BotLogin(c, token)
		channelUser = strings.Split(token, ":")[0]
	}

	us.log.Debug("uploading file", zap.String("fileName", uploadQuery.FileName),
		zap.String("partName", uploadQuery.PartName),
		zap.String("bot", channelUser), zap.Int("botNo", index),
		zap.Int("chunkNo", uploadQuery.PartNo), zap.Int64("partSize", fileSize))

	err = tgc.RunWithAuth(c, us.log, client, token, func(ctx context.Context) error {

		channel, err := GetChannelById(ctx, client, channelId, channelUser)

		if err != nil {
			us.log.Error("channel", zap.Error(err))
			return err
		}

		var salt string

		if uploadQuery.Encrypted {

			//gen random Salt
			salt, _ = generateRandomSalt()
			cipher, _ := crypt.NewCipher(encryptedKey, salt)
			fileSize = crypt.EncryptedSize(fileSize)
			fileStream, _ = cipher.EncryptData(fileStream)
		}

		api := client.API()

		u := uploader.NewUploader(api).WithThreads(16).WithPartSize(512 * 1024)

		upload, err := u.Upload(c, uploader.NewUpload(uploadQuery.PartName, fileStream, fileSize))

		if err != nil {
			return err
		}

		document := message.UploadedDocument(upload).Filename(uploadQuery.PartName).ForceFile(true)

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

		partUpload := &models.Upload{
			Name:      uploadQuery.PartName,
			UploadId:  uploadId,
			PartId:    message.ID,
			ChannelID: channelId,
			Size:      fileSize,
			PartNo:    uploadQuery.PartNo,
			UserId:    userId,
			Encrypted: uploadQuery.Encrypted,
			Salt:      salt,
		}

		if err := us.Db.Create(partUpload).Error; err != nil {
			//delete uploaded part if upload fails
			if message.ID != 0 {
				api.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{Channel: channel, ID: []int{message.ID}})
			}
			return err
		}

		out = mapper.ToUploadOut(partUpload)

		return nil
	})

	if err != nil {
		return nil, us.logAndReturn("uploadFile", err, http.StatusInternalServerError)
	}

	us.log.Debug("upload finished", zap.String("fileName", uploadQuery.FileName),
		zap.String("partName", uploadQuery.PartName),
		zap.Int("chunkNo", uploadQuery.PartNo))

	return out, nil
}

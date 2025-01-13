package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/pool"
	"github.com/tgdrive/teldrive/internal/tgc"
	"go.uber.org/zap"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/telegram/uploader"
	"github.com/gotd/td/tg"
	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/models"
)

const saltLength = 32

func (a *apiService) UploadsDelete(ctx context.Context, params api.UploadsDeleteParams) error {
	if err := a.db.Where("upload_id = ?", params.ID).Delete(&models.Upload{}).Error; err != nil {
		return &api.ErrorStatusCode{StatusCode: 500, Response: api.Error{Message: err.Error(), Code: 500}}
	}
	return nil
}

func (a *apiService) UploadsPartsById(ctx context.Context, params api.UploadsPartsByIdParams) ([]api.UploadPart, error) {
	parts := []models.Upload{}
	if err := a.db.Model(&models.Upload{}).Order("part_no").Where("upload_id = ?", params.ID).
		Where("created_at < ?", time.Now().UTC().Add(a.cnf.TG.Uploads.Retention)).
		Find(&parts).Error; err != nil {
		return nil, &apiError{err: err}
	}
	return mapper.ToUploadOut(parts), nil
}

func (a *apiService) UploadsStats(ctx context.Context, params api.UploadsStatsParams) ([]api.UploadStats, error) {
	userId := auth.GetUser(ctx)
	var stats []api.UploadStats
	err := a.db.Raw(`
    SELECT 
    dates.upload_date::date AS upload_date,
    COALESCE(SUM(files.size), 0)::bigint AS total_uploaded
    FROM 
        generate_series(
            (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date - INTERVAL '1 day' * @days,
            (CURRENT_TIMESTAMP AT TIME ZONE 'UTC')::date,
            '1 day'
        ) AS dates(upload_date)
    LEFT JOIN 
    teldrive.files AS files
    ON 
        dates.upload_date = DATE_TRUNC('day', files.created_at)::date
        AND files.type = 'file'
        AND files.user_id = @userId
    GROUP BY 
        dates.upload_date
    ORDER BY 
        dates.upload_date
  `, sql.Named("days", params.Days-1), sql.Named("userId", userId)).Scan(&stats).Error

	if err != nil {
		return nil, &apiError{err: err}

	}
	return stats, nil
}

func (a *apiService) UploadsUpload(ctx context.Context, req *api.UploadsUploadReqWithContentType, params api.UploadsUploadParams) (*api.UploadPart, error) {
	var (
		channelId   int64
		err         error
		client      *telegram.Client
		token       string
		index       int
		channelUser string
		out         api.UploadPart
	)

	if params.Encrypted.Value && a.cnf.TG.Uploads.EncryptionKey == "" {
		return nil, &apiError{err: errors.New("encryption is not enabled"), code: 400}
	}

	userId := auth.GetUser(ctx)

	fileStream := req.Content.Data

	fileSize := params.ContentLength

	if params.ChannelId.Value == 0 {
		channelId, err = getDefaultChannel(a.db, a.cache, userId)
		if err != nil {
			return nil, err
		}
	} else {
		channelId = params.ChannelId.Value
	}

	tokens, err := getBotsToken(a.db, a.cache, userId, channelId)

	if err != nil {
		return nil, err
	}

	if len(tokens) == 0 {
		client, err = tgc.AuthClient(ctx, &a.cnf.TG, auth.GetJWTUser(ctx).TgSession)
		if err != nil {
			return nil, err
		}
		channelUser = strconv.FormatInt(userId, 10)
	} else {
		a.worker.Set(tokens, channelId)
		token, index = a.worker.Next(channelId)
		client, err = tgc.BotClient(ctx, a.boltdb, &a.cnf.TG, token)

		if err != nil {
			return nil, err
		}

		channelUser = strings.Split(token, ":")[0]
	}

	middlewares := tgc.NewMiddleware(&a.cnf.TG, tgc.WithFloodWait(),
		tgc.WithRecovery(ctx),
		tgc.WithRetry(a.cnf.TG.Uploads.MaxRetries),
		tgc.WithRateLimit())

	uploadPool := pool.NewPool(client, int64(a.cnf.TG.PoolSize), middlewares...)

	defer uploadPool.Close()

	logger := logging.FromContext(ctx)

	logger.Debug("uploading chunk",
		zap.String("fileName", params.FileName),
		zap.String("partName", params.PartName),
		zap.String("bot", channelUser),
		zap.Int("botNo", index),
		zap.Int("chunkNo", params.PartNo),
		zap.Int64("partSize", fileSize),
	)

	err = tgc.RunWithAuth(ctx, client, token, func(ctx context.Context) error {

		channel, err := tgc.GetChannelById(ctx, client.API(), channelId)

		if err != nil {
			return err
		}

		var salt string

		if params.Encrypted.Value {
			//gen random Salt
			salt, _ = generateRandomSalt()
			cipher, err := crypt.NewCipher(a.cnf.TG.Uploads.EncryptionKey, salt)
			if err != nil {
				return err
			}
			fileSize = crypt.EncryptedSize(fileSize)
			fileStream, err = cipher.EncryptData(fileStream)
			if err != nil {
				return err
			}
		}

		client := uploadPool.Default(ctx)

		u := uploader.NewUploader(client).WithThreads(a.cnf.TG.Uploads.Threads).WithPartSize(512 * 1024)

		upload, err := u.Upload(ctx, uploader.NewUpload(params.PartName, fileStream, fileSize))

		if err != nil {
			return err
		}

		document := message.UploadedDocument(upload).Filename(params.PartName).ForceFile(true)

		sender := message.NewSender(client)

		target := sender.To(&tg.InputPeerChannel{ChannelID: channel.ChannelID,
			AccessHash: channel.AccessHash})

		res, err := target.Media(ctx, document)

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
			return fmt.Errorf("upload failed")
		}

		partUpload := &models.Upload{
			Name:      params.PartName,
			UploadId:  params.ID,
			PartId:    message.ID,
			ChannelId: channelId,
			Size:      fileSize,
			PartNo:    int(params.PartNo),
			UserId:    userId,
			Encrypted: params.Encrypted.Value,
			Salt:      salt,
		}

		if err := a.db.Create(partUpload).Error; err != nil {
			if message.ID != 0 {
				client.ChannelsDeleteMessages(ctx, &tg.ChannelsDeleteMessagesRequest{Channel: channel, ID: []int{message.ID}})
			}
			return err
		}

		msgs, _ := client.ChannelsGetMessages(ctx,
			&tg.ChannelsGetMessagesRequest{Channel: channel, ID: []tg.InputMessageClass{&tg.InputMessageID{ID: message.ID}}})

		if msgs != nil && len(msgs.(*tg.MessagesChannelMessages).Messages) == 0 {
			return errors.New("upload failed")
		}
		out = api.UploadPart{
			Name:      partUpload.Name,
			PartId:    partUpload.PartId,
			ChannelId: partUpload.ChannelId,
			PartNo:    partUpload.PartNo,
			Size:      partUpload.Size,
			Encrypted: partUpload.Encrypted,
		}
		out.SetSalt(api.NewOptString(partUpload.Salt))
		return nil

	})

	if err != nil {
		logger.Debug("upload failed", zap.String("fileName", params.FileName),
			zap.String("partName", params.PartName),
			zap.Int("chunkNo", params.PartNo))
		return nil, err
	}
	logger.Debug("upload finished", zap.String("fileName", params.FileName),
		zap.String("partName", params.PartName),
		zap.Int("chunkNo", params.PartNo))
	return &out, nil

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

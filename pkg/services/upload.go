package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strconv"
	"strings"

	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/utils"
	"go.uber.org/zap"

	"github.com/tgdrive/teldrive/pkg/mapper"
	"github.com/tgdrive/teldrive/pkg/repositories"
)

var (
	saltLength      = 32
	ErrUploadFailed = errors.New("upload failed")
)

func (a *apiService) UploadsDelete(ctx context.Context, params api.UploadsDeleteParams) error {
	if err := a.repo.Uploads.Delete(ctx, params.ID); err != nil {
		return &api.ErrorStatusCode{StatusCode: 500, Response: api.Error{Message: err.Error(), Code: 500}}
	}
	return nil
}

func (a *apiService) UploadsPartsById(ctx context.Context, params api.UploadsPartsByIdParams) ([]api.UploadPart, error) {
	parts, err := a.repo.Uploads.GetByUploadIDAndRetention(ctx, params.ID, a.cnf.TG.Uploads.Retention)
	if err != nil {
		return nil, &apiError{err: err}
	}
	return mapper.ToUploadOut(parts), nil
}

func (a *apiService) UploadsStats(ctx context.Context, params api.UploadsStatsParams) ([]api.UploadStats, error) {
	userId := auth.User(ctx)
	stats, err := a.repo.Uploads.StatsByDays(ctx, userId, params.Days)
	if err != nil {
		return nil, &apiError{err: err}
	}

	return utils.Map(stats, func(s repositories.UploadStat) api.UploadStats {
		return api.UploadStats{UploadDate: s.UploadDate, TotalUploaded: s.TotalUploaded}
	}), nil
}

func (a *apiService) prepareEncryption(encrypted bool, fileStream io.Reader, fileSize int64, logger *zap.Logger) (io.Reader, int64, string, error) {
	if !encrypted {
		return fileStream, fileSize, "", nil
	}
	salt, err := generateRandomSalt()
	if err != nil {
		return nil, 0, "", err
	}
	cipher, err := crypt.NewCipher(a.cnf.TG.Uploads.EncryptionKey, salt)
	if err != nil {
		return nil, 0, "", err
	}
	fileSize = crypt.EncryptedSize(fileSize)
	fileStream, err = cipher.EncryptData(fileStream)
	if err != nil {
		return nil, 0, "", err
	}
	return fileStream, fileSize, salt, nil
}

func (a *apiService) getUploadClient(ctx context.Context, userId int64) (TelegramClient, string, int, string, error) {
	tokens, err := a.channelManager.BotTokens(ctx, userId)
	if err != nil {
		return nil, "", 0, "", err
	}

	if len(tokens) == 0 {
		client, err := a.telegram.AuthClient(ctx, auth.JWTUser(ctx).TgSession, a.cnf.TG.Uploads.MaxRetries)
		if err != nil {
			return nil, "", 0, "", err
		}
		return client, "", 0, strconv.FormatInt(userId, 10), nil
	}

	token, index, err := a.telegram.SelectBotToken(ctx, TelegramOpUpload, userId, tokens)
	if err != nil {
		return nil, "", 0, "", err
	}
	client, err := a.telegram.BotClient(ctx, token, a.cnf.TG.Uploads.MaxRetries)
	if err != nil {
		return nil, "", 0, "", err
	}
	return client, token, index, strings.Split(token, ":")[0], nil
}

func (a *apiService) UploadsUpload(ctx context.Context, req *api.UploadsUploadReqWithContentType, params api.UploadsUploadParams) (*api.UploadPart, error) {
	if params.Encrypted.Value && a.cnf.TG.Uploads.EncryptionKey == "" {
		return nil, &apiError{err: errors.New("encryption is not enabled"), code: 400}
	}

	userId := auth.User(ctx)
	// Create upload component logger with common fields
	logger := logging.Component("UPLOAD").With(
		zap.String("file_name", params.FileName),
		zap.Int("part_no", params.PartNo),
		zap.Int64("size", params.ContentLength),
	)

	stager, err := a.newUploadStager(ctx, userId, params.ChannelId.Value)
	if err != nil {
		return nil, &apiError{err: err}
	}
	defer stager.Close()

	logger.Debug("upload.started",
		zap.String("bot", stager.channelUser),
		zap.Int("bot_no", stager.botIndex),
		zap.Int64("size", params.ContentLength),
		zap.Int64("channel_id", stager.channelID),
	)

	var out api.UploadPart
	err = stager.Run(ctx, func(ctx context.Context) error {
		partUpload, err := stager.StagePart(ctx, uploadStagePartRequest{
			UploadID:  params.ID,
			FileName:  params.FileName,
			PartNo:    params.PartNo,
			Reader:    req.Content.Data,
			Size:      params.ContentLength,
			Encrypted: params.Encrypted.Value,
			Hashing:   params.Hashing.Value,
			Threads:   a.cnf.TG.Uploads.Threads,
		}, logger)
		if err != nil {
			return err
		}

		out = api.UploadPart{
			Name:      partUpload.Name,
			PartId:    int(partUpload.PartID),
			ChannelId: partUpload.ChannelID,
			PartNo:    int(partUpload.PartNo),
			Size:      partUpload.Size,
			Encrypted: partUpload.Encrypted,
		}
		if partUpload.Salt != nil {
			out.SetSalt(api.NewOptString(*partUpload.Salt))
		}
		return nil
	})

	if err != nil {
		logger.Error("upload.failed", zap.String("file_name", params.FileName),
			zap.Int("part_no", params.PartNo), zap.Error(err))
		return nil, &apiError{err: err}
	}
	logger.Debug("upload.complete", zap.Int("message_id", out.PartId), zap.Int64("final_size", out.Size), zap.Bool("encrypted", out.Encrypted))
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

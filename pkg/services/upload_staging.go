package services

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/google/uuid"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	"github.com/tgdrive/teldrive/internal/hash"
	"go.uber.org/zap"
)

type uploadStager struct {
	api         *apiService
	userID      int64
	channelID   int64
	client      TelegramClient
	token       string
	botIndex    int
	channelUser string
	pool        UploadPool
}

type uploadStagePartRequest struct {
	UploadID  string
	FileName  string
	PartNo    int
	Reader    io.Reader
	Size      int64
	Encrypted bool
	Hashing   bool
	Threads   int
}

func (a *apiService) resolveUploadChannel(ctx context.Context, userID, requestedChannelID int64) (int64, error) {
	if requestedChannelID != 0 {
		return requestedChannelID, nil
	}

	channelID, err := a.channelManager.CurrentChannel(ctx, userID)
	if err == nil && (!a.cnf.TG.AutoChannelCreate || !a.channelManager.ChannelLimitReached(channelID)) {
		return channelID, nil
	}
	if err != nil && !a.telegram.IsNoDefaultChannelError(err) {
		return 0, err
	}

	return a.channelManager.CreateNewChannel(ctx, "", userID, true)
}

func (a *apiService) newUploadStager(ctx context.Context, userID, requestedChannelID int64) (*uploadStager, error) {
	channelID, err := a.resolveUploadChannel(ctx, userID, requestedChannelID)
	if err != nil {
		return nil, err
	}

	client, token, index, channelUser, err := a.getUploadClient(ctx, userID)
	if err != nil {
		return nil, err
	}

	pool, err := a.telegram.NewUploadPool(ctx, client, int64(a.cnf.TG.PoolSize), a.cnf.TG.Uploads.MaxRetries)
	if err != nil {
		return nil, err
	}

	return &uploadStager{
		api:         a,
		userID:      userID,
		channelID:   channelID,
		client:      client,
		token:       token,
		botIndex:    index,
		channelUser: channelUser,
		pool:        pool,
	}, nil
}

func (s *uploadStager) Close() {
	if s.pool != nil {
		s.pool.Close()
	}
}

func (s *uploadStager) Run(ctx context.Context, fn func(context.Context) error) error {
	return s.api.telegram.RunWithAuth(ctx, s.client, s.token, fn)
}

func (s *uploadStager) StagePart(ctx context.Context, req uploadStagePartRequest, logger *zap.Logger) (*jetmodel.Uploads, error) {
	reader := req.Reader
	fileSize := req.Size
	partName := s.generatePartName(req.FileName, req.PartNo)

	var (
		salt        string
		blockHasher *hash.BlockHasher
	)

	if req.Hashing {
		blockHasher = hash.NewBlockHasher()
		reader = io.TeeReader(reader, blockHasher)
	}

	fileStream, encryptedSize, salt, err := s.api.prepareEncryption(req.Encrypted, reader, fileSize, logger)
	if err != nil {
		return nil, err
	}

	messageID, telegramFileSize, err := s.api.telegram.UploadPart(
		ctx,
		s.pool.Default(ctx),
		s.channelID,
		partName,
		fileStream,
		encryptedSize,
		req.Threads,
	)
	if err != nil {
		return nil, err
	}

	if messageID == 0 || telegramFileSize != encryptedSize {
		return nil, ErrUploadFailed
	}

	var saltPtr *string
	if salt != "" {
		saltPtr = &salt
	}

	var blockHashesPtr *[]byte
	if blockHasher != nil {
		blockHashes := blockHasher.Sum()
		if len(blockHashes) > 0 {
			blockHashesPtr = &blockHashes
		}
	}

	partUpload := &jetmodel.Uploads{
		Name:        partName,
		UploadID:    req.UploadID,
		PartID:      int32(messageID),
		ChannelID:   s.channelID,
		Size:        encryptedSize,
		PartNo:      int32(req.PartNo),
		UserID:      &s.userID,
		Encrypted:   req.Encrypted,
		Salt:        saltPtr,
		BlockHashes: blockHashesPtr,
	}

	if err := s.api.repo.Uploads.Create(ctx, partUpload); err != nil {
		return nil, err
	}

	return partUpload, nil
}

func (s *uploadStager) generatePartName(fileName string, partNo int) string {
	if s.api.cnf.TG.Uploads.ChunkNaming == "deterministic" {
		if partNo <= 1 {
			return fileName
		}
		return fmt.Sprintf("%s.part.%03d", fileName, partNo)
	}

	hash := md5.Sum([]byte(uuid.New().String()))
	return hex.EncodeToString(hash[:])
}

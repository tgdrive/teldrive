package cron

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"time"

	"github.com/divyam234/teldrive/config"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/database"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/services"
	"github.com/go-co-op/gocron"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
)

type Files []File
type File struct {
	ID    string        `json:"id"`
	Parts []models.Part `json:"parts"`
}

func (a Files) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *Files) Scan(value interface{}) error {
	if err := json.Unmarshal(value.([]byte), &a); err != nil {
		return err
	}
	return nil
}

type UpParts []int

func (a UpParts) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *UpParts) Scan(value interface{}) error {
	if err := json.Unmarshal(value.([]byte), &a); err != nil {
		return err
	}
	return nil
}

type Result struct {
	Files     Files
	Session   string
	UserId    int64
	ChannelId int64
}

type UploadResult struct {
	Parts     UpParts
	Session   string
	UserId    int64
	ChannelId int64
}

func deleteTGMessages(ctx context.Context, logger *zap.Logger, result Result) error {

	db := database.DB

	client, err := tgc.UserLogin(ctx, result.Session)

	if err != nil {
		return err
	}

	ids := []int{}

	fileIds := []string{}

	for _, file := range result.Files {
		fileIds = append(fileIds, file.ID)
		for _, part := range file.Parts {
			ids = append(ids, int(part.ID))
		}

	}

	err = tgc.RunWithAuth(ctx, logger, client, "", func(ctx context.Context) error {

		channel, err := services.GetChannelById(ctx, client, result.ChannelId, strconv.FormatInt(result.UserId, 10))

		if err != nil {
			return err
		}

		messageDeleteRequest := tg.ChannelsDeleteMessagesRequest{Channel: channel, ID: ids}

		_, err = client.API().ChannelsDeleteMessages(ctx, &messageDeleteRequest)
		if err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		db.Where("id = any($1)", fileIds).Delete(&models.File{})
	}

	return err
}

func cleanUploadsMessages(ctx context.Context, logger *zap.Logger, result UploadResult) error {

	db := database.DB

	client, err := tgc.UserLogin(ctx, result.Session)

	if err != nil {
		return err
	}

	err = tgc.RunWithAuth(ctx, logger, client, "", func(ctx context.Context) error {

		channel, err := services.GetChannelById(ctx, client, result.ChannelId, strconv.FormatInt(result.UserId, 10))

		if err != nil {
			return err
		}

		messageDeleteRequest := tg.ChannelsDeleteMessagesRequest{Channel: channel, ID: result.Parts}
		_, err = client.API().ChannelsDeleteMessages(ctx, &messageDeleteRequest)
		if err != nil {
			return err
		}
		return nil
	})
	parts := []int{}
	for _, id := range result.Parts {
		parts = append(parts, id)
	}

	if err == nil {
		db.Where("part_id = any($1)", parts).Delete(&models.Upload{})
	}

	return err
}

func filesDeleteJob(logger *zap.Logger) {
	db := database.DB
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	var results []Result
	if err := db.Model(&models.File{}).
		Select("JSONB_AGG(jsonb_build_object('id',files.id, 'parts',files.parts)) as files", "files.channel_id", "files.user_id", "s.session").
		Joins("left join teldrive.users as u  on u.user_id = files.user_id").
		Joins("left join (select * from teldrive.sessions order by created_at desc limit 1) as s on u.user_id = s.user_id").
		Where("type = ?", "file").
		Where("status = ?", "pending_deletion").
		Group("files.channel_id").Group("files.user_id").Group("s.session").
		Scan(&results).Error; err != nil {
		return
	}

	for _, row := range results {
		err := deleteTGMessages(ctx, logger, row)
		if err != nil {
			logger.Error("failed to clean pending files", zap.Int64("user", row.UserId), zap.Error(err))
		}
		logger.Info("cleaned pending files", zap.Int64("user", row.UserId),
			zap.Int64("channel", row.ChannelId))
	}
}

func uploadCleanJob(logger *zap.Logger) {
	db := database.DB
	ctx, cancel := context.WithCancel(context.Background())
	c := config.GetConfig()

	defer cancel()

	var upResults []UploadResult
	if err := db.Model(&models.Upload{}).
		Select("JSONB_AGG(uploads.part_id) as parts", "uploads.channel_id", "uploads.user_id", "s.session").
		Joins("left join teldrive.users as u  on u.user_id = uploads.user_id").
		Joins("left join (select * from teldrive.sessions order by created_at desc limit 1) as s on s.user_id = uploads.user_id").
		Where("uploads.created_at < ?", time.Now().UTC().AddDate(0, 0, -c.UploadRetention)).
		Group("uploads.channel_id").Group("uploads.user_id").Group("s.session").
		Scan(&upResults).Error; err != nil {
		return
	}
	for _, row := range upResults {
		err := cleanUploadsMessages(ctx, logger, row)
		if err != nil {
			logger.Error("failed to clean orpahan file parts", zap.Int64("user", row.UserId), zap.Error(err))
		}
		logger.Info("cleaned orpahan file parts", zap.Int64("user", row.UserId),
			zap.Int64("channel", row.ChannelId))
	}
}

func folderSizeUpdate(logger *zap.Logger) {
	database.DB.Exec("call teldrive.update_size();")
	logger.Info("updated folder sizes")

}

func StartCronJobs(logger *zap.Logger) {
	scheduler := gocron.NewScheduler(time.UTC)

	scheduler.Every(1).Hour().Do(filesDeleteJob, logger)

	scheduler.Every(12).Hour().Do(uploadCleanJob, logger)

	scheduler.Every(2).Hour().Do(folderSizeUpdate, logger)

	scheduler.StartAsync()
}

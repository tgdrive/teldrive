package cron

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"time"

	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/tgc"
	"github.com/divyam234/teldrive/pkg/logging"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/services"
	"github.com/go-co-op/gocron"
	"github.com/gotd/td/tg"
	"go.uber.org/zap"
	"gorm.io/gorm"
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

type CronService struct {
	db     *gorm.DB
	cnf    *config.Config
	logger *zap.SugaredLogger
}

func StartCronJobs(db *gorm.DB, cnf *config.Config) {
	scheduler := gocron.NewScheduler(time.UTC)

	ctx := context.Background()

	cron := CronService{db: db, cnf: cnf, logger: logging.DefaultLogger()}

	scheduler.Every(1).Hour().Do(cron.CleanFiles, ctx)

	scheduler.Every(2).Hour().Do(cron.UpdateFolderSize)

	scheduler.Every(12).Hour().Do(cron.CleanUploads, ctx)

	scheduler.StartAsync()
}

func (c *CronService) CleanFiles(ctx context.Context) {

	var results []Result
	if err := c.db.Model(&models.File{}).
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
		ids := []int{}

		fileIds := []string{}

		for _, file := range row.Files {
			fileIds = append(fileIds, file.ID)
			for _, part := range file.Parts {
				ids = append(ids, int(part.ID))
			}

		}
		err := deleteTGMessages(ctx, c.cnf, row.Session, row.ChannelId, row.UserId, ids)
		if err != nil {
			c.logger.Errorw("failed to clean files", err)
		}
		if err == nil {
			c.db.Where("id = any($1)", fileIds).Delete(&models.File{})
		}
		c.logger.Infow("cleaned files", "user", row.UserId, "channel", row.ChannelId)
	}
}

func (c *CronService) CleanUploads(ctx context.Context) {

	var upResults []UploadResult
	if err := c.db.Model(&models.Upload{}).
		Select("JSONB_AGG(uploads.part_id) as parts", "uploads.channel_id", "uploads.user_id", "s.session").
		Joins("left join teldrive.users as u  on u.user_id = uploads.user_id").
		Joins("left join (select * from teldrive.sessions order by created_at desc limit 1) as s on s.user_id = uploads.user_id").
		Where("uploads.created_at > ?", time.Now().UTC().Add(c.cnf.TG.Uploads.Retention)).
		Group("uploads.channel_id").Group("uploads.user_id").Group("s.session").
		Scan(&upResults).Error; err != nil {
		return
	}

	for _, result := range upResults {
		err := deleteTGMessages(ctx, c.cnf, result.Session, result.ChannelId, result.UserId, result.Parts)
		c.logger.Errorw("failed to delete messages", err)
		parts := []int{}
		for _, id := range result.Parts {
			parts = append(parts, id)
		}

		if err == nil {
			c.db.Where("part_id = any($1)", parts).Delete(&models.Upload{})
		}
	}
}

func (c *CronService) UpdateFolderSize() {
	c.db.Exec("call teldrive.update_size();")
}

func deleteTGMessages(ctx context.Context, cnf *config.Config, session string, channelId, userId int64, ids []int) error {

	client, _ := tgc.AuthClient(ctx, &cnf.TG, session)

	err := tgc.RunWithAuth(ctx, client, "", func(ctx context.Context) error {

		channel, err := services.GetChannelById(ctx, client, channelId, strconv.FormatInt(userId, 10))

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
	return err
}

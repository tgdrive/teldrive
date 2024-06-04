package cron

import (
	"context"
	"time"

	"github.com/divyam234/teldrive/internal/config"
	"github.com/divyam234/teldrive/internal/logging"
	"github.com/divyam234/teldrive/pkg/models"
	"github.com/divyam234/teldrive/pkg/schemas"
	"github.com/divyam234/teldrive/pkg/services"
	"github.com/go-co-op/gocron"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type File struct {
	ID    string         `json:"id"`
	Parts []schemas.Part `json:"parts"`
}

type Result struct {
	Files     datatypes.JSONSlice[File]
	Session   string
	UserId    int64
	ChannelId int64
}

type UploadResult struct {
	Parts     datatypes.JSONSlice[int]
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

		if row.Session == "" {
			break
		}
		ids := []int{}

		fileIds := []string{}

		for _, file := range row.Files {
			fileIds = append(fileIds, file.ID)
			for _, part := range file.Parts {
				ids = append(ids, int(part.ID))
			}

		}
		err := services.DeleteTGMessages(ctx, &c.cnf.TG, row.Session, row.ChannelId, row.UserId, ids)
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
		Where("uploads.created_at < ?", time.Now().UTC().Add(-c.cnf.TG.Uploads.Retention)).
		Group("uploads.channel_id").Group("uploads.user_id").Group("s.session").
		Scan(&upResults).Error; err != nil {
		return
	}

	for _, result := range upResults {

		if result.Session == "" && len(result.Parts) > 0 {
			c.db.Where("part_id = any($1)", result.Parts).Delete(&models.Upload{})
			break
		}
		err := services.DeleteTGMessages(ctx, &c.cnf.TG, result.Session, result.ChannelId, result.UserId, result.Parts)
		c.logger.Errorw("failed to delete messages", err)

		if err == nil {
			c.db.Where("part_id = any($1)", result.Parts).Delete(&models.Upload{})
		}
	}
}

func (c *CronService) UpdateFolderSize() {
	c.db.Exec("call teldrive.update_size();")
}

package cron

import (
	"context"
	"time"

	gormlock "github.com/go-co-op/gocron-gorm-lock/v2"
	"github.com/go-co-op/gocron/v2"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/pkg/models"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type file struct {
	ID    string     `json:"id"`
	Parts []api.Part `json:"parts"`
}

type result struct {
	Files     datatypes.JSONSlice[file]
	ChannelId int64
	UserId    int64
	Session   string
}

type uploadResult struct {
	Parts     datatypes.JSONSlice[int]
	Session   string
	UserId    int64
	ChannelId int64
}

type CronService struct {
	db     *gorm.DB
	cnf    *config.ServerCmdConfig
	logger *zap.Logger
}

func StartCronJobs(ctx context.Context, db *gorm.DB, cnf *config.ServerCmdConfig) error {

	err := db.AutoMigrate(&gormlock.CronJobLock{})
	if err != nil {
		return err
	}

	locker, err := gormlock.NewGormLocker(db, cnf.CronJobs.LockerInstance,
		gormlock.WithCleanInterval(time.Hour*12))

	if err != nil {
		return err
	}

	scheduler, err := gocron.NewScheduler(gocron.WithLocation(time.UTC),
		gocron.WithDistributedLocker(locker))

	if err != nil {
		return err
	}

	cron := CronService{db: db, cnf: cnf, logger: logging.Component("CRON")}
	_, err = scheduler.NewJob(gocron.DurationJob(cnf.CronJobs.CleanFilesInterval),
		gocron.NewTask(cron.cleanFiles, ctx))
	if err != nil {
		return err
	}
	_, err = scheduler.NewJob(gocron.DurationJob(cnf.CronJobs.FolderSizeInterval),
		gocron.NewTask(cron.updateFolderSize))
	if err != nil {
		return err
	}
	_, err = scheduler.NewJob(gocron.DurationJob(cnf.CronJobs.CleanUploadsInterval),
		gocron.NewTask(cron.cleanUploads, ctx))
	if err != nil {
		return err
	}
	_, err = scheduler.NewJob(gocron.DurationJob(time.Hour*12),
		gocron.NewTask(cron.cleanOldEvents))
	if err != nil {
		return err
	}

	scheduler.Start()
	return nil
}

func (c *CronService) cleanFiles(ctx context.Context) {
	c.logger.Info("cron.clean_files.started")
	var results []result
	if err := c.db.Table("teldrive.files as f").
		Select("JSONB_AGG(jsonb_build_object('id', f.id, 'parts', f.parts)) as files,f.channel_id,f.user_id,s.session").
		Joins("LEFT JOIN teldrive.users as u ON u.user_id = f.user_id").
		Joins(`LEFT JOIN (
        SELECT user_id, session
        FROM teldrive.sessions
        WHERE created_at = (
            SELECT MAX(created_at)
            FROM teldrive.sessions s2
            WHERE s2.user_id = sessions.user_id
        )
    ) as s ON u.user_id = s.user_id`).
		Where("f.type = ?", "file").
		Where("f.status = ?", "pending_deletion").
		Group("f.channel_id").
		Group("f.user_id").
		Group("s.session").
		Scan(&results).Error; err != nil {
		return
	}

	middlewares := tgc.NewMiddleware(&c.cnf.TG, tgc.WithFloodWait(), tgc.WithRateLimit())

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

		client, _ := tgc.AuthClient(ctx, &c.cnf.TG, row.Session, middlewares...)
		err := tgc.DeleteMessages(ctx, client, row.ChannelId, ids)

		if err != nil {
			c.logger.Error("cron.file_delete_failed", zap.Error(err), zap.Int64("channel_id", row.ChannelId))
			return
		}

		items := pgtype.Array[string]{
			Elements: fileIds,
			Valid:    true,
			Dims:     []pgtype.ArrayDimension{{Length: int32(len(fileIds)), LowerBound: 1}},
		}

		c.db.Where("id = any($1)", items).Delete(&models.File{})

		c.logger.Info("cron.files_cleaned", zap.Int64("user_id", row.UserId), zap.Int64("channel_id", row.ChannelId), zap.Int("file_count", len(fileIds)))
	}
}

func (c *CronService) cleanUploads(ctx context.Context) {
	c.logger.Info("cron.clean_uploads.started")
	var results []uploadResult
	if err := c.db.Table("teldrive.uploads as up").
		Select("JSONB_AGG(up.part_id) as parts,up.channel_id,up.user_id,s.session").
		Joins("LEFT JOIN teldrive.users as u ON u.user_id = up.user_id").
		Joins(`LEFT JOIN (
        SELECT user_id, session
        FROM teldrive.sessions
        WHERE created_at = (
            SELECT MAX(created_at)
            FROM teldrive.sessions s2
            WHERE s2.user_id = sessions.user_id
        )
    ) as s ON u.user_id = s.user_id`).
		Where("up.created_at < ?", time.Now().UTC().Add(-c.cnf.TG.Uploads.Retention)).
		Group("up.channel_id").
		Group("up.user_id").
		Group("s.session").
		Scan(&results).Error; err != nil {
		return
	}

	middlewares := tgc.NewMiddleware(&c.cnf.TG, tgc.WithFloodWait(), tgc.WithRateLimit())
	for _, result := range results {

		if result.Session != "" && len(result.Parts) > 0 {
			client, _ := tgc.AuthClient(ctx, &c.cnf.TG, result.Session, middlewares...)

			err := tgc.DeleteMessages(ctx, client, result.ChannelId, result.Parts)
			if err != nil {
				c.logger.Error("failed to delete messages", zap.Error(err))
				return
			}
		}
		items := pgtype.Array[int]{
			Elements: result.Parts,
			Valid:    true,
			Dims:     []pgtype.ArrayDimension{{Length: int32(len(result.Parts)), LowerBound: 1}},
		}
		c.db.Where("part_id = any(?)", items).Where("channel_id = ?", result.ChannelId).
			Where("user_id = ?", result.UserId).Delete(&models.Upload{}).Delete(&models.Upload{})

	}
}

func (c *CronService) updateFolderSize() {
	c.logger.Info("cron.folder_size.started")
	query := `
	WITH RECURSIVE folder_hierarchy AS (
		SELECT id, id as root_id
		FROM teldrive.files
		WHERE type = 'folder'
		UNION ALL
		SELECT f.id, fh.root_id
		FROM teldrive.files f
		JOIN folder_hierarchy fh ON f.parent_id = fh.id
	),
	folder_sizes AS (
		SELECT root_id, COALESCE(SUM(size), 0) as total_size
		FROM folder_hierarchy fh
		JOIN teldrive.files f ON fh.id = f.id
		WHERE f.type = 'file' AND f.status = 'active'
		GROUP BY root_id
	)
	UPDATE teldrive.files f
	SET size = fs.total_size
	FROM folder_sizes fs
	WHERE f.id = fs.root_id;
	`
	c.db.Exec(query)
}

func (c *CronService) cleanOldEvents() {
	c.db.Exec("DELETE FROM teldrive.events WHERE created_at < NOW() - INTERVAL '5 days';")
}

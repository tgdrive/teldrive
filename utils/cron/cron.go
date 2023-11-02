package cron

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"strconv"
	"time"

	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/services"
	"github.com/divyam234/teldrive/utils/tgc"
	"github.com/gotd/td/tg"
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

type UpFiles []UpFile
type UpFile struct {
	ID     string `json:"id"`
	PartID int    `json:"partId"`
}

func (a UpFiles) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *UpFiles) Scan(value interface{}) error {
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
	Files     UpFiles
	Session   string
	UserId    int64
	ChannelId int64
}

func deleteTGMessages(ctx context.Context, result Result) error {

	db := database.DB

	client, err := tgc.UserLogin(result.Session)

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

	err = tgc.RunWithAuth(ctx, client, "", func(ctx context.Context) error {

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

	return nil
}

func cleanUploadsMessages(ctx context.Context, result UploadResult) error {

	db := database.DB

	client, err := tgc.UserLogin(result.Session)

	if err != nil {
		return err
	}

	ids := []int{}

	fileIds := []string{}

	for _, file := range result.Files {
		fileIds = append(fileIds, file.ID)
		ids = append(ids, file.PartID)

	}
	err = tgc.RunWithAuth(ctx, client, "", func(ctx context.Context) error {

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
		db.Where("id = any($1)", fileIds).Delete(&models.Upload{})
	}

	return nil
}

func FilesDeleteJob() {
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
		deleteTGMessages(ctx, row)
	}
}

func UploadCleanJob() {
	db := database.DB
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	var upResults []UploadResult
	if err := db.Model(&models.Upload{}).
		Select("JSONB_AGG(jsonb_build_object('id',uploads.id,'partId',uploads.part_id)) as files", "uploads.channel_id", "uploads.user_id", "s.session").
		Joins("left join teldrive.users as u  on u.user_id = uploads.user_id").
		Joins("left join (select * from teldrive.sessions order by created_at desc limit 1) as s on s.user_id = uploads.user_id").
		Where("uploads.created_at < ?", time.Now().UTC().AddDate(0, 0, -15)).
		Group("uploads.channel_id").Group("uploads.user_id").Group("s.session").
		Scan(&upResults).Error; err != nil {
		return
	}

	for _, row := range upResults {
		cleanUploadsMessages(ctx, row)
	}
}

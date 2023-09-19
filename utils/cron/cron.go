package cron

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"strconv"

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

type Result struct {
	Files     Files
	TgSession string
	UserId    int64
	ChannelId int64
}

func deleteTGMessages(ctx context.Context, result Result) error {

	db := database.DB

	client, err := tgc.UserLogin(result.TgSession)

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

func FilesDeleteJob() {
	db := database.DB
	ctx, cancel := context.WithCancel(context.Background())

	defer cancel()

	var results []Result
	if err := db.Model(&models.File{}).
		Select("JSONB_AGG(jsonb_build_object('id',files.id, 'parts',files.parts)) as files", "files.channel_id", "files.user_id", "u.tg_session").
		Joins("left join teldrive.users as u  on u.user_id = files.user_id").
		Where("type = ?", "file").
		Where("status = ?", "pending_deletion").
		Group("files.channel_id").Group("files.user_id").Group("u.tg_session").
		Scan(&results).Error; err != nil {
		return
	}

	for _, row := range results {
		deleteTGMessages(ctx, row)
	}
}

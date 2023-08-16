package cron

import (
	"context"
	"fmt"

	"github.com/divyam234/teldrive/cache"
	"github.com/divyam234/teldrive/database"
	"github.com/divyam234/teldrive/models"
	"github.com/divyam234/teldrive/utils"
	"github.com/gotd/td/tg"
)

type Result struct {
	ID        string
	Parts     models.Parts
	TgSession string
	UserId    int
	ChannelId int64
}

func deleteTGMessage(ctx context.Context, client *tg.Client, result Result) error {

	ids := make([]int, len(result.Parts))

	for i, part := range result.Parts {
		ids[i] = int(part.ID)
	}

	res, err := cache.CachedFunction(utils.GetChannelById, fmt.Sprintf("channels:%d", result.ChannelId))(ctx, client, result.ChannelId)

	if err != nil {
		return err
	}

	channel := res.(*tg.Channel)

	messageDeleteRequest := tg.ChannelsDeleteMessagesRequest{Channel: &tg.InputChannel{ChannelID: result.ChannelId, AccessHash: channel.AccessHash},
		ID: ids}

	_, err = client.ChannelsDeleteMessages(ctx, &messageDeleteRequest)
	if err != nil {
		return err
	}
	return nil
}

func FilesDeleteJob() {
	db := database.DB
	ctx := context.Background()
	var results []Result
	if err := db.Model(&models.File{}).Select("files.id", "files.parts", "files.user_id", "files.channel_id", "u.tg_session").
		Joins("left join teldrive.users as u  on u.user_id = files.user_id").
		Where("status = ?", "pending_deletion").Scan(&results).Error; err != nil {
		return
	}

	for _, file := range results {
		client, stop, err := utils.GetAuthClient(file.TgSession, file.UserId)
		if err != nil {
			break
		}
		if stop != nil {
			defer func() {
				utils.StopClient(stop, file.UserId)
			}()
		}

		err = deleteTGMessage(ctx, client.Tg.API(), file)

		if err == nil {
			db.Where("id = ?", file.ID).Delete(&models.File{})
		}
	}
}

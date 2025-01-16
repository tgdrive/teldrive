package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/k0kubun/go-ansi"
	"github.com/manifoldco/promptui"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"golang.org/x/term"
	"gorm.io/datatypes"
)

type channel struct {
	tg.InputPeerChannel
	ChannelName string
}

type file struct {
	ID    string
	Name  string
	Parts datatypes.JSONSlice[api.Part]
}

type exportFile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type channelExport struct {
	ChannelID int64        `json:"channel_id"`
	Timestamp string       `json:"timestamp"`
	FileCount int          `json:"file_count"`
	Files     []exportFile `json:"files"`
}

const dateLayout = "2006-01-02_15-04-05"

var termWidth = func() (width int, err error) {
	width, _, err = term.GetSize(int(os.Stdout.Fd()))
	if err == nil {
		return width, nil
	}

	return 0, err
}

func NewCheckCmd() *cobra.Command {
	var cfg config.ServerCmdConfig
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check and purge incomplete files",
		Run: func(cmd *cobra.Command, args []string) {
			runCheckCmd(cmd, &cfg)
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			loader := config.NewConfigLoader()
			if err := loader.InitializeConfig(cmd); err != nil {
				return err
			}
			if err := loader.Load(&cfg); err != nil {
				return err
			}
			if err := checkRequiredCheckFlags(&cfg); err != nil {
				return err
			}
			return nil
		},
	}
	addChecktFlags(cmd, &cfg)
	return cmd
}

func addChecktFlags(cmd *cobra.Command, cfg *config.ServerCmdConfig) {
	flags := cmd.Flags()
	config.AddCommonFlags(flags, cfg)
	flags.Bool("export", true, "Export incomplete files to json file")
	flags.Bool("clean", false, "Clean missing and orphan file parts")
	flags.String("user", "", "Telegram User Name")
}

func checkRequiredCheckFlags(cfg *config.ServerCmdConfig) error {
	var missingFields []string

	if cfg.DB.DataSource == "" {
		missingFields = append(missingFields, "db-data-source")
	}
	if len(missingFields) > 0 {
		return fmt.Errorf("required configuration values not set: %s", strings.Join(missingFields, ", "))
	}

	return nil
}

func selectUser(user string, users []models.User) (*models.User, error) {
	if user != "" {
		res := utils.Filter(users, func(u models.User) bool {
			return u.UserName == user
		})
		if len(res) == 0 {
			return nil, fmt.Errorf("invalid user name: %s", user)
		}
		return &res[0], nil
	}
	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "{{ .UserName | cyan }}",
		Inactive: "{{ .UserName | white }}",
		Selected: "{{ .UserName | red | cyan }}",
	}

	prompt := promptui.Select{
		Label:     "Select User",
		Items:     users,
		Templates: templates,
		Size:      50,
	}

	index, _, err := prompt.Run()
	if err != nil {
		return nil, err
	}
	return &users[index], nil
}

func runCheckCmd(cmd *cobra.Command, cfg *config.ServerCmdConfig) {

	lg := logging.DefaultLogger().Sugar()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)

	defer func() {
		stop()
		logging.DefaultLogger().Sync()
	}()

	db, err := database.NewDatabase(&cfg.DB, lg)

	if err != nil {
		lg.Fatalw("failed to create database", "err", err)
	}

	users := []models.User{}
	if err := db.Model(&models.User{}).Find(&users).Error; err != nil {
		lg.Fatalw("failed to get users", "err", err)
	}
	userName, _ := cmd.Flags().GetString("user")
	user, err := selectUser(userName, users)
	if err != nil {
		lg.Fatalw("failed to select user", "err", err)
	}
	session := models.Session{}
	if err := db.Model(&models.Session{}).Where("user_id = ?", user.UserId).Order("created_at desc").First(&session).Error; err != nil {
		lg.Fatalw("failed to get session", "err", err)
	}
	channelIds := []int64{}

	if err := db.Model(&models.Channel{}).Where("user_id = ?", user.UserId).Pluck("channel_id", &channelIds).Error; err != nil {
		lg.Fatalw("failed to get channels", "err", err)
	}
	if len(channelIds) == 0 {
		lg.Fatalw("no channels found")
	}

	tgconfig := &config.TGConfig{
		RateLimit: true,
		RateBurst: 5,
		Rate:      100,
	}

	middlewares := tgc.NewMiddleware(tgconfig, tgc.WithFloodWait(), tgc.WithRateLimit())

	export, _ := cmd.Flags().GetBool("export")

	clean, _ := cmd.Flags().GetBool("clean")

	var channelExports []channelExport
	for _, id := range channelIds {
		lg.Info("Processing channel: ", id)
		const batchSize = 1000
		var offset int
		var files []file

		for {
			var batch []file
			result := db.Model(&models.File{}).
				Offset(offset).
				Limit(batchSize).
				Where("user_id = ?", user.UserId).
				Where("channel_id = ?", id).
				Where("type = ?", "file").
				Scan(&batch)
			if result.Error != nil {
				lg.Errorw("failed to load files", "err", result.Error)
				break
			}

			files = append(files, batch...)
			if len(batch) < batchSize {
				break
			}
			offset += batchSize
		}
		if len(files) == 0 {
			continue
		}

		lg.Infof("Channel %d: %d files found", id, len(files))

		lg.Infof("Loading messages from telegram")

		client, err := tgc.AuthClient(ctx, tgconfig, session.Session, middlewares...)

		if err != nil {
			lg.Fatalw("failed to create client", "err", err)
		}

		msgs, total, err := loadChannelMessages(ctx, client, id)
		if err != nil {
			lg.Fatalw("failed to load channel messages", "err", err)
		}
		if total == 0 && len(msgs) == 0 {
			lg.Infof("Channel %d: no messages found", id)
			continue
		}
		if len(msgs) < total {
			lg.Fatalf("Channel %d: found %d messages out of %d", id, len(msgs), total)
			continue
		}
		uploadPartIds := []int{}
		if err := db.Model(&models.Upload{}).Where("user_id = ?", user.UserId).Where("channel_id = ?", id).
			Pluck("part_id", &uploadPartIds).Error; err != nil {
			lg.Errorw("failed to get upload part ids", "err", err)
		}

		uploadPartMap := make(map[int]bool)

		for _, partID := range uploadPartIds {
			uploadPartMap[partID] = true
		}
		msgMap := make(map[int]bool)
		for _, m := range msgs {
			if m > 0 && !uploadPartMap[m] {
				msgMap[m] = true
			}
		}
		filesWithMissingParts := []file{}
		allPartIDs := make(map[int]bool)
		for _, f := range files {
			for _, p := range f.Parts {
				if p.ID == 0 {
					filesWithMissingParts = append(filesWithMissingParts, f)
					break
				}
				allPartIDs[p.ID] = true
			}
		}
		if len(allPartIDs) == 0 {
			continue
		}

		for _, f := range files {
			for _, p := range f.Parts {
				if !msgMap[p.ID] {
					filesWithMissingParts = append(filesWithMissingParts, f)
					break
				}
			}

		}
		missingMsgIDs := []int{}
		for msgID := range msgMap {
			if !allPartIDs[msgID] {
				missingMsgIDs = append(missingMsgIDs, msgID)
			}
		}

		if len(filesWithMissingParts) > 0 {
			lg.Infof("Channel %d: found %d files with missing parts", id, len(filesWithMissingParts))
		}

		if export && len(filesWithMissingParts) > 0 {
			channelData := channelExport{
				ChannelID: id,
				Timestamp: time.Now().Format(time.RFC3339),
				FileCount: len(filesWithMissingParts),
				Files:     make([]exportFile, 0, len(filesWithMissingParts)),
			}

			for _, f := range filesWithMissingParts {
				channelData.Files = append(channelData.Files, exportFile{
					ID:   f.ID,
					Name: f.Name,
				})
			}
			if clean {
				err = db.Exec("call teldrive.delete_files_bulk($1 , $2)",
					utils.Map(filesWithMissingParts, func(f file) string { return f.ID }), user.UserId).Error
				if err != nil {
					lg.Errorw("failed to delete files", "err", err)
				}
			}

			channelExports = append(channelExports, channelData)
		}
		if clean && len(missingMsgIDs) > 0 {
			lg.Infof("Channel %d: cleaning %d orphan messages", id, len(missingMsgIDs))
			tgc.DeleteMessages(ctx, client, id, missingMsgIDs)
		}

	}
	if len(channelExports) > 0 {
		jsonData, err := json.MarshalIndent(channelExports, "", "    ")
		if err != nil {
			lg.Errorw("failed to marshal JSON", "err", err)
			return
		}
		err = os.WriteFile("missing_files.json", jsonData, 0644)
		if err != nil {
			lg.Errorw("failed to write JSON file", "err", err)
			return
		}
		lg.Infof("Exported data to missing_files.json")
	}

}

func loadChannelMessages(ctx context.Context, client *telegram.Client, channelId int64) (msgs []int, total int, err error) {
	errChan := make(chan error, 1)

	go func() {
		errChan <- tgc.RunWithAuth(ctx, client, "", func(ctx context.Context) error {
			channel, err := tgc.GetChannelById(ctx, client.API(), channelId)
			if err != nil {
				return err
			}

			count := 0

			q := query.NewQuery(client.API()).Messages().GetHistory(&tg.InputPeerChannel{
				ChannelID:  channelId,
				AccessHash: channel.AccessHash,
			})

			msgiter := messages.NewIterator(q, 100)

			total, err = msgiter.Total(ctx)
			if err != nil {
				return fmt.Errorf("failed to get total messages: %w", err)
			}

			width, err := termWidth()
			if err != nil {
				width = 50
			}
			bar := progressbar.NewOptions(
				total,
				progressbar.OptionSetWriter(ansi.NewAnsiStdout()),
				progressbar.OptionEnableColorCodes(true),
				progressbar.OptionThrottle(65*time.Millisecond),
				progressbar.OptionShowCount(),
				progressbar.OptionSetWidth(width/3),
				progressbar.OptionShowIts(),
				progressbar.OptionSetTheme(progressbar.Theme{
					Saucer:        "[green]=[reset]",
					SaucerHead:    "[green]>[reset]",
					SaucerPadding: " ",
					BarStart:      "[",
					BarEnd:        "]",
				}),
			)

			defer bar.Clear()

			for msgiter.Next(ctx) {
				msg := msgiter.Value()
				msgs = append(msgs, msg.Msg.GetID())
				count++
				bar.Set(count)
			}
			return nil
		})
	}()

	select {
	case err = <-errChan:
		if err != nil {
			return
		}
	case <-ctx.Done():
		fmt.Print("\r\033[K")
		err = ctx.Err()
	}
	return
}

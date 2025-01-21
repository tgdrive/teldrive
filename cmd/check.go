package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var termWidth = func() (width int, err error) {
	width, _, err = term.GetSize(int(os.Stdout.Fd()))
	if err == nil {
		return width, nil
	}

	return 0, err
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

type channelProcessor struct {
	id             int64
	files          []file
	missingFiles   []file
	orphanMessages []int
	totalCount     int64
	pw             progress.Writer
	tracker        *progress.Tracker
	channelExport  *channelExport
	client         *telegram.Client
	ctx            context.Context
	db             *gorm.DB
	userId         int64
	clean          bool
}

func NewCheckCmd() *cobra.Command {
	var cfg config.ServerCmdConfig
	loader := config.NewConfigLoader()
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check and purge incomplete files",
		Run: func(cmd *cobra.Command, args []string) {
			runCheckCmd(cmd, &cfg)
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := loader.Load(cmd, &cfg); err != nil {
				return err
			}
			if err := checkRequiredCheckFlags(&cfg); err != nil {
				return err
			}
			return nil
		},
	}
	loader.RegisterPlags(cmd.Flags(), "", cfg, true)
	cmd.Flags().Bool("export", true, "Export incomplete files to json file")
	cmd.Flags().Bool("clean", false, "Clean missing and orphan file parts")
	cmd.Flags().String("user", "", "Telegram User Name")
	cmd.Flags().Int("concurrent", 4, "Number of concurrent channel processing")
	return cmd
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

func (cp *channelProcessor) updateStatus(status string, value int64) {
	cp.tracker.SetValue(value)
	cp.tracker.UpdateMessage(fmt.Sprintf("Channel %d: %s", cp.id, status))
}

func (cp *channelProcessor) process() error {
	cp.updateStatus("Loading files", 0)
	files, err := cp.loadFiles()
	if err != nil {
		return fmt.Errorf("failed to load files: %w", err)
	}
	cp.files = files

	if len(cp.files) == 0 {
		cp.updateStatus("No files found", 100)
		return nil
	}

	cp.updateStatus("Loading messages from Telegram", 0)
	msgs, total, err := cp.loadChannelMessages()
	if err != nil {
		return fmt.Errorf("failed to load messages: %w", err)
	}

	if total == 0 && len(msgs) == 0 {
		cp.updateStatus("No messages found", 100)
		return nil
	}
	if len(msgs) < total {
		return fmt.Errorf("found %d messages out of %d", len(msgs), total)
	}

	cp.updateStatus("Processing messages and parts", 0)
	msgIds := utils.Map(msgs, func(m messages.Elem) int { return m.Msg.GetID() })
	uploadPartIds := []int{}
	if err := cp.db.Model(&models.Upload{}).
		Where("user_id = ?", cp.userId).
		Where("channel_id = ?", cp.id).
		Pluck("part_id", &uploadPartIds).Error; err != nil {
		return err
	}

	uploadPartMap := make(map[int]bool)
	for _, partID := range uploadPartIds {
		uploadPartMap[partID] = true
	}

	msgMap := make(map[int]bool)
	for _, m := range msgIds {
		if m > 0 && !uploadPartMap[m] {
			msgMap[m] = true
		}
	}

	cp.updateStatus("Checking file integrity", 0)
	allPartIDs := make(map[int]bool)
	for _, f := range cp.files {
		for _, p := range f.Parts {
			if p.ID == 0 {
				cp.missingFiles = append(cp.missingFiles, f)
				break
			}
			allPartIDs[p.ID] = true
		}
	}

	if len(allPartIDs) == 0 {
		cp.updateStatus("No parts found", 100)
		return nil
	}

	for _, f := range cp.files {
		for _, p := range f.Parts {
			if !msgMap[p.ID] {
				cp.missingFiles = append(cp.missingFiles, f)
				break
			}
		}
	}

	for msgID := range msgMap {
		if !allPartIDs[msgID] {
			cp.orphanMessages = append(cp.orphanMessages, msgID)
		}
	}

	if len(cp.missingFiles) > 0 {
		cp.channelExport = &channelExport{
			ChannelID: cp.id,
			Timestamp: time.Now().Format(time.RFC3339),
			FileCount: len(cp.missingFiles),
			Files:     make([]exportFile, 0, len(cp.missingFiles)),
		}

		for _, f := range cp.missingFiles {
			cp.channelExport.Files = append(cp.channelExport.Files, exportFile{
				ID:   f.ID,
				Name: f.Name,
			})
		}

		if cp.clean {
			cp.updateStatus("Cleaning files", 0)
			err = cp.db.Exec("call teldrive.delete_files_bulk($1 , $2)",
				utils.Map(cp.missingFiles, func(f file) string { return f.ID }), cp.userId).Error
			if err != nil {
				return err
			}
		}
	}

	if cp.clean && len(cp.orphanMessages) > 0 {
		cp.updateStatus("Cleaning orphan messages", 0)
		tgc.DeleteMessages(cp.ctx, cp.client, cp.id, cp.orphanMessages)
	}

	cp.updateStatus("Complete", 100)
	return nil
}

func (cp *channelProcessor) loadFiles() ([]file, error) {
	var files []file
	const batchSize = 1000
	var totalFiles int64
	var lastID string

	if err := cp.db.Model(&models.File{}).
		Where("user_id = ?", cp.userId).
		Where("channel_id = ?", cp.id).
		Where("type = ?", "file").
		Count(&totalFiles).Error; err != nil {
		return nil, err
	}

	if totalFiles == 0 {
		return nil, nil
	}

	processed := int64(0)
	for {
		var batch []file
		query := cp.db.WithContext(cp.ctx).Model(&models.File{}).
			Where("user_id = ?", cp.userId).
			Where("channel_id = ?", cp.id).
			Where("type = ?", "file").
			Order("id").
			Limit(batchSize)

		if lastID != "" {
			query = query.Where("id > ?", lastID)
		}

		result := query.Scan(&batch)
		if result.Error != nil {
			return nil, result.Error
		}

		if len(batch) == 0 {
			break
		}

		files = append(files, batch...)
		processed += int64(len(batch))

		lastID = batch[len(batch)-1].ID
		progress := (float64(processed) / float64(totalFiles)) * 100
		cp.updateStatus(fmt.Sprintf("Loading files: %d/%d", processed, totalFiles), int64(progress))
		if len(batch) < batchSize {
			break
		}
	}

	return files, nil
}

func (cp *channelProcessor) loadChannelMessages() (msgs []messages.Elem, total int, err error) {

	err = tgc.RunWithAuth(cp.ctx, cp.client, "", func(ctx context.Context) error {
		var channel *tg.InputChannel
		channel, err = tgc.GetChannelById(ctx, cp.client.API(), cp.id)
		if err != nil {
			return err
		}

		q := query.NewQuery(cp.client.API()).Messages().GetHistory(&tg.InputPeerChannel{
			ChannelID:  cp.id,
			AccessHash: channel.AccessHash,
		})

		msgiter := messages.NewIterator(q, 100)
		total, err = msgiter.Total(ctx)
		if err != nil {
			return fmt.Errorf("failed to get total messages: %w", err)
		}

		processed := 0
		for msgiter.Next(ctx) {
			msg := msgiter.Value()
			msgs = append(msgs, msg)
			processed++

			if processed%100 == 0 {
				progress := (float64(processed) / float64(total)) * 100
				cp.updateStatus(fmt.Sprintf("Loading messages: %d/%d", processed, total), int64(progress))
			}
		}
		return nil
	})
	return
}

func runCheckCmd(cmd *cobra.Command, cfg *config.ServerCmdConfig) {

	ctx := cmd.Context()

	lg := logging.DefaultLogger().Sugar()

	defer logging.DefaultLogger().Sync()

	cfg.DB.LogLevel = "fatal"
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
	if err := db.Model(&models.Session{}).
		Where("user_id = ?", user.UserId).
		Order("created_at desc").
		First(&session).Error; err != nil {
		lg.Fatalw("failed to get session", "err", err)
	}

	channelIds := []int64{}
	if err := db.Model(&models.Channel{}).
		Where("user_id = ?", user.UserId).
		Pluck("channel_id", &channelIds).Error; err != nil {
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
	concurrent, _ := cmd.Flags().GetInt("concurrent")

	pw := progress.NewWriter()
	pw.SetAutoStop(false)
	width := 75
	if size, err := termWidth(); err == nil {
		width = int((float32(3) / float32(4)) * float32(size))
	}
	pw.SetTrackerLength(width / 5)
	pw.SetMessageLength(width * 3 / 5)
	pw.SetStyle(progress.StyleDefault)
	pw.SetTrackerPosition(progress.PositionRight)
	pw.SetUpdateFrequency(time.Millisecond * 100)
	pw.Style().Colors = progress.StyleColorsExample
	pw.Style().Colors.Message = text.Colors{text.FgBlue}
	pw.Style().Options.PercentFormat = "%4.1f%%"
	pw.Style().Visibility.Value = false
	pw.Style().Options.TimeInProgressPrecision = time.Millisecond
	pw.Style().Options.ErrorString = color.RedString("failed!")
	pw.Style().Options.DoneString = color.GreenString("done!")

	var channelExports []channelExport
	var mutex sync.Mutex

	g, ctx := errgroup.WithContext(ctx)

	g.SetLimit(concurrent)

	go pw.Render()

	for _, id := range channelIds {

		g.Go(func() error {

			client, err := tgc.AuthClient(ctx, tgconfig, session.Session, middlewares...)
			if err != nil {
				lg.Errorw("failed to create client", "err", err, "channel", id)
				return fmt.Errorf("failed to create client for channel %d: %w", id, err)
			}

			tracker := &progress.Tracker{
				Message: fmt.Sprintf("Channel %d: Initializing", id),
				Total:   100,
				Units:   progress.UnitsDefault,
			}
			pw.AppendTracker(tracker)

			processor := &channelProcessor{
				id:         id,
				client:     client,
				ctx:        ctx,
				db:         db,
				userId:     user.UserId,
				clean:      clean,
				pw:         pw,
				tracker:    tracker,
				totalCount: 100,
			}

			if err := processor.process(); err != nil {
				tracker.MarkAsErrored()
				return err
			}

			if processor.channelExport != nil {
				mutex.Lock()
				channelExports = append(channelExports, *processor.channelExport)
				mutex.Unlock()
			}

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lg.Fatal(fmt.Errorf("one or more channels failed to process"))
	}

	pw.Stop()

	if export && len(channelExports) > 0 {
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

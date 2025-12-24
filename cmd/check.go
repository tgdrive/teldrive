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
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/logging"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"go.uber.org/zap"
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
	ID        string
	Name      string
	Size      int64
	Encrypted bool
	Parts     datatypes.JSONSlice[api.Part]
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
	dryRun         bool
	verbose        bool
	logger         *zap.SugaredLogger
	cfg            *config.TGConfig
	session        string
}

func NewCheckCmd() *cobra.Command {
	var cfg config.ServerCmdConfig
	loader := config.NewConfigLoader()
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check and optionally purge incomplete files in Telegram channels",
		Long: `Check the integrity of files stored in Telegram channels by comparing database records
with actual Telegram messages. Identifies missing file parts and orphan messages.

Examples:
  # Preview issues without making changes
  teldrive check --user alice --dry-run

  # Check and export missing files to a custom file
  teldrive check  --export-file missing_files.json

  # Clean missing files and orphan messages after confirmation
  teldrive check --clean --verbose

  # Quiet mode with concurrent processing
  teldrive check --quiet --concurrent 8`,
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
	loader.RegisterFlags(cmd.Flags(), "", cfg, true)
	cmd.Flags().String("export-file", "results.json", "Path for exported JSON file")
	cmd.Flags().Bool("clean", false, "Clean missing files and orphan messages")
	cmd.Flags().Bool("dry-run", false, "Simulate check/clean process without making changes")
	cmd.Flags().Bool("verbose", false, "Enable detailed logging for debugging")
	cmd.Flags().String("user", "", "Telegram username to check (prompts if not specified)")
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
	if cp.tracker != nil {
		cp.tracker.SetValue(value)
		cp.tracker.UpdateMessage(fmt.Sprintf("Channel %d: %s", cp.id, status))
	}
}

func (cp *channelProcessor) process() error {
	if cp.verbose {
		cp.logger.Infof("Starting check for channel %d", cp.id)
	}
	cp.updateStatus("Loading files", 0)
	files, err := cp.loadFiles()
	if err != nil {
		return fmt.Errorf("failed to load files for channel %d: %w", cp.id, err)
	}
	cp.files = files

	if len(cp.files) == 0 {
		if cp.verbose {
			cp.logger.Infof("Channel %d: No files found", cp.id)
		}
		cp.updateStatus("No files found", 100)
		return nil
	}

	cp.updateStatus("Loading messages from Telegram", 0)
	msgs, total, err := cp.loadChannelMessages()
	if err != nil {
		return fmt.Errorf("failed to load messages for channel %d: %w", cp.id, err)
	}

	if total == 0 && len(msgs) == 0 {
		if cp.verbose {
			cp.logger.Infof("Channel %d: No messages found", cp.id)
		}
		cp.updateStatus("No messages found", 100)
		return nil
	}
	if len(msgs) < total {
		return fmt.Errorf("channel %d: found %d messages out of %d", cp.id, len(msgs), total)
	}

	cp.updateStatus("Processing messages and parts", 0)
	uploadPartIds := []int{}
	if err := cp.db.Model(&models.Upload{}).
		Where("user_id = ?", cp.userId).
		Where("channel_id = ?", cp.id).
		Pluck("part_id", &uploadPartIds).Error; err != nil {
		return fmt.Errorf("failed to query uploads for channel %d: %w", cp.id, err)
	}

	uploadPartMap := make(map[int]bool)
	for _, partID := range uploadPartIds {
		uploadPartMap[partID] = true
	}

	msgMap := make(map[int]int64)
	for _, m := range msgs {
		id := m.Msg.GetID()
		_, ok := uploadPartMap[id]
		if id > 0 && !ok {
			doc, ok := m.Document()
			if !ok {
				msgMap[id] = 0
			} else {
				msgMap[id] = doc.GetSize()
			}
		}
	}

	cp.updateStatus("Checking file integrity", 0)

	allPartIDs := make(map[int]bool)

	for _, f := range cp.files {
		size := int64(0)
		for _, p := range f.Parts {
			if p.ID != 0 {
				allPartIDs[p.ID] = true
			}
			_, ok := msgMap[p.ID]
			if !ok {
				cp.missingFiles = append(cp.missingFiles, f)
				if cp.verbose {
					cp.logger.Warnf("Channel %d: File %s (%s) is missing part %d", cp.id, f.Name, f.ID, p.ID)
				}
				break
			}
			if f.Encrypted {
				d, _ := crypt.DecryptedSize(msgMap[p.ID])
				size += d
			} else {
				size += msgMap[p.ID]
			}
		}
		if size != f.Size {
			cp.missingFiles = append(cp.missingFiles, f)
			if cp.verbose {
				cp.logger.Warnf("Channel %d: File %s (%s) size mismatch: expected %d, got %d", cp.id, f.Name, f.ID, f.Size, size)
			}
		}
	}

	if len(allPartIDs) == 0 {
		if cp.verbose {
			cp.logger.Infof("Channel %d: No parts found", cp.id)
		}
		cp.updateStatus("No parts found", 100)
		return nil
	}

	for msgID := range msgMap {
		_, ok := allPartIDs[msgID]
		if !ok {
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
			if cp.dryRun {
				cp.logger.Infof("Channel %d: Would clean %d missing files (dry run)", cp.id, len(cp.missingFiles))

			} else {
				err = cp.db.Exec("call teldrive.delete_files_bulk($1 , $2)",
					utils.Map(cp.missingFiles, func(f file) string { return f.ID }), cp.userId).Error
				if err != nil {
					return fmt.Errorf("failed to clean files for channel %d: %w", cp.id, err)
				}
				if cp.verbose {
					cp.logger.Infof("Channel %d: Cleaned %d missing files", cp.id, len(cp.missingFiles))
				}
			}
		}
	}

	if cp.clean && len(cp.orphanMessages) > 0 {
		cp.updateStatus("Cleaning orphan messages", 0)
		if cp.dryRun {
			cp.logger.Infof("Channel %d: Would clean %d orphan messages (dry run)", cp.id, len(cp.orphanMessages))

		} else {
			err = cp.initClient()
			if err != nil {
				return err
			}
			err = tgc.DeleteMessages(cp.ctx, cp.client, cp.id, cp.orphanMessages)
			if err != nil {
				return err
			}
			if cp.verbose {
				cp.logger.Infof("Channel %d: Cleaned %d orphan messages", cp.id, len(cp.orphanMessages))
			}
		}
	}
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

func (cp *channelProcessor) initClient() error {
	middlewares := tgc.NewMiddleware(cp.cfg, tgc.WithFloodWait(), tgc.WithRateLimit())
	client, err := tgc.AuthClient(cp.ctx, cp.cfg, cp.session, middlewares...)
	if err != nil {
		return fmt.Errorf("failed to create client %w", err)
	}
	cp.client = client
	return nil
}

func (cp *channelProcessor) loadChannelMessages() (msgs []messages.Elem, total int, err error) {
	err = cp.initClient()
	if err != nil {
		return
	}
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
			return err
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
	db, err := database.NewDatabase(ctx, &cfg.DB, lg)
	if err != nil {
		lg.Fatalw("failed to connect to database", "err", err)
	}

	users := []models.User{}
	if err := db.Model(&models.User{}).Find(&users).Error; err != nil {
		lg.Fatalw("failed to retrieve users from database", "err", err)
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
		lg.Fatalw("failed to get user session - ensure user has logged in", "err", err)
	}

	channelIds := []int64{}
	if err := db.Model(&models.Channel{}).
		Where("user_id = ?", user.UserId).
		Pluck("channel_id", &channelIds).Error; err != nil {
		lg.Fatalw("failed to get user channels", "err", err)
	}

	if len(channelIds) == 0 {
		lg.Fatalw("no channels found for user - ensure channels are configured")
	}

	exportFile, _ := cmd.Flags().GetString("export-file")
	clean, _ := cmd.Flags().GetBool("clean")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	verbose, _ := cmd.Flags().GetBool("verbose")
	concurrent, _ := cmd.Flags().GetInt("concurrent")

	if dryRun {
		lg.Info("Running in dry-run mode - no changes will be made")
	}

	var pw progress.Writer

	pw = progress.NewWriter()
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
	go pw.Render()

	var channelExports []channelExport
	var mutex sync.Mutex
	var totalFiles, totalMissing, totalOrphans, totalCleanedFiles, totalCleanedOrphans int

	g, ctx := errgroup.WithContext(ctx)

	g.SetLimit(concurrent)

	for _, id := range channelIds {

		g.Go(func() error {

			var tracker *progress.Tracker
			if pw != nil {
				tracker = &progress.Tracker{
					Message: fmt.Sprintf("Channel %d: Initializing", id),
					Total:   100,
					Units:   progress.UnitsDefault,
				}
				pw.AppendTracker(tracker)
			}

			processor := &channelProcessor{
				id:         id,
				ctx:        ctx,
				cfg:        &cfg.TG,
				session:    session.Session,
				db:         db,
				userId:     user.UserId,
				clean:      clean,
				dryRun:     dryRun,
				verbose:    verbose,
				logger:     lg,
				pw:         pw,
				tracker:    tracker,
				totalCount: 100,
			}

			if err := processor.process(); err != nil {
				if tracker != nil {
					tracker.MarkAsErrored()
				}
				return err
			}

			if processor.channelExport != nil {
				mutex.Lock()
				channelExports = append(channelExports, *processor.channelExport)
				totalMissing += len(processor.missingFiles)
				totalOrphans += len(processor.orphanMessages)
				mutex.Unlock()
			}
			mutex.Lock()
			totalFiles += len(processor.files)
			if clean && !dryRun {
				totalCleanedFiles += len(processor.missingFiles)
				totalCleanedOrphans += len(processor.orphanMessages)
			}
			mutex.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		lg.Fatal("one or more channels failed to process - check logs for details")
	}

	pw.Stop()

	if !dryRun && len(channelExports) > 0 {
		jsonData, err := json.MarshalIndent(channelExports, "", "    ")
		if err != nil {
			lg.Errorw("failed to generate JSON export", "err", err)
			return
		}

		err = os.WriteFile(exportFile, jsonData, 0644)
		if err != nil {
			lg.Errorw("failed to write export file", "err", err)
			return
		}

		lg.Infof("Exported %d incomplete files to %s", totalMissing, exportFile)

	}

	fmt.Println("\n=== Check Summary ===")
	fmt.Printf("Channels processed: %d\n", len(channelIds))
	fmt.Printf("Total files checked: %d\n", totalFiles)
	fmt.Printf("Missing files: %d\n", totalMissing)
	fmt.Printf("Orphan messages: %d\n", totalOrphans)
	if clean {
		if dryRun {
			fmt.Printf("Would clean: %d files, %d messages\n", totalMissing, totalOrphans)
		} else {
			fmt.Printf("Cleaned: %d files, %d messages\n", totalCleanedFiles, totalCleanedOrphans)
		}
	}

}

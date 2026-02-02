package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type file struct {
	ID        string
	Name      string
	Size      int64
	Encrypted bool
	Status    string
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
	id              int64
	cmd             *cobra.Command
	files           []file
	missingFiles    []file
	orphanMessages  []int
	totalCount      int64
	totalPartsDB    int
	totalMessagesTG int
	channelExport   *channelExport
	ctx             context.Context
	db              *gorm.DB
	userId          int64
	dryRun          bool
	cfg             *config.CheckCmdConfig
	session         string
	logger          *channelLogger
}

type channelLogger struct {
	channelID int64
	mutex     sync.Mutex
}

func newChannelLogger(channelID int64) *channelLogger {
	return &channelLogger{channelID: channelID}
}

func (cl *channelLogger) log(status string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	c := color.New(color.FgCyan)
	c.Printf("[Channel %d] %s\n", cl.channelID, status)
}

func (cl *channelLogger) success(status string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	c := color.New(color.FgGreen)
	c.Printf("[Channel %d] ✓ %s\n", cl.channelID, status)
}

func (cl *channelLogger) error(errMsg string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	c := color.New(color.FgRed)
	c.Printf("[Channel %d] ✗ %s\n", cl.channelID, errMsg)
}

func (cl *channelLogger) progress(current, total int64, operation string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	if total > 0 {
		percent := float64(current) / float64(total) * 100
		c := color.New(color.FgYellow)
		c.Printf("[Channel %d] %s: %d/%d (%.1f%%)\n", cl.channelID, operation, current, total, percent)
	}
}

func NewCheckCmd() *cobra.Command {
	var cfg config.CheckCmdConfig
	loader := config.NewConfigLoader()
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check and purge incomplete files in Telegram channels",
		Long: `Check the integrity of files stored in Telegram channels by comparing database records
with actual Telegram messages. Identifies and removes missing file parts and orphan messages.

Examples:
  # Preview issues without making changes (dry-run)
  teldrive check --user alice --dry-run

  # Check, clean and export missing files to a custom file
  teldrive check --export-file missing_files.json

  # Clean missing and pending files along with incompleted uploads
  teldrive check --clean-pending --clean-uploads

  # Concurrent processing
  teldrive check --concurrent 8`,
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
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[config.CheckCmdConfig]())
	return cmd
}

func checkRequiredCheckFlags(cfg *config.CheckCmdConfig) error {
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

	var options []string
	for _, u := range users {
		options = append(options, u.UserName)
	}

	fmt.Println("Available users:")
	for i, u := range options {
		fmt.Printf("  %d. %s\n", i+1, u)
	}
	fmt.Print("Select user (number): ")

	var selection int
	_, err := fmt.Scanln(&selection)
	if err != nil || selection < 1 || selection > len(options) {
		return nil, fmt.Errorf("invalid selection")
	}

	return &users[selection-1], nil
}

func (cp *channelProcessor) process() error {
	cleanPending := cp.cfg.CleanPending
	cleanUploads := cp.cfg.CleanUploads

	if cleanUploads && !cp.dryRun {
		cp.logger.log("Deleting incomplete uploads...")
		if err := cp.db.Where("user_id = ?", cp.userId).
			Where("channel_id = ?", cp.id).
			Delete(&models.Upload{}).Error; err != nil {
			return fmt.Errorf("failed to delete uploads: %w", err)
		}
	}

	cp.logger.log("Loading files from database...")
	files, err := cp.loadFiles()
	if err != nil {
		return fmt.Errorf("failed to load files for channel %d: %w", cp.id, err)
	}
	cp.files = files
	cp.logger.success(fmt.Sprintf("Loaded %d files", len(files)))

	cp.logger.log("Loading messages from Telegram...")
	msgs, total, err := cp.loadChannelMessages()
	if err != nil {
		return fmt.Errorf("failed to load messages for channel %d: %w", cp.id, err)
	}

	if total == 0 && len(msgs) == 0 {
		cp.logger.log("No messages found in channel")
		return nil
	}
	if len(msgs) < total {
		return fmt.Errorf("channel %d: found %d messages out of %d", cp.id, len(msgs), total)
	}

	cp.logger.success(fmt.Sprintf("Loaded %d messages from Telegram", len(msgs)))

	cp.logger.log("Processing messages and parts...")

	uploadPartMap := make(map[int]bool)
	if !cp.dryRun && !cleanUploads {
		uploadPartIds := []int{}
		if err := cp.db.Model(&models.Upload{}).
			Where("user_id = ?", cp.userId).
			Where("channel_id = ?", cp.id).
			Pluck("part_id", &uploadPartIds).Error; err != nil {
			return fmt.Errorf("failed to query uploads for channel %d: %w", cp.id, err)
		}
		for _, id := range uploadPartIds {
			uploadPartMap[id] = true
		}
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

	cp.logger.log("Checking file integrity...")

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
		}
	}

	if len(allPartIDs) == 0 && len(cp.files) > 0 {
		cp.logger.log("No parts found in database")
	}

	for msgID := range msgMap {
		if msgID == 1 {
			continue
		}
		_, ok := allPartIDs[msgID]
		if !ok {
			cp.orphanMessages = append(cp.orphanMessages, msgID)
		}
	}
	cp.totalPartsDB += len(allPartIDs)

	msgCount := len(msgMap)
	if _, hasMsg1 := msgMap[1]; hasMsg1 {
		msgCount--
	}
	cp.totalMessagesTG += msgCount

	if cleanPending && !cp.dryRun {
		cp.logger.log("Deleting pending files from database...")
		if err := cp.db.Where("user_id = ?", cp.userId).
			Where("channel_id = ?", cp.id).
			Where("status = ?", "pending_deletion").
			Delete(&models.File{}).Error; err != nil {
			return fmt.Errorf("failed to delete pending files: %w", err)
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

		if !cp.dryRun {
			cp.logger.log(fmt.Sprintf("Cleaning %d missing files...", len(cp.missingFiles)))
			err = cp.deleteFilesBulk(utils.Map(cp.missingFiles, func(f file) string { return f.ID }), cp.userId)
			if err != nil {
				return fmt.Errorf("failed to clean files for channel %d: %w", cp.id, err)
			}
		}
	}

	if len(cp.orphanMessages) > 0 {
		if !cp.dryRun {
			cp.logger.log(fmt.Sprintf("Cleaning %d orphan messages...", len(cp.orphanMessages)))
			err = cp.deleteOrphanMessages()
			if err != nil {
				return err
			}
		}
	}

	if len(cp.missingFiles) > 0 || len(cp.orphanMessages) > 0 {
		cp.logger.success(fmt.Sprintf("Found %d missing files, %d orphans", len(cp.missingFiles), len(cp.orphanMessages)))
	} else {
		cp.logger.success("All files verified successfully")
	}

	return nil
}

func (cp *channelProcessor) deleteFilesBulk(fileIds []string, userId int64) error {
	query := `
	WITH RECURSIVE target_folders AS (
		SELECT id FROM teldrive.files WHERE id IN (?) AND user_id = ?
		UNION ALL
		SELECT f.id FROM teldrive.files f JOIN target_folders tf ON f.parent_id = tf.id
	),
	mark_deleted AS (
		UPDATE teldrive.files SET status = 'pending_deletion'
		WHERE (parent_id IN (SELECT id FROM target_folders) OR id IN (?))
		AND type = 'file'
	)
	DELETE FROM teldrive.files WHERE id IN (SELECT id FROM target_folders) AND type = 'folder';
	`
	return cp.db.Exec(query, fileIds, userId, fileIds).Error
}

func (cp *channelProcessor) deleteOrphanMessages() error {
	middlewares := tgc.NewMiddleware(&cp.cfg.TG, tgc.WithFloodWait(), tgc.WithRateLimit())
	client, err := tgc.AuthClient(cp.ctx, &cp.cfg.TG, cp.session, middlewares...)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	return tgc.RunWithAuth(cp.ctx, client, cp.session, func(ctx context.Context) error {
		return tgc.DeleteMessages(ctx, client, cp.id, cp.orphanMessages)
	})
}

func (cp *channelProcessor) loadFiles() ([]file, error) {
	var files []file
	const batchSize = 1000
	var totalFiles int64
	var lastID string

	cleanPending := cp.cfg.CleanPending

	db := cp.db.Model(&models.File{}).
		Where("user_id = ?", cp.userId).
		Where("channel_id = ?", cp.id).
		Where("type = ?", "file")

	if cleanPending && cp.dryRun {
		db = db.Where("status IN ?", []string{"active", "pending_deletion"})
	} else {
		db = db.Where("status = ?", "active")
	}

	if err := db.Count(&totalFiles).Error; err != nil {
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

		if cleanPending && cp.dryRun {
			query = query.Where("status IN ?", []string{"active", "pending_deletion"})
		} else {
			query = query.Where("status = ?", "active")
		}

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

		if processed%1000 == 0 || len(batch) < batchSize {
			percent := float64(processed) / float64(totalFiles) * 100
			fmt.Printf("[Channel %d] Loading files: %d/%d (%.1f%%)\r", cp.id, processed, totalFiles, percent)
		}

		if len(batch) < batchSize {
			break
		}
	}

	return files, nil
}

func (cp *channelProcessor) loadChannelMessages() (msgs []messages.Elem, total int, err error) {
	middlewares := tgc.NewMiddleware(&cp.cfg.TG, tgc.WithFloodWait(), tgc.WithRateLimit())
	client, err := tgc.AuthClient(cp.ctx, &cp.cfg.TG, cp.session, middlewares...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create client: %w", err)
	}

	var channel *tg.InputChannel
	var loadedMsgs []messages.Elem

	err = tgc.RunWithAuth(cp.ctx, client, cp.session, func(ctx context.Context) error {
		channel, err = tgc.GetChannelById(ctx, client.API(), cp.id)
		if err != nil {
			return err
		}

		countQ := query.NewQuery(client.API()).Messages().GetHistory(&tg.InputPeerChannel{
			ChannelID:  cp.id,
			AccessHash: channel.AccessHash,
		})
		countIter := messages.NewIterator(countQ, 1)
		total, err = countIter.Total(ctx)
		if err != nil {
			return err
		}

		if total == 0 {
			return nil
		}

		fmt.Printf("[Channel %d] Loading %d messages...\n", cp.id, total)

		loadQ := query.NewQuery(client.API()).Messages().GetHistory(&tg.InputPeerChannel{
			ChannelID:  cp.id,
			AccessHash: channel.AccessHash,
		})
		loadIter := messages.NewIterator(loadQ, 100)
		processed := 0
		for loadIter.Next(ctx) {
			loadedMsgs = append(loadedMsgs, loadIter.Value())
			processed++

			if processed%100 == 0 || processed == total {
				percent := float64(processed) / float64(total) * 100
				fmt.Printf("[Channel %d] Loading: %d/%d (%.1f%%)\r", cp.id, processed, total, percent)
			}
		}
		fmt.Printf("\n")

		return nil
	})

	return loadedMsgs, total, err
}

func runCheckCmd(cmd *cobra.Command, cfg *config.CheckCmdConfig) {
	ctx := cmd.Context()

	logCfg := &config.DBLoggingConfig{
		Level: "fatal",
	}
	db, err := database.NewDatabase(ctx, &cfg.DB, logCfg, zap.NewNop())
	if err != nil {
		color.Red("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}

	users := []models.User{}
	if err := db.Model(&models.User{}).Find(&users).Error; err != nil {
		color.Red("Failed to retrieve users from database: %v\n", err)
		os.Exit(1)
	}

	userName := cfg.User
	user, err := selectUser(userName, users)
	if err != nil {
		color.Red("Failed to select user: %v\n", err)
		os.Exit(1)
	}

	session := models.Session{}
	if err := db.Model(&models.Session{}).
		Where("user_id = ?", user.UserId).
		Order("created_at desc").
		First(&session).Error; err != nil {
		color.Red("Failed to get user session - ensure user has logged in: %v\n", err)
		os.Exit(1)
	}

	channelIds := []int64{}
	if err := db.Model(&models.Channel{}).
		Where("user_id = ?", user.UserId).
		Pluck("channel_id", &channelIds).Error; err != nil {
		color.Red("Failed to get user channels: %v\n", err)
		os.Exit(1)
	}

	if len(channelIds) == 0 {
		color.Red("No channels found for user - ensure channels are configured\n")
		os.Exit(1)
	}

	exportFile := cfg.ExportFile
	dryRun := cfg.DryRun
	concurrent := cfg.Concurrent

	if dryRun {
		color.Yellow("Running in dry-run mode - no changes will be made\n")
	}

	color.Cyan("Processing %d channels with concurrency %d...\n\n", len(channelIds), concurrent)

	var channelExports []channelExport
	var mutex sync.Mutex
	var totalFiles, totalMissing, totalOrphans, totalCleanedFiles, totalCleanedOrphans int
	var totalPartsDB, totalMessagesTG int

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrent)

	for _, id := range channelIds {
		g.Go(func() error {
			logger := newChannelLogger(id)
			logger.log("Starting processing...")

			processor := &channelProcessor{
				id:      id,
				cmd:     cmd,
				ctx:     ctx,
				cfg:     cfg,
				session: session.Session,
				db:      db,
				userId:  user.UserId,
				dryRun:  dryRun,
				logger:  logger,
			}

			if err := processor.process(); err != nil {
				logger.error(err.Error())
				return err
			}

			mutex.Lock()
			if processor.channelExport != nil {
				channelExports = append(channelExports, *processor.channelExport)
			}
			totalMissing += len(processor.missingFiles)
			totalOrphans += len(processor.orphanMessages)
			totalFiles += len(processor.files)
			totalPartsDB += processor.totalPartsDB
			totalMessagesTG += processor.totalMessagesTG
			if !dryRun {
				totalCleanedFiles += len(processor.missingFiles)
				totalCleanedOrphans += len(processor.orphanMessages)
			}
			mutex.Unlock()

			return nil
		})
	}

	if err := g.Wait(); err != nil {
		color.Red("\nOne or more channels failed to process: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	color.Green("✓ All channels processed successfully\n")

	if !dryRun && len(channelExports) > 0 {
		jsonData, err := json.MarshalIndent(channelExports, "", "    ")
		if err != nil {
			color.Red("Failed to generate JSON export: %v\n", err)
			return
		}

		err = os.WriteFile(exportFile, jsonData, 0644)
		if err != nil {
			color.Red("Failed to write export file: %v\n", err)
			return
		}

		color.Cyan("Exported %d incomplete files to %s\n", totalMissing, exportFile)
	}

	fmt.Println()
	color.Cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	color.Cyan("                    Check Summary                   \n")
	color.Cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  %-25s %d\n", "Channels Processed:", len(channelIds))
	fmt.Printf("  %-25s %d\n", "Total Files Checked:", totalFiles)
	fmt.Printf("  %-25s %d\n", "Total Parts (DB):", totalPartsDB)
	fmt.Printf("  %-25s %d\n", "Total Messages (TG):", totalMessagesTG)
	fmt.Printf("  %-25s %d\n", "Missing Files:", totalMissing)
	fmt.Printf("  %-25s %d\n", "Orphan Messages:", totalOrphans)

	if dryRun {
		fmt.Printf("  %-25s %d\n", "Would Clean Files:", totalMissing)
		fmt.Printf("  %-25s %d\n", "Would Clean Orphans:", totalOrphans)
	} else {
		fmt.Printf("  %-25s %d\n", "Cleaned Files:", totalCleanedFiles)
		fmt.Printf("  %-25s %d\n", "Cleaned Orphans:", totalCleanedOrphans)
	}
	color.Cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

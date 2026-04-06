package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/gotd/td/telegram/query"
	"github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
	"github.com/spf13/cobra"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/crypt"
	"github.com/tgdrive/teldrive/internal/database"
	jetmodel "github.com/tgdrive/teldrive/internal/database/jet/gen/model"
	dbtypes "github.com/tgdrive/teldrive/internal/database/types"
	"github.com/tgdrive/teldrive/internal/tgc"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/repositories"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

type checkFile struct {
	ID        uuid.UUID
	Name      string
	Size      int64
	Encrypted bool
	Status    string
	Parts     []api.Part
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
	files           []checkFile
	missingFiles    []checkFile
	orphanMessages  []int
	totalCount      int64
	totalPartsDB    int
	totalMessagesTG int
	channelExport   *channelExport
	ctx             context.Context
	repos           *repositories.Repositories
	userID          int64
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
	color.New(color.FgCyan).Printf("[Channel %d] %s\n", cl.channelID, status)
}

func (cl *channelLogger) success(status string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	color.New(color.FgGreen).Printf("[Channel %d] ✓ %s\n", cl.channelID, status)
}

func (cl *channelLogger) error(errMsg string) {
	cl.mutex.Lock()
	defer cl.mutex.Unlock()
	color.New(color.FgRed).Printf("[Channel %d] ✗ %s\n", cl.channelID, errMsg)
}

func NewCheckCmd() *cobra.Command {
	var cfg config.CheckCmdConfig
	loader := config.NewConfigLoader()
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check and purge incomplete files in Telegram channels",
		Long: `Check file integrity in Telegram channels by comparing database records
with the actual Telegram messages. Missing files can be exported and optional cleanup
removes missing files and orphan channel messages.

Examples:
  teldrive check --user alice --dry-run
  teldrive check --export-file missing_files.json
  teldrive check --concurrent 8`,
		Run: func(cmd *cobra.Command, args []string) {
			runCheckCmd(cmd, &cfg)
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if err := loader.Load(cmd, &cfg); err != nil {
				return err
			}
			return checkRequiredCheckFlags(&cfg)
		},
	}
	loader.RegisterFlags(cmd.Flags(), reflect.TypeFor[config.CheckCmdConfig]())
	return cmd
}

func checkRequiredCheckFlags(cfg *config.CheckCmdConfig) error {
	if cfg.DB.DataSource == "" {
		return fmt.Errorf("required configuration values not set: db-data-source")
	}
	return nil
}

func selectUser(user string, users []jetmodel.Users) (*jetmodel.Users, error) {
	if user != "" {
		res := utils.Filter(users, func(u jetmodel.Users) bool {
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

	uploadPartMap := make(map[int]bool)
	if err := cp.loadUploadPartMap(uploadPartMap); err != nil {
		return fmt.Errorf("failed to query uploads for channel %d: %w", cp.id, err)
	}

	msgMap := make(map[int]int64)
	for _, m := range msgs {
		id := m.Msg.GetID()
		if id <= 0 || uploadPartMap[id] {
			continue
		}
		doc, ok := m.Document()
		if !ok {
			msgMap[id] = 0
			continue
		}
		msgMap[id] = doc.GetSize()
	}

	allPartIDs := make(map[int]bool)
	for _, f := range cp.files {
		var size int64
		missing := false
		for _, p := range f.Parts {
			if p.ID != 0 {
				allPartIDs[p.ID] = true
			}
			msgSize, ok := msgMap[p.ID]
			if !ok {
				missing = true
				break
			}
			if f.Encrypted {
				d, _ := crypt.DecryptedSize(msgSize)
				size += d
			} else {
				size += msgSize
			}
		}
		if missing || size != f.Size {
			cp.missingFiles = append(cp.missingFiles, f)
		}
	}

	for msgID := range msgMap {
		if msgID == 1 {
			continue
		}
		if !allPartIDs[msgID] {
			cp.orphanMessages = append(cp.orphanMessages, msgID)
		}
	}
	cp.totalPartsDB += len(allPartIDs)
	msgCount := len(msgMap)
	if _, hasMsg1 := msgMap[1]; hasMsg1 {
		msgCount--
	}
	cp.totalMessagesTG += msgCount

	if len(cp.missingFiles) > 0 {
		cp.channelExport = &channelExport{ChannelID: cp.id, Timestamp: time.Now().Format(time.RFC3339), FileCount: len(cp.missingFiles), Files: make([]exportFile, 0, len(cp.missingFiles))}
		for _, f := range cp.missingFiles {
			cp.channelExport.Files = append(cp.channelExport.Files, exportFile{ID: f.ID.String(), Name: f.Name})
		}
		if !cp.dryRun {
			cp.logger.log(fmt.Sprintf("Cleaning %d missing files...", len(cp.missingFiles)))
			ids := utils.Map(cp.missingFiles, func(f checkFile) uuid.UUID { return f.ID })
			if err := cp.deleteFilesBulk(ids, cp.userID); err != nil {
				return fmt.Errorf("failed to clean files for channel %d: %w", cp.id, err)
			}
		}
	}

	if len(cp.orphanMessages) > 0 && !cp.dryRun {
		cp.logger.log(fmt.Sprintf("Cleaning %d orphan messages...", len(cp.orphanMessages)))
		if err := cp.deleteOrphanMessages(); err != nil {
			return err
		}
	}

	if len(cp.missingFiles) > 0 || len(cp.orphanMessages) > 0 {
		cp.logger.success(fmt.Sprintf("Found %d missing files, %d orphans", len(cp.missingFiles), len(cp.orphanMessages)))
	} else {
		cp.logger.success("All files verified successfully")
	}
	return nil
}

func (cp *channelProcessor) loadUploadPartMap(out map[int]bool) error {
	ids, err := cp.repos.Uploads.ListPartIDsByChannel(cp.ctx, cp.userID, cp.id)
	if err != nil {
		return err
	}
	for _, id := range ids {
		out[id] = true
	}
	return nil
}

func (cp *channelProcessor) deleteFilesBulk(fileIDs []uuid.UUID, userID int64) error {
	query := `
WITH RECURSIVE target_folders AS (
    SELECT id FROM teldrive.files WHERE id = ANY($1::uuid[]) AND user_id = $2 AND type = 'folder'
    UNION ALL
    SELECT f.id FROM teldrive.files f JOIN target_folders tf ON f.parent_id = tf.id
), mark_deleted AS (
    UPDATE teldrive.files
    SET status = 'pending_deletion', updated_at = NOW() AT TIME ZONE 'UTC'
    WHERE user_id = $2
      AND type = 'file'
      AND (parent_id IN (SELECT id FROM target_folders) OR id = ANY($1::uuid[]))
)
DELETE FROM teldrive.files WHERE id IN (SELECT id FROM target_folders) AND type = 'folder';`
	_, err := cp.repos.Pool.Exec(cp.ctx, query, fileIDs, userID)
	return err
}

func (cp *channelProcessor) deleteOrphanMessages() error {
	return cp.deleteSpecificMessages(cp.orphanMessages)
}

func (cp *channelProcessor) deleteSpecificMessages(ids []int) error {
	if len(ids) == 0 {
		return nil
	}
	middlewares := tgc.NewMiddleware(&cp.cfg.TG, tgc.WithFloodWait(), tgc.WithRateLimit())
	client, err := tgc.AuthClient(cp.ctx, &cp.cfg.TG, cp.session, middlewares...)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	return tgc.DeleteMessages(cp.ctx, client, cp.id, ids)
}

func (cp *channelProcessor) loadFiles() ([]checkFile, error) {
	files, err := cp.repos.Files.ListCheckFiles(cp.ctx, cp.userID, cp.id, false)
	if err != nil {
		return nil, err
	}
	return utils.Map(files, func(f repositories.CheckFile) checkFile {
		return checkFile{
			ID:        f.ID,
			Name:      f.Name,
			Size:      f.Size,
			Encrypted: f.Encrypted,
			Status:    f.Status,
			Parts: utils.Map(f.Parts, func(p dbtypes.Part) api.Part {
				return api.Part{ID: p.ID, Salt: api.NewOptString(p.Salt)}
			}),
		}
	}), nil
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
		channel, err = tgc.ChannelByID(ctx, client.API(), cp.id)
		if err != nil {
			return err
		}
		countQ := query.NewQuery(client.API()).Messages().GetHistory(&tg.InputPeerChannel{ChannelID: cp.id, AccessHash: channel.AccessHash})
		countIter := messages.NewIterator(countQ, 1)
		total, err = countIter.Total(ctx)
		if err != nil {
			return err
		}
		if total == 0 {
			return nil
		}
		loadQ := query.NewQuery(client.API()).Messages().GetHistory(&tg.InputPeerChannel{ChannelID: cp.id, AccessHash: channel.AccessHash})
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
		return loadIter.Err()
	})
	return loadedMsgs, total, err
}

func runCheckCmd(cmd *cobra.Command, cfg *config.CheckCmdConfig) {
	ctx := cmd.Context()
	logCfg := &config.DBLoggingConfig{Level: "error", LogSQL: false}
	pool, err := database.NewDatabase(ctx, &cfg.DB, logCfg, zap.NewNop())
	if err != nil {
		color.Red("Failed to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	repos := repositories.NewRepositories(pool)
	users, err := repositories.NewJetUserRepository(pool).All(ctx)
	if err != nil {
		color.Red("Failed to retrieve users from database: %v\n", err)
		os.Exit(1)
	}
	user, err := selectUser(cfg.User, users)
	if err != nil {
		color.Red("Failed to select user: %v\n", err)
		os.Exit(1)
	}

	sessions, err := repos.Sessions.GetByUserID(ctx, user.UserID)
	if err != nil || len(sessions) == 0 {
		color.Red("Failed to get user session - ensure user has logged in: %v\n", err)
		os.Exit(1)
	}
	channels, err := repos.Channels.GetByUserID(ctx, user.UserID)
	if err != nil {
		color.Red("Failed to get user channels: %v\n", err)
		os.Exit(1)
	}
	if len(channels) == 0 {
		color.Red("No channels found for user - ensure channels are configured\n")
		os.Exit(1)
	}
	channelIDs := utils.Map(channels, func(c jetmodel.Channels) int64 { return c.ChannelID })

	if cfg.DryRun {
		color.Yellow("Running in dry-run mode - no changes will be made\n")
	}
	color.Cyan("Processing %d channels with concurrency %d...\n\n", len(channelIDs), cfg.Concurrent)

	var channelExports []channelExport
	var mu sync.Mutex
	var totalFiles, totalMissing, totalOrphans, totalCleanedFiles, totalCleanedOrphans, totalPartsDB, totalMessagesTG int
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(cfg.Concurrent)
	for _, id := range channelIDs {
		id := id
		g.Go(func() error {
			logger := newChannelLogger(id)
			logger.log("Starting processing...")
			processor := &channelProcessor{cmd: cmd, id: id, ctx: gctx, cfg: cfg, session: sessions[0].TgSession, repos: repos, userID: user.UserID, dryRun: cfg.DryRun, logger: logger}
			if err := processor.process(); err != nil {
				logger.error(err.Error())
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			if processor.channelExport != nil {
				channelExports = append(channelExports, *processor.channelExport)
			}
			totalMissing += len(processor.missingFiles)
			totalOrphans += len(processor.orphanMessages)
			totalFiles += len(processor.files)
			totalPartsDB += processor.totalPartsDB
			totalMessagesTG += processor.totalMessagesTG
			if !cfg.DryRun {
				totalCleanedFiles += len(processor.missingFiles)
				totalCleanedOrphans += len(processor.orphanMessages)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		color.Red("\nOne or more channels failed to process: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	color.Green("✓ All channels processed successfully\n")
	if !cfg.DryRun && len(channelExports) > 0 {
		jsonData, err := json.MarshalIndent(channelExports, "", "    ")
		if err != nil {
			color.Red("Failed to generate JSON export: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(cfg.ExportFile, jsonData, 0o644); err != nil {
			color.Red("Failed to write export file: %v\n", err)
			os.Exit(1)
		}
		color.Cyan("Exported %d incomplete files to %s\n", totalMissing, cfg.ExportFile)
	}

	fmt.Println()
	color.Cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	color.Cyan("                    Check Summary                   \n")
	color.Cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("  %-25s %d\n", "Channels Processed:", len(channelIDs))
	fmt.Printf("  %-25s %d\n", "Total Files Checked:", totalFiles)
	fmt.Printf("  %-25s %d\n", "Total Parts (DB):", totalPartsDB)
	fmt.Printf("  %-25s %d\n", "Total Messages (TG):", totalMessagesTG)
	fmt.Printf("  %-25s %d\n", "Missing Files:", totalMissing)
	fmt.Printf("  %-25s %d\n", "Orphan Messages:", totalOrphans)
	if cfg.DryRun {
		fmt.Printf("  %-25s %d\n", "Would Clean Files:", totalMissing)
		fmt.Printf("  %-25s %d\n", "Would Clean Orphans:", totalOrphans)
	} else {
		fmt.Printf("  %-25s %d\n", "Cleaned Files:", totalCleanedFiles)
		fmt.Printf("  %-25s %d\n", "Cleaned Orphans:", totalCleanedOrphans)
	}
	color.Cyan("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
}

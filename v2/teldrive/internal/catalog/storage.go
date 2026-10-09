package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

type StorageDashboard struct {
	Summary    StorageSummary
	Growth     []StorageGrowthPoint
	Categories []CategoryStatistic
	Channels   []StorageChannelStatistic
	Cleanup    StorageCleanupStatistics
	Activity   []StorageActivity
}

type StorageSummary struct {
	LogicalBytes  int64
	ActiveFiles   int64
	ActiveFolders int64
	TrashedFiles  int64
	TrashBytes    int64
}

type StorageGrowthPoint struct {
	Day          time.Time
	AddedBytes   int64
	LogicalBytes int64
}

type StorageChannelStatistic struct {
	ChannelID     int64
	Name          string
	Selected      bool
	Health        string
	LastCheckedAt *time.Time
	PartCount     int64
	StoredBytes   int64
}

type StorageCleanupStatistics struct {
	TrashBytes            int64
	StaleUploadBytes      int64
	StaleUploads          int64
	TotalReclaimableBytes int64
}

type StorageActivity struct {
	ID           int64
	Type         string
	ResourceType string
	ResourceID   string
	Label        string
	OccurredAt   time.Time
}

func (s *Service) StorageDashboard(ctx context.Context, userID int64) (StorageDashboard, error) {
	if userID <= 0 {
		return StorageDashboard{}, ErrInvalidOwner
	}
	totals, err := s.queries.GetStorageDashboardTotals(ctx, userID)
	if err != nil {
		return StorageDashboard{}, fmt.Errorf("get storage totals: %w", err)
	}
	cleanup, err := s.queries.GetStorageCleanupStatistics(ctx, userID)
	if err != nil {
		return StorageDashboard{}, fmt.Errorf("get storage cleanup statistics: %w", err)
	}
	growthRows, err := s.queries.ListStorageGrowth(ctx, userID)
	if err != nil {
		return StorageDashboard{}, fmt.Errorf("list storage growth: %w", err)
	}
	channelRows, err := s.queries.ListStorageChannelStatistics(ctx, userID)
	if err != nil {
		return StorageDashboard{}, fmt.Errorf("list storage channels: %w", err)
	}
	activityRows, err := s.queries.ListRecentStorageActivity(ctx, sqlcgen.ListRecentStorageActivityParams{
		UserID: userID, ActivityLimit: 8,
	})
	if err != nil {
		return StorageDashboard{}, fmt.Errorf("list storage activity: %w", err)
	}
	categories, err := s.CategoryStatistics(ctx, userID)
	if err != nil {
		return StorageDashboard{}, err
	}

	result := StorageDashboard{
		Summary: StorageSummary{
			LogicalBytes: totals.LogicalBytes,
			ActiveFiles:  totals.ActiveFiles, ActiveFolders: totals.ActiveFolders,
			TrashedFiles: totals.TrashedFiles, TrashBytes: totals.TrashBytes,
		},
		Categories: categories,
		Cleanup: StorageCleanupStatistics{
			TrashBytes: cleanup.TrashBytes, StaleUploadBytes: cleanup.StaleUploadBytes,
			StaleUploads:          cleanup.StaleUploads,
			TotalReclaimableBytes: cleanup.TrashBytes + cleanup.StaleUploadBytes,
		},
		Growth:   make([]StorageGrowthPoint, 0, len(growthRows)),
		Channels: make([]StorageChannelStatistic, 0, len(channelRows)),
		Activity: make([]StorageActivity, 0, len(activityRows)),
	}
	for _, row := range growthRows {
		result.Growth = append(result.Growth, StorageGrowthPoint{
			Day: row.Day.Time.UTC(), AddedBytes: row.AddedBytes, LogicalBytes: row.LogicalBytes,
		})
	}
	for _, row := range channelRows {
		item := StorageChannelStatistic{
			ChannelID: row.ChannelID, Name: row.Name, Selected: row.Selected, Health: row.Health,
			PartCount: row.PartCount, StoredBytes: row.StoredBytes,
		}
		if row.LastCheckedAt.Valid {
			value := row.LastCheckedAt.Time.UTC()
			item.LastCheckedAt = &value
		}
		result.Channels = append(result.Channels, item)
	}
	for _, row := range activityRows {
		item := StorageActivity{
			ID: row.ID, Type: row.EventType, ResourceType: row.ResourceType,
			OccurredAt: row.OccurredAt.Time.UTC(), Label: storageActivityLabel(row.EventType, row.Payload),
		}
		if row.ResourceID.Valid {
			item.ResourceID = row.ResourceID.String
		}
		result.Activity = append(result.Activity, item)
	}
	return result, nil
}

func storageActivityLabel(eventType string, payload []byte) string {
	var values map[string]any
	if json.Unmarshal(payload, &values) == nil {
		if name, ok := values["name"].(string); ok && name != "" {
			return name
		}
	}
	return eventType
}

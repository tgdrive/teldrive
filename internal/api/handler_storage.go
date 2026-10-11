package api

import (
	"context"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
)

func (h *Handler) GetStorageStats(ctx context.Context) (gen.GetStorageStatsRes, error) {
	userID, err := UserIDFromContext(ctx)
	if err != nil {
		return nil, mapServiceError(err)
	}
	if h.Catalog == nil {
		return nil, mapServiceError(ErrOperationUnavailable)
	}
	dashboard, err := h.Catalog.StorageDashboard(ctx, userID)
	if err != nil {
		return nil, mapServiceError(err)
	}

	response := gen.StorageDashboard{
		Summary: gen.StorageSummary{
			LogicalBytes:  dashboard.Summary.LogicalBytes,
			ActiveFiles:   dashboard.Summary.ActiveFiles,
			ActiveFolders: dashboard.Summary.ActiveFolders,
			TrashedFiles:  dashboard.Summary.TrashedFiles,
			TrashBytes:    dashboard.Summary.TrashBytes,
		},
		Growth:     make([]gen.StorageGrowthPoint, 0, len(dashboard.Growth)),
		Categories: make([]gen.FileCategoryStatistics, 0, len(dashboard.Categories)),
		Channels:   make([]gen.StorageChannelStatistic, 0, len(dashboard.Channels)),
		Cleanup: gen.StorageCleanupStatistics{
			TrashBytes:            dashboard.Cleanup.TrashBytes,
			StaleUploadBytes:      dashboard.Cleanup.StaleUploadBytes,
			StaleUploads:          dashboard.Cleanup.StaleUploads,
			TotalReclaimableBytes: dashboard.Cleanup.TotalReclaimableBytes,
		},
		Activity: make([]gen.StorageActivity, 0, len(dashboard.Activity)),
	}
	for _, point := range dashboard.Growth {
		response.Growth = append(response.Growth, gen.StorageGrowthPoint{
			Day: point.Day, AddedBytes: point.AddedBytes, LogicalBytes: point.LogicalBytes,
		})
	}
	for _, category := range dashboard.Categories {
		response.Categories = append(response.Categories, gen.FileCategoryStatistics{
			Category:   gen.FileCategory(category.Category),
			TotalFiles: category.TotalFiles,
			TotalSize:  category.TotalSize,
		})
	}
	for _, channel := range dashboard.Channels {
		item := gen.StorageChannelStatistic{
			ChannelId:   channel.ChannelID,
			Name:        channel.Name,
			Selected:    channel.Selected,
			Health:      channel.Health,
			PartCount:   channel.PartCount,
			StoredBytes: channel.StoredBytes,
		}
		if channel.LastCheckedAt != nil {
			item.LastCheckedAt = gen.NewOptDateTime(*channel.LastCheckedAt)
		}
		response.Channels = append(response.Channels, item)
	}
	for _, activity := range dashboard.Activity {
		item := gen.StorageActivity{
			ID:           activity.ID,
			Type:         activity.Type,
			ResourceType: activity.ResourceType,
			Label:        activity.Label,
			OccurredAt:   activity.OccurredAt,
		}
		if activity.ResourceID != "" {
			item.ResourceId = gen.NewOptString(activity.ResourceID)
		}
		response.Activity = append(response.Activity, item)
	}
	return &response, nil
}

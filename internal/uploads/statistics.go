package uploads

import (
	"context"
	"fmt"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

type DailyStatistic struct {
	Date           time.Time
	UploadedBytes  int64
	CompletedFiles int64
}

func (s *Service) Statistics(ctx context.Context, userID int64, days int32) ([]DailyStatistic, error) {
	if s == nil || s.queries == nil || userID <= 0 || days < 1 || days > 366 {
		return nil, ErrInvalidInput
	}
	rows, err := s.queries.ListUploadDailyStatistics(ctx, sqlcgen.ListUploadDailyStatisticsParams{
		Days: days, UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("query upload statistics: %w", err)
	}
	items := make([]DailyStatistic, 0, len(rows))
	for _, row := range rows {
		if !row.Day.Valid {
			return nil, fmt.Errorf("query upload statistics: invalid day")
		}
		items = append(items, DailyStatistic{
			Date: row.Day.Time, UploadedBytes: row.UploadedBytes, CompletedFiles: row.CompletedFiles,
		})
	}
	return items, nil
}

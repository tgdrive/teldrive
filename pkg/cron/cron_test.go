package cron

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/internal/database"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// Requires a disposable database (same as tests/integration); the jobs under
// test delete every pending row they can.
func openTestDB(t *testing.T) *gorm.DB {
	dsn := os.Getenv("TELDRIVE_DB_DATASOURCE")
	if dsn == "" {
		t.Skip("TELDRIVE_DB_DATASOURCE not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "teldrive.", SingularTable: false},
		NowFunc:        func() time.Time { return time.Now().UTC() },
	})
	require.NoError(t, err)
	require.NoError(t, database.MigrateDB(db))
	return db
}

func TestCleanJobsSkipUncleanableGroups(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	// A user with no stored session: their messages cannot be deleted.
	const userID = 555000111
	require.NoError(t, db.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.User{
		UserId: userID, Name: "Cron User", UserName: "cronuser",
	}).Error)
	db.Where("user_id = ?", userID).Delete(&models.Session{})
	t.Cleanup(func() {
		db.Where("user_id = ?", userID).Delete(&models.File{})
		db.Where("user_id = ?", userID).Delete(&models.Upload{})
	})

	pending := func(name string, channelID *int64, parts []api.Part) models.File {
		f := models.File{
			Name: name, Type: "file", MimeType: "text/plain", UserId: userID,
			Status: "pending_deletion", ChannelId: channelID,
			UpdatedAt: utils.Ptr(time.Now().UTC()),
		}
		if parts != nil {
			f.Parts = utils.Ptr(datatypes.NewJSONSlice(parts))
		}
		require.NoError(t, db.Create(&f).Error)
		return f
	}

	noParts := pending("no-parts", nil, nil)
	emptyParts := pending("empty-parts", utils.Ptr(int64(42)), []api.Part{})
	withParts := pending("with-parts", utils.Ptr(int64(42)), []api.Part{{ID: 7}})

	require.NoError(t, db.Create(&models.Upload{
		UploadId: "cron-test-upload", Name: "part", UserId: userID, PartNo: 1,
		PartId: 8, ChannelId: 42, Size: 1,
		CreatedAt: time.Now().UTC().Add(-48 * time.Hour),
	}).Error)

	svc := &CronService{
		db: db,
		cnf: &config.ServerCmdConfig{TG: config.TGConfig{
			Uploads: config.TGUpload{Retention: time.Hour},
		}},
		logger: zap.NewNop(),
	}

	svc.cleanFiles(ctx)
	svc.cleanUploads(ctx)

	exists := func(id string) bool {
		var n int64
		require.NoError(t, db.Model(&models.File{}).Where("id = ?", id).Count(&n).Error)
		return n > 0
	}
	assert.False(t, exists(noParts.ID), "pending file without parts is dropped")
	assert.False(t, exists(emptyParts.ID), "pending file with empty parts is dropped")
	assert.True(t, exists(withParts.ID), "file whose messages could not be deleted stays pending")

	var uploads int64
	require.NoError(t, db.Model(&models.Upload{}).Where("upload_id = ?", "cron-test-upload").Count(&uploads).Error)
	assert.Equal(t, int64(1), uploads, "expired upload is kept until its messages can be deleted")
}

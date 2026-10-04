package integration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/utils"
	"github.com/tgdrive/teldrive/pkg/models"
	"gorm.io/gorm/clause"
)

func TestDeleteIgnoresOtherUsersFiles(t *testing.T) {
	if testDB == nil {
		t.Fatal("DB not initialized")
	}
	service := newTestApiService(testDB)
	ctx, _ := getAuthenticatedContext(t, service)

	const otherUserID = 987654321
	require.NoError(t, testDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.User{
		UserId:   otherUserID,
		Name:     "Other User",
		UserName: "otheruser",
	}).Error)

	own, err := service.FilesCreate(ctx, &api.File{
		Name:      "own_file.txt",
		Type:      api.FileTypeFile,
		Size:      api.NewOptInt64(100),
		MimeType:  api.NewOptString("text/plain"),
		Path:      api.NewOptString("/"),
		ChannelId: api.NewOptInt64(999999),
		Parts:     []api.Part{{ID: 500}},
	})
	require.NoError(t, err)

	otherFolder := models.File{
		Name:      "other_folder",
		Type:      "folder",
		MimeType:  "drive/folder",
		UserId:    otherUserID,
		Status:    "active",
		UpdatedAt: utils.Ptr(time.Now().UTC()),
	}
	require.NoError(t, testDB.Create(&otherFolder).Error)

	otherFile := models.File{
		Name:      "other_file.txt",
		Type:      "file",
		MimeType:  "text/plain",
		UserId:    otherUserID,
		Status:    "active",
		UpdatedAt: utils.Ptr(time.Now().UTC()),
		ParentId:  &otherFolder.ID,
	}
	require.NoError(t, testDB.Create(&otherFile).Error)

	// The first id is owned by the caller, which used to be the only ownership check.
	err = service.FilesDelete(ctx, &api.FileDelete{
		Ids: []string{own.ID.Value, otherFile.ID, otherFolder.ID},
	})
	require.NoError(t, err)

	var ownGot models.File
	require.NoError(t, testDB.Where("id = ?", own.ID.Value).First(&ownGot).Error)
	assert.Equal(t, "pending_deletion", ownGot.Status)

	var fileGot models.File
	require.NoError(t, testDB.Where("id = ?", otherFile.ID).First(&fileGot).Error)
	assert.Equal(t, "active", fileGot.Status, "another user's file must not be deleted")

	var folderGot models.File
	require.NoError(t, testDB.Where("id = ?", otherFolder.ID).First(&folderGot).Error,
		"another user's folder must not be deleted")
}

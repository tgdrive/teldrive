package integration

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/internal/auth"
	"github.com/tgdrive/teldrive/internal/cache"
	"github.com/tgdrive/teldrive/internal/config"
	"github.com/tgdrive/teldrive/pkg/models"
	"gorm.io/gorm/clause"
)

func TestWholeFileFlow(t *testing.T) {
	if testDB == nil {
		t.Fatal("DB not initialized")
	}

	// 1. Setup
	err := createDummyUser(testDB)
	require.NoError(t, err)

	token, err := createSession(testDB)
	require.NoError(t, err)

	service := newTestApiService(testDB)
	ctx := context.Background()

	// 2. Auth Verification
	authRes, err := service.AuthSession(ctx, api.AuthSessionParams{AccessToken: api.NewOptString(token)})
	require.NoError(t, err)
	_, ok := authRes.(*api.SessionHeaders)
	require.True(t, ok, "AuthSession should return SessionHeaders")

	// Set up authenticated context
	c := cache.NewCache(context.Background(), config.CacheConfig{}.MaxSize, nil,nil)
	security := auth.NewSecurityHandler(testDB, c, &config.JWTConfig{Secret: testJWTSecret})
	ctx, err = security.HandleBearerAuth(ctx, "FilesCreate", api.BearerAuth{Token: token})
	require.NoError(t, err)

	// 3. Create Folder
	folderName := "Documents"
	folder, err := service.FilesCreate(ctx, &api.File{
		Name: folderName,
		Type: api.FileTypeFolder,
		Path: api.NewOptString("/"),
	})
	require.NoError(t, err)
	assert.Equal(t, folderName, folder.Name)
	assert.Equal(t, api.FileTypeFolder, folder.Type)

	// 4. Simulate File Upload (FilesCreate with parts)
	fileName := "test.txt"
	fileSize := int64(1024)
	partID := int64(100)
	// Manually insert a channel
	channelID := int64(999999)
	err = testDB.Clauses(clause.OnConflict{DoNothing: true}).Create(&models.Channel{
		UserId:      testUserID,
		ChannelId:   channelID,
		ChannelName: "Test Channel",
		Selected:    true,
	}).Error
	require.NoError(t, err)

	file, err := service.FilesCreate(ctx, &api.File{
		Name:      fileName,
		Type:      api.FileTypeFile,
		Size:      api.NewOptInt64(fileSize),
		MimeType:  api.NewOptString("text/plain"),
		ParentId:  api.NewOptString(folder.ID.Value),
		ChannelId: api.NewOptInt64(channelID),
		Parts: []api.Part{
			{ID: int(partID)},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, fileName, file.Name)
	assert.Equal(t, folder.ID.Value, file.ParentId.Value)

	// 5. List Files
	list, err := service.FilesList(ctx, api.FilesListParams{
		ParentId:  api.NewOptString(folder.ID.Value),
		Status:    api.NewOptFileQueryStatus(api.FileQueryStatusActive),
		Limit:     api.NewOptInt(10),
		Page:      api.NewOptInt(1),
		Operation: api.NewOptFileQueryOperation(api.FileQueryOperationList),
		Order:     api.NewOptFileQueryOrder(api.FileQueryOrderDesc),
		Sort:      api.NewOptFileQuerySort(api.FileQuerySortUpdatedAt),
	})
	require.NoError(t, err)
	require.Equal(t, 1, len(list.Items))
	assert.Equal(t, fileName, list.Items[0].Name)

	// 6. Move File (to root)
	err = service.FilesMove(ctx, &api.FileMove{
		Ids:               []string{file.ID.Value},
		DestinationParent: "/",
	})
	require.NoError(t, err)

	// Verify move
	movedFile, err := service.FilesGetById(ctx, api.FilesGetByIdParams{ID: file.ID.Value})
	require.NoError(t, err)
	assert.False(t, movedFile.ParentId.IsSet()) // Root has no parent

	// 7. Delete File
	err = service.FilesDelete(ctx, &api.FileDelete{
		Ids: []string{file.ID.Value},
	})
	require.NoError(t, err)

	// Verify deletion (status pending_deletion)
	var dbFile models.File
	err = testDB.Where("id = ?", file.ID.Value).First(&dbFile).Error
	require.NoError(t, err)
	assert.Equal(t, "pending_deletion", dbFile.Status)
}

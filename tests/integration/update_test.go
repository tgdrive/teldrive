package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tgdrive/teldrive/internal/api"
)

func TestFileUpdateEdgeCases(t *testing.T) {
	if testDB == nil {
		t.Fatal("DB not initialized")
	}
	service := newTestApiService(testDB)
	ctx, _ := getAuthenticatedContext(t, service)

	// 1. Setup File
	file, err := service.FilesCreate(ctx, &api.File{
		Name:      "update_test.txt",
		Type:      api.FileTypeFile,
		Size:      api.NewOptInt64(100),
		MimeType:  api.NewOptString("text/plain"),
		Path:      api.NewOptString("/"),
		ChannelId: api.NewOptInt64(999999),
		Parts:     []api.Part{{ID: 900}},
	})
	require.NoError(t, err)

	// 2. Standard Update (Rename)
	newName := "update_test_renamed.txt"
	updated, err := service.FilesUpdate(ctx, &api.FileUpdate{
		Name: api.NewOptString(newName),
	}, api.FilesUpdateParams{ID: file.ID.Value})
	require.NoError(t, err)
	assert.Equal(t, newName, updated.Name)

	// 3. Update with existing name (Collision)
	// Create another file
	file2, err := service.FilesCreate(ctx, &api.File{
		Name:      "collision.txt",
		Type:      api.FileTypeFile,
		Size:      api.NewOptInt64(100),
		MimeType:  api.NewOptString("text/plain"),
		Path:      api.NewOptString("/"),
		ChannelId: api.NewOptInt64(999999),
		Parts:     []api.Part{{ID: 901}},
	})
	require.NoError(t, err)

	// Try to rename file2 to newName (which exists)
	// This should FAIL due to unique constraint.
	_, err = service.FilesUpdate(ctx, &api.FileUpdate{
		Name: api.NewOptString(newName),
	}, api.FilesUpdateParams{ID: file2.ID.Value})
	assert.Error(t, err)

	// 4. Update Non-Existent
	_, err = service.FilesUpdate(ctx, &api.FileUpdate{
		Name: api.NewOptString("ghost"),
	}, api.FilesUpdateParams{ID: "00000000-0000-0000-0000-000000000000"})
	assert.Error(t, err) // Should be "file not found" or similar

	// 5. Empty Update
	// Update without fields. Should succeed and change nothing.
	updatedSame, err := service.FilesUpdate(ctx, &api.FileUpdate{}, api.FilesUpdateParams{ID: file.ID.Value})
	require.NoError(t, err)
	assert.Equal(t, newName, updatedSame.Name)
}

package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tgdrive/teldrive/internal/api"
	"github.com/tgdrive/teldrive/pkg/models"
)

func TestOverwriteRetiresReplacedParts(t *testing.T) {
	if testDB == nil {
		t.Fatal("DB not initialized")
	}
	service := newTestApiService(testDB)
	ctx, _ := getAuthenticatedContext(t, service)

	create := func(name string, parts ...int) *api.File {
		apiParts := make([]api.Part, 0, len(parts))
		for _, id := range parts {
			apiParts = append(apiParts, api.Part{ID: id})
		}
		file, err := service.FilesCreate(ctx, &api.File{
			Name:      name,
			Type:      api.FileTypeFile,
			Size:      api.NewOptInt64(100),
			MimeType:  api.NewOptString("text/plain"),
			Path:      api.NewOptString("/"),
			ChannelId: api.NewOptInt64(999999),
			Parts:     apiParts,
		})
		require.NoError(t, err)
		return file
	}

	pendingParts := func(name string) []int {
		var rows []models.File
		require.NoError(t, testDB.Where("name = ? AND status = 'pending_deletion'", name).Find(&rows).Error)
		var ids []int
		for _, row := range rows {
			require.NotNil(t, row.Parts)
			for _, part := range *row.Parts {
				ids = append(ids, part.ID)
			}
		}
		return ids
	}

	t.Run("overwrite queues old parts for deletion", func(t *testing.T) {
		first := create("overwrite.bin", 700, 701)
		second := create("overwrite.bin", 702, 703)

		assert.Equal(t, first.ID.Value, second.ID.Value, "file id is kept across overwrite")
		assert.ElementsMatch(t, []int{700, 701}, pendingParts("overwrite.bin"))

		var live models.File
		require.NoError(t, testDB.Where("id = ?", second.ID.Value).First(&live).Error)
		assert.Equal(t, "active", live.Status)
		require.NotNil(t, live.Parts)
		assert.Equal(t, 702, (*live.Parts)[0].ID)
	})

	t.Run("re-creating with the same parts deletes nothing", func(t *testing.T) {
		create("retry.bin", 710)
		create("retry.bin", 710)
		assert.Empty(t, pendingParts("retry.bin"))
	})

	t.Run("only parts dropped by the new file are queued", func(t *testing.T) {
		create("partial.bin", 720, 721)
		create("partial.bin", 721, 722)
		assert.ElementsMatch(t, []int{720}, pendingParts("partial.bin"))
	})

	t.Run("folder created over a file queues the file's parts", func(t *testing.T) {
		create("clash", 730)
		_, err := service.FilesCreate(ctx, &api.File{
			Name: "clash",
			Type: api.FileTypeFolder,
			Path: api.NewOptString("/"),
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []int{730}, pendingParts("clash"))
	})
}

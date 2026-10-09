package teldrive

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/google/uuid"
	"github.com/rclone/rclone/backend/teldrive/api"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/lib/rest"
)

var commandHelp = []fs.CommandHelp{
	{
		Name:  "trash-list",
		Short: "List top-level trashed files and directories.",
		Long: `Lists top-level trashed entries reachable from the configured remote root.

The result is returned as TelDrive file metadata including each entry ID.`,
	},
	{
		Name:  "restore",
		Short: "Restore a trashed file or directory by ID.",
		Long: `Restores one trashed file or directory.

Usage:

    rclone backend restore teldrive: FILE_ID`,
	},
	{
		Name:  "purge",
		Short: "Permanently purge a trashed file or directory by ID.",
		Long: `Schedules permanent deletion of one trashed file or directory and its subtree.

Usage:

    rclone backend purge teldrive: FILE_ID`,
	},
}

// CleanUp permanently purges every top-level trashed entry reachable from the
// configured root. Purging a trashed directory also purges its trashed subtree.
func (f *Fs) CleanUp(ctx context.Context) error {
	entries, err := f.listTopLevelTrash(ctx)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := f.purgeFile(ctx, entry.Id); err != nil {
			return fmt.Errorf("purge trashed entry %q (%s): %w", entry.Name, entry.Id, err)
		}
	}
	return nil
}

// Command implements TelDrive-specific trash lifecycle commands.
func (f *Fs) Command(ctx context.Context, name string, arg []string, _ map[string]string) (any, error) {
	switch name {
	case "trash-list":
		if len(arg) != 0 {
			return nil, fmt.Errorf("trash-list takes no arguments")
		}
		return f.listTopLevelTrash(ctx)
	case "restore":
		if len(arg) != 1 {
			return nil, fmt.Errorf("restore requires exactly one file ID")
		}
		return f.restoreFile(ctx, arg[0])
	case "purge":
		if len(arg) != 1 {
			return nil, fmt.Errorf("purge requires exactly one file ID")
		}
		return nil, f.purgeFile(ctx, arg[0])
	default:
		return nil, fs.ErrorCommandNotFound
	}
}

func (f *Fs) restoreFile(ctx context.Context, fileID string) (*api.FileInfo, error) {
	if _, err := uuid.Parse(fileID); err != nil {
		return nil, fmt.Errorf("invalid file ID %q: %w", fileID, err)
	}
	opts := rest.Opts{
		Method:       http.MethodPost,
		Path:         "/api/v1/files/" + fileID + "/restore",
		ExtraHeaders: map[string]string{"Idempotency-Key": uuid.NewString()},
	}
	var file api.FileInfo
	var resp *http.Response
	err := f.pacer.Call(func() (bool, error) {
		var callErr error
		resp, callErr = f.srv.CallJSON(ctx, &opts, nil, &file)
		return shouldRetry(ctx, resp, callErr)
	})
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func (f *Fs) purgeFile(ctx context.Context, fileID string) error {
	if _, err := uuid.Parse(fileID); err != nil {
		return fmt.Errorf("invalid file ID %q: %w", fileID, err)
	}
	opts := rest.Opts{
		Method:     http.MethodDelete,
		Path:       "/api/v1/files/" + fileID + "/purge",
		NoResponse: true,
	}
	return f.pacer.Call(func() (bool, error) {
		resp, err := f.srv.Call(ctx, &opts)
		return shouldRetry(ctx, resp, err)
	})
}

// listTopLevelTrash walks only the active directory tree. Trashed children of
// each active directory are top-level trash roots; their descendants are
// intentionally not listed separately.
func (f *Fs) listTopLevelTrash(ctx context.Context) ([]api.FileInfo, error) {
	root, err := f.dirCache.RootID(ctx, false)
	if err != nil {
		return nil, err
	}
	parents := []string{root}
	seen := map[string]struct{}{root: {}}
	var trashed []api.FileInfo

	for len(parents) > 0 {
		parentID := parents[0]
		parents = parents[1:]

		trashPage, err := f.listFilesByParent(ctx, parentID, "trashed")
		if err != nil {
			return nil, err
		}
		trashed = append(trashed, trashPage...)

		active, err := f.listFilesByParent(ctx, parentID, "active")
		if err != nil {
			return nil, err
		}
		for _, entry := range active {
			if entry.Kind != "folder" {
				continue
			}
			if _, ok := seen[entry.Id]; ok {
				continue
			}
			seen[entry.Id] = struct{}{}
			parents = append(parents, entry.Id)
		}
	}
	return trashed, nil
}

func (f *Fs) listFilesByParent(ctx context.Context, parentID, status string) ([]api.FileInfo, error) {
	cursor := ""
	var files []api.FileInfo
	for {
		parameters := url.Values{
			"limit":  []string{"200"},
			"status": []string{status},
		}
		setParentParameter(parameters, parentID)
		if cursor != "" {
			parameters.Set("cursor", cursor)
		}
		opts := rest.Opts{Method: http.MethodGet, Path: "/api/v1/files", Parameters: parameters}
		var page api.ReadMetadataResponse
		var resp *http.Response
		err := f.pacer.Call(func() (bool, error) {
			var callErr error
			resp, callErr = f.srv.CallJSON(ctx, &opts, nil, &page)
			return shouldRetry(ctx, resp, callErr)
		})
		if err != nil {
			return nil, err
		}
		// The root trash query includes entries whose parent is any active
		// folder. Restrict it to direct children while walking the tree.
		for _, entry := range page.Files {
			if parentIDOrRoot(entry.ParentId) == parentIDOrRoot(parentID) {
				files = append(files, entry)
			}
		}
		if page.NextCursor == "" {
			return files, nil
		}
		cursor = page.NextCursor
	}
}

var (
	_ fs.CleanUpper = (*Fs)(nil)
	_ fs.Commander  = (*Fs)(nil)
)

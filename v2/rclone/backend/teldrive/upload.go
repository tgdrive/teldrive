// Package teldrive implements TelDrive v2 upload sessions.
package teldrive

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rclone/rclone/backend/teldrive/api"
	"github.com/rclone/rclone/backend/teldrive/tdhash"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/fs/operations"
	"github.com/rclone/rclone/lib/pool"
	"github.com/rclone/rclone/lib/rest"
)

type storedPart struct {
	size     int64
	checksum string
}

type uploadInfo struct {
	uploadID     string
	chunkSize    int64
	expectedSize int64
	totalChunks  int64
	fileName     string
	dir          string
	storedParts  map[int]storedPart
}

type objectChunkWriter struct {
	o          *Object
	uploadInfo *uploadInfo
}

func (w *objectChunkWriter) WriteChunk(ctx context.Context, chunkNumber int, reader io.ReadSeeker) (int64, error) {
	if chunkNumber < 0 {
		return 0, fmt.Errorf("invalid chunk number %d", chunkNumber)
	}
	var size int64
	var checksum string
	if w.uploadInfo.expectedSize >= 0 {
		offset := int64(chunkNumber) * w.uploadInfo.chunkSize
		if offset < 0 || offset >= w.uploadInfo.expectedSize {
			return 0, fmt.Errorf("upload chunk %d exceeds expected size", chunkNumber)
		}
		size = min(w.uploadInfo.chunkSize, w.uploadInfo.expectedSize-offset)
	} else {
		var err error
		size, err = reader.Seek(0, io.SeekEnd)
		if err != nil {
			return 0, err
		}
		if size == 0 {
			return 0, nil
		}
		if _, err := reader.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		hasher := tdhash.New()
		if _, err := io.CopyN(hasher, reader, size); err != nil {
			return 0, err
		}
		checksum = hex.EncodeToString(hasher.Sum(nil))
		if _, err := reader.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
	}
	if stored, ok := w.uploadInfo.storedParts[chunkNumber+1]; ok {
		if stored.size == size && (checksum == "" || strings.EqualFold(stored.checksum, checksum)) {
			if w.uploadInfo.expectedSize >= 0 {
				switch r := reader.(type) {
				case *operations.ReOpen:
					r.Account(int(stored.size))
				case *pool.RW:
					r.Account(int(stored.size))
				}
			}
			fs.Debugf(w.o, "Reusing previously uploaded part %d", chunkNumber+1)
			return size, nil
		}
		resumeErr := fmt.Errorf("stored upload part %d does not match source", chunkNumber+1)
		if abortErr := w.Abort(ctx); abortErr != nil {
			return 0, errors.Join(resumeErr, fmt.Errorf("abort incompatible upload: %w", abortErr))
		}
		return 0, resumeErr
	}
	if err := w.o.putUploadPart(ctx, w.uploadInfo, chunkNumber+1, reader, size, checksum); err != nil {
		return 0, err
	}
	return size, nil
}

func (w *objectChunkWriter) Close(ctx context.Context) error {
	_, err := w.o.completeUpload(ctx, w.uploadInfo)
	return err
}

func (w *objectChunkWriter) Abort(ctx context.Context) error {
	return w.o.abortUpload(ctx, w.uploadInfo)
}

func (o *Object) prepareUpload(ctx context.Context, remote string, src fs.ObjectInfo) (*uploadInfo, error) {
	leaf, directoryID, err := o.fs.dirCache.FindPath(ctx, remote, true)
	if err != nil {
		return nil, err
	}
	request := api.UploadCreateRequest{
		ParentId:          requestParentID(directoryID),
		Name:              o.fs.opt.Enc.FromStandardName(leaf),
		Size:              src.Size(),
		MimeType:          fs.MimeType(ctx, src),
		ModTime:           src.ModTime(ctx).UTC(),
		Encryption:        o.fs.opt.EncryptFiles,
		ConflictPolicy:    "replace",
		PreferredPartSize: int64(o.fs.opt.ChunkSize),
	}
	if o.fs.opt.HashEnabled {
		value, hashErr := src.Hash(ctx, telDriveHash)
		switch {
		case hashErr == nil && value != "":
			request.Hash = &api.FileHash{Algorithm: "blake3", Value: value}
		case hashErr != nil && !errors.Is(hashErr, hash.ErrUnsupported):
			return nil, fmt.Errorf("read source BLAKE3 hash: %w", hashErr)
		}
	}

	resumed, err := o.findResumableUpload(ctx, request, leaf, directoryID)
	if err != nil {
		return nil, err
	}
	if resumed != nil {
		fs.Debugf(o.fs, "Resuming TelDrive upload %s with %d stored parts", resumed.uploadID, len(resumed.storedParts))
		return resumed, nil
	}

	opts := rest.Opts{
		Method:       http.MethodPost,
		Path:         "/api/v1/uploads",
		ExtraHeaders: map[string]string{"Idempotency-Key": uuid.NewString()},
	}
	var session api.UploadSession
	var resp *http.Response
	err = o.fs.pacer.Call(func() (bool, error) {
		var callErr error
		resp, callErr = o.fs.srv.CallJSON(ctx, &opts, &request, &session)
		return shouldRetry(ctx, resp, callErr)
	})
	if err != nil {
		return nil, err
	}
	return uploadInfoFromSession(session, leaf, directoryID, nil)
}

func (o *Object) findResumableUpload(ctx context.Context, request api.UploadCreateRequest, leaf, directoryID string) (*uploadInfo, error) {
	cursor := ""
	for {
		parameters := url.Values{"state": []string{"open"}, "limit": []string{"200"}}
		if cursor != "" {
			parameters.Set("cursor", cursor)
		}
		opts := rest.Opts{Method: http.MethodGet, Path: "/api/v1/uploads", Parameters: parameters}
		var page api.UploadSessionPage
		var resp *http.Response
		err := o.fs.pacer.Call(func() (bool, error) {
			var callErr error
			resp, callErr = o.fs.srv.CallJSON(ctx, &opts, nil, &page)
			return shouldRetry(ctx, resp, callErr)
		})
		if err != nil {
			return nil, err
		}
		for _, session := range page.Items {
			if !uploadSessionMatches(session, request, time.Now().UTC()) {
				continue
			}
			parts, err := o.listUploadParts(ctx, session.ID)
			if err != nil {
				var apiErr *apiError
				if errors.As(err, &apiErr) && (apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusGone) {
					continue
				}
				return nil, err
			}
			stored, ok := compatibleStoredParts(session, parts)
			if !ok {
				continue
			}
			return uploadInfoFromSession(session, leaf, directoryID, stored)
		}
		if page.NextCursor == "" {
			return nil, nil
		}
		cursor = page.NextCursor
	}
}

func (o *Object) listUploadParts(ctx context.Context, uploadID string) ([]api.UploadPart, error) {
	cursor := ""
	var parts []api.UploadPart
	for {
		parameters := url.Values{"limit": []string{"200"}}
		if cursor != "" {
			parameters.Set("cursor", cursor)
		}
		opts := rest.Opts{
			Method:     http.MethodGet,
			Path:       "/api/v1/uploads/" + uploadID + "/parts",
			Parameters: parameters,
		}
		var page api.UploadPartPage
		var resp *http.Response
		err := o.fs.pacer.Call(func() (bool, error) {
			var callErr error
			resp, callErr = o.fs.srv.CallJSON(ctx, &opts, nil, &page)
			return shouldRetry(ctx, resp, callErr)
		})
		if err != nil {
			return nil, err
		}
		parts = append(parts, page.Items...)
		if page.NextCursor == "" {
			return parts, nil
		}
		cursor = page.NextCursor
	}
}

func uploadSessionMatches(session api.UploadSession, request api.UploadCreateRequest, now time.Time) bool {
	return session.State == "open" &&
		session.ID != "" && session.PartSize > 0 && session.ExpiresAt.After(now) &&
		session.ParentId == request.ParentId && session.Name == request.Name &&
		session.ExpectedSize == request.Size && session.MimeType == request.MimeType &&
		session.Encryption == request.Encryption && session.ConflictPolicy == request.ConflictPolicy &&
		session.ModTime.UTC().Truncate(time.Microsecond).Equal(request.ModTime.UTC().Truncate(time.Microsecond)) &&
		equalFileHash(session.ExpectedHash, request.Hash)
}

func equalFileHash(left, right *api.FileHash) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Algorithm == right.Algorithm && left.Value == right.Value
}

func compatibleStoredParts(session api.UploadSession, parts []api.UploadPart) (map[int]storedPart, bool) {
	stored := make(map[int]storedPart)
	maxPart := 0
	for _, part := range parts {
		if part.State != "stored" {
			continue
		}
		if part.PartNo < 1 || part.PlainSize <= 0 {
			return nil, false
		}
		if session.ExpectedSize >= 0 {
			expected, ok := expectedUploadPartSize(session.ExpectedSize, session.PartSize, part.PartNo)
			if !ok || part.PlainSize != expected {
				return nil, false
			}
		} else {
			checksum, err := hex.DecodeString(part.Checksum)
			if part.PlainSize > session.PartSize || err != nil || len(checksum) != tdhash.Size/2 {
				return nil, false
			}
		}
		stored[part.PartNo] = storedPart{size: part.PlainSize, checksum: part.Checksum}
		maxPart = max(maxPart, part.PartNo)
	}
	if session.ExpectedSize < 0 {
		if maxPart != len(stored) {
			return nil, false
		}
		for partNo := 1; partNo < maxPart; partNo++ {
			if stored[partNo].size != session.PartSize {
				return nil, false
			}
		}
	}
	return stored, true
}

func expectedUploadPartSize(totalSize, partSize int64, partNo int) (int64, bool) {
	if totalSize < 0 || partSize <= 0 || partNo < 1 {
		return 0, false
	}
	offset := int64(partNo-1) * partSize
	if offset >= totalSize {
		return 0, false
	}
	remaining := totalSize - offset
	if remaining < partSize {
		return remaining, true
	}
	return partSize, true
}

func uploadInfoFromSession(session api.UploadSession, leaf, directoryID string, stored map[int]storedPart) (*uploadInfo, error) {
	if session.ID == "" || session.PartSize <= 0 || session.ExpectedSize < -1 {
		return nil, fmt.Errorf("invalid TelDrive upload session")
	}
	totalChunks := int64(0)
	if session.ExpectedSize > 0 {
		totalChunks = (session.ExpectedSize + session.PartSize - 1) / session.PartSize
	}
	if stored == nil {
		stored = make(map[int]storedPart)
	}
	return &uploadInfo{
		uploadID: session.ID, chunkSize: session.PartSize, expectedSize: session.ExpectedSize, totalChunks: totalChunks,
		fileName: leaf, dir: directoryID, storedParts: stored,
	}, nil
}
func (o *Object) uploadMultipart(ctx context.Context, remote string, in io.Reader, src fs.ObjectInfo) (*uploadInfo, error) {
	info, err := o.prepareUpload(ctx, remote, src)
	if err != nil {
		return nil, err
	}
	var consumed int64
	for partNo := int64(1); partNo <= info.totalChunks; partNo++ {
		partSize := info.chunkSize
		if remaining := src.Size() - consumed; remaining < partSize {
			partSize = remaining
		}
		if _, ok := info.storedParts[int(partNo)]; ok {
			if _, err := io.CopyN(io.Discard, in, partSize); err != nil {
				return nil, fmt.Errorf("skip stored upload part %d: %w", partNo, err)
			}
			consumed += partSize
			continue
		}
		if err := o.putUploadPart(ctx, info, int(partNo), io.LimitReader(in, partSize), partSize, ""); err != nil {
			return nil, err
		}
		consumed += partSize
	}
	return info, nil
}

func (o *Object) putUploadPart(ctx context.Context, info *uploadInfo, partNo int, reader io.Reader, size int64, checksum string) error {
	if info == nil || info.uploadID == "" || partNo < 1 || size < 0 {
		return fmt.Errorf("invalid TelDrive upload part")
	}
	path := fmt.Sprintf("/api/v1/uploads/%s/parts/%d", info.uploadID, partNo)
	opts := rest.Opts{
		Method: "PUT", Path: path, Body: reader, ContentLength: &size,
		ContentType: "application/octet-stream",
	}
	if checksum != "" {
		opts.ExtraHeaders = map[string]string{"X-Part-Checksum": checksum}
	}
	if o.fs.opt.UploadHost != "" {
		opts.RootURL = strings.TrimRight(o.fs.opt.UploadHost, "/") + path
		opts.Path = ""
	}
	var resp *http.Response
	var previousErr error
	attempt := 0
	err := o.fs.pacer.Call(func() (bool, error) {
		if attempt > 0 {
			seeker, ok := reader.(io.Seeker)
			if !ok {
				return false, previousErr
			}
			if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr != nil {
				return false, seekErr
			}
		}
		attempt++
		var callErr error
		resp, callErr = o.fs.srv.Call(ctx, &opts)
		previousErr = callErr
		return shouldRetry(ctx, resp, callErr)
	})
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	return err
}

func (o *Object) completeUpload(ctx context.Context, info *uploadInfo) (*api.FileInfo, error) {
	if info == nil || info.uploadID == "" {
		return nil, fmt.Errorf("invalid TelDrive upload session")
	}
	opts := rest.Opts{
		Method:       "POST",
		Path:         "/api/v1/uploads/" + info.uploadID + "/complete",
		ExtraHeaders: map[string]string{"Idempotency-Key": uuid.NewString()},
	}
	var file api.FileInfo
	var resp *http.Response
	err := o.fs.pacer.Call(func() (bool, error) {
		var callErr error
		resp, callErr = o.fs.srv.CallJSON(ctx, &opts, nil, &file)
		return shouldRetry(ctx, resp, callErr)
	})
	if err != nil {
		return nil, err
	}
	o.applyFileInfo(&file)
	return &file, nil
}

func (o *Object) abortUpload(ctx context.Context, info *uploadInfo) error {
	if info == nil || info.uploadID == "" {
		return nil
	}
	opts := rest.Opts{Method: "DELETE", Path: "/api/v1/uploads/" + info.uploadID, NoResponse: true}
	return o.fs.pacer.Call(func() (bool, error) {
		resp, err := o.fs.srv.Call(ctx, &opts)
		return shouldRetry(ctx, resp, err)
	})
}

func (o *Object) createFile(ctx context.Context, _ fs.ObjectInfo, info *uploadInfo) error {
	_, err := o.completeUpload(ctx, info)
	return err
}

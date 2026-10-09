// Package teldrive provides an interface to the TelDrive storage system.
//
// TelDrive is a self-hosted file storage solution that uses Telegram
// channels as the underlying storage backend. Files are split into chunks,
// optionally encrypted, and uploaded as messages in a designated Telegram
// channel or group. The TelDrive API server indexes and manages the file
// metadata, providing a file-system-like interface over Telegram's storage.
//
// Key features:
//   - Chunked uploads with configurable chunk size (auto-aligned to 16 MiB)
//   - BLAKE3 tree hashing for integrity verification across 16 MiB blocks
//   - Optional client-side encryption via the TelDrive server
//   - SSE-based real-time change notification
//   - Server-side copy, move, and directory operations
//   - Public link sharing with optional password protection
package teldrive

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rclone/rclone/backend/teldrive/api"
	"github.com/rclone/rclone/backend/teldrive/tdhash"
	"github.com/rclone/rclone/fs"
	"github.com/rclone/rclone/fs/config"
	"github.com/rclone/rclone/fs/config/configmap"
	"github.com/rclone/rclone/fs/config/configstruct"
	"github.com/rclone/rclone/fs/fserrors"
	"github.com/rclone/rclone/fs/fshttp"
	"github.com/rclone/rclone/fs/hash"
	"github.com/rclone/rclone/fs/list"
	"github.com/rclone/rclone/lib/dircache"
	"github.com/rclone/rclone/lib/encoder"
	"github.com/rclone/rclone/lib/multipart"
	"github.com/rclone/rclone/lib/pacer"
	"github.com/rclone/rclone/lib/rest"
)

const (
	rootID = "root"

	// timeFormat is the format used for timestamps in API requests
	timeFormat = time.RFC3339

	// maxChunkSize is the maximum upload chunk size (2000 MiB).
	// This is 125 × 16 MiB blocks, constrained by Telegram's per-message
	// upload limit.
	maxChunkSize = 2000 * fs.Mebi

	// defaultChunkSize is the default upload chunk size (512 MiB).
	// This equals 32 × 16 MiB blocks, chosen for optimal BLAKE3 tree
	// hashing performance with a good balance of concurrency and memory use.
	defaultChunkSize = 512 * fs.Mebi

	// minChunkSize is the minimum allowed upload chunk size (64 MiB).
	// This equals 4 × 16 MiB blocks — smaller values would waste
	// Telegram message capacity.
	minChunkSize = 64 * fs.Mebi

	// apiKeyHeaderName is the HTTP header used to pass the API key
	apiKeyHeaderName = "X-Api-Key"
)

var telDriveHash hash.Type

func init() {
	fs.Register(&fs.RegInfo{
		Name:        "teldrive",
		Description: "Tel Drive",
		NewFs:       NewFs,
		CommandHelp: commandHelp,
		Options: []fs.Option{{
			Help: `API key for authentication with the TelDrive server.

This is the API token used to authenticate requests. Obtain it from
your TelDrive dashboard or server admin.
`,
			Name:      "api_key",
			Sensitive: true,
		}, {
			Help: `URL of the TelDrive API server.

The base URL for the TelDrive API endpoint (e.g. https://teldrive.example.com).
This is required for all operations.
`,
			Name:      "api_host",
			Sensitive: true,
		}, {
			Help: `Upload chunk size.

Files larger than this will be uploaded in multiple chunks. The chunk size
is automatically aligned to the nearest 16 MiB multiple (the BLAKE3 tree
hash block size) for optimal hashing performance.

Larger chunk sizes reduce the number of API calls but use more memory
during uploads (one chunk is buffered per concurrent upload stream).
Minimum: 64 MiB, Maximum: 2000 MiB.
`,
			Name:    "chunk_size",
			Default: defaultChunkSize,
		}, {
			Name: "link_password",
			Help: `Password to set on created public links.

When set, all public links created via the link command will require
this password to access. If not set, links will be publicly accessible.
`,
		}, {
			Help: `Number of items to return per page when listing files.

Controls the page size for directory listing API calls. Larger values
reduce the number of API requests needed for directories with many files,
but may increase response time and memory usage for each request.
`,
			Name:    "page_size",
			Default: 200,
		}, {
			Name:    "upload_concurrency",
			Default: 4,
			Help: `Number of chunks to upload in parallel per file.

Higher concurrency can speed up large file uploads at the cost of
more memory usage and potential rate-limiting. Each concurrent upload
buffers one chunk in memory.
`,
			Advanced: true,
		}, {
			Help: `Optional alternative URL for upload API calls.

If set, upload chunk requests will be sent to this host instead of
the main api_host. Useful for load-balancing or directing upload
traffic to a different server or CDN endpoint.
`,
			Name:      "upload_host",
			Sensitive: true,
		}, {
			Name:    "encrypt_files",
			Default: false,
			Help: `Enable native TelDrive encryption for stored files.

When enabled, files will be encrypted at rest using TelDrive's
built-in encryption before being sent to Telegram. The encryption
keys are managed by the TelDrive server.
`,
		}, {
			Name:    "hash_enabled",
			Default: true,
			Help: `Enable BLAKE3 tree hashing for file integrity verification.

Files are split into 16 MiB blocks, each hashed with BLAKE3, then
the block hashes are combined into a final tree hash. This allows
rclone to verify upload integrity and detect modifications.

Disable only if the server does not support this hash type.
`,
		}, {
			Name:     config.ConfigEncoding,
			Help:     config.ConfigEncodingHelp,
			Advanced: true,
			Default:  encoder.Standard | encoder.EncodeBackSlash | encoder.EncodeLeftSpace | encoder.EncodeRightSpace | encoder.EncodeInvalidUtf8,
		}},
	})

	telDriveHash = hash.RegisterHash("teldrive", "TelDriveHash", tdhash.Size, tdhash.New)
}

// Options defines the configuration for this backend.
// These are set via the rclone config file, command-line flags, or
// environment variables prefixed with RCLONE_TELDRIVE_.
type Options struct {
	ApiHost           string               `config:"api_host"`           // URL of the TelDrive API server
	UploadHost        string               `config:"upload_host"`        // Optional alternative host for uploads
	APIKey            string               `config:"api_key"`            // API authentication token
	LinkPassword      string               `config:"link_password"`      // Password for public share links
	ChunkSize         fs.SizeSuffix        `config:"chunk_size"`         // Upload chunk size (auto-aligned to 16 MiB)
	UploadConcurrency int                  `config:"upload_concurrency"` // Parallel upload streams per file
	EncryptFiles      bool                 `config:"encrypt_files"`      // Enable server-side encryption
	PageSize          int64                `config:"page_size"`          // Directory listing page size
	HashEnabled       bool                 `config:"hash_enabled"`       // Enable BLAKE3 tree hashing
	Enc               encoder.MultiEncoder `config:"encoding"`           // Filename encoding rules
}

// Fs represents a remote TelDrive file system.
// It manages the connection to the TelDrive API, directory cache,
// rate-limiting pacers, and per-remote configuration.
type Fs struct {
	root     string             // Root path on this remote
	name     string             // Name of this remote in the config
	opt      Options            // Backend configuration
	features *fs.Features       // Optional feature flags for this backend
	srv      *rest.Client       // HTTP client for API calls
	pacer    *fs.Pacer          // Rate limiter for API requests
	ssePacer *fs.Pacer          // Dedicated pacer for SSE connection retries
	userId   int64              // Authenticated user ID from session
	dirCache *dircache.DirCache // Cached directory tree for fast path resolution
}

// Object represents an teldrive object
type Object struct {
	fs       *Fs
	remote   string
	id       string
	size     int64
	parentId string
	name     string
	modTime  time.Time
	mimeType string
	hash     string // BLAKE3 tree hash from server
}

type apiError struct {
	StatusCode int
	Code       string
	Message    string
}

func (e *apiError) Error() string {
	if e == nil {
		return "TelDrive API error"
	}
	if e.Code != "" && e.Message != "" {
		return fmt.Sprintf("TelDrive API error %d (%s): %s", e.StatusCode, e.Code, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("TelDrive API error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("TelDrive API error %d", e.StatusCode)
}

func (e *apiError) Unwrap() error {
	if e == nil {
		return nil
	}
	switch e.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fs.ErrorPermissionDenied
	case http.StatusNotFound:
		return fs.ErrorObjectNotFound
	case http.StatusPreconditionFailed:
		return fs.ErrorImmutableModified
	default:
		return nil
	}
}

func errorHandler(resp *http.Response) error {
	envelope := new(api.ErrorEnvelope)
	if err := rest.DecodeJSON(resp, envelope); err != nil {
		return &apiError{StatusCode: resp.StatusCode, Message: resp.Status}
	}
	return &apiError{
		StatusCode: resp.StatusCode,
		Code:       envelope.Error.Code,
		Message:    envelope.Error.Message,
	}
}

// Name of the remote (as passed into NewFs)
func (f *Fs) Name() string {
	return f.name
}

// Root of the remote (as passed into NewFs)
func (f *Fs) Root() string {
	return f.root
}

// String returns a description of the FS
func (f *Fs) String() string {
	return fmt.Sprintf("teldrive root '%s'", f.root)
}

// Precision of the ModTimes in this Fs
func (f *Fs) Precision() time.Duration {
	return time.Second
}

// Hashes returns the supported hash types of the filesystem
// TelDrive uses BLAKE3 tree hashing only (16MB fixed blocks)
func (f *Fs) Hashes() hash.Set {
	if f.opt.HashEnabled {
		return hash.Set(telDriveHash)
	}
	return hash.NewHashSet(hash.None)

}

// Features returns the optional features of this Fs
func (f *Fs) Features() *fs.Features {
	return f.features
}

// retryErrorCodes is a slice of error codes that we will retry
var retryErrorCodes = []int{
	429, // Too Many Requests.
	500, // Internal Server Error
	502, // Bad Gateway
	503, // Service Unavailable
	504, // Gateway Timeout
	509, // Bandwidth Limit Exceeded
}

// shouldRetry returns a boolean as to whether this resp and err
// deserve to be retried.  It returns the err as a convenience
func shouldRetry(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if fserrors.ContextError(ctx, &err) {
		return false, err
	}
	return fserrors.ShouldRetry(err) || fserrors.ShouldRetryHTTP(resp, retryErrorCodes), err
}

// alignChunkSize rounds the chunk size to the nearest 16MB multiple
// and clamps it to min/max bounds
func alignChunkSize(cs fs.SizeSuffix) fs.SizeSuffix {
	blockSize := int64(16 * 1024 * 1024) // 16MB
	chunkSizeBytes := min(max(int64(cs), int64(minChunkSize)), int64(maxChunkSize))
	// Round to nearest 16MB multiple
	// Ensure we don't exceed max after rounding
	alignedSize := min(((chunkSizeBytes+blockSize/2)/blockSize)*blockSize, int64(maxChunkSize))

	return fs.SizeSuffix(alignedSize)
}

func Ptr[T any](t T) *T {
	return &t
}

// NewFs makes a new Fs object from the path
//
// The path is of the form remote:path
//
// Remotes are looked up in the config file.  If the remote isn't
// found then NotFoundInConfigFile will be returned.
//
// On Windows avoid single character remote names as they can be mixed
// up with drive letters.
func NewFs(ctx context.Context, name string, root string, config configmap.Mapper) (fs.Fs, error) {
	opt := new(Options)
	err := configstruct.Set(config, opt)
	if err != nil {
		return nil, err
	}

	// Align chunk size to 16MB multiple for optimal BLAKE3 tree hashing
	opt.ChunkSize = alignChunkSize(opt.ChunkSize)
	if opt.PageSize < 1 {
		opt.PageSize = 100
	} else if opt.PageSize > 200 {
		opt.PageSize = 200
	}

	f := &Fs{
		name:  name,
		root:  root,
		opt:   *opt,
		pacer: fs.NewPacer(ctx, pacer.NewDefault()),
		// Dedicated SSE pacer with optimized settings for connection retries
		ssePacer: fs.NewPacer(ctx, pacer.NewDefault(
			pacer.MinSleep(1*time.Second),
			pacer.MaxSleep(30*time.Second),
			pacer.DecayConstant(2),
		)),
	}

	f.root = strings.Trim(root, "/")

	f.features = (&fs.Features{
		CanHaveEmptyDirectories: true,
		ReadMimeType:            true,
		WriteMimeType:           true,
		ChunkWriterDoesntSeek:   true,
	}).Fill(ctx, f)

	client := fshttp.NewClient(ctx)
	f.srv = rest.NewClient(client).
		SetRoot(strings.Trim(opt.ApiHost, "/")).
		SetErrorHandler(errorHandler)
	if opt.APIKey == "" {
		return nil, errors.New("missing api_key")
	}
	f.srv.SetHeader(apiKeyHeaderName, opt.APIKey)

	opts := rest.Opts{
		Method: "GET",
		Path:   "/api/v1/me",
	}

	var (
		session     api.UserProfile
		sessionResp *http.Response
	)

	err = f.pacer.Call(func() (bool, error) {
		sessionResp, err = f.srv.CallJSON(ctx, &opts, nil, &session)
		return shouldRetry(ctx, sessionResp, err)
	})

	if err != nil {
		return nil, err
	}
	if session.UserId == 0 {
		return nil, errors.New("invalid session")
	}

	f.userId = session.UserId

	f.dirCache = dircache.New(f.root, rootID, f)
	err = f.dirCache.FindRoot(ctx, false)
	if err != nil {
		// Assume it is a file
		newRoot, remote := dircache.SplitPath(root)
		tempF := *f
		tempF.dirCache = dircache.New(newRoot, rootID, &tempF)
		tempF.root = newRoot
		err = tempF.dirCache.FindRoot(ctx, false)
		if err != nil {
			// No root so return old f
			return f, nil
		}
		_, err := tempF.NewObject(ctx, remote)
		if err != nil {
			if errors.Is(err, fs.ErrorObjectNotFound) || errors.Is(err, fs.ErrorIsDir) {
				// File doesn't exist so return old f
				return f, nil
			}
			return nil, err
		}
		f.features.Fill(ctx, &tempF)
		// XXX: update the old f here instead of returning tempF, since
		// `features` were already filled with functions having *f as a receiver.
		// See https://github.com/rclone/rclone/issues/2182
		f.dirCache = tempF.dirCache
		f.root = tempF.root
		return f, fs.ErrorIsFile

	}
	return f, nil
}

func (f *Fs) readMetaDataForPath(ctx context.Context, path string, options *api.MetadataRequestOptions) (*api.ReadMetadataResponse, error) {
	directoryID, err := f.dirCache.FindDir(ctx, path, false)
	if err != nil {
		return nil, err
	}
	opts := rest.Opts{
		Method: "GET",
		Path:   "/api/v1/files",
		Parameters: url.Values{
			"limit": []string{strconv.FormatInt(options.Limit, 10)},
		},
	}
	setParentParameter(opts.Parameters, directoryID)
	if options.Cursor != "" {
		opts.Parameters.Set("cursor", options.Cursor)
	}
	if options.Status != "" {
		opts.Parameters.Set("status", options.Status)
	}
	var info api.ReadMetadataResponse
	var resp *http.Response
	err = f.pacer.Call(func() (bool, error) {
		resp, err = f.srv.CallJSON(ctx, &opts, nil, &info)
		return shouldRetry(ctx, resp, err)
	})
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (f *Fs) getFileShares(ctx context.Context, id string) ([]api.FileShare, error) {
	cursor := ""
	var shares []api.FileShare
	for {
		parameters := url.Values{"limit": []string{"200"}}
		if cursor != "" {
			parameters.Set("cursor", cursor)
		}
		opts := rest.Opts{
			Method: "GET", Path: "/api/v1/files/" + id + "/shares", Parameters: parameters,
		}
		var page api.FileSharePage
		var resp *http.Response
		err := f.pacer.Call(func() (bool, error) {
			var callErr error
			resp, callErr = f.srv.CallJSON(ctx, &opts, nil, &page)
			return shouldRetry(ctx, resp, callErr)
		})
		if err != nil {
			if resp != nil && resp.StatusCode == http.StatusNotFound {
				return nil, fs.ErrorObjectNotFound
			}
			return nil, err
		}
		shares = append(shares, page.Items...)
		if page.NextCursor == "" {
			return shares, nil
		}
		cursor = page.NextCursor
	}
}

func (f *Fs) getFileShare(ctx context.Context, id string) (*api.FileShare, error) {
	res, err := f.getFileShares(ctx, id)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for i := range res {
		if res[i].ExpiresAt != nil && res[i].ExpiresAt.UTC().Before(now) {
			continue
		}
		return &res[i], nil
	}
	return nil, fs.ErrorObjectNotFound
}

// List the objects and directories in dir into entries.  The
// entries can be returned in any order but should be for a
// complete directory.
//
// dir should be "" to list the root, and should not have
// trailing slashes.
//
// This should return ErrDirNotFound if the directory isn't
// found.
func (f *Fs) List(ctx context.Context, dir string) (fs.DirEntries, error) {
	return list.WithListP(ctx, dir, f)
}

// ListP lists one directory and invokes callback once per API page.
func (f *Fs) ListP(ctx context.Context, dir string, callback fs.ListRCallback) error {
	opts := &api.MetadataRequestOptions{Limit: f.opt.PageSize}
	for {
		info, err := f.readMetaDataForPath(ctx, dir, opts)
		if err != nil {
			if errors.Is(err, fs.ErrorObjectNotFound) {
				return fs.ErrorDirNotFound
			}
			return err
		}
		entries := make(fs.DirEntries, 0, len(info.Files))
		for _, item := range info.Files {
			remote := path.Join(dir, f.opt.Enc.ToStandardName(item.Name))
			switch item.Kind {
			case "folder":
				f.dirCache.Put(remote, item.Id)
				entries = append(entries, fs.NewDir(remote, item.ModTime).
					SetID(item.Id).
					SetParentID(parentIDOrRoot(item.ParentId)).
					SetSize(item.Size))
			case "file":
				o, err := f.newObjectWithInfo(ctx, remote, &item)
				if err == nil {
					entries = append(entries, o)
				}
			}
		}
		if len(entries) > 0 {
			if err := callback(entries); err != nil {
				return err
			}
		}
		if info.NextCursor == "" {
			return nil
		}
		opts.Cursor = info.NextCursor
	}
}

// Return an Object from a path
//
// If it can't be found it returns the error fs.ErrorObjectNotFound.
func (f *Fs) newObjectWithInfo(_ context.Context, remote string, info *api.FileInfo) (fs.Object, error) {
	if info == nil {
		return nil, fs.ErrorObjectNotFound
	}
	o := &Object{
		fs:       f,
		remote:   remote,
		id:       info.Id,
		size:     info.Size,
		parentId: parentIDOrRoot(info.ParentId),
		name:     info.Name,
		modTime:  info.ModTime,
		mimeType: info.MimeType,
		hash:     fileHashValue(info.Hash),
	}
	if info.Kind == "folder" {
		return o, fs.ErrorIsDir
	}
	return o, nil
}

// NewObject finds the Object at remote.  If it can't be found it
// returns the error fs.ErrorObjectNotFound.
func (f *Fs) NewObject(ctx context.Context, remote string) (fs.Object, error) {
	leaf, directoryID, err := f.dirCache.FindPath(ctx, remote, false)
	if err != nil {
		if err == fs.ErrorDirNotFound {
			return nil, fs.ErrorObjectNotFound
		}
	}

	res, err := f.findObject(ctx, directoryID, leaf)
	if err != nil {
		return nil, err
	}
	if len(res) == 0 {
		return nil, fs.ErrorObjectNotFound
	}
	if res[0].Kind == "folder" {
		return nil, fs.ErrorIsDir
	}

	return f.newObjectWithInfo(ctx, remote, &res[0])
}

func (f *Fs) findObject(ctx context.Context, pathID, leaf string) ([]api.FileInfo, error) {
	encodedLeaf := f.opt.Enc.FromStandardName(leaf)
	cursor := ""
	for {
		opts := rest.Opts{
			Method:     "GET",
			Path:       "/api/v1/files",
			Parameters: url.Values{"limit": []string{"200"}},
		}
		setParentParameter(opts.Parameters, pathID)
		if cursor != "" {
			opts.Parameters.Set("cursor", cursor)
		}
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
		for _, file := range page.Files {
			if file.Name == encodedLeaf {
				return []api.FileInfo{file}, nil
			}
		}
		if page.NextCursor == "" {
			return nil, nil
		}
		cursor = page.NextCursor
	}
}

func (f *Fs) moveTo(ctx context.Context, id, srcLeaf, dstLeaf, srcDirectoryID, dstDirectoryID string) error {
	if srcDirectoryID == dstDirectoryID {
		if srcLeaf == dstLeaf {
			return nil
		}
		if err := f.updateFileInformation(ctx, &api.UpdateFileInformation{Name: f.opt.Enc.FromStandardName(dstLeaf)}, id); err != nil {
			return fmt.Errorf("move: failed rename: %w", err)
		}
		return nil
	}
	if srcLeaf == dstLeaf {
		return f.moveFileToParent(ctx, id, dstDirectoryID)
	}

	// Moving and renaming are separate v2 operations. If the source name is
	// already occupied in the destination, rename before moving; otherwise move
	// first. Roll back the first operation if the second fails.
	collisions, err := f.findObject(ctx, dstDirectoryID, srcLeaf)
	if err != nil {
		return err
	}
	encodedSrc := f.opt.Enc.FromStandardName(srcLeaf)
	encodedDst := f.opt.Enc.FromStandardName(dstLeaf)
	if len(collisions) > 0 {
		if err := f.updateFileInformation(ctx, &api.UpdateFileInformation{Name: encodedDst}, id); err != nil {
			return fmt.Errorf("move: failed pre-rename: %w", err)
		}
		if err := f.moveFileToParent(ctx, id, dstDirectoryID); err != nil {
			rollbackErr := f.updateFileInformation(context.Background(), &api.UpdateFileInformation{Name: encodedSrc}, id)
			return errors.Join(fmt.Errorf("couldn't move file: %w", err), rollbackErr)
		}
		return nil
	}
	if err := f.moveFileToParent(ctx, id, dstDirectoryID); err != nil {
		return fmt.Errorf("couldn't move file: %w", err)
	}
	if err := f.updateFileInformation(ctx, &api.UpdateFileInformation{Name: encodedDst}, id); err != nil {
		rollbackErr := f.moveFileToParent(context.Background(), id, srcDirectoryID)
		return errors.Join(fmt.Errorf("move: failed rename: %w", err), rollbackErr)
	}
	return nil
}

func (f *Fs) moveFileToParent(ctx context.Context, id, destinationID string) error {
	opts := rest.Opts{
		Method:       "POST",
		Path:         "/api/v1/files/" + id + "/move",
		ExtraHeaders: map[string]string{"Idempotency-Key": uuid.NewString()},
	}
	move := api.MoveFileRequest{ParentId: requestParentID(destinationID), ConflictPolicy: "fail"}
	return f.pacer.Call(func() (bool, error) {
		resp, err := f.srv.CallJSON(ctx, &opts, &move, nil)
		return shouldRetry(ctx, resp, err)
	})
}

// updateFileInformation set's various file attributes most importantly it's name
func (f *Fs) updateFileInformation(ctx context.Context, update *api.UpdateFileInformation, fileID string) error {
	opts := rest.Opts{Method: "PATCH", Path: "/api/v1/files/" + fileID, NoResponse: true}
	if err := f.pacer.Call(func() (bool, error) {
		resp, err := f.srv.CallJSON(ctx, &opts, update, nil)
		return shouldRetry(ctx, resp, err)
	}); err != nil {
		return fmt.Errorf("couldn't update file info: %w", err)
	}
	return nil
}

func (f *Fs) putUnchecked(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) error {
	if src.Size() < 0 {
		_, err := multipart.UploadMultipart(ctx, src, in, multipart.UploadMultipartOptions{
			Open: f, OpenOptions: options,
		})
		return err
	}
	o := &Object{fs: f}
	uploadInfo, err := o.uploadMultipart(ctx, src.Remote(), in, src)
	if err != nil {
		return err
	}
	return o.createFile(ctx, src, uploadInfo)
}

// FindLeaf finds a directory of name leaf in the folder with ID pathID
func (f *Fs) FindLeaf(ctx context.Context, pathID, leaf string) (pathIDOut string, found bool, err error) {
	files, err := f.findObject(ctx, pathID, leaf)
	if err != nil {
		return "", false, err
	}
	if len(files) == 0 {
		return "", false, nil
	}
	if files[0].Kind == "file" {
		return "", false, fs.ErrorIsFile
	}
	return files[0].Id, true, nil
}

// Put in to the remote path with the modTime given of the given size
//
// When called from outside an Fs by rclone, src.Size() will always be >= 0.
// But for unknown-sized objects (indicated by src.Size() == -1), Put should either
// return an error or upload it properly (rather than e.g. calling panic).
//
// May create the object even if it returns an error - if so
// will return the object and the error, otherwise will return
// nil and the error
func (f *Fs) Put(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	existingObj, err := f.NewObject(ctx, src.Remote())
	switch err {
	case nil:
		return existingObj, existingObj.Update(ctx, in, src, options...)
	case fs.ErrorObjectNotFound:
		// Not found so create it
		return f.PutUnchecked(ctx, in, src, options...)
	default:
		return nil, err
	}
}

// PutUnchecked uploads the object
//
// This will create a duplicate if we upload a new file without
// checking to see if there is one already - use Put() for that.
func (f *Fs) PutUnchecked(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	err := f.putUnchecked(ctx, in, src, options...)
	if err != nil {
		return nil, err
	}
	return f.NewObject(ctx, src.Remote())
}

// Update the already existing object
//
// Copy the reader into the object updating modTime and size.
//
// The new object may have been created if an error is returned
func (o *Object) Update(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) error {
	if src.Size() < 0 {
		_, err := multipart.UploadMultipart(ctx, src, in, multipart.UploadMultipartOptions{
			Open: o.fs, OpenOptions: options,
		})
		if err != nil {
			return err
		}
		updated, err := o.fs.NewObject(ctx, o.Remote())
		if err != nil {
			return err
		}
		*o = *(updated.(*Object))
		return nil
	}
	info, err := o.uploadMultipart(ctx, o.Remote(), in, src)
	if err != nil {
		return err
	}
	file, err := o.completeUpload(ctx, info)
	if err != nil {
		return err
	}
	o.applyFileInfo(file)
	return nil
}

// ChangeNotify calls the passed function with a path that has had changes.
func (f *Fs) ChangeNotify(ctx context.Context, notifyFunc func(string, fs.EntryType), pollIntervalChan <-chan time.Duration) {
	go f.changeNotifyLoop(ctx, notifyFunc, pollIntervalChan, f.changeNotifySSE)
}

type changeNotifySSEFunc func(context.Context, func(string, fs.EntryType)) error

func (f *Fs) changeNotifyLoop(ctx context.Context, notifyFunc func(string, fs.EntryType), pollIntervalChan <-chan time.Duration, runSSE changeNotifySSEFunc) {
	var (
		enabled   bool
		sseCancel context.CancelFunc
		sseDone   <-chan error
	)

	startSSE := func() {
		if sseDone != nil {
			return
		}
		sseCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		sseCancel = cancel
		sseDone = done
		fs.Debugf(f, "Starting SSE event stream")
		go func() {
			done <- runSSE(sseCtx, notifyFunc)
		}()
	}
	stopSSE := func() {
		if sseCancel != nil {
			sseCancel()
		}
	}
	defer stopSSE()

	for {
		select {
		case <-ctx.Done():
			return
		case pollInterval, ok := <-pollIntervalChan:
			if !ok {
				fs.Debugf(f, "ChangeNotify: channel closed, stopping")
				return
			}
			enabled = pollInterval > 0
			if enabled {
				startSSE()
			} else {
				stopSSE()
			}
		case err := <-sseDone:
			sseDone = nil
			sseCancel = nil
			if ctx.Err() != nil {
				return
			}
			if !enabled {
				continue
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				fs.Infof(f, "SSE connection failed permanently: %s", err)
				return
			}
			startSSE()
		}
	}
}

func isFatalError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "404")
}

func (f *Fs) changeNotifySSE(ctx context.Context, notifyFunc func(string, fs.EntryType)) error {
	lastEventID := ""
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var connErr error
		err := f.ssePacer.Call(func() (bool, error) {
			connErr = f.connectAndProcessSSE(ctx, notifyFunc, &lastEventID)
			if connErr == nil {
				return false, nil
			}
			if fserrors.ContextError(ctx, &connErr) {
				return false, connErr
			}
			if isFatalError(connErr) {
				return false, connErr
			}
			return true, connErr
		})

		if err != nil {
			return err
		}

		fs.Debugf(f, "SSE connection ended, will retry")
	}
}

func (f *Fs) connectAndProcessSSE(ctx context.Context, notifyFunc func(string, fs.EntryType), lastEventID *string) error {
	headers := map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	}
	if lastEventID != nil && *lastEventID != "" {
		headers["Last-Event-ID"] = *lastEventID
	}
	opts := rest.Opts{
		Method:       "GET",
		Path:         "/api/v1/events",
		ContentType:  "text/event-stream",
		ExtraHeaders: headers,
		Parameters: url.Values{
			"types": []string{"file.created,file.updated,file.trashed,file.restored,file.purged"},
		},
	}
	resp, err := f.srv.Call(ctx, &opts)
	if err != nil {
		return fmt.Errorf("failed to connect to SSE endpoint: %w", err)
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("no response from SSE endpoint")
	}
	defer resp.Body.Close()
	if contentType := resp.Header.Get("Content-Type"); !strings.Contains(contentType, "text/event-stream") {
		return fmt.Errorf("unexpected content type: %s", contentType)
	}
	fs.Debugf(f, "SSE connection established")
	reader := bufio.NewReader(resp.Body)
	var eventID string
	var eventName string
	var eventData strings.Builder
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return fmt.Errorf("SSE stream closed by server")
			}
			return fmt.Errorf("error reading SSE stream: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if eventData.Len() > 0 {
				data := eventData.String()
				eventData.Reset()
				if err := f.processSSEEvent(eventName, data, notifyFunc); err != nil {
					fs.Debugf(f, "Failed to process SSE event: %s", err)
				} else if eventID != "" && lastEventID != nil {
					*lastEventID = eventID
				}
			}
			eventID = ""
			eventName = ""
			continue
		}
		if after, ok := strings.CutPrefix(line, "id:"); ok {
			eventID = strings.TrimSpace(after)
			continue
		}
		if after, ok := strings.CutPrefix(line, "event:"); ok {
			eventName = strings.TrimSpace(after)
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if eventData.Len() > 0 {
				eventData.WriteByte('\n')
			}
			eventData.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func (f *Fs) processSSEEvent(eventName, data string, notifyFunc func(string, fs.EntryType)) error {
	if eventName == "sync.required" {
		f.dirCache.ResetRoot()
		notifyFunc("", fs.EntryDirectory)
		return nil
	}
	if eventName == "" || eventName == "stream.error" {
		return nil
	}
	var event api.EventEnvelope
	if err := json.Unmarshal([]byte(data), &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}
	if event.ResourceType != "file" || event.Payload.Name == "" {
		return nil
	}
	parentID := parentIDOrRoot(event.Payload.ParentId)
	parentPath, ok := f.dirCache.GetInv(parentID)
	if !ok {
		return nil
	}
	name := f.opt.Enc.ToStandardName(event.Payload.Name)
	fullPath := path.Join(parentPath, name)
	entryType := fs.EntryObject
	if event.Payload.Kind == "folder" {
		entryType = fs.EntryDirectory
	}
	fs.Debugf(f, "SSE event: %s (%v, type=%s)", fullPath, entryType, eventName)
	notifyFunc(fullPath, entryType)
	return nil
}

// PutStream uploads to the remote path with the modTime given of indeterminate size
func (f *Fs) PutStream(ctx context.Context, in io.Reader, src fs.ObjectInfo, options ...fs.OpenOption) (fs.Object, error) {
	return f.Put(ctx, in, src, options...)
}

// OpenChunkWriter returns the chunk size and a ChunkWriter
//
// Pass in the remote and the src object
// You can also use options to hint at the desired chunk size
func (f *Fs) OpenChunkWriter(
	ctx context.Context,
	remote string,
	src fs.ObjectInfo,
	options ...fs.OpenOption) (info fs.ChunkWriterInfo, writer fs.ChunkWriter, err error) {
	o := &Object{fs: f, remote: remote}
	uploadInfo, err := o.prepareUpload(ctx, remote, src)
	if err != nil {
		return info, nil, fmt.Errorf("prepare upload: %w", err)
	}
	return fs.ChunkWriterInfo{
		ChunkSize:         uploadInfo.chunkSize,
		Concurrency:       f.opt.UploadConcurrency,
		LeavePartsOnError: src.Size() >= 0,
	}, &objectChunkWriter{o: o, uploadInfo: uploadInfo}, nil
}

// CreateDir makes a directory with pathID as parent and name leaf
func (f *Fs) CreateDir(ctx context.Context, pathID, leaf string) (string, error) {
	opts := rest.Opts{
		Method:       "POST",
		Path:         "/api/v1/folders",
		ExtraHeaders: map[string]string{"Idempotency-Key": uuid.NewString()},
	}
	mkdir := api.FolderCreateRequest{
		ParentId: requestParentID(pathID), Name: f.opt.Enc.FromStandardName(leaf), ConflictPolicy: "fail",
	}
	var info api.FileInfo
	err := f.pacer.Call(func() (bool, error) {
		resp, err := f.srv.CallJSON(ctx, &opts, &mkdir, &info)
		return shouldRetry(ctx, resp, err)
	})
	if err != nil {
		return "", err
	}
	return info.Id, nil
}

// Mkdir makes the directory (container, bucket)
//
// Shouldn't return an error if it already exists
func (f *Fs) Mkdir(ctx context.Context, dir string) (err error) {
	_, err = f.dirCache.FindDir(ctx, dir, true)
	return err
}

func (f *Fs) purgeCheck(ctx context.Context, dir string, check bool) error {
	root := path.Join(f.root, dir)
	if root == "" {
		return errors.New("can't purge root directory")
	}
	directoryID, err := f.dirCache.FindDir(ctx, dir, false)
	if err != nil {
		return err
	}
	if check {
		info, err := f.readMetaDataForPath(ctx, dir, &api.MetadataRequestOptions{Limit: 1})
		if err != nil {
			return err
		}
		if len(info.Files) > 0 {
			return fs.ErrorDirectoryNotEmpty
		}
	}
	opts := rest.Opts{Method: "DELETE", Path: "/api/v1/files/" + directoryID, NoResponse: true}
	if err := f.pacer.Call(func() (bool, error) {
		resp, err := f.srv.Call(ctx, &opts)
		return shouldRetry(ctx, resp, err)
	}); err != nil {
		return err
	}
	f.dirCache.FlushDir(dir)
	return nil
}

// Rmdir removes the directory (container, bucket) if empty
//
// Return an error if it doesn't exist or isn't empty
func (f *Fs) Rmdir(ctx context.Context, dir string) (err error) {
	return f.purgeCheck(ctx, dir, true)
}

// Purge all files in the directory specified
//
// Implement this if you have a way of deleting all the files
// quicker than just running Remove() on the result of List()
//
// Return an error if it doesn't exist
func (f *Fs) Purge(ctx context.Context, dir string) error {
	return f.purgeCheck(ctx, dir, false)
}

// Move src to this remote using server-side move operations.
//
// This is stored with the remote path given.
//
// It returns the destination Object and a possible error.
//
// Will only be called if src.Fs().Name() == f.Name()
//
// If it isn't possible then return fs.ErrorCantMove
func (f *Fs) Move(ctx context.Context, src fs.Object, remote string) (fs.Object, error) {
	srcObj, ok := src.(*Object)
	if !ok {
		fs.Debugf(src, "Can't move - not same remote type")
		return nil, fs.ErrorCantMove
	}

	srcLeaf, srcParentID, err := srcObj.fs.dirCache.FindPath(ctx, src.Remote(), false)
	if err != nil {
		return nil, err
	}

	dstLeaf, directoryID, err := f.dirCache.FindPath(ctx, remote, true)
	if err != nil {
		return nil, err
	}

	err = f.moveTo(ctx, srcObj.id, srcLeaf, dstLeaf, srcParentID, directoryID)
	if err != nil {
		return nil, err
	}
	f.dirCache.FlushDir(src.Remote())
	newObj := *srcObj
	newObj.remote = remote
	newObj.fs = f
	return &newObj, nil
}

// DirMove moves src, srcRemote to this remote at dstRemote
// using server-side move operations.
//
// Will only be called if src.Fs().Name() == f.Name()
//
// If it isn't possible then return fs.ErrorCantDirMove

// If destination exists then return fs.ErrorDirExists
func (f *Fs) DirMove(ctx context.Context, src fs.Fs, srcRemote, dstRemote string) error {
	srcFs, ok := src.(*Fs)
	if !ok {
		fs.Debugf(srcFs, "Can't move directory - not same remote type")
		return fs.ErrorCantDirMove
	}
	srcID, srcDirectoryID, srcLeaf, dstDirectoryID, dstLeaf, err := f.dirCache.DirMove(ctx, srcFs.dirCache, srcFs.root, srcRemote, f.root, dstRemote)
	if err != nil {
		return err
	}
	err = f.moveTo(ctx, srcID, srcLeaf, dstLeaf, srcDirectoryID, dstDirectoryID)
	if err != nil {
		return fmt.Errorf("dirmove: failed to move: %w", err)
	}
	srcFs.dirCache.FlushDir(srcRemote)
	return nil
}

func (o *Object) Remove(ctx context.Context) error {
	opts := rest.Opts{Method: "DELETE", Path: "/api/v1/files/" + o.id, NoResponse: true}
	return o.fs.pacer.Call(func() (bool, error) {
		resp, err := o.fs.srv.Call(ctx, &opts)
		return shouldRetry(ctx, resp, err)
	})
}

// PublicLink adds a "readable by anyone with link" permission on the given file or folder.
func (f *Fs) PublicLink(ctx context.Context, remote string, expire fs.Duration, unlink bool) (string, error) {
	id, err := f.dirCache.FindDir(ctx, remote, false)
	if err != nil {
		o, objectErr := f.NewObject(ctx, remote)
		if objectErr != nil {
			return "", objectErr
		}
		id = o.(fs.IDer).ID()
	}
	if unlink {
		shares, err := f.getFileShares(ctx, id)
		if err != nil {
			if errors.Is(err, fs.ErrorObjectNotFound) {
				return "", nil
			}
			return "", err
		}
		for _, share := range shares {
			if share.RevokedAt != nil {
				continue
			}
			opts := rest.Opts{Method: "DELETE", Path: "/api/v1/shares/" + share.ID, NoResponse: true}
			if err := f.pacer.Call(func() (bool, error) {
				resp, callErr := f.srv.Call(ctx, &opts)
				return shouldRetry(ctx, resp, callErr)
			}); err != nil {
				return "", err
			}
		}
		return "", nil
	}
	payload := api.FileShareCreate{}
	if f.opt.LinkPassword != "" {
		payload.Password = f.opt.LinkPassword
	}
	if expire < fs.DurationOff {
		expiresAt := time.Now().Add(time.Duration(expire)).UTC()
		payload.ExpiresAt = &expiresAt
	}
	opts := rest.Opts{
		Method:       "POST",
		Path:         "/api/v1/files/" + id + "/shares",
		ExtraHeaders: map[string]string{"Idempotency-Key": uuid.NewString()},
	}
	var created api.FileShareCreated
	if err := f.pacer.Call(func() (bool, error) {
		resp, callErr := f.srv.CallJSON(ctx, &opts, &payload, &created)
		return shouldRetry(ctx, resp, callErr)
	}); err != nil {
		return "", err
	}
	if created.PublicURL == "" {
		return "", fmt.Errorf("TelDrive returned an empty public URL")
	}
	return created.PublicURL, nil
}

// Open an object for read
func (o *Object) Open(ctx context.Context, options ...fs.OpenOption) (in io.ReadCloser, err error) {
	var resp *http.Response

	fs.FixRangeOption(options, o.size)

	opts := rest.Opts{
		Method:  "GET",
		Path:    fmt.Sprintf("/api/v1/files/%s/content", o.id),
		Options: options,
	}

	err = o.fs.pacer.Call(func() (bool, error) {
		resp, err = o.fs.srv.Call(ctx, &opts)
		return shouldRetry(ctx, resp, err)
	})

	if err != nil {
		return nil, err
	}
	return resp.Body, err
}

// Copy src to this remote using server-side copy operations.
//
// This is stored with the remote path given.
//
// It returns the destination Object and a possible error.
//
// Will only be called if src.Fs().Name() == f.Name()
//
// If it isn't possible then return fs.ErrorCantCopy
func (f *Fs) Copy(ctx context.Context, src fs.Object, remote string) (fs.Object, error) {
	srcObj, ok := src.(*Object)
	if !ok {
		return nil, fs.ErrorCantCopy
	}
	srcLeaf, srcParentID, err := srcObj.fs.dirCache.FindPath(ctx, src.Remote(), false)
	if err != nil {
		return nil, err
	}
	dstLeaf, directoryID, err := f.dirCache.FindPath(ctx, remote, true)
	if err != nil {
		return nil, err
	}
	if srcParentID == directoryID && dstLeaf == srcLeaf {
		return nil, fs.ErrorCantCopy
	}
	opts := rest.Opts{
		Method:       "POST",
		Path:         "/api/v1/files/" + srcObj.id + "/copy",
		ExtraHeaders: map[string]string{"Idempotency-Key": uuid.NewString()},
	}
	payload := api.FileCopy{
		ParentId: requestParentID(directoryID), Name: f.opt.Enc.FromStandardName(dstLeaf), ConflictPolicy: "fail",
	}
	var info api.FileInfo
	err = f.pacer.Call(func() (bool, error) {
		resp, err := f.srv.CallJSON(ctx, &opts, &payload, &info)
		return shouldRetry(ctx, resp, err)
	})
	if err != nil {
		return nil, err
	}
	return f.newObjectWithInfo(ctx, remote, &info)
}

// About gets quota information
func (f *Fs) About(ctx context.Context) (usage *fs.Usage, err error) {
	opts := rest.Opts{
		Method: "GET",
		Path:   "/api/v1/files/statistics/categories",
	}
	var stats []api.CategorySize
	err = f.pacer.Call(func() (bool, error) {
		resp, err := f.srv.CallJSON(ctx, &opts, nil, &stats)
		return shouldRetry(ctx, resp, err)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to read user info: %w", err)
	}

	total := int64(0)
	for category := range stats {
		total += stats[category].TotalSize
	}
	return &fs.Usage{Used: fs.NewUsageValue(total)}, nil
}

// Fs returns the parent Fs
func (o *Object) Fs() fs.Info {
	return o.fs
}

// Return a string version
func (o *Object) String() string {
	if o == nil {
		return "<nil>"
	}
	return o.remote
}

// Remote returns the remote path
func (o *Object) Remote() string {
	return o.remote
}

// ModTime returns the modification time of the object
//
// It attempts to read the objects mtime and if that isn't present the
// LastModified returned in the http headers
func (o *Object) ModTime(ctx context.Context) time.Time {
	return o.modTime
}

func (o *Object) MimeType(ctx context.Context) string {
	return o.mimeType
}

// Size returns the size of an object in bytes
func (o *Object) Size() int64 {
	return o.size
}

func (o *Object) Hash(ctx context.Context, t hash.Type) (string, error) {
	if t != telDriveHash {
		return "", hash.ErrUnsupported
	}

	if o.hash != "" {
		return o.hash, nil
	}

	// Fetch from server if not cached
	var file api.FileInfo
	opts := rest.Opts{
		Method: "GET",
		Path:   "/api/v1/files/" + o.id,
	}

	err := o.fs.pacer.Call(func() (bool, error) {
		resp, err := o.fs.srv.CallJSON(ctx, &opts, nil, &file)
		return shouldRetry(ctx, resp, err)
	})

	if err != nil {
		return "", fmt.Errorf("failed to get file hash: %w", err)
	}

	if file.Hash != nil && file.Hash.Value != "" {
		o.hash = file.Hash.Value
		return o.hash, nil
	}

	return "", hash.ErrUnsupported
}

// ID returns the ID of the Object if known, or "" if not
func (o *Object) ID() string {
	return o.id
}

// ParentID implements fs.ParentIDer.
func (o *Object) ParentID() string {
	return o.parentId
}

// Storable returns whether this object is storable
func (o *Object) Storable() bool {
	return true
}

// SetModTime sets the modification time of the local fs object
func (o *Object) SetModTime(ctx context.Context, modTime time.Time) error {
	modTime = modTime.UTC().Truncate(o.fs.Precision())
	updateInfo := &api.UpdateFileInformation{
		ModTime: Ptr(modTime),
	}
	err := o.fs.updateFileInformation(ctx, updateInfo, o.id)
	if err != nil {
		return fmt.Errorf("couldn't update mod time: %w", err)
	}
	o.modTime = modTime
	return nil
}

// DirCacheFlush an optional interface to flush internal directory cache
// DirCacheFlush resets the directory cache - used in testing
// as an optional interface
func (f *Fs) DirCacheFlush() {
	f.dirCache.ResetRoot()
}

func (o *Object) applyFileInfo(info *api.FileInfo) {
	if o == nil || info == nil {
		return
	}
	o.id = info.Id
	o.size = info.Size
	o.parentId = parentIDOrRoot(info.ParentId)
	o.name = info.Name
	o.modTime = info.ModTime
	o.mimeType = info.MimeType
	o.hash = fileHashValue(info.Hash)
}

func setParentParameter(values url.Values, parentID string) {
	if parentID != "" && parentID != rootID {
		values.Set("parentId", parentID)
	}
}

func parentIDOrRoot(parentID string) string {
	if parentID == "" {
		return rootID
	}
	return parentID
}

func requestParentID(parentID string) string {
	if parentID == rootID {
		return ""
	}
	return parentID
}

func fileHashValue(value *api.FileHash) string {
	if value == nil || value.Algorithm != "blake3" {
		return ""
	}
	return value.Value
}

// Check the interfaces are satisfied
var (
	_ fs.Fs              = (*Fs)(nil)
	_ fs.Copier          = (*Fs)(nil)
	_ fs.Mover           = (*Fs)(nil)
	_ fs.DirMover        = (*Fs)(nil)
	_ fs.Object          = (*Object)(nil)
	_ fs.MimeTyper       = &Object{}
	_ fs.OpenChunkWriter = (*Fs)(nil)
	_ fs.IDer            = (*Object)(nil)
	_ fs.DirCacheFlusher = (*Fs)(nil)
	_ fs.PublicLinker    = (*Fs)(nil)
	_ fs.ParentIDer      = (*Object)(nil)
	_ fs.Abouter         = (*Fs)(nil)
)

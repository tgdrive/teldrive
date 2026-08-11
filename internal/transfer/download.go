package transfer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/google/uuid"
	varccache "github.com/tgdrive/varc/cache"
	varcsource "github.com/tgdrive/varc/source"

	"github.com/tgdrive/teldrive/v2/internal/contentcrypto"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
)

var (
	ErrInvalidDownload       = errors.New("invalid download request")
	ErrRangeNotSatisfiable   = errors.New("download range is not satisfiable")
	ErrCorruptPartLayout     = errors.New("file part layout is inconsistent")
	ErrDownloadNotConfigured = errors.New("download pipeline is not configured")
)

// FileCatalog is the finalized catalog boundary required by Downloader.
type FileCatalog interface {
	Get(context.Context, int64, uuid.UUID) (*sqlcgen.File, error)
	Parts(context.Context, int64, uuid.UUID) ([]*sqlcgen.FilePart, error)
}

type PartSizeBackfiller interface {
	UpdatePartSizes(context.Context, uuid.UUID, int32, int64, int64) error
}

type Downloader struct {
	catalog FileCatalog
	storage telegramstore.Storage
	keys    KeyProvider
	cache   *varccache.Cache
}

func NewDownloader(catalog FileCatalog, storage telegramstore.Storage, keys KeyProvider, caches ...*varccache.Cache) *Downloader {
	var streamCache *varccache.Cache
	if len(caches) > 0 {
		streamCache = caches[0]
	}
	return &Downloader{catalog: catalog, storage: storage, keys: keys, cache: streamCache}
}

type DownloadRequest struct {
	UserID int64
	FileID uuid.UUID
	Offset int64
	Length int64 // -1 means through end of file.
}

type Download struct {
	Reader      DownloadReader
	File        *sqlcgen.File
	Offset      int64
	Length      int64
	TotalSize   int64
	ContentType string
	ETag        string
}

type DownloadReader interface {
	io.Reader
	io.ReaderAt
	io.Seeker
	io.Closer
}

func (d *Downloader) Open(ctx context.Context, request DownloadRequest) (*Download, error) {
	if d.catalog == nil || d.storage == nil {
		return nil, ErrDownloadNotConfigured
	}
	if request.UserID <= 0 || request.FileID == uuid.Nil || request.Offset < 0 || request.Length < -1 {
		return nil, ErrInvalidDownload
	}
	if d.cache != nil {
		return d.openCached(ctx, request)
	}
	return d.openOrigin(ctx, request)
}

func (d *Downloader) openOrigin(ctx context.Context, request DownloadRequest) (*Download, error) {
	file, err := d.catalog.Get(ctx, request.UserID, request.FileID)
	if err != nil {
		return nil, err
	}
	if file.Kind != sqlcgen.FileKindFile || file.Status != sqlcgen.FileStatusActive || !file.Size.Valid || file.Size.Int64 < 0 {
		return nil, ErrInvalidDownload
	}
	parts, err := d.catalog.Parts(ctx, request.UserID, request.FileID)
	if err != nil {
		return nil, err
	}
	session, err := d.openDownloadSession(ctx, request.UserID)
	if err != nil {
		return nil, err
	}
	keepSession := false
	defer func() {
		if !keepSession {
			_ = session.Close()
		}
	}()
	if err := d.resolveMissingPartSizes(ctx, session, request.UserID, request.FileID, file, parts); err != nil {
		return nil, err
	}
	segments, length, err := planSegments(parts, file.Size.Int64, request.Offset, request.Length)
	if err != nil {
		return nil, err
	}

	contentType := fileContentType(file)
	etag := fileETag(file)
	if length == 0 {
		return &Download{
			Reader: nopDownloadReader{bytes.NewReader(nil)}, File: file, Offset: request.Offset,
			Length: 0, TotalSize: file.Size.Int64, ContentType: contentType, ETag: etag,
		}, nil
	}

	var encryptionKey string
	if file.Encryption {
		if d.keys == nil || !file.EncryptionKeyVersion.Valid {
			return nil, ErrEncryptionKey
		}
		encryptionKey, err = d.keys.Key(ctx, request.UserID, file.EncryptionKeyVersion.Int32)
		if err != nil || encryptionKey == "" {
			return nil, errors.Join(ErrEncryptionKey, err)
		}
	}

	keepSession = true
	return &Download{
		Reader: &downloadReader{
			ctx:     ctx,
			session: session,
			userID:  request.UserID,
			file:    file,
			parts:   segments,
			length:  length,
			key:     encryptionKey,
		}, File: file,
		Offset: request.Offset, Length: length, TotalSize: file.Size.Int64,
		ContentType: contentType, ETag: etag,
	}, nil
}

func (d *Downloader) openCached(ctx context.Context, request DownloadRequest) (*Download, error) {
	file, err := d.catalog.Get(ctx, request.UserID, request.FileID)
	if err != nil {
		return nil, err
	}
	if file.Kind != sqlcgen.FileKindFile || file.Status != sqlcgen.FileStatusActive || !file.Size.Valid || file.Size.Int64 < 0 {
		return nil, ErrInvalidDownload
	}
	length, err := normalizeDownloadRange(file.Size.Int64, request.Offset, request.Length)
	if err != nil {
		return nil, err
	}
	contentType := fileContentType(file)
	etag := fileETag(file)
	if length == 0 {
		return &Download{Reader: nopDownloadReader{bytes.NewReader(nil)}, File: file, Offset: request.Offset, Length: 0, TotalSize: file.Size.Int64, ContentType: contentType, ETag: etag}, nil
	}

	object := &downloadCacheObject{
		metadata: varcsource.Metadata{Size: file.Size.Int64, ETag: strconv.FormatInt(file.Generation, 10), LastModified: file.UpdatedAt.Time, ContentType: contentType},
		openRange: func(rangeCtx context.Context, start, end int64) (io.ReadCloser, error) {
			download, err := d.openOrigin(rangeCtx, DownloadRequest{UserID: request.UserID, FileID: request.FileID, Offset: start, Length: end - start})
			if err != nil {
				return nil, err
			}
			return download.Reader, nil
		},
	}
	reader, err := d.cache.Open(ctx, fmt.Sprintf("%d/%s", request.UserID, request.FileID), object)
	if err != nil {
		return nil, fmt.Errorf("open stream cache: %w", err)
	}
	section := io.NewSectionReader(reader, request.Offset, length)
	return &Download{
		Reader: &cachedDownloadReader{SectionReader: section, closer: reader}, File: file,
		Offset: request.Offset, Length: length, TotalSize: file.Size.Int64, ContentType: contentType, ETag: etag,
	}, nil
}

type downloadCacheObject struct {
	metadata  varcsource.Metadata
	openRange func(context.Context, int64, int64) (io.ReadCloser, error)
}

func (o *downloadCacheObject) Metadata() varcsource.Metadata { return o.metadata }
func (o *downloadCacheObject) OpenRange(ctx context.Context, start, end int64) (io.ReadCloser, error) {
	return o.openRange(ctx, start, end)
}

type cachedDownloadReader struct {
	*io.SectionReader
	closer io.Closer
}

func (r *cachedDownloadReader) Close() error { return r.closer.Close() }

func (d *Downloader) resolveMissingPartSizes(ctx context.Context, session telegramstore.DownloadSession, userID int64, fileID uuid.UUID, file *sqlcgen.File, parts []*sqlcgen.FilePart) error {
	backfiller, _ := d.catalog.(PartSizeBackfiller)
	for _, part := range parts {
		if part == nil || (part.PlainSize.Valid && part.StoredSize.Valid) {
			continue
		}
		stored, err := session.Metadata(ctx, telegramstore.MetadataRequest{
			UserID: userID, ChannelID: part.ChannelID, MessageID: part.MessageID,
		})
		if err != nil {
			return fmt.Errorf("resolve Telegram part %d metadata: %w", part.PartNo, err)
		}
		plainSize := stored.Size
		if file.Encryption {
			plainSize, err = contentcrypto.DecryptedSize(stored.Size)
			if err != nil {
				return fmt.Errorf("derive plaintext size for part %d: %w", part.PartNo, err)
			}
		}
		part.StoredSize.Int64, part.StoredSize.Valid = stored.Size, true
		part.PlainSize.Int64, part.PlainSize.Valid = plainSize, true
		if backfiller != nil {
			if err := backfiller.UpdatePartSizes(ctx, fileID, part.PartNo, plainSize, stored.Size); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *Downloader) openDownloadSession(ctx context.Context, userID int64) (telegramstore.DownloadSession, error) {
	if opener, ok := d.storage.(telegramstore.DownloadSessionOpener); ok {
		return opener.OpenDownloadSession(ctx, userID)
	}
	return storageDownloadSession{storage: d.storage}, nil
}

type storageDownloadSession struct{ storage telegramstore.Storage }

func (s storageDownloadSession) Metadata(ctx context.Context, request telegramstore.MetadataRequest) (telegramstore.StoredPart, error) {
	metadata, ok := s.storage.(telegramstore.MetadataReader)
	if !ok {
		return telegramstore.StoredPart{}, ErrDownloadNotConfigured
	}
	return metadata.Metadata(ctx, request)
}

func (s storageDownloadSession) OpenRange(ctx context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	return s.storage.OpenRange(ctx, request)
}

func (storageDownloadSession) Close() error { return nil }

type downloadSegment struct {
	part   *sqlcgen.FilePart
	offset int64
	length int64
}

type downloadReader struct {
	ctx     context.Context
	session telegramstore.DownloadSession
	userID  int64
	file    *sqlcgen.File
	parts   []downloadSegment
	length  int64
	key     string

	mu     sync.Mutex
	pos    int64
	closed bool
	reader io.ReadCloser
}

func (r *downloadReader) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	read := 0
	for read < len(p) && r.pos < r.length {
		if r.reader == nil {
			segment, segmentStart, err := r.segmentAt(r.pos)
			if err != nil {
				return read, err
			}
			segmentOffset := r.pos - segmentStart
			r.reader, err = r.openPartReader(segment, segment.offset+segmentOffset, segment.length-segmentOffset)
			if err != nil {
				return read, err
			}
		}
		n, err := r.reader.Read(p[read:])
		read += n
		r.pos += int64(n)
		if err == io.EOF {
			_ = r.reader.Close()
			r.reader = nil
			continue
		}
		if err != nil {
			return read, err
		}
		if n == 0 {
			return read, io.ErrNoProgress
		}
	}
	if read == 0 {
		return 0, io.EOF
	}
	return read, nil
}

func (r *downloadReader) ReadAt(p []byte, off int64) (int, error) {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return 0, io.ErrClosedPipe
	}
	if len(p) == 0 {
		return 0, nil
	}
	if off < 0 {
		return 0, fmt.Errorf("download reader: negative offset %d", off)
	}
	if off >= r.length {
		return 0, io.EOF
	}
	want := len(p)
	if off+int64(want) > r.length {
		want = int(r.length - off)
	}

	read := 0
	for read < want {
		segment, segmentStart, err := r.segmentAt(off + int64(read))
		if err != nil {
			return read, err
		}
		segmentOffset := off + int64(read) - segmentStart
		span := min(int64(want-read), segment.length-segmentOffset)
		n, err := r.readSegmentAt(p[read:read+int(span)], segment, segmentOffset)
		read += n
		if err != nil {
			return read, err
		}
		if n != int(span) {
			return read, io.ErrUnexpectedEOF
		}
	}
	if want < len(p) {
		return read, io.EOF
	}
	return read, nil
}

func (r *downloadReader) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, io.ErrClosedPipe
	}
	var abs int64
	switch whence {
	case io.SeekStart:
		abs = offset
	case io.SeekCurrent:
		abs = r.pos + offset
	case io.SeekEnd:
		abs = r.length + offset
	default:
		return r.pos, fmt.Errorf("download reader: invalid whence %d", whence)
	}
	if abs < 0 {
		abs = 0
	}
	if abs > r.length {
		abs = r.length
	}
	if r.reader != nil {
		_ = r.reader.Close()
		r.reader = nil
	}
	r.pos = abs
	return r.pos, nil
}

func (r *downloadReader) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	if r.reader != nil {
		_ = r.reader.Close()
		r.reader = nil
	}
	r.mu.Unlock()
	return r.session.Close()
}

func (r *downloadReader) segmentAt(off int64) (downloadSegment, int64, error) {
	var start int64
	for _, segment := range r.parts {
		end := start + segment.length
		if off < end {
			return segment, start, nil
		}
		start = end
	}
	return downloadSegment{}, 0, io.EOF
}

func (r *downloadReader) readSegmentAt(p []byte, segment downloadSegment, off int64) (int, error) {
	span := int64(len(p))
	reader, err := r.openPartReader(segment, segment.offset+off, span)
	if err != nil {
		return 0, err
	}
	defer reader.Close()
	n, err := io.ReadFull(reader, p)
	if err == io.ErrUnexpectedEOF || err == io.EOF {
		return n, err
	}
	return n, err
}

func (r *downloadReader) openPartReader(segment downloadSegment, partOffset, span int64) (io.ReadCloser, error) {
	var reader io.ReadCloser
	var err error
	if r.file.Encryption {
		if !segment.part.Salt.Valid || segment.part.Salt.String == "" {
			return nil, ErrCorruptPartLayout
		}
		cipher, cipherErr := contentcrypto.NewCipher(r.key, segment.part.Salt.String)
		if cipherErr != nil {
			return nil, fmt.Errorf("create part cipher: %w", cipherErr)
		}
		reader, err = cipher.DecryptDataSeek(r.ctx, func(openCtx context.Context, offset, limit int64) (io.ReadCloser, error) {
			return r.session.OpenRange(openCtx, telegramstore.RangeRequest{
				UserID: r.userID, ChannelID: segment.part.ChannelID, MessageID: segment.part.MessageID,
				Offset: offset, Length: limit,
			})
		}, partOffset, span)
	} else {
		reader, err = r.session.OpenRange(r.ctx, telegramstore.RangeRequest{
			UserID: r.userID, ChannelID: segment.part.ChannelID, MessageID: segment.part.MessageID,
			Offset: partOffset, Length: span,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("open Telegram part %d: %w", segment.part.PartNo, err)
	}
	return reader, nil
}

func planSegments(parts []*sqlcgen.FilePart, totalSize, offset, requestedLength int64) ([]downloadSegment, int64, error) {
	length, err := normalizeDownloadRange(totalSize, offset, requestedLength)
	if err != nil {
		return nil, 0, err
	}
	if totalSize == 0 {
		if len(parts) != 0 || offset != 0 || length != 0 {
			return nil, 0, ErrCorruptPartLayout
		}
		return nil, 0, nil
	}

	var layoutSize int64
	for index, part := range parts {
		if part == nil || part.PartNo != int32(index+1) || !part.PlainSize.Valid || part.PlainSize.Int64 <= 0 || part.ChannelID == 0 || part.MessageID <= 0 || !part.StoredSize.Valid || part.StoredSize.Int64 <= 0 {
			return nil, 0, ErrCorruptPartLayout
		}
		layoutSize += part.PlainSize.Int64
	}
	if layoutSize != totalSize {
		return nil, 0, ErrCorruptPartLayout
	}
	if length == 0 {
		return nil, 0, nil
	}

	end := offset + length
	segments := make([]downloadSegment, 0)
	var partStart int64
	for _, part := range parts {
		partEnd := partStart + part.PlainSize.Int64
		if end <= partStart {
			break
		}
		if offset < partEnd && end > partStart {
			segmentStart := max(offset, partStart)
			segmentEnd := min(end, partEnd)
			segments = append(segments, downloadSegment{
				part: part, offset: segmentStart - partStart, length: segmentEnd - segmentStart,
			})
		}
		partStart = partEnd
	}
	if len(segments) == 0 {
		return nil, 0, ErrRangeNotSatisfiable
	}
	return segments, length, nil
}

func normalizeDownloadRange(totalSize, offset, requestedLength int64) (int64, error) {
	if totalSize < 0 || offset < 0 || requestedLength < -1 || offset > totalSize {
		return 0, ErrRangeNotSatisfiable
	}
	length := requestedLength
	available := totalSize - offset
	if length == -1 || length > available {
		length = available
	}
	if length < 0 {
		return 0, ErrRangeNotSatisfiable
	}
	return length, nil
}

func fileContentType(file *sqlcgen.File) string {
	if file.MimeType.Valid && file.MimeType.String != "" {
		return file.MimeType.String
	}
	return "application/octet-stream"
}

func fileETag(file *sqlcgen.File) string {
	if file.HashValue.Valid {
		return file.HashValue.String
	}
	return ""
}

type nopDownloadReader struct {
	*bytes.Reader
}

func (nopDownloadReader) Close() error { return nil }

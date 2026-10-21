package transfer

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/tgdrive/varc/source"

	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
)

func (d *Downloader) openCached(ctx context.Context, request DownloadRequest) (*Download, error) {
	// Authorization and current catalog state must be checked even on a cache hit.
	file, err := d.catalog.Get(ctx, request.UserID, request.FileID)
	if err != nil {
		return nil, err
	}
	if file.Kind != sqlcgen.FileKindFile || file.Status != sqlcgen.FileStatusActive || !file.Size.Valid || file.Size.Int64 < 0 {
		return nil, ErrInvalidDownload
	}
	var keyIdentity string
	if file.Encryption {
		if d.keys == nil || !file.EncryptionKeyVersion.Valid {
			return nil, ErrEncryptionKey
		}
		key, err := d.keys.Key(ctx, request.UserID, file.EncryptionKeyVersion.Int32)
		if err != nil || key == "" {
			return nil, errors.Join(ErrEncryptionKey, err)
		}
		// A changed key must not make previously decrypted cache bytes readable
		// when the same key version is accidentally configured with a new value.
		keyIdentity = fmt.Sprintf("/%x", sha256.Sum256([]byte(key)))
	}
	length, err := normalizeDownloadRange(file.Size.Int64, request.Offset, request.Length)
	if err != nil {
		return nil, err
	}
	version := downloadCacheVersion(file)
	object := &downloadCacheObject{
		downloader: d, request: request, version: version,
		meta: source.Metadata{
			Size: file.Size.Int64, ETag: version + keyIdentity,
			LastModified: file.ModTime.Time, ContentType: fileContentType(file),
		},
	}
	// File IDs are globally unique. Direct and shared URLs use the same entry;
	// metadata validators invalidate its bytes when the representation changes.
	reader, err := d.cache.Open(ctx, request.FileID.String(), object)
	if err != nil {
		return nil, fmt.Errorf("open download cache: %w", err)
	}
	return &Download{
		Reader: &cachedDownloadReader{SectionReader: io.NewSectionReader(reader, request.Offset, length), closer: reader},
		File:   file, Offset: request.Offset, Length: length, TotalSize: file.Size.Int64,
		ContentType: fileContentType(file), ETag: fileETag(file),
	}, nil
}

func downloadCacheVersion(file *sqlcgen.File) string {
	return fmt.Sprintf("%d-%d-%s", file.Generation, file.Size.Int64, fileETag(file))
}

type downloadCacheObject struct {
	downloader *Downloader
	request    DownloadRequest
	version    string
	meta       source.Metadata
}

func (o *downloadCacheObject) Metadata() source.Metadata { return o.meta }

func (o *downloadCacheObject) OpenRange(ctx context.Context, start, end int64) (io.ReadCloser, error) {
	if start < 0 || end < start || end > o.meta.Size {
		return nil, source.ErrRangeNotSatisfiable
	}
	request := o.request
	request.Offset, request.Length = start, end-start
	download, err := o.downloader.openOrigin(ctx, request)
	if err != nil {
		return nil, err
	}
	if downloadCacheVersion(download.File) != o.version {
		_ = download.Reader.Close()
		return nil, source.ErrObjectChanged
	}
	return download.Reader, nil
}

type cachedDownloadReader struct {
	*io.SectionReader
	closer io.Closer
}

func (r *cachedDownloadReader) Close() error { return r.closer.Close() }

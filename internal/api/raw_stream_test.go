package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/gotd/td/tgerr"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/db/sqlcgen"
	"github.com/tgdrive/teldrive/v2/internal/telegramstore"
	"github.com/tgdrive/teldrive/v2/internal/transfer"
)

func TestStreamFileAnswers502WhenTelegramFailsBeforeTheFirstByte(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"-503 behind the retry deadline", fmt.Errorf("download Telegram document chunk at 0: %w", errors.Join(context.DeadlineExceeded, tgerr.New(-503, "Timeout"))), "telegram_part_unavailable"},
		{"message gone", telegramstore.ErrMessageNotFound, "telegram_part_unavailable"},
		{"connection trouble", errors.New("engine was closed"), "telegram_read_failed"},
	} {
		file, parts := streamTestFile(64 * 1024)
		handler := &RawHandler{handler: &Handler{Downloader: transfer.NewDownloader(streamTestCatalog{file, parts}, &streamTestStorage{err: tc.err}, nil)}}
		fileID, _ := uuid.FromBytes(file.ID.Bytes[:])
		recorder := httptest.NewRecorder()
		err := handler.streamFile(context.Background(), recorder, file.UserID, fileID, file, gen.NewOptString("bytes=0-"), gen.OptETag{}, false)
		var p *Problem
		if !errors.As(err, &p) || p.Status != http.StatusBadGateway || p.Code != tc.code {
			t.Errorf("%s: streamFile() = %v, want 502 %s", tc.name, err, tc.code)
			continue
		}
		if len(recorder.Header().Get("Content-Range")) != 0 || recorder.Body.Len() != 0 {
			t.Errorf("%s: the 206 was committed before the error", tc.name)
		}
	}
}

func TestStreamFileAnswers502WhenAPartWithoutSizeIsGone(t *testing.T) {
	file, parts := streamTestFile(64 * 1024)
	parts[0].PlainSize, parts[0].StoredSize = pgtype.Int8{}, pgtype.Int8{} // an older upload: sizes resolved on open
	handler := &RawHandler{handler: &Handler{Downloader: transfer.NewDownloader(streamTestCatalog{file, parts}, &streamTestStorage{metadataErr: telegramstore.ErrMessageNotFound}, nil)}}
	fileID, _ := uuid.FromBytes(file.ID.Bytes[:])
	err := handler.streamFile(context.Background(), httptest.NewRecorder(), file.UserID, fileID, file, gen.NewOptString("bytes=0-"), gen.OptETag{}, false)
	var p *Problem
	if !errors.As(err, &p) || p.Status != http.StatusBadGateway || p.Code != "telegram_part_unavailable" {
		t.Fatalf("streamFile() = %v, want 502 telegram_part_unavailable", err)
	}
}

func TestStreamFileStreamsWholeRangeAfterTheFirstRead(t *testing.T) {
	file, parts := streamTestFile(300 * 1024)
	payload := bytes.Repeat([]byte("abcdef"), 50*1024)
	handler := &RawHandler{handler: &Handler{Downloader: transfer.NewDownloader(streamTestCatalog{file, parts}, &streamTestStorage{data: payload}, nil)}}
	fileID, _ := uuid.FromBytes(file.ID.Bytes[:])
	recorder := httptest.NewRecorder()
	if err := handler.streamFile(context.Background(), recorder, file.UserID, fileID, file, gen.NewOptString("bytes=10-"), gen.OptETag{}, false); err != nil {
		t.Fatalf("streamFile() = %v", err)
	}
	if recorder.Code != http.StatusPartialContent || !bytes.Equal(recorder.Body.Bytes(), payload[10:]) {
		t.Fatalf("code %d, %d bytes, want 206 with %d bytes", recorder.Code, recorder.Body.Len(), len(payload)-10)
	}
}

func TestMapStreamErrorKeepsOwnStatuses(t *testing.T) {
	var p *Problem
	if err := mapStreamError(context.DeadlineExceeded); !errors.As(err, &p) || p.Status != http.StatusGatewayTimeout {
		t.Fatalf("deadline: %v, want 504", err)
	}
	if err := mapStreamError(io.ErrUnexpectedEOF); !errors.As(err, &p) || p.Status != http.StatusBadGateway {
		t.Fatalf("short read: %v, want 502", err)
	}
}

func streamTestFile(size int64) (*sqlcgen.File, []*sqlcgen.FilePart) {
	file := &sqlcgen.File{
		ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, UserID: 7, Name: "film.mkv",
		Kind: sqlcgen.FileKindFile, Size: pgtype.Int8{Int64: size, Valid: true}, Status: sqlcgen.FileStatusActive,
	}
	return file, []*sqlcgen.FilePart{{
		PartNo: 1, ChannelID: 11, MessageID: 101,
		PlainSize: pgtype.Int8{Int64: size, Valid: true}, StoredSize: pgtype.Int8{Int64: size, Valid: true},
	}}
}

type streamTestCatalog struct {
	file  *sqlcgen.File
	parts []*sqlcgen.FilePart
}

func (c streamTestCatalog) Get(context.Context, int64, uuid.UUID) (*sqlcgen.File, error) {
	return c.file, nil
}

func (c streamTestCatalog) Parts(context.Context, int64, uuid.UUID) ([]*sqlcgen.FilePart, error) {
	return c.parts, nil
}

type streamTestStorage struct {
	data          []byte
	err           error
	metadataErr   error
	metadataCalls int
}

func (s *streamTestStorage) OpenRange(_ context.Context, request telegramstore.RangeRequest) (io.ReadCloser, error) {
	if s.err != nil {
		return io.NopCloser(failingReader{s.err}), nil
	}
	end := int64(len(s.data))
	if request.Length >= 0 && request.Offset+request.Length < end {
		end = request.Offset + request.Length
	}
	return io.NopCloser(bytes.NewReader(s.data[request.Offset:end])), nil
}

func (s *streamTestStorage) Upload(context.Context, telegramstore.UploadRequest) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, nil
}

func (s *streamTestStorage) DeleteMessages(context.Context, int64, int64, []int64) error { return nil }

func (s *streamTestStorage) CopyPart(context.Context, int64, int64, int64, int64) (telegramstore.StoredPart, error) {
	return telegramstore.StoredPart{}, nil
}

func (s *streamTestStorage) CreateChannel(context.Context, int64, string) (telegramstore.Channel, error) {
	return telegramstore.Channel{}, nil
}

func (s *streamTestStorage) DeleteChannel(context.Context, int64, int64) error { return nil }

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func (s *streamTestStorage) Metadata(_ context.Context, request telegramstore.MetadataRequest) (telegramstore.StoredPart, error) {
	s.metadataCalls++
	if s.metadataErr != nil {
		return telegramstore.StoredPart{}, s.metadataErr
	}
	return telegramstore.StoredPart{ChannelID: request.ChannelID, MessageID: request.MessageID, Size: int64(len(s.data))}, nil
}

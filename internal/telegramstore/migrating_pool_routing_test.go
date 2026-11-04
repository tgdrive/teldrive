package telegramstore

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

func TestMigratingPoolRoutesEachFileIndependently(t *testing.T) {
	ctx := WithClientID(context.Background(), 42)
	var charged, homeCalls int
	home := &testDCPool{InvokeFunc: func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
		homeCalls++
		dc := int(input.(*tg.UploadGetFileRequest).Location.(*tg.InputDocumentFileLocation).ID)
		if dc == 1 {
			output.(*tg.UploadFileBox).File = &tg.UploadFile{Bytes: []byte{1}}
			return nil
		}
		return tgerr.New(303, fmt.Sprintf("FILE_MIGRATE_%d", dc))
	}}
	opens := make(map[int]int)
	router := newMigratingPool(home, func(_ context.Context, dc int) (telegram.CloseInvoker, error) {
		opens[dc]++
		return &testDCPool{InvokeFunc: func(gotCtx context.Context, input bin.Encoder, output bin.Decoder) error {
			if gotCtx != ctx {
				t.Error("request context lost")
			}
			if id := input.(*tg.UploadGetFileRequest).Location.(*tg.InputDocumentFileLocation).ID; id != int64(dc) {
				t.Errorf("file %d routed to DC %d", id, dc)
			}
			output.(*tg.UploadFileBox).File = &tg.UploadFile{Bytes: []byte{byte(dc)}}
			return nil
		}}, nil
	}, func(api tg.Invoker) tg.Invoker {
		return telegram.InvokeFunc(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
			charged++
			return api.Invoke(ctx, input, output)
		})
	})
	defer router.Close()
	for _, dc := range []int{4, 1, 2, 4, 2, 1} {
		var output tg.UploadFileBox
		if err := router.Invoke(ctx, &tg.UploadGetFileRequest{Location: &tg.InputDocumentFileLocation{ID: int64(dc)}}, &output); err != nil {
			t.Fatal(err)
		}
		if got := output.File.(*tg.UploadFile).Bytes[0]; got != byte(dc) {
			t.Fatalf("bytes=%d want %d", got, dc)
		}
	}
	if homeCalls != 6 || opens[2] != 1 || opens[4] != 1 || len(opens) != 2 {
		t.Fatalf("home calls=%d opens=%v", homeCalls, opens)
	}
	if charged != 10 {
		t.Fatalf("middleware calls=%d, want 10 (6 home + 4 redirected)", charged)
	}
}

func TestMigratingPoolStreamsCrossDCChunksWithoutMovingHome(t *testing.T) {
	for _, parallel := range []int{1, 4} {
		t.Run(fmt.Sprintf("parallel_%d", parallel), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			// Exercise parallel full chunks and the short tail from the report.
			data := make([]byte, 4*telegramReadChunk+1056)
			for n := range data {
				data[n] = byte(n % 251)
			}
			serveChunk := func(_ context.Context, input bin.Encoder, output bin.Decoder) error {
				req := input.(*tg.UploadGetFileRequest)
				end := min(req.Offset+int64(req.Limit), int64(len(data)))
				output.(*tg.UploadFileBox).File = &tg.UploadFile{Bytes: data[req.Offset:end]}
				return nil
			}
			var redirects, homeReads, opens atomic.Int32
			home := &testDCPool{InvokeFunc: func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
				if input.(*tg.UploadGetFileRequest).Location.(*tg.InputDocumentFileLocation).ID == 4 {
					redirects.Add(1)
					return tgerr.New(303, "FILE_MIGRATE_4")
				}
				homeReads.Add(1)
				return serveChunk(ctx, input, output)
			}}
			router := newMigratingPool(home, func(_ context.Context, dc int) (telegram.CloseInvoker, error) {
				if dc != 4 {
					return nil, fmt.Errorf("unexpected target DC %d", dc)
				}
				opens.Add(1)
				return &testDCPool{InvokeFunc: serveChunk}, nil
			}, unwrappedPool)
			defer router.Close()
			api := tg.NewClient(router)
			for _, dc := range []int64{4, 1, 4} {
				readCtx, stop := context.WithCancel(ctx)
				reader := newTelegramRangeReader(readCtx, stop, 8, parallel)
				go func() {
					reader.finish(reader.fill(readCtx, api, &tg.InputDocumentFileLocation{ID: dc}, 0, int64(len(data)), nil))
				}()
				got, err := io.ReadAll(reader)
				reader.Close()
				if err != nil {
					t.Fatalf("stream from DC %d: %v", dc, err)
				}
				if !bytes.Equal(got, data) {
					t.Fatalf("stream from DC %d returned incorrect bytes", dc)
				}
			}
			if opens.Load() != 1 || redirects.Load() == 0 || homeReads.Load() == 0 {
				t.Fatalf("opens=%d redirects=%d home reads=%d", opens.Load(), redirects.Load(), homeReads.Load())
			}
		})
	}
}

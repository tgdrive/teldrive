package telegramstore

import (
	"context"
	"fmt"
	"testing"

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

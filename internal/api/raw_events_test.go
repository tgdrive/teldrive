package api

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	userevents "github.com/tgdrive/teldrive/v2/internal/events"
)

func TestEventCursor(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		params  gen.StreamEventsParams
		want    int64
		wantErr bool
	}{
		{name: "empty", want: 0},
		{name: "header", params: gen.StreamEventsParams{LastEventID: gen.NewOptString("42")}, want: 42},
		{name: "query", params: gen.StreamEventsParams{After: gen.NewOptInt64(7)}, want: 7},
		{name: "matching", params: gen.StreamEventsParams{LastEventID: gen.NewOptString("7"), After: gen.NewOptInt64(7)}, want: 7},
		{name: "conflict", params: gen.StreamEventsParams{LastEventID: gen.NewOptString("7"), After: gen.NewOptInt64(8)}, wantErr: true},
		{name: "negative", params: gen.StreamEventsParams{After: gen.NewOptInt64(-1)}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, err := eventCursor(test.params)
			if (err != nil) != test.wantErr {
				t.Fatalf("eventCursor() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("eventCursor() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestNormalizeEventTypes(t *testing.T) {
	t.Parallel()
	got, err := normalizeEventTypes([]string{" file.created ", "file.updated", "file.created"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "file.created" || got[1] != "file.updated" {
		t.Fatalf("normalizeEventTypes() = %#v", got)
	}
	if _, err := normalizeEventTypes([]string{""}); err == nil {
		t.Fatal("empty event type was accepted")
	}
	if _, err := normalizeEventTypes(make([]string, 51)); err == nil {
		t.Fatal("too many event types were accepted")
	}
}

func TestWriteStreamEvent(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	generation := int64(3)
	err := writeStreamEvent(response, userevents.Event{
		ID: 12, Type: "file.updated", ResourceType: "file", ResourceID: "abc",
		Generation: &generation, Payload: []byte(`{"name":"new.txt"}`),
		OccurredAt: time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("writeStreamEvent() error = %v", err)
	}
	body := response.Body.String()
	for _, want := range []string{"id: 12\n", "event: file.updated\n", `"resourceType":"file"`, `"generation":3`, `"name":"new.txt"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream body %q does not contain %q", body, want)
		}
	}
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (w *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	w.deadlines = append(w.deadlines, deadline)
	return nil
}

func (w *deadlineRecorder) FlushError() error {
	w.Flush()
	return nil
}

func TestWriteAndFlushStreamSetsAndClearsDeadline(t *testing.T) {
	t.Parallel()
	response := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	if err := writeAndFlushStream(response, time.Second, func() error {
		_, err := response.WriteString("data: test\n\n")
		return err
	}); err != nil {
		t.Fatalf("writeAndFlushStream() error = %v", err)
	}
	if !response.Flushed || response.Body.String() != "data: test\n\n" {
		t.Fatalf("response = flushed %t, body %q", response.Flushed, response.Body.String())
	}
	if len(response.deadlines) != 2 || response.deadlines[0].IsZero() || !response.deadlines[1].IsZero() {
		t.Fatalf("write deadlines = %#v", response.deadlines)
	}
}

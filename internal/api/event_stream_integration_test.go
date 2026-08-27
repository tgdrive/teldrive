//go:build integration

package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	api "github.com/tgdrive/teldrive/v2/internal/api"
	"github.com/tgdrive/teldrive/v2/internal/api/gen"
	"github.com/tgdrive/teldrive/v2/internal/events"
	testpostgres "github.com/tgdrive/teldrive/v2/internal/testutil/postgres"
)

func TestGeneratedServerEventStreamReplayTicketAndShutdown(t *testing.T) {
	db := testpostgres.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := db.Pool.Exec(ctx, "INSERT INTO users (user_id) VALUES (1001)"); err != nil {
		t.Fatal(err)
	}

	eventService, err := events.NewService(db.Pool, slog.New(slog.NewTextHandler(io.Discard, nil)), events.Config{
		BatchSize:             10,
		MaxConnectionsPerUser: 1,
		Heartbeat:             40 * time.Millisecond,
		WriteTimeout:          time.Second,
		TicketTTL:             time.Minute,
		Retention:             time.Hour,
		CleanupInterval:       time.Hour,
		ConnectTimeout:        time.Second,
		PingInterval:          20 * time.Millisecond,
		ReconnectMin:          time.Millisecond,
		ReconnectMax:          10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if err := eventService.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	serviceClosed := false
	defer func() {
		if serviceClosed {
			return
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
		defer closeCancel()
		if err := eventService.Close(closeCtx); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	handler := api.NewHandler(nil, nil, nil, nil, nil, 0, eventService)
	generated, err := api.NewServer(handler, api.NewSecurity(apiAuthenticator{}, eventService))
	if err != nil {
		t.Fatalf("NewServer() error = %v", err)
	}
	server := httptest.NewServer(generated)
	defer server.Close()

	unauthorized, err := http.Get(server.URL + "/v1/events")
	if err != nil {
		t.Fatalf("unauthorized event request: %v", err)
	}
	_ = unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}

	ticketRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/events/ticket", nil)
	if err != nil {
		t.Fatal(err)
	}
	ticketRequest.Header.Set("Authorization", "Bearer test-token")
	ticketResponse, err := http.DefaultClient.Do(ticketRequest)
	if err != nil {
		t.Fatalf("create ticket: %v", err)
	}
	defer ticketResponse.Body.Close()
	if ticketResponse.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(ticketResponse.Body)
		t.Fatalf("ticket status = %d, body = %s", ticketResponse.StatusCode, body)
	}
	var ticket gen.EventStreamTicket
	if err := json.NewDecoder(ticketResponse.Body).Decode(&ticket); err != nil {
		t.Fatalf("decode ticket: %v", err)
	}
	if ticket.Ticket == "" || !ticket.ExpiresAt.After(time.Now()) {
		t.Fatalf("ticket = %#v", ticket)
	}

	var fileID string
	if err := db.Pool.QueryRow(ctx, `
		INSERT INTO files (user_id, name, normalized_name, kind, mod_time)
		VALUES (1001, 'first', 'first', 'folder', now())
		RETURNING id::text
	`).Scan(&fileID); err != nil {
		t.Fatalf("insert file: %v", err)
	}

	firstURL := server.URL + "/v1/events?ticket=" + url.QueryEscape(ticket.Ticket) + "&after=0"
	firstResponse, firstCancel := openEventStream(t, firstURL, nil)
	if got := firstResponse.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := firstResponse.Header.Get("Cache-Control"); got != "no-cache, no-transform" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := firstResponse.Header.Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q", got)
	}
	firstFrame := readEventFrame(t, bufio.NewReader(firstResponse.Body), "file.created")
	firstCancel()
	_ = firstResponse.Body.Close()
	firstID, err := strconv.ParseInt(firstFrame["id"], 10, 64)
	if err != nil || firstID <= 0 {
		t.Fatalf("first event id = %q, %v", firstFrame["id"], err)
	}
	var firstData struct {
		Version      int             `json:"version"`
		ResourceType string          `json:"resourceType"`
		ResourceID   string          `json:"resourceId"`
		Payload      json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal([]byte(firstFrame["data"]), &firstData); err != nil {
		t.Fatalf("decode first event: %v", err)
	}
	if firstData.Version != 1 || firstData.ResourceType != "file" || firstData.ResourceID != fileID {
		t.Fatalf("first event data = %#v", firstData)
	}

	liveResponse, liveCancel := openEventStream(t, server.URL+"/v1/events?types=file.created,file.updated", map[string]string{
		"Authorization": "Bearer test-token",
	})
	liveReader := bufio.NewReader(liveResponse.Body)
	if frame := readSSEFrame(t, liveReader); frame["retry"] != "3000" {
		t.Fatalf("initial live stream frame = %#v", frame)
	}

	if _, err := db.Pool.Exec(ctx, "UPDATE files SET name = 'second', normalized_name = 'second', generation = generation + 1 WHERE id = $1", fileID); err != nil {
		t.Fatal(err)
	}
	liveFrame := readEventFrame(t, liveReader, "file.updated")
	liveCancel()
	_ = liveResponse.Body.Close()
	liveID, err := strconv.ParseInt(liveFrame["id"], 10, 64)
	if err != nil || liveID <= firstID {
		t.Fatalf("live event id = %q, first = %d, error = %v", liveFrame["id"], firstID, err)
	}

	secondResponse, secondCancel := openEventStream(t, server.URL+"/v1/events?types=file.created,file.updated", map[string]string{
		"Authorization": "Bearer test-token",
		"Last-Event-ID": strconv.FormatInt(firstID, 10),
	})
	secondFrame := readEventFrame(t, bufio.NewReader(secondResponse.Body), "file.updated")
	secondCancel()
	_ = secondResponse.Body.Close()
	secondID, err := strconv.ParseInt(secondFrame["id"], 10, 64)
	if err != nil || secondID <= firstID {
		t.Fatalf("second event id = %q, first = %d, error = %v", secondFrame["id"], firstID, err)
	}

	invalidRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/events?after=-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	invalidRequest.Header.Set("Authorization", "Bearer test-token")
	invalidResponse, err := http.DefaultClient.Do(invalidRequest)
	if err != nil {
		t.Fatalf("invalid cursor request: %v", err)
	}
	_ = invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid cursor status = %d", invalidResponse.StatusCode)
	}

	futureRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/events?after="+strconv.FormatInt(secondID+1000, 10), nil)
	if err != nil {
		t.Fatal(err)
	}
	futureRequest.Header.Set("Authorization", "Bearer test-token")
	futureResponse, err := http.DefaultClient.Do(futureRequest)
	if err != nil {
		t.Fatalf("future cursor request: %v", err)
	}
	_ = futureResponse.Body.Close()
	if futureResponse.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("future cursor status = %d", futureResponse.StatusCode)
	}

	if _, err := db.Pool.Exec(ctx, "DELETE FROM user_events WHERE user_id = 1001"); err != nil {
		t.Fatal(err)
	}
	gapResponse, gapCancel := openEventStream(t, server.URL+"/v1/events", map[string]string{
		"Authorization": "Bearer test-token",
		"Last-Event-ID": strconv.FormatInt(firstID, 10),
	})
	gapReader := bufio.NewReader(gapResponse.Body)
	if frame := readSSEFrame(t, gapReader); frame["retry"] != "3000" {
		t.Fatalf("gap retry frame = %#v", frame)
	}
	if frame := readSSEFrame(t, gapReader); frame["event"] != "sync.required" || !strings.Contains(frame["data"], "cursor_expired") {
		t.Fatalf("gap control frame = %#v", frame)
	}
	gapCancel()
	_ = gapResponse.Body.Close()

	shutdownResponse, shutdownCancel := openEventStream(t, server.URL+"/v1/events", map[string]string{
		"Authorization": "Bearer test-token",
		"Last-Event-ID": strconv.FormatInt(secondID, 10),
	})
	shutdownReader := bufio.NewReader(shutdownResponse.Body)
	if frame := readSSEFrame(t, shutdownReader); frame["retry"] != "3000" {
		t.Fatalf("initial stream frame = %#v", frame)
	}
	if _, err := db.Pool.Exec(ctx, "ALTER TABLE user_events DISABLE TRIGGER user_events_notify_after_insert"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, `
		INSERT INTO user_events (user_id, event_type, resource_type, resource_id, payload)
		VALUES (1001, 'test.polled', 'test', 'poll-only', '{}'::jsonb)
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Pool.Exec(ctx, "ALTER TABLE user_events ENABLE TRIGGER user_events_notify_after_insert"); err != nil {
		t.Fatal(err)
	}
	if frame := readEventFrame(t, shutdownReader, "test.polled"); !strings.Contains(frame["data"], "poll-only") {
		t.Fatalf("polled event frame = %#v", frame)
	}
	if frame := readSSEFrame(t, shutdownReader); frame["comment"] != "keep-alive" {
		t.Fatalf("heartbeat frame = %#v", frame)
	}

	limitedRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/v1/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	limitedRequest.Header.Set("Authorization", "Bearer test-token")
	limitedResponse, err := http.DefaultClient.Do(limitedRequest)
	if err != nil {
		t.Fatalf("limited stream request: %v", err)
	}
	_ = limitedResponse.Body.Close()
	if limitedResponse.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("limited stream status = %d", limitedResponse.StatusCode)
	}

	closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
	if err := eventService.Close(closeCtx); err != nil {
		closeCancel()
		t.Fatalf("eventService.Close() error = %v", err)
	}
	closeCancel()
	serviceClosed = true

	readResult := make(chan error, 1)
	go func() {
		_, err := shutdownReader.ReadByte()
		readResult <- err
	}()
	select {
	case err := <-readResult:
		if err != nil && err != io.EOF && !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("stream shutdown read error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("event stream did not close with the event service")
	}
	shutdownCancel()
	_ = shutdownResponse.Body.Close()
}

func openEventStream(t *testing.T, endpoint string, headers map[string]string) (*http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		cancel()
		t.Fatalf("open event stream: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		cancel()
		t.Fatalf("event stream status = %d, body = %s", response.StatusCode, body)
	}
	return response, cancel
}

func readEventFrame(t *testing.T, reader *bufio.Reader, eventName string) map[string]string {
	t.Helper()
	for {
		frame := readSSEFrame(t, reader)
		if frame["event"] == eventName {
			return frame
		}
	}
}

func readSSEFrame(t *testing.T, reader *bufio.Reader) map[string]string {
	t.Helper()
	frame := make(map[string]string)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE frame: %v", err)
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			return frame
		}
		if strings.HasPrefix(line, ":") {
			frame["comment"] = strings.TrimSpace(strings.TrimPrefix(line, ":"))
			continue
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("invalid SSE line %q", line)
		}
		frame[name] = strings.TrimSpace(value)
	}
}

func Example_eventStreamFrame() {
	fmt.Println("id: 42")
	fmt.Println("event: file.updated")
	fmt.Println(`data: {"version":1}`)
	// Output:
	// id: 42
	// event: file.updated
	// data: {"version":1}
}

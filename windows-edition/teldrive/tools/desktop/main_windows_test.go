package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRcloneArgsDoNotInvokeShell(t *testing.T) {
	q := request{Remote: "teldrive", Action: "upload", Local: `D:\Archivos con espacios\$(malicioso)`, Destination: "--delete", DryRun: true}
	args, e := rcloneArgs(q)
	if e != nil {
		t.Fatal(e)
	}
	joined := strings.Join(args, "|")
	if !strings.Contains(joined, "|"+q.Local+"|") || !strings.Contains(joined, "|teldrive:--delete|") || !strings.Contains(joined, "--dry-run") {
		t.Fatal(args)
	}
	for _, bad := range []request{{Remote: "[injected]", Action: "list"}, {Remote: "teldrive", Action: "purge"}, {Remote: "teldrive", Action: "upload", Local: "relative"}, {Remote: "teldrive", Action: "mount", Drive: "C:"}} {
		if _, e = rcloneArgs(bad); e == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
func TestDesktopAPIRejectsCrossOriginAndNeedsCookie(t *testing.T) {
	m := &manager{dir: t.TempDir(), origin: "http://127.0.0.1:12345", secret: "private", jobs: map[string]*job{}}
	for _, tc := range []struct {
		origin, cookie string
		want           int
	}{{"http://attacker.invalid", "private", 403}, {m.origin, "", 403}, {m.origin, "private", 200}} {
		r := httptest.NewRequest(http.MethodPost, m.origin+"/desktop/api/config", bytes.NewBufferString(`{"config":"[jwt]\nsecret = \"test\"\n"}`))
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("X-Teldrive-Desktop", "1")
		if tc.cookie != "" {
			r.AddCookie(&http.Cookie{Name: "teldrive_desktop", Value: tc.cookie})
		}
		w := httptest.NewRecorder()
		m.api(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: %d %s", tc.origin, w.Code, w.Body.String())
		}
	}
	b, e := os.ReadFile(filepath.Join(m.dir, "config.toml"))
	if e != nil || !strings.Contains(string(b), `secret = "test"`) {
		t.Fatalf("saved %q %v", b, e)
	}
}
func TestInvalidTOMLPreservesSavedConfiguration(t *testing.T) {
	m := &manager{dir: t.TempDir(), origin: "http://127.0.0.1:12345", secret: "private", jobs: map[string]*job{}}
	p := filepath.Join(m.dir, "config.toml")
	os.WriteFile(p, []byte("original"), 0600)
	r := httptest.NewRequest("POST", m.origin+"/desktop/api/config", strings.NewReader(`{"config":"[broken"}`))
	r.Header.Set("Origin", m.origin)
	r.Header.Set("X-Teldrive-Desktop", "1")
	r.AddCookie(&http.Cookie{Name: "teldrive_desktop", Value: m.secret})
	w := httptest.NewRecorder()
	m.api(w, r)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "original" {
		t.Fatal("overwrote config")
	}
}

func TestBundledPostgresCreatesAndReopensLocalDatabase(t *testing.T) {
	if os.Getenv("TELDRIVE_DESKTOP_DB_TEST") != "1" {
		t.Skip("set TELDRIVE_DESKTOP_DB_TEST=1 to exercise bundled PostgreSQL")
	}
	m := &manager{dir: t.TempDir(), jobs: map[string]*job{}}
	if e := os.WriteFile(filepath.Join(m.dir, "config.toml"), []byte("[jwt]\nsecret = \"test-only-secret\"\n"), 0600); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := m.stopDatabase(); e != nil {
			t.Error(e)
		}
	})
	if e := m.setupDatabase(); e != nil {
		t.Fatal(e)
	}
	d, e := m.databaseInfo()
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(m.dir, "config.toml"))
	if e != nil || !strings.Contains(string(b), d.dsn()) || !strings.Contains(string(b), "test-only-secret") {
		t.Fatalf("configuration was not preserved: %v", e)
	}
	c := m.dbCommand("psql.exe", d, "-h", "127.0.0.1", "-p", d.Port, "-U", "teldrive", "-d", "teldrive", "-tAc", "SELECT current_database()")
	output, e := c.Output()
	if e != nil || strings.TrimSpace(string(output)) != "teldrive" {
		t.Fatalf("database query: %s %v", output, e)
	}
	if e = m.setupDatabase(); e != nil {
		t.Fatalf("reopening existing database: %v", e)
	}
}

func TestDesktopRunsBundledRcloneAndReportsProgress(t *testing.T) {
	data, e := assets.ReadFile("bundle/rclone.exe.gz")
	if e != nil {
		t.Fatal(e)
	}
	reader, e := gzip.NewReader(bytes.NewReader(data))
	if e != nil {
		t.Fatal(e)
	}
	binary, e := io.ReadAll(reader)
	reader.Close()
	if e != nil {
		t.Fatal(e)
	}
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/auth/session" {
			w.Write([]byte(`{"name":"Cuenta de prueba","userName":"demo","userId":123,"hash":"test-only","expires":"2027-01-01T00:00:00Z"}`))
			return
		}
		w.Write([]byte(`{"items":[{"id":"folder-1","name":"Proyectos","type":"folder","mimeType":"drive/folder","size":0,"createdAt":"2026-10-01T12:00:00Z","updatedAt":"2026-10-08T18:30:00Z"}],"meta":{"totalPages":1,"currentPage":1,"count":1}}`))
	}))
	defer fixture.Close()
	m := &manager{dir: t.TempDir(), origin: "http://127.0.0.1:12345", secret: "private", jobs: map[string]*job{}}
	if e = os.WriteFile(filepath.Join(m.dir, "rclone.exe"), binary, 0600); e != nil {
		t.Fatal(e)
	}
	q := request{Action: "list", Remote: "td", Config: "[td]\ntype = teldrive\napi_host = " + fixture.URL + "\naccess_token = test-only-token\n"}
	body, _ := json.Marshal(q)
	r := httptest.NewRequest("POST", m.origin+"/desktop/api/rclone/start", bytes.NewReader(body))
	r.Header.Set("Origin", m.origin)
	r.Header.Set("X-Teldrive-Desktop", "1")
	r.AddCookie(&http.Cookie{Name: "teldrive_desktop", Value: m.secret})
	w := httptest.NewRecorder()
	m.api(w, r)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	defer m.stop("rclone")
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		j := m.jobs["rclone"]
		running := j.Running
		exit := j.Exit
		m.mu.Unlock()
		if !running {
			if exit != "Finalizado correctamente" {
				b, _ := os.ReadFile(filepath.Join(m.dir, "rclone.log"))
				t.Fatalf("%s: %s", exit, b)
			}
			b, _ := os.ReadFile(filepath.Join(m.dir, "rclone.log"))
			if !strings.Contains(string(b), "Proyectos") {
				t.Fatalf("missing listing output: %s", b)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("rclone did not finish")
}

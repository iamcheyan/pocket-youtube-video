package media

import (
	"database/sql"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestStubDownloadRunsThroughQueue(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\necho '[download] 12.5% of 8.00MiB at 1MiB/s'\necho '{\"title\":\"stub\"}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE accounts(id TEXT PRIMARY KEY,active INTEGER NOT NULL DEFAULT 0); INSERT INTO accounts(id,active) VALUES('demo',1);`); err != nil {
		t.Fatal(err)
	}
	s, err := New(db, Config{YtDlpPath: stub, DownloadDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	r := httptest.NewRequest("POST", "/api/downloads", strings.NewReader(`{"url":"https://example.invalid/video"}`))
	w := httptest.NewRecorder()
	s.downloads(w, r)
	if w.Code != 202 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var state string
		if err := db.QueryRow("SELECT state FROM downloads WHERE id=1").Scan(&state); err == nil && state == "completed" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("stub job did not complete")
}

func TestHeadlessMpvIPCCommands(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv unavailable")
	}
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s, err := New(db, Config{Headless: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	p, err := s.launch("av://lavfi:testsrc=size=64x64:rate=1")
	if err != nil {
		t.Fatal(err)
	}
	defer p.conn.Close()
	if _, err = p.request(map[string]any{"command": []any{"get_property", "pause"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = p.request(map[string]any{"command": []any{"set_property", "volume", 25}}); err != nil {
		t.Fatal(err)
	}
}

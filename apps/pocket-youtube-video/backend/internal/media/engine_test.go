package media

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func engineFixture(t *testing.T, script string) (*Service, *sql.DB, string) {
	t.Helper()
	dir := t.TempDir()
	stub := filepath.Join(dir, "yt-dlp")
	if err := os.WriteFile(stub, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "test.db")+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE accounts(id TEXT PRIMARY KEY, active INTEGER NOT NULL DEFAULT 0); INSERT INTO accounts(id,active) VALUES('demo',1); CREATE TABLE settings(account_id TEXT NOT NULL,key TEXT NOT NULL,value TEXT NOT NULL,PRIMARY KEY(account_id,key));`); err != nil {
		t.Fatal(err)
	}
	s, err := New(db, Config{YtDlpPath: stub, DownloadDir: dir, Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close(); db.Close() })
	return s, db, dir
}

func TestYtDlpArgsAndProgressParsing(t *testing.T) {
	s, _, dir := engineFixture(t, `#!/bin/sh
printf '%s\n' "$@" > "$0.args"
echo '[download]  12.3% of 10.00MiB at 1.00MiB/s ETA 00:08'
echo '[download] 100.0% of 10.00MiB in 00:02'
echo '{"id":"stub"}'
printf '%s/stub-video.mp4\n' "$(dirname "$0")"
exit 0
`)
	if _, err := s.db.Exec(`INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority) VALUES('demo','video-0001','https://example.invalid/v','both','720p','192',0)`); err != nil {
		t.Fatal(err)
	}
	j := &job{ID: 1, AccountID: "demo", VideoID: "video-0001", URL: "https://example.invalid/v", Mode: "both", Quality: "720p", AudioBitrate: "192", State: "queued"}
	s.mu.Lock()
	s.jobs[1] = j
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.start(j)
	for ctx.Err() == nil {
		var state string
		_ = s.db.QueryRow(`SELECT state FROM downloads WHERE id=1`).Scan(&state)
		if state == "completed" || state == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "yt-dlp.args"))
	if err != nil {
		t.Fatal(err)
	}
	args := string(raw)
	for _, want := range []string{"--newline", "--print-json", "--continue", "bestvideo[height<=720]+bestaudio/best", "--print\nafter_move:filepath"} {
		if !strings.Contains(args, want) {
			t.Fatalf("missing arg %q in\n%s", want, args)
		}
	}
	var state, path string
	var progress float64
	if err := s.db.QueryRow(`SELECT state,progress,output_path FROM downloads WHERE id=1`).Scan(&state, &progress, &path); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || progress != 100 {
		t.Fatalf("state=%s progress=%v", state, progress)
	}
}

func TestPauseResumeUsesSignals(t *testing.T) {
	s, _, _ := engineFixture(t, `#!/bin/sh
trap 'exit 143' TERM
i=0
while [ $i -lt 200 ]; do echo '[download]  1.0% of 10.00MiB at 1.00MiB/s ETA 00:08'; i=$((i+1)); sleep 0.05; done
`)
	if _, err := s.db.Exec(`INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority) VALUES('demo','video-0002','https://example.invalid/pause','video','720p','192',0)`); err != nil {
		t.Fatal(err)
	}
	j := &job{ID: 1, AccountID: "demo", VideoID: "video-0002", URL: "https://example.invalid/pause", Mode: "video", Quality: "720p", AudioBitrate: "192", State: "queued"}
	s.mu.Lock()
	s.jobs[1] = j
	s.mu.Unlock()
	s.start(j)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		cmd := j.cmd
		state := j.State
		s.mu.Unlock()
		if cmd != nil && cmd.Process != nil && state == "downloading" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	cmd := j.cmd
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		t.Fatal("download process did not start")
	}
	if err := cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	var state string
	_ = s.db.QueryRow(`SELECT state FROM downloads WHERE id=1`).Scan(&state)
	if state != "downloading" {
		t.Fatalf("unexpected state %s", state)
	}
}

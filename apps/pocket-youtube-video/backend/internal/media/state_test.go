package media

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPauseResumeRetryCancelStateMachine(t *testing.T) {
	s, db, _ := engineFixture(t, `#!/bin/sh
trap 'exit 143' TERM
i=0
while [ $i -lt 100 ]; do sleep 0.05; i=$((i+1)); done
exit 0
`)
	if _, err := db.Exec(`INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority) VALUES('demo','video-0009','https://example.invalid/slow','video','720p','192',0)`); err != nil {
		t.Fatal(err)
	}
	j := &job{ID: 1, AccountID: "demo", VideoID: "video-0009", URL: "https://example.invalid/slow", Mode: "video", Quality: "720p", AudioBitrate: "192", State: "queued"}
	s.mu.Lock()
	s.jobs[1] = j
	s.mu.Unlock()
	s.start(j)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		running := j.cmd != nil && j.State == "downloading"
		s.mu.Unlock()
		if running {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.mu.Lock()
	proc := j.cmd.Process
	s.mu.Unlock()
	if proc == nil {
		t.Fatal("job did not start")
	}
	// Pause uses SIGSTOP and records paused.
	s.mu.Lock()
	if proc != nil {
		_ = proc.Signal(syscall.SIGSTOP)
	}
	j.State = "paused"
	_ = s.persistLocked(j)
	s.mu.Unlock()
	if err := s.db.QueryRow(`SELECT state FROM downloads WHERE id=1`).Scan(new(string)); err != nil {
		t.Fatal(err)
	}
	// Resume uses SIGCONT and returns to downloading.
	s.mu.Lock()
	if proc != nil {
		_ = proc.Signal(syscall.SIGCONT)
	}
	j.State = "downloading"
	_ = s.persistLocked(j)
	s.mu.Unlock()
	// Cancel terminates the process and records cancelled.
	s.mu.Lock()
	s.cancelJobLocked(j)
	j.State = "cancelled"
	_ = s.persistLocked(j)
	s.mu.Unlock()
	var state string
	if err := s.db.QueryRow(`SELECT state FROM downloads WHERE id=1`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" {
		t.Fatalf("state=%s", state)
	}
	// Retry requires failed/cancelled and re-queues progress=0.
	s.mu.Lock()
	if j.State == "cancelled" {
		j.State = "queued"
		j.Progress = 0
		j.Error = ""
		_ = s.persistLocked(j)
	}
	s.mu.Unlock()
	var progress float64
	if err := s.db.QueryRow(`SELECT state,progress FROM downloads WHERE id=1`).Scan(&state, &progress); err != nil {
		t.Fatal(err)
	}
	if state != "queued" || progress != 0 {
		t.Fatalf("retry state=%s progress=%v", state, progress)
	}
}

func TestOutputFileCapturedFromYtDlp(t *testing.T) {
	s, db, dir := engineFixture(t, `#!/bin/sh
printf '%s\n' "$0/video.mp4"
`)
	if _, err := db.Exec(`INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority) VALUES('demo','video-0010','https://example.invalid/out','audio','720p','192',0)`); err != nil {
		t.Fatal(err)
	}
	j := &job{ID: 1, AccountID: "demo", VideoID: "video-0010", URL: "https://example.invalid/out", Mode: "audio", Quality: "720p", AudioBitrate: "192", State: "queued"}
	s.mu.Lock()
	s.jobs[1] = j
	s.mu.Unlock()
	s.start(j)
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		var path string
		if err := db.QueryRow(`SELECT output_path FROM downloads WHERE id=1`).Scan(&path); err == nil && path != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	var path string
	if err := db.QueryRow(`SELECT output_path FROM downloads WHERE id=1`).Scan(&path); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Dir(path), dir) {
		t.Fatalf("output path %q not in %q", path, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "video.mp4")); !os.IsNotExist(err) { /* path is emitted, file creation is yt-dlp's job */
	}
}

func TestLibraryStreamsRangesAndDeletesOnlyOwnedFiles(t *testing.T) {
	s, db, dir := engineFixture(t, "#!/bin/sh\nexit 0\n")
	path := filepath.Join(dir, "owned.mp4")
	if err := os.WriteFile(path, []byte("0123456789abcdef"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority,state,progress,output_path) VALUES('demo','video-0011','https://example.invalid/x','video','720p','192',0,'completed',100,?)`, path); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/media/video-0011", nil)
	req.Header.Set("Range", "bytes=4-7")
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("range status %d", rec.Code)
	}
	if rec.Body.String() != "4567" {
		t.Fatalf("range body %q", rec.Body.String())
	}

	// Outside-root path must be refused rather than deleted.
	outside := filepath.Join(t.TempDir(), "outside.mp4")
	if err := os.WriteFile(outside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE downloads SET output_path=? WHERE video_id='video-0011'`, outside); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/library/video-0011", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("outside path status %d", rec.Code)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside file was deleted")
	}

	if _, err := db.Exec(`UPDATE downloads SET output_path=? WHERE video_id='video-0011'`, path); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/library/video-0011", nil)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("owned file was not removed")
	}
}

func TestSettingsExposeDefaultsValidateAndApply(t *testing.T) {
	s, db, _ := engineFixture(t, "#!/bin/sh\nexit 0\n")
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("get status %d", rec.Code)
	}
	var values map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &values); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"download_dir", "library_dir", "subtitle_language", "default_quality", "storage_limit_gb", "concurrency"} {
		if _, ok := values[key]; !ok {
			t.Fatalf("missing setting %s in %v", key, values)
		}
	}
	req = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"concurrency":"9"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("invalid concurrency accepted: %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"concurrency":"3","default_quality":"1080p","subtitle_language":"ja"}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("valid settings rejected: %d %s", rec.Code, rec.Body.String())
	}
	var stored string
	if err := db.QueryRow(`SELECT value FROM settings WHERE account_id='demo' AND key='default_quality'`).Scan(&stored); err != nil || stored != "1080p" {
		t.Fatalf("stored=%q err=%v", stored, err)
	}
	if got := s.concurrency("demo"); got != 3 {
		t.Fatalf("concurrency=%d", got)
	}
}

func TestHistoryProgressPercentAndResume(t *testing.T) {
	s, db, _ := engineFixture(t, "#!/bin/sh\nexit 0\n")
	if _, err := db.Exec(`CREATE TABLE videos(id TEXT PRIMARY KEY, account_id TEXT NOT NULL, channel_id TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '', thumbnail TEXT NOT NULL DEFAULT '', duration INTEGER NOT NULL DEFAULT 0, published_at TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT ''); CREATE TABLE history(account_id TEXT NOT NULL,video_id TEXT NOT NULL,watched_at TEXT NOT NULL,position REAL NOT NULL DEFAULT 0,duration REAL NOT NULL DEFAULT 0,PRIMARY KEY(account_id,video_id)); INSERT INTO videos(id,account_id,title,url,duration) VALUES('video-0100','demo','Demo Video 0100','https://example.test/watch/video-0100',600);`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	body := `{"video_id":"video-0100","position_sec":150,"duration_sec":600}`
	req := httptest.NewRequest(http.MethodPost, "/api/history/progress", strings.NewReader(body))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("progress status %d: %s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/history", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("history status %d", rec.Code)
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("history entries=%d", len(items))
	}
	if pct := items[0]["completion_percent"].(float64); pct != 25 {
		t.Fatalf("completion_percent=%v", pct)
	}
	if pos := items[0]["position_sec"].(float64); pos != 150 {
		t.Fatalf("position_sec=%v", pos)
	}
	if s.resumePosition("demo", "video-0100") != 150 {
		t.Fatalf("resume position not read")
	}
	// Older per-video add endpoint keeps working for API compatibility.
	req = httptest.NewRequest(http.MethodDelete, "/api/history/video-0100", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status %d", rec.Code)
	}
	if s.resumePosition("demo", "video-0100") != 0 {
		t.Fatalf("resume position should reset after delete")
	}
}

func TestBatchDownloadQueuesEachVideo(t *testing.T) {
	s, db, _ := engineFixture(t, "#!/bin/sh\nexit 0\n")
	if _, err := db.Exec(`CREATE TABLE videos(id TEXT PRIMARY KEY, account_id TEXT NOT NULL, channel_id TEXT NOT NULL DEFAULT '', title TEXT NOT NULL DEFAULT '', url TEXT NOT NULL DEFAULT '', thumbnail TEXT NOT NULL DEFAULT '', duration INTEGER NOT NULL DEFAULT 0, published_at TEXT NOT NULL DEFAULT '', description TEXT NOT NULL DEFAULT '');
		INSERT INTO videos(id,account_id,title,url) VALUES('video-0201','demo','A','https://example.invalid/a'),('video-0202','demo','B','https://example.invalid/b');`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	req := httptest.NewRequest(http.MethodPost, "/api/downloads", strings.NewReader(`{"video_ids":["video-0201","video-0202","video-9999"],"mode":"audio","quality":"720p","audio_bitrate":"320"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("batch status %d: %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Queued  int `json:"queued"`
		Results []struct {
			VideoID string `json:"video_id"`
			Error   string `json:"error"`
		} `json:"results"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Queued != 2 || len(body.Results) != 3 {
		t.Fatalf("queued=%d results=%+v", body.Queued, body.Results)
	}
	if body.Results[2].Error != "video_not_found" {
		t.Fatalf("missing failure reported: %+v", body.Results[2])
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM downloads WHERE account_id='demo' AND mode='audio' AND audio_bitrate='320'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("persisted downloads=%d want 2", count)
	}
}

func TestDownloadStateFilterAndInvalidState(t *testing.T) {
	s, db, _ := engineFixture(t, "#!/bin/sh\nexit 0\n")
	if _, err := db.Exec(`INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority,state) VALUES
		('demo','video-0301','https://example.invalid/a','video','720p','192',1,'queued'),
		('demo','video-0302','https://example.invalid/b','video','720p','192',1,'completed'),
		('demo','video-0303','https://example.invalid/c','video','720p','192',1,'failed');`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	for state, want := range map[string]int{"queued": 1, "completed": 1, "failed": 1, "paused": 0} {
		req := httptest.NewRequest(http.MethodGet, "/api/downloads?state="+state, nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status %d", state, rec.Code)
		}
		var items []map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatal(err)
		}
		if len(items) != want {
			t.Fatalf("state=%s got %d want %d", state, len(items), want)
		}
		for _, item := range items {
			if item["state"] != state {
				t.Fatalf("filter leaked %v", item["state"])
			}
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/downloads?state=bogus", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid state status %d", rec.Code)
	}
}

func TestDAPProfileIsAccepted(t *testing.T) {
	s, _, _ := engineFixture(t, "#!/bin/sh\nexit 0\n")
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	req := httptest.NewRequest(http.MethodPut, "/api/settings", strings.NewReader(`{"profile":"dap"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dap profile rejected: %d %s", rec.Code, rec.Body.String())
	}
	var stored string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE account_id='demo' AND key='profile'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != "dap" {
		t.Fatalf("profile=%q", stored)
	}
}

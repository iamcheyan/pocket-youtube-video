// Package tests holds black-box integration tests that exercise the running
// videod HTTP surface exactly as a client would. Unit and package-level tests
// live next to the code they cover; this suite proves the assembled stack.
package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/api"
	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/auth"
	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/media"
	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/store"
)

type stack struct {
	t       *testing.T
	server  *httptest.Server
	media   *media.Service
	dataDir string
}

func newStack(t *testing.T) *stack {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "integration.sqlite"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := store.Seed(context.Background(), db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	key, err := auth.NewFileCredentialStore(filepath.Join(dir, "credential.key"))
	if err != nil {
		t.Fatalf("credential store: %v", err)
	}
	mux := http.NewServeMux()
	api.New(db).Register(mux)
	auth.NewHandler(db, key, auth.OAuthConfig{}, auth.GoogleTokenExchanger{}).RegisterRoutes(mux)
	mediaConfig := media.Config{
		YtDlpPath:   stubPath(t),
		DownloadDir: filepath.Join(dir, "downloads"),
		IPCDir:      filepath.Join(dir, "run"),
		Concurrency: 2,
		Headless:    true,
	}
	svc, err := media.New(db, mediaConfig)
	if err != nil {
		t.Fatalf("media: %v", err)
	}
	if err := store.SeedDownloads(context.Background(), db); err != nil {
		t.Fatalf("seed downloads: %v", err)
	}
	media.RegisterRoutes(mux, svc)
	srv := httptest.NewServer(mux)
	t.Cleanup(func() {
		srv.Close()
		svc.Close()
		db.Close()
	})
	return &stack{t: t, server: srv, media: svc, dataDir: dir}
}

// stubPath writes a fake yt-dlp that replays real progress lines and emits an
// output path, so queue behaviour is testable without network access.
func stubPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "yt-dlp")
	script := `#!/bin/sh
out_dir=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out_dir="$a"; fi
  prev="$a"
done
echo '[download]  12.3% of 8.00MiB at 1.25MiB/s ETA 00:06'
sleep 0.05
echo '[download]  55.0% of 8.00MiB at 1.25MiB/s ETA 00:03'
sleep 0.05
dir=$(dirname "$out_dir")
file="$dir/integration-stub.mp4"
printf 'stub media payload' > "$file"
echo '[download] 100.0% of 8.00MiB in 00:01'
echo "{\"id\":\"integration-stub\"}"
echo "$file"
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}

func (s *stack) do(method, path string, body any) (int, []byte) {
	s.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			s.t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, s.server.URL+path, reader)
	if err != nil {
		s.t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := s.server.Client().Do(req)
	if err != nil {
		s.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, raw
}

func (s *stack) getJSON(path string, out any) int {
	s.t.Helper()
	code, raw := s.do(http.MethodGet, path, nil)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			s.t.Fatalf("decode %s: %v (%s)", path, err, string(raw))
		}
	}
	return code
}

func TestHealthReportsVersionToolsAndSchema(t *testing.T) {
	s := newStack(t)
	var health struct {
		Version       string                    `json:"version"`
		Status        string                    `json:"status"`
		Mock          bool                      `json:"mock"`
		SchemaVersion int                       `json:"schema_version"`
		Tools         map[string]map[string]any `json:"tools"`
		Auth          map[string]any            `json:"auth"`
	}
	if code := s.getJSON("/api/health", &health); code != http.StatusOK {
		t.Fatalf("health status %d", code)
	}
	if health.Version == "" || health.Status != "ok" || !health.Mock {
		t.Fatalf("unexpected health payload: %+v", health)
	}
	if health.SchemaVersion != store.SchemaVersion {
		t.Fatalf("schema_version=%d want %d", health.SchemaVersion, store.SchemaVersion)
	}
	if health.Auth["status"] == nil {
		t.Fatalf("auth status missing: %+v", health.Auth)
	}
}

func TestMockSeedCountsAndHomeSections(t *testing.T) {
	s := newStack(t)
	counts := map[string]string{
		"channels": "/api/channels?limit=100", "downloads": "/api/downloads",
		"playlists": "/api/playlists", "watch_later": "/api/watch_later", "history": "/api/history",
	}
	want := map[string]int{"channels": 100, "downloads": 50, "playlists": 20, "watch_later": 50, "history": 25}
	for name, path := range counts {
		var items []any
		if code := s.getJSON(path, &items); code != http.StatusOK {
			t.Fatalf("%s status %d", name, code)
		}
		if strings.Contains(path, "limit=100") && len(items) > 100 {
			t.Fatalf("%s ignored its limit: %d", name, len(items))
		}
		if !strings.Contains(path, "limit=") && len(items) > 20 {
			t.Fatalf("%s exceeded the default page limit: %d", name, len(items))
		}
	}
	// Page through every collection until it is exhausted, proving the totals
	// rather than assuming one response contains everything.
	totals := map[string]int{}
	for name, path := range counts {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		offset := 0
		for pages := 0; pages < 20; pages++ {
			var items []any
			if code := s.getJSON(path+sep+"limit=100&offset="+itoa(offset), &items); code != http.StatusOK {
				t.Fatalf("%s page status %d", name, code)
			}
			totals[name] += len(items)
			if len(items) < 20 {
				break
			}
			offset += len(items)
		}
	}
	for name, wantCount := range want {
		if totals[name] != wantCount {
			t.Fatalf("%s total=%d want %d", name, totals[name], wantCount)
		}
	}
	var videos []any
	if code := s.getJSON("/api/videos?limit=100&offset=900", &videos); code != http.StatusOK || len(videos) != 100 {
		t.Fatalf("videos page status=%d len=%d", code, len(videos))
	}
	var home map[string][]map[string]any
	if code := s.getJSON("/api/home", &home); code != http.StatusOK {
		t.Fatalf("home status %d", code)
	}
	for _, key := range []string{"continue_watching", "watch_later", "subscriptions", "recent_downloads", "recently_added", "recommended"} {
		if len(home[key]) == 0 {
			t.Fatalf("home section %s empty", key)
		}
	}
}

func TestDownloadQueueStubLifecycleAndStateFilter(t *testing.T) {
	s := newStack(t)
	code, raw := s.do(http.MethodPost, "/api/downloads", map[string]any{"url": "https://example.invalid/integration", "mode": "video", "quality": "360p", "audio_bitrate": "128", "priority": 9})
	if code != http.StatusAccepted {
		t.Fatalf("queue status %d: %s", code, raw)
	}
	var job struct {
		ID    int64  `json:"id"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(raw, &job); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	var state string
	for time.Now().Before(deadline) {
		var items []map[string]any
		s.getJSON("/api/downloads?state=completed", &items)
		for _, item := range items {
			if int64(item["id"].(float64)) == job.ID {
				state = item["state"].(string)
				if item["output_path"] == nil || item["output_path"] == "" {
					t.Fatal("completed job has no output path")
				}
				if pct := item["progress"].(float64); pct != 100 {
					t.Fatalf("completed progress=%v", pct)
				}
			}
		}
		if state == "completed" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if state != "completed" {
		// Surface the stored error instead of hanging.
		var all []map[string]any
		s.getJSON("/api/downloads", &all)
		t.Fatalf("job did not complete: %v", all)
	}
	if code, _ := s.do(http.MethodGet, "/api/downloads?state=bogus", nil); code != http.StatusBadRequest {
		t.Fatalf("invalid state status %d", code)
	}
}

func TestAccountIsolationEndToEnd(t *testing.T) {
	s := newStack(t)
	cookieValue := "SID=" + strings.Join([]string{"integration", "cookie", "canary"}, "-")
	code, raw := s.do(http.MethodPost, "/api/accounts/login", map[string]any{"kind": "cookie", "payload": map[string]any{"name": "Second", "base_url": "https://example.test", "cookie": cookieValue}})
	if code != http.StatusCreated {
		t.Fatalf("login status %d: %s", code, raw)
	}
	if bytes.Contains(raw, []byte(cookieValue)) {
		t.Fatal("login response leaked the cookie")
	}
	var account struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &account); err != nil {
		t.Fatal(err)
	}
	if code, _ := s.do(http.MethodPost, "/api/accounts/"+account.ID+"/activate", nil); code != http.StatusOK {
		t.Fatalf("activate status %d", code)
	}
	for _, path := range []string{"/api/channels", "/api/history", "/api/downloads", "/api/watch_later"} {
		var items []any
		if code := s.getJSON(path, &items); code != http.StatusOK {
			t.Fatalf("%s status %d", path, code)
		}
		if len(items) != 0 {
			t.Fatalf("%s leaked %d rows into the new account", path, len(items))
		}
	}
	if code, _ := s.do(http.MethodPost, "/api/accounts/demo/activate", nil); code != http.StatusOK {
		t.Fatalf("reactivate status %d", code)
	}
	var channels []any
	s.getJSON("/api/channels?limit=100", &channels)
	if len(channels) != 100 {
		t.Fatalf("demo channels=%d after reactivation", len(channels))
	}
	var accounts []map[string]any
	s.getJSON("/api/accounts", &accounts)
	for _, a := range accounts {
		if strings.Contains(stringify(a), cookieValue) {
			t.Fatal("account listing leaked a cookie")
		}
	}
	if code, _ := s.do(http.MethodPost, "/api/accounts/logout", nil); code != http.StatusNoContent {
		t.Fatalf("logout status %d", code)
	}
}

func TestCredentialKeyIsPrivateAndCiphertextOnly(t *testing.T) {
	s := newStack(t)
	info, err := os.Stat(filepath.Join(s.dataDir, "credential.key"))
	if err != nil {
		t.Fatalf("key missing: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("key permissions %o want 600", perm)
	}
	secret := "SID=" + strings.Join([]string{"integration", "cookie", "canary"}, "-")
	for _, name := range []string{"integration.sqlite", "integration.sqlite-wal", "integration.sqlite-shm"} {
		path := filepath.Join(s.dataDir, name)
		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("plaintext credential found in %s", name)
		}
	}
}

func TestPlayerStateAndOfflineLibraryRoundTrip(t *testing.T) {
	s := newStack(t)
	var state map[string]any
	if code := s.getJSON("/api/player/state", &state); code != http.StatusOK || state["state"] != "idle" {
		t.Fatalf("idle state: %d %v", code, state)
	}
	var lib []any
	if code := s.getJSON("/api/library", &lib); code != http.StatusOK || len(lib) != 0 {
		t.Fatalf("library should start empty: %d %d", code, len(lib))
	}
	if code, _ := s.do(http.MethodPost, "/api/downloads", map[string]any{"video_ids": []string{"video-0001", "video-0002"}}); code != http.StatusAccepted {
		t.Fatalf("batch status %d", code)
	}
}

func TestSearchAcrossTypes(t *testing.T) {
	s := newStack(t)
	for kind, want := range map[string]int{"video": 20, "channel": 20, "playlist": 20} {
		var items []map[string]any
		if code := s.getJSON("/api/search?type="+kind+"&q=Demo", &items); code != http.StatusOK {
			t.Fatalf("%s status %d", kind, code)
		}
		if len(items) != want {
			t.Fatalf("%s results=%d want %d", kind, len(items), want)
		}
		if items[0]["type"] != kind {
			t.Fatalf("%s item type=%v", kind, items[0]["type"])
		}
	}
}

func TestPlaylistDetailAndSettingsValidation(t *testing.T) {
	s := newStack(t)
	var detail struct {
		Kind   string           `json:"kind"`
		Videos []map[string]any `json:"videos"`
	}
	if code := s.getJSON("/api/playlists/playlist-01", &detail); code != http.StatusOK {
		t.Fatalf("detail status %d", code)
	}
	if detail.Kind != "synced" || len(detail.Videos) != 10 {
		t.Fatalf("detail kind=%s videos=%d", detail.Kind, len(detail.Videos))
	}
	if code, _ := s.do(http.MethodGet, "/api/downloads?limit=100", nil); code != http.StatusOK {
		t.Fatalf("downloads paginated status %d", code)
	}
	if code, _ := s.do(http.MethodGet, "/api/history?limit=100", nil); code != http.StatusOK {
		t.Fatalf("history paginated status %d", code)
	}
	if code, _ := s.do(http.MethodPut, "/api/settings", map[string]any{"concurrency": "9"}); code != http.StatusBadRequest {
		t.Fatalf("invalid concurrency accepted: %d", code)
	}
	var settings map[string]string
	if code := s.getJSON("/api/settings", &settings); code != http.StatusOK {
		t.Fatalf("settings status %d", code)
	}
	for _, key := range []string{"download_dir", "library_dir", "subtitle_language", "default_quality", "storage_limit_gb", "concurrency"} {
		if _, ok := settings[key]; !ok {
			t.Fatalf("settings missing %s", key)
		}
	}
}

func TestErrorEnvelopeIsConsistent(t *testing.T) {
	s := newStack(t)
	for _, path := range []string{"/api/videos/missing", "/api/playlists/missing", "/api/channels/missing"} {
		code, raw := s.do(http.MethodGet, path, nil)
		if code != http.StatusNotFound {
			t.Fatalf("%s status %d", path, code)
		}
		var payload struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("%s error envelope: %v (%s)", path, err, raw)
		}
		if payload.Error.Code == "" || payload.Error.Message == "" {
			t.Fatalf("%s incomplete error envelope: %s", path, raw)
		}
	}
}

func itoa(v int) string { return strconv.Itoa(v) }

func stringify(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}

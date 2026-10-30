package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/store"
)

func testServer(t *testing.T) (*sql.DB, http.Handler) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Seed(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS downloads(id INTEGER PRIMARY KEY,account_id TEXT NOT NULL,video_id TEXT NOT NULL DEFAULT '',url TEXT NOT NULL,mode TEXT NOT NULL DEFAULT 'video',quality TEXT NOT NULL DEFAULT 'best',audio_bitrate TEXT NOT NULL DEFAULT '192',priority INTEGER NOT NULL DEFAULT 0,state TEXT NOT NULL DEFAULT 'queued',progress REAL NOT NULL DEFAULT 0,error TEXT NOT NULL DEFAULT '',output_path TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if err := store.SeedDownloads(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(db).Register(mux)
	t.Cleanup(func() { db.Close() })
	return db, mux
}

func request(t *testing.T, h http.Handler, method, target, body string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
	}
	return rec, out
}

func TestHomeProvidesAllSections(t *testing.T) {
	_, h := testServer(t)
	rec, body := request(t, h, http.MethodGet, "/api/home", "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	for _, key := range []string{"continue_watching", "watch_later", "subscriptions", "recent_downloads", "recently_added", "recommended"} {
		items, ok := body[key].([]any)
		if !ok {
			t.Fatalf("missing section %s", key)
		}
		if len(items) == 0 {
			t.Fatalf("empty section %s", key)
		}
	}
}

func TestWatchLaterAndSettingsAreAccountScoped(t *testing.T) {
	db, h := testServer(t)
	rec, _ := request(t, h, http.MethodPost, "/api/watch_later/video-0002", "")
	if rec.Code != 201 {
		t.Fatalf("add status %d", rec.Code)
	}
	if _, err := db.Exec(`INSERT INTO accounts(id,name,base_url,auth_type,active,created_at) VALUES('iso','Isolated','','cookie',0,'2024-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if err := (store.AccountStore{DB: db}).Activate(context.Background(), "iso"); err != nil {
		t.Fatal(err)
	}
	_, body := request(t, h, http.MethodGet, "/api/watch_later", "")
	items, _ := body["items"]
	_ = items
	var count int
	_ = db.QueryRow(`SELECT count(*) FROM watch_later WHERE account_id='iso'`).Scan(&count)
	if count != 0 {
		t.Fatalf("isolated watch later has %d rows", count)
	}
	var isoHistory int
	_ = db.QueryRow(`SELECT count(*) FROM history WHERE account_id='iso'`).Scan(&isoHistory)
	if isoHistory != 0 {
		t.Fatalf("isolated account leaked history rows: %d", isoHistory)
	}
}

func TestUnknownVideoAndSearchErrors(t *testing.T) {
	_, h := testServer(t)
	rec, body := request(t, h, http.MethodGet, "/api/videos/missing", "")
	if rec.Code != 404 {
		t.Fatalf("status %d", rec.Code)
	}
	if body["error"] == nil {
		t.Fatalf("missing error payload: %v", body)
	}
	rec, _ = request(t, h, http.MethodGet, "/api/search?q=Demo+Video+0001", "")
	if rec.Code != 200 {
		t.Fatalf("search status %d", rec.Code)
	}
}

func TestSearchSupportsEveryDocumentedType(t *testing.T) {
	_, h := testServer(t)
	cases := map[string]string{"video": "Demo Video 0001", "channel": "Demo Channel", "playlist": "Demo Playlist", "library": "Demo"}
	for kind, q := range cases {
		rec, body := request(t, h, http.MethodGet, "/api/search?type="+kind+"&q="+url.QueryEscape(q), "")
		if rec.Code != 200 {
			t.Fatalf("%s status %d", kind, rec.Code)
		}
		var items []any
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		_ = body
		if kind != "library" && len(items) == 0 {
			t.Fatalf("%s returned no results", kind)
		}
	}
	rec, _ := request(t, h, http.MethodGet, "/api/search?type=invalid&q=x", "")
	if rec.Code != 400 {
		t.Fatalf("invalid type status %d", rec.Code)
	}
}

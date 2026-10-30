package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPlaylistDetailAndUserPlaylistLifecycle(t *testing.T) {
	_, h := testServer(t)
	// Seeded playlist detail exposes ordered videos.
	rec, body := request(t, h, http.MethodGet, "/api/playlists/playlist-01", "")
	if rec.Code != 200 {
		t.Fatalf("detail status %d: %s", rec.Code, rec.Body.String())
	}
	videos, _ := body["videos"].([]any)
	if len(videos) != 10 {
		t.Fatalf("seeded playlist videos=%d want 10", len(videos))
	}
	if body["kind"] != "synced" {
		t.Fatalf("kind=%v", body["kind"])
	}

	// Synced playlists are read-only.
	rec, _ = request(t, h, http.MethodPost, "/api/playlists/playlist-01", `{"video_id":"video-0500"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("synced write status %d", rec.Code)
	}

	// Create a user playlist.
	rec, created := request(t, h, http.MethodPost, "/api/playlists", `{"title":"My list","description":"personal"}`)
	if rec.Code != 200 {
		t.Fatalf("create status %d: %s", rec.Code, rec.Body.String())
	}
	id, _ := created["id"].(string)
	if !strings.HasPrefix(id, "user-") {
		t.Fatalf("id=%q", id)
	}

	// Add a video, then confirm ordering and duplicate protection.
	rec, _ = request(t, h, http.MethodPost, "/api/playlists/"+id, `{"video_id":"video-0500"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("add status %d: %s", rec.Code, rec.Body.String())
	}
	rec, _ = request(t, h, http.MethodPost, "/api/playlists/"+id, `{"video_id":"video-0500"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate add status %d", rec.Code)
	}
	rec, body = request(t, h, http.MethodGet, "/api/playlists/"+id, "")
	if rec.Code != 200 {
		t.Fatalf("detail after add %d", rec.Code)
	}
	var items []map[string]any
	b, _ := json.Marshal(body["videos"])
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0]["id"] != "video-0500" {
		t.Fatalf("videos=%v", items)
	}

	// Remove the video, then the playlist.
	rec, _ = request(t, h, http.MethodDelete, "/api/playlists/"+id+"/videos/video-0500", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("remove video status %d", rec.Code)
	}
	rec, _ = request(t, h, http.MethodDelete, "/api/playlists/"+id, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete playlist status %d", rec.Code)
	}
	rec, _ = request(t, h, http.MethodGet, "/api/playlists/"+id, "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted playlist still readable: %d", rec.Code)
	}
}

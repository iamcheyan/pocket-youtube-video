package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPlayerPlayPauseSeekVolumeSpeedFullscreen(t *testing.T) {
	s, _, _ := engineFixture(t, "#!/bin/sh\nexit 0\n")
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	play := httptest.NewRequest(http.MethodPost, "/api/player/play", strings.NewReader(`{"url":"av://lavfi:testsrc=size=64x64:rate=1"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, play)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("play status %d: %s", rec.Code, rec.Body.String())
	}
	defer func() {
		s.mu.Lock()
		if p := s.players["current"]; p != nil {
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			_ = p.conn.Close()
		}
		s.mu.Unlock()
	}()
	for _, action := range []string{"pause", "seek", "volume", "speed", "fullscreen"} {
		body := "{}"
		if action == "seek" {
			body = `{"value":1}`
		}
		if action == "volume" {
			body = `{"value":30}`
		}
		if action == "speed" {
			body = `{"value":1.5}`
		}
		req := httptest.NewRequest(http.MethodPost, "/api/player/"+action, strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status %d: %s", action, rec.Code, rec.Body.String())
		}
		time.Sleep(60 * time.Millisecond)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/player/state", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("state status %d", rec.Code)
	}
	state := rec.Body.String()
	for _, want := range []string{"volume", "speed", "pause"} {
		if !strings.Contains(state, want) {
			t.Fatalf("state missing %s: %s", want, state)
		}
	}
	req = httptest.NewRequest(http.MethodPost, "/api/player/stop", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("stop status %d", rec.Code)
	}
	_ = context.Background()
}

func TestPlayerStateRepeatedPollsUsePersistedIPCReader(t *testing.T) {
	s, _, _ := engineFixture(t, "#!/bin/sh\nexit 0\n")
	mux := http.NewServeMux()
	RegisterRoutes(mux, s)
	play := httptest.NewRequest(http.MethodPost, "/api/player/play", strings.NewReader(`{"url":"av://lavfi:testsrc=size=64x64:rate=1"}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, play)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("play %d", rec.Code)
	}
	defer func() {
		s.mu.Lock()
		if p := s.players["current"]; p != nil {
			if p.cmd.Process != nil {
				_ = p.cmd.Process.Kill()
			}
			_ = p.conn.Close()
		}
		s.mu.Unlock()
	}()
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/player/state", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("poll %d status %d", i, rec.Code)
		}
		if !strings.Contains(rec.Body.String(), `"pause"`) {
			t.Fatalf("poll %d missing pause: %s", i, rec.Body.String())
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/player/stop", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("stop status %d", rec.Code)
	}
}

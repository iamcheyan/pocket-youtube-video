package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/api"
	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/auth"
	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/media"
	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/store"
)

const version = "0.1.0"

func main() {
	if err := run(); err != nil {
		log.Printf("videod stopped: %v", err)
		os.Exit(1)
	}
}

func run() error {
	dataDir := os.Getenv("POCKET_YOUTUBE_DATA_DIR")
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataDir = filepath.Join(home, ".local", "share", "pocket-youtube-video")
	}
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	dbPath := os.Getenv("POCKET_YOUTUBE_DB")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "videod.sqlite")
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if os.Getenv("POCKET_YOUTUBE_SEED") != "0" {
		if err := store.Seed(context.Background(), db); err != nil {
			return err
		}
	}

	mux := http.NewServeMux()
	mediaConfig := media.Config{DownloadDir: filepath.Join(dataDir, "downloads"), IPCDir: filepath.Join(dataDir, "run"), Headless: true}
	apiHandler := api.New(db)
	apiHandler.SetHealth(func(ctx context.Context) (map[string]any, error) {
		probes := media.Probe(ctx, mediaConfig)
		return map[string]any{"version": version, "tools": probes, "yt_dlp": probes["yt-dlp"], "ffmpeg": probes["ffmpeg"], "mock": true, "database": "sqlite"}, nil
	})
	apiHandler.Register(mux)
	keyPath := os.Getenv("POCKET_YOUTUBE_AUTH_KEY")
	if keyPath == "" {
		keyPath = filepath.Join(dataDir, "credential.key")
	}
	credentials, err := auth.NewFileCredentialStore(keyPath)
	if err != nil {
		return err
	}
	authConfig := auth.OAuthConfig{ClientID: os.Getenv("GOOGLE_OAUTH_CLIENT_ID"), ClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET")}
	auth.NewHandler(db, credentials, authConfig, auth.GoogleTokenExchanger{ClientID: authConfig.ClientID, ClientSecret: authConfig.ClientSecret}).RegisterRoutes(mux)
	mediaService, err := media.New(db, mediaConfig)
	if err != nil {
		return err
	}
	defer mediaService.Close()
	media.RegisterRoutes(mux, mediaService)
	if err := store.SeedDownloads(context.Background(), db); err != nil {
		return err
	}

	addr := os.Getenv("POCKET_YOUTUBE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8732"
	}
	server := &http.Server{Addr: addr, Handler: cors(mux), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() {
		log.Printf("videod %s listening on %s; mock=%t", version, addr, true)
		errCh <- server.ListenAndServe()
	}()
	select {
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Range")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

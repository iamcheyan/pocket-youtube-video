package media

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"

	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/store"
)

type Config struct {
	YtDlpPath, MpvPath, DownloadDir, IPCDir string
	Concurrency                             int
	Headless                                bool
}

type Service struct {
	db             *sql.DB
	cfg            Config
	mu             sync.Mutex
	jobs           map[int64]*job
	players        map[string]*player
	paused, closed bool
	nextRequestID  uint64
}

type job struct {
	ID           int64   `json:"id"`
	AccountID    string  `json:"account_id"`
	VideoID      string  `json:"video_id,omitempty"`
	URL          string  `json:"url,omitempty"`
	Mode         string  `json:"mode"`
	Quality      string  `json:"quality"`
	AudioBitrate string  `json:"audio_bitrate"`
	Priority     int     `json:"priority"`
	State        string  `json:"state"`
	Progress     float64 `json:"progress"`
	Error        string  `json:"error,omitempty"`
	OutputPath   string  `json:"output_path,omitempty"`
	cmd          *exec.Cmd
	cancel       context.CancelFunc
}

type player struct {
	cmd       *exec.Cmd
	conn      net.Conn
	reader    *bufio.Reader
	mu        sync.Mutex
	requestID uint64
	videoID   string
}

const schema = `
CREATE TABLE IF NOT EXISTS downloads (
 id INTEGER PRIMARY KEY, account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 video_id TEXT NOT NULL DEFAULT '', url TEXT NOT NULL, mode TEXT NOT NULL DEFAULT 'video',
 quality TEXT NOT NULL DEFAULT 'best', audio_bitrate TEXT NOT NULL DEFAULT '192',
 priority INTEGER NOT NULL DEFAULT 0, state TEXT NOT NULL DEFAULT 'queued',
 progress REAL NOT NULL DEFAULT 0, error TEXT NOT NULL DEFAULT '', output_path TEXT NOT NULL DEFAULT '',
 created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS downloads_account_state ON downloads(account_id,state,priority DESC,created_at);
CREATE TABLE IF NOT EXISTS media_settings(account_id TEXT NOT NULL DEFAULT 'demo', key TEXT NOT NULL, value TEXT NOT NULL, PRIMARY KEY(account_id,key));
`

var progressPattern = regexp.MustCompile(`\[download\]\s+([0-9]+(?:\.[0-9]+)?)%`)

func New(db *sql.DB, cfg Config) (*Service, error) {
	if db == nil {
		return nil, errors.New("media: nil database")
	}
	if cfg.YtDlpPath == "" {
		cfg.YtDlpPath = "yt-dlp"
	}
	if cfg.MpvPath == "" {
		cfg.MpvPath = "mpv"
	}
	if cfg.DownloadDir == "" {
		cfg.DownloadDir = "downloads"
	}
	if cfg.IPCDir == "" {
		cfg.IPCDir = os.TempDir()
	}
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 2
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.IPCDir, 0700); err != nil {
		return nil, err
	}
	s := &Service{db: db, cfg: cfg, jobs: make(map[int64]*job), players: make(map[string]*player)}
	if err := s.restoreJobs(context.Background()); err != nil {
		return nil, err
	}
	go s.kick()
	return s, nil
}

func (s *Service) restoreJobs(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id,account_id,video_id,url,mode,quality,audio_bitrate,priority,progress,error FROM downloads WHERE state IN ('queued','downloading','paused')`)
	if err != nil {
		return err
	}
	var restored []*job
	for rows.Next() {
		j := &job{}
		if err := rows.Scan(&j.ID, &j.AccountID, &j.VideoID, &j.URL, &j.Mode, &j.Quality, &j.AudioBitrate, &j.Priority, &j.Progress, &j.Error); err != nil {
			rows.Close()
			return err
		}
		if strings.HasPrefix(j.URL, "https://example.test/") {
			continue
		}
		j.State = "queued"
		restored = append(restored, j)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, j := range restored {
		s.jobs[j.ID] = j
		if _, err = s.db.ExecContext(ctx, `UPDATE downloads SET state='queued' WHERE id=?`, j.ID); err != nil {
			return err
		}
	}
	return nil
}

func RegisterRoutes(mux *http.ServeMux, s *Service) {
	mux.HandleFunc("/api/downloads", s.downloads)
	mux.HandleFunc("/api/downloads/", s.downloadAction)
	mux.HandleFunc("PUT /api/downloads/order", s.downloadOrder)
	mux.HandleFunc("/api/player/", s.playerAction)
	mux.HandleFunc("GET /api/player/state", s.playerState)
	mux.HandleFunc("/api/settings", s.settings)
	mux.HandleFunc("GET /api/library", s.library)
	mux.HandleFunc("DELETE /api/library/{video_id}", s.deleteLibraryVideo)
	mux.HandleFunc("GET /api/media/{video_id}", s.mediaStream)
	mux.HandleFunc("/api/history", s.history)
}

func (s *Service) downloads(w http.ResponseWriter, r *http.Request) {
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet:
		rows, err := s.db.QueryContext(r.Context(), `SELECT id,account_id,video_id,url,mode,quality,audio_bitrate,priority,state,progress,error,output_path FROM downloads WHERE account_id=? ORDER BY priority DESC,created_at,id`, account)
		if err != nil {
			apiError(w, 500, "database_error", "unable to read downloads")
			return
		}
		defer rows.Close()
		out := []job{}
		for rows.Next() {
			var j job
			if err := rows.Scan(&j.ID, &j.AccountID, &j.VideoID, &j.URL, &j.Mode, &j.Quality, &j.AudioBitrate, &j.Priority, &j.State, &j.Progress, &j.Error, &j.OutputPath); err != nil {
				apiError(w, 500, "database_error", "unable to read downloads")
				return
			}
			out = append(out, j)
		}
		if rows.Err() != nil {
			apiError(w, 500, "database_error", "unable to read downloads")
			return
		}
		writeJSON(w, 200, out)
	case http.MethodPost:
		var in struct {
			VideoID      string `json:"video_id"`
			URL          string `json:"url"`
			Mode         string `json:"mode"`
			Quality      string `json:"quality"`
			AudioBitrate string `json:"audio_bitrate"`
			Priority     int    `json:"priority"`
		}
		if decodeJSON(r, &in) != nil {
			apiError(w, 400, "invalid_request", "invalid download request")
			return
		}
		if in.Mode == "" {
			in.Mode = "video"
		}
		if in.Quality == "" {
			in.Quality = "720p"
			var configured string
			if err := s.db.QueryRowContext(r.Context(), `SELECT value FROM settings WHERE account_id=? AND key='default_quality'`, account).Scan(&configured); err == nil && oneOf(configured, "360p", "720p", "1080p", "1440p", "2160p") {
				in.Quality = configured
			}
		}
		if in.AudioBitrate == "" {
			in.AudioBitrate = "192"
		}
		if !oneOf(in.Mode, "audio", "video", "both") || !oneOf(in.Quality, "360p", "720p", "1080p", "1440p", "2160p") || !oneOf(in.AudioBitrate, "128", "192", "320") {
			apiError(w, 400, "invalid_options", "unsupported mode, quality, or audio bitrate")
			return
		}
		if in.URL == "" {
			if in.VideoID == "" {
				apiError(w, 400, "video_required", "video_id is required")
				return
			}
			err = s.db.QueryRowContext(r.Context(), `SELECT url FROM videos WHERE id=? AND account_id=?`, in.VideoID, account).Scan(&in.URL)
			if errors.Is(err, sql.ErrNoRows) {
				apiError(w, 404, "video_not_found", "video not found for active account")
				return
			}
			if err != nil {
				apiError(w, 500, "database_error", "unable to resolve video")
				return
			}
		}
		if in.VideoID == "" {
			in.VideoID = mediaIDFromURL(in.URL)
		}
		res, err := s.db.ExecContext(r.Context(), `INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority) VALUES(?,?,?,?,?,?,?)`, account, in.VideoID, in.URL, in.Mode, in.Quality, in.AudioBitrate, in.Priority)
		if err != nil {
			apiError(w, 500, "database_error", "unable to queue download")
			return
		}
		id, _ := res.LastInsertId()
		j := &job{ID: id, AccountID: account, VideoID: in.VideoID, URL: in.URL, Mode: in.Mode, Quality: in.Quality, AudioBitrate: in.AudioBitrate, Priority: in.Priority, State: "queued"}
		s.mu.Lock()
		s.jobs[id] = j
		s.mu.Unlock()
		s.kick()
		writeJSON(w, http.StatusAccepted, j)
	default:
		apiError(w, 405, "method_not_allowed", "method not allowed")
	}
}

func (s *Service) Close() {
	s.mu.Lock()
	s.closed = true
	for _, j := range s.jobs {
		if j.cancel != nil {
			j.cancel()
		}
		if j.cmd != nil && j.cmd.Process != nil {
			_ = j.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	for _, p := range s.players {
		if p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
		}
		if p.conn != nil {
			_ = p.conn.Close()
		}
	}
	s.mu.Unlock()
}
func (s *Service) YtDlp() YtDlpEngine { return YtDlpEngine{Path: s.cfg.YtDlpPath} }
func (s *Service) Mpv() MpvEngine     { return MpvEngine{Path: s.cfg.MpvPath, Headless: s.cfg.Headless} }

type YtDlpEngine struct{ Path string }
type MpvEngine struct {
	Path     string
	Headless bool
}

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	d.DisallowUnknownFields()
	return d.Decode(v)
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

// mediaIDFromURL extracts the YouTube video ID, or preserves an opaque id for other URLs.
func mediaIDFromURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err == nil {
		if id := parsed.Query().Get("v"); id != "" {
			return id
		}
		if strings.Contains(parsed.Host, "youtu.be") {
			if id := strings.Trim(parsed.Path, "/"); id != "" {
				return id
			}
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) >= 2 && (parts[0] == "shorts" || parts[0] == "embed" || parts[0] == "live") {
			return parts[1]
		}
	}
	return strings.TrimSpace(raw)
}

func oneOf(v string, choices ...string) bool {
	for _, c := range choices {
		if v == c {
			return true
		}
	}
	return false
}

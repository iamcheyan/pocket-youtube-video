package media

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

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

func (s *Service) concurrency(accountID string) int {
	limit := s.cfg.Concurrency
	var configured string
	if err := s.db.QueryRow(`SELECT value FROM settings WHERE account_id=? AND key='concurrency'`, accountID).Scan(&configured); err == nil {
		if n, err := strconv.Atoi(configured); err == nil && n >= 1 && n <= 8 {
			limit = n
		}
	}
	return limit
}

func (s *Service) kick() {
	s.mu.Lock()
	if s.paused || s.closed {
		s.mu.Unlock()
		return
	}
	running := 0
	queued := []*job{}
	for _, j := range s.jobs {
		if j.State == "downloading" {
			running++
		} else if j.State == "queued" {
			queued = append(queued, j)
		}
	}
	sort.Slice(queued, func(i, j int) bool {
		if queued[i].Priority == queued[j].Priority {
			return queued[i].ID < queued[j].ID
		}
		return queued[i].Priority > queued[j].Priority
	})
	limit := s.cfg.Concurrency
	if len(queued) > 0 {
		limit = s.concurrency(queued[0].AccountID)
	}
	capacity := limit - running
	if capacity > len(queued) {
		capacity = len(queued)
	}
	if capacity < 0 {
		capacity = 0
	}
	selected := append([]*job(nil), queued[:max(capacity, 0)]...)
	s.mu.Unlock()
	for _, j := range selected {
		s.start(j)
	}
}
func (s *Service) start(j *job) {
	s.mu.Lock()
	if s.closed || s.paused || j.State != "queued" {
		s.mu.Unlock()
		return
	}
	running := 0
	for _, x := range s.jobs {
		if x.State == "downloading" {
			running++
		}
	}
	if running >= s.concurrency(j.AccountID) {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	j.cancel = cancel
	j.State = "downloading"
	_ = s.persistLocked(j)
	s.mu.Unlock()
	go s.run(ctx, j)
}
func (s *Service) run(ctx context.Context, j *job) {
	if err := os.MkdirAll(s.cfg.DownloadDir, 0750); err != nil {
		s.finish(j, "failed", err)
		return
	}
	format := j.Quality
	switch j.Mode {
	case "audio":
		format = "bestaudio[abr<=" + j.AudioBitrate + "]"
	case "both":
		format = "bestvideo[height<=" + strings.TrimSuffix(j.Quality, "p") + "]+bestaudio/best"
	default:
		height := strings.TrimSuffix(j.Quality, "p")
		format = "bestvideo[height<=" + height + "]+bestaudio/best[height<=" + height + "]/best"
	}
	template := filepath.Join(s.cfg.DownloadDir, "%(id)s.%(ext)s")
	args := []string{"--newline", "--no-warnings", "-f", format, "-o", template, "--print-json", "--print", "after_move:filepath", "--continue", j.URL}
	cmd := exec.Command(s.cfg.YtDlpPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		s.finish(j, "failed", err)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		s.finish(j, "failed", err)
		return
	}
	if err = cmd.Start(); err != nil {
		s.finish(j, "failed", err)
		return
	}
	s.mu.Lock()
	j.cmd = cmd
	s.mu.Unlock()
	stderrDone := make(chan struct{})
	go func() { defer close(stderrDone); _, _ = io.Copy(io.Discard, stderr) }()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 2<<20)
	var outputPath string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if m := progressPattern.FindStringSubmatch(line); len(m) == 2 {
			if v, e := strconv.ParseFloat(m[1], 64); e == nil {
				s.mu.Lock()
				j.Progress = v
				_ = s.persistLocked(j)
				s.mu.Unlock()
			}
			continue
		}
		if strings.HasPrefix(line, "/") {
			outputPath = line
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	<-stderrDone
	s.mu.Lock()
	j.cmd = nil
	s.mu.Unlock()
	if ctx.Err() != nil {
		s.mu.Lock()
		if j.State == "downloading" {
			j.State = "queued"
		}
		_ = s.persistLocked(j)
		s.mu.Unlock()
		s.kick()
		return
	}
	if scanErr != nil {
		s.finish(j, "failed", scanErr)
		return
	}
	if waitErr != nil {
		s.finish(j, "failed", errors.New("yt-dlp download failed"))
		return
	}
	j.OutputPath = outputPath
	j.Progress = 100
	s.mu.Lock()
	j.State = "completed"
	_ = s.persistLocked(j)
	s.mu.Unlock()
	s.kick()
}
func (s *Service) finish(j *job, state string, err error) {
	s.mu.Lock()
	j.State = state
	if err != nil {
		j.Error = err.Error()
	}
	j.cmd = nil
	_ = s.persistLocked(j)
	s.mu.Unlock()
	s.kick()
}
func (s *Service) persistLocked(j *job) error {
	_, err := s.db.Exec(`UPDATE downloads SET state=?,progress=?,error=?,output_path=? WHERE id=? AND account_id=?`, j.State, j.Progress, j.Error, j.OutputPath, j.ID, j.AccountID)
	return err
}

func (s *Service) downloadAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		apiError(w, 404, "not_found", "download action not found")
		return
	}
	action := parts[2]
	if action == "pause" || action == "resume" {
		s.globalDownloadAction(w, r, action)
		return
	}
	id, err := strconv.ParseInt(action, 10, 64)
	if err != nil {
		apiError(w, 400, "invalid_id", "invalid download id")
		return
	}
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	s.mu.Lock()
	j := s.jobs[id]
	if j == nil || j.AccountID != account {
		s.mu.Unlock()
		apiError(w, 404, "download_not_found", "download not found")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		s.cancelJobLocked(j)
		j.State = "cancelled"
		_ = s.persistLocked(j)
		s.mu.Unlock()
		writeJSON(w, 200, j)
	case http.MethodPost:
		var in struct {
			Action   string `json:"action"`
			Priority int    `json:"priority"`
		}
		if decodeJSON(r, &in) != nil {
			s.mu.Unlock()
			apiError(w, 400, "invalid_request", "invalid action request")
			return
		}
		switch in.Action {
		case "pause":
			if j.cmd != nil && j.cmd.Process != nil {
				_ = j.cmd.Process.Signal(syscall.SIGSTOP)
			}
			j.State = "paused"
		case "resume":
			if j.State != "paused" {
				s.mu.Unlock()
				apiError(w, 409, "invalid_state", "download is not paused")
				return
			}
			if j.cmd != nil && j.cmd.Process != nil {
				_ = j.cmd.Process.Signal(syscall.SIGCONT)
			}
			j.State = "downloading"
		case "retry":
			if j.State != "failed" && j.State != "cancelled" {
				s.mu.Unlock()
				apiError(w, 409, "invalid_state", "only failed or cancelled downloads can retry")
				return
			}
			j.State = "queued"
			j.Progress = 0
			j.Error = ""
		case "cancel":
			s.cancelJobLocked(j)
			j.State = "cancelled"
		case "priority":
			j.Priority = in.Priority
		default:
			s.mu.Unlock()
			apiError(w, 400, "unknown_action", "unknown download action")
			return
		}
		_ = s.persistLocked(j)
		s.mu.Unlock()
		if in.Action == "retry" || in.Action == "resume" {
			s.kick()
		}
		writeJSON(w, 200, j)
	default:
		s.mu.Unlock()
		apiError(w, 405, "method_not_allowed", "method not allowed")
	}
}
func (s *Service) cancelJobLocked(j *job) {
	if j.cmd != nil && j.cmd.Process != nil {
		_ = j.cmd.Process.Signal(syscall.SIGTERM)
	}
	if j.cancel != nil {
		j.cancel()
	}
}
func (s *Service) globalDownloadAction(w http.ResponseWriter, r *http.Request, action string) {
	if r.Method != http.MethodPost {
		apiError(w, 405, "method_not_allowed", "method not allowed")
		return
	}
	s.mu.Lock()
	s.paused = action == "pause"
	for _, j := range s.jobs {
		if action == "pause" && j.State == "downloading" {
			if j.cmd != nil && j.cmd.Process != nil {
				_ = j.cmd.Process.Signal(syscall.SIGSTOP)
			}
			j.State = "paused"
			_ = s.persistLocked(j)
		} else if action == "resume" && j.State == "paused" {
			if j.cmd != nil && j.cmd.Process != nil {
				_ = j.cmd.Process.Signal(syscall.SIGCONT)
			}
			j.State = "downloading"
			_ = s.persistLocked(j)
		}
	}
	s.mu.Unlock()
	if action == "resume" {
		s.kick()
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Service) downloadOrder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if decodeJSON(r, &in) != nil {
		apiError(w, 400, "invalid_request", "ids array required")
		return
	}
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		apiError(w, 500, "database_error", "unable to reorder downloads")
		return
	}
	defer tx.Rollback()
	for rank, id := range in.IDs {
		if j := s.jobs[id]; j != nil && j.AccountID == account {
			j.Priority = len(in.IDs) - rank
			_, err = tx.ExecContext(r.Context(), `UPDATE downloads SET priority=? WHERE id=? AND account_id=?`, j.Priority, id, account)
			if err != nil {
				apiError(w, 500, "database_error", "unable to reorder downloads")
				return
			}
		}
	}
	if err = tx.Commit(); err != nil {
		apiError(w, 500, "database_error", "unable to reorder downloads")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// settingDefaults are the account-level preferences the application understands.
var settingDefaults = map[string]string{
	"theme": "dark", "language": "en", "profile": "desktop", "scale": "100",
	"download_dir": "", "library_dir": "", "subtitle_language": "off",
	"default_quality": "720p", "storage_limit_gb": "20", "concurrency": "2",
}

func (s *Service) settings(w http.ResponseWriter, r *http.Request) {
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	if r.Method == http.MethodGet {
		rows, err := s.db.QueryContext(r.Context(), `SELECT key,value FROM settings WHERE account_id=? UNION ALL SELECT key,value FROM media_settings WHERE account_id=?`, account, account)
		if err != nil {
			apiError(w, 500, "database_error", "unable to read settings")
			return
		}
		defer rows.Close()
		m := map[string]string{}
		for k, v := range settingDefaults {
			m[k] = v
		}
		for rows.Next() {
			var k, v string
			if err := rows.Scan(&k, &v); err != nil {
				apiError(w, 500, "database_error", "unable to read settings")
				return
			}
			m[k] = v
		}
		if m["download_dir"] == "" {
			m["download_dir"] = s.cfg.DownloadDir
		}
		if m["library_dir"] == "" {
			m["library_dir"] = s.cfg.DownloadDir
		}
		writeJSON(w, 200, m)
		return
	}
	if r.Method != http.MethodPut {
		apiError(w, 405, "method_not_allowed", "method not allowed")
		return
	}
	var values map[string]string
	if decodeJSON(r, &values) != nil {
		apiError(w, 400, "invalid_request", "settings must be a string map")
		return
	}
	for k, v := range values {
		switch k {
		case "theme":
			if !oneOf(v, "light", "dark", "amoled", "system") {
				apiError(w, 400, "invalid_setting", "unsupported theme")
				return
			}
		case "language":
			if !oneOf(v, "en", "zh-CN", "ja") {
				apiError(w, 400, "invalid_setting", "unsupported language")
				return
			}
		case "profile":
			if !oneOf(v, "desktop", "tv", "handheld", "compact") {
				apiError(w, 400, "invalid_setting", "unsupported profile")
				return
			}
		case "scale":
			if n, e := strconv.Atoi(v); e != nil || n < 100 || n > 300 {
				apiError(w, 400, "invalid_setting", "scale must be 100-300")
				return
			}
		case "default_quality":
			if !oneOf(v, "360p", "720p", "1080p", "1440p", "2160p") {
				apiError(w, 400, "invalid_setting", "unsupported quality")
				return
			}
		case "subtitle_language":
			if !oneOf(v, "off", "en", "zh-Hans", "ja") {
				apiError(w, 400, "invalid_setting", "unsupported subtitle preference")
				return
			}
		case "concurrency":
			if n, e := strconv.Atoi(v); e != nil || n < 1 || n > 8 {
				apiError(w, 400, "invalid_setting", "concurrency must be 1-8")
				return
			}
		case "storage_limit_gb":
			if n, e := strconv.Atoi(v); e != nil || n < 1 {
				apiError(w, 400, "invalid_setting", "storage limit must be positive")
				return
			}
		case "download_dir", "library_dir":
			if strings.Contains(v, "..") {
				apiError(w, 400, "invalid_setting", "directory must not escape the data directory")
				return
			}
		default:
			apiError(w, 400, "invalid_setting", "unknown setting key")
			return
		}
	}
	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		apiError(w, 500, "database_error", "unable to update settings")
		return
	}
	defer tx.Rollback()
	for k, v := range values {
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO settings(account_id,key,value) VALUES(?,?,?) ON CONFLICT(account_id,key) DO UPDATE SET value=excluded.value`, account, k, v); err != nil {
			apiError(w, 500, "database_error", "unable to update settings")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		apiError(w, 500, "database_error", "unable to update settings")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Service) history(w http.ResponseWriter, r *http.Request) {
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	if r.Method == http.MethodDelete {
		if _, err = s.db.ExecContext(r.Context(), `DELETE FROM history WHERE account_id=?`, account); err != nil {
			apiError(w, 500, "database_error", "unable to clear history")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodPost {
		var in struct {
			VideoID  string  `json:"video_id"`
			Position float64 `json:"position_sec"`
			Duration float64 `json:"duration_sec"`
		}
		if decodeJSON(r, &in) != nil || in.VideoID == "" {
			apiError(w, 400, "invalid_request", "video_id and progress are required")
			return
		}
		res, err := s.db.ExecContext(r.Context(), `INSERT INTO history(account_id,video_id,watched_at,position) SELECT ?,id,CURRENT_TIMESTAMP,? FROM videos WHERE id=? AND account_id=? ON CONFLICT(account_id,video_id) DO UPDATE SET watched_at=CURRENT_TIMESTAMP,position=excluded.position`, account, in.Position, in.VideoID, account)
		if err != nil {
			apiError(w, 500, "database_error", "unable to save history")
			return
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			apiError(w, 404, "video_not_found", "video not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	rows, err := s.db.QueryContext(r.Context(), `SELECT h.video_id,h.watched_at,h.position,v.duration FROM history h JOIN videos v ON v.id=h.video_id AND v.account_id=h.account_id WHERE h.account_id=? ORDER BY h.watched_at DESC`, account)
	if err != nil {
		apiError(w, 500, "database_error", "unable to read history")
		return
	}
	defer rows.Close()
	type entry struct {
		VideoID   string  `json:"video_id"`
		WatchedAt string  `json:"watched_at"`
		Position  float64 `json:"position_sec"`
		Duration  float64 `json:"duration_sec"`
		Percent   float64 `json:"completion_percent"`
	}
	out := []entry{}
	for rows.Next() {
		var x entry
		if err := rows.Scan(&x.VideoID, &x.WatchedAt, &x.Position, &x.Duration); err != nil {
			apiError(w, 500, "database_error", "unable to read history")
			return
		}
		if x.Duration > 0 {
			x.Percent = x.Position / x.Duration * 100
		}
		out = append(out, x)
	}
	writeJSON(w, 200, out)
}

func (s *Service) library(w http.ResponseWriter, r *http.Request) {
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	filter := r.URL.Query().Get("filter")
	sortBy := r.URL.Query().Get("sort")
	order := "d.created_at DESC"
	if sortBy == "title" {
		order = "COALESCE(v.title,d.video_id) COLLATE NOCASE ASC"
	} else if sortBy == "duration" {
		order = "COALESCE(v.duration,0) DESC"
	}
	base := `SELECT COALESCE(v.id,d.video_id),d.account_id,COALESCE(v.channel_id,''),COALESCE(v.title,d.video_id),COALESCE(v.url,d.url),COALESCE(v.thumbnail,''),COALESCE(v.duration,0),COALESCE(v.published_at,''),COALESCE(v.description,''),d.mode,d.quality,d.output_path FROM downloads d LEFT JOIN videos v ON v.id=d.video_id AND v.account_id=d.account_id WHERE d.account_id=? AND d.state='completed' AND d.output_path<>''`
	args := []any{account}
	if q != "" {
		base += ` AND (COALESCE(v.title,d.video_id) LIKE ? OR COALESCE(v.description,'') LIKE ?)`
		args = append(args, "%"+q+"%", "%"+q+"%")
	}
	if filter != "" {
		base += ` AND d.mode=?`
		args = append(args, filter)
	}
	rows, err := s.db.QueryContext(r.Context(), base+" ORDER BY "+order, args...)
	if err != nil {
		apiError(w, 500, "database_error", "unable to read offline library")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, aid, cid, title, url, thumb, pub, desc, mode, quality, path string
		var duration int
		if err := rows.Scan(&id, &aid, &cid, &title, &url, &thumb, &duration, &pub, &desc, &mode, &quality, &path); err != nil {
			apiError(w, 500, "database_error", "unable to read offline library")
			return
		}
		out = append(out, map[string]any{"id": id, "account_id": aid, "channel_id": cid, "title": title, "url": url, "thumbnail": thumb, "duration": duration, "published_at": pub, "description": desc, "mode": mode, "quality": quality, "path": path})
	}
	if err := rows.Err(); err != nil {
		apiError(w, 500, "database_error", "unable to read offline library")
		return
	}
	writeJSON(w, 200, out)
}
func (s *Service) deleteLibraryVideo(w http.ResponseWriter, r *http.Request) {
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	id := r.PathValue("video_id")
	var path string
	err = s.db.QueryRowContext(r.Context(), `SELECT output_path FROM downloads WHERE account_id=? AND video_id=? AND state='completed' ORDER BY created_at DESC LIMIT 1`, account, id).Scan(&path)
	if errors.Is(err, sql.ErrNoRows) {
		apiError(w, 404, "media_not_found", "offline media not found")
		return
	}
	if err != nil {
		apiError(w, 500, "database_error", "unable to locate media")
		return
	}
	if strings.TrimSpace(path) == "" {
		apiError(w, http.StatusConflict, "no_media_file", "download record has no local media file")
		return
	}
	root, err := filepath.Abs(s.cfg.DownloadDir)
	if err != nil {
		apiError(w, 500, "storage_error", "storage unavailable")
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil || !strings.HasPrefix(abs, root+string(os.PathSeparator)) {
		apiError(w, 400, "invalid_media_path", "media path outside library")
		return
	}
	if err = os.Remove(abs); err != nil && !os.IsNotExist(err) {
		apiError(w, 500, "storage_error", "unable to remove media")
		return
	}
	_, err = s.db.ExecContext(r.Context(), `DELETE FROM downloads WHERE account_id=? AND video_id=? AND state='completed'`, account, id)
	if err != nil {
		apiError(w, 500, "database_error", "unable to remove media record")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Service) mediaStream(w http.ResponseWriter, r *http.Request) {
	account, err := store.ActiveAccount(r.Context(), s.db)
	if err != nil {
		apiError(w, 500, "account_unavailable", "active account unavailable")
		return
	}
	var path string
	err = s.db.QueryRowContext(r.Context(), `SELECT output_path FROM downloads WHERE account_id=? AND video_id=? AND state='completed' AND output_path<>'' ORDER BY created_at DESC LIMIT 1`, account, r.PathValue("video_id")).Scan(&path)
	if err != nil {
		apiError(w, 404, "media_not_found", "offline media not found")
		return
	}
	f, err := os.Open(path)
	if err != nil {
		apiError(w, 404, "media_not_found", "offline media file missing")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		apiError(w, 500, "storage_error", "unable to read media")
		return
	}
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

func (s *Service) playerAction(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		apiError(w, 404, "not_found", "player action not found")
		return
	}
	action := parts[2]
	if action == "play" {
		var in struct {
			VideoID string `json:"video_id"`
			URL     string `json:"url"`
		}
		if decodeJSON(r, &in) != nil {
			apiError(w, 400, "invalid_request", "video_id or url required")
			return
		}
		if in.URL == "" {
			account, err := store.ActiveAccount(r.Context(), s.db)
			if err != nil {
				apiError(w, 500, "account_unavailable", "active account unavailable")
				return
			}
			err = s.db.QueryRowContext(r.Context(), `SELECT url FROM videos WHERE id=? AND account_id=?`, in.VideoID, account).Scan(&in.URL)
			if err != nil {
				apiError(w, 404, "video_not_found", "video not found")
				return
			}
		}
		p, err := s.launch(in.URL)
		if err != nil {
			apiError(w, 503, "player_unavailable", "unable to start mpv")
			return
		}
		p.videoID = in.VideoID
		s.mu.Lock()
		if old := s.players["current"]; old != nil && old.cmd.Process != nil {
			_ = old.cmd.Process.Signal(syscall.SIGTERM)
		}
		s.players["current"] = p
		s.mu.Unlock()
		writeJSON(w, http.StatusAccepted, map[string]any{"state": "playing", "video_id": in.VideoID})
		return
	}
	s.mu.Lock()
	p := s.players["current"]
	s.mu.Unlock()
	if p == nil {
		apiError(w, 409, "player_idle", "no video is loaded")
		return
	}
	var body struct {
		Value float64 `json:"value"`
	}
	_ = decodeJSON(r, &body)
	var cmd []any
	switch action {
	case "pause":
		cmd = []any{"cycle", "pause"}
	case "stop":
		cmd = []any{"stop"}
	case "seek":
		cmd = []any{"seek", body.Value, "absolute"}
	case "volume":
		cmd = []any{"set_property", "volume", body.Value}
	case "speed":
		cmd = []any{"set_property", "speed", body.Value}
	case "fullscreen":
		cmd = []any{"cycle", "fullscreen"}
	default:
		apiError(w, 404, "not_found", "player action not found")
		return
	}
	if _, err := p.request(map[string]any{"command": cmd}); err != nil {
		apiError(w, 502, "player_ipc_error", "mpv command failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Service) playerState(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	p := s.players["current"]
	s.mu.Unlock()
	if p == nil {
		writeJSON(w, 200, map[string]any{"state": "idle"})
		return
	}
	props := map[string]any{}
	for _, name := range []string{"time-pos", "duration", "pause", "volume", "speed", "fullscreen"} {
		res, err := p.request(map[string]any{"command": []any{"get_property", name}})
		if err != nil {
			continue
		}
		props[name] = res["data"]
	}
	props["video_id"] = p.videoID
	props["state"] = "playing"
	writeJSON(w, 200, props)
}
func (s *Service) launch(url string) (*player, error) {
	exe, err := exec.LookPath(s.cfg.MpvPath)
	if err != nil {
		return nil, err
	}
	sock := filepath.Join(s.cfg.IPCDir, fmt.Sprintf("pocket-mpv-%d.sock", time.Now().UnixNano()))
	args := []string{"--no-config", "--no-terminal", "--vo=null", "--ao=null", "--idle=no", "--input-ipc-server=" + sock, url}
	cmd := exec.Command(exe, args...)
	cmd.Env = append(os.Environ(), "DISPLAY=", "WAYLAND_DISPLAY=")
	if err = cmd.Start(); err != nil {
		return nil, err
	}
	for range 100 {
		conn, e := net.DialTimeout("unix", sock, 100*time.Millisecond)
		if e == nil {
			return &player{cmd: cmd, conn: conn, reader: bufio.NewReader(conn)}, nil
		}
		if cmd.ProcessState != nil {
			return nil, errors.New("mpv exited before IPC became ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	return nil, errors.New("mpv IPC socket unavailable")
}
func (p *player) request(v any) (map[string]any, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requestID++
	id := p.requestID
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("invalid mpv command")
	}
	m["request_id"] = id
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	_ = p.conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer p.conn.SetDeadline(time.Time{})
	if _, err = p.conn.Write(append(b, '\n')); err != nil {
		return nil, err
	}
	if p.reader == nil {
		p.reader = bufio.NewReader(p.conn)
	}
	for {
		line, err := p.reader.ReadBytes('\n')
		if err != nil {
			return nil, err
		}
		var out map[string]any
		if err = json.Unmarshal(line, &out); err != nil {
			return nil, err
		}
		if rid, ok := out["request_id"].(float64); ok && uint64(rid) == id {
			return out, nil
		}
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

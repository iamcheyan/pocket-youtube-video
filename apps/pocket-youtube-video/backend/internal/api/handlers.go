package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/store"
	"net/http"
	"strconv"
	"strings"
)

type Handler struct {
	DB          *sql.DB
	Accounts    store.AccountStore
	HealthProbe func(context.Context) (map[string]any, error)
}

func New(db *sql.DB) *Handler { return &Handler{DB: db, Accounts: store.AccountStore{DB: db}} }
func (h *Handler) SetHealth(probe func(context.Context) (map[string]any, error)) {
	h.HealthProbe = probe
}
func (h *Handler) Register(m *http.ServeMux) {
	m.HandleFunc("GET /api/health", h.health)
	m.HandleFunc("GET /api/accounts", h.accounts)
	m.HandleFunc("GET /api/channels", h.channels)
	m.HandleFunc("GET /api/channels/{id}", h.channel)
	m.HandleFunc("GET /api/videos", h.videos)
	m.HandleFunc("GET /api/videos/{id}", h.video)
	m.HandleFunc("GET /api/playlists", h.playlists)
	m.HandleFunc("GET /api/home", h.home)
	m.HandleFunc("GET /api/subscriptions", h.channels)
	m.HandleFunc("GET /api/history", h.listVideos("history"))
	m.HandleFunc("POST /api/history/{id}", h.historyAdd)
	m.HandleFunc("DELETE /api/history/{id}", h.historyDelete)
	m.HandleFunc("GET /api/watch_later", h.listVideos("watch_later"))
	m.HandleFunc("POST /api/watch_later/{id}", h.watchLaterAdd)
	m.HandleFunc("DELETE /api/watch_later/{id}", h.watchLaterDelete)
	m.HandleFunc("GET /api/favorites", h.listVideos("favorites"))
	m.HandleFunc("POST /api/favorites/{id}", h.favoriteAdd)
	m.HandleFunc("DELETE /api/favorites/{id}", h.favoriteDelete)
	m.HandleFunc("GET /api/search", h.search)
	m.HandleFunc("POST /api/sync", h.sync)
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) {
	slug := map[int]string{400: "bad_request", 404: "not_found", 405: "method_not_allowed", 409: "conflict", 500: "internal_error", 503: "unavailable"}[code]
	if slug == "" {
		slug = "request_failed"
	}
	w.WriteHeader(code)
	jsonOut(w, map[string]any{"error": map[string]string{"code": slug, "message": msg}})
}
func (h *Handler) account(r *http.Request) string {
	if id := r.URL.Query().Get("account_id"); id != "" {
		return id
	}
	id, _ := store.ActiveAccount(r.Context(), h.DB)
	return id
}
func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if e := h.DB.PingContext(r.Context()); e != nil {
		fail(w, 503, "database unavailable")
		return
	}
	var active int
	_ = h.DB.QueryRowContext(r.Context(), `SELECT count(*) FROM accounts WHERE active=1 AND auth_type NOT IN ('demo','')`).Scan(&active)
	payload := map[string]any{"status": "ok", "version": "0.1.0", "tools": map[string]any{}, "mock": true, "database": "sqlite"}
	if h.HealthProbe != nil {
		if values, err := h.HealthProbe(r.Context()); err == nil {
			for k, v := range values {
				payload[k] = v
			}
		}
	}
	payload["auth"] = map[string]any{"configured": active > 0, "status": map[bool]string{true: "configured", false: "not_configured"}[active > 0]}
	jsonOut(w, payload)
}
func (h *Handler) accounts(w http.ResponseWriter, r *http.Request) {
	v, e := h.Accounts.List(r.Context())
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, v)
}
func (h *Handler) activate(w http.ResponseWriter, r *http.Request) {
	e := h.Accounts.Activate(r.Context(), r.PathValue("id"))
	if errors.Is(e, sql.ErrNoRows) {
		fail(w, 404, "account not found")
		return
	}
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, map[string]bool{"ok": true})
}
func paging(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit < 1 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

type videoRow struct {
	ID          string `json:"id"`
	AccountID   string `json:"account_id"`
	ChannelID   string `json:"channel_id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Thumbnail   string `json:"thumbnail"`
	Duration    int    `json:"duration"`
	PublishedAt string `json:"published_at"`
	Description string `json:"description"`
}

const videoCols = `v.id,v.account_id,v.channel_id,v.title,v.url,v.thumbnail,v.duration,v.published_at,v.description`

func scanVideos(rows *sql.Rows) ([]videoRow, error) {
	out := []videoRow{}
	for rows.Next() {
		var v videoRow
		if e := rows.Scan(&v.ID, &v.AccountID, &v.ChannelID, &v.Title, &v.URL, &v.Thumbnail, &v.Duration, &v.PublishedAt, &v.Description); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (h *Handler) videos(w http.ResponseWriter, r *http.Request) {
	l, o := paging(r)
	rows, e := h.DB.QueryContext(r.Context(), `SELECT `+videoCols+` FROM videos v WHERE v.account_id=? ORDER BY v.published_at DESC,v.id LIMIT ? OFFSET ?`, h.account(r), l, o)
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	defer rows.Close()
	v, e := scanVideos(rows)
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, v)
}
func (h *Handler) video(w http.ResponseWriter, r *http.Request) {
	var v videoRow
	e := h.DB.QueryRowContext(r.Context(), `SELECT `+videoCols+` FROM videos v WHERE v.account_id=? AND v.id=?`, h.account(r), r.PathValue("id")).Scan(&v.ID, &v.AccountID, &v.ChannelID, &v.Title, &v.URL, &v.Thumbnail, &v.Duration, &v.PublishedAt, &v.Description)
	if errors.Is(e, sql.ErrNoRows) {
		fail(w, 404, "video not found")
		return
	}
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, v)
}
func (h *Handler) channels(w http.ResponseWriter, r *http.Request) {
	l, o := paging(r)
	rows, e := h.DB.QueryContext(r.Context(), `SELECT id,account_id,title,url,thumbnail,description FROM channels WHERE account_id=? ORDER BY title,id LIMIT ? OFFSET ?`, h.account(r), l, o)
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, a, t, u, th, d string
		if e = rows.Scan(&id, &a, &t, &u, &th, &d); e != nil {
			fail(w, 500, "database error")
			return
		}
		out = append(out, map[string]any{"id": id, "account_id": a, "title": t, "url": u, "thumbnail": th, "description": d})
	}
	if rows.Err() != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, out)
}
func (h *Handler) channel(w http.ResponseWriter, r *http.Request) {
	var id, a, t, u, th, d string
	e := h.DB.QueryRowContext(r.Context(), `SELECT id,account_id,title,url,thumbnail,description FROM channels WHERE account_id=? AND id=?`, h.account(r), r.PathValue("id")).Scan(&id, &a, &t, &u, &th, &d)
	if errors.Is(e, sql.ErrNoRows) {
		fail(w, 404, "channel not found")
		return
	}
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, map[string]any{"id": id, "account_id": a, "title": t, "url": u, "thumbnail": th, "description": d})
}
func (h *Handler) playlists(w http.ResponseWriter, r *http.Request) {
	l, o := paging(r)
	rows, e := h.DB.QueryContext(r.Context(), `SELECT p.id,p.account_id,p.title,p.description,COUNT(pv.video_id) FROM playlists p LEFT JOIN playlist_videos pv ON pv.playlist_id=p.id WHERE p.account_id=? GROUP BY p.id ORDER BY p.title LIMIT ? OFFSET ?`, h.account(r), l, o)
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, a, t, d string
		var n int
		if e = rows.Scan(&id, &a, &t, &d, &n); e != nil {
			fail(w, 500, "database error")
			return
		}
		out = append(out, map[string]any{"id": id, "account_id": a, "title": t, "description": d, "video_count": n})
	}
	if rows.Err() != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, out)
}
func (h *Handler) home(w http.ResponseWriter, r *http.Request) {
	a := h.account(r)
	load := func(query string, args ...any) []videoRow {
		rows, err := h.DB.QueryContext(r.Context(), query, args...)
		if err != nil {
			return []videoRow{}
		}
		defer rows.Close()
		items, err := scanVideos(rows)
		if err != nil {
			return []videoRow{}
		}
		return items
	}
	jsonOut(w, map[string]any{
		"continue_watching": load(`SELECT `+videoCols+` FROM history x JOIN videos v ON v.id=x.video_id AND v.account_id=x.account_id WHERE x.account_id=? AND x.position>0 ORDER BY x.watched_at DESC LIMIT 8`, a),
		"watch_later":       load(`SELECT `+videoCols+` FROM watch_later x JOIN videos v ON v.id=x.video_id AND v.account_id=x.account_id WHERE x.account_id=? ORDER BY x.added_at DESC LIMIT 8`, a),
		"subscriptions":     load(`SELECT `+videoCols+` FROM videos v WHERE v.account_id=? ORDER BY v.published_at DESC LIMIT 8`, a),
		"recent_downloads":  load(`SELECT `+videoCols+` FROM downloads d JOIN videos v ON v.id=d.video_id AND v.account_id=d.account_id WHERE d.account_id=? ORDER BY d.created_at DESC LIMIT 8`, a),
		"recently_added":    load(`SELECT `+videoCols+` FROM videos v WHERE v.account_id=? ORDER BY v.published_at DESC LIMIT 8`, a),
		"recommended":       load(`SELECT `+videoCols+` FROM videos v WHERE v.account_id=? ORDER BY v.id LIMIT 8`, a),
	})
}
func (h *Handler) listVideos(table string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, o := paging(r)
		sortCol := "x.added_at"
		if table == "history" {
			sortCol = "x.watched_at"
		}
		rows, e := h.DB.QueryContext(r.Context(), `SELECT `+videoCols+` FROM `+table+` x JOIN videos v ON v.id=x.video_id AND v.account_id=x.account_id WHERE x.account_id=? ORDER BY `+sortCol+` DESC LIMIT ? OFFSET ?`, h.account(r), l, o)
		if e != nil {
			fail(w, 500, "database error")
			return
		}
		defer rows.Close()
		v, e := scanVideos(rows)
		if e != nil {
			fail(w, 500, "database error")
			return
		}
		jsonOut(w, v)
	}
}
func (h *Handler) mutate(table string, add bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, id := h.account(r), r.PathValue("id")
		var e error
		if add {
			var n int
			e = h.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM videos WHERE account_id=? AND id=?`, a, id).Scan(&n)
			if e != nil || n == 0 {
				fail(w, 404, "video not found")
				return
			}
			_, e = h.DB.ExecContext(r.Context(), `INSERT OR IGNORE INTO `+table+`(account_id,video_id,added_at) VALUES(?,?,datetime('now'))`, a, id)
		} else {
			_, e = h.DB.ExecContext(r.Context(), `DELETE FROM `+table+` WHERE account_id=? AND video_id=?`, a, id)
		}
		if e != nil {
			fail(w, 500, "database error")
			return
		}
		if add {
			w.WriteHeader(201)
			jsonOut(w, map[string]bool{"ok": true})
		} else {
			w.WriteHeader(204)
		}
	}
}
func (h *Handler) watchLaterAdd(w http.ResponseWriter, r *http.Request) {
	h.mutate("watch_later", true)(w, r)
}
func (h *Handler) watchLaterDelete(w http.ResponseWriter, r *http.Request) {
	h.mutate("watch_later", false)(w, r)
}
func (h *Handler) favoriteAdd(w http.ResponseWriter, r *http.Request) {
	h.mutate("favorites", true)(w, r)
}
func (h *Handler) favoriteDelete(w http.ResponseWriter, r *http.Request) {
	h.mutate("favorites", false)(w, r)
}
func (h *Handler) historyAdd(w http.ResponseWriter, r *http.Request) {
	a, id := h.account(r), r.PathValue("id")
	res, e := h.DB.ExecContext(r.Context(), `INSERT INTO history(account_id,video_id,watched_at,position) SELECT ?,id,datetime('now'),0 FROM videos WHERE account_id=? AND id=? ON CONFLICT(account_id,video_id) DO UPDATE SET watched_at=datetime('now')`, a, a, id)
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		fail(w, 404, "video not found")
		return
	}
	w.WriteHeader(201)
	jsonOut(w, map[string]bool{"ok": true})
}
func (h *Handler) historyDelete(w http.ResponseWriter, r *http.Request) {
	_, e := h.DB.ExecContext(r.Context(), `DELETE FROM history WHERE account_id=? AND video_id=?`, h.account(r), r.PathValue("id"))
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	w.WriteHeader(204)
}
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		jsonOut(w, []videoRow{})
		return
	}
	l, o := paging(r)
	rows, e := h.DB.QueryContext(r.Context(), `SELECT `+videoCols+` FROM video_search s JOIN videos v ON v.id=s.video_id AND v.account_id=s.account_id WHERE s.account_id=? AND video_search MATCH ? ORDER BY rank LIMIT ? OFFSET ?`, h.account(r), q, l, o)
	if e != nil {
		fail(w, 400, "invalid search query")
		return
	}
	defer rows.Close()
	v, e := scanVideos(rows)
	if e != nil {
		fail(w, 500, "database error")
		return
	}
	jsonOut(w, v)
}
func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, map[string]any{"status": "queued", "account_id": h.account(r)})
}

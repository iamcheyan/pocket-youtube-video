package media

import (
	"database/sql"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/store"
)

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

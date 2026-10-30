package media

import (
	"net/http"

	"github.com/iamcheyan/pocket-youtube-video/apps/pocket-youtube-video/backend/internal/store"
)

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

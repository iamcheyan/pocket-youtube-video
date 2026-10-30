package store

import "database/sql"

func addMediaSchemaForTest(db *sql.DB) (bool, error) {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS downloads(id INTEGER PRIMARY KEY,account_id TEXT NOT NULL,video_id TEXT NOT NULL DEFAULT '',url TEXT NOT NULL,mode TEXT NOT NULL DEFAULT 'video',quality TEXT NOT NULL DEFAULT 'best',audio_bitrate TEXT NOT NULL DEFAULT '192',priority INTEGER NOT NULL DEFAULT 0,state TEXT NOT NULL DEFAULT 'queued',progress REAL NOT NULL DEFAULT 0,error TEXT NOT NULL DEFAULT '',output_path TEXT NOT NULL DEFAULT '',created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP)`)
	return true, err
}

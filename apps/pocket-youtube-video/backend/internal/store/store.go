package store

import (
	"context"
	"database/sql"
	"fmt"
	_ "modernc.org/sqlite"
	"time"
)

const schema = `PRAGMA foreign_keys=ON;
CREATE TABLE IF NOT EXISTS accounts(id TEXT PRIMARY KEY,name TEXT NOT NULL,base_url TEXT NOT NULL DEFAULT '',auth_type TEXT NOT NULL DEFAULT '',active INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS oauth_tokens(account_id TEXT PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,encrypted_access_token BLOB,encrypted_refresh_token BLOB,encrypted_cookie BLOB,encrypted_credentials BLOB,token_expires_at TEXT,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS channels(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,title TEXT NOT NULL,url TEXT NOT NULL,thumbnail TEXT NOT NULL DEFAULT '',description TEXT NOT NULL DEFAULT '',UNIQUE(account_id,url));
CREATE TABLE IF NOT EXISTS videos(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,channel_id TEXT NOT NULL REFERENCES channels(id) ON DELETE CASCADE,title TEXT NOT NULL,url TEXT NOT NULL,thumbnail TEXT NOT NULL DEFAULT '',duration INTEGER NOT NULL DEFAULT 0,published_at TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',UNIQUE(account_id,url));
CREATE INDEX IF NOT EXISTS videos_account_published ON videos(account_id,published_at DESC);
CREATE TABLE IF NOT EXISTS playlists(id TEXT PRIMARY KEY,account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,title TEXT NOT NULL,description TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS playlist_videos(playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,position INTEGER NOT NULL,PRIMARY KEY(playlist_id,video_id));
CREATE TABLE IF NOT EXISTS watch_later(account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,added_at TEXT NOT NULL,PRIMARY KEY(account_id,video_id));
CREATE TABLE IF NOT EXISTS favorites(account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,added_at TEXT NOT NULL,PRIMARY KEY(account_id,video_id));
CREATE TABLE IF NOT EXISTS history(account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,video_id TEXT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,watched_at TEXT NOT NULL,position INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(account_id,video_id));
CREATE TABLE IF NOT EXISTS settings(account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,key TEXT NOT NULL,value TEXT NOT NULL,PRIMARY KEY(account_id,key));
CREATE VIRTUAL TABLE IF NOT EXISTS video_search USING fts5(video_id UNINDEXED,account_id UNINDEXED,title,description,channel_title);
CREATE TRIGGER IF NOT EXISTS videos_search_insert AFTER INSERT ON videos BEGIN INSERT INTO video_search(video_id,account_id,title,description,channel_title) SELECT NEW.id,NEW.account_id,NEW.title,NEW.description,title FROM channels WHERE id=NEW.channel_id; END;
CREATE TRIGGER IF NOT EXISTS videos_search_update AFTER UPDATE ON videos BEGIN DELETE FROM video_search WHERE video_id=OLD.id; INSERT INTO video_search(video_id,account_id,title,description,channel_title) SELECT NEW.id,NEW.account_id,NEW.title,NEW.description,title FROM channels WHERE id=NEW.channel_id; END;
CREATE TRIGGER IF NOT EXISTS videos_search_delete AFTER DELETE ON videos BEGIN DELETE FROM video_search WHERE video_id=OLD.id; END;`

func Open(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	if err = Migrate(context.Background(), db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, schema)
	return err
}
func Seed(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO accounts(id,name,base_url,auth_type,active,created_at) VALUES('demo','Demo account','', 'demo',1,?)`, now); err != nil {
		return err
	}
	for i := range 100 {
		n := i + 1
		cid := fmt.Sprintf("channel-%03d", n)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO channels(id,account_id,title,url) VALUES(?,?,?,?)`, cid, "demo", fmt.Sprintf("Demo Channel %03d", n), "https://example.test/channel/"+cid); err != nil {
			return err
		}
	}
	for i := range 1000 {
		n := i + 1
		vid := fmt.Sprintf("video-%04d", n)
		ch := fmt.Sprintf("channel-%03d", (n-1)%100+1)
		title := fmt.Sprintf("Demo Video %04d", n)
		date := time.Date(2024, 1, 1+n%365, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO videos(id,account_id,channel_id,title,url,duration,published_at,description) VALUES(?,?,?,?,?,?,?,?)`, vid, "demo", ch, title, "https://example.test/watch/"+vid, 120+n%600, date, fmt.Sprintf("Description for %s", title)); err != nil {
			return err
		}
	}
	for i := range 20 {
		n := i + 1
		pid := fmt.Sprintf("playlist-%02d", n)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO playlists(id,account_id,title,description) VALUES(?,?,?,?)`, pid, "demo", fmt.Sprintf("Demo Playlist %02d", n), "Seeded playlist"); err != nil {
			return err
		}
		for j := range 10 {
			vid := fmt.Sprintf("video-%04d", (n-1)*10+j+1)
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO playlist_videos(playlist_id,video_id,position) VALUES(?,?,?)`, pid, vid, j); err != nil {
				return err
			}
		}
	}
	for i := range 50 {
		vid := fmt.Sprintf("video-%04d", i+1)
		if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO watch_later(account_id,video_id,added_at) VALUES('demo',?,?)`, vid, now); err != nil {
			return err
		}
		if i < 25 {
			if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO history(account_id,video_id,watched_at,position) VALUES('demo',?,?,30)`, vid, now); err != nil {
				return err
			}
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO settings(account_id,key,value) VALUES('demo','theme','dark')`)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// SeedDownloads inserts deterministic demo queue fixtures after media tables exist.
func SeedDownloads(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i := range 50 {
		vid := fmt.Sprintf("video-%04d", i+1)
		state := []string{"queued", "downloading", "paused", "completed", "failed", "cancelled"}[i%6]
		if i < 8 {
			state = "completed"
		}
		progress := 0.0
		if state == "completed" {
			progress = 100
		}
		var exists int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM downloads WHERE account_id='demo' AND video_id=?`, vid).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO downloads(account_id,video_id,url,mode,quality,audio_bitrate,priority,state,progress,output_path) VALUES('demo',?,?,?,?,?,?,?,?,?)`, vid, "https://example.test/watch/"+vid, "video", "720p", "192", 50-i, state, progress, ""); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type AccountStore struct{ DB *sql.DB }

func (s AccountStore) List(ctx context.Context) ([]map[string]any, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id,name,base_url,auth_type,active,created_at FROM accounts ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, name, url, auth, created string
		var active int
		if err := rows.Scan(&id, &name, &url, &auth, &active, &created); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": id, "name": name, "base_url": url, "auth_type": auth, "active": active != 0, "created_at": created})
	}
	return out, rows.Err()
}
func (s AccountStore) Activate(ctx context.Context, id string) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id=?`, id).Scan(&exists); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE accounts SET active=(id=?)`, id); err != nil {
		return err
	}
	return tx.Commit()
}
func ActiveAccount(ctx context.Context, db *sql.DB) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT id FROM accounts WHERE active=1 ORDER BY id LIMIT 1`).Scan(&id)
	return id, err
}

package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSeedCountsAreDeterministic(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "one.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := Seed(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	media, err := addMediaSchemaForTest(db)
	if err != nil {
		t.Fatal(err)
	}
	_ = media
	if err := SeedDownloads(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for name, query := range map[string]string{
		"channels":    `SELECT count(*) FROM channels WHERE account_id='demo'`,
		"videos":      `SELECT count(*) FROM videos WHERE account_id='demo'`,
		"playlists":   `SELECT count(*) FROM playlists WHERE account_id='demo'`,
		"downloads":   `SELECT count(*) FROM downloads WHERE account_id='demo'`,
		"watch_later": `SELECT count(*) FROM watch_later WHERE account_id='demo'`,
		"history":     `SELECT count(*) FROM history WHERE account_id='demo'`,
	} {
		var n int
		if err := db.QueryRow(query).Scan(&n); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		counts[name] = n
	}
	want := map[string]int{"channels": 100, "videos": 1000, "playlists": 20, "downloads": 50, "watch_later": 50, "history": 25}
	for k, v := range want {
		if counts[k] != v {
			t.Errorf("%s=%d want %d", k, counts[k], v)
		}
	}
	// Re-running must not duplicate any seeded row.
	if err := Seed(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := SeedDownloads(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM downloads`).Scan(&n)
	if n != 50 {
		t.Fatalf("idempotent seed downloads=%d", n)
	}
}

func TestActiveAccountIsolatesRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := Seed(ctx, db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO accounts(id,name,base_url,auth_type,active,created_at) VALUES('other','Other','','cookie',0,'2024-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO channels(id,account_id,title,url) VALUES('other-channel','other','Other channel','https://example.test/c')`); err != nil {
		t.Fatal(err)
	}
	if id, _ := ActiveAccount(ctx, db); id != "demo" {
		t.Fatalf("active=%q", id)
	}
	if err := (AccountStore{DB: db}).Activate(ctx, "other"); err != nil {
		t.Fatal(err)
	}
	if id, _ := ActiveAccount(ctx, db); id != "other" {
		t.Fatalf("active=%q", id)
	}
	var demoVideos, otherChannels int
	_ = db.QueryRow(`SELECT count(*) FROM videos WHERE account_id='demo'`).Scan(&demoVideos)
	_ = db.QueryRow(`SELECT count(*) FROM channels WHERE account_id='other'`).Scan(&otherChannels)
	if demoVideos != 1000 || otherChannels != 1 {
		t.Fatalf("isolation failed demo=%d other=%d", demoVideos, otherChannels)
	}
}

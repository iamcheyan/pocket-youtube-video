package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestMigrationsApplyOnceAndAreRecorded(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "m.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	current, applied, err := SchemaState(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if current != SchemaVersion {
		t.Fatalf("version=%d want %d", current, SchemaVersion)
	}
	if len(applied) != SchemaVersion {
		t.Fatalf("recorded %d migrations want %d", len(applied), SchemaVersion)
	}
	// Re-running is a no-op even after a restart.
	if err := Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	again, appliedAgain, err := SchemaState(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if again != SchemaVersion || len(appliedAgain) != SchemaVersion {
		t.Fatalf("re-apply changed state: %d/%d", again, len(appliedAgain))
	}
	for _, col := range []string{"duration", "position"} {
		var n int
		_ = db.QueryRow(`SELECT count(*) FROM pragma_table_info('history') WHERE name=?`, col).Scan(&n)
		if n != 1 {
			t.Fatalf("history.%s missing", col)
		}
	}
	var synced int
	_ = db.QueryRow(`SELECT count(*) FROM pragma_table_info('channels') WHERE name='synced_at'`).Scan(&synced)
	if synced != 1 {
		t.Fatal("channels.synced_at missing")
	}
}

func TestMigrationUpgradesLegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`CREATE TABLE accounts(id TEXT PRIMARY KEY,name TEXT NOT NULL,base_url TEXT NOT NULL DEFAULT '',auth_type TEXT NOT NULL DEFAULT '',active INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL);
		CREATE TABLE history(account_id TEXT NOT NULL,video_id TEXT NOT NULL,watched_at TEXT NOT NULL,position REAL NOT NULL DEFAULT 0,PRIMARY KEY(account_id,video_id));
		CREATE TABLE channels(id TEXT PRIMARY KEY,account_id TEXT NOT NULL,title TEXT NOT NULL,url TEXT NOT NULL,thumbnail TEXT NOT NULL DEFAULT '',description TEXT NOT NULL DEFAULT '',UNIQUE(account_id,url));
		CREATE TABLE playlists(id TEXT PRIMARY KEY,account_id TEXT NOT NULL,title TEXT NOT NULL,description TEXT NOT NULL DEFAULT '');
		CREATE TABLE settings(account_id TEXT NOT NULL,key TEXT NOT NULL,value TEXT NOT NULL,PRIMARY KEY(account_id,key));`); err != nil {
		t.Fatal(err)
	}
	legacy.Close()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("upgrade failed: %v", err)
	}
	defer db.Close()
	current, _, err := SchemaState(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if current != SchemaVersion {
		t.Fatalf("version=%d", current)
	}
}

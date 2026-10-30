package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

func TestChannelSearchAndSubscriptionPaging(t *testing.T) {
	_, h := testServer(t)
	rec, _ := request(t, h, http.MethodGet, "/api/channels?q="+url.QueryEscape("Channel 001"), "")
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var filtered []map[string]any
	if err := jsonUnmarshal(rec.Body.Bytes(), &filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0]["id"] != "channel-001" {
		t.Fatalf("filtered=%v", filtered)
	}

	rec, _ = request(t, h, http.MethodGet, "/api/channels?limit=10&page=1", "")
	var page1 []map[string]any
	_ = jsonUnmarshal(rec.Body.Bytes(), &page1)
	rec, _ = request(t, h, http.MethodGet, "/api/subscriptions?limit=10&page=2", "")
	var page2 []map[string]any
	_ = jsonUnmarshal(rec.Body.Bytes(), &page2)
	if len(page1) != 10 || len(page2) != 10 {
		t.Fatalf("paging lengths %d/%d", len(page1), len(page2))
	}
	if page1[0]["id"] == page2[0]["id"] {
		t.Fatal("page 2 repeated page 1")
	}
}

func TestSyncReportsHonestProviderState(t *testing.T) {
	db, h := testServer(t)
	rec, body := request(t, h, http.MethodPost, "/api/sync", `{"what":"all"}`)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if body["provider_sync"] != "not_configured" {
		t.Fatalf("provider_sync=%v", body["provider_sync"])
	}
	if body["synced_at"] == "" {
		t.Fatal("missing synced_at")
	}
	counts, _ := body["counts"].(map[string]any)
	if counts["channels"].(float64) != 100 {
		t.Fatalf("counts=%v", counts)
	}
	var stamped int
	if err := db.QueryRow(`SELECT count(*) FROM channels WHERE account_id='demo' AND synced_at<>''`).Scan(&stamped); err != nil {
		t.Fatal(err)
	}
	if stamped != 100 {
		t.Fatalf("synced_at rows=%d", stamped)
	}
	rec, _ = request(t, h, http.MethodPost, "/api/sync", `{"what":"bogus"}`)
	if rec.Code != 400 {
		t.Fatalf("bogus scope status %d", rec.Code)
	}
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

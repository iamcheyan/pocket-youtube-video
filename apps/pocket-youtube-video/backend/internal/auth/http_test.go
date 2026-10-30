package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCookieImportNeverReturnsOrStoresPlaintext(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE accounts(id TEXT PRIMARY KEY,name TEXT NOT NULL,base_url TEXT NOT NULL DEFAULT '',auth_type TEXT NOT NULL DEFAULT '',active INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL); CREATE TABLE oauth_tokens(account_id TEXT PRIMARY KEY,encrypted_access_token BLOB,encrypted_refresh_token BLOB,encrypted_cookie BLOB,encrypted_credentials BLOB,token_expires_at TEXT,updated_at TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileCredentialStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(db, store, OAuthConfig{}, GoogleTokenExchanger{})
	secret := "SID=top-secret-cookie-value"
	req := httptest.NewRequest(http.MethodPost, "/api/auth/cookie", strings.NewReader(`{"name":"Account","base_url":"https://example.test","cookie":"`+secret+`"}`))
	rec := httptest.NewRecorder()
	h.ImportCookie(rec, req)
	if rec.Code != 201 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatal("response leaked cookie")
	}
	var tokenBytes []byte
	if err := db.QueryRow(`SELECT encrypted_credentials FROM oauth_tokens LIMIT 1`).Scan(&tokenBytes); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tokenBytes), secret) {
		t.Fatal("database row leaked cookie")
	}
	var listed int
	if err := db.QueryRow(`SELECT count(*) FROM accounts WHERE name='Account'`).Scan(&listed); err != nil || listed != 1 {
		t.Fatal("account not persisted")
	}
}

func TestOAuthBeginProducesPKCEAndLoopbackCallback(t *testing.T) {
	g := &GoogleOAuth{Config: OAuthConfig{ClientID: "client"}}
	session, err := g.Begin(context.Background(), "https://www.googleapis.com/auth/youtube.readonly")
	if err != nil {
		t.Fatal(err)
	}
	defer session.Listener.Close()
	if session.State == "" || session.Verifier == "" || session.Listener == nil {
		t.Fatal("incomplete session")
	}
	if !strings.Contains(session.URL, "code_challenge_method=S256") || !strings.Contains(session.URL, "redirect_uri=http%3A%2F%2F127.0.0.1") {
		t.Fatalf("unexpected URL %s", session.URL)
	}
	// State mismatch must be rejected without exchanging a token.
	bad := httptest.NewRequest(http.MethodGet, "/oauth/callback?state=wrong&code=x", nil)
	if _, err := g.Complete(context.Background(), session, bad.URL); err == nil {
		t.Fatal("state mismatch accepted")
	}
	if _, err := json.Marshal(session); err != nil {
		t.Fatal(err)
	}
}

func TestUnifiedLoginEndpointForCookie(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "login.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE accounts(id TEXT PRIMARY KEY,name TEXT NOT NULL,base_url TEXT NOT NULL DEFAULT '',auth_type TEXT NOT NULL DEFAULT '',active INTEGER NOT NULL DEFAULT 0,created_at TEXT NOT NULL); CREATE TABLE oauth_tokens(account_id TEXT PRIMARY KEY,encrypted_access_token BLOB,encrypted_refresh_token BLOB,encrypted_cookie BLOB,encrypted_credentials BLOB,token_expires_at TEXT,updated_at TEXT NOT NULL);`); err != nil {
		t.Fatal(err)
	}
	store, err := NewFileCredentialStore(filepath.Join(dir, "key"))
	if err != nil {
		t.Fatal(err)
	}
	h := NewHandler(db, store, OAuthConfig{}, GoogleTokenExchanger{})
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/accounts/login", strings.NewReader(`{"kind":"cookie","payload":{"name":"L","base_url":"https://example.test","cookie":"SID={{SECRET_x42qm9ad}}","scopes":[]}}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "{{SECRET_7o0pm2fs}}") {
		t.Fatal("login leaked cookie")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/accounts/login", strings.NewReader(`{"kind":"nope","payload":{}}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid kind status %d", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["error"].(map[string]any); !ok {
		t.Fatalf("error envelope missing: %v", body)
	}

	// OAuth kind without a client id still returns a protocol-shaped result or a clear error, never a fake success.
	req = httptest.NewRequest(http.MethodPost, "/api/accounts/login", strings.NewReader(`{"kind":"oauth","payload":{"name":"O","base_url":"https://example.test","scopes":["https://www.googleapis.com/auth/youtube.readonly"]}}`))
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted && rec.Code != http.StatusBadRequest && rec.Code != http.StatusInternalServerError {
		t.Fatalf("oauth status %d: %s", rec.Code, rec.Body.String())
	}
}

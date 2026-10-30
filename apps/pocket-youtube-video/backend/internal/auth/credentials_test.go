package auth

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestFileCredentialStoreEncryptsAndUsesPrivateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "key")
	store, err := NewFileCredentialStore(path)
	if err != nil {
		t.Fatal("store initialization failed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal("key file missing")
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("key permissions %o", info.Mode().Perm())
	}
	secret := Secret{AccessToken: "access-secret", RefreshToken: "refresh-secret", Cookie: "cookie-secret", ExpiresAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	sealed, err := store.Put("acct", secret)
	if err != nil {
		t.Fatal("seal failed")
	}
	joined := string(sealed.Data)
	for _, value := range []string{secret.AccessToken, secret.RefreshToken, secret.Cookie} {
		if strings.Contains(joined, value) {
			t.Fatal("ciphertext contains plaintext")
		}
	}
	opened, err := store.Get("acct", sealed)
	if err != nil || !reflect.DeepEqual(opened, secret) {
		t.Fatal("round trip failed")
	}
	if _, err := store.Get("other", sealed); err == nil {
		t.Fatal("account binding was not enforced")
	}
}
func TestSecretJSONOmitsCredentials(t *testing.T) {
	encoded, err := json.Marshal(Secret{AccessToken: "access-secret", RefreshToken: "refresh-secret", Cookie: "cookie-secret"})
	if err != nil {
		t.Fatal("marshal failed")
	}
	if strings.Contains(string(encoded), "secret") {
		t.Fatal("JSON exposed credential material")
	}
}

func TestCookieRouteNeverReturnsImportedCookie(t *testing.T) {
	const cookie = "SID=private-cookie-value"
	handler := NewHandlers(&AuthService{})
	request := httptest.NewRequest("POST", "/api/auth/cookie", strings.NewReader(`{"name":"demo","base_url":"https://example.test","cookie":"`+cookie+`"}`))
	response := httptest.NewRecorder()
	handler.ImportCookie(response, request)
	if strings.Contains(response.Body.String(), cookie) {
		t.Fatal("API response exposed imported cookie")
	}
}

func TestCredentialStoreContract(t *testing.T) {
	var store CredentialStore = &FileEncryptedStore{}
	_ = store
	f, err := NewFileCredentialStore(filepath.Join(t.TempDir(), "k"))
	if err != nil {
		t.Fatal(err)
	}
	secret := Secret{AccessToken: "a", Cookie: "c"}
	cipher, err := f.Put("acct-1", secret)
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.Get("acct-1", cipher)
	if err != nil || got.AccessToken != "a" || got.Cookie != "c" {
		t.Fatalf("roundtrip: %v %+v", err, got)
	}
	ids, err := f.List()
	if err != nil || len(ids) != 1 || ids[0] != "acct-1" {
		t.Fatalf("list=%v err=%v", ids, err)
	}
	if err := f.Delete("acct-1"); err != nil {
		t.Fatal(err)
	}
	ids, _ = f.List()
	if len(ids) != 0 {
		t.Fatalf("delete left %v", ids)
	}
	// The reserved keyring store must fail loudly, never silently fall back.
	var keyring CredentialStore = SystemKeyringStore{}
	if _, err := keyring.Put("x", Secret{}); err == nil {
		t.Fatal("keyring store should be unimplemented")
	}
}

package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/url"
	"time"
)

type Handlers struct{ Service *AuthService }
type cookieRequest struct {
	Name    string `json:"name"`
	BaseURL string `json:"base_url"`
	Cookie  string `json:"cookie"`
}
type oauthBeginRequest struct {
	Name    string   `json:"name"`
	BaseURL string   `json:"base_url"`
	Scopes  []string `json:"scopes"`
}

func NewHandler(db *sql.DB, credentials CredentialStore, config OAuthConfig, exchanger OAuthExchanger) *Handlers {
	return NewHandlers(NewService(db, credentials, config, exchanger))
}
func NewHandlers(service *AuthService) *Handlers { return &Handlers{Service: service} }
func (h *Handlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/cookie", h.ImportCookie)
	mux.HandleFunc("POST /api/auth/oauth", h.BeginOAuth)
	mux.HandleFunc("DELETE /api/accounts/{id}", h.RemoveAccount)
	mux.HandleFunc("POST /api/accounts/{id}/activate", h.ActivateAccount)
	mux.HandleFunc("POST /api/accounts/logout", h.Logout)
}
func RegisterRoutes(mux *http.ServeMux, h *Handlers) { h.RegisterRoutes(mux) }
func (h *Handlers) ImportCookie(w http.ResponseWriter, r *http.Request) {
	var req cookieRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil || req.Name == "" || req.BaseURL == "" || req.Cookie == "" {
		http.Error(w, "invalid cookie import request", 400)
		return
	}
	if h == nil || h.Service == nil {
		http.Error(w, "authentication unavailable", 503)
		return
	}
	id, err := h.Service.ImportCookie(r.Context(), req.Name, req.BaseURL, req.Cookie)
	if err != nil {
		http.Error(w, "unable to import cookie", 500)
		return
	}
	writeSafeJSON(w, 201, map[string]string{"id": id, "name": req.Name, "base_url": req.BaseURL, "auth_type": "cookie"})
}

func (h *Handlers) BeginOAuth(w http.ResponseWriter, r *http.Request) {
	var req oauthBeginRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req) != nil || req.Name == "" || req.BaseURL == "" {
		http.Error(w, "invalid OAuth request", 400)
		return
	}
	if h == nil || h.Service == nil || h.Service.OAuth == nil {
		http.Error(w, "authentication unavailable", 503)
		return
	}
	session, err := h.Service.OAuth.Begin(r.Context(), req.Scopes...)
	if err != nil {
		http.Error(w, "unable to start authorization", 500)
		return
	}
	callbackCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	callbackURL := session.Listener.Addr().String()
	writeSafeJSON(w, 202, map[string]string{"url": session.URL, "callback": "http://" + callbackURL + "/oauth/callback"})
	go func() {
		defer cancel()
		defer session.Listener.Close()
		_ = ServeCallback(callbackCtx, session, func(callback *url.URL) error {
			_, err := h.Service.CompleteOAuth(callbackCtx, session, callback, req.Name, req.BaseURL)
			return err
		})
	}()
}

func (h *Handlers) RemoveAccount(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil || h.Service.Store == nil {
		http.Error(w, "authentication unavailable", 503)
		return
	}
	if err := h.Service.Store.RemoveAccount(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, "account not found", 404)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *Handlers) ActivateAccount(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil || h.Service.Store == nil {
		http.Error(w, "authentication unavailable", 503)
		return
	}
	if err := h.Service.Store.ActivateAccount(r.Context(), r.PathValue("id")); err != nil {
		http.Error(w, "account not found", 404)
		return
	}
	writeSafeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.Service == nil || h.Service.Store == nil {
		http.Error(w, "authentication unavailable", 503)
		return
	}
	if err := h.Service.Store.Logout(r.Context()); err != nil {
		http.Error(w, "unable to log out", 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func writeSafeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

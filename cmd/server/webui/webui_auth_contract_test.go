package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lich0821/ccNexus/internal/config"
	"github.com/lich0821/ccNexus/internal/storage"
)

func webUIRequest(mux *http.ServeMux, method, path, body, username, password string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func requireWebUIStatus(t *testing.T, mux *http.ServeMux, path, username, password string, want int) {
	t.Helper()
	rec := webUIRequest(mux, http.MethodGet, path, "", username, password)
	if rec.Code != want {
		t.Fatalf("GET %s status = %d, want %d", path, rec.Code, want)
	}
}

func TestBasicAuthChangesApplyImmediatelyToAPIAndUI(t *testing.T) {
	db, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "webui.db"))
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	defer db.Close()

	cfg := config.DefaultConfig()
	cfg.UpdateBasicAuth(false, "admin", "old-password")
	mux := http.NewServeMux()
	if err := New(cfg, nil, db).RegisterRoutes(mux); err != nil {
		t.Fatalf("register routes: %v", err)
	}

	rec := webUIRequest(mux, http.MethodPut, "/api/config/basic-auth", `{"enabled":true,"username":"alice","password":"first-password"}`, "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("enable auth status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/config", "/ui/"} {
		requireWebUIStatus(t, mux, path, "", "", http.StatusUnauthorized)
		requireWebUIStatus(t, mux, path, "alice", "first-password", http.StatusOK)
	}

	rec = webUIRequest(mux, http.MethodPut, "/api/config/basic-auth", `{"enabled":true,"username":"bob","password":"second-password"}`, "alice", "first-password")
	if rec.Code != http.StatusOK {
		t.Fatalf("change auth status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/config", "/ui/"} {
		requireWebUIStatus(t, mux, path, "alice", "first-password", http.StatusUnauthorized)
		requireWebUIStatus(t, mux, path, "bob", "second-password", http.StatusOK)
	}

	rec = webUIRequest(mux, http.MethodPost, "/api/config/basic-auth/reset-password", "", "bob", "second-password")
	if rec.Code != http.StatusOK {
		t.Fatalf("reset password status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Data struct {
			Password string `json:"password"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode reset response: %v", err)
	}
	if response.Data.Password == "" {
		t.Fatal("reset response did not contain the new password")
	}
	for _, path := range []string{"/api/config", "/ui/"} {
		requireWebUIStatus(t, mux, path, "bob", "second-password", http.StatusUnauthorized)
		requireWebUIStatus(t, mux, path, "bob", response.Data.Password, http.StatusOK)
	}

	rec = webUIRequest(mux, http.MethodPut, "/api/config/basic-auth", `{"enabled":false,"username":"bob","password":"***"}`, "bob", response.Data.Password)
	if rec.Code != http.StatusOK {
		t.Fatalf("disable auth status = %d, body = %s", rec.Code, rec.Body.String())
	}
	for _, path := range []string{"/api/config", "/ui/"} {
		requireWebUIStatus(t, mux, path, "", "", http.StatusOK)
	}
}

func TestAPIRoutesApplySameOriginPolicy(t *testing.T) {
	mux := http.NewServeMux()
	if err := New(config.DefaultConfig(), nil, nil).RegisterRoutes(mux); err != nil {
		t.Fatalf("register routes: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6677/api/not-found", nil)
	req.Header.Set("Origin", "https://attacker.example")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-origin API status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

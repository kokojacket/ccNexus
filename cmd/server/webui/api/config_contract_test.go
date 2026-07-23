package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lich0821/ccNexus/internal/config"
	"github.com/lich0821/ccNexus/internal/storage"
)

func newConfigContractHandler(t *testing.T) (*Handler, *config.Config, *storage.SQLiteStorage) {
	t.Helper()

	db, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "webui.db"))
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	cfg := config.DefaultConfig()
	cfg.UpdateBasicAuth(false, "admin", "old-password")
	return NewHandler(cfg, nil, db), cfg, db
}

func performConfigRequest(h *Handler, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestUpdateConfigTreatsFieldsAsOptional(t *testing.T) {
	h, cfg, _ := newConfigContractHandler(t)
	cfg.UpdateLogLevel(2)

	rec := performConfigRequest(h, http.MethodPut, "/api/config", `{"port":4321}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update port status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := cfg.GetPort(); got != 4321 {
		t.Fatalf("port = %d, want 4321", got)
	}
	if got := cfg.GetLogLevel(); got != 2 {
		t.Fatalf("omitted logLevel changed to %d", got)
	}

	rec = performConfigRequest(h, http.MethodPut, "/api/config", `{"logLevel":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update log level status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := cfg.GetPort(); got != 4321 {
		t.Fatalf("omitted port changed to %d", got)
	}
	if got := cfg.GetLogLevel(); got != 3 {
		t.Fatalf("logLevel = %d, want 3", got)
	}
}

func TestBasicAuthConfigReturnsPasswordPresenceWithoutPlaceholder(t *testing.T) {
	h, _, _ := newConfigContractHandler(t)
	rec := performConfigRequest(h, http.MethodGet, "/api/config/basic-auth", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"password"`) || !strings.Contains(rec.Body.String(), `"hasPassword":true`) {
		t.Fatalf("response must expose password presence, not a placeholder: %s", rec.Body.String())
	}
}

func TestUpdateConfigValidatesAllFieldsBeforeMutation(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		lockPort   bool
		wantStatus int
	}{
		{name: "port too low", body: `{"port":0,"logLevel":3}`, wantStatus: http.StatusBadRequest},
		{name: "port too high", body: `{"port":65536,"logLevel":3}`, wantStatus: http.StatusBadRequest},
		{name: "log level too low", body: `{"port":4321,"logLevel":-1}`, wantStatus: http.StatusBadRequest},
		{name: "log level too high", body: `{"port":4321,"logLevel":4}`, wantStatus: http.StatusBadRequest},
		{name: "locked port", body: `{"port":4321,"logLevel":3}`, lockPort: true, wantStatus: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, cfg, _ := newConfigContractHandler(t)
			cfg.UpdateLogLevel(2)
			if tt.lockPort {
				cfg.LockPort()
			}

			rec := performConfigRequest(h, http.MethodPut, "/api/config", tt.body)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if got := cfg.GetPort(); got != 3000 {
				t.Fatalf("port mutated to %d", got)
			}
			if got := cfg.GetLogLevel(); got != 2 {
				t.Fatalf("logLevel mutated to %d", got)
			}
		})
	}
}

func TestBasicAuthUpdateRollsBackWhenPersistenceFails(t *testing.T) {
	h, cfg, db := newConfigContractHandler(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	rec := performConfigRequest(h, http.MethodPut, "/api/config/basic-auth", `{"enabled":true,"username":"new-user","password":"new-password"}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	if cfg.GetBasicAuthEnabled() {
		t.Fatal("failed update left Basic Auth enabled in memory")
	}
	if got := cfg.GetBasicAuthUsername(); got != "admin" {
		t.Fatalf("username = %q, want rollback to admin", got)
	}
	if got := cfg.GetBasicAuthPassword(); got != "old-password" {
		t.Fatalf("password = %q, want rollback", got)
	}
}

func TestConfigUpdateRollsBackWhenPersistenceFails(t *testing.T) {
	h, cfg, db := newConfigContractHandler(t)
	cfg.UpdateLogLevel(2)
	if err := db.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	rec := performConfigRequest(h, http.MethodPut, "/api/config", `{"port":4321,"logLevel":3}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	if got := cfg.GetPort(); got != 3000 {
		t.Fatalf("port = %d, want rollback to 3000", got)
	}
	if got := cfg.GetLogLevel(); got != 2 {
		t.Fatalf("logLevel = %d, want rollback to 2", got)
	}
}

func TestDedicatedConfigUpdatesRollBackWhenPersistenceFails(t *testing.T) {
	t.Run("port", func(t *testing.T) {
		h, cfg, db := newConfigContractHandler(t)
		if err := db.Close(); err != nil {
			t.Fatalf("close storage: %v", err)
		}

		rec := performConfigRequest(h, http.MethodPut, "/api/config/port", `{"port":4321}`)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
		}
		if got := cfg.GetPort(); got != 3000 {
			t.Fatalf("port = %d, want rollback to 3000", got)
		}
	})

	t.Run("log level", func(t *testing.T) {
		h, cfg, db := newConfigContractHandler(t)
		cfg.UpdateLogLevel(2)
		if err := db.Close(); err != nil {
			t.Fatalf("close storage: %v", err)
		}

		rec := performConfigRequest(h, http.MethodPut, "/api/config/log-level", `{"logLevel":3}`)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
		}
		if got := cfg.GetLogLevel(); got != 2 {
			t.Fatalf("logLevel = %d, want rollback to 2", got)
		}
	})
}

func TestBasicAuthResetRollsBackWhenPersistenceFails(t *testing.T) {
	h, cfg, db := newConfigContractHandler(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	rec := performConfigRequest(h, http.MethodPost, "/api/config/basic-auth/reset-password", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500, body = %s", rec.Code, rec.Body.String())
	}
	if got := cfg.GetBasicAuthPassword(); got != "old-password" {
		t.Fatalf("password = %q, want rollback", got)
	}
}

func TestConfigUpdatesDoNotOverwriteStoredEndpoints(t *testing.T) {
	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "runtime config", path: "/api/config", body: `{"logLevel":2}`},
		{name: "basic auth", path: "/api/config/basic-auth", body: `{"enabled":false,"username":"admin","password":""}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, db := newConfigContractHandler(t)
			endpoint := storage.Endpoint{
				Name: "stored-endpoint", APIUrl: "https://example.com", APIKey: "secret",
				AuthMode: config.AuthModeAPIKey, Enabled: true, Transformer: "openai",
			}
			if err := db.SaveEndpoint(&endpoint); err != nil {
				t.Fatalf("save endpoint: %v", err)
			}

			rec := performConfigRequest(h, http.MethodPut, tt.path, tt.body)
			if rec.Code != http.StatusOK {
				t.Fatalf("config update status=%d body=%s", rec.Code, rec.Body.String())
			}
			endpoints, err := db.GetEndpoints()
			if err != nil || len(endpoints) != 1 || endpoints[0].Name != "stored-endpoint" {
				t.Fatalf("config update overwrote endpoints: endpoints=%v err=%v", endpoints, err)
			}
		})
	}
}

func TestBasicAuthCannotBeEnabledWithoutCredentials(t *testing.T) {
	h, cfg, _ := newConfigContractHandler(t)
	cfg.UpdateBasicAuth(false, "", "")

	rec := performConfigRequest(h, http.MethodPut, "/api/config/basic-auth", `{"enabled":true,"username":"","password":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if cfg.GetBasicAuthEnabled() {
		t.Fatal("invalid update enabled Basic Auth")
	}
}

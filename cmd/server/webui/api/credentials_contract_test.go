package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lich0821/ccNexus/internal/storage"
)

func saveCredentialForTest(t *testing.T, h *Handler, endpointName string) storage.EndpointCredential {
	t.Helper()
	credential := storage.EndpointCredential{
		EndpointName: endpointName,
		ProviderType: "codex",
		AccountID:    "account-1",
		Email:        "user@example.com",
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		IDToken:      "id-secret",
		Status:       "active",
		Enabled:      true,
		Remark:       "primary",
	}
	if err := h.storage.SaveEndpointCredential(&credential); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	return credential
}

func assertCredentialResponseHasNoSecrets(t *testing.T, body string) {
	t.Helper()
	for _, field := range []string{`"accessToken"`, `"refreshToken"`, `"idToken"`} {
		if strings.Contains(body, field) {
			t.Fatalf("credential response contains secret field %s: %s", field, body)
		}
	}
}

func TestListEndpointCredentialsReturnsTokenPresenceWithoutSecrets(t *testing.T) {
	h := newEndpointTestHandler(t)
	credential := saveCredentialForTest(t, h, "endpoint-a")
	credential.LastError = "denied access-secret refresh-secret id-secret"
	if err := h.storage.UpdateEndpointCredential(&credential); err != nil {
		t.Fatalf("save credential error: %v", err)
	}
	if err := h.storage.UpsertCredentialRateLimits(credential.ID, &storage.CodexRateLimitsData{Source: "test"}, "error", "rate access-secret refresh-secret id-secret", time.Now()); err != nil {
		t.Fatalf("save rate limits: %v", err)
	}

	rec := httptest.NewRecorder()
	h.listEndpointCredentials(rec, httptest.NewRequest(http.MethodGet, "/api/endpoints/endpoint-a/credentials", nil), "endpoint-a")

	if rec.Code != http.StatusOK {
		t.Fatalf("list credentials failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	assertCredentialResponseHasNoSecrets(t, rec.Body.String())
	for _, secret := range []string{"access-secret", "refresh-secret", "id-secret"} {
		if strings.Contains(rec.Body.String(), secret) {
			t.Fatalf("credential response leaked %q: %s", secret, rec.Body.String())
		}
	}
	var response struct {
		Data struct {
			Credentials []map[string]interface{} `json:"credentials"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Credentials) != 1 {
		t.Fatalf("expected one credential, got %d", len(response.Data.Credentials))
	}
	got := response.Data.Credentials[0]
	for _, field := range []string{"hasAccessToken", "hasRefreshToken", "hasIdToken"} {
		if present, _ := got[field].(bool); !present {
			t.Fatalf("expected %s=true: %s", field, rec.Body.String())
		}
	}
	if got["rateLimits"] == nil {
		t.Fatalf("expected rateLimits in response: %s", rec.Body.String())
	}
}

func TestCredentialRoutesRejectExtraSegments(t *testing.T) {
	h := newEndpointTestHandler(t)
	h.config.UpdateBasicAuth(false, "", "")
	saveEndpointForTest(t, h, storage.Endpoint{
		Name: "endpoint-a", APIUrl: "https://example.com", APIKey: "secret", AuthMode: "api_key", Transformer: "openai",
	})
	credential := saveCredentialForTest(t, h, "endpoint-a")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/api/endpoints/endpoint-a/credentials/1/garbage", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("extra credential path status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got, err := h.storage.GetCredentialByID(credential.ID); err != nil || got == nil {
		t.Fatalf("extra path changed credential: credential=%v err=%v", got, err)
	}
}

func TestActivateCredentialClearsRecoverableFailureState(t *testing.T) {
	h := newEndpointTestHandler(t)
	credential := saveCredentialForTest(t, h, "endpoint-a")
	cooldown := time.Now().Add(time.Hour)
	credential.Enabled = false
	credential.Status = "cooldown"
	credential.FailureCount = 3
	credential.CooldownUntil = &cooldown
	credential.LastError = "temporary failure"
	if err := h.storage.UpdateEndpointCredential(&credential); err != nil {
		t.Fatalf("prepare credential: %v", err)
	}

	rec := httptest.NewRecorder()
	h.updateEndpointCredential(rec, httptest.NewRequest(http.MethodPatch, "/api/endpoints/endpoint-a/credentials/1", strings.NewReader(`{"status":"active"}`)), "endpoint-a", credential.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("activate status=%d body=%s", rec.Code, rec.Body.String())
	}
	got, err := h.storage.GetCredentialByID(credential.ID)
	if err != nil || got == nil {
		t.Fatalf("load activated credential: credential=%v err=%v", got, err)
	}
	if !got.Enabled || got.Status != "active" || got.FailureCount != 0 || got.CooldownUntil != nil || got.LastError != "" {
		t.Fatalf("activation left failure state: %+v", got)
	}

	expired := time.Now().Add(-time.Minute)
	got.ExpiresAt = &expired
	if err := h.storage.UpdateEndpointCredential(got); err != nil {
		t.Fatalf("expire credential: %v", err)
	}
	rec = httptest.NewRecorder()
	h.updateEndpointCredential(rec, httptest.NewRequest(http.MethodPatch, "/api/endpoints/endpoint-a/credentials/1", strings.NewReader(`{"status":"active"}`)), "endpoint-a", credential.ID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expired activation status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpdateEndpointCredentialTokenContract(t *testing.T) {
	h := newEndpointTestHandler(t)
	credential := saveCredentialForTest(t, h, "endpoint-a")

	patch := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		h.updateEndpointCredential(rec, httptest.NewRequest(http.MethodPatch, "/api/endpoints/endpoint-a/credentials/1", strings.NewReader(body)), "endpoint-a", credential.ID)
		return rec
	}
	load := func() *storage.EndpointCredential {
		t.Helper()
		got, err := h.storage.GetCredentialByID(credential.ID)
		if err != nil || got == nil {
			t.Fatalf("load credential: credential=%v err=%v", got, err)
		}
		return got
	}

	rec := patch(`{"accessToken":"  ","refreshToken":"","idToken":"   "}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("blank token patch failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	got := load()
	if got.AccessToken != "access-secret" || got.RefreshToken != "refresh-secret" || got.IDToken != "id-secret" {
		t.Fatalf("blank token fields must preserve values: %+v", got)
	}

	rec = patch(`{"accessToken":"new-access","refreshToken":"new-refresh","idToken":"new-id"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("token replacement failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	assertCredentialResponseHasNoSecrets(t, rec.Body.String())
	got = load()
	if got.AccessToken != "new-access" || got.RefreshToken != "new-refresh" || got.IDToken != "new-id" {
		t.Fatalf("non-empty token fields must replace values: %+v", got)
	}

	rec = patch(`{"clearRefreshToken":true,"clearIdToken":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("explicit token clear failed: status=%d body=%s", rec.Code, rec.Body.String())
	}
	got = load()
	if got.AccessToken != "new-access" || got.RefreshToken != "" || got.IDToken != "" {
		t.Fatalf("explicit clear produced unexpected tokens: %+v", got)
	}

	rec = patch(`{"clearAccessToken":true}`)
	if rec.Code != http.StatusBadRequest || load().AccessToken != "new-access" {
		t.Fatalf("access token clear must be rejected: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeleteEndpointCredentialRejectsCrossEndpointID(t *testing.T) {
	h := newEndpointTestHandler(t)
	credential := saveCredentialForTest(t, h, "endpoint-b")
	if err := h.storage.UpsertCredentialRateLimits(credential.ID, &storage.CodexRateLimitsData{Source: "test"}, "ok", "", time.Now()); err != nil {
		t.Fatalf("save rate limits: %v", err)
	}
	if err := h.storage.UpsertCredentialUsage(credential.ID, "endpoint-b", 1, 2, 3, 4, time.Now()); err != nil {
		t.Fatalf("save usage: %v", err)
	}

	rec := httptest.NewRecorder()
	h.deleteEndpointCredential(rec, httptest.NewRequest(http.MethodDelete, "/api/endpoints/endpoint-a/credentials/1", nil), "endpoint-a", credential.ID)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got, err := h.storage.GetCredentialByID(credential.ID); err != nil || got == nil {
		t.Fatalf("credential changed after rejected delete: credential=%v err=%v", got, err)
	}
	if got, err := h.storage.GetCredentialRateLimits(credential.ID); err != nil || got == nil {
		t.Fatalf("rate limits changed after rejected delete: rateLimits=%v err=%v", got, err)
	}
	usage, err := h.storage.GetCredentialUsageByEndpoint("endpoint-b")
	if err != nil || usage[credential.ID] == nil {
		t.Fatalf("usage changed after rejected delete: usage=%v err=%v", usage, err)
	}
}

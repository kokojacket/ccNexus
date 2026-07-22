package proxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lich0821/ccNexus/internal/config"
	"github.com/lich0821/ccNexus/internal/logger"
	"github.com/lich0821/ccNexus/internal/storage"
)

func TestGeminiAPIKeyUsesHeaderInsteadOfURL(t *testing.T) {
	incoming := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{}`))
	proxyRequest, err := buildProxyRequest(
		incoming,
		config.Endpoint{APIUrl: "https://generativelanguage.googleapis.com", Transformer: "gemini", Model: "gemini-test"},
		"gemini-secret",
		[]byte(`{"stream":false}`),
		"cc_gemini",
		"gemini-test",
		nil,
	)
	if err != nil {
		t.Fatalf("build Gemini request: %v", err)
	}
	if proxyRequest.URL.Query().Get("key") != "" || strings.Contains(proxyRequest.URL.String(), "gemini-secret") {
		t.Fatalf("Gemini key leaked into URL: %s", proxyRequest.URL)
	}
	if proxyRequest.Header.Get("x-goog-api-key") != "gemini-secret" {
		t.Fatalf("Gemini auth header = %q", proxyRequest.Header.Get("x-goog-api-key"))
	}
}

func TestAttemptErrorsRedactKnownSecretsFromLogsAndStorage(t *testing.T) {
	db, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "security.db"))
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	storedEndpoint := storage.Endpoint{
		Name: "secure", APIUrl: "https://endpoint-user:endpoint-pass@example.invalid",
		APIKey: "api-secret", AuthMode: config.AuthModeAPIKey, Enabled: true, Transformer: "gemini",
	}
	if err := db.SaveEndpoint(&storedEndpoint); err != nil {
		t.Fatalf("save endpoint: %v", err)
	}
	credential := storage.EndpointCredential{
		EndpointName: "secure", ProviderType: "codex", AccountID: "account-1",
		AccessToken: "access-secret", RefreshToken: "refresh-secret", IDToken: "id-secret",
		Status: "active", Enabled: true,
	}
	if err := db.SaveEndpointCredential(&credential); err != nil {
		t.Fatalf("save credential: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.UpdateEndpoints([]config.Endpoint{{Name: "secure", Enabled: true}})
	p := New(cfg, storage.NewStatsStorageAdapter(db), db, "test")
	attempt := &endpointAttempt{
		endpoint: config.Endpoint{
			Name: "secure", APIUrl: storedEndpoint.APIUrl, APIKey: storedEndpoint.APIKey,
			AuthMode: config.AuthModeCodexTokenPool, Transformer: "gemini",
		},
		apiKey:             credential.AccessToken,
		credentialID:       credential.ID,
		selectedCredential: &credential,
	}

	log := logger.GetLogger()
	oldLevel := log.GetMinLevel()
	log.SetMinLevel(logger.DEBUG)
	log.Clear()
	t.Cleanup(func() {
		log.Clear()
		log.SetMinLevel(oldLevel)
	})

	providerBody := "denied api-secret access-secret refresh-secret id-secret endpoint-user endpoint-pass"
	p.handleRetryableStatus(&http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(providerBody)),
	}, attempt)
	assertSecretsAbsentFromProxyState(t, log, db, credential.ID)

	requestErr := &url.Error{
		Op:  "Get",
		URL: "https://endpoint-user:endpoint-pass@example.invalid/v1/models?key=access-secret",
		Err: errors.New("network failed"),
	}
	p.handleSendError(requestErr, attempt)
	assertSecretsAbsentFromProxyState(t, log, db, credential.ID)

	cfg.UpdateProxy(&config.ProxyConfig{URL: "http://proxy-user:proxy-password@127.0.0.1:8080"})
	proxyRequest := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	p.logUpstreamRequest(&proxyRequestContext{requestBytes: 1}, &endpointAttempt{
		endpoint:     config.Endpoint{Name: "secure"},
		modelName:    "gpt-test",
		proxyRequest: proxyRequest,
	})
	assertLogOmits(t, log, "proxy-user", "proxy-password")

	attempt.response = &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(providerBody)),
	}
	recorder := httptest.NewRecorder()
	p.handleFinalStatus(recorder, &proxyRequestContext{}, attempt)
	for _, secret := range []string{"api-secret", "access-secret", "refresh-secret", "id-secret", "endpoint-user", "endpoint-pass"} {
		if strings.Contains(recorder.Body.String(), secret) {
			t.Fatalf("client error body leaked %q: %s", secret, recorder.Body.String())
		}
	}
}

func TestSuccessfulResponseDebugLogOmitsPayloads(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "debug.log")
	log := logger.GetLogger()
	log.Close()
	if err := log.EnableDebugFile(logPath); err != nil {
		t.Fatalf("enable debug log: %v", err)
	}
	t.Cleanup(log.Close)

	endpoint := config.Endpoint{
		Name: "secure", APIUrl: "https://url-user:url-pass@example.invalid", APIKey: "api-secret",
		AuthMode: config.AuthModeAPIKey, Enabled: true, Transformer: "openai2",
	}
	cfg := config.DefaultConfig()
	cfg.UpdateEndpoints([]config.Endpoint{endpoint})
	p := &Proxy{config: cfg}

	nonStreaming := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"output_text":"api-secret url-user url-pass"}`)),
	}
	if _, _, err := p.handleNonStreamingResponse(httptest.NewRecorder(), nonStreaming, endpoint, &passthroughResponseTransformer{}); err != nil {
		t.Fatalf("handle non-stream response: %v", err)
	}

	streaming := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.completed\",\"response\":{\"output_text\":\"api-secret url-user url-pass\"}}\n\n",
		)),
	}
	if _, _, _, err := p.handleStreamingResponse(httptest.NewRecorder(), streaming, endpoint, &passthroughResponseTransformer{}, "cx_resp_openai2", false, "gpt-test", []byte(`{}`), 0); err != nil {
		t.Fatalf("handle stream response: %v", err)
	}

	log.Close()
	contents, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read debug log: %v", err)
	}
	for _, secret := range []string{"api-secret", "url-user", "url-pass"} {
		if strings.Contains(string(contents), secret) {
			t.Fatalf("debug log leaked %q:\n%s", secret, contents)
		}
	}
}

func assertSecretsAbsentFromProxyState(t *testing.T, log *logger.Logger, db *storage.SQLiteStorage, credentialID int64) {
	t.Helper()
	assertLogOmits(t, log,
		"api-secret", "access-secret", "refresh-secret", "id-secret", "endpoint-user", "endpoint-pass",
	)
	credential, err := db.GetCredentialByID(credentialID)
	if err != nil || credential == nil {
		t.Fatalf("load credential: credential=%v err=%v", credential, err)
	}
	for _, secret := range []string{"api-secret", "access-secret", "refresh-secret", "id-secret", "endpoint-user", "endpoint-pass"} {
		if strings.Contains(credential.LastError, secret) {
			t.Fatalf("stored error leaked %q: %s", secret, credential.LastError)
		}
	}
}

func assertLogOmits(t *testing.T, log *logger.Logger, secrets ...string) {
	t.Helper()
	entries := log.GetLogs()
	var messages strings.Builder
	for _, entry := range entries {
		messages.WriteString(entry.Message)
		messages.WriteByte('\n')
	}
	for _, secret := range secrets {
		if strings.Contains(messages.String(), secret) {
			t.Fatalf("logs leaked %q:\n%s", secret, messages.String())
		}
	}
}

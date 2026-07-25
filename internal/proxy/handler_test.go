package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/lich0821/ccNexus/internal/config"
)

func TestHealthResponseDoesNotExposeAPIKeyContent(t *testing.T) {
	const secret = "top-secret-health-key"
	const healthURL = "https://health-user:health-password@example.com/v1?api_key=health-query-secret&region=us"
	cfg := config.DefaultConfig()
	cfg.UpdateEndpoints([]config.Endpoint{{
		Name:        "primary",
		APIUrl:      healthURL,
		APIKey:      secret,
		AuthMode:    config.AuthModeAPIKey,
		Enabled:     true,
		Transformer: "openai",
	}})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	(&Proxy{config: cfg}).handleHealth(recorder, request)

	body := recorder.Body.String()
	for _, sensitive := range []string{secret[:4], secret[len(secret)-4:], "apiKey", "health-user", "health-password", "health-query-secret"} {
		if strings.Contains(body, sensitive) {
			t.Fatalf("health response exposed sensitive content %q: %s", sensitive, body)
		}
	}
	if recorder.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var response struct {
		EnabledEndpoints int                      `json:"enabled_endpoints"`
		Endpoints        []map[string]interface{} `json:"endpoints"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.EnabledEndpoints != 1 || len(response.Endpoints) != 1 {
		t.Fatalf("health response lost endpoint information: %+v", response)
	}
	endpoint := response.Endpoints[0]
	for key, want := range map[string]interface{}{
		"name":        "primary",
		"authMode":    config.AuthModeAPIKey,
		"enabled":     true,
		"transformer": "openai",
	} {
		if got := endpoint[key]; got != want {
			t.Errorf("health endpoint %s = %v, want %v", key, got, want)
		}
	}
	if _, exists := endpoint["apiKey"]; exists {
		t.Fatalf("health endpoint must not contain apiKey: %+v", endpoint)
	}
	safeURL, err := url.Parse(endpoint["apiUrl"].(string))
	if err != nil {
		t.Fatalf("parse health API URL: %v", err)
	}
	if safeURL.Host != "example.com" || safeURL.Path != "/v1" || safeURL.User != nil {
		t.Fatalf("health API URL lost public fields or retained userinfo: %s", safeURL)
	}
	if safeURL.Query().Get("api_key") != redactedSecret || safeURL.Query().Get("region") != "us" {
		t.Fatalf("health API URL query redaction = %s", safeURL.RawQuery)
	}
}

func TestHealthAndConfigUpdateAreRaceFree(t *testing.T) {
	configs := []*config.Config{config.DefaultConfig(), config.DefaultConfig()}
	for i, cfg := range configs {
		cfg.UpdateEndpoints([]config.Endpoint{{Name: string(rune('a' + i)), Enabled: true}})
	}
	proxy := &Proxy{config: configs[0]}
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 200; i++ {
			proxy.handleHealth(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 200; i++ {
			if err := proxy.UpdateConfig(configs[i%len(configs)]); err != nil {
				t.Errorf("update config: %v", err)
				return
			}
		}
	}()
	close(start)
	workers.Wait()
}

func TestConfigConsumersAndUpdateAreRaceFree(t *testing.T) {
	configs := []*config.Config{config.DefaultConfig(), config.DefaultConfig()}
	for i, cfg := range configs {
		cfg.ModelsCacheRefreshEnabled = i%2 == 0
		cfg.UpdateProxy(&config.ProxyConfig{})
		cfg.UpdateCodexProxy(&config.ProxyConfig{})
	}

	p := New(configs[0], nil, nil, "test")
	proxyRequest := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	reqCtx := &proxyRequestContext{requestBytes: 1}
	attempt := &endpointAttempt{
		endpoint:     config.Endpoint{Name: "test"},
		modelName:    "gpt-test",
		proxyRequest: proxyRequest,
	}

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 200; i++ {
			p.handleModels(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/models", nil))
			_ = p.codexRefreshHTTPClient()
			_ = p.codexRateLimitHTTPClient()
			p.logUpstreamRequest(reqCtx, attempt)
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 200; i++ {
			if err := p.UpdateConfig(configs[i%len(configs)]); err != nil {
				t.Errorf("update config: %v", err)
				return
			}
		}
	}()
	close(start)
	workers.Wait()
}

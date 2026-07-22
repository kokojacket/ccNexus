package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/lich0821/ccNexus/internal/config"
	"github.com/lich0821/ccNexus/internal/storage"
)

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (f providerRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSendTestRequestSupportsCodexTokenPoolProtocol(t *testing.T) {
	h := newEndpointTestHandler(t)
	saveCredentialForTest(t, h, "codex-pool")

	var requestURL string
	var requestHeaders http.Header
	var requestBody map[string]interface{}
	h.providerHTTPClient = &http.Client{Transport: providerRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requestURL = req.URL.String()
		requestHeaders = req.Header.Clone()
		if err := json.NewDecoder(req.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		body := strings.Join([]string{
			`data: {"type":"response.output_text.delta","delta":"codex-ok"}`,
			`data: {"type":"response.completed","response":{"output":[{"content":[{"type":"output_text","text":"codex-ok"}]}]}}`,
			`data: [DONE]`,
			"",
		}, "\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})}

	got, err := h.sendTestRequest(&storage.Endpoint{
		Name:        "codex-pool",
		APIUrl:      config.CodexTokenPoolAPIURL,
		AuthMode:    config.AuthModeCodexTokenPool,
		Transformer: config.CodexTokenPoolTransformer,
		Model:       "gpt-test",
	})
	if err != nil {
		t.Fatalf("send Codex token pool test request: %v", err)
	}
	if got != "codex-ok" {
		t.Fatalf("response text = %q, want codex-ok", got)
	}
	if requestURL != config.CodexTokenPoolAPIURL+"/responses" {
		t.Fatalf("request URL = %q", requestURL)
	}
	if requestHeaders.Get("Authorization") != "Bearer access-secret" ||
		requestHeaders.Get("Chatgpt-Account-Id") != "account-1" ||
		requestHeaders.Get("Originator") != "codex_cli_rs" ||
		requestHeaders.Get("Accept") != "text/event-stream" {
		t.Fatalf("incomplete Codex headers: %v", requestHeaders)
	}
	if requestBody["stream"] != true || requestBody["store"] != false {
		t.Fatalf("Codex request flags = %v", requestBody)
	}
	if _, ok := requestBody["instructions"]; !ok {
		t.Fatalf("Codex request omitted instructions: %v", requestBody)
	}
}

func TestExtractResponsesSSETextRequiresCompletedEvent(t *testing.T) {
	_, err := extractResponsesSSEText([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n"))
	if err == nil || !strings.Contains(err.Error(), "before response.completed") {
		t.Fatalf("incomplete stream error = %v", err)
	}
}

func TestExtractResponsesSSETextReportsFailureEvents(t *testing.T) {
	for _, eventType := range []string{"response.failed", "error"} {
		t.Run(eventType, func(t *testing.T) {
			body := `data: {"type":"` + eventType + `","error":{"message":"provider detail"}}` + "\n\n"
			_, err := extractResponsesSSEText([]byte(body))
			if err == nil || !strings.Contains(err.Error(), eventType) {
				t.Fatalf("failure event error = %v", err)
			}
		})
	}
}

package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lich0821/ccNexus/internal/config"
)

func TestEndpointLightWithModelUsesTemporaryModelWithoutSaving(t *testing.T) {
	models := make(chan string, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		models <- body["model"].(string)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer upstream.Close()

	cfg := config.DefaultConfig()
	cfg.UpdateEndpoints([]config.Endpoint{{
		Name: "desktop-test", APIUrl: upstream.URL, APIKey: "secret",
		AuthMode: config.AuthModeAPIKey, Transformer: "openai2", Model: "configured-model",
	}})
	service := NewEndpointService(cfg, nil, nil)
	result := service.TestEndpointLightWithModel(0, "temporary-model")
	if got := <-models; got != "temporary-model" {
		t.Fatalf("test model=%q, want temporary-model", got)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(result), &decoded); err != nil || decoded["success"] != true {
		t.Fatalf("unexpected test result: %s err=%v", result, err)
	}
	if got := cfg.GetEndpoints()[0].Model; got != "configured-model" {
		t.Fatalf("temporary test model was persisted: %q", got)
	}
}

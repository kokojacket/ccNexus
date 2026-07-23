package main

import (
	"os"
	"strings"
	"testing"
)

func readDesktopFrontend(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestDesktopEndpointTestUsesTemporaryModelPrompt(t *testing.T) {
	endpoints := readDesktopFrontend(t, "frontend/src/modules/endpoints.js")
	if strings.Count(endpoints, "item.dataset.model = model;") < 2 {
		t.Error("detail and compact endpoint cards must expose their configured model to the test action")
	}

	modal := readDesktopFrontend(t, "frontend/src/modules/modal.js")
	for _, value := range []string{
		"requestTestModel",
		"await testEndpointLight(index, testModel)",
		"input.value = configuredModel;",
		"test-model-fetch",
		"FetchModels(endpoint.apiUrl, endpoint.apiKey, endpoint.transformer)",
	} {
		if !strings.Contains(modal, value) {
			t.Errorf("desktop test flow must contain %q", value)
		}
	}
	if strings.Contains(modal, "return Promise.resolve(model)") {
		t.Error("desktop test flow must always open the model prompt")
	}

	configJS := readDesktopFrontend(t, "frontend/src/modules/config.js")
	if !strings.Contains(configJS, "TestEndpointLightWithModel(index, model)") {
		t.Error("desktop frontend must pass the temporary model through the Wails binding")
	}
}

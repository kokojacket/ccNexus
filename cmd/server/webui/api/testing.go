package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lich0821/ccNexus/internal/config"
	"github.com/lich0821/ccNexus/internal/logger"
	"github.com/lich0821/ccNexus/internal/proxy"
	"github.com/lich0821/ccNexus/internal/storage"
)

// testEndpoint tests an endpoint's connectivity
func (h *Handler) testEndpoint(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Get endpoint
	endpoints, err := h.storage.GetEndpoints()
	if err != nil {
		logger.Error("Failed to get endpoints: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get endpoints")
		return
	}

	var endpoint *storage.Endpoint
	for i := range endpoints {
		if endpoints[i].Name == name {
			endpoint = &endpoints[i]
			break
		}
	}

	if endpoint == nil {
		WriteError(w, http.StatusNotFound, "Endpoint not found")
		return
	}

	var request struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil && err != io.EOF {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if model := strings.TrimSpace(request.Model); model != "" {
		endpoint.Model = model
	}

	// Test the endpoint
	start := time.Now()
	response, err := h.sendTestRequest(endpoint)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		WriteJSON(w, http.StatusOK, map[string]interface{}{
			"success": false,
			"latency": latency,
			"error":   err.Error(),
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]interface{}{
		"success":  true,
		"latency":  latency,
		"response": response,
	})
}

// sendTestRequest sends a test request to an endpoint
func (h *Handler) sendTestRequest(endpoint *storage.Endpoint) (string, error) {
	apiKey, credential, authErr := h.resolveEndpointAuth(endpoint)
	if authErr != nil {
		return "", authErr
	}
	model := strings.TrimSpace(endpoint.Model)
	if model == "" {
		return "", fmt.Errorf("model is required to test endpoint")
	}
	codexTokenPool := config.NormalizeAuthMode(endpoint.AuthMode) == config.AuthModeCodexTokenPool
	redact := func(message string) string {
		secrets := []string{apiKey, endpoint.APIKey}
		if credential != nil {
			secrets = append(secrets, credential.AccessToken, credential.RefreshToken, credential.IDToken)
		}
		return proxy.RedactProviderMessage(message, endpoint.APIUrl, secrets...)
	}

	var reqBody []byte
	var url string
	var err error

	switch endpoint.Transformer {
	case "claude":
		url = providerURL(endpoint.APIUrl, "/v1/messages")
		reqBody, err = json.Marshal(map[string]interface{}{
			"model": model,
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": "你是什么模型?",
				},
			},
			"max_tokens": 16,
		})
	case "openai":
		url = providerURL(endpoint.APIUrl, "/v1/chat/completions")
		reqBody, err = json.Marshal(map[string]interface{}{
			"model": model,
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": "你是什么模型?",
				},
			},
			"max_tokens": 16,
		})
	case "openai2":
		requestPath := "/v1/responses"
		payload := map[string]interface{}{
			"model":             model,
			"input":             "你是什么模型?",
			"max_output_tokens": 16,
		}
		if codexTokenPool {
			requestPath = "/responses"
			payload["instructions"] = ""
			payload["store"] = false
			payload["stream"] = true
		}
		url = providerURL(endpoint.APIUrl, requestPath)
		reqBody, err = json.Marshal(payload)
	case "gemini":
		url = providerURL(endpoint.APIUrl, fmt.Sprintf("/v1beta/models/%s:generateContent", model))
		reqBody, err = json.Marshal(map[string]interface{}{
			"contents": []map[string]interface{}{
				{
					"parts": []map[string]interface{}{
						{
							"text": "你是什么模型?",
						},
					},
				},
			},
		})
	default:
		return "", fmt.Errorf("unsupported transformer: %s", endpoint.Transformer)
	}

	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %s", redact(err.Error()))
	}

	req.Header.Set("Content-Type", "application/json")

	// Add authentication based on transformer
	switch endpoint.Transformer {
	case "claude":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "openai", "openai2":
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case "gemini":
		req.Header.Set("x-goog-api-key", apiKey)
	}
	proxy.ApplyCodexCredentialHeaders(req, credential, reqBody)

	resp, err := h.providerClient(30 * time.Second).Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %s", redact(err.Error()))
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, redact(string(body)))
	}

	if endpoint.Transformer == "openai2" &&
		(codexTokenPool || strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")) {
		text, err := extractResponsesSSEText(body)
		if err != nil {
			return "", err
		}
		return redact(text), nil
	}

	// Parse response to extract the actual message
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Extract message based on transformer
	switch endpoint.Transformer {
	case "claude":
		if content, ok := result["content"].([]interface{}); ok && len(content) > 0 {
			if block, ok := content[0].(map[string]interface{}); ok {
				if text, ok := block["text"].(string); ok {
					return redact(text), nil
				}
			}
		}
	case "openai":
		if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if message, ok := choice["message"].(map[string]interface{}); ok {
					if content, ok := message["content"].(string); ok {
						return redact(content), nil
					}
				}
			}
		}
	case "openai2":
		if text := extractResponsesText(result); text != "" {
			return redact(text), nil
		}
	case "gemini":
		if candidates, ok := result["candidates"].([]interface{}); ok && len(candidates) > 0 {
			if candidate, ok := candidates[0].(map[string]interface{}); ok {
				if content, ok := candidate["content"].(map[string]interface{}); ok {
					if parts, ok := content["parts"].([]interface{}); ok && len(parts) > 0 {
						if part, ok := parts[0].(map[string]interface{}); ok {
							if text, ok := part["text"].(string); ok {
								return redact(text), nil
							}
						}
					}
				}
			}
		}
	}

	return "", fmt.Errorf("missing expected response content")
}

func (h *Handler) resolveEndpointAPIKey(endpoint *storage.Endpoint) (string, error) {
	apiKey, _, err := h.resolveEndpointAuth(endpoint)
	return apiKey, err
}

func (h *Handler) resolveEndpointAuth(endpoint *storage.Endpoint) (string, *storage.EndpointCredential, error) {
	authMode := config.NormalizeAuthMode(endpoint.AuthMode)
	if config.IsTokenPoolAuthMode(authMode) {
		cred, err := h.storage.GetUsableEndpointCredential(endpoint.Name, time.Now().UTC())
		if err != nil {
			return "", nil, fmt.Errorf("failed to get token from pool: %w", err)
		}
		if cred == nil || strings.TrimSpace(cred.AccessToken) == "" {
			return "", nil, fmt.Errorf("no usable token in token pool")
		}
		return strings.TrimSpace(cred.AccessToken), cred, nil
	}

	apiKey := strings.TrimSpace(endpoint.APIKey)
	if apiKey == "" {
		return "", nil, fmt.Errorf("apiKey is empty")
	}
	return apiKey, nil, nil
}

// handleFetchModels fetches available models from a provider
func (h *Handler) handleFetchModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req struct {
		EndpointName string `json:"endpointName"`
		APIUrl       string `json:"apiUrl"`
		APIKey       string `json:"apiKey"`
		Transformer  string `json:"transformer"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	apiURL := req.APIUrl
	apiKey := req.APIKey
	transformer := req.Transformer
	if endpointName := strings.TrimSpace(req.EndpointName); endpointName != "" {
		endpoints, err := h.storage.GetEndpoints()
		if err != nil {
			logger.Error("Failed to get endpoints: %v", err)
			WriteError(w, http.StatusInternalServerError, "Failed to get endpoints")
			return
		}
		var endpoint *storage.Endpoint
		for i := range endpoints {
			if endpoints[i].Name == endpointName {
				endpoint = &endpoints[i]
				break
			}
		}
		if endpoint == nil {
			WriteError(w, http.StatusNotFound, "Endpoint not found")
			return
		}
		apiURL = endpoint.APIUrl
		transformer = endpoint.Transformer
		resolvedAPIKey, err := h.resolveEndpointAPIKey(endpoint)
		if err != nil {
			WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		apiKey = resolvedAPIKey
	}
	if !isSupportedEndpointTransformer(transformer) {
		WriteError(w, http.StatusBadRequest, fmt.Sprintf("unsupported transformer: %s", transformer))
		return
	}

	models, err := h.fetchModelsFromProvider(apiURL, apiKey, transformer)
	if err != nil {
		logger.Error("Failed to fetch models: %v", err)
		WriteError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to fetch models: %v", err))
		return
	}

	WriteSuccess(w, map[string]interface{}{
		"models": models,
	})
}

// fetchModelsFromProvider fetches available models from a provider
func (h *Handler) fetchModelsFromProvider(apiUrl, apiKey, transformer string) ([]string, error) {
	normalizedAPIURL := normalizeAPIUrl(strings.TrimSpace(apiUrl))
	redact := func(message string) string {
		return proxy.RedactProviderMessage(message, normalizedAPIURL, apiKey)
	}
	codexAPIURL := strings.TrimSuffix(normalizedAPIURL, "/v1")
	if transformer == "openai2" && strings.EqualFold(codexAPIURL, config.CodexTokenPoolAPIURL) {
		return nil, fmt.Errorf("models are unavailable for Codex token pool endpoints")
	}

	var modelsPath string
	switch transformer {
	case "claude", "openai", "openai2":
		modelsPath = "/v1/models"
	case "gemini":
		modelsPath = "/v1beta/models"
	default:
		return nil, fmt.Errorf("unsupported transformer: %s", transformer)
	}
	url := providerURL(normalizedAPIURL, modelsPath)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %s", redact(err.Error()))
	}

	req.Header.Set("Accept", "application/json")
	switch transformer {
	case "claude":
		req.Header.Set("x-api-key", apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")
	case "openai", "openai2":
		req.Header.Set("Authorization", "Bearer "+apiKey)
	case "gemini":
		req.Header.Set("x-goog-api-key", apiKey)
	}

	resp, err := h.providerClient(10 * time.Second).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %s", redact(err.Error()))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, redact(string(body)))
	}

	if transformer == "gemini" {
		var result struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}
		models := make([]string, 0, len(result.Models))
		for _, model := range result.Models {
			name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(model.Name), "models/"))
			if name != "" {
				models = append(models, name)
			}
		}
		return models, nil
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	models := make([]string, 0, len(result.Data))
	for _, model := range result.Data {
		if id := strings.TrimSpace(model.ID); id != "" {
			models = append(models, id)
		}
	}
	return models, nil
}

func providerURL(apiURL, requestPath string) string {
	apiURL = strings.TrimRight(normalizeAPIUrl(apiURL), "/")
	for _, versionPath := range []string{"/v1", "/v1beta"} {
		if strings.HasPrefix(requestPath, versionPath+"/") && strings.HasSuffix(apiURL, versionPath) {
			return apiURL + strings.TrimPrefix(requestPath, versionPath)
		}
	}
	return apiURL + requestPath
}

func redactAPIKey(message, apiKey string) string {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		return message
	}
	return strings.ReplaceAll(message, apiKey, "[REDACTED]")
}

func (h *Handler) providerClient(timeout time.Duration) *http.Client {
	if h.providerHTTPClient != nil {
		return h.providerHTTPClient
	}
	return &http.Client{Timeout: timeout}
}

func extractResponsesSSEText(body []byte) (string, error) {
	var deltas strings.Builder
	completedText := ""
	completed := false
	for _, rawLine := range bytes.Split(body, []byte("\n")) {
		line := strings.TrimSpace(string(rawLine))
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}
		switch event["type"] {
		case "response.output_text.delta":
			if delta, ok := event["delta"].(string); ok {
				deltas.WriteString(delta)
			}
		case "response.completed":
			completed = true
			if response, ok := event["response"].(map[string]interface{}); ok {
				completedText = extractResponsesText(response)
			}
		case "response.failed", "error":
			return "", fmt.Errorf("test response received %s", event["type"])
		}
	}
	if !completed {
		return "", fmt.Errorf("test response ended before response.completed")
	}
	if completedText != "" {
		return completedText, nil
	}
	if deltas.Len() > 0 {
		return deltas.String(), nil
	}
	return "", fmt.Errorf("missing expected response content")
}

func extractResponsesText(result map[string]interface{}) string {
	if text, ok := result["output_text"].(string); ok {
		return text
	}
	output, _ := result["output"].([]interface{})
	for _, item := range output {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		content, _ := itemMap["content"].([]interface{})
		for _, block := range content {
			blockMap, ok := block.(map[string]interface{})
			if !ok {
				continue
			}
			if text, ok := blockMap["text"].(string); ok {
				return text
			}
		}
	}
	return ""
}

package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lich0821/ccNexus/internal/logger"
	"github.com/lich0821/ccNexus/internal/storage"
)

type importCredentialItem struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
	LastRefresh  string `json:"last_refresh"`
	Email        string `json:"email"`
	Type         string `json:"type"`
	Expired      string `json:"expired"`
	Remark       string `json:"remark"`
	Enabled      *bool  `json:"enabled"`
}

type importCredentialsRequest struct {
	Items     []importCredentialItem `json:"items"`
	Overwrite bool                   `json:"overwrite"`
	Remark    string                 `json:"remark"`
}

type credentialResponse struct {
	ID              int64                         `json:"id"`
	EndpointName    string                        `json:"endpointName"`
	ProviderType    string                        `json:"providerType"`
	AccountID       string                        `json:"accountId,omitempty"`
	Email           string                        `json:"email,omitempty"`
	HasAccessToken  bool                          `json:"hasAccessToken"`
	HasRefreshToken bool                          `json:"hasRefreshToken"`
	HasIDToken      bool                          `json:"hasIdToken"`
	LastRefresh     *time.Time                    `json:"lastRefresh,omitempty"`
	ExpiresAt       *time.Time                    `json:"expiresAt,omitempty"`
	Status          string                        `json:"status"`
	Enabled         bool                          `json:"enabled"`
	FailureCount    int                           `json:"failureCount"`
	CooldownUntil   *time.Time                    `json:"cooldownUntil,omitempty"`
	LastCheckedAt   *time.Time                    `json:"lastCheckedAt,omitempty"`
	LastUsedAt      *time.Time                    `json:"lastUsedAt,omitempty"`
	LastError       string                        `json:"lastError,omitempty"`
	Remark          string                        `json:"remark,omitempty"`
	RateLimits      *storage.CredentialRateLimits `json:"rateLimits,omitempty"`
	Usage           *storage.CredentialUsage      `json:"usage,omitempty"`
	CreatedAt       time.Time                     `json:"createdAt"`
	UpdatedAt       time.Time                     `json:"updatedAt"`
}

type updateCredentialRequest struct {
	AccessToken       *string `json:"accessToken"`
	RefreshToken      *string `json:"refreshToken"`
	IDToken           *string `json:"idToken"`
	ClearAccessToken  bool    `json:"clearAccessToken"`
	ClearRefreshToken bool    `json:"clearRefreshToken"`
	ClearIDToken      bool    `json:"clearIdToken"`
	Remark            *string `json:"remark"`
	Enabled           *bool   `json:"enabled"`
	ExpiresAt         *string `json:"expiresAt"`
	LastRefresh       *string `json:"lastRefresh"`
	Status            *string `json:"status"`
}

func newCredentialResponse(credential storage.EndpointCredential) credentialResponse {
	rateLimits := credential.RateLimits
	if rateLimits != nil {
		copy := *rateLimits
		copy.Error = redactCredentialSecrets(copy.Error, credential)
		rateLimits = &copy
	}
	return credentialResponse{
		ID:              credential.ID,
		EndpointName:    credential.EndpointName,
		ProviderType:    credential.ProviderType,
		AccountID:       credential.AccountID,
		Email:           credential.Email,
		HasAccessToken:  strings.TrimSpace(credential.AccessToken) != "",
		HasRefreshToken: strings.TrimSpace(credential.RefreshToken) != "",
		HasIDToken:      strings.TrimSpace(credential.IDToken) != "",
		LastRefresh:     credential.LastRefresh,
		ExpiresAt:       credential.ExpiresAt,
		Status:          credential.Status,
		Enabled:         credential.Enabled,
		FailureCount:    credential.FailureCount,
		CooldownUntil:   credential.CooldownUntil,
		LastCheckedAt:   credential.LastCheckedAt,
		LastUsedAt:      credential.LastUsedAt,
		LastError:       redactCredentialSecrets(credential.LastError, credential),
		Remark:          credential.Remark,
		RateLimits:      rateLimits,
		Usage:           credential.Usage,
		CreatedAt:       credential.CreatedAt,
		UpdatedAt:       credential.UpdatedAt,
	}
}

func redactCredentialSecrets(message string, credential storage.EndpointCredential) string {
	for _, secret := range []string{credential.AccessToken, credential.RefreshToken, credential.IDToken} {
		message = redactAPIKey(message, secret)
	}
	return message
}

func (h *Handler) handleEndpointCredentials(w http.ResponseWriter, r *http.Request, endpointName string, parts []string) {
	endpoint, err := h.getEndpointByName(endpointName)
	if err != nil {
		logger.Error("Failed to get endpoint: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get endpoint")
		return
	}
	if endpoint == nil {
		WriteError(w, http.StatusNotFound, "Endpoint not found")
		return
	}

	if len(parts) == 0 || parts[0] == "" {
		if len(parts) > 1 {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			h.listEndpointCredentials(w, r, endpointName)
		case http.MethodPost:
			h.importEndpointCredentials(w, r, endpointName)
		default:
			WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
		}
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}

	switch parts[0] {
	case "import":
		if r.Method != http.MethodPost {
			WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		h.importEndpointCredentials(w, r, endpointName)
		return
	case "stats":
		if r.Method != http.MethodGet {
			WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
			return
		}
		h.getEndpointCredentialStats(w, r, endpointName)
		return
	}

	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		WriteError(w, http.StatusBadRequest, "Invalid credential id")
		return
	}

	switch r.Method {
	case http.MethodPatch:
		h.updateEndpointCredential(w, r, endpointName, id)
	case http.MethodDelete:
		h.deleteEndpointCredential(w, r, endpointName, id)
	default:
		WriteError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (h *Handler) listEndpointCredentials(w http.ResponseWriter, r *http.Request, endpointName string) {
	credentials, err := h.storage.GetEndpointCredentials(endpointName)
	if err != nil {
		logger.Error("Failed to get endpoint credentials: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get endpoint credentials")
		return
	}
	rateLimits, err := h.storage.GetCredentialRateLimitsByEndpoint(endpointName)
	if err != nil {
		logger.Warn("Failed to load rate limits: %v", err)
		rateLimits = nil
	}

	stats, err := h.storage.GetTokenPoolStats(endpointName)
	if err != nil {
		logger.Warn("Failed to get credential stats: %v", err)
	}

	responseCredentials := make([]credentialResponse, len(credentials))
	for i := range credentials {
		if rateLimits != nil {
			if entry, ok := rateLimits[credentials[i].ID]; ok {
				credentials[i].RateLimits = entry
			}
		}
		responseCredentials[i] = newCredentialResponse(credentials[i])
	}

	WriteSuccess(w, map[string]interface{}{
		"credentials": responseCredentials,
		"stats":       stats,
	})
}

func (h *Handler) importEndpointCredentials(w http.ResponseWriter, r *http.Request, endpointName string) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req, items, err := parseImportCredentialsPayload(rawBody)
	if err != nil {
		WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := h.storage.GetEndpointCredentials(endpointName)
	if err != nil {
		logger.Error("Failed to get existing credentials: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get existing credentials")
		return
	}

	accountIndex := make(map[string]storage.EndpointCredential)
	emailIndex := make(map[string]storage.EndpointCredential)
	for _, cred := range existing {
		if cred.AccountID != "" {
			accountIndex[cred.AccountID] = cred
		}
		if cred.Email != "" {
			emailIndex[cred.Email] = cred
		}
	}

	created := 0
	updated := 0
	skipped := 0
	failed := 0
	errors := make([]string, 0)

	for i, item := range items {
		if strings.TrimSpace(item.AccessToken) == "" {
			failed++
			errors = append(errors, fmt.Sprintf("item[%d]: access_token is required", i))
			continue
		}

		expiresAt, err := parseOptionalRFC3339(item.Expired)
		if err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("item[%d]: invalid expired: %v", i, err))
			continue
		}

		lastRefresh, err := parseOptionalRFC3339(item.LastRefresh)
		if err != nil {
			failed++
			errors = append(errors, fmt.Sprintf("item[%d]: invalid last_refresh: %v", i, err))
			continue
		}

		enabled := true
		if item.Enabled != nil {
			enabled = *item.Enabled
		}

		cred := storage.EndpointCredential{
			EndpointName: endpointName,
			ProviderType: strings.TrimSpace(item.Type),
			AccountID:    strings.TrimSpace(item.AccountID),
			Email:        strings.TrimSpace(item.Email),
			AccessToken:  strings.TrimSpace(item.AccessToken),
			RefreshToken: strings.TrimSpace(item.RefreshToken),
			IDToken:      strings.TrimSpace(item.IDToken),
			LastRefresh:  lastRefresh,
			ExpiresAt:    expiresAt,
			Status:       "active",
			Enabled:      enabled,
			Remark:       strings.TrimSpace(item.Remark),
		}
		if cred.ProviderType == "" {
			cred.ProviderType = "codex"
		}
		if cred.Remark == "" {
			cred.Remark = strings.TrimSpace(req.Remark)
		}

		existingCred := findExistingCredential(accountIndex, emailIndex, &cred)
		if existingCred == nil {
			if err := h.storage.SaveEndpointCredential(&cred); err != nil {
				failed++
				errors = append(errors, fmt.Sprintf("item[%d]: save failed: %v", i, err))
				continue
			}
			created++
		} else {
			if !req.Overwrite {
				skipped++
				continue
			}

			cred.ID = existingCred.ID
			if item.Enabled == nil {
				cred.Enabled = existingCred.Enabled
			}
			if cred.Remark == "" {
				cred.Remark = existingCred.Remark
			}
			if cred.LastRefresh == nil {
				cred.LastRefresh = existingCred.LastRefresh
			}
			if cred.ExpiresAt == nil {
				cred.ExpiresAt = existingCred.ExpiresAt
			}
			if cred.RefreshToken == "" {
				cred.RefreshToken = existingCred.RefreshToken
			}
			if cred.IDToken == "" {
				cred.IDToken = existingCred.IDToken
			}

			if err := h.storage.UpdateEndpointCredential(&cred); err != nil {
				failed++
				errors = append(errors, fmt.Sprintf("item[%d]: update failed: %v", i, err))
				continue
			}
			updated++
		}

		if cred.AccountID != "" {
			accountIndex[cred.AccountID] = cred
		}
		if cred.Email != "" {
			emailIndex[cred.Email] = cred
		}
	}

	WriteSuccess(w, map[string]interface{}{
		"created":   created,
		"updated":   updated,
		"skipped":   skipped,
		"failed":    failed,
		"processed": len(items),
		"errors":    errors,
	})
}

func (h *Handler) updateEndpointCredential(w http.ResponseWriter, r *http.Request, endpointName string, id int64) {
	var req updateCredentialRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	cred, err := h.storage.GetCredentialByID(id)
	if err != nil {
		logger.Error("Failed to get credential: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get credential")
		return
	}
	if cred == nil || cred.EndpointName != endpointName {
		WriteError(w, http.StatusNotFound, "Credential not found")
		return
	}

	if req.ClearAccessToken {
		WriteError(w, http.StatusBadRequest, "accessToken cannot be cleared")
		return
	}
	if req.AccessToken != nil && strings.TrimSpace(*req.AccessToken) != "" {
		token := strings.TrimSpace(*req.AccessToken)
		cred.AccessToken = token
	}
	if req.RefreshToken != nil && strings.TrimSpace(*req.RefreshToken) != "" {
		cred.RefreshToken = strings.TrimSpace(*req.RefreshToken)
	}
	if req.IDToken != nil && strings.TrimSpace(*req.IDToken) != "" {
		cred.IDToken = strings.TrimSpace(*req.IDToken)
	}
	if req.ClearRefreshToken {
		cred.RefreshToken = ""
	}
	if req.ClearIDToken {
		cred.IDToken = ""
	}
	if req.Remark != nil {
		cred.Remark = strings.TrimSpace(*req.Remark)
	}
	if req.Enabled != nil {
		cred.Enabled = *req.Enabled
	}
	if req.ExpiresAt != nil {
		expiresAt, err := parseOptionalRFC3339(*req.ExpiresAt)
		if err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid expiresAt: %v", err))
			return
		}
		cred.ExpiresAt = expiresAt
	}
	if req.LastRefresh != nil {
		lastRefresh, err := parseOptionalRFC3339(*req.LastRefresh)
		if err != nil {
			WriteError(w, http.StatusBadRequest, fmt.Sprintf("invalid lastRefresh: %v", err))
			return
		}
		cred.LastRefresh = lastRefresh
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != "" {
			if !isAllowedCredentialStatus(status) {
				WriteError(w, http.StatusBadRequest, "invalid status")
				return
			}
			if status == "active" {
				if cred.ExpiresAt != nil && !time.Now().UTC().Before(cred.ExpiresAt.UTC()) {
					WriteError(w, http.StatusBadRequest, "expired credential requires a new token")
					return
				}
				cred.Enabled = true
				cred.FailureCount = 0
				cred.CooldownUntil = nil
				cred.LastError = ""
			}
			cred.Status = status
		}
	}

	if err := h.storage.UpdateEndpointCredential(cred); err != nil {
		logger.Error("Failed to update credential: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to update credential")
		return
	}

	updated, err := h.storage.GetCredentialByID(id)
	if err != nil {
		logger.Error("Failed to reload credential: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to reload credential")
		return
	}
	if updated == nil {
		WriteError(w, http.StatusNotFound, "Credential not found")
		return
	}
	updated.RateLimits, _ = h.storage.GetCredentialRateLimits(id)
	if usage, usageErr := h.storage.GetCredentialUsageByEndpoint(endpointName); usageErr == nil {
		updated.Usage = usage[id]
	}
	WriteSuccess(w, newCredentialResponse(*updated))
}

func (h *Handler) deleteEndpointCredential(w http.ResponseWriter, r *http.Request, endpointName string, id int64) {
	credential, err := h.storage.GetCredentialByID(id)
	if err != nil {
		logger.Error("Failed to get credential: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get credential")
		return
	}
	if credential == nil || credential.EndpointName != endpointName {
		WriteError(w, http.StatusNotFound, "Credential not found")
		return
	}

	if err := h.storage.DeleteEndpointCredential(endpointName, id); err != nil {
		if errors.Is(err, storage.ErrCredentialNotFound) {
			WriteError(w, http.StatusNotFound, "Credential not found")
			return
		}
		logger.Error("Failed to delete credential: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to delete credential")
		return
	}

	WriteSuccess(w, map[string]interface{}{
		"message": "Credential deleted successfully",
	})
}

func (h *Handler) getEndpointCredentialStats(w http.ResponseWriter, r *http.Request, endpointName string) {
	stats, err := h.storage.GetTokenPoolStats(endpointName)
	if err != nil {
		logger.Error("Failed to get token pool stats: %v", err)
		WriteError(w, http.StatusInternalServerError, "Failed to get token pool stats")
		return
	}

	WriteSuccess(w, stats)
}

func (h *Handler) getEndpointByName(name string) (*storage.Endpoint, error) {
	endpoints, err := h.storage.GetEndpoints()
	if err != nil {
		return nil, err
	}
	for i := range endpoints {
		if endpoints[i].Name == name {
			return &endpoints[i], nil
		}
	}
	return nil, nil
}

func findExistingCredential(accountIndex map[string]storage.EndpointCredential, emailIndex map[string]storage.EndpointCredential, cred *storage.EndpointCredential) *storage.EndpointCredential {
	if cred.AccountID != "" {
		if existing, ok := accountIndex[cred.AccountID]; ok {
			return &existing
		}
	}
	if cred.Email != "" {
		if existing, ok := emailIndex[cred.Email]; ok {
			return &existing
		}
	}
	return nil
}

func parseImportCredentialsPayload(rawBody []byte) (*importCredentialsRequest, []importCredentialItem, error) {
	var req importCredentialsRequest
	if err := json.Unmarshal(rawBody, &req); err == nil && len(req.Items) > 0 {
		return &req, req.Items, nil
	}

	var items []importCredentialItem
	if err := json.Unmarshal(rawBody, &items); err == nil && len(items) > 0 {
		return &importCredentialsRequest{Items: items}, items, nil
	}

	var single importCredentialItem
	if err := json.Unmarshal(rawBody, &single); err == nil && strings.TrimSpace(single.AccessToken) != "" {
		return &importCredentialsRequest{Items: []importCredentialItem{single}}, []importCredentialItem{single}, nil
	}

	return nil, nil, fmt.Errorf("request body must be a credential object, array, or {items:[...]}")
}

func parseOptionalRFC3339(value string) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, trimmed)
	if err != nil {
		return nil, err
	}
	utc := t.UTC()
	return &utc, nil
}

func isAllowedCredentialStatus(status string) bool {
	switch status {
	case "active", "invalid", "cooldown":
		return true
	default:
		return false
	}
}

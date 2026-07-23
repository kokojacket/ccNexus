package proxy

import (
	"net/url"
	"strings"

	"github.com/lich0821/ccNexus/internal/config"
	"github.com/lich0821/ccNexus/internal/storage"
)

const redactedSecret = "[REDACTED]"

func redactAttemptMessage(message string, attempt *endpointAttempt) string {
	if attempt == nil {
		return message
	}
	secrets := []string{attempt.apiKey, attempt.endpoint.APIKey}
	secrets = append(secrets, credentialSecrets(attempt.selectedCredential)...)
	return RedactProviderMessage(message, attempt.endpoint.APIUrl, secrets...)
}

func redactEndpointMessage(message string, endpoint config.Endpoint) string {
	return RedactProviderMessage(message, endpoint.APIUrl, endpoint.APIKey)
}

func redactCredentialMessage(message string, credential *storage.EndpointCredential) string {
	return redactKnownSecrets(message, credentialSecrets(credential)...)
}

func credentialSecrets(credential *storage.EndpointCredential) []string {
	if credential == nil {
		return nil
	}
	return []string{credential.AccessToken, credential.RefreshToken, credential.IDToken}
}

func redactKnownSecrets(message string, secrets ...string) string {
	for _, secret := range secrets {
		secret = strings.TrimSpace(secret)
		if secret != "" {
			message = strings.ReplaceAll(message, secret, redactedSecret)
		}
	}
	return message
}

// RedactProviderMessage removes caller-provided secrets plus credentials embedded in a provider URL.
func RedactProviderMessage(message, rawURL string, secrets ...string) string {
	secrets = append(secrets, urlSecrets(rawURL)...)
	return redactKnownSecrets(message, secrets...)
}

// RedactURLSecrets returns a display-safe provider URL.
func RedactURLSecrets(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return "[INVALID_URL]"
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if isSensitiveQueryKey(key) {
			query.Set(key, redactedSecret)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func redactURLForLog(raw string) string {
	return RedactURLSecrets(raw)
}

func urlSecrets(raw string) []string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return nil
	}
	secrets := make([]string, 0)
	if parsed.User != nil {
		secrets = append(secrets, parsed.User.Username())
		if password, ok := parsed.User.Password(); ok {
			secrets = append(secrets, password)
		}
	}
	for key, values := range parsed.Query() {
		if isSensitiveQueryKey(key) {
			secrets = append(secrets, values...)
		}
	}
	return secrets
}

func isSensitiveQueryKey(key string) bool {
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(lower, "key") ||
		strings.Contains(lower, "token") ||
		strings.Contains(lower, "secret") ||
		strings.Contains(lower, "password") ||
		strings.Contains(lower, "credential") ||
		lower == "auth" || lower == "authorization"
}

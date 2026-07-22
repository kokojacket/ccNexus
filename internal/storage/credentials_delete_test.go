package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func newCredentialDeleteTestStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	storage, err := NewSQLiteStorage(filepath.Join(t.TempDir(), "storage.db"))
	if err != nil {
		t.Fatalf("create storage: %v", err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	return storage
}

func seedCredentialDeleteGraph(t *testing.T, storage *SQLiteStorage, endpointName string) EndpointCredential {
	t.Helper()
	endpoint := Endpoint{
		Name: endpointName, APIUrl: "https://example.com", APIKey: "key",
		AuthMode: "api_key", Transformer: "openai", Enabled: true,
	}
	if err := storage.SaveEndpoint(&endpoint); err != nil {
		t.Fatalf("save endpoint: %v", err)
	}
	credential := EndpointCredential{
		EndpointName: endpointName,
		ProviderType: "codex",
		AccessToken:  "access-secret",
		RefreshToken: "refresh-secret",
		IDToken:      "id-secret",
		Status:       "active",
		Enabled:      true,
	}
	if err := storage.SaveEndpointCredential(&credential); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if err := storage.UpsertCredentialRateLimits(credential.ID, &CodexRateLimitsData{Source: "test"}, "ok", "", time.Now()); err != nil {
		t.Fatalf("save rate limits: %v", err)
	}
	if err := storage.UpsertCredentialUsage(credential.ID, endpointName, 1, 2, 3, 4, time.Now()); err != nil {
		t.Fatalf("save usage: %v", err)
	}
	return credential
}

func assertCredentialGraphCounts(t *testing.T, storage *SQLiteStorage, endpointName string, credentialID int64, wantEndpoint, wantCredential, wantRateLimit, wantUsage int) {
	t.Helper()
	checks := []struct {
		query string
		arg   interface{}
		want  int
	}{
		{`SELECT COUNT(*) FROM endpoints WHERE name=?`, endpointName, wantEndpoint},
		{`SELECT COUNT(*) FROM endpoint_credentials WHERE id=?`, credentialID, wantCredential},
		{`SELECT COUNT(*) FROM credential_rate_limits WHERE credential_id=?`, credentialID, wantRateLimit},
		{`SELECT COUNT(*) FROM credential_usage WHERE credential_id=?`, credentialID, wantUsage},
	}
	for _, check := range checks {
		var got int
		if err := storage.db.QueryRow(check.query, check.arg).Scan(&got); err != nil {
			t.Fatalf("count rows: %v", err)
		}
		if got != check.want {
			t.Fatalf("query %q: want %d rows, got %d", check.query, check.want, got)
		}
	}
}

func TestDeleteEndpointCredentialIsScopedAndTransactional(t *testing.T) {
	t.Run("wrong endpoint preserves graph", func(t *testing.T) {
		storage := newCredentialDeleteTestStorage(t)
		credential := seedCredentialDeleteGraph(t, storage, "endpoint-b")

		if err := storage.DeleteEndpointCredential("endpoint-a", credential.ID); err == nil {
			t.Fatal("expected credential not found error")
		}
		assertCredentialGraphCounts(t, storage, "endpoint-b", credential.ID, 1, 1, 1, 1)
	})

	t.Run("valid delete cleans associated rows", func(t *testing.T) {
		storage := newCredentialDeleteTestStorage(t)
		credential := seedCredentialDeleteGraph(t, storage, "endpoint-a")

		if err := storage.DeleteEndpointCredential("endpoint-a", credential.ID); err != nil {
			t.Fatalf("delete credential: %v", err)
		}
		assertCredentialGraphCounts(t, storage, "endpoint-a", credential.ID, 1, 0, 0, 0)
	})

	t.Run("failure rolls back associated deletes", func(t *testing.T) {
		storage := newCredentialDeleteTestStorage(t)
		credential := seedCredentialDeleteGraph(t, storage, "endpoint-a")
		if _, err := storage.db.Exec(`
			CREATE TRIGGER fail_credential_delete
			BEFORE DELETE ON endpoint_credentials
			BEGIN SELECT RAISE(ABORT, 'forced delete failure'); END;
		`); err != nil {
			t.Fatalf("create failure trigger: %v", err)
		}

		if err := storage.DeleteEndpointCredential("endpoint-a", credential.ID); err == nil {
			t.Fatal("expected forced delete failure")
		}
		assertCredentialGraphCounts(t, storage, "endpoint-a", credential.ID, 1, 1, 1, 1)
	})
}

func TestDeleteEndpointCleansCredentialGraphTransactionally(t *testing.T) {
	t.Run("valid delete cleans endpoint graph", func(t *testing.T) {
		storage := newCredentialDeleteTestStorage(t)
		credential := seedCredentialDeleteGraph(t, storage, "endpoint-a")

		if err := storage.DeleteEndpoint("endpoint-a"); err != nil {
			t.Fatalf("delete endpoint: %v", err)
		}
		assertCredentialGraphCounts(t, storage, "endpoint-a", credential.ID, 0, 0, 0, 0)
	})

	t.Run("failure rolls back endpoint graph", func(t *testing.T) {
		storage := newCredentialDeleteTestStorage(t)
		credential := seedCredentialDeleteGraph(t, storage, "endpoint-a")
		if _, err := storage.db.Exec(`
			CREATE TRIGGER fail_endpoint_credential_delete
			BEFORE DELETE ON endpoint_credentials
			BEGIN SELECT RAISE(ABORT, 'forced delete failure'); END;
		`); err != nil {
			t.Fatalf("create failure trigger: %v", err)
		}

		if err := storage.DeleteEndpoint("endpoint-a"); err == nil {
			t.Fatal("expected forced delete failure")
		}
		assertCredentialGraphCounts(t, storage, "endpoint-a", credential.ID, 1, 1, 1, 1)
	})
}

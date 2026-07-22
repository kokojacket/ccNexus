package storage

import (
	"errors"
	"testing"
)

func TestUpdateEndpointByNameRenamesReferences(t *testing.T) {
	storage := newCredentialDeleteTestStorage(t)
	credential := seedCredentialDeleteGraph(t, storage, "old-name")
	if err := storage.RecordDailyStat(&DailyStat{EndpointName: "old-name", Date: "2026-07-22", Requests: 1, DeviceID: "test"}); err != nil {
		t.Fatalf("record stats: %v", err)
	}

	updated := Endpoint{
		Name: "new-name", APIUrl: "https://new.example", APIKey: "new-key",
		AuthMode: "api_key", Enabled: true, Transformer: "openai", SortOrder: 3,
	}
	if err := storage.UpdateEndpointByName("old-name", &updated); err != nil {
		t.Fatalf("rename endpoint: %v", err)
	}

	checks := []struct {
		query string
		args  []interface{}
	}{
		{`SELECT 1 FROM endpoints WHERE name=?`, []interface{}{"new-name"}},
		{`SELECT 1 FROM endpoint_credentials WHERE id=? AND endpoint_name=?`, []interface{}{credential.ID, "new-name"}},
		{`SELECT 1 FROM credential_usage WHERE credential_id=? AND endpoint_name=?`, []interface{}{credential.ID, "new-name"}},
		{`SELECT 1 FROM daily_stats WHERE endpoint_name=?`, []interface{}{"new-name"}},
	}
	for _, check := range checks {
		var one int
		if err := storage.db.QueryRow(check.query, check.args...).Scan(&one); err != nil {
			t.Fatalf("renamed reference missing for %q: %v", check.query, err)
		}
	}
}

func TestDeleteEndpointReturnsNotFound(t *testing.T) {
	storage := newCredentialDeleteTestStorage(t)
	if err := storage.DeleteEndpoint("missing"); !errors.Is(err, ErrEndpointNotFound) {
		t.Fatalf("delete missing endpoint error=%v", err)
	}
}

func TestReorderEndpointsRollsBackOnFailure(t *testing.T) {
	storage := newCredentialDeleteTestStorage(t)
	for index, name := range []string{"first", "second"} {
		endpoint := Endpoint{
			Name: name, APIUrl: "https://example.com", APIKey: "key",
			AuthMode: "api_key", Transformer: "openai", SortOrder: index,
		}
		if err := storage.SaveEndpoint(&endpoint); err != nil {
			t.Fatalf("save endpoint: %v", err)
		}
	}
	if _, err := storage.db.Exec(`
		CREATE TRIGGER fail_second_reorder
		BEFORE UPDATE OF sort_order ON endpoints
		WHEN OLD.name='second'
		BEGIN SELECT RAISE(ABORT, 'forced reorder failure'); END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := storage.ReorderEndpoints([]string{"second", "first"}); err == nil {
		t.Fatal("expected reorder failure")
	}
	endpoints, err := storage.GetEndpoints()
	if err != nil {
		t.Fatalf("get endpoints: %v", err)
	}
	if len(endpoints) != 2 || endpoints[0].Name != "first" || endpoints[0].SortOrder != 0 || endpoints[1].Name != "second" || endpoints[1].SortOrder != 1 {
		t.Fatalf("failed reorder was not rolled back: %+v", endpoints)
	}
}

func TestSetConfigsRollsBackOnFailure(t *testing.T) {
	storage := newCredentialDeleteTestStorage(t)
	if err := storage.SetConfig("port", "3000"); err != nil {
		t.Fatalf("seed port: %v", err)
	}
	if err := storage.SetConfig("logLevel", "1"); err != nil {
		t.Fatalf("seed log level: %v", err)
	}
	if _, err := storage.db.Exec(`
		CREATE TRIGGER fail_log_level
		BEFORE UPDATE ON app_config
		WHEN OLD.key='logLevel'
		BEGIN SELECT RAISE(ABORT, 'forced config failure'); END;
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := storage.SetConfigs(map[string]string{"port": "4321", "logLevel": "3"}); err == nil {
		t.Fatal("expected config update failure")
	}
	port, _ := storage.GetConfig("port")
	logLevel, _ := storage.GetConfig("logLevel")
	if port != "3000" || logLevel != "1" {
		t.Fatalf("failed config update was not rolled back: port=%q logLevel=%q", port, logLevel)
	}
}

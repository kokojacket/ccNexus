package api

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lich0821/ccNexus/internal/config"
	"github.com/lich0821/ccNexus/internal/storage"
)

func TestStatsEventUsesSnapshotAndActualCurrentEndpoint(t *testing.T) {
	h := newEndpointTestHandler(t)
	h.config.UpdateEndpoints([]config.Endpoint{
		{Name: "first", APIUrl: "https://first.example", Enabled: true},
		{Name: "current", APIUrl: "https://current.example", Enabled: true},
	})
	if err := h.proxy.SetCurrentEndpoint("current"); err != nil {
		t.Fatalf("set current endpoint: %v", err)
	}
	if err := h.storage.RecordDailyStat(&storage.DailyStat{
		EndpointName: "current",
		Date:         time.Now().Format("2006-01-02"),
		Requests:     3,
		Errors:       1,
		InputTokens:  11,
		OutputTokens: 7,
		DeviceID:     "test",
	}); err != nil {
		t.Fatalf("record stats: %v", err)
	}

	event := h.newStatsEvent(time.Unix(123, 0))
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	encoded := string(data)

	if !strings.Contains(encoded, `"currentEndpoint":"current"`) {
		t.Fatalf("event did not use proxy current endpoint: %s", encoded)
	}
	if !strings.Contains(encoded, `"TotalRequests":4`) || !strings.Contains(encoded, `"TotalSuccess":3`) || !strings.Contains(encoded, `"requests":4`) {
		t.Fatalf("event did not contain the serializable stats snapshot: %s", encoded)
	}
	if strings.Contains(encoded, `"lastUsed"`) {
		t.Fatalf("event must not fabricate a last-used timestamp: %s", encoded)
	}
}

func TestPeriodStatsIncludeErrorOnlyEndpoints(t *testing.T) {
	h := newEndpointTestHandler(t)
	today := time.Now().Format("2006-01-02")
	if err := h.storage.RecordDailyStat(&storage.DailyStat{
		EndpointName: "failed-only", Date: today, Errors: 2, DeviceID: "test",
	}); err != nil {
		t.Fatalf("record error stats: %v", err)
	}

	stats, err := h.getStatsForPeriod(today, today)
	if err != nil {
		t.Fatalf("get period stats: %v", err)
	}
	if stats["totalRequests"] != 2 || stats["totalSuccess"] != 0 || stats["totalErrors"] != 2 {
		t.Fatalf("unexpected totals: %+v", stats)
	}
	endpoints := stats["endpoints"].(map[string]interface{})
	if endpoints["failed-only"] == nil {
		t.Fatalf("error-only endpoint missing: %+v", stats)
	}
}

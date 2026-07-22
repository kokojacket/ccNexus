package webui

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func readUIAsset(t *testing.T, path string) string {
	t.Helper()
	data, err := uiFS.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func requireUIAssetContains(t *testing.T, path string, values ...string) {
	t.Helper()
	content := readUIAsset(t, path)
	for _, value := range values {
		if !strings.Contains(content, value) {
			t.Errorf("%s must contain %q", path, value)
		}
	}
}

func TestEndpointUIHasScopedActionsAndAccessibleReordering(t *testing.T) {
	requireUIAssetContains(t, "ui/js/components/endpoints.js",
		"this.actionVersion",
		"this.reorderQueue",
		"move-up-btn",
		"move-down-btn",
		"configured ? t('endpoints.tokenConfigured')",
	)
}

func TestEndpointAuthModesDriveFieldsAndCapabilities(t *testing.T) {
	requireUIAssetContains(t, "ui/js/components/endpoints.js",
		"const usesCodexTokenPool = authMode === 'codex_token_pool'",
		"apiUrl.readOnly = usesCodexTokenPool",
		"transformer.disabled = usesCodexTokenPool",
		"fetchModelsButton.hidden = usesCodexTokenPool",
		"apiUrl.value = 'https://chatgpt.com/backend-api/codex'",
		"transformer.value = 'openai2'",
		"endpoints.codexTokenPoolModeHint",
		"endpoints.codexModelsUnavailable",
	)
}

func TestEndpointMutationsConvergeAndCloneReportsAfterSave(t *testing.T) {
	content := readUIAsset(t, "ui/js/components/endpoints.js")
	for _, value := range []string{
		"this.mutationQueue",
		"queueMutation",
		"\\(Copy\\)|（副本）",
	} {
		if !strings.Contains(content, value) {
			t.Errorf("endpoints.js must contain %q", value)
		}
	}
	if strings.Count(content, "queueMutation(") < 8 {
		t.Error("all endpoint and credential mutations must use the shared mutation queue")
	}
	testStart := strings.Index(content, "async testEndpoint(")
	if testStart < 0 {
		t.Fatal("testEndpoint block not found")
	}
	testEnd := strings.Index(content[testStart:], "showTestResultModal(")
	if testEnd < 0 || !strings.Contains(content[testStart:testStart+testEnd], "this.renderTable()") {
		t.Error("successful endpoint tests must update the visible status before opening the result modal")
	}
	cloneStart := strings.Index(content, "\n    cloneEndpoint(")
	if cloneStart < 0 {
		t.Fatal("cloneEndpoint block not found")
	}
	cloneEnd := strings.Index(content[cloneStart:], "async showTokenPoolModal(")
	if cloneEnd < 0 {
		t.Fatal("cloneEndpoint block not found")
	}
	if strings.Contains(content[cloneStart:cloneStart+cloneEnd], "notifications.success") {
		t.Error("opening the clone form must not report a successful clone")
	}
}

func TestTestingAndStatsDiscardStaleResponses(t *testing.T) {
	requireUIAssetContains(t, "ui/js/components/testing.js",
		"this.requestVersion",
		"this.lastResult",
		"testing.endpoint",
		"message || t(success ? 'testing.noResponse' : 'testing.unknownError')",
	)
	requireUIAssetContains(t, "ui/js/components/stats.js",
		"aria-pressed",
		"renderLoading",
		"renderError",
	)
}

func TestDashboardDiscardsOlderRealtimeResponse(t *testing.T) {
	workingDir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve package directory: %v", err)
	}
	script := `
import { pathToFileURL } from 'node:url';
globalThis.document = { getElementById() { return null; } };
globalThis.window = { addEventListener() {} };
const dashboardModule = await import(pathToFileURL(process.argv[1]));
const apiModule = await import(pathToFileURL(process.argv[2]));
const stateModule = await import(pathToFileURL(process.argv[3]));
const pending = [];
const rendered = [];
apiModule.api.getStatsDaily = () => new Promise(resolve => pending.push(resolve));
dashboardModule.dashboard.renderChart = value => rendered.push(value.marker);
stateModule.state.update('currentView', 'dashboard');
const older = dashboardModule.dashboard.refreshRealtime();
const newer = dashboardModule.dashboard.refreshRealtime();
pending[1]({ marker: 'newer' });
await newer;
pending[0]({ marker: 'older' });
await older;
if (dashboardModule.dashboard.dailyStats?.marker !== 'newer' || rendered.join(',') !== 'newer') {
  throw new Error(JSON.stringify({ final: dashboardModule.dashboard.dailyStats, rendered }));
}
`
	cmd := exec.Command(
		"node", "--input-type=module", "-e", script,
		filepath.Join(workingDir, "ui/js/components/dashboard.js"),
		filepath.Join(workingDir, "ui/js/api.js"),
		filepath.Join(workingDir, "ui/js/state.js"),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("dashboard stale-response harness failed: %v\n%s", err, output)
	}
}

func TestModalNavigationAndThemeAccessibilityContracts(t *testing.T) {
	requireUIAssetContains(t, "ui/js/utils/modal.js",
		"inert = true",
		"removeAttribute('aria-hidden')",
	)
	requireUIAssetContains(t, "ui/js/router.js", "aria-current")
	requireUIAssetContains(t, "ui/js/main.js",
		"themeChanged",
		"document.title =",
	)
	requireUIAssetContains(t, "ui/js/components/dashboard.js",
		"themeChanged",
		"TotalSuccess",
		"--text-secondary",
	)
	requireUIAssetContains(t, "ui/css/main.css", "--on-primary")
	requireUIAssetContains(t, "ui/css/components.css", "max-width: 820px")
}

func TestFrontendFallbackErrorsAreLocalized(t *testing.T) {
	requireUIAssetContains(t, "ui/js/api.js",
		"import { t }",
		"errors.invalidJSONResponse",
		"errors.requestFailed",
	)
}

func TestRealtimeRefreshIsCoalescedAndNetworkErrorsAreLocalized(t *testing.T) {
	requireUIAssetContains(t, "ui/js/main.js",
		"let realtimeDisconnected = false",
		"let realtimeRefreshTimer = null",
		"eventSource.onopen = () =>",
		"notifications.warning(t('notifications.realtimeDisconnected'))",
		"notifications.success(t('notifications.realtimeRestored'))",
		"clearTimeout(realtimeRefreshTimer)",
		"dashboard.refreshRealtime()",
		"stats.refreshRealtime()",
		"state.update('currentEndpoint'",
	)

	dashboard := readUIAsset(t, "ui/js/components/dashboard.js")
	refreshStart := strings.Index(dashboard, "async refreshRealtime()")
	refreshEnd := strings.Index(dashboard, "\n    updateStats(")
	if refreshStart < 0 || refreshEnd < refreshStart {
		t.Fatal("dashboard realtime refresh block not found")
	}
	dashboardRefresh := dashboard[refreshStart:refreshEnd]
	if strings.Count(dashboardRefresh, "api.") != 1 || !strings.Contains(dashboardRefresh, "api.getStatsDaily()") {
		t.Error("dashboard realtime refresh must issue only the daily activity request")
	}
	if !strings.Contains(dashboardRefresh, "state.get('currentView') !== 'dashboard'") {
		t.Error("dashboard realtime refresh must skip work when hidden")
	}

	requireUIAssetContains(t, "ui/js/components/stats.js",
		"async refreshRealtime()",
		"state.get('currentView') !== 'stats'",
		"this.loadStats(this.period, false)",
	)
	requireUIAssetContains(t, "ui/js/api.js",
		"error instanceof TypeError",
		"errors.networkError",
	)
	requireUIAssetContains(t, "ui/js/i18n/zh-CN.js",
		"networkError:",
		"realtimeDisconnected:",
		"realtimeRestored:",
	)
	requireUIAssetContains(t, "ui/js/i18n/en.js",
		"networkError:",
		"realtimeDisconnected:",
		"realtimeRestored:",
	)
}

func TestThemeModalAndMobileTableRegressions(t *testing.T) {
	dashboard := readUIAsset(t, "ui/js/components/dashboard.js")
	if strings.Count(dashboard, "this.dailyStats = null") < 2 {
		t.Error("dashboard must clear cached chart data when its page lifecycle resets")
	}
	modal := readUIAsset(t, "ui/js/utils/modal.js")
	if strings.Contains(modal, "queueMicrotask") {
		t.Error("modal focus must move before the previous layer becomes hidden")
	}
	focusIndex := strings.LastIndex(modal, "(target || dialog).focus();")
	syncIndex := strings.LastIndex(modal, "syncModalStack();")
	if focusIndex < 0 || syncIndex < focusIndex {
		t.Error("new modal must receive focus before lower modal layers become hidden")
	}
	requireUIAssetContains(t, "ui/css/components.css",
		".inline-alert.error {\n    border-color: var(--danger-color);\n    background: var(--danger-soft);\n    color: var(--danger-text);",
		".table td[colspan]",
	)
}

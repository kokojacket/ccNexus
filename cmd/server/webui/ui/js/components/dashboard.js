import { api } from '../api.js';
import { state } from '../state.js';
import { notifications } from '../utils/notifications.js';
import { escapeHtml, formatNumber, formatTokens } from '../utils/formatters.js';
import { t } from '../utils/i18n.js';

class Dashboard {
    constructor() {
        this.container = document.getElementById('view-container');
        this.activityChart = null;
        this.dailyStats = null;
        this.renderVersion = 0;
        this.dailyRequestVersion = 0;
        state.subscribe('stats', stats => {
            if (state.get('currentView') === 'dashboard' && document.getElementById('stat-requests')) {
                this.updateStats(stats);
            }
        });
        // 监听语言切换
        window.addEventListener('languageChanged', () => {
            if (state.get('currentView') === 'dashboard') {
                this.render();
            }
        });
        window.addEventListener('themeChanged', () => {
            if (state.get('currentView') === 'dashboard' && this.dailyStats) {
                this.renderChart(this.dailyStats);
            }
        });
    }

    async render() {
        const renderVersion = ++this.renderVersion;
        this.dailyStats = null;
        this.destroyChart();
        this.container.innerHTML = `
            <div class="dashboard">
                <div class="page-header"><h1>${t('dashboard.title')}</h1></div>
                <div id="stats-cards" class="grid grid-cols-4 mt-3">
                    <div class="stat-card">
                        <div class="stat-label">${t('dashboard.totalRequests')}</div>
                        <div class="stat-value" id="stat-requests">-</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">${t('dashboard.successRate')}</div>
                        <div class="stat-value" id="stat-success">-</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">${t('dashboard.inputTokens')}</div>
                        <div class="stat-value" id="stat-input-tokens">-</div>
                    </div>
                    <div class="stat-card">
                        <div class="stat-label">${t('dashboard.outputTokens')}</div>
                        <div class="stat-value" id="stat-output-tokens">-</div>
                    </div>
                </div>

                <div class="grid grid-cols-2 mt-4">
                    <div class="card">
                        <div class="card-header">
                            <h3 class="card-title">${t('dashboard.activeEndpoints')}</h3>
                        </div>
                        <div class="card-body">
                            <div id="endpoints-list"></div>
                        </div>
                    </div>

                    <div class="card">
                        <div class="card-header">
                            <h3 class="card-title">${t('dashboard.recentActivity')}</h3>
                        </div>
                        <div class="card-body">
                            <div class="chart-shell" id="activity-chart-shell">
                                <canvas id="activity-chart" role="img" aria-label="${t('dashboard.recentActivity')}"></canvas>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        `;

        await this.loadData(renderVersion);
    }

    async loadData(renderVersion = this.renderVersion) {
        const dailyRequestVersion = ++this.dailyRequestVersion;
        try {
            const [stats, endpointsData, dailyStats] = await Promise.all([
                api.getStatsSummary(),
                api.getEndpoints(),
                api.getStatsDaily()
            ]);
            if (renderVersion !== this.renderVersion || state.get('currentView') !== 'dashboard') {
                return;
            }
            this.updateStats(stats);
            this.updateEndpoints(endpointsData.endpoints);
            if (dailyRequestVersion === this.dailyRequestVersion) {
                this.dailyStats = dailyStats;
                this.renderChart(this.dailyStats);
            }
        } catch (error) {
            if (renderVersion === this.renderVersion && state.get('currentView') === 'dashboard') {
                notifications.error(`${t('dashboard.failedToLoad')}: ${error.message}`);
            }
        }
    }

    async refreshRealtime() {
        if (state.get('currentView') !== 'dashboard') {
            return;
        }
        const renderVersion = this.renderVersion;
        const dailyRequestVersion = ++this.dailyRequestVersion;
        try {
            const dailyStats = await api.getStatsDaily();
            if (dailyRequestVersion !== this.dailyRequestVersion || renderVersion !== this.renderVersion || state.get('currentView') !== 'dashboard') {
                return;
            }
            this.dailyStats = dailyStats;
            this.renderChart(this.dailyStats);
        } catch (error) {
            if (renderVersion === this.renderVersion && state.get('currentView') === 'dashboard') {
                console.error('Failed to refresh dashboard activity:', error);
            }
        }
    }

    updateStats(stats) {
        const totalRequests = stats.TotalRequests || 0;
        const totalSuccess = stats.TotalSuccess || 0;
        const successRate = totalRequests > 0
            ? (totalSuccess / totalRequests * 100).toFixed(1)
            : 0;

        const requests = document.getElementById('stat-requests');
        const success = document.getElementById('stat-success');
        const inputTokens = document.getElementById('stat-input-tokens');
        const outputTokens = document.getElementById('stat-output-tokens');
        if (!requests || !success || !inputTokens || !outputTokens) {
            return;
        }
        requests.textContent = formatNumber(totalRequests);
        success.textContent = successRate + '%';
        inputTokens.textContent = formatTokens(stats.TotalInputTokens || 0);
        outputTokens.textContent = formatTokens(stats.TotalOutputTokens || 0);
    }

    updateEndpoints(endpoints) {
        const container = document.getElementById('endpoints-list');
        if (!container) {
            return;
        }

        if (!endpoints || endpoints.length === 0) {
            container.innerHTML = `<div class="empty-state"><p>${t('dashboard.noEndpoints')}</p></div>`;
            return;
        }

        const enabledEndpoints = endpoints.filter(ep => ep.enabled);

        if (enabledEndpoints.length === 0) {
            container.innerHTML = `<div class="empty-state"><p>${t('dashboard.noEnabledEndpoints')}</p></div>`;
            return;
        }

        container.innerHTML = `
            <div class="table-container table-responsive">
                <table class="table">
                    <thead>
                        <tr>
                            <th>${t('common.name')}</th>
                            <th>${t('endpoints.transformer')}</th>
                            <th>${t('common.status')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${enabledEndpoints.map(ep => `
                            <tr>
                                <td data-label="${t('common.name')}">${escapeHtml(ep.name)}</td>
                                <td data-label="${t('endpoints.transformer')}">${escapeHtml(ep.transformer)}</td>
                                <td data-label="${t('common.status')}">
                                    <span class="status-indicator online"></span>
                                    <span class="badge badge-success">${t('common.active')}</span>
                                </td>
                            </tr>
                        `).join('')}
                    </tbody>
                </table>
            </div>
        `;
    }

    renderChart(dailyStats) {
        const canvas = document.getElementById('activity-chart');
        const shell = document.getElementById('activity-chart-shell');
        this.destroyChart();
        if (!canvas || !shell || typeof window.Chart !== 'function') {
            this.renderChartFallback(shell, t('dashboard.chartUnavailable'));
            return;
        }
        const ctx = canvas.getContext('2d');
        if (!ctx) {
            this.renderChartFallback(shell, t('dashboard.chartUnavailable'));
            return;
        }

        // Simple bar chart showing requests
        const stats = dailyStats?.stats || {};
        const endpoints = Object.keys(stats.endpoints || {});
        const requests = endpoints.map(ep => stats.endpoints[ep].requests || 0);
        if (endpoints.length === 0) {
            this.renderChartFallback(shell, t('dashboard.noActivityData'));
            return;
        }
        const styles = getComputedStyle(document.body);
        const primaryColor = styles.getPropertyValue('--primary-color').trim();
        const primaryHover = styles.getPropertyValue('--primary-hover').trim();
        const textColor = styles.getPropertyValue('--text-secondary').trim();
        const gridColor = styles.getPropertyValue('--border-color').trim();

        this.activityChart = new window.Chart(ctx, {
            type: 'bar',
            data: {
                labels: endpoints,
                datasets: [{
                    label: t('dashboard.requests'),
                    data: requests,
                    backgroundColor: primaryColor,
                    borderColor: primaryHover,
                    borderWidth: 1
                }]
            },
            options: {
                responsive: true,
                maintainAspectRatio: true,
                plugins: {
                    legend: {
                        display: false
                    }
                },
                scales: {
                    x: {
                        ticks: { color: textColor },
                        grid: { color: gridColor }
                    },
                    y: {
                        beginAtZero: true,
                        ticks: { color: textColor },
                        grid: { color: gridColor }
                    }
                }
            }
        });
    }

    renderChartFallback(shell, message) {
        if (!shell) {
            return;
        }
        shell.replaceChildren();
        const alert = document.createElement('div');
        alert.className = 'inline-alert';
        alert.textContent = message;
        shell.appendChild(alert);
    }

    destroyChart() {
        this.activityChart?.destroy();
        this.activityChart = null;
    }

    destroy() {
        this.renderVersion++;
        this.dailyRequestVersion++;
        this.dailyStats = null;
        this.destroyChart();
    }
}

export const dashboard = new Dashboard();

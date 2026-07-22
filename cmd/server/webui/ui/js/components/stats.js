import { api } from '../api.js';
import { state } from '../state.js';
import { notifications } from '../utils/notifications.js';
import { escapeHtml, formatNumber, formatTokens } from '../utils/formatters.js';
import { t } from '../utils/i18n.js';

class Stats {
    constructor() {
        this.container = document.getElementById('view-container');
        this.period = 'daily';
        this.requestVersion = 0;
        // 监听语言切换
        window.addEventListener('languageChanged', () => {
            if (state.get('currentView') === 'stats') {
                this.render();
            }
        });
    }

    async render() {
        this.container.innerHTML = `
            <div class="stats">
                <div class="page-header">
                    <h1>${t('stats.title')}</h1>
                    <div class="actions">
                        <button class="btn btn-sm btn-primary period-btn active" type="button" data-period="daily" aria-pressed="true">${t('stats.daily')}</button>
                        <button class="btn btn-sm btn-secondary period-btn" type="button" data-period="weekly" aria-pressed="false">${t('stats.weekly')}</button>
                        <button class="btn btn-sm btn-secondary period-btn" type="button" data-period="monthly" aria-pressed="false">${t('stats.monthly')}</button>
                    </div>
                </div>

                <div id="stats-content"></div>
            </div>
        `;

        this.updatePeriodButtons();
        document.querySelectorAll('.period-btn').forEach(btn => {
            btn.addEventListener('click', () => {
                this.period = btn.dataset.period;
                this.updatePeriodButtons();
                this.loadStats(btn.dataset.period);
            });
        });

        await this.loadStats(this.period);
    }

    async loadStats(period, showLoading = true) {
        this.period = period;
        const requestVersion = ++this.requestVersion;
        if (showLoading) {
            this.renderLoading();
        }
        try {
            let data;
            switch (period) {
                case 'daily':
                    data = await api.getStatsDaily();
                    break;
                case 'weekly':
                    data = await api.getStatsWeekly();
                    break;
                case 'monthly':
                    data = await api.getStatsMonthly();
                    break;
            }

            if (requestVersion !== this.requestVersion || state.get('currentView') !== 'stats') {
                return;
            }
            this.renderStats(data);
        } catch (error) {
            if (requestVersion === this.requestVersion && state.get('currentView') === 'stats') {
                this.renderError(error.message);
                notifications.error(`${t('stats.failedToLoad')}: ${error.message}`);
            }
        }
    }

    async refreshRealtime() {
        if (state.get('currentView') !== 'stats') {
            return;
        }
        await this.loadStats(this.period, false);
    }

    updatePeriodButtons() {
        document.querySelectorAll('.period-btn').forEach(button => {
            const active = button.dataset.period === this.period;
            button.classList.toggle('btn-primary', active);
            button.classList.toggle('active', active);
            button.classList.toggle('btn-secondary', !active);
            button.setAttribute('aria-pressed', String(active));
        });
    }

    renderLoading() {
        const container = document.getElementById('stats-content');
        if (!container) {
            return;
        }
        container.innerHTML = `<div class="flex-center"><div class="spinner" aria-hidden="true"></div><span class="text-muted">${t('common.loading')}</span></div>`;
    }

    renderError(message) {
        const container = document.getElementById('stats-content');
        if (!container) {
            return;
        }
        container.innerHTML = '<div class="inline-alert error" id="stats-error"></div>';
        container.querySelector('#stats-error').textContent = `${t('stats.failedToLoad')}: ${message}`;
    }

    renderStats(data) {
        const stats = data.stats || {};
        const container = document.getElementById('stats-content');
        if (!container) {
            return;
        }

        container.innerHTML = `
            <div class="grid grid-cols-4 mb-4">
                <div class="stat-card">
                    <div class="stat-label">${t('stats.totalRequests')}</div>
                    <div class="stat-value">${formatNumber(stats.totalRequests || 0)}</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">${t('stats.successful')}</div>
                    <div class="stat-value">${formatNumber(stats.totalSuccess || 0)}</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">${t('stats.errors')}</div>
                    <div class="stat-value">${formatNumber(stats.totalErrors || 0)}</div>
                </div>
                <div class="stat-card">
                    <div class="stat-label">${t('stats.totalTokens')}</div>
                    <div class="stat-value">${formatTokens((stats.totalInputTokens || 0) + (stats.totalOutputTokens || 0))}</div>
                </div>
            </div>

            <div class="card">
                <div class="card-header">
                    <h3 class="card-title">${t('stats.endpointBreakdown')}</h3>
                </div>
                <div class="card-body">
                    ${this.renderEndpointTable(stats.endpoints || {})}
                </div>
            </div>
        `;
    }

    renderEndpointTable(endpoints) {
        const endpointNames = Object.keys(endpoints);

        if (endpointNames.length === 0) {
            return `<div class="empty-state"><p>${t('stats.noDataAvailable')}</p></div>`;
        }

        return `
            <div class="table-container table-responsive">
                <table class="table">
                    <thead>
                        <tr>
                            <th>${t('stats.endpoint')}</th>
                            <th>${t('stats.requests')}</th>
                            <th>${t('stats.errors')}</th>
                            <th>${t('stats.inputTokens')}</th>
                            <th>${t('stats.outputTokens')}</th>
                        </tr>
                    </thead>
                    <tbody>
                        ${endpointNames.map(name => {
                            const ep = endpoints[name];
                            return `
                                <tr>
                                    <td data-label="${t('stats.endpoint')}"><strong>${escapeHtml(name)}</strong></td>
                                    <td data-label="${t('stats.requests')}">${formatNumber(ep.requests || 0)}</td>
                                    <td data-label="${t('stats.errors')}">${formatNumber(ep.errors || 0)}</td>
                                    <td data-label="${t('stats.inputTokens')}">${formatTokens(ep.inputTokens || 0)}</td>
                                    <td data-label="${t('stats.outputTokens')}">${formatTokens(ep.outputTokens || 0)}</td>
                                </tr>
                            `;
                        }).join('')}
                    </tbody>
                </table>
            </div>
        `;
    }

    destroy() {
        this.requestVersion++;
    }
}

export const stats = new Stats();

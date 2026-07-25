import { api } from '../api.js';
import { notifications } from '../utils/notifications.js';
import { escapeHtml, formatNumber, formatTokens } from '../utils/formatters.js';
import { t } from '../utils/i18n.js';

class Stats {
    constructor() {
        this.container = document.getElementById('statistics-panel');
        this.period = 'daily';
        this.requestVersion = 0;
        window.addEventListener('languageChanged', () => this.render());
    }

    async render() {
        this.container.innerHTML = `
            <div class="section-header">
                <h2><span aria-hidden="true">📊</span> ${t('stats.title')}</h2>
                <div class="stats-tabs" role="group" aria-label="${t('stats.title')}">
                    <button class="stats-tab-btn" type="button" data-period="daily">${t('stats.daily')}</button>
                    <button class="stats-tab-btn" type="button" data-period="weekly">${t('stats.weekly')}</button>
                    <button class="stats-tab-btn" type="button" data-period="monthly">${t('stats.monthly')}</button>
                </div>
            </div>
            <div id="stats-content"></div>`;

        this.updatePeriodButtons();
        this.container.querySelectorAll('.stats-tab-btn').forEach(button => {
            button.addEventListener('click', () => {
                this.period = button.dataset.period;
                this.updatePeriodButtons();
                this.loadStats(this.period);
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
            const statsRequest = period === 'weekly'
                ? api.getStatsWeekly()
                : period === 'monthly'
                    ? api.getStatsMonthly()
                    : api.getStatsDaily();
            const [data, endpointData] = await Promise.all([statsRequest, api.getEndpoints()]);
            if (requestVersion !== this.requestVersion) {
                return;
            }
            this.renderStats(data, endpointData.endpoints || []);
        } catch (error) {
            if (requestVersion === this.requestVersion) {
                this.renderError(error.message);
                notifications.error(`${t('stats.failedToLoad')}: ${error.message}`);
            }
        }
    }

    async refreshRealtime() {
        await this.loadStats(this.period, false);
    }

    updatePeriodButtons() {
        this.container.querySelectorAll('.stats-tab-btn').forEach(button => {
            const active = button.dataset.period === this.period;
            button.classList.toggle('active', active);
            button.setAttribute('aria-pressed', String(active));
        });
    }

    renderLoading() {
        const container = document.getElementById('stats-content');
        if (container) {
            container.innerHTML = `<div class="loading-state"><div class="spinner" aria-hidden="true"></div><span>${t('common.loading')}</span></div>`;
        }
    }

    renderError(message) {
        const container = document.getElementById('stats-content');
        if (container) {
            container.innerHTML = '<div class="inline-alert error" id="stats-error"></div>';
            container.querySelector('#stats-error').textContent = `${t('stats.failedToLoad')}: ${message}`;
        }
    }

    renderStats(data, endpoints) {
        const stats = data.stats || {};
        const container = document.getElementById('stats-content');
        if (!container) {
            return;
        }
        const activeEndpoints = endpoints.filter(endpoint => endpoint.enabled).length;
        const totalRequests = stats.totalRequests || 0;
        const totalSuccess = stats.totalSuccess || 0;
        const totalErrors = stats.totalErrors || 0;
        const inputTokens = stats.totalInputTokens || 0;
        const outputTokens = stats.totalOutputTokens || 0;

        container.innerHTML = `
            <div class="stats-grid">
                <div class="stat-box">
                    <div class="stat-label">${t('stats.endpoints')}</div>
                    <div class="stat-value"><span>${formatNumber(activeEndpoints)}</span><span class="stat-secondary"> / ${formatNumber(endpoints.length)}</span></div>
                    <div class="stat-detail">${t('stats.activeTotal')}</div>
                </div>
                <div class="stat-box">
                    <div class="stat-label">${t('stats.totalRequests')}</div>
                    <div class="stat-value">${formatNumber(totalRequests)}</div>
                    <div class="stat-detail">${formatNumber(totalSuccess)} ${t('stats.successful')} / ${formatNumber(totalErrors)} ${t('stats.errors')}</div>
                </div>
                <div class="stat-box">
                    <div class="stat-label">${t('stats.totalTokens')}</div>
                    <div class="stat-value">${formatTokens(inputTokens + outputTokens)}</div>
                    <div class="stat-detail">${formatTokens(inputTokens)} ${t('stats.input')} / ${formatTokens(outputTokens)} ${t('stats.output')}</div>
                </div>
            </div>
            <details class="stats-details">
                <summary>${t('stats.endpointBreakdown')}</summary>
                ${this.renderEndpointTable(stats.endpoints || {})}
            </details>`;
    }

    renderEndpointTable(endpoints) {
        const endpointNames = Object.keys(endpoints);
        if (endpointNames.length === 0) {
            return `<div class="empty-state compact"><p>${t('stats.noDataAvailable')}</p></div>`;
        }
        return `
            <div class="table-container table-responsive">
                <table class="table">
                    <thead><tr>
                        <th>${t('stats.endpoint')}</th>
                        <th>${t('stats.requests')}</th>
                        <th>${t('stats.errors')}</th>
                        <th>${t('stats.inputTokens')}</th>
                        <th>${t('stats.outputTokens')}</th>
                    </tr></thead>
                    <tbody>${endpointNames.map(name => {
                        const endpoint = endpoints[name];
                        return `<tr>
                            <td data-label="${t('stats.endpoint')}"><strong>${escapeHtml(name)}</strong></td>
                            <td data-label="${t('stats.requests')}">${formatNumber(endpoint.requests || 0)}</td>
                            <td data-label="${t('stats.errors')}">${formatNumber(endpoint.errors || 0)}</td>
                            <td data-label="${t('stats.inputTokens')}">${formatTokens(endpoint.inputTokens || 0)}</td>
                            <td data-label="${t('stats.outputTokens')}">${formatTokens(endpoint.outputTokens || 0)}</td>
                        </tr>`;
                    }).join('')}</tbody>
                </table>
            </div>`;
    }

    destroy() {
        this.requestVersion++;
    }
}

export const stats = new Stats();

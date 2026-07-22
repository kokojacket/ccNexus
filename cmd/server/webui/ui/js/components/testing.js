import { api } from '../api.js';
import { state } from '../state.js';
import { notifications } from '../utils/notifications.js';
import { escapeHtml } from '../utils/formatters.js';
import { t } from '../utils/i18n.js';

class Testing {
    constructor() {
        this.container = document.getElementById('view-container');
        this.endpoints = [];
        this.selectedEndpoint = '';
        this.renderVersion = 0;
        this.requestVersion = 0;
        this.lastResult = null;
        // 监听语言切换
        window.addEventListener('languageChanged', () => {
            if (state.get('currentView') === 'testing') {
                this.selectedEndpoint = document.getElementById('test-endpoint-select')?.value || this.selectedEndpoint;
                this.render();
            }
        });
    }

    async render() {
        const renderVersion = ++this.renderVersion;
        this.container.innerHTML = `
            <div class="testing">
                <div class="page-header"><h1>${t('testing.title')}</h1></div>

                <div class="card mt-3">
                    <div class="card-body">
                        <div class="form-group">
                            <label class="form-label" for="test-endpoint-select">${t('testing.selectEndpoint')}</label>
                            <select class="form-select" id="test-endpoint-select">
                                <option value="">${t('common.loading')}</option>
                            </select>
                        </div>

                        <div class="form-group">
                            <button class="btn btn-primary" id="test-btn" type="button">${t('testing.runTest')}</button>
                        </div>

                        <div id="test-result" class="mt-3" hidden></div>
                    </div>
                </div>
            </div>
        `;

        document.getElementById('test-btn').addEventListener('click', () => this.runTest());
        document.getElementById('test-endpoint-select').addEventListener('change', event => {
            this.selectedEndpoint = event.currentTarget.value;
        });

        if (this.lastResult) {
            this.renderResult(
                this.lastResult.success,
                this.lastResult.latency,
                this.lastResult.message,
                this.lastResult.endpointName
            );
        }

        await this.loadEndpoints(renderVersion);
    }

    async loadEndpoints(renderVersion = this.renderVersion) {
        try {
            const data = await api.getEndpoints();
            if (renderVersion !== this.renderVersion || state.get('currentView') !== 'testing') {
                return;
            }
            this.endpoints = data.endpoints || [];

            const select = document.getElementById('test-endpoint-select');
            if (!select) {
                return;
            }
            const enabledEndpoints = this.endpoints.filter(ep => ep.enabled);

            if (enabledEndpoints.length === 0) {
                select.innerHTML = `<option value="">${t('testing.noEnabledEndpoints')}</option>`;
                return;
            }

            select.innerHTML = enabledEndpoints.map(ep =>
                `<option value="${escapeHtml(ep.name)}">${escapeHtml(ep.name)}</option>`
            ).join('');
            if (enabledEndpoints.some(endpoint => endpoint.name === this.selectedEndpoint)) {
                select.value = this.selectedEndpoint;
            } else {
                this.selectedEndpoint = select.value;
            }
        } catch (error) {
            if (renderVersion === this.renderVersion && state.get('currentView') === 'testing') {
                notifications.error(`${t('testing.failedToLoadEndpoints')}: ${error.message}`);
            }
        }
    }

    async runTest() {
        const select = document.getElementById('test-endpoint-select');
        if (!select) {
            return;
        }
        const endpointName = select.value;
        this.selectedEndpoint = endpointName;

        if (!endpointName) {
            notifications.warning(t('testing.pleaseSelectEndpoint'));
            return;
        }

        const resultDiv = document.getElementById('test-result');
        if (!resultDiv) {
            return;
        }
        const requestVersion = ++this.requestVersion;
        this.lastResult = null;
        resultDiv.hidden = false;
        resultDiv.innerHTML = '<div class="flex-center"><div class="spinner"></div></div>';
        const renderVersion = this.renderVersion;

        try {
            const result = await api.testEndpoint(endpointName);
            if (requestVersion !== this.requestVersion || renderVersion !== this.renderVersion || state.get('currentView') !== 'testing') {
                return;
            }

            if (result.success) {
                this.lastResult = {
                    endpointName,
                    success: true,
                    latency: result.latency,
                    message: result.response ?? ''
                };
                this.renderResult(true, this.lastResult.latency, this.lastResult.message, endpointName);
                notifications.success(t('testing.testCompletedSuccessfully'));
            } else {
                this.lastResult = {
                    endpointName,
                    success: false,
                    latency: result.latency,
                    message: result.error ?? ''
                };
                this.renderResult(false, this.lastResult.latency, this.lastResult.message, endpointName);
                notifications.error(t('testing.testFailed'));
            }
        } catch (error) {
            if (requestVersion !== this.requestVersion || renderVersion !== this.renderVersion || state.get('currentView') !== 'testing') {
                return;
            }
            this.lastResult = {
                endpointName,
                success: false,
                latency: null,
                message: error.message
            };
            this.renderResult(false, null, error.message, endpointName);
            notifications.error(`${t('testing.testFailed')}: ${error.message}`);
        }
    }

    renderResult(success, latency, message, endpointName) {
        const resultDiv = document.getElementById('test-result');
        if (!resultDiv) {
            return;
        }
        const displayMessage = message || t(success ? 'testing.noResponse' : 'testing.unknownError');
        resultDiv.hidden = false;
        resultDiv.innerHTML = `
            <div class="inline-alert test-result${success ? '' : ' error'}">
                <div class="mb-2 test-result-meta">
                    <span class="badge" id="test-result-status"></span>
                    <span class="text-muted" id="test-result-latency"></span>
                </div>
                <div class="mb-2">
                    <strong>${t('testing.endpoint')}:</strong>
                    <span id="test-result-endpoint"></span>
                </div>
                <div>
                    <strong id="test-result-label"></strong>
                    <div class="code-block mt-1" id="test-result-message"></div>
                </div>
            </div>
        `;
        const badge = resultDiv.querySelector('#test-result-status');
        badge.classList.add(success ? 'badge-success' : 'badge-danger');
        badge.textContent = success ? t('common.success') : t('common.error');
        resultDiv.querySelector('#test-result-endpoint').textContent = endpointName;
        resultDiv.querySelector('#test-result-latency').textContent = latency == null ? '' : `${t('testing.latency')}: ${latency}ms`;
        resultDiv.querySelector('#test-result-label').textContent = `${t(success ? 'testing.response' : 'testing.error')}:`;
        resultDiv.querySelector('#test-result-message').textContent = String(displayMessage);
    }

    destroy() {
        this.renderVersion++;
        this.requestVersion++;
    }
}

export const testing = new Testing();

import { api } from '../api.js';
import { state } from '../state.js';
import { notifications } from '../utils/notifications.js';
import { escapeHtml, formatDateTime, getTransformerLabel, getStatusBadge } from '../utils/formatters.js';
import { t } from '../utils/i18n.js';
import { activateModal, closeAllModals, confirmDialog } from '../utils/modal.js';

const tokenPoolAuthModes = new Set(['token_pool', 'codex_token_pool']);

class Endpoints {
    constructor() {
        this.container = document.getElementById('view-container');
        this.endpoints = [];
        this.tokenPools = {};
        this.currentEndpoint = null;
        this.draggedIndex = null;
        this.currentTokenPoolEndpoint = null;
        this.renderVersion = 0;
        this.actionVersion = 0;
        this.mutationVersion = 0;
        this.mutationQueue = Promise.resolve();
        this.reorderQueue = Promise.resolve();
        state.subscribe('currentEndpoint', currentEndpoint => {
            if (state.get('currentView') === 'endpoints' && currentEndpoint !== this.currentEndpoint) {
                this.currentEndpoint = currentEndpoint;
                this.renderTable();
            }
        });
        // 监听语言切换
        window.addEventListener('languageChanged', () => {
            if (state.get('currentView') === 'endpoints') {
                closeAllModals();
                this.render();
            }
        });
    }

    async render() {
        const renderVersion = ++this.renderVersion;
        this.invalidateActions();
        this.container.innerHTML = `
            <div class="endpoints">
                <div class="page-header">
                    <h1>${t('endpoints.title')}</h1>
                    <button class="btn btn-primary" id="add-endpoint-btn" type="button">
                        <span>+ ${t('endpoints.addEndpoint')}</span>
                    </button>
                </div>

                <div class="card">
                    <div class="card-body">
                        <div id="endpoints-table"></div>
                    </div>
                </div>
            </div>
        `;

        document.getElementById('add-endpoint-btn').addEventListener('click', () => this.showAddModal());

        await this.loadEndpoints(renderVersion);
    }

    beginAction() {
        return ++this.actionVersion;
    }

    invalidateActions() {
        this.actionVersion++;
    }

    isActionCurrent(actionVersion) {
        return actionVersion === this.actionVersion && state.get('currentView') === 'endpoints';
    }

    isLoadCurrent(renderVersion, actionVersion) {
        return renderVersion === this.renderVersion &&
            state.get('currentView') === 'endpoints' &&
            (actionVersion == null || actionVersion === this.actionVersion);
    }

    queueMutation(operation) {
        const mutationVersion = ++this.mutationVersion;
        const result = this.mutationQueue.then(operation);
        this.mutationQueue = result.catch(() => {}).then(() => {
            if (mutationVersion === this.mutationVersion) {
                return this.loadEndpoints(this.renderVersion);
            }
        });
        return result;
    }

    activateEndpointModal(overlay, options = {}) {
        const { onClose, ...modalOptions } = options;
        return activateModal(overlay, {
            ...modalOptions,
            onClose: () => {
                this.invalidateActions();
                onClose?.();
            }
        });
    }

    async loadEndpoints(renderVersion = this.renderVersion, actionVersion = null) {
        const mutationVersion = this.mutationVersion;
        try {
            const data = await api.getEndpoints();
            if (!this.isLoadCurrent(renderVersion, actionVersion) || mutationVersion !== this.mutationVersion) {
                return;
            }
            const endpoints = data.endpoints || [];
            const tokenPools = data.tokenPools || {};
            let currentEndpoint = null;

            // Get current endpoint
            try {
                const currentData = await api.getCurrentEndpoint();
                if (!this.isLoadCurrent(renderVersion, actionVersion) || mutationVersion !== this.mutationVersion) {
                    return;
                }
                currentEndpoint = currentData.name || null;
            } catch (error) {
                if (!this.isLoadCurrent(renderVersion, actionVersion) || mutationVersion !== this.mutationVersion) {
                    return;
                }
                console.error('Failed to get current endpoint:', error);
            }

            if (this.isLoadCurrent(renderVersion, actionVersion) && mutationVersion === this.mutationVersion) {
                this.endpoints = endpoints;
                this.tokenPools = tokenPools;
                this.currentEndpoint = currentEndpoint;
                this.renderTable();
            }
        } catch (error) {
            if (this.isLoadCurrent(renderVersion, actionVersion) && mutationVersion === this.mutationVersion) {
                notifications.error(`${t('endpoints.failedToLoad')}: ${error.message}`);
            }
        }
    }

    renderTable() {
        const container = document.getElementById('endpoints-table');
        if (!container) {
            return;
        }

        if (this.endpoints.length === 0) {
            container.innerHTML = `
                <div class="empty-state">
                    <div class="empty-state-icon" aria-hidden="true">+</div>
                    <div class="empty-state-title">${t('endpoints.noEndpoints')}</div>
                    <div class="empty-state-message">${t('endpoints.noEndpointsMessage')}</div>
                </div>
            `;
            return;
        }

        container.innerHTML = `
            <div class="table-container table-responsive">
                <table class="table">
                    <thead>
                        <tr>
                            <th class="reorder-column" aria-label="${t('endpoints.reorder')}"></th>
                            <th>${t('common.name')}</th>
                            <th>${t('endpoints.apiUrl')}</th>
                            <th>${t('endpoints.authMode')}</th>
                            <th>${t('endpoints.transformer')}</th>
                            <th>${t('endpoints.model')}</th>
                            <th>${t('endpoints.tokenPool')}</th>
                            <th>${t('common.status')}</th>
                            <th>${t('common.actions')}</th>
                        </tr>
                    </thead>
                    <tbody id="endpoints-tbody">
                        ${this.endpoints.map((ep, index) => this.renderEndpointRow(ep, index)).join('')}
                    </tbody>
                </table>
            </div>
        `;

        // Attach event listeners
        this.attachEventListeners();
        this.attachDragListeners();
    }

    renderEndpointRow(ep, index) {
        const isCurrentEndpoint = ep.name === this.currentEndpoint;
        const isTokenPool = tokenPoolAuthModes.has(ep.authMode);
        const testStatus = this.getTestStatus(ep.name);
        const reorderLabel = escapeHtml(t('endpoints.reorder'));
        let testStatusClass = 'badge-warning';
        let testStatusLabel = t('endpoints.notTested');

        if (testStatus === true) {
            testStatusClass = 'badge-success';
            testStatusLabel = t('endpoints.testPassed');
        } else if (testStatus === false) {
            testStatusClass = 'badge-danger';
            testStatusLabel = t('endpoints.testFailed');
        }

        return `
            <tr class="endpoint-row" data-endpoint="${escapeHtml(ep.name)}" data-index="${index}" draggable="true">
                <td class="drag-handle" data-label="${t('endpoints.reorder')}">
                    <div class="actions">
                        <button class="btn btn-sm btn-secondary move-up-btn" type="button" draggable="false" data-index="${index}" title="${reorderLabel} ↑" aria-label="${reorderLabel} ↑" ${index === 0 ? 'disabled' : ''}>↑</button>
                        <button class="btn btn-sm btn-secondary move-down-btn" type="button" draggable="false" data-index="${index}" title="${reorderLabel} ↓" aria-label="${reorderLabel} ↓" ${index === this.endpoints.length - 1 ? 'disabled' : ''}>↓</button>
                        <span aria-hidden="true">⋮⋮</span>
                    </div>
                </td>
                <td data-label="${t('common.name')}">
                    <div class="endpoint-meta">
                        <strong>${escapeHtml(ep.name)}</strong>
                        <span>
                            <span class="badge ${testStatusClass}" title="${escapeHtml(testStatusLabel)}">${escapeHtml(testStatusLabel)}</span>
                            ${isCurrentEndpoint ? `<span class="badge badge-primary">${t('endpoints.current')}</span>` : ''}
                        </span>
                        ${!isTokenPool ? `<span class="secret-status ${ep.hasApiKey ? 'is-configured' : ''}">${ep.hasApiKey ? t('endpoints.apiKeyConfigured') : t('endpoints.apiKeyMissing')}</span>` : ''}
                    </div>
                </td>
                <td data-label="${t('endpoints.apiUrl')}">
                    <code class="endpoint-url">${escapeHtml(ep.apiUrl)}</code>
                    <button class="btn-icon copy-btn" type="button" data-copy="${escapeHtml(ep.apiUrl)}" title="${t('endpoints.copyUrl')}" aria-label="${t('endpoints.copyUrl')}">
                        <span aria-hidden="true">⧉</span>
                    </button>
                </td>
                <td data-label="${t('endpoints.authMode')}">${t(`authModes.${ep.authMode || 'api_key'}`)}</td>
                <td data-label="${t('endpoints.transformer')}">${escapeHtml(getTransformerLabel(ep.transformer))}</td>
                <td data-label="${t('endpoints.model')}">${escapeHtml(ep.model || '-')}</td>
                <td data-label="${t('endpoints.tokenPool')}">${isTokenPool ? this.renderTokenPoolSummary(this.tokenPools[ep.name]) : '<span class="text-muted">-</span>'}</td>
                <td data-label="${t('common.status')}">${getStatusBadge(ep.enabled)}</td>
                <td data-label="${t('common.actions')}">
                    <div class="actions">
                        ${ep.enabled && !isCurrentEndpoint ? `
                            <button class="btn btn-sm btn-secondary switch-btn" type="button" data-name="${escapeHtml(ep.name)}" title="${t('endpoints.switchToEndpoint')}">
                                ${t('common.switch')}
                            </button>
                        ` : ''}
                        <button class="btn btn-sm btn-secondary test-btn" type="button" data-name="${escapeHtml(ep.name)}">
                            ${t('common.test')}
                        </button>
                        ${isTokenPool ? `<button class="btn btn-sm btn-secondary token-pool-btn" type="button" data-name="${escapeHtml(ep.name)}">${t('endpoints.tokenPoolManagement')}</button>` : ''}
                        <label class="toggle-switch">
                            <input type="checkbox" class="toggle-endpoint" data-name="${escapeHtml(ep.name)}" aria-label="${escapeHtml(ep.name)}: ${t('common.enabled')}" ${ep.enabled ? 'checked' : ''}>
                            <span class="toggle-slider"></span>
                        </label>
                        <button class="btn btn-sm btn-secondary edit-btn" type="button" data-name="${escapeHtml(ep.name)}">
                            ${t('common.edit')}
                        </button>
                        <button class="btn btn-sm btn-secondary clone-btn" type="button" data-name="${escapeHtml(ep.name)}">
                            ${t('common.clone')}
                        </button>
                        <button class="btn btn-sm btn-danger delete-btn" type="button" data-name="${escapeHtml(ep.name)}">
                            ${t('common.delete')}
                        </button>
                    </div>
                </td>
            </tr>
        `;
    }

    renderTokenPoolSummary(pool) {
        if (!pool || !pool.total) {
            return '<span class="text-muted">0</span>';
        }

        return `
            <div class="token-pool-summary">
                <div>${t('endpoints.total')}: <strong>${pool.total}</strong></div>
                <span>${t('endpoints.active')}: ${pool.active || 0}</span>
                <span>${t('endpoints.expiring')}: ${pool.expiring || 0}</span>
                <span>${t('endpoints.expired')}: ${pool.expired || 0}</span>
                <span>${t('endpoints.invalid')}: ${pool.invalid || 0}</span>
                <span>${t('endpoints.cooldown')}: ${pool.cooldown || 0}</span>
                <span>${t('endpoints.needRefresh')}: ${pool.needRefresh || 0}</span>
                <span>${t('common.disabled')}: ${pool.disabled || 0}</span>
            </div>
        `;
    }

    attachEventListeners() {
        // Test buttons
        document.querySelectorAll('.test-btn').forEach(btn => {
            btn.addEventListener('click', () => this.testEndpoint(btn.dataset.name));
        });

        // Toggle switches
        document.querySelectorAll('.toggle-endpoint').forEach(toggle => {
            toggle.addEventListener('change', () => this.toggleEndpoint(toggle.dataset.name, toggle.checked));
        });

        // Edit buttons
        document.querySelectorAll('.edit-btn').forEach(btn => {
            btn.addEventListener('click', () => this.showEditModal(btn.dataset.name));
        });

        // Clone buttons
        document.querySelectorAll('.clone-btn').forEach(btn => {
            btn.addEventListener('click', () => this.cloneEndpoint(btn.dataset.name));
        });

        // Delete buttons
        document.querySelectorAll('.delete-btn').forEach(btn => {
            btn.addEventListener('click', () => this.deleteEndpoint(btn.dataset.name));
        });

        // Switch buttons
        document.querySelectorAll('.switch-btn').forEach(btn => {
            btn.addEventListener('click', () => this.switchEndpoint(btn.dataset.name));
        });

        // Token pool buttons
        document.querySelectorAll('.token-pool-btn').forEach(btn => {
            btn.addEventListener('click', () => this.showTokenPoolModal(btn.dataset.name));
        });

        // Copy buttons
        document.querySelectorAll('.copy-btn').forEach(btn => {
            btn.addEventListener('click', () => this.copyToClipboard(btn.dataset.copy, btn));
        });

        document.querySelectorAll('.move-up-btn').forEach(btn => {
            btn.addEventListener('click', () => this.moveEndpoint(Number(btn.dataset.index), -1));
        });
        document.querySelectorAll('.move-down-btn').forEach(btn => {
            btn.addEventListener('click', () => this.moveEndpoint(Number(btn.dataset.index), 1));
        });
    }

    attachDragListeners() {
        const rows = document.querySelectorAll('#endpoints-tbody tr[draggable="true"]');

        rows.forEach(row => {
            row.addEventListener('dragstart', (e) => {
                this.draggedIndex = parseInt(row.dataset.index);
                row.classList.add('is-dragging');
            });

            row.addEventListener('dragend', (e) => {
                row.classList.remove('is-dragging');
                document.querySelectorAll('.endpoint-row.is-drop-target').forEach(target => {
                    target.classList.remove('is-drop-target');
                });
                this.draggedIndex = null;
            });

            row.addEventListener('dragover', (e) => {
                e.preventDefault();
                row.classList.add('is-drop-target');
            });

            row.addEventListener('dragleave', (e) => {
                row.classList.remove('is-drop-target');
            });

            row.addEventListener('drop', (e) => {
                e.preventDefault();
                row.classList.remove('is-drop-target');

                const fromIndex = this.draggedIndex;
                const dropIndex = parseInt(row.dataset.index);
                this.draggedIndex = null;
                if (fromIndex !== null && fromIndex !== dropIndex) {
                    this.reorderEndpoints(fromIndex, dropIndex);
                }
            });
        });
    }

    moveEndpoint(fromIndex, offset) {
        const toIndex = fromIndex + offset;
        if (toIndex < 0 || toIndex >= this.endpoints.length) {
            return;
        }
        this.reorderEndpoints(fromIndex, toIndex);
    }

    reorderEndpoints(fromIndex, toIndex) {
        const actionVersion = this.beginAction();
        const [movedItem] = this.endpoints.splice(fromIndex, 1);
        this.endpoints.splice(toIndex, 0, movedItem);
        const names = this.endpoints.map(endpoint => endpoint.name);
        const orderKey = JSON.stringify(names);
        this.renderTable();

        const mutation = this.queueMutation(() => api.reorderEndpoints(names));
        this.reorderQueue = mutation.then(
            () => {
                if (!this.isActionCurrent(actionVersion)) {
                    return;
                }
                notifications.success(t('notifications.endpointsReordered'));
            },
            error => {
                const visibleOrder = JSON.stringify(this.endpoints.map(endpoint => endpoint.name));
                if (this.isActionCurrent(actionVersion) && visibleOrder === orderKey) {
                    notifications.error(`${t('endpoints.failedToReorder')}: ${error.message}`);
                }
            }
        );
        return this.reorderQueue;
    }

    async switchEndpoint(name) {
        const actionVersion = this.beginAction();
        try {
            await this.queueMutation(() => api.switchEndpoint(name));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(`${t('notifications.endpointSwitched')} ${name}`);
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToSwitch')}: ${error.message}`);
            }
        }
    }

    copyToClipboard(text, button) {
        if (!navigator.clipboard?.writeText) {
            notifications.error(t('endpoints.failedToCopy'));
            return;
        }
        navigator.clipboard.writeText(text).then(() => {
            const originalText = button.textContent;
            button.textContent = '✓';
            setTimeout(() => {
                button.textContent = originalText;
            }, 1000);
        }).catch(err => {
            notifications.error(t('endpoints.failedToCopy'));
        });
    }

    getTestStatus(endpointName) {
        try {
            const statusMap = JSON.parse(localStorage.getItem('ccNexus_endpointTestStatus') || '{}');
            return statusMap[endpointName];
        } catch {
            return undefined;
        }
    }

    saveTestStatus(endpointName, success) {
        try {
            const statusMap = JSON.parse(localStorage.getItem('ccNexus_endpointTestStatus') || '{}');
            statusMap[endpointName] = success;
            localStorage.setItem('ccNexus_endpointTestStatus', JSON.stringify(statusMap));
        } catch (error) {
            console.error('Failed to save test status:', error);
        }
    }

    showAddModal() {
        this.showEndpointModal(null);
    }

    showEditModal(name) {
        const endpoint = this.endpoints.find(ep => ep.name === name);
        if (endpoint) {
            this.showEndpointModal(endpoint);
        }
    }

    showEndpointModal(endpoint, isClone = false) {
        this.invalidateActions();
        const isEdit = !!endpoint && !isClone;
        const modalContainer = document.getElementById('modal-container');
        const authMode = endpoint?.authMode || 'api_key';
        const hasApiKey = endpoint?.hasApiKey === true;
        const apiKeyHint = isEdit || isClone ? `<small class="form-hint">${t('endpoints.keepExistingKey')}</small>` : '';
        const cloneHiddenInput = isClone ? '<input type="hidden" name="isClone" value="true">' : '';
        const cloneFromValue = endpoint?.cloneFrom || '';
        const cloneFromInput = isClone && cloneFromValue ? `<input type="hidden" name="cloneFrom" value="${escapeHtml(cloneFromValue)}">` : '';

        closeAllModals();
        modalContainer.innerHTML = `
            <div class="modal-overlay">
                <div class="modal modal--wide">
                    <div class="modal-header">
                        <h3 class="modal-title">${isClone ? t('endpoints.cloneEndpoint') : (isEdit ? t('common.edit') : t('common.add'))} ${t('endpoints.title')}</h3>
                        <button class="modal-close" id="close-modal" type="button" aria-label="${t('common.close')}">×</button>
                    </div>
                    <div class="modal-body">
                        <form id="endpoint-form">
                            ${cloneHiddenInput}
                            ${cloneFromInput}
                            <div class="form-group">
                                <label class="form-label" for="endpoint-name">${t('common.name')} *</label>
                                <input type="text" class="form-input" id="endpoint-name" name="name" value="${endpoint ? escapeHtml(endpoint.name) : ''}" required ${isEdit ? 'readonly' : ''}>
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="endpoint-api-url">${t('endpoints.apiUrl')} <span id="api-url-required">*</span></label>
                                <input type="url" class="form-input" id="endpoint-api-url" name="apiUrl" value="${endpoint ? escapeHtml(endpoint.apiUrl) : ''}" placeholder="${t('endpoints.apiUrlPlaceholder')}" required>
                            </div>
                            <div class="form-group" id="api-key-group">
                                <label class="form-label" for="endpoint-api-key">${t('endpoints.apiKey')} <span id="api-key-required">*</span></label>
                                <input type="password" class="form-input" id="endpoint-api-key" name="apiKey" value="" placeholder="${t('endpoints.apiKeyPlaceholder')}" autocomplete="new-password">
                                ${apiKeyHint}
                                ${(isEdit || isClone) ? `<span class="secret-status ${hasApiKey ? 'is-configured' : ''}">${hasApiKey ? t('endpoints.apiKeyConfigured') : t('endpoints.apiKeyMissing')}</span>` : ''}
                                <div class="form-error" id="api-key-error" role="alert"></div>
                                ${isEdit && hasApiKey ? `
                                    <label>
                                        <input type="checkbox" class="form-checkbox" id="clear-api-key" name="clearApiKey">
                                        ${t('endpoints.clearApiKey')}
                                    </label>
                                ` : ''}
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="endpoint-auth-mode">${t('endpoints.authMode')} *</label>
                                <select class="form-select" id="endpoint-auth-mode" name="authMode" required>
                                    <option value="api_key" ${authMode === 'api_key' ? 'selected' : ''}>${t('authModes.api_key')}</option>
                                    <option value="token_pool" ${authMode === 'token_pool' ? 'selected' : ''}>${t('authModes.token_pool')}</option>
                                    <option value="codex_token_pool" ${authMode === 'codex_token_pool' ? 'selected' : ''}>${t('authModes.codex_token_pool')}</option>
                                </select>
                                <small class="form-hint" id="auth-mode-hint"></small>
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="endpoint-transformer">${t('endpoints.transformer')} *</label>
                                <select class="form-select" id="endpoint-transformer" name="transformer" required>
                                    <option value="claude" ${endpoint?.transformer === 'claude' ? 'selected' : ''}>${t('transformers.claude')}</option>
                                    <option value="openai" ${endpoint?.transformer === 'openai' ? 'selected' : ''}>${t('transformers.openai')}</option>
                                    <option value="openai2" ${endpoint?.transformer === 'openai2' ? 'selected' : ''}>${t('transformers.openai2')}</option>
                                    <option value="gemini" ${endpoint?.transformer === 'gemini' ? 'selected' : ''}>${t('transformers.gemini')}</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="model-input">${t('endpoints.model')}</label>
                                <div class="model-picker-row">
                                    <input type="text" class="form-input model-picker-input" name="model" id="model-input" value="${endpoint ? escapeHtml(endpoint.model || '') : ''}" placeholder="${t('endpoints.modelPlaceholder')}">
                                    <button type="button" class="btn btn-secondary model-picker-button" id="fetch-models-btn">
                                        ${t('endpoints.fetchModels')}
                                    </button>
                                </div>
                                <small class="form-hint" id="fetch-models-hint">${t('endpoints.fetchModelsHint')}</small>
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="endpoint-remark">${t('endpoints.remark')}</label>
                                <textarea class="form-textarea" id="endpoint-remark" name="remark">${endpoint ? escapeHtml(endpoint.remark || '') : ''}</textarea>
                            </div>
                            <div class="form-group">
                                <label>
                                    <input type="checkbox" class="form-checkbox" name="enabled" ${endpoint?.enabled !== false ? 'checked' : ''}>
                                    ${t('common.enabled')}
                                </label>
                            </div>
                        </form>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary" id="cancel-btn" type="button">${t('common.cancel')}</button>
                        <button class="btn btn-primary" id="save-btn" type="submit" form="endpoint-form">${isEdit ? t('common.update') : t('common.create')}</button>
                    </div>
                </div>
            </div>
        `;

        const overlay = modalContainer.querySelector('.modal-overlay');
        this.activateEndpointModal(overlay, { initialFocus: '#endpoint-name' });
        document.getElementById('close-modal').addEventListener('click', () => this.closeModal());
        document.getElementById('cancel-btn').addEventListener('click', () => this.closeModal());
        document.getElementById('endpoint-form').addEventListener('submit', event => {
            event.preventDefault();
            this.saveEndpoint(isEdit, endpoint?.name, isClone);
        });
        document.getElementById('fetch-models-btn').addEventListener('click', () => this.fetchModels(isEdit ? endpoint.name : cloneFromValue));
        document.getElementById('endpoint-auth-mode').addEventListener('change', () => this.updateEndpointAuthFields(isEdit, isClone));
        document.getElementById('clear-api-key')?.addEventListener('change', () => this.updateEndpointAuthFields(isEdit, isClone));
        document.getElementById('endpoint-api-key').addEventListener('input', event => {
            event.currentTarget.removeAttribute('aria-invalid');
            document.getElementById('api-key-error').textContent = '';
        });
        this.updateEndpointAuthFields(isEdit, isClone);
    }

    updateEndpointAuthFields(isEdit, isClone) {
        const authMode = document.getElementById('endpoint-auth-mode')?.value;
        const group = document.getElementById('api-key-group');
        const input = document.getElementById('endpoint-api-key');
        const required = document.getElementById('api-key-required');
        const clear = document.getElementById('clear-api-key');
        const apiUrl = document.getElementById('endpoint-api-url');
        const apiUrlRequired = document.getElementById('api-url-required');
        const transformer = document.getElementById('endpoint-transformer');
        const fetchModelsButton = document.getElementById('fetch-models-btn');
        const fetchModelsHint = document.getElementById('fetch-models-hint');
        const authModeHint = document.getElementById('auth-mode-hint');
        if (!group || !input) {
            return;
        }

        const usesApiKey = authMode === 'api_key';
        const usesCodexTokenPool = authMode === 'codex_token_pool';
        const keyRequired = usesApiKey && !isEdit && !isClone;
        const urlRequired = !usesCodexTokenPool;
        group.hidden = !usesApiKey;
        input.required = keyRequired;
        input.disabled = !usesApiKey || clear?.checked === true;
        if (clear) {
            clear.disabled = !usesApiKey;
        }
        required.hidden = !keyRequired;
        apiUrl.required = urlRequired;
        apiUrl.readOnly = usesCodexTokenPool;
        apiUrlRequired.hidden = !urlRequired;
        transformer.disabled = usesCodexTokenPool;
        fetchModelsButton.hidden = usesCodexTokenPool;
        fetchModelsButton.disabled = usesCodexTokenPool;
        fetchModelsHint.textContent = t(usesCodexTokenPool ? 'endpoints.codexModelsUnavailable' : 'endpoints.fetchModelsHint');
        if (usesApiKey) {
            authModeHint.textContent = t('endpoints.apiKeyModeHint');
        } else if (usesCodexTokenPool) {
            authModeHint.textContent = t('endpoints.codexTokenPoolModeHint');
        } else {
            authModeHint.textContent = t('endpoints.tokenPoolModeHint');
        }
        if (usesCodexTokenPool) {
            apiUrl.value = 'https://chatgpt.com/backend-api/codex';
            transformer.value = 'openai2';
        }
        if (!usesApiKey) {
            input.value = '';
        }
        document.getElementById('api-key-error').textContent = '';
    }

    async fetchModels(endpointName = '') {
        const apiUrlInput = document.querySelector('input[name="apiUrl"]');
        const apiKeyInput = document.querySelector('input[name="apiKey"]');
        const transformerSelect = document.querySelector('select[name="transformer"]');
        const modelInput = document.getElementById('model-input');
        const fetchBtn = document.getElementById('fetch-models-btn');

        const apiUrl = apiUrlInput.value.trim();
        const apiKey = apiKeyInput.value.trim();
        const transformer = transformerSelect.value;

        if (!apiUrl || (!apiKey && !endpointName)) {
            notifications.error(t('endpoints.enterApiUrlAndKey'));
            return;
        }

        const actionVersion = this.beginAction();
        try {
            fetchBtn.disabled = true;
            fetchBtn.textContent = t('endpoints.fetchingModels');

            const result = await api.fetchModels({
                apiUrl,
                apiKey,
                transformer,
                endpointName: apiKey ? '' : endpointName
            });
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }

            if (Array.isArray(result?.models) && result.models.length > 0) {
                // Show model selection modal
                this.showModelSelectionModal(result.models, modelInput);
            } else {
                notifications.info(t('endpoints.noModelsFound'));
            }
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToFetchModels')}: ${error.message}`);
            }
        } finally {
            if (fetchBtn.isConnected) {
                fetchBtn.disabled = false;
                fetchBtn.textContent = t('endpoints.fetchModels');
            }
        }
    }

    showModelSelectionModal(models, modelInput) {
        const modalContainer = document.getElementById('modal-container');
        const modelModal = document.createElement('div');
        modelModal.className = 'modal-overlay modal-overlay--nested';
        modelModal.innerHTML = `
            <div class="modal modal--compact">
                <div class="modal-header">
                    <h3 class="modal-title">${t('endpoints.selectModel')}</h3>
                    <button class="modal-close" type="button" aria-label="${t('common.close')}">×</button>
                </div>
                <div class="modal-body">
                    <div class="model-list">
                        ${models.map((model, index) => `
                            <button class="btn btn-secondary model-item" type="button" data-index="${index}">
                                ${escapeHtml(String(model))}
                            </button>
                        `).join('')}
                    </div>
                </div>
                <div class="modal-footer">
                    <button class="btn btn-secondary cancel-model-btn" type="button">${t('common.cancel')}</button>
                </div>
            </div>
        `;

        modalContainer.appendChild(modelModal);
        const controller = this.activateEndpointModal(modelModal, { initialFocus: '.model-item' });
        modelModal.querySelector('.modal-close').addEventListener('click', () => controller.close());
        modelModal.querySelector('.cancel-model-btn').addEventListener('click', () => controller.close());
        modelModal.querySelectorAll('.model-item').forEach(item => {
            item.addEventListener('click', () => {
                const selectedModel = String(models[Number(item.dataset.index)]);
                modelInput.value = selectedModel;
                notifications.success(`${t('notifications.modelSelected')} ${selectedModel}`);
                controller.close();
            });
        });
    }

    async saveEndpoint(isEdit, originalName, isClone = false) {
        const actionVersion = this.beginAction();
        const form = document.getElementById('endpoint-form');
        const keyInput = document.getElementById('endpoint-api-key');
        if (keyInput.required && !keyInput.value.trim()) {
            document.getElementById('api-key-error').textContent = t('endpoints.apiKeyRequired');
            keyInput.setAttribute('aria-invalid', 'true');
            keyInput.focus();
            return;
        }
        if (!form.reportValidity()) {
            return;
        }
        const formData = new FormData(form);
        const clearApiKey = formData.get('clearApiKey') === 'on';
        if (clearApiKey) {
            const confirmed = await confirmDialog({
                title: t('endpoints.clearApiKeyTitle'),
                message: t('endpoints.confirmClearApiKey'),
                confirmLabel: t('endpoints.clearApiKey'),
                cancelLabel: t('common.cancel'),
                danger: true
            });
            if (!confirmed) {
                return;
            }
        }
        if (!this.isActionCurrent(actionVersion)) {
            return;
        }

        const data = {
            name: formData.get('name').trim(),
            apiUrl: document.getElementById('endpoint-api-url').value.trim(),
            authMode: formData.get('authMode'),
            transformer: document.getElementById('endpoint-transformer').value,
            model: formData.get('model').trim(),
            remark: formData.get('remark').trim(),
            enabled: formData.get('enabled') === 'on'
        };
        const apiKey = (formData.get('apiKey') || '').trim();
        if (apiKey) {
            data.apiKey = apiKey;
        }
        if (clearApiKey) {
            data.clearApiKey = true;
        }
        // For clone mode, add cloneFrom field if available
        const cloneFromInput = document.querySelector('input[name="cloneFrom"]');
        if (isClone && cloneFromInput && cloneFromInput.value) {
            data.cloneFrom = cloneFromInput.value;
        }

        try {
            await this.queueMutation(() => isEdit ? api.updateEndpoint(originalName, data) : api.createEndpoint(data));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }

            notifications.success(t(isClone ? 'notifications.endpointCloned' : (isEdit ? 'notifications.endpointUpdated' : 'notifications.endpointCreated')));
            this.closeModal();
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToSave')}: ${error.message}`);
            }
        }
    }

    async toggleEndpoint(name, enabled) {
        const actionVersion = this.beginAction();
        try {
            await this.queueMutation(() => api.toggleEndpoint(name, enabled));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(enabled ? t('notifications.endpointEnabled') : t('notifications.endpointDisabled'));
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToToggle')}: ${error.message}`);
            }
        }
    }

    async testEndpoint(name) {
        const actionVersion = this.beginAction();
        try {
            notifications.info(t('endpoints.testing'));
            const result = await api.testEndpoint(name);
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }

            if (result.success) {
                this.saveTestStatus(name, true);
                this.renderTable();
                notifications.success(`${t('notifications.testSuccessful')} ${result.latency}ms`);
                this.showTestResultModal(name, result);
            } else {
                this.saveTestStatus(name, false);
                this.renderTable();
                notifications.error(`${t('notifications.testFailed')} ${result.error}`);
            }
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                this.saveTestStatus(name, false);
                this.renderTable();
                notifications.error(`${t('endpoints.failedToTest')}: ${error.message}`);
            }
        }
    }

    showTestResultModal(name, result) {
        const modalContainer = document.getElementById('modal-container');

        closeAllModals();
        modalContainer.innerHTML = `
            <div class="modal-overlay">
                <div class="modal">
                    <div class="modal-header">
                        <h3 class="modal-title">${t('endpoints.testResult')}: ${escapeHtml(name)}</h3>
                        <button class="modal-close" id="close-modal" type="button" aria-label="${t('common.close')}">×</button>
                    </div>
                    <div class="modal-body">
                        <div class="mb-2">
                            <strong>${t('common.status')}:</strong> <span class="badge badge-success">${t('common.success')}</span>
                        </div>
                        <div class="mb-2">
                            <strong>${t('endpoints.latency')}:</strong> ${result.latency}ms
                        </div>
                        <div class="mb-2">
                            <strong>${t('endpoints.response')}:</strong>
                            <div class="code-block mt-1">${escapeHtml(result.response || t('endpoints.noResponse'))}</div>
                        </div>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-primary" id="close-btn" type="button">${t('common.close')}</button>
                    </div>
                </div>
            </div>
        `;

        this.activateEndpointModal(modalContainer.querySelector('.modal-overlay'), { initialFocus: '#close-btn' });
        document.getElementById('close-modal').addEventListener('click', () => this.closeModal());
        document.getElementById('close-btn').addEventListener('click', () => this.closeModal());
    }

    async deleteEndpoint(name) {
        const actionVersion = this.beginAction();
        const confirmed = await confirmDialog({
            title: t('endpoints.deleteEndpoint'),
            message: t('endpoints.confirmDelete').replace('{name}', name),
            confirmLabel: t('common.delete'),
            cancelLabel: t('common.cancel'),
            danger: true
        });
        if (!confirmed) {
            return;
        }
        if (!this.isActionCurrent(actionVersion)) {
            return;
        }

        try {
            await this.queueMutation(() => api.deleteEndpoint(name));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(t('notifications.endpointDeleted'));
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToDelete')}: ${error.message}`);
            }
        }
    }

    cloneEndpoint(name) {
        const endpoint = this.endpoints.find(ep => ep.name === name);
        if (!endpoint) {
            notifications.error(t('endpoints.failedToClone'));
            return;
        }

        const copySuffix = t('endpoints.copySuffix');
        const baseName = name.replace(/\s+(?:\(Copy\)|（副本）)(?:\s+\d+)?$/, '').trim();
        let newName = `${baseName} ${copySuffix}`;
        let counter = 1;
        while (this.endpoints.some(ep => ep.name === newName)) {
            newName = `${baseName} ${copySuffix} ${counter}`;
            counter++;
        }

        // Create cloned endpoint - don't include apiKey, use cloneFrom instead
        const clonedEndpoint = {
            name: newName,
            apiUrl: endpoint.apiUrl,
            transformer: endpoint.transformer,
            model: endpoint.model,
            remark: endpoint.remark,
            enabled: endpoint.enabled,
            authMode: endpoint.authMode || 'api_key',
            hasApiKey: endpoint.hasApiKey === true,
            cloneFrom: name  // Reference to source endpoint
        };

        try {
            this.showEndpointModal(clonedEndpoint, true);
        } catch (error) {
            notifications.error(`${t('endpoints.failedToClone')}: ${error.message}`);
        }
    }

    async showTokenPoolModal(endpointName, actionVersion = null) {
        const endpoint = this.endpoints.find(item => item.name === endpointName);
        if (!endpoint || !tokenPoolAuthModes.has(endpoint.authMode)) {
            notifications.warning(t('endpoints.tokenPoolUnavailable'));
            return false;
        }
        actionVersion ??= this.beginAction();
        if (!this.isActionCurrent(actionVersion)) {
            return false;
        }
        this.currentTokenPoolEndpoint = endpointName;

        try {
            const result = await api.getEndpointCredentials(endpointName);
            if (!this.isActionCurrent(actionVersion)) {
                return false;
            }
            const credentials = result.credentials || [];
            this.currentCredentials = credentials;
            const stats = result.stats || {};
            const modalContainer = document.getElementById('modal-container');

            closeAllModals();
            modalContainer.innerHTML = `
                <div class="modal-overlay">
                    <div class="modal modal--wide">
                        <div class="modal-header">
                            <h3 class="modal-title">${t('endpoints.tokenPoolTitle')} ${escapeHtml(endpointName)}</h3>
                            <button class="modal-close" id="close-modal" type="button" aria-label="${t('common.close')}">×</button>
                        </div>
                        <div class="modal-body">
                            <div class="token-pool-stats mb-2">
                                <span><strong>${t('endpoints.total')}:</strong> ${stats.total || 0}</span>
                                <span><strong>${t('endpoints.active')}:</strong> ${stats.active || 0}</span>
                                <span><strong>${t('endpoints.expiring')}:</strong> ${stats.expiring || 0}</span>
                                <span><strong>${t('endpoints.needRefresh')}:</strong> ${stats.needRefresh || 0}</span>
                                <span><strong>${t('endpoints.expired')}:</strong> ${stats.expired || 0}</span>
                                <span><strong>${t('endpoints.invalid')}:</strong> ${stats.invalid || 0}</span>
                            </div>

                            <div class="form-group">
                                <label class="form-label" for="token-import-json">${t('endpoints.batchImportJson')}</label>
                                <textarea class="form-textarea token-import-input" id="token-import-json" placeholder='${t('endpoints.jsonPasteHint')}'></textarea>
                                <label class="form-check-row">
                                    <input type="checkbox" id="token-import-overwrite">
                                    ${t('endpoints.overwriteExisting')}
                                </label>
                                <div class="token-import-actions">
                                    <button class="btn btn-primary" id="token-import-btn" type="button">${t('common.import')}</button>
                                </div>
                            </div>

                            <div class="table-container table-responsive">
                                <table class="table">
                                    <thead>
                                        <tr>
                                            <th>${t('endpoints.id')}</th>
                                            <th>${t('endpoints.account')}</th>
                                            <th>${t('endpoints.email')}</th>
                                            <th>${t('endpoints.tokens')}</th>
                                            <th>${t('common.status')}</th>
                                            <th>${t('endpoints.expiresAt')}</th>
                                            <th>${t('endpoints.lastError')}</th>
                                            <th>${t('common.actions')}</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        ${this.renderCredentialRows(credentials)}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                        <div class="modal-footer">
                            <button class="btn btn-secondary" id="refresh-token-pool-btn" type="button">${t('common.refresh')}</button>
                            <button class="btn btn-secondary" id="close-token-pool-btn" type="button">${t('common.close')}</button>
                        </div>
                    </div>
                </div>
            `;

            this.activateEndpointModal(modalContainer.querySelector('.modal-overlay'), { initialFocus: '#token-import-json' });
            document.getElementById('close-modal').addEventListener('click', () => this.closeModal());
            document.getElementById('close-token-pool-btn').addEventListener('click', () => this.closeModal());
            document.getElementById('refresh-token-pool-btn').addEventListener('click', () => this.showTokenPoolModal(endpointName));
            document.getElementById('token-import-btn').addEventListener('click', () => this.importEndpointCredentials(endpointName));

            document.querySelectorAll('.token-enable-toggle').forEach(toggle => {
                toggle.addEventListener('change', () => this.updateCredentialEnabled(endpointName, toggle.dataset.id, toggle.checked));
            });
            document.querySelectorAll('.token-update-btn').forEach(btn => {
                btn.addEventListener('click', () => this.updateCredentialToken(endpointName, btn.dataset.id));
            });
            document.querySelectorAll('.token-activate-btn').forEach(btn => {
                btn.addEventListener('click', () => this.activateCredential(endpointName, btn.dataset.id));
            });
            document.querySelectorAll('.token-delete-btn').forEach(btn => {
                btn.addEventListener('click', () => this.deleteCredential(endpointName, btn.dataset.id));
            });
            return true;
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToLoadTokenPool')}: ${error.message}`);
            }
            return false;
        }
    }

    renderCredentialRows(credentials) {
        if (!credentials || credentials.length === 0) {
            return `<tr><td colspan="8" class="text-center text-muted" data-label="">${t('endpoints.noCredentials')}</td></tr>`;
        }

        return credentials.map(cred => `
            <tr>
                <td data-label="${t('endpoints.id')}">${cred.id}</td>
                <td data-label="${t('endpoints.account')}"><code>${escapeHtml(cred.accountId || '-')}</code></td>
                <td data-label="${t('endpoints.email')}">${escapeHtml(cred.email || '-')}</td>
                <td data-label="${t('endpoints.tokens')}">${this.renderCredentialTokenStatus(cred)}</td>
                <td data-label="${t('common.status')}">${this.renderCredentialStatusBadge(cred.status)}</td>
                <td data-label="${t('endpoints.expiresAt')}">${escapeHtml(cred.expiresAt ? formatDateTime(cred.expiresAt) : '-')}</td>
                <td class="credential-error-cell" data-label="${t('endpoints.lastError')}" title="${escapeHtml(cred.lastError || '')}">
                    ${escapeHtml(cred.lastError || '-')}
                </td>
                <td data-label="${t('common.actions')}">
                    <div class="actions">
                        <label class="credential-enabled-toggle">
                            <input type="checkbox" class="token-enable-toggle" data-id="${cred.id}" ${cred.enabled ? 'checked' : ''}>
                            ${t('common.enabled')}
                        </label>
                        <button class="btn btn-sm btn-secondary token-update-btn" type="button" data-id="${cred.id}">${t('common.update')}</button>
                        <button class="btn btn-sm btn-secondary token-activate-btn" type="button" data-id="${cred.id}">${t('endpoints.activate')}</button>
                        <button class="btn btn-sm btn-danger token-delete-btn" type="button" data-id="${cred.id}">${t('common.delete')}</button>
                    </div>
                </td>
            </tr>
        `).join('');
    }

    renderCredentialStatusBadge(status) {
        const normalized = status || 'unknown';
        const statuses = {
            active: { label: 'endpoints.active', badge: 'badge-success' },
            expiring: { label: 'endpoints.expiring', badge: 'badge-warning' },
            need_refresh: { label: 'endpoints.needRefresh', badge: 'badge-warning' },
            expired: { label: 'endpoints.expired', badge: 'badge-danger' },
            invalid: { label: 'endpoints.invalid', badge: 'badge-danger' },
            cooldown: { label: 'endpoints.cooldown', badge: 'badge-info' },
            disabled: { label: 'common.disabled', badge: 'badge-danger' }
        };
        const display = statuses[normalized] || { label: 'common.unknown', badge: 'badge-info' };
        return `<span class="badge ${display.badge}">${escapeHtml(t(display.label))}</span>`;
    }

    renderCredentialTokenStatus(credential) {
        return [
            ['A', credential.hasAccessToken],
            ['R', credential.hasRefreshToken],
            ['ID', credential.hasIdToken]
        ].map(([label, configured]) => {
            const status = configured ? t('endpoints.tokenConfigured') : t('endpoints.tokenMissing');
            return `<span class="secret-status ${configured ? 'is-configured' : ''}"><strong>${label}</strong>: ${escapeHtml(status)}</span>`;
        }).join(' ');
    }

    async refreshTokenPoolAfterAction(endpointName, actionVersion) {
        if (this.isActionCurrent(actionVersion)) {
            await this.showTokenPoolModal(endpointName, actionVersion);
        }
    }

    async importEndpointCredentials(endpointName) {
        const jsonInput = document.getElementById('token-import-json');
        const overwriteInput = document.getElementById('token-import-overwrite');
        const raw = (jsonInput?.value || '').trim();

        if (!raw) {
            notifications.warning(t('endpoints.pleasePasteJson'));
            return;
        }

        let payload;
        try {
            payload = JSON.parse(raw);
        } catch {
            notifications.error(t('endpoints.invalidJson'));
            return;
        }

        let requestBody;
        if (Array.isArray(payload)) {
            requestBody = { items: payload, overwrite: overwriteInput?.checked === true };
        } else if (payload.items && Array.isArray(payload.items)) {
            requestBody = { ...payload, overwrite: overwriteInput?.checked === true };
        } else {
            requestBody = { items: [payload], overwrite: overwriteInput?.checked === true };
        }

        const actionVersion = this.beginAction();
        try {
            const result = await this.queueMutation(() => api.importEndpointCredentials(endpointName, requestBody));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(t('notifications.importDone').replace('{created}', result.created || 0).replace('{updated}', result.updated || 0).replace('{skipped}', result.skipped || 0).replace('{failed}', result.failed || 0));
            jsonInput.value = '';
            await this.refreshTokenPoolAfterAction(endpointName, actionVersion);
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToImport')}: ${error.message}`);
            }
        }
    }

    async updateCredentialEnabled(endpointName, credentialId, enabled) {
        const actionVersion = this.beginAction();
        try {
            await this.queueMutation(() => api.updateEndpointCredential(endpointName, credentialId, { enabled }));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(enabled ? t('notifications.credentialEnabled') : t('notifications.credentialDisabled'));
            await this.refreshTokenPoolAfterAction(endpointName, actionVersion);
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToUpdateCredential')}: ${error.message}`);
                await this.showTokenPoolModal(endpointName, actionVersion);
            }
        }
    }

    async activateCredential(endpointName, credentialId) {
        const actionVersion = this.beginAction();
        try {
            await this.queueMutation(() => api.updateEndpointCredential(endpointName, credentialId, { status: 'active' }));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(t('notifications.credentialActivated'));
            await this.refreshTokenPoolAfterAction(endpointName, actionVersion);
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToActivateCredential')}: ${error.message}`);
            }
        }
    }

    async updateCredentialToken(endpointName, credentialId) {
        const credential = this.currentCredentials?.find(item => String(item.id) === String(credentialId));
        if (!credential) {
            notifications.error(t('endpoints.credentialNotFound'));
            return;
        }
        const values = await this.showCredentialTokenModal(credential);
        if (!values) {
            return;
        }

        const payload = {};
        if (values.accessToken) {
            payload.accessToken = values.accessToken;
            payload.status = 'active';
        }
        if (values.refreshToken) {
            payload.refreshToken = values.refreshToken;
        }
        if (values.idToken) {
            payload.idToken = values.idToken;
        }
        if (values.clearRefreshToken) {
            payload.clearRefreshToken = true;
        }
        if (values.clearIdToken) {
            payload.clearIdToken = true;
        }
        if (values.expiresAt) {
            payload.expiresAt = values.expiresAt;
        }
        if (Object.keys(payload).length === 0) {
            notifications.info(t('endpoints.noCredentialChanges'));
            return;
        }

        const actionVersion = this.beginAction();
        try {
            await this.queueMutation(() => api.updateEndpointCredential(endpointName, credentialId, payload));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(t('notifications.tokenUpdated'));
            await this.refreshTokenPoolAfterAction(endpointName, actionVersion);
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToUpdateToken')}: ${error.message}`);
            }
        }
    }

    showCredentialTokenModal(credential) {
        this.invalidateActions();
        const container = document.getElementById('modal-container');
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay modal-overlay--nested';
        overlay.innerHTML = `
            <div class="modal">
                <div class="modal-header">
                    <h3 class="modal-title">${t('endpoints.updateCredential')} #${credential.id}</h3>
                    <button class="modal-close" type="button" aria-label="${t('common.close')}">×</button>
                </div>
                <div class="modal-body">
                    <form id="credential-token-form">
                        <div class="form-group">
                            <label class="form-label" for="credential-access-token">${t('endpoints.accessToken')}</label>
                            <input class="form-input" id="credential-access-token" name="accessToken" type="password" autocomplete="new-password">
                            <small class="form-hint">${t('endpoints.keepExistingToken')}</small>
                            <span class="secret-status ${credential.hasAccessToken ? 'is-configured' : ''}">${credential.hasAccessToken ? t('endpoints.tokenConfigured') : t('endpoints.tokenMissing')}</span>
                        </div>
                        <div class="form-group">
                            <label class="form-label" for="credential-refresh-token">${t('endpoints.refreshToken')}</label>
                            <input class="form-input" id="credential-refresh-token" name="refreshToken" type="password" autocomplete="new-password">
                            <small class="form-hint">${t('endpoints.keepExistingToken')}</small>
                            ${credential.hasRefreshToken ? `<label><input class="form-checkbox clear-token" type="checkbox" name="clearRefreshToken" data-input="credential-refresh-token"> ${t('endpoints.clearRefreshToken')}</label>` : ''}
                        </div>
                        <div class="form-group">
                            <label class="form-label" for="credential-id-token">${t('endpoints.idToken')}</label>
                            <input class="form-input" id="credential-id-token" name="idToken" type="password" autocomplete="new-password">
                            <small class="form-hint">${t('endpoints.keepExistingToken')}</small>
                            ${credential.hasIdToken ? `<label><input class="form-checkbox clear-token" type="checkbox" name="clearIdToken" data-input="credential-id-token"> ${t('endpoints.clearIdToken')}</label>` : ''}
                        </div>
                        <div class="form-group">
                            <label class="form-label" for="credential-expires-at">${t('endpoints.enterExpiresAt')}</label>
                            <input class="form-input" id="credential-expires-at" name="expiresAt" type="text" placeholder="2026-12-31T23:59:59Z">
                        </div>
                    </form>
                </div>
                <div class="modal-footer">
                    <button class="btn btn-secondary credential-cancel" type="button">${t('common.cancel')}</button>
                    <button class="btn btn-primary" type="submit" form="credential-token-form">${t('common.update')}</button>
                </div>
            </div>
        `;
        container.appendChild(overlay);

        return new Promise(resolve => {
            let settled = false;
            const finish = value => {
                if (settled) {
                    return;
                }
                settled = true;
                controller.close();
                resolve(value);
            };
            const controller = this.activateEndpointModal(overlay, {
                initialFocus: '#credential-access-token',
                onClose: () => {
                    if (!settled) {
                        settled = true;
                        resolve(null);
                    }
                }
            });
            overlay.querySelector('.modal-close').addEventListener('click', () => finish(null));
            overlay.querySelector('.credential-cancel').addEventListener('click', () => finish(null));
            overlay.querySelectorAll('.clear-token').forEach(checkbox => {
                checkbox.addEventListener('change', () => {
                    const input = document.getElementById(checkbox.dataset.input);
                    input.disabled = checkbox.checked;
                    if (checkbox.checked) {
                        input.value = '';
                    }
                });
            });
            overlay.querySelector('#credential-token-form').addEventListener('submit', event => {
                event.preventDefault();
                const form = event.currentTarget;
                if (!form.reportValidity()) {
                    return;
                }
                const formData = new FormData(form);
                finish({
                    accessToken: formData.get('accessToken').trim(),
                    refreshToken: (formData.get('refreshToken') || '').trim(),
                    idToken: (formData.get('idToken') || '').trim(),
                    clearRefreshToken: formData.get('clearRefreshToken') === 'on',
                    clearIdToken: formData.get('clearIdToken') === 'on',
                    expiresAt: formData.get('expiresAt').trim()
                });
            });
        });
    }

    async deleteCredential(endpointName, credentialId) {
        const actionVersion = this.beginAction();
        const confirmed = await confirmDialog({
            title: t('endpoints.deleteCredential'),
            message: t('endpoints.confirmDeleteCredential').replace('{id}', credentialId),
            confirmLabel: t('common.delete'),
            cancelLabel: t('common.cancel'),
            danger: true
        });
        if (!confirmed) {
            return;
        }
        if (!this.isActionCurrent(actionVersion)) {
            return;
        }

        try {
            await this.queueMutation(() => api.deleteEndpointCredential(endpointName, credentialId));
            if (!this.isActionCurrent(actionVersion)) {
                return;
            }
            notifications.success(t('notifications.credentialDeleted'));
            await this.refreshTokenPoolAfterAction(endpointName, actionVersion);
        } catch (error) {
            if (this.isActionCurrent(actionVersion)) {
                notifications.error(`${t('endpoints.failedToDeleteCredential')}: ${error.message}`);
            }
        }
    }

    closeModal() {
        this.invalidateActions();
        closeAllModals();
    }

    destroy() {
        this.renderVersion++;
        this.invalidateActions();
        closeAllModals();
    }
}

export const endpoints = new Endpoints();

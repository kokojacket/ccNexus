import { api } from './api.js';
import { state } from './state.js';
import { endpoints } from './components/endpoints.js';
import { stats } from './components/stats.js';
import { escapeHtml } from './utils/formatters.js';
import { activateModal, closeAllModals } from './utils/modal.js';
import { notifications } from './utils/notifications.js';
import { getLanguage, initLanguage, loadTranslations, setLanguage, t } from './utils/i18n.js';
import zhCN from './i18n/zh-CN.js';
import en from './i18n/en.js';

loadTranslations({ 'zh-CN': zhCN, en });
initLanguage();
state.update('currentView', 'endpoints');

let eventSource = null;
let realtimeDisconnected = false;
let realtimeRefreshTimer = null;

function updateStaticText() {
    document.title = t('common.pageTitle');
    document.getElementById('app-subtitle').textContent = t('dashboard.subtitle');
    document.getElementById('port-label').textContent = t('settings.port');

    const languageButton = document.getElementById('lang-toggle');
    languageButton.firstElementChild.textContent = getLanguage() === 'zh-CN' ? '中' : 'EN';
    languageButton.title = t('common.toggleLanguage');
    languageButton.setAttribute('aria-label', t('common.toggleLanguage'));

    const settingsButton = document.getElementById('settings-btn');
    settingsButton.title = t('settings.title');
    settingsButton.setAttribute('aria-label', t('settings.title'));
    updateThemeButton();
}

function updateThemeButton() {
    const button = document.getElementById('theme-toggle');
    const isDark = document.body.classList.contains('dark-theme');
    const label = t(isDark ? 'common.switchToLightTheme' : 'common.switchToDarkTheme');
    button.title = label;
    button.setAttribute('aria-label', label);
}

function initTheme() {
    const isDark = localStorage.getItem('theme') === 'dark';
    document.body.classList.toggle('dark-theme', isDark);
    updateThemeButton();
    document.getElementById('theme-toggle').addEventListener('click', () => {
        const enabled = document.body.classList.toggle('dark-theme');
        localStorage.setItem('theme', enabled ? 'dark' : 'light');
        updateThemeButton();
        window.dispatchEvent(new Event('themeChanged'));
    });
}

function initLanguageToggle() {
    document.getElementById('lang-toggle').addEventListener('click', () => {
        setLanguage(getLanguage() === 'zh-CN' ? 'en' : 'zh-CN');
    });
    window.addEventListener('languageChanged', () => {
        closeAllModals();
        updateStaticText();
    });
}

async function loadPort() {
    try {
        const data = await api.getPort();
        document.getElementById('proxy-port').textContent = data.port ?? '-';
    } catch (error) {
        document.getElementById('proxy-port').textContent = '-';
    }
}

async function showSettingsModal() {
    try {
        const [portData, logData, authData] = await Promise.all([
            api.getPort(),
            api.getLogLevel(),
            api.getBasicAuth()
        ]);
        closeAllModals();
        const container = document.getElementById('modal-container');
        container.innerHTML = `
            <div class="modal-overlay">
                <div class="modal settings-modal">
                    <div class="modal-header">
                        <h2 class="modal-title">⚙ ${t('settings.title')}</h2>
                        <button class="modal-close" type="button" aria-label="${t('common.close')}">×</button>
                    </div>
                    <div class="modal-body">
                        <form id="settings-form">
                            <div class="form-group">
                                <label class="form-label" for="settings-port">${t('settings.port')}</label>
                                <input class="form-input" id="settings-port" name="port" type="number" min="1" max="65535" value="${portData.port}" ${portData.portLocked ? 'disabled' : ''} required>
                                <small class="form-hint">${t(portData.portLocked ? 'settings.portLocked' : 'settings.portRestart')}</small>
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="settings-log-level">${t('settings.logLevel')}</label>
                                <select class="form-select" id="settings-log-level" name="logLevel">
                                    ${[0, 1, 2, 3].map(level => `<option value="${level}" ${logData.logLevel === level ? 'selected' : ''}>${t(`settings.logLevel${level}`)}</option>`).join('')}
                                </select>
                            </div>
                            <div class="settings-divider"></div>
                            <label class="form-check-row">
                                <input class="form-checkbox" id="settings-basic-auth" name="basicAuth" type="checkbox" ${authData.enabled ? 'checked' : ''}>
                                <span>${t('settings.basicAuth')}</span>
                            </label>
                            <div class="form-group">
                                <label class="form-label" for="settings-username">${t('settings.username')}</label>
                                <input class="form-input" id="settings-username" name="username" type="text" value="${escapeHtml(authData.username || '')}" autocomplete="username">
                            </div>
                            <div class="form-group">
                                <label class="form-label" for="settings-password">${t('settings.password')}</label>
                                <input class="form-input" id="settings-password" name="password" type="password" autocomplete="new-password">
                                <small class="form-hint">${t(authData.hasPassword ? 'settings.passwordKeep' : 'settings.passwordRequired')}</small>
                            </div>
                        </form>
                    </div>
                    <div class="modal-footer">
                        <button class="btn btn-secondary settings-cancel" type="button">${t('common.cancel')}</button>
                        <button class="btn btn-primary" type="submit" form="settings-form">${t('common.save')}</button>
                    </div>
                </div>
            </div>`;

        const overlay = container.querySelector('.modal-overlay');
        const controller = activateModal(overlay, { initialFocus: '#settings-port' });
        overlay.querySelector('.modal-close').addEventListener('click', () => controller.close());
        overlay.querySelector('.settings-cancel').addEventListener('click', () => controller.close());
        overlay.querySelector('#settings-form').addEventListener('submit', async event => {
            event.preventDefault();
            const form = new FormData(event.currentTarget);
            const basicAuth = form.get('basicAuth') === 'on';
            const username = String(form.get('username') || '').trim();
            const password = String(form.get('password') || '');
            if (basicAuth && (!username || (!password && !authData.hasPassword))) {
                notifications.error(t('settings.authRequired'));
                return;
            }
            const config = { logLevel: Number(form.get('logLevel')) };
            if (!portData.portLocked) {
                config.port = Number(form.get('port'));
            }
            try {
                await api.updateConfig(config);
                await api.updateBasicAuth({ enabled: basicAuth, username, password });
                document.getElementById('proxy-port').textContent = config.port ?? portData.port;
                notifications.success(t('settings.saved'));
                controller.close();
            } catch (error) {
                notifications.error(`${t('settings.failedToSave')}: ${error.message}`);
            }
        });
    } catch (error) {
        notifications.error(`${t('settings.failedToLoad')}: ${error.message}`);
    }
}

function scheduleRealtimeRefresh() {
    if (realtimeRefreshTimer !== null) {
        return;
    }
    realtimeRefreshTimer = setTimeout(() => {
        realtimeRefreshTimer = null;
        stats.refreshRealtime();
    }, 300);
}

function setRealtimeStatus(connected) {
    const status = document.getElementById('realtime-status');
    status.textContent = t(connected ? 'notifications.realtimeConnected' : 'notifications.realtimeDisconnected');
    status.classList.toggle('is-offline', !connected);
}

function initRealtime() {
    eventSource?.close();
    eventSource = new EventSource('/api/events');
    eventSource.onmessage = event => {
        try {
            const data = JSON.parse(event.data);
            if (data.type === 'stats') {
                state.update('stats', data.stats);
                state.update('currentEndpoint', data.currentEndpoint);
                scheduleRealtimeRefresh();
            }
        } catch (error) {
            console.error('Failed to parse SSE event:', error);
        }
    };
    eventSource.onopen = () => {
        setRealtimeStatus(true);
        if (realtimeDisconnected) {
            notifications.success(t('notifications.realtimeRestored'));
            realtimeDisconnected = false;
        }
    };
    eventSource.onerror = () => {
        setRealtimeStatus(false);
        if (!realtimeDisconnected) {
            notifications.warning(t('notifications.realtimeDisconnected'));
            realtimeDisconnected = true;
        }
    };
}

async function init() {
    initTheme();
    initLanguageToggle();
    updateStaticText();
    document.getElementById('settings-btn').addEventListener('click', showSettingsModal);
    document.getElementById('port-settings-btn').addEventListener('click', showSettingsModal);
    await Promise.all([stats.render(), endpoints.render(), loadPort()]);
    initRealtime();
    window.addEventListener('beforeunload', () => {
        clearTimeout(realtimeRefreshTimer);
        eventSource?.close();
    }, { once: true });
}

if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init, { once: true });
} else {
    init();
}

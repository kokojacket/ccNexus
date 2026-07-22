import { router } from './router.js';
import { state } from './state.js';
import { setLanguage, getLanguage, initLanguage, loadTranslations, t } from './utils/i18n.js';
import { dashboard } from './components/dashboard.js';
import { endpoints } from './components/endpoints.js';
import { stats } from './components/stats.js';
import { testing } from './components/testing.js';
import { notifications } from './utils/notifications.js';
import zhCN from './i18n/zh-CN.js';
import en from './i18n/en.js';

// 加载翻译
loadTranslations({ 'zh-CN': zhCN, 'en': en });
initLanguage();

// Initialize theme
function initTheme() {
    const savedTheme = localStorage.getItem('theme') || 'light';
    document.body.classList.toggle('dark-theme', savedTheme === 'dark');

    const themeToggle = document.getElementById('theme-toggle');
    themeToggle.addEventListener('click', () => {
        const isDark = document.body.classList.toggle('dark-theme');
        localStorage.setItem('theme', isDark ? 'dark' : 'light');
        updateThemeToggle(themeToggle, isDark);
        window.dispatchEvent(new Event('themeChanged'));
    });

    updateThemeToggle(themeToggle, savedTheme === 'dark');
}

function updateThemeToggle(button, isDark) {
    const label = t(isDark ? 'common.switchToLightTheme' : 'common.switchToDarkTheme');
    button.querySelector('.icon').textContent = isDark ? 'L' : 'D';
    button.title = label;
    button.setAttribute('aria-label', label);
}

// Update sidebar translations
function updateSidebarTranslations() {
    document.title = t('common.pageTitle');

    // Update subtitle
    const subtitle = document.getElementById('sidebar-subtitle');
    if (subtitle) {
        subtitle.textContent = t('dashboard.subtitle');
    }

    // Update navigation menu items
    document.querySelectorAll('.nav-label').forEach(el => {
        const key = el.getAttribute('data-i18n');
        if (key) {
            el.textContent = t(key);
        }
    });

    const skipLink = document.querySelector('.skip-link');
    if (skipLink) {
        skipLink.textContent = t('common.skipToContent');
    }
    const langToggle = document.getElementById('lang-toggle');
    langToggle.title = t('common.toggleLanguage');
    langToggle.setAttribute('aria-label', t('common.toggleLanguage'));
    updateThemeToggle(document.getElementById('theme-toggle'), document.body.classList.contains('dark-theme'));
}

// Initialize language toggle
function initLanguageToggle() {
    const langToggle = document.getElementById('lang-toggle');
    const langLabels = {
        'zh-CN': '中',
        'en': 'EN'
    };

    // 设置初始图标
    langToggle.querySelector('.icon').textContent = langLabels[getLanguage()];

    langToggle.addEventListener('click', () => {
        const currentLang = getLanguage();
        const newLang = currentLang === 'zh-CN' ? 'en' : 'zh-CN';
        setLanguage(newLang);
        langToggle.querySelector('.icon').textContent = langLabels[newLang];
        // 更新侧边栏翻译
        updateSidebarTranslations();
    });
}

// Initialize real-time updates
let eventSource = null;
let realtimeDisconnected = false;
let realtimeRefreshTimer = null;

function scheduleRealtimeRefresh() {
    const scheduledView = state.get('currentView');
    if (!['dashboard', 'stats'].includes(scheduledView) || realtimeRefreshTimer !== null) {
        return;
    }
    realtimeRefreshTimer = setTimeout(() => {
        realtimeRefreshTimer = null;
        if (state.get('currentView') !== scheduledView) {
            return;
        }
        if (scheduledView === 'dashboard') {
            dashboard.refreshRealtime();
        } else {
            stats.refreshRealtime();
        }
    }, 300);
}

function initRealtime() {
    if (eventSource) {
        eventSource.close();
    }
    eventSource = new EventSource('/api/events');

    eventSource.onmessage = (event) => {
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
        if (realtimeDisconnected) {
            notifications.success(t('notifications.realtimeRestored'));
            realtimeDisconnected = false;
        }
    };

    eventSource.onerror = (error) => {
        console.error('SSE connection error:', error);
        if (!realtimeDisconnected) {
            realtimeDisconnected = true;
            notifications.warning(t('notifications.realtimeDisconnected'));
        }
        // EventSource reconnects automatically.
    };
}

// Initialize application
function init() {
    // Register routes
    router.register('dashboard', dashboard);
    router.register('endpoints', endpoints);
    router.register('stats', stats);
    router.register('testing', testing);

    // Initialize theme
    initTheme();

    // Initialize language toggle
    initLanguageToggle();

    // Initialize sidebar translations
    updateSidebarTranslations();

    // Initialize router
    router.init();

    // Initialize real-time updates
    initRealtime();

    window.addEventListener('beforeunload', () => {
        clearTimeout(realtimeRefreshTimer);
        eventSource?.close();
    }, { once: true });

    console.log('ccNexus Admin initialized');
}

// Start application when DOM is ready
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}

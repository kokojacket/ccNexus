// Utility functions for formatting data
import { getLanguage, t } from './i18n.js';

export function formatNumber(num) {
    if (num >= 1000000) {
        return (num / 1000000).toFixed(1) + 'M';
    }
    if (num >= 1000) {
        return (num / 1000).toFixed(1) + 'K';
    }
    return num.toString();
}

export function formatTokens(tokens) {
    return formatNumber(tokens);
}

export function formatPercentage(value) {
    const sign = value >= 0 ? '+' : '';
    return `${sign}${value.toFixed(1)}%`;
}

export function formatDate(dateString) {
    const date = new Date(dateString);
    return date.toLocaleDateString(getLanguage());
}

export function formatDateTime(dateString) {
    const date = new Date(dateString);
    return date.toLocaleString(getLanguage());
}

export function formatLatency(ms) {
    if (ms < 1000) {
        return `${ms}ms`;
    }
    return `${(ms / 1000).toFixed(2)}s`;
}

export function getTransformerLabel(transformer) {
    const key = `transformers.${transformer}`;
    const label = t(key);
    return label === key ? transformer : label;
}

export function getStatusBadge(enabled) {
    if (enabled) {
        return `<span class="badge badge-success">${t('common.enabled')}</span>`;
    }
    return `<span class="badge badge-danger">${t('common.disabled')}</span>`;
}

export function escapeHtml(text) {
    const entities = {
        '&': '&amp;',
        '<': '&lt;',
        '>': '&gt;',
        '"': '&quot;',
        "'": '&#39;'
    };
    return String(text ?? '').replace(/[&<>"']/g, character => entities[character]);
}

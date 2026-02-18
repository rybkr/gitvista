/**
 * @fileoverview Formatting helpers for the Git visualization UI.
 * Provides reusable utilities to keep display logic consistent.
 */

/**
 * Returns a shortened commit hash suitable for labels and tooltips.
 *
 * @param {string} hash Full commit hash string.
 * @returns {string} Seven-character abbreviated hash when possible,
 * otherwise the original input.
 */
export function shortenHash(hash) {
    if (typeof hash !== "string") {
        return hash;
    }

    return hash.length >= 7 ? hash.slice(0, 7) : hash;
}

/**
 * Formats a date string or timestamp as a human-readable relative time string.
 * Examples: "just now", "5m ago", "3h ago", "2d ago", "1mo ago", "2y ago".
 *
 * @param {string|number|Date} dateInput Date string, Unix timestamp (ms), or Date object.
 * @returns {string} Relative time string.
 */
export function formatRelativeTime(dateInput) {
    const date = new Date(dateInput);
    const now = Date.now();
    const diffMs = now - date.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    if (diffMin < 1) return "just now";
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay < 30) return `${diffDay}d ago`;
    const diffMon = Math.floor(diffDay / 30);
    if (diffMon < 12) return `${diffMon}mo ago`;
    const diffYr = Math.floor(diffDay / 365);
    return `${diffYr}y ago`;
}

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
 * Returns "—" when dateInput is null, undefined, or produces an invalid Date,
 * preventing "NaNy ago" from appearing in the UI for missing timestamps.
 *
 * @param {string|number|Date|null|undefined} dateInput Date string, Unix timestamp (ms), or Date object.
 * @returns {string} Relative time string, or "—" when the input is missing or unparseable.
 */
export function formatRelativeTime(dateInput) {
    // Reject null/undefined before constructing a Date — new Date(null) produces
    // epoch (0) rather than an Invalid Date, which would silently show "55y ago".
    if (dateInput == null) return "\u2014";
    const date = new Date(dateInput);
    // Reject any input that the Date constructor could not parse (e.g. empty
    // string, garbage value, or NaN), which would otherwise propagate NaN through
    // all arithmetic and produce "NaNy ago" as the final output.
    if (isNaN(date.getTime())) return "\u2014";
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

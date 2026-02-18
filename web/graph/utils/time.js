/**
 * @fileoverview Time-related helpers for graph entities.
 * Provides utilities for parsing commit timestamps safely and formatting
 * human-readable relative time strings.
 */

/**
 * Formats a date-like value as a short relative time string (e.g. "3d ago").
 * Returns an empty string when the input is missing or unparseable.
 *
 * @param {string | Date | null | undefined} when Date string or Date object.
 * @returns {string} Relative time string, or "" when unavailable.
 */
export function relativeTime(when) {
    if (!when) return "";
    const ms = when instanceof Date ? when.getTime() : Date.parse(String(when));
    if (!Number.isFinite(ms) || ms === 0) return "";
    const seconds = Math.floor((Date.now() - ms) / 1000);
    if (seconds < 60) return "just now";
    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    const days = Math.floor(hours / 24);
    if (days < 30) return `${days}d ago`;
    const months = Math.floor(days / 30);
    if (months < 12) return `${months}mo ago`;
    return `${Math.floor(months / 12)}y ago`;
}

/**
 * Returns the timestamp (ms) associated with a commit object.
 *
 * @param {import("../types.js").GraphCommit | undefined | null} commit Commit data structure.
 * @returns {number} Millisecond timestamp or 0 when unavailable.
 */
export function getCommitTimestamp(commit) {
    if (!commit) {
        return 0;
    }

    const when =
        commit.committer?.when ??
        commit.author?.when ??
        commit.committer?.When ??
        commit.author?.When;
    const time = new Date(when ?? 0).getTime();
    if (!Number.isFinite(time) || Number.isNaN(time)) {
        return 0;
    }
    return time;
}

/**
 * Instrumented fetch wrapper for API calls.
 *
 * Provides a cross-cutting layer that:
 *   - Detects "Repository not available" responses and updates errorState
 *   - Tracks consecutive failures for repository-unavailable escalation
 *   - Resets failure tracking on success
 *
 * Does NOT auto-retry — retry decisions belong to the calling component.
 * Returns the raw Response so callers handle JSON parsing themselves.
 */

import { setRepositoryAvailable } from "./errorState.js";
import { logger } from "./logger.js";

const REPO_UNAVAILABLE_MSG = "Repository not available";
const ESCALATION_THRESHOLD = 3;
const ESCALATION_WINDOW_MS = 10_000;

/** @type {{ time: number }[]} */
let recentFailures = [];

/**
 * Fetch wrapper for API endpoints.
 *
 * @param {string} url - The API URL to fetch
 * @param {RequestInit} [options] - Standard fetch options
 * @returns {Promise<Response>}
 */
export async function apiFetch(url, options) {
    let response;
    try {
        response = await fetch(url, options);
    } catch (err) {
        trackFailure();
        throw err;
    }

    if (response.ok) {
        recentFailures = [];
        return response;
    }

    if (response.status === 500) {
        // Clone before reading body so callers can still consume the original
        const clone = response.clone();
        try {
            const text = await clone.text();
            if (text.includes(REPO_UNAVAILABLE_MSG)) {
                logger.warn("Repository not available detected", { url });
                setRepositoryAvailable(false);
                return response;
            }
        } catch {
            // Body read failed — fall through
        }
        trackFailure();
    }

    return response;
}

function trackFailure() {
    const now = Date.now();
    recentFailures.push({ time: now });
    // Trim to window
    recentFailures = recentFailures.filter(
        (f) => now - f.time < ESCALATION_WINDOW_MS,
    );
    if (recentFailures.length >= ESCALATION_THRESHOLD) {
        logger.warn("Multiple consecutive API failures — marking repository unavailable");
        setRepositoryAvailable(false);
    }
}

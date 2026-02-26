/**
 * @fileoverview Default graph settings values and localStorage persistence helpers.
 * Shared between graphSettings.js (UI) and graphController.js (application).
 */

const STORAGE_KEY = "gitvista-graph-settings";

/** Default physics values matching constants.js defaults. */
export const DEFAULT_PHYSICS = Object.freeze({
    chargeStrength: -110,
    linkDistance: 50,
    collisionRadius: 14,
    velocityDecay: 0.55,
});

/** Default scope values (no filtering). */
export const DEFAULT_SCOPE = Object.freeze({
    depthLimit: Infinity,
    timeWindow: "all",
    branchRules: {},
});

/** Returns a fresh copy of the full default settings object. */
export function getDefaults() {
    return {
        scope: { ...DEFAULT_SCOPE, branchRules: {} },
        physics: { ...DEFAULT_PHYSICS },
    };
}

/**
 * Loads persisted settings from localStorage, merging with defaults.
 * Returns defaults on parse failure.
 *
 * @returns {{ scope: typeof DEFAULT_SCOPE, physics: typeof DEFAULT_PHYSICS }}
 */
export function loadSettings() {
    const defaults = getDefaults();
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return defaults;
        const parsed = JSON.parse(raw);
        if (parsed.physics) {
            for (const key of Object.keys(defaults.physics)) {
                if (typeof parsed.physics[key] === "number") {
                    defaults.physics[key] = parsed.physics[key];
                }
            }
        }
        if (parsed.scope) {
            if (typeof parsed.scope.depthLimit === "number" || parsed.scope.depthLimit === "Infinity") {
                defaults.scope.depthLimit = parsed.scope.depthLimit === "Infinity"
                    ? Infinity : parsed.scope.depthLimit;
            }
            if (typeof parsed.scope.timeWindow === "string") {
                defaults.scope.timeWindow = parsed.scope.timeWindow;
            }
            if (parsed.scope.branchRules && typeof parsed.scope.branchRules === "object") {
                defaults.scope.branchRules = { ...parsed.scope.branchRules };
            }
        }
        return defaults;
    } catch {
        return defaults;
    }
}

/**
 * Persists settings to localStorage.
 *
 * @param {{ scope: object, physics: object }} settings
 */
export function saveSettings(settings) {
    try {
        const serializable = {
            scope: {
                ...settings.scope,
                depthLimit: settings.scope.depthLimit === Infinity
                    ? "Infinity" : settings.scope.depthLimit,
            },
            physics: { ...settings.physics },
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(serializable));
    } catch {
        // Silently fail on storage quota errors
    }
}

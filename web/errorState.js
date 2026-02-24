/**
 * Global error state module.
 *
 * Tracks two cross-cutting error conditions:
 *   1. WebSocket connection state (connected / reconnecting / disconnected)
 *   2. Repository availability (whether the backend can serve data)
 *
 * Components subscribe to changes via subscribe(). Per-component errors
 * (e.g. "this tree fetch failed") are NOT tracked here — they remain local.
 */

/** @typedef {"connected" | "reconnecting" | "disconnected"} ConnectionState */

const state = {
    /** @type {ConnectionState} */
    connectionState: "connected",
    /** @type {number} */
    reconnectAttempt: 0,
    /** @type {boolean} */
    repositoryAvailable: true,
};

/** @type {Set<(state: typeof state) => void>} */
const subscribers = new Set();

function notify() {
    const snapshot = { ...state };
    for (const fn of subscribers) {
        fn(snapshot);
    }
}

/**
 * Update the connection state.
 * @param {ConnectionState} connectionState
 * @param {number} [attempt]
 */
export function setConnectionState(connectionState, attempt) {
    state.connectionState = connectionState;
    if (attempt !== undefined) {
        state.reconnectAttempt = attempt;
    } else if (connectionState === "connected") {
        state.reconnectAttempt = 0;
    }
    notify();
}

/**
 * Update repository availability.
 * @param {boolean} available
 */
export function setRepositoryAvailable(available) {
    state.repositoryAvailable = available;
    notify();
}

/**
 * Subscribe to state changes.
 * @param {(state: typeof state) => void} callback
 * @returns {() => void} Unsubscribe function
 */
export function subscribe(callback) {
    subscribers.add(callback);
    return () => subscribers.delete(callback);
}

/**
 * Get the current state snapshot.
 */
export function getState() {
    return { ...state };
}

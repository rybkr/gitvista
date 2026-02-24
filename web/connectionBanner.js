/**
 * Connection status banner.
 *
 * Fixed-position banner at the top of the viewport that shows when the
 * WebSocket connection is lost. Subscribes to errorState.
 *
 * - Hidden when connected
 * - Amber when reconnecting (with attempt count)
 * - Red when disconnected
 * - Brief green "Reconnected" on recovery
 * - Dismissible per-episode
 */

import { subscribe, getState } from "./errorState.js";

export function createConnectionBanner() {
    const el = document.createElement("div");
    el.className = "connection-banner";
    el.setAttribute("role", "alert");
    el.style.display = "none";

    const textEl = document.createElement("span");
    textEl.className = "connection-banner-text";
    el.appendChild(textEl);

    const dismissBtn = document.createElement("button");
    dismissBtn.className = "connection-banner-dismiss";
    dismissBtn.textContent = "\u00D7";
    dismissBtn.setAttribute("aria-label", "Dismiss connection warning");
    el.appendChild(dismissBtn);

    let dismissed = false;
    let wasDisconnected = false;
    let successTimeout = null;

    dismissBtn.addEventListener("click", () => {
        dismissed = true;
        el.style.display = "none";
    });

    subscribe((state) => {
        if (successTimeout) {
            clearTimeout(successTimeout);
            successTimeout = null;
        }

        if (state.connectionState === "connected") {
            if (wasDisconnected) {
                // Show brief "Reconnected" success banner
                el.className = "connection-banner connection-banner--success";
                el.setAttribute("role", "status");
                textEl.textContent = "Reconnected";
                el.style.display = "flex";
                dismissed = false;

                successTimeout = setTimeout(() => {
                    el.style.display = "none";
                }, 3000);
            } else {
                el.style.display = "none";
            }
            wasDisconnected = false;
            return;
        }

        wasDisconnected = true;

        if (dismissed) {
            return;
        }

        if (state.connectionState === "reconnecting") {
            el.className = "connection-banner connection-banner--warning";
            el.setAttribute("role", "status");
            const attempt = state.reconnectAttempt || 1;
            textEl.textContent = `Connection lost. Reconnecting\u2026 (attempt ${attempt})`;
            el.style.display = "flex";
        } else {
            el.className = "connection-banner connection-banner--error";
            el.setAttribute("role", "alert");
            textEl.textContent = "Disconnected from server";
            el.style.display = "flex";
        }
    });

    // Set initial state
    const initial = getState();
    if (initial.connectionState !== "connected") {
        wasDisconnected = true;
        textEl.textContent = initial.connectionState === "reconnecting"
            ? "Connection lost. Reconnecting\u2026"
            : "Disconnected from server";
        el.className = `connection-banner connection-banner--${
            initial.connectionState === "reconnecting" ? "warning" : "error"
        }`;
        el.style.display = "flex";
    }

    return { el };
}

/**
 * @fileoverview Keyboard shortcut help overlay for GitVista.
 * Renders a floating card listing all keyboard shortcuts.
 * Toggled by the ? key; dismissed by Escape or a click outside the card.
 */

// Guard so the stylesheet is only injected once even if called multiple times.
let stylesInjected = false;

/** All shortcut rows displayed in the help table. */
const SHORTCUTS = [
    { keys: ["G", "H"], description: "Jump to HEAD commit" },
    { keys: ["J"], description: "Navigate to next (newer) commit" },
    { keys: ["K"], description: "Navigate to previous (older) commit" },
    { keys: ["N"], description: "Jump to next search result" },
    { keys: ["Shift+N"], description: "Jump to previous search result" },
    { keys: ["/"], description: "Focus search" },
    { keys: ["?"], description: "Toggle this help overlay" },
    { keys: ["Esc"], description: "Dismiss overlay / deselect" },
];

/**
 * Injects the help overlay stylesheet into the document <head> once.
 * Using a <style> block keeps this module self-contained without adding
 * an external CSS file.
 */
function injectStyles() {
    if (stylesInjected) {
        return;
    }
    stylesInjected = true;

    const style = document.createElement("style");
    style.textContent = `
.kbd-help-backdrop {
    position: fixed;
    inset: 0;
    z-index: 200;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(27, 31, 36, 0.45);
    backdrop-filter: blur(2px);
    animation: kbd-help-fade-in 0.12s ease;
}

@media (prefers-color-scheme: dark) {
    .kbd-help-backdrop {
        background: rgba(0, 0, 0, 0.6);
    }
}

@keyframes kbd-help-fade-in {
    from { opacity: 0; }
    to   { opacity: 1; }
}

.kbd-help-card {
    background: var(--surface-color);
    border: 1px solid var(--border-color);
    border-radius: 10px;
    box-shadow: 0 16px 48px rgba(27, 31, 36, 0.25);
    padding: 24px 28px;
    min-width: 340px;
    max-width: min(480px, 90vw);
    animation: kbd-help-slide-up 0.14s ease;
}

@media (prefers-color-scheme: dark) {
    .kbd-help-card {
        box-shadow: 0 16px 48px rgba(0, 0, 0, 0.6);
    }
}

@keyframes kbd-help-slide-up {
    from { transform: translateY(8px); opacity: 0; }
    to   { transform: translateY(0);   opacity: 1; }
}

.kbd-help-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-color);
    margin-bottom: 16px;
    padding-bottom: 12px;
    border-bottom: 1px solid var(--border-color);
}

.kbd-help-table {
    width: 100%;
    border-collapse: collapse;
}

.kbd-help-table tr + tr td {
    padding-top: 8px;
}

.kbd-help-keys {
    white-space: nowrap;
    padding-right: 20px;
    vertical-align: middle;
}

.kbd-help-desc {
    font-size: 13px;
    color: var(--text-color);
    vertical-align: middle;
}

.kbd-key {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 24px;
    padding: 2px 6px;
    border: 1px solid var(--border-color);
    border-bottom-width: 2px;
    border-radius: 4px;
    background: var(--bg-color);
    font-family: ui-monospace, SFMono-Regular, SF Mono, Menlo, Consolas, Liberation Mono, monospace;
    font-size: 11px;
    font-weight: 600;
    color: var(--text-color);
    line-height: 1.5;
}

.kbd-key-sep {
    font-size: 11px;
    color: var(--text-secondary);
    margin: 0 3px;
}

.kbd-help-footer {
    margin-top: 16px;
    padding-top: 12px;
    border-top: 1px solid var(--border-color);
    font-size: 11px;
    color: var(--text-secondary);
    text-align: center;
}
`;
    document.head.appendChild(style);
}

/**
 * Creates the keyboard help overlay controller.
 *
 * @returns {{ toggle(): void, show(): void, hide(): void, destroy(): void }}
 */
export function createKeyboardHelp() {
    injectStyles();

    let backdropEl = null;

    /**
     * Builds a keyboard key badge element.
     *
     * @param {string} key Label text for the key.
     * @returns {HTMLElement}
     */
    function makeKeyBadge(key) {
        const span = document.createElement("span");
        span.className = "kbd-key";
        span.textContent = key;
        return span;
    }

    /**
     * Builds the full overlay DOM and appends it to the document body.
     * Clicking the backdrop (outside the card) dismisses the overlay.
     */
    function buildOverlay() {
        const backdrop = document.createElement("div");
        backdrop.className = "kbd-help-backdrop";
        backdrop.setAttribute("role", "dialog");
        backdrop.setAttribute("aria-modal", "true");
        backdrop.setAttribute("aria-label", "Keyboard shortcuts");

        const card = document.createElement("div");
        card.className = "kbd-help-card";

        const title = document.createElement("div");
        title.className = "kbd-help-title";
        title.textContent = "Keyboard Shortcuts";
        card.appendChild(title);

        const table = document.createElement("table");
        table.className = "kbd-help-table";

        for (const { keys, description } of SHORTCUTS) {
            const row = document.createElement("tr");

            const keysTd = document.createElement("td");
            keysTd.className = "kbd-help-keys";

            keys.forEach((key, idx) => {
                if (idx > 0) {
                    const sep = document.createElement("span");
                    sep.className = "kbd-key-sep";
                    // "then" separator for sequential key presses; "or" would use /
                    sep.textContent = "then";
                    keysTd.appendChild(sep);
                }
                keysTd.appendChild(makeKeyBadge(key));
            });

            const descTd = document.createElement("td");
            descTd.className = "kbd-help-desc";
            descTd.textContent = description;

            row.appendChild(keysTd);
            row.appendChild(descTd);
            table.appendChild(row);
        }

        card.appendChild(table);

        const footer = document.createElement("div");
        footer.className = "kbd-help-footer";
        footer.textContent = "Press ? or Esc to close";
        card.appendChild(footer);

        // Clicking directly on the backdrop (not the card) dismisses the overlay.
        backdrop.addEventListener("click", (event) => {
            if (event.target === backdrop) {
                hide();
            }
        });

        // Stop click events on the card from bubbling up to the backdrop handler.
        card.addEventListener("click", (event) => {
            event.stopPropagation();
        });

        backdrop.appendChild(card);
        document.body.appendChild(backdrop);
        return backdrop;
    }

    function show() {
        if (backdropEl) {
            return;
        }
        backdropEl = buildOverlay();
    }

    function hide() {
        if (!backdropEl) {
            return;
        }
        backdropEl.remove();
        backdropEl = null;
    }

    function toggle() {
        if (backdropEl) {
            hide();
        } else {
            show();
        }
    }

    function destroy() {
        hide();
    }

    return { toggle, show, hide, destroy };
}

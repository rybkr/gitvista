/**
 * Reusable inline error component with retry button.
 *
 * Factory function returns a DOM element. Used by file explorer, diff view,
 * file content viewer, etc. when individual fetch operations fail.
 *
 * @param {{ message: string, onRetry?: () => void }} options
 * @returns {HTMLElement}
 */
export function createInlineError({ message, onRetry }) {
    const el = document.createElement("div");
    el.className = "inline-error";
    el.setAttribute("role", "alert");

    const icon = document.createElement("span");
    icon.className = "inline-error-icon";
    icon.textContent = "\u26A0";
    icon.setAttribute("aria-hidden", "true");
    el.appendChild(icon);

    const msg = document.createElement("span");
    msg.className = "inline-error-message";
    msg.textContent = message;
    el.appendChild(msg);

    if (onRetry) {
        const btn = document.createElement("button");
        btn.className = "inline-error-retry";
        btn.textContent = "Retry";
        btn.setAttribute("aria-label", `Retry: ${message}`);
        btn.addEventListener("click", onRetry);
        el.appendChild(btn);
    }

    return el;
}

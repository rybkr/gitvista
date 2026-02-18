/**
 * @fileoverview Toast notification system for GitVista.
 * Displays brief dismissible cards anchored to the bottom-left of the viewport.
 * Queues toasts and limits visible count to MAX_VISIBLE.
 */

const MAX_VISIBLE = 3;
const DEFAULT_DURATION_MS = 4000;

const CONTAINER_STYLES = `
    position: fixed;
    bottom: 48px;
    left: 16px;
    z-index: 9000;
    display: flex;
    flex-direction: column-reverse;
    gap: 8px;
    pointer-events: none;
`;

const TOAST_BASE_STYLES = `
    pointer-events: auto;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    padding: 10px 14px;
    border-radius: 8px;
    background: var(--surface-color, #ffffff);
    border: 1px solid var(--border-color, #d0d7de);
    box-shadow: 0 4px 12px rgba(0,0,0,0.15);
    font-size: 13px;
    color: var(--text-color, #1f2328);
    max-width: 320px;
    cursor: default;
    opacity: 0;
    transform: translateY(8px);
    transition: opacity 200ms ease, transform 200ms ease;
`;

let container = null;

function getContainer() {
    if (container) return container;
    container = document.createElement("div");
    container.setAttribute("role", "status");
    container.setAttribute("aria-live", "polite");
    container.setAttribute("aria-atomic", "false");
    container.style.cssText = CONTAINER_STYLES;
    document.body.appendChild(container);
    return container;
}

const queue = [];
const visible = [];

/**
 * Displays a toast notification.
 *
 * @param {string} message Text to display.
 * @param {Object} [options]
 * @param {number} [options.duration=4000] Auto-dismiss delay in milliseconds.
 * @param {Function} [options.onClick] Optional callback when the toast body is clicked.
 */
export function showToast(message, options = {}) {
    const duration = options.duration ?? DEFAULT_DURATION_MS;
    const onClick = options.onClick ?? null;
    const entry = { message, duration, onClick };

    if (visible.length >= MAX_VISIBLE) {
        queue.push(entry);
    } else {
        renderToast(entry);
    }
}

function renderToast(entry) {
    const c = getContainer();
    const { message, duration, onClick } = entry;

    const el = document.createElement("div");
    el.style.cssText = TOAST_BASE_STYLES;

    const text = document.createElement("span");
    text.textContent = message;
    el.appendChild(text);

    const dismiss = document.createElement("button");
    dismiss.textContent = "×";
    dismiss.setAttribute("aria-label", "Dismiss notification");
    dismiss.style.cssText = `
        background: none; border: none; cursor: pointer; font-size: 16px;
        line-height: 1; padding: 0 2px; color: var(--text-secondary, #656d76); flex-shrink: 0;
    `;
    el.appendChild(dismiss);

    if (onClick) {
        el.style.cursor = "pointer";
        text.addEventListener("click", () => { onClick(); removeToast(el); });
    }
    dismiss.addEventListener("click", (e) => { e.stopPropagation(); removeToast(el); });

    c.appendChild(el);
    visible.push(el);

    requestAnimationFrame(() => {
        el.style.opacity = "1";
        el.style.transform = "translateY(0)";
    });

    el._toastTimer = setTimeout(() => removeToast(el), duration);
}

function removeToast(el) {
    if (!el.parentElement) return;
    clearTimeout(el._toastTimer);
    el.style.opacity = "0";
    el.style.transform = "translateY(8px)";

    const cleanup = () => {
        clearTimeout(fallbackTimer);
        if (!el.parentElement) return; // Already removed
        el.remove();
        const idx = visible.indexOf(el);
        if (idx !== -1) visible.splice(idx, 1);
        if (queue.length > 0) renderToast(queue.shift());
    };

    // Safety net: if transitionend never fires (tab backgrounded, interrupted
    // animation, etc.), remove the element after the transition duration anyway.
    const fallbackTimer = setTimeout(cleanup, 300);
    el.addEventListener("transitionend", cleanup, { once: true });
}

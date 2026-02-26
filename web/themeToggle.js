/**
 * @fileoverview Theme toggle: cycles system → light → dark.
 * Persists preference in localStorage under "gitvista-theme".
 * Applies data-theme attribute on <html> to override the @media query.
 */

const STORAGE_KEY = "gitvista-theme";
const MODES = ["system", "light", "dark"];

/** Unicode icons for each mode. */
const ICONS = {
    system: "\u25D0",  // Half-circle ◐ — follows OS
    light:  "\u2600",  // Sun ☀
    dark:   "\u263E",  // Crescent moon ☾
};

const LABELS = {
    system: "Theme: follow system",
    light:  "Theme: always light",
    dark:   "Theme: always dark",
};

function getCurrentMode() {
    const stored = localStorage.getItem(STORAGE_KEY);
    return MODES.includes(stored) ? stored : "dark";
}

function applyMode(mode) {
    const root = document.documentElement;
    if (mode === "system") {
        root.removeAttribute("data-theme");
    } else {
        root.setAttribute("data-theme", mode);
    }
}

function nextMode(current) {
    return MODES[(MODES.indexOf(current) + 1) % MODES.length];
}

/**
 * Creates and mounts the theme toggle button, applying any persisted theme immediately.
 *
 * @param {Function} [onThemeChange] Optional callback invoked with the new mode string
 *   after each toggle. Useful for prompting a graph palette re-read.
 */
export function initThemeToggle(onThemeChange) {
    let mode = getCurrentMode();
    applyMode(mode);

    const btn = document.createElement("button");
    btn.className = "theme-toggle";
    btn.type = "button";
    btn.setAttribute("aria-label", LABELS[mode]);
    btn.textContent = ICONS[mode];

    btn.addEventListener("click", () => {
        mode = nextMode(mode);
        localStorage.setItem(STORAGE_KEY, mode);
        applyMode(mode);
        btn.textContent = ICONS[mode];
        btn.setAttribute("aria-label", LABELS[mode]);
        if (typeof onThemeChange === "function") {
            onThemeChange(mode);
        }
    });

    document.body.appendChild(btn);
}

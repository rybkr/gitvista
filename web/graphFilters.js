/**
 * @fileoverview Graph filter panel component.
 * Renders a collapsible strip of toggles and a branch-focus dropdown above the
 * canvas. Changes fire an onChange callback with the updated filter state so
 * callers can push it into the graph controller without tight coupling.
 *
 * Filter state shape:
 *   { hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }
 *
 * focusBranch is the full branch name (e.g. "refs/heads/main") or "" for none.
 */

import { logger } from "./logger.js";

/** localStorage key for persisting filter preferences. */
const STORAGE_KEY = "gitvista-filter-state";

/**
 * Default filter state — everything visible, no branch focus.
 *
 * @returns {{ hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }}
 */
function defaultFilterState() {
    return {
        hideRemotes: false,
        hideMerges: false,
        hideStashes: false,
        focusBranch: "",
    };
}

/**
 * Loads filter state from localStorage. Falls back gracefully on parse errors.
 *
 * @returns {{ hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }}
 */
export function loadFilterState() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return defaultFilterState();
        const parsed = JSON.parse(raw);
        // Merge against defaults so new fields added in future versions are safe.
        return { ...defaultFilterState(), ...parsed };
    } catch {
        return defaultFilterState();
    }
}

/**
 * Persists filter state to localStorage.
 *
 * @param {{ hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }} state
 */
function saveFilterState(state) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
        // Ignore quota errors — filter state loss on next reload is acceptable.
        logger.warn("graphFilters: unable to save filter state to localStorage");
    }
}

/**
 * Creates and mounts the graph filter panel.
 *
 * The panel is a collapsible strip that renders above the graph canvas. It
 * contains three toggles (hide remotes, hide merges, hide stashes) and a
 * branch-focus dropdown populated from the live branch map.
 *
 * @param {HTMLElement} container The element to prepend the panel into (the
 *   graph root element).
 * @param {{
 *   initialState?: { hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string },
 *   onChange: (state: { hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }) => void,
 * }} options
 * @returns {{
 *   el: HTMLElement,
 *   updateBranches: (branches: Map<string, string>) => void,
 *   getState: () => { hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string },
 *   destroy: () => void,
 * }}
 */
export function createGraphFilters(container, options) {
    const { onChange } = options;

    // Clone to avoid mutating the caller's object.
    let filterState = { ...(options.initialState ?? loadFilterState()) };

    // ── Root element ──────────────────────────────────────────────────────────

    const el = document.createElement("div");
    el.className = "graph-filter-panel";
    el.setAttribute("role", "toolbar");
    el.setAttribute("aria-label", "Graph filters");

    // ── Collapse toggle ───────────────────────────────────────────────────────

    const header = document.createElement("div");
    header.className = "graph-filter-header";

    const collapseBtn = document.createElement("button");
    collapseBtn.className = "graph-filter-collapse";
    collapseBtn.type = "button";
    collapseBtn.setAttribute("aria-expanded", "true");
    collapseBtn.setAttribute("aria-controls", "graph-filter-body");
    collapseBtn.title = "Toggle filter panel";

    // Simple chevron rendered as inline SVG for visual consistency with the rest of the UI.
    collapseBtn.innerHTML = `
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true" class="graph-filter-chevron">
            <path d="M2 3.5L5 6.5L8 3.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>`;

    const headerLabel = document.createElement("span");
    headerLabel.className = "graph-filter-label";
    headerLabel.textContent = "Filters";

    // Active-filter count badge — tells users at a glance how many filters are on.
    const badge = document.createElement("span");
    badge.className = "graph-filter-badge";
    badge.setAttribute("aria-live", "polite");
    badge.hidden = true;

    header.appendChild(collapseBtn);
    header.appendChild(headerLabel);
    header.appendChild(badge);

    // ── Filter body ───────────────────────────────────────────────────────────

    const body = document.createElement("div");
    body.className = "graph-filter-body";
    body.id = "graph-filter-body";

    // ── Toggle helpers ────────────────────────────────────────────────────────

    /**
     * Creates a labelled checkbox toggle.
     *
     * @param {{ id: string, label: string, checked: boolean, onChange: (checked: boolean) => void }} opts
     * @returns {HTMLElement}
     */
    function createToggle({ id, label, checked, onChange: onToggle }) {
        const wrapper = document.createElement("label");
        wrapper.className = "graph-filter-toggle";
        wrapper.htmlFor = id;

        const checkbox = document.createElement("input");
        checkbox.type = "checkbox";
        checkbox.id = id;
        checkbox.checked = checked;
        checkbox.addEventListener("change", () => {
            onToggle(checkbox.checked);
        });

        const text = document.createElement("span");
        text.textContent = label;

        wrapper.appendChild(checkbox);
        wrapper.appendChild(text);

        return wrapper;
    }

    // ── Build the three toggles ───────────────────────────────────────────────

    const remotesToggle = createToggle({
        id: "gf-hide-remotes",
        label: "Hide remote branches",
        checked: filterState.hideRemotes,
        onChange: (checked) => {
            filterState = { ...filterState, hideRemotes: checked };
            notifyChange();
        },
    });

    const mergesToggle = createToggle({
        id: "gf-hide-merges",
        label: "Hide merge commits",
        checked: filterState.hideMerges,
        onChange: (checked) => {
            filterState = { ...filterState, hideMerges: checked };
            notifyChange();
        },
    });

    const stashesToggle = createToggle({
        id: "gf-hide-stashes",
        label: "Hide stash entries",
        checked: filterState.hideStashes,
        onChange: (checked) => {
            filterState = { ...filterState, hideStashes: checked };
            notifyChange();
        },
    });

    // ── Branch-focus dropdown ─────────────────────────────────────────────────

    const dropdownWrapper = document.createElement("div");
    dropdownWrapper.className = "graph-filter-dropdown-wrapper";

    const dropdownLabel = document.createElement("label");
    dropdownLabel.htmlFor = "gf-focus-branch";
    dropdownLabel.className = "graph-filter-dropdown-label";
    dropdownLabel.textContent = "Focus branch";

    const dropdown = document.createElement("select");
    dropdown.id = "gf-focus-branch";
    dropdown.className = "graph-filter-dropdown";
    dropdown.setAttribute("aria-label", "Focus branch — show only reachable commits");

    // Sentinel "none" option.
    const noneOption = document.createElement("option");
    noneOption.value = "";
    noneOption.textContent = "— All branches —";
    dropdown.appendChild(noneOption);
    dropdown.value = filterState.focusBranch;

    dropdown.addEventListener("change", () => {
        filterState = { ...filterState, focusBranch: dropdown.value };
        notifyChange();
    });

    dropdownWrapper.appendChild(dropdownLabel);
    dropdownWrapper.appendChild(dropdown);

    // ── Assemble body ─────────────────────────────────────────────────────────

    body.appendChild(remotesToggle);
    body.appendChild(mergesToggle);
    body.appendChild(stashesToggle);
    body.appendChild(dropdownWrapper);

    // ── Collapse behaviour ────────────────────────────────────────────────────

    let isCollapsed = false;

    collapseBtn.addEventListener("click", () => {
        isCollapsed = !isCollapsed;
        body.classList.toggle("is-hidden", isCollapsed);
        collapseBtn.setAttribute("aria-expanded", String(!isCollapsed));
        collapseBtn.querySelector(".graph-filter-chevron")?.classList.toggle("is-collapsed", isCollapsed);
    });

    // ── Compose DOM ───────────────────────────────────────────────────────────

    el.appendChild(header);
    el.appendChild(body);

    // Prepend so the panel sits above the canvas inside the graph root.
    container.prepend(el);

    // ── Helpers ───────────────────────────────────────────────────────────────

    /**
     * Counts how many filters are currently active and updates the badge.
     */
    function updateBadge() {
        let count = 0;
        if (filterState.hideRemotes) count++;
        if (filterState.hideMerges) count++;
        if (filterState.hideStashes) count++;
        if (filterState.focusBranch) count++;
        if (count > 0) {
            badge.textContent = String(count);
            badge.hidden = false;
        } else {
            badge.hidden = true;
        }
    }

    /**
     * Persists current state, updates the badge, and fires the onChange callback.
     */
    function notifyChange() {
        saveFilterState(filterState);
        updateBadge();
        onChange(filterState);
    }

    // Initialize the badge to reflect any persisted state on load.
    updateBadge();

    // ── Public API ────────────────────────────────────────────────────────────

    /**
     * Repopulates the branch dropdown from the live branch map.
     * Preserves the current selection when the chosen branch still exists.
     *
     * @param {Map<string, string>} branches branch-name → commit-hash map.
     */
    function updateBranches(branches) {
        const previousValue = dropdown.value;

        // Remove all options except the sentinel "none" at index 0.
        while (dropdown.options.length > 1) {
            dropdown.remove(1);
        }

        // Sort branch names for stable display: local branches first, then remotes.
        const sorted = [...branches.keys()].sort((a, b) => {
            const aRemote = a.startsWith("refs/remotes/");
            const bRemote = b.startsWith("refs/remotes/");
            if (aRemote !== bRemote) return aRemote ? 1 : -1;
            return a.localeCompare(b);
        });

        for (const name of sorted) {
            const opt = document.createElement("option");
            opt.value = name;
            // Display a friendlier short form while keeping the full ref as the value.
            opt.textContent = friendlyBranchName(name);
            dropdown.appendChild(opt);
        }

        // Restore selection if it still exists; otherwise clear it.
        if (previousValue && branches.has(previousValue)) {
            dropdown.value = previousValue;
        } else if (previousValue) {
            // The previously focused branch no longer exists — clear focus silently.
            filterState = { ...filterState, focusBranch: "" };
            dropdown.value = "";
            saveFilterState(filterState);
            updateBadge();
            // Do not fire onChange here to avoid a spurious re-filter on startup.
        }
    }

    function destroy() {
        el.remove();
    }

    return {
        el,
        updateBranches,
        getState: () => ({ ...filterState }),
        destroy,
    };
}

// ── Utilities ─────────────────────────────────────────────────────────────────

/**
 * Converts a full ref name to a short, human-readable form.
 * Examples:
 *   "refs/heads/main"          → "main"
 *   "refs/remotes/origin/main" → "origin/main"
 *   "refs/tags/v1.0"           → "tags/v1.0"
 *
 * @param {string} name Full ref name.
 * @returns {string} Short display name.
 */
function friendlyBranchName(name) {
    if (name.startsWith("refs/heads/")) return name.slice("refs/heads/".length);
    if (name.startsWith("refs/remotes/")) return name.slice("refs/remotes/".length);
    if (name.startsWith("refs/")) return name.slice("refs/".length);
    return name;
}

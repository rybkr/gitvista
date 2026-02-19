/**
 * @fileoverview Graph filter popover component.
 *
 * Renders a toolbar trigger button with an active-filter badge. Clicking the
 * button opens a popover dropdown with filter toggles and a branch-focus
 * dropdown. Click-outside or Escape dismisses the popover.
 *
 * Filter state shape:
 *   { hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }
 *
 * focusBranch is the full branch name (e.g. "refs/heads/main") or "" for none.
 */

import { logger } from "./logger.js";

const STORAGE_KEY = "gitvista-filter-state";

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
 */
export function loadFilterState() {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return defaultFilterState();
        const parsed = JSON.parse(raw);
        return { ...defaultFilterState(), ...parsed };
    } catch {
        return defaultFilterState();
    }
}

function saveFilterState(state) {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch {
        logger.warn("graphFilters: unable to save filter state to localStorage");
    }
}

// ── SVG icons ─────────────────────────────────────────────────────────────────

const FILTER_ICON = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
    <path d="M1.5 2h13M3.5 6h9M5.5 10h5M7 14h2"/>
</svg>`;

/**
 * Creates the graph filter popover system.
 *
 * Returns an object with a trigger button element (to mount in a toolbar) and
 * methods to update branches and read state.
 *
 * @param {{
 *   initialState?: object,
 *   onChange: (state: object) => void,
 * }} options
 */
export function createGraphFilters(options) {
    const { onChange } = options;
    let filterState = { ...(options.initialState ?? loadFilterState()) };
    let isOpen = false;

    // ── Trigger button ────────────────────────────────────────────────────────

    const trigger = document.createElement("button");
    trigger.className = "graph-filter-trigger";
    trigger.type = "button";
    trigger.setAttribute("aria-label", "Graph filters");
    trigger.setAttribute("aria-expanded", "false");
    trigger.setAttribute("aria-haspopup", "true");

    const triggerIcon = document.createElement("span");
    triggerIcon.className = "graph-filter-trigger-icon";
    triggerIcon.innerHTML = FILTER_ICON;

    const triggerLabel = document.createElement("span");
    triggerLabel.className = "graph-filter-trigger-label";
    triggerLabel.textContent = "Filters";

    trigger.appendChild(triggerIcon);
    trigger.appendChild(triggerLabel);

    // ── Popover ───────────────────────────────────────────────────────────────

    const popover = document.createElement("div");
    popover.className = "graph-filter-popover";
    popover.setAttribute("role", "dialog");
    popover.setAttribute("aria-label", "Filter options");

    // Popover header
    const popoverHeader = document.createElement("div");
    popoverHeader.className = "graph-filter-popover-header";

    const popoverTitle = document.createElement("span");
    popoverTitle.className = "graph-filter-popover-title";
    popoverTitle.textContent = "Filters";

    const resetBtn = document.createElement("button");
    resetBtn.className = "graph-filter-reset";
    resetBtn.type = "button";
    resetBtn.textContent = "Reset";
    resetBtn.addEventListener("click", () => {
        filterState = defaultFilterState();
        // Sync checkbox states
        remotesCheckbox.checked = false;
        mergesCheckbox.checked = false;
        stashesCheckbox.checked = false;
        dropdown.value = "";
        notifyChange();
    });

    popoverHeader.appendChild(popoverTitle);
    popoverHeader.appendChild(resetBtn);

    // Popover body — filter controls
    const popoverBody = document.createElement("div");
    popoverBody.className = "graph-filter-popover-body";

    // Section: visibility filters
    const visSection = document.createElement("div");
    visSection.className = "graph-filter-section";

    const visSectionLabel = document.createElement("div");
    visSectionLabel.className = "graph-filter-section-label";
    visSectionLabel.textContent = "Visibility";
    visSection.appendChild(visSectionLabel);

    function createToggle({ id, label, checked, onChange: onToggle }) {
        const wrapper = document.createElement("label");
        wrapper.className = "graph-filter-toggle";
        wrapper.htmlFor = id;

        const checkbox = document.createElement("input");
        checkbox.type = "checkbox";
        checkbox.id = id;
        checkbox.checked = checked;
        checkbox.addEventListener("change", () => onToggle(checkbox.checked));

        const text = document.createElement("span");
        text.textContent = label;

        wrapper.appendChild(checkbox);
        wrapper.appendChild(text);

        return { el: wrapper, checkbox };
    }

    const remotes = createToggle({
        id: "gf-hide-remotes",
        label: "Hide remote branches",
        checked: filterState.hideRemotes,
        onChange: (checked) => {
            filterState = { ...filterState, hideRemotes: checked };
            notifyChange();
        },
    });
    const remotesCheckbox = remotes.checkbox;

    const merges = createToggle({
        id: "gf-hide-merges",
        label: "Hide merge commits",
        checked: filterState.hideMerges,
        onChange: (checked) => {
            filterState = { ...filterState, hideMerges: checked };
            notifyChange();
        },
    });
    const mergesCheckbox = merges.checkbox;

    const stashes = createToggle({
        id: "gf-hide-stashes",
        label: "Hide stash entries",
        checked: filterState.hideStashes,
        onChange: (checked) => {
            filterState = { ...filterState, hideStashes: checked };
            notifyChange();
        },
    });
    const stashesCheckbox = stashes.checkbox;

    visSection.appendChild(remotes.el);
    visSection.appendChild(merges.el);
    visSection.appendChild(stashes.el);

    // Section: branch focus
    const branchSection = document.createElement("div");
    branchSection.className = "graph-filter-section";

    const branchSectionLabel = document.createElement("div");
    branchSectionLabel.className = "graph-filter-section-label";
    branchSectionLabel.textContent = "Branch focus";

    const dropdown = document.createElement("select");
    dropdown.id = "gf-focus-branch";
    dropdown.className = "graph-filter-dropdown";
    dropdown.setAttribute("aria-label", "Focus branch — show only reachable commits");

    const noneOption = document.createElement("option");
    noneOption.value = "";
    noneOption.textContent = "All branches";
    dropdown.appendChild(noneOption);
    dropdown.value = filterState.focusBranch;

    dropdown.addEventListener("change", () => {
        filterState = { ...filterState, focusBranch: dropdown.value };
        notifyChange();
    });

    branchSection.appendChild(branchSectionLabel);
    branchSection.appendChild(dropdown);

    // Assemble popover
    popoverBody.appendChild(visSection);
    popoverBody.appendChild(branchSection);
    popover.appendChild(popoverHeader);
    popover.appendChild(popoverBody);

    // Mount popover as sibling of trigger (positioned absolutely)
    // We'll wrap both in a container for positioning context
    const wrapper = document.createElement("div");
    wrapper.className = "graph-filter-wrapper";
    wrapper.appendChild(trigger);
    wrapper.appendChild(popover);

    // ── Open / close ──────────────────────────────────────────────────────────

    function openPopover() {
        isOpen = true;
        popover.classList.add("is-open");
        trigger.classList.add("is-active");
        trigger.setAttribute("aria-expanded", "true");
        // Listen for outside clicks on next tick (so the current click doesn't close it)
        requestAnimationFrame(() => {
            document.addEventListener("pointerdown", onOutsideClick, true);
            document.addEventListener("keydown", onEscapeKey, true);
        });
    }

    function closePopover() {
        isOpen = false;
        popover.classList.remove("is-open");
        trigger.classList.remove("is-active");
        trigger.setAttribute("aria-expanded", "false");
        document.removeEventListener("pointerdown", onOutsideClick, true);
        document.removeEventListener("keydown", onEscapeKey, true);
    }

    function onOutsideClick(e) {
        if (!wrapper.contains(e.target)) {
            closePopover();
        }
    }

    function onEscapeKey(e) {
        if (e.key === "Escape") {
            e.stopPropagation();
            closePopover();
            trigger.focus();
        }
    }

    trigger.addEventListener("click", () => {
        if (isOpen) {
            closePopover();
        } else {
            openPopover();
        }
    });

    // ── State helpers ─────────────────────────────────────────────────────────

    function updateBadge() {
        const hasActive =
            filterState.hideRemotes ||
            filterState.hideMerges ||
            filterState.hideStashes ||
            !!filterState.focusBranch;
        trigger.classList.toggle("has-active-filters", hasActive);
        // Show/hide reset button
        resetBtn.style.display = hasActive ? "" : "none";
    }

    function notifyChange() {
        saveFilterState(filterState);
        updateBadge();
        onChange(filterState);
    }

    updateBadge();

    // ── Public API ────────────────────────────────────────────────────────────

    function updateBranches(branches) {
        const previousValue = dropdown.value;

        while (dropdown.options.length > 1) {
            dropdown.remove(1);
        }

        const sorted = [...branches.keys()].sort((a, b) => {
            const aRemote = a.startsWith("refs/remotes/");
            const bRemote = b.startsWith("refs/remotes/");
            if (aRemote !== bRemote) return aRemote ? 1 : -1;
            return a.localeCompare(b);
        });

        for (const name of sorted) {
            const opt = document.createElement("option");
            opt.value = name;
            opt.textContent = friendlyBranchName(name);
            dropdown.appendChild(opt);
        }

        if (previousValue && branches.has(previousValue)) {
            dropdown.value = previousValue;
        } else if (previousValue) {
            filterState = { ...filterState, focusBranch: "" };
            dropdown.value = "";
            saveFilterState(filterState);
            updateBadge();
        }
    }

    function destroy() {
        closePopover();
        wrapper.remove();
    }

    return {
        el: wrapper,
        updateBranches,
        getState: () => ({ ...filterState }),
        destroy,
    };
}

// ── Utilities ─────────────────────────────────────────────────────────────────

function friendlyBranchName(name) {
    if (name.startsWith("refs/heads/")) return name.slice("refs/heads/".length);
    if (name.startsWith("refs/remotes/")) return name.slice("refs/remotes/".length);
    if (name.startsWith("refs/")) return name.slice("refs/".length);
    return name;
}

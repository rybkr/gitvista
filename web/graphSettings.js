/**
 * @fileoverview Floating settings overlay with graph scope and physics controls.
 * Includes a gear trigger button for the toolbar and a frosted-glass panel.
 */

import { DEFAULT_PHYSICS, getDefaults } from "./graphSettingsDefaults.js";

/**
 * Creates the graph settings overlay and trigger button.
 *
 * @param {{
 *   initialSettings: { scope: object, physics: object },
 *   onChange: (settings: { scope?: object, physics?: object }) => void,
 *   getBranches?: () => Map<string, string>,
 *   getLayoutMode?: () => string,
 *   getCommitCount?: () => { total: number },
 * }} options
 * @returns {{
 *   triggerEl: HTMLElement,
 *   overlayEl: HTMLElement,
 *   toggle: () => void,
 *   show: () => void,
 *   hide: () => void,
 *   isVisible: () => boolean,
 *   updateBranches: () => void,
 *   destroy: () => void,
 * }}
 */
export function createGraphSettings(options) {
    const { initialSettings, onChange, getBranches, getLayoutMode, getCommitCount } = options;
    const defaults = getDefaults();
    let settings = {
        scope: { ...initialSettings.scope },
        physics: { ...initialSettings.physics },
    };
    let branchFilterQuery = "";

    // Debounce timer for physics slider changes (16ms = one frame)
    let debounceTimer = null;
    function emitChange(partial) {
        clearTimeout(debounceTimer);
        debounceTimer = setTimeout(() => {
            onChange(partial);
        }, 16);
    }

    // ── Trigger button ─────────────────────────────────────────────────
    const triggerEl = document.createElement("button");
    triggerEl.className = "graph-settings-trigger";
    triggerEl.title = "Graph settings (,)";
    triggerEl.innerHTML = "\u2699"; // ⚙
    triggerEl.type = "button";

    // ── Overlay panel ──────────────────────────────────────────────────
    const overlayEl = document.createElement("div");
    overlayEl.className = "graph-settings-overlay";

    const headerEl = document.createElement("div");
    headerEl.className = "graph-settings-overlay__header";

    const titleWrap = document.createElement("div");
    titleWrap.className = "graph-settings-overlay__title-wrap";

    const titleEl = document.createElement("div");
    titleEl.className = "graph-settings-overlay__title";
    titleEl.textContent = "Graph Controls";

    const modePill = document.createElement("span");
    modePill.className = "graph-settings-overlay__mode-pill";

    titleWrap.appendChild(titleEl);
    titleWrap.appendChild(modePill);

    const resetAllBtn = document.createElement("button");
    resetAllBtn.className = "graph-settings-overlay__action";
    resetAllBtn.type = "button";
    resetAllBtn.textContent = "Reset all";

    headerEl.appendChild(titleWrap);
    headerEl.appendChild(resetAllBtn);
    overlayEl.appendChild(headerEl);

    // ── Scope section ──────────────────────────────────────────────────
    const scopeSection = createSection("Graph Scope");

    function getDepthBounds() {
        const total = Math.max(0, getCommitCount?.()?.total ?? 0);
        if (total <= 0) return { min: 10, max: 10 };
        if (total <= 10) return { min: 1, max: total };
        return { min: 10, max: total };
    }

    function getDepthSliderValue() {
        const { min, max } = getDepthBounds();
        if (settings.scope.depthLimit === Infinity) return max;
        return Math.max(min, Math.min(max, settings.scope.depthLimit));
    }

    // Depth limit slider
    const depthRow = createSliderRow({
        label: "Depth",
        min: getDepthBounds().min,
        max: getDepthBounds().max,
        step: 10,
        value: getDepthSliderValue(),
        formatValue: (v) => {
            const { max } = getDepthBounds();
            return v >= max ? "All" : String(v);
        },
        onInput: (v) => {
            const { max } = getDepthBounds();
            settings.scope.depthLimit = v >= max ? Infinity : v;
            emitChange({ scope: settings.scope });
        },
        onReset: () => {
            settings.scope.depthLimit = Infinity;
            emitChange({ scope: settings.scope });
        },
        defaultValue: getDepthBounds().max,
    });

    // Time window select
    const timeRow = document.createElement("div");
    timeRow.className = "graph-settings-overlay__row";
    const timeLabel = document.createElement("span");
    timeLabel.className = "graph-settings-overlay__label";
    timeLabel.textContent = "Time";
    const timeSelect = document.createElement("select");
    const timeOptions = [
        ["all", "All time"],
        ["7d", "Last 7 days"],
        ["30d", "Last 30 days"],
        ["90d", "Last 90 days"],
        ["1y", "Last year"],
    ];
    for (const [val, label] of timeOptions) {
        const opt = document.createElement("option");
        opt.value = val;
        opt.textContent = label;
        if (val === settings.scope.timeWindow) opt.selected = true;
        timeSelect.appendChild(opt);
    }
    timeSelect.addEventListener("change", () => {
        settings.scope.timeWindow = timeSelect.value;
        emitChange({ scope: settings.scope });
    });
    timeRow.appendChild(timeLabel);
    timeRow.appendChild(timeSelect);

    scopeSection.body.appendChild(depthRow.el);
    scopeSection.body.appendChild(timeRow);

    // Branch include/exclude checkboxes
    const branchHeader = document.createElement("div");
    branchHeader.className = "graph-settings-overlay__row";
    const branchLabel = document.createElement("span");
    branchLabel.className = "graph-settings-overlay__label";
    branchLabel.textContent = "Branches";
    branchHeader.appendChild(branchLabel);

    const branchList = document.createElement("div");
    branchList.className = "graph-settings-overlay__branch-list";

    const branchTools = document.createElement("div");
    branchTools.className = "graph-settings-overlay__branch-tools";
    const branchSearch = document.createElement("input");
    branchSearch.className = "graph-settings-overlay__branch-search";
    branchSearch.type = "search";
    branchSearch.placeholder = "Filter branches";
    branchSearch.setAttribute("aria-label", "Filter branches");

    const branchActions = document.createElement("div");
    branchActions.className = "graph-settings-overlay__branch-actions";
    const branchAllBtn = createActionButton("All");
    const branchLocalBtn = createActionButton("Local");
    const branchRemoteBtn = createActionButton("Remote");
    branchActions.appendChild(branchAllBtn);
    branchActions.appendChild(branchLocalBtn);
    branchActions.appendChild(branchRemoteBtn);

    branchTools.appendChild(branchSearch);
    branchTools.appendChild(branchActions);

    scopeSection.body.appendChild(branchHeader);
    scopeSection.body.appendChild(branchTools);
    scopeSection.body.appendChild(branchList);

    overlayEl.appendChild(scopeSection.el);

    // ── Physics section ────────────────────────────────────────────────
    const physicsSection = createSection("Physics (Force Mode)");

    const chargeRow = createSliderRow({
        label: "Charge",
        min: -300,
        max: -10,
        step: 5,
        value: settings.physics.chargeStrength,
        formatValue: (v) => String(v),
        onInput: (v) => {
            settings.physics.chargeStrength = v;
            emitChange({ physics: settings.physics });
        },
        onReset: () => {
            settings.physics.chargeStrength = DEFAULT_PHYSICS.chargeStrength;
            emitChange({ physics: settings.physics });
        },
        defaultValue: DEFAULT_PHYSICS.chargeStrength,
    });

    const linkRow = createSliderRow({
        label: "Link dist",
        min: 20,
        max: 200,
        step: 5,
        value: settings.physics.linkDistance,
        formatValue: (v) => String(v),
        onInput: (v) => {
            settings.physics.linkDistance = v;
            emitChange({ physics: settings.physics });
        },
        onReset: () => {
            settings.physics.linkDistance = DEFAULT_PHYSICS.linkDistance;
            emitChange({ physics: settings.physics });
        },
        defaultValue: DEFAULT_PHYSICS.linkDistance,
    });

    const collisionRow = createSliderRow({
        label: "Collision",
        min: 5,
        max: 50,
        step: 1,
        value: settings.physics.collisionRadius,
        formatValue: (v) => String(v),
        onInput: (v) => {
            settings.physics.collisionRadius = v;
            emitChange({ physics: settings.physics });
        },
        onReset: () => {
            settings.physics.collisionRadius = DEFAULT_PHYSICS.collisionRadius;
            emitChange({ physics: settings.physics });
        },
        defaultValue: DEFAULT_PHYSICS.collisionRadius,
    });

    const decayRow = createSliderRow({
        label: "Decay",
        min: 0.1,
        max: 0.9,
        step: 0.05,
        value: settings.physics.velocityDecay,
        formatValue: (v) => v.toFixed(2),
        onInput: (v) => {
            settings.physics.velocityDecay = v;
            emitChange({ physics: settings.physics });
        },
        onReset: () => {
            settings.physics.velocityDecay = DEFAULT_PHYSICS.velocityDecay;
            emitChange({ physics: settings.physics });
        },
        defaultValue: DEFAULT_PHYSICS.velocityDecay,
    });

    physicsSection.body.appendChild(chargeRow.el);
    physicsSection.body.appendChild(linkRow.el);
    physicsSection.body.appendChild(collisionRow.el);
    physicsSection.body.appendChild(decayRow.el);

    overlayEl.appendChild(physicsSection.el);

    // ── Visibility ─────────────────────────────────────────────────────
    function toggle() {
        overlayEl.classList.toggle("is-visible");
        triggerEl.classList.toggle("is-active", overlayEl.classList.contains("is-visible"));
        if (overlayEl.classList.contains("is-visible")) {
            syncModePill();
            updatePhysicsVisibility();
            updateBranches();
            positionOverlay();
        }
    }

    function show() {
        overlayEl.classList.add("is-visible");
        triggerEl.classList.add("is-active");
        syncModePill();
        updatePhysicsVisibility();
        updateBranches();
        positionOverlay();
    }

    function hide() {
        overlayEl.classList.remove("is-visible");
        triggerEl.classList.remove("is-active");
    }

    function isVisible() {
        return overlayEl.classList.contains("is-visible");
    }

    function updatePhysicsVisibility() {
        const mode = getLayoutMode?.() ?? "force";
        physicsSection.el.style.display = mode === "force" ? "" : "none";
    }

    function updateDepthControl() {
        const { min, max } = getDepthBounds();
        depthRow.slider.min = String(min);
        depthRow.slider.max = String(max);
        depthRow.slider.step = String(totalStep(min, max));
        const displayValue = getDepthSliderValue();
        depthRow.setValue(displayValue);
        depthRow.setDefaultValue(max);
    }

    function syncModePill() {
        const mode = getLayoutMode?.() ?? "force";
        modePill.textContent = mode === "force" ? "Force mode" : "Lane mode";
    }

    function isBranchVisible(name) {
        if (name === "HEAD") return false;
        if (name === "refs/stash" || name.startsWith("stash@{")) return false;
        return name.startsWith("refs/heads/") || name.startsWith("refs/remotes/");
    }

    function getBranchGroupKey(name) {
        if (name.startsWith("refs/heads/")) {
            return name.replace(/^refs\/heads\//, "");
        }
        if (name.startsWith("refs/remotes/")) {
            return name.replace(/^refs\/remotes\/[^/]+\//, "");
        }
        return name;
    }

    function getBranchGroups() {
        if (!getBranches) return [];
        const groups = new Map();
        for (const name of getBranches().keys()) {
            if (!isBranchVisible(name)) continue;
            const key = getBranchGroupKey(name);
            if (!groups.has(key)) {
                groups.set(key, {
                    key,
                    label: key,
                    refs: [],
                    hasLocal: false,
                    hasRemote: false,
                });
            }
            const group = groups.get(key);
            group.refs.push(name);
            if (name.startsWith("refs/heads/")) group.hasLocal = true;
            if (name.startsWith("refs/remotes/")) group.hasRemote = true;
        }
        return [...groups.values()].sort((a, b) => a.label.localeCompare(b.label));
    }

    function applyBranchPreset(preset) {
        const groups = getBranchGroups();
        const rules = { ...settings.scope.branchRules };
        for (const group of groups) {
            const shouldEnable =
                preset === "all" ||
                (preset === "local" && group.hasLocal) ||
                (preset === "remote" && group.hasRemote);
            for (const ref of group.refs) {
                if (shouldEnable) {
                    delete rules[ref];
                } else {
                    rules[ref] = false;
                }
            }
        }
        settings.scope.branchRules = rules;
        emitChange({ scope: settings.scope });
        updateBranches();
    }

    function updateBranches() {
        if (!getBranches) return;
        branchList.innerHTML = "";
        const filteredGroups = getBranchGroups().filter((group) => {
            if (!branchFilterQuery) return true;
            return group.label.toLowerCase().includes(branchFilterQuery);
        });
        if (filteredGroups.length === 0) {
            const empty = document.createElement("div");
            empty.className = "graph-settings-overlay__branch-empty";
            empty.textContent = "No branches match this filter.";
            branchList.appendChild(empty);
            return;
        }
        for (const group of filteredGroups) {
            const item = document.createElement("label");
            item.className = "graph-settings-overlay__branch-item";
            const cb = document.createElement("input");
            cb.type = "checkbox";
            cb.checked = group.refs.every((ref) => settings.scope.branchRules[ref] !== false);
            cb.addEventListener("change", () => {
                for (const ref of group.refs) {
                    if (cb.checked) {
                        delete settings.scope.branchRules[ref];
                    } else {
                        settings.scope.branchRules[ref] = false;
                    }
                }
                emitChange({ scope: settings.scope });
            });
            const span = document.createElement("span");
            span.textContent = group.label;
            const meta = document.createElement("span");
            meta.className = "graph-settings-overlay__branch-meta";
            if (group.hasLocal && group.hasRemote) {
                meta.textContent = "local + remote";
            } else if (group.hasLocal) {
                meta.textContent = "local";
            } else {
                meta.textContent = "remote";
            }
            item.appendChild(cb);
            item.appendChild(span);
            item.appendChild(meta);
            branchList.appendChild(item);
        }
    }

    function resetAllSettings() {
        settings = {
            scope: {
                ...defaults.scope,
                branchRules: {},
            },
            physics: { ...defaults.physics },
        };
        branchFilterQuery = "";
        branchSearch.value = "";
        updateDepthControl();
        updateBranches();
        emitChange({ scope: settings.scope, physics: settings.physics });
    }

    function positionOverlay() {
        if (!isVisible()) return;
        const host = overlayEl.offsetParent || overlayEl.parentElement;
        if (!host) return;
        overlayEl.style.left = "0px";
        overlayEl.style.right = "auto";
        overlayEl.style.top = "0px";
        overlayEl.style.bottom = "auto";
        overlayEl.classList.remove("is-left-aligned", "is-bottom-aligned");

        const hostRect = host.getBoundingClientRect();
        const triggerRect = triggerEl.getBoundingClientRect();
        const overlayRect = overlayEl.getBoundingClientRect();
        const margin = 8;

        let left = triggerRect.right - hostRect.left - overlayRect.width;
        let top = triggerRect.bottom - hostRect.top + margin;

        if (left < margin) {
            left = margin;
            overlayEl.classList.add("is-left-aligned");
        }
        if (left + overlayRect.width > hostRect.width - margin) {
            left = Math.max(margin, hostRect.width - overlayRect.width - margin);
        }

        if (top + overlayRect.height > hostRect.height - margin) {
            const aboveTop = triggerRect.top - hostRect.top - overlayRect.height - margin;
            if (aboveTop >= margin) {
                top = aboveTop;
                overlayEl.classList.add("is-bottom-aligned");
            } else {
                top = Math.max(margin, hostRect.height - overlayRect.height - margin);
            }
        }

        overlayEl.style.left = `${Math.round(left)}px`;
        overlayEl.style.top = `${Math.round(top)}px`;
    }

    function refresh() {
        updateDepthControl();
        if (isVisible()) {
            syncModePill();
            updatePhysicsVisibility();
            updateBranches();
            positionOverlay();
        }
    }

    // Wire trigger
    triggerEl.addEventListener("click", (e) => {
        e.stopPropagation();
        toggle();
    });

    resetAllBtn.addEventListener("click", () => {
        resetAllSettings();
    });
    branchSearch.addEventListener("input", () => {
        branchFilterQuery = branchSearch.value.trim().toLowerCase();
        updateBranches();
    });
    branchAllBtn.addEventListener("click", () => applyBranchPreset("all"));
    branchLocalBtn.addEventListener("click", () => applyBranchPreset("local"));
    branchRemoteBtn.addEventListener("click", () => applyBranchPreset("remote"));

    // Close on outside click
    function handleDocClick(e) {
        if (!isVisible()) return;
        if (overlayEl.contains(e.target) || triggerEl.contains(e.target)) return;
        hide();
    }
    document.addEventListener("click", handleDocClick);
    window.addEventListener("resize", positionOverlay);

    function destroy() {
        clearTimeout(debounceTimer);
        document.removeEventListener("click", handleDocClick);
        window.removeEventListener("resize", positionOverlay);
        triggerEl.remove();
        overlayEl.remove();
    }

    return {
        triggerEl,
        overlayEl,
        toggle,
        show,
        hide,
        isVisible,
        updateBranches,
        refresh,
        destroy,
    };
}

function totalStep(min, max) {
    if (max <= min) return 1;
    return min >= 10 ? 10 : 1;
}

function createActionButton(label) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "graph-settings-overlay__action graph-settings-overlay__action--subtle";
    button.textContent = label;
    return button;
}

// ── Helpers ──────────────────────────────────────────────────────────

function createSection(title) {
    const el = document.createElement("div");
    el.className = "graph-settings-overlay__section";
    const header = document.createElement("div");
    header.className = "graph-settings-overlay__section-header";
    header.textContent = title;
    const body = document.createElement("div");
    el.appendChild(header);
    el.appendChild(body);
    return { el, body };
}

function createSliderRow({ label, min, max, step, value, formatValue, onInput, onReset, defaultValue }) {
    const el = document.createElement("div");
    el.className = "graph-settings-overlay__row";

    const labelEl = document.createElement("span");
    labelEl.className = "graph-settings-overlay__label";
    labelEl.textContent = label;

    const slider = document.createElement("input");
    slider.type = "range";
    slider.min = String(min);
    slider.max = String(max);
    slider.step = String(step);
    slider.value = String(value);

    const valueEl = document.createElement("span");
    valueEl.className = "graph-settings-overlay__value";
    valueEl.textContent = formatValue(value);

    const resetBtn = document.createElement("button");
    resetBtn.className = "graph-settings-overlay__reset";
    resetBtn.title = "Reset to default";
    resetBtn.textContent = "\u21BA"; // ↺

    slider.addEventListener("input", () => {
        const v = parseFloat(slider.value);
        valueEl.textContent = formatValue(v);
        onInput(v);
    });

    let currentDefaultValue = defaultValue;

    resetBtn.addEventListener("click", () => {
        slider.value = String(currentDefaultValue);
        valueEl.textContent = formatValue(currentDefaultValue);
        onReset();
    });

    el.appendChild(labelEl);
    el.appendChild(slider);
    el.appendChild(valueEl);
    el.appendChild(resetBtn);

    return {
        el,
        slider,
        setValue: (v) => { slider.value = v; valueEl.textContent = formatValue(v); },
        setDefaultValue: (v) => { currentDefaultValue = v; },
    };
}

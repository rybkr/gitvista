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
    const { initialSettings, onChange, getBranches, getLayoutMode } = options;
    let settings = {
        scope: { ...initialSettings.scope },
        physics: { ...initialSettings.physics },
    };

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

    // ── Scope section ──────────────────────────────────────────────────
    const scopeSection = createSection("Graph Scope");

    // Depth limit slider
    const depthRow = createSliderRow({
        label: "Depth",
        min: 10,
        max: 1000,
        step: 10,
        value: settings.scope.depthLimit === Infinity ? 1000 : settings.scope.depthLimit,
        formatValue: (v) => v >= 1000 ? "All" : String(v),
        onInput: (v) => {
            settings.scope.depthLimit = v >= 1000 ? Infinity : v;
            emitChange({ scope: settings.scope });
        },
        onReset: () => {
            settings.scope.depthLimit = Infinity;
            emitChange({ scope: settings.scope });
        },
        defaultValue: 1000,
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

    scopeSection.body.appendChild(branchHeader);
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
            updatePhysicsVisibility();
            updateBranches();
        }
    }

    function show() {
        overlayEl.classList.add("is-visible");
        triggerEl.classList.add("is-active");
        updatePhysicsVisibility();
        updateBranches();
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

    function updateBranches() {
        if (!getBranches) return;
        const branches = getBranches();
        branchList.innerHTML = "";
        const sortedNames = [...branches.keys()].sort();
        for (const name of sortedNames) {
            const displayName = name.replace(/^refs\/heads\//, "");
            const item = document.createElement("label");
            item.className = "graph-settings-overlay__branch-item";
            const cb = document.createElement("input");
            cb.type = "checkbox";
            cb.checked = settings.scope.branchRules[name] !== false;
            cb.addEventListener("change", () => {
                if (cb.checked) {
                    delete settings.scope.branchRules[name];
                } else {
                    settings.scope.branchRules[name] = false;
                }
                emitChange({ scope: settings.scope });
            });
            const span = document.createElement("span");
            span.textContent = displayName;
            item.appendChild(cb);
            item.appendChild(span);
            branchList.appendChild(item);
        }
    }

    // Wire trigger
    triggerEl.addEventListener("click", (e) => {
        e.stopPropagation();
        toggle();
    });

    // Close on outside click
    function handleDocClick(e) {
        if (!isVisible()) return;
        if (overlayEl.contains(e.target) || triggerEl.contains(e.target)) return;
        hide();
    }
    document.addEventListener("click", handleDocClick);

    function destroy() {
        clearTimeout(debounceTimer);
        document.removeEventListener("click", handleDocClick);
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
        destroy,
    };
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

    resetBtn.addEventListener("click", () => {
        slider.value = String(defaultValue);
        valueEl.textContent = formatValue(defaultValue);
        onReset();
    });

    el.appendChild(labelEl);
    el.appendChild(slider);
    el.appendChild(valueEl);
    el.appendChild(resetBtn);

    return { el, slider, setValue: (v) => { slider.value = v; valueEl.textContent = formatValue(v); } };
}

import { apiUrl } from "./apiBase.js";
import { apiFetch } from "./apiFetch.js";

const CONFLICT_CONFIG = {
    none: { badge: "OK", className: "merge-status--clean" },
    conflicting: { badge: "!!", className: "merge-status--conflict" },
    both_added: { badge: "AA", className: "merge-status--conflict" },
    delete_modify: { badge: "DM", className: "merge-status--delete-modify" },
};

const SWAP_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
    <path d="M4 6l-3 3 3 3M12 4l3 3-3 3M1 9h14M15 7H1"/>
</svg>`;

const COMPARE_EMPTY_SVG = `<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" opacity="0.3" stroke-width="1.5">
    <circle cx="8" cy="12" r="5"/>
    <circle cx="16" cy="12" r="5"/>
</svg>`;

export function createMergePreviewView({ getBranches, onPreviewResult }) {
    const el = document.createElement("div");
    el.className = "merge-preview-view";

    const state = {
        oursBranch: "",
        theirsBranch: "",
        entries: [],
        stats: null,
        loading: false,
        error: null,
        generation: 0,
    };

    function render() {
        el.innerHTML = "";

        // Branch selectors
        const selectorRow = document.createElement("div");
        selectorRow.className = "merge-preview-branch-selectors";

        const oursSelect = createBranchSelect("ours", state.oursBranch, (val) => {
            state.oursBranch = val;
            fetchPreview();
        });

        const swapBtn = document.createElement("button");
        swapBtn.className = "merge-preview-swap-btn";
        swapBtn.innerHTML = SWAP_SVG;
        swapBtn.title = "Swap branches";
        swapBtn.addEventListener("click", () => {
            const tmp = state.oursBranch;
            state.oursBranch = state.theirsBranch;
            state.theirsBranch = tmp;
            render();
            fetchPreview();
        });

        const theirsSelect = createBranchSelect("theirs", state.theirsBranch, (val) => {
            state.theirsBranch = val;
            fetchPreview();
        });

        selectorRow.appendChild(oursSelect);
        selectorRow.appendChild(swapBtn);
        selectorRow.appendChild(theirsSelect);
        el.appendChild(selectorRow);

        // Same branch check
        if (state.oursBranch && state.theirsBranch && state.oursBranch === state.theirsBranch) {
            const msg = document.createElement("div");
            msg.className = "merge-preview-message";
            msg.textContent = "Same branch selected — choose two different branches to compare.";
            el.appendChild(msg);
            return;
        }

        // Empty state
        if (!state.oursBranch || !state.theirsBranch) {
            const empty = document.createElement("div");
            empty.className = "merge-preview-empty";
            empty.innerHTML = COMPARE_EMPTY_SVG;
            const hint = document.createElement("p");
            hint.textContent = "Select two branches to preview a merge.";
            empty.appendChild(hint);
            el.appendChild(empty);
            return;
        }

        // Loading
        if (state.loading) {
            const loader = document.createElement("div");
            loader.className = "merge-preview-loading";
            loader.textContent = "Computing merge preview...";
            el.appendChild(loader);
            return;
        }

        // Error
        if (state.error) {
            const errEl = document.createElement("div");
            errEl.className = "merge-preview-error";
            errEl.textContent = state.error;
            el.appendChild(errEl);
            return;
        }

        // Stats bar
        if (state.stats) {
            const statsBar = document.createElement("div");
            statsBar.className = "merge-preview-stats";

            const total = document.createElement("span");
            total.className = "merge-preview-stat";
            total.textContent = `${state.stats.totalFiles} file${state.stats.totalFiles !== 1 ? "s" : ""} changed`;
            statsBar.appendChild(total);

            if (state.stats.conflicts > 0) {
                const conf = document.createElement("span");
                conf.className = "merge-preview-stat merge-preview-stat--conflict";
                conf.textContent = `${state.stats.conflicts} conflict${state.stats.conflicts !== 1 ? "s" : ""}`;
                statsBar.appendChild(conf);
            }

            if (state.stats.cleanMerge > 0) {
                const clean = document.createElement("span");
                clean.className = "merge-preview-stat merge-preview-stat--clean";
                clean.textContent = `${state.stats.cleanMerge} clean`;
                statsBar.appendChild(clean);
            }

            el.appendChild(statsBar);
        }

        // File list
        if (state.entries.length === 0 && state.stats) {
            const msg = document.createElement("div");
            msg.className = "merge-preview-message";
            msg.textContent = "Branches are already in sync — no changes to merge.";
            el.appendChild(msg);
            return;
        }

        const list = document.createElement("div");
        list.className = "merge-preview-file-list";

        // Sort: conflicts first, then alphabetical.
        const sorted = [...state.entries].sort((a, b) => {
            const aPriority = a.conflictType === "none" ? 1 : 0;
            const bPriority = b.conflictType === "none" ? 1 : 0;
            if (aPriority !== bPriority) return aPriority - bPriority;
            return a.path.localeCompare(b.path);
        });

        for (const entry of sorted) {
            const item = document.createElement("div");
            item.className = "merge-preview-file-item";

            const config = CONFLICT_CONFIG[entry.conflictType] || CONFLICT_CONFIG.none;
            const badge = document.createElement("span");
            badge.className = `merge-preview-badge ${config.className}`;
            badge.textContent = config.badge;
            item.appendChild(badge);

            const pathEl = document.createElement("span");
            pathEl.className = "merge-preview-file-path";
            const lastSlash = entry.path.lastIndexOf("/");
            if (lastSlash !== -1) {
                const dir = document.createElement("span");
                dir.className = "merge-preview-file-dir";
                dir.textContent = entry.path.slice(0, lastSlash + 1);
                pathEl.appendChild(dir);

                const base = document.createElement("span");
                base.textContent = entry.path.slice(lastSlash + 1);
                pathEl.appendChild(base);
            } else {
                pathEl.textContent = entry.path;
            }
            item.appendChild(pathEl);

            // Side status indicators
            const sides = document.createElement("span");
            sides.className = "merge-preview-sides";
            if (entry.oursStatus) {
                const ours = document.createElement("span");
                ours.className = "merge-preview-side-indicator";
                ours.textContent = entry.oursStatus.charAt(0).toUpperCase();
                ours.title = `Ours: ${entry.oursStatus}`;
                sides.appendChild(ours);
            }
            if (entry.theirsStatus) {
                const theirs = document.createElement("span");
                theirs.className = "merge-preview-side-indicator";
                theirs.textContent = entry.theirsStatus.charAt(0).toUpperCase();
                theirs.title = `Theirs: ${entry.theirsStatus}`;
                sides.appendChild(theirs);
            }
            item.appendChild(sides);

            list.appendChild(item);
        }

        el.appendChild(list);
    }

    function createBranchSelect(label, currentValue, onChange) {
        const wrapper = document.createElement("div");
        wrapper.className = "merge-preview-select-wrapper";

        const labelEl = document.createElement("label");
        labelEl.className = "merge-preview-select-label";
        labelEl.textContent = label;
        wrapper.appendChild(labelEl);

        const select = document.createElement("select");
        select.className = "merge-preview-select";

        const defaultOpt = document.createElement("option");
        defaultOpt.value = "";
        defaultOpt.textContent = `Select ${label}...`;
        select.appendChild(defaultOpt);

        const branches = getBranches();
        const branchNames = [...branches.keys()].sort();
        for (const name of branchNames) {
            const opt = document.createElement("option");
            opt.value = name;
            opt.textContent = name;
            if (name === currentValue) opt.selected = true;
            select.appendChild(opt);
        }

        select.addEventListener("change", () => onChange(select.value));
        wrapper.appendChild(select);
        return wrapper;
    }

    async function fetchPreview() {
        if (!state.oursBranch || !state.theirsBranch || state.oursBranch === state.theirsBranch) {
            state.entries = [];
            state.stats = null;
            state.error = null;
            state.loading = false;
            onPreviewResult(null);
            render();
            return;
        }

        const gen = ++state.generation;
        state.loading = true;
        state.error = null;
        render();

        try {
            const url = apiUrl(`/merge-preview?ours=${encodeURIComponent(state.oursBranch)}&theirs=${encodeURIComponent(state.theirsBranch)}`);
            const resp = await apiFetch(url);
            if (gen !== state.generation) return;

            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(text || `HTTP ${resp.status}`);
            }

            const data = await resp.json();
            if (gen !== state.generation) return;

            state.entries = data.entries || [];
            state.stats = data.stats || null;
            state.loading = false;
            state.error = null;

            onPreviewResult({
                oursHash: data.oursHash,
                theirsHash: data.theirsHash,
                mergeBaseHash: data.mergeBaseHash,
            });
        } catch (err) {
            if (gen !== state.generation) return;
            state.loading = false;
            state.error = err.message || "Failed to compute merge preview";
            onPreviewResult(null);
        }

        render();
    }

    function updateBranches() {
        const branches = getBranches();
        if (state.oursBranch && !branches.has(state.oursBranch)) {
            state.oursBranch = "";
            state.entries = [];
            state.stats = null;
            onPreviewResult(null);
        }
        if (state.theirsBranch && !branches.has(state.theirsBranch)) {
            state.theirsBranch = "";
            state.entries = [];
            state.stats = null;
            onPreviewResult(null);
        }
        render();
    }

    function getSelectedBranches() {
        return { ours: state.oursBranch, theirs: state.theirsBranch };
    }

    function refresh() {
        if (state.oursBranch && state.theirsBranch) {
            fetchPreview();
        }
    }

    function close() {
        state.oursBranch = "";
        state.theirsBranch = "";
        state.entries = [];
        state.stats = null;
        state.error = null;
        onPreviewResult(null);
        render();
    }

    // Don't render eagerly — getBranches() may reference state that isn't
    // initialized yet.  The first call to updateBranches() (from onDelta)
    // will trigger the initial render once the graph is ready.

    return { el, getSelectedBranches, updateBranches, refresh, close };
}

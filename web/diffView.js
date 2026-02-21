// SVG icons
const FILE_ICON_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 2C3.44772 2 3 2.44772 3 3V13C3 13.5523 3.44772 14 4 14H12C12.5523 14 13 13.5523 13 13V6L9 2H4Z" fill="currentColor" opacity="0.2"/>
    <path d="M9 2V6H13M4 2H9L13 6V13C13 13.5523 12.5523 14 12 14H4C3.44772 14 3 13.5523 3 13V3C3 2.44772 3.44772 2 4 2Z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const EMPTY_STATE_SVG = `<svg width="48" height="48" viewBox="0 0 24 24" fill="none">
    <path d="M9 3H5C3.89543 3 3 3.89543 3 5V19C3 20.1046 3.89543 21 5 21H19C20.1046 21 21 20.1046 21 19V9M9 3L21 9M9 3V9H21" stroke="currentColor" opacity="0.2" stroke-width="1.5"/>
    <path d="M12 11v6m-3-3h6" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const ALERT_SVG = `<svg width="16" height="16" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="8" r="6.5" stroke="currentColor" stroke-width="1.2"/>
    <path d="M8 4v5M8 11v1" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const STATUS_CONFIG = {
    added: { badge: "A", className: "diff-status--added" },
    modified: { badge: "M", className: "diff-status--modified" },
    deleted: { badge: "D", className: "diff-status--deleted" },
    renamed: { badge: "R", className: "diff-status--renamed" }, // Future enhancement
};

import { apiUrl } from "./apiBase.js";

export function createDiffView(backend, diffContentViewer) {
    const el = document.createElement("div");
    el.className = "diff-view";
    el.style.display = "none";

    const state = {
        commitHash: null,       // Currently displayed commit
        parentHash: null,       // Parent commit hash (null for root commits)
        entries: [],            // DiffEntry[] from API
        stats: null,            // { added, modified, deleted, total }
        loading: false,         // True while fetching commit diff list
        selectedFile: null,     // Currently selected file path
        showFileContent: false, // True when content viewer replaces the file list
        filterText: "",         // Filter input text (optional for v1, included for completeness)
        generation: 0,          // Stale response protection counter
        commitMessage: null,    // First line of commit message for header
    };

    async function fetchCommitDiff(commitHash) {
        const response = await fetch(apiUrl(`/commit/diff/${commitHash}`));
        if (!response.ok) {
            throw new Error(`Failed to fetch commit diff: ${response.status} ${response.statusText}`);
        }
        return response.json();
    }

    async function fetchFileDiff(commitHash, filePath) {
        const url = apiUrl(`/commit/diff/${commitHash}/file?path=${encodeURIComponent(filePath)}`);
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`Failed to fetch file diff: ${response.status} ${response.statusText}`);
        }
        return response.json();
    }

    function render() {
        el.innerHTML = "";

        const header = document.createElement("div");
        header.className = "diff-view-header";

        const commitInfo = document.createElement("div");
        commitInfo.className = "diff-view-commit-info";
        const shortHash = state.commitHash ? state.commitHash.substring(0, 7) : "";
        const message = state.commitMessage || "";
        const firstLine = message.split("\n")[0];
        commitInfo.textContent = `Commit: ${shortHash}${firstLine ? ` - "${firstLine}"` : ""}`;
        header.appendChild(commitInfo);

        el.appendChild(header);

        if (state.showFileContent) {
            if (diffContentViewer && diffContentViewer.el) {
                el.appendChild(diffContentViewer.el);
            }
            return;
        }

        if (!state.loading && state.stats) {
            const statsBar = renderStatsBar();
            el.appendChild(statsBar);
        }

        if (state.loading) {
            el.appendChild(renderLoadingState());
        } else if (state.entries === null) {
            el.appendChild(renderErrorState());
        } else if (state.entries.length === 0) {
            el.appendChild(renderEmptyState());
        } else {
            el.appendChild(renderFileList());
        }
        if (diffContentViewer && diffContentViewer.el) {
            el.appendChild(diffContentViewer.el);
        }
    }

    function renderStatsBar() {
        const bar = document.createElement("div");
        bar.className = "diff-stats-bar";

        if (state.stats.added > 0) {
            const addedSpan = document.createElement("span");
            addedSpan.className = "diff-stat diff-stat--added";
            addedSpan.textContent = `+${state.stats.added} added`;
            bar.appendChild(addedSpan);
        }

        if (state.stats.modified > 0) {
            const modifiedSpan = document.createElement("span");
            modifiedSpan.className = "diff-stat diff-stat--modified";
            modifiedSpan.textContent = `~${state.stats.modified} modified`;
            bar.appendChild(modifiedSpan);
        }

        if (state.stats.deleted > 0) {
            const deletedSpan = document.createElement("span");
            deletedSpan.className = "diff-stat diff-stat--deleted";
            deletedSpan.textContent = `-${state.stats.deleted} deleted`;
            bar.appendChild(deletedSpan);
        }

        const totalSpan = document.createElement("span");
        totalSpan.className = "diff-stat-total";
        totalSpan.textContent = `${state.stats.filesChanged} file${state.stats.filesChanged === 1 ? "" : "s"} changed`;
        bar.appendChild(totalSpan);

        return bar;
    }

    function renderFileList() {
        const container = document.createElement("div");
        container.className = "diff-file-list-container";

        const heading = document.createElement("h3");
        heading.className = "diff-file-list-heading";
        heading.textContent = "Changed Files";
        container.appendChild(heading);

        const list = document.createElement("div");
        list.className = "diff-file-list";
        list.setAttribute("role", "list");

        for (const entry of state.entries) {
            const item = document.createElement("div");
            item.className = "diff-file-item";
            item.setAttribute("role", "listitem");
            item.setAttribute("data-path", entry.path);

            if (state.selectedFile === entry.path) {
                item.classList.add("is-selected");
            }

            const config = STATUS_CONFIG[entry.status] || { badge: "?", className: "diff-status--unknown" };
            const badge = document.createElement("span");
            badge.className = `diff-status ${config.className}`;
            badge.textContent = config.badge;
            badge.title = entry.status;
            item.appendChild(badge);

            const icon = document.createElement("span");
            icon.className = "diff-file-icon";
            icon.innerHTML = FILE_ICON_SVG;
            item.appendChild(icon);

            const path = document.createElement("span");
            path.className = "diff-file-path";
            path.textContent = entry.path;
            path.title = entry.path;
            item.appendChild(path);

            if (entry.binary) {
                const binaryBadge = document.createElement("span");
                binaryBadge.className = "diff-file-binary-badge";
                binaryBadge.textContent = "binary";
                item.appendChild(binaryBadge);
            }

            item.addEventListener("click", () => {
                handleFileClick(entry);
            });

            list.appendChild(item);
        }

        container.appendChild(list);
        return container;
    }

    async function handleFileClick(entry) {
        state.selectedFile = entry.path;
        state.showFileContent = true;
        render();

        if (diffContentViewer && typeof diffContentViewer.showLoading === "function") {
            diffContentViewer.showLoading();
        }

        const gen = state.generation;
        try {
            const fileDiff = await fetchFileDiff(state.commitHash, entry.path);

            if (state.generation !== gen) return;

            if (diffContentViewer && typeof diffContentViewer.show === "function") {
                diffContentViewer.show(fileDiff);
            }
        } catch (err) {
            console.error("Failed to fetch file diff:", err);
            if (state.generation !== gen) return;
            if (diffContentViewer && typeof diffContentViewer.showError === "function") {
                diffContentViewer.showError(`Failed to load diff for ${entry.path}: ${err.message}`);
            }
        }
    }

    function renderLoadingState() {
        const container = document.createElement("div");
        container.className = "diff-view-loading";

        const spinner = document.createElement("div");
        spinner.className = "diff-loading-spinner";
        container.appendChild(spinner);

        const text = document.createElement("div");
        text.className = "diff-loading-text";
        text.textContent = "Loading commit diff...";
        container.appendChild(text);

        return container;
    }

    function renderEmptyState() {
        const empty = document.createElement("div");
        empty.className = "diff-view-empty";

        const icon = document.createElement("div");
        icon.className = "diff-empty-icon";
        icon.innerHTML = EMPTY_STATE_SVG;
        empty.appendChild(icon);

        const title = document.createElement("div");
        title.className = "diff-empty-title";
        title.textContent = "No changes";
        empty.appendChild(title);

        const hint = document.createElement("div");
        hint.className = "diff-empty-hint";
        hint.textContent = "This commit has no file changes";
        empty.appendChild(hint);

        return empty;
    }

    function renderErrorState() {
        const error = document.createElement("div");
        error.className = "diff-view-error";

        const icon = document.createElement("div");
        icon.className = "diff-error-icon";
        icon.innerHTML = ALERT_SVG;
        error.appendChild(icon);

        const title = document.createElement("div");
        title.className = "diff-error-title";
        title.textContent = "Failed to load diff";
        error.appendChild(title);

        const hint = document.createElement("div");
        hint.className = "diff-error-hint";
        hint.textContent = "There was an error loading the commit diff. Please try again.";
        error.appendChild(hint);

        return error;
    }

    async function open(commitHash, commitMessage = null) {
        state.generation++;
        state.commitHash = commitHash;
        state.commitMessage = commitMessage;
        state.parentHash = null;
        state.entries = [];
        state.stats = null;
        state.loading = true;
        state.selectedFile = null;
        state.showFileContent = false;
        state.filterText = "";

        el.style.display = "flex";
        render();

        const gen = state.generation;
        try {
            const diffData = await fetchCommitDiff(commitHash);

            if (state.generation !== gen) return;

            state.parentHash = diffData.parentHash;
            state.entries = diffData.entries || [];
            state.stats = diffData.stats || { added: 0, modified: 0, deleted: 0, filesChanged: 0 };
            state.loading = false;
            render();
        } catch (err) {
            console.error("Failed to fetch commit diff:", err);
            if (state.generation !== gen) return;
            state.entries = null;
            state.loading = false;
            render();
        }
    }

    function close() {
        el.style.display = "none";
        state.commitHash = null;
        state.parentHash = null;
        state.entries = [];
        state.stats = null;
        state.selectedFile = null;
        state.showFileContent = false;
        state.filterText = "";

        if (diffContentViewer && typeof diffContentViewer.close === "function") {
            diffContentViewer.close();
        }
    }

    function isOpen() {
        return el.style.display !== "none";
    }

    function getCommitHash() {
        return state.commitHash;
    }

    // Register internal back handler: return from file-level diff to file list.
    if (diffContentViewer && typeof diffContentViewer.onBack === "function") {
        diffContentViewer.onBack(() => {
            state.selectedFile = null;
            state.showFileContent = false;
            diffContentViewer.clear();
            render();
        });
    }

    // Initial render (empty state)
    render();

    return {
        el,
        open,
        close,
        isOpen,
        getCommitHash,
    };
}

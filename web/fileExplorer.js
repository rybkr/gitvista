/**
 * File explorer component — a tree-based file browser with expand/collapse,
 * lazy blame annotations, file content viewing, keyboard navigation, and ARIA.
 *
 * Architecture:
 * - Flat list rendering: builds visibleEntries array from expanded directories
 * - Lazy fetching: tree data cached on expand, blame data fetched async
 * - Stale response handling: generation counter discards outdated responses
 * - File viewing: switches to file content viewer when file is clicked
 * - Keyboard navigation: W3C APG TreeView pattern (arrow keys, Enter, Home/End)
 * - ARIA: role="tree", role="treeitem", aria-expanded, aria-activedescendant
 */

import { apiUrl } from "./apiBase.js";
import { createFileContentViewer } from "./fileContentViewer.js";
import { createDiffView } from "./diffView.js";
import { createDiffContentViewer } from "./diffContentViewer.js";
import { getFileIcon } from "./fileIcons.js";
import { formatRelativeTime } from "./utils/format.js";

// SVG icons
const FOLDER_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M1.5 3C1.5 2.44772 1.94772 2 2.5 2H6.29289C6.42551 2 6.55268 2.05268 6.64645 2.14645L7.85355 3.35355C7.94732 3.44732 8.07449 3.5 8.20711 3.5H13.5C14.0523 3.5 14.5 3.94772 14.5 4.5V12.5C14.5 13.0523 14.0523 13.5 13.5 13.5H2.5C1.94772 13.5 1.5 13.0523 1.5 12.5V3Z" fill="currentColor" opacity="0.2"/>
    <path d="M2.5 2H6.29289C6.42551 2 6.55268 2.05268 6.64645 2.14645L7.85355 3.35355C7.94732 3.44732 8.07449 3.5 8.20711 3.5H13.5C14.0523 3.5 14.5 3.94772 14.5 4.5V12.5C14.5 13.0523 14.0523 13.5 13.5 13.5H2.5C1.94772 13.5 1.5 13.0523 1.5 12.5V3C1.5 2.44772 1.94772 2 2.5 2Z" stroke="currentColor" stroke-width="1.2"/>
</svg>`;

const CHEVRON_SVG = `<svg width="10" height="10" viewBox="0 0 16 16" fill="none">
    <path d="M6 4l4 4-4 4" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const COLLAPSE_ALL_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 4h8M4 8h8M4 12h8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const EXPAND_ALL_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M3 3v4h4M13 13V9H9" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
    <path d="M13 3L8 8M3 13l5-5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const SEARCH_SVG = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
    <circle cx="7" cy="7" r="4.5" stroke="currentColor" stroke-width="1.5"/>
    <path d="M10.5 10.5L14 14" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const CLEAR_SVG = `<svg width="10" height="10" viewBox="0 0 16 16" fill="none">
    <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const DIFF_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M8 3v10M3 8h10" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const TREE_SVG = FOLDER_SVG;

const EMPTY_FOLDER_SVG = `<svg width="48" height="48" viewBox="0 0 16 16" fill="none">
    <path d="M1.5 3C1.5 2.44772 1.94772 2 2.5 2H6.29289C6.42551 2 6.55268 2.05268 6.64645 2.14645L7.85355 3.35355C7.94732 3.44732 8.07449 3.5 8.20711 3.5H13.5C14.0523 3.5 14.5 3.94772 14.5 4.5V12.5C14.5 13.0523 14.0523 13.5 13.5 13.5H2.5C1.94772 13.5 1.5 13.0523 1.5 12.5V3Z" fill="currentColor" opacity="0.1"/>
    <path d="M2.5 2H6.29289C6.42551 2 6.55268 2.05268 6.64645 2.14645L7.85355 3.35355C7.94732 3.44732 8.07449 3.5 8.20711 3.5H13.5C14.0523 3.5 14.5 3.94772 14.5 4.5V12.5C14.5 13.0523 14.0523 13.5 13.5 13.5H2.5C1.94772 13.5 1.5 13.0523 1.5 12.5V3C1.5 2.44772 1.94772 2 2.5 2Z" stroke="currentColor" stroke-width="0.8"/>
</svg>`;

/** Convert a file path to a valid DOM id for ARIA references. */
function pathToId(path) {
    return `explorer-item-${path.replace(/[^a-zA-Z0-9]/g, "-") || "root"}`;
}

export function createFileExplorer() {
    const el = document.createElement("div");
    el.className = "file-explorer";

    const contentViewer = createFileContentViewer();
    contentViewer.onBack(() => {
        state.selectedFile = null;
        state.breadcrumbPath = "";
        contentViewer.close();
        render();
    });

    const diffContentViewer = createDiffContentViewer();
    const diffView = createDiffView(null, diffContentViewer);
    diffView.el.style.display = "none"; // Initially hidden

    const state = {
        commitHash: null,          // Currently browsed commit hash
        commitMessage: null,        // First line of commit message
        rootTreeHash: null,         // Root tree hash of the commit
        expandedDirs: new Set(),    // Set of expanded directory paths (e.g. "src/internal")
        treeCache: new Map(),       // treeHash -> Tree { hash, entries: [] }
        blameCache: new Map(),      // "commitHash:path" -> { filename: BlameEntry }
        selectedFile: null,         // { path, blobHash } or null
        generation: 0,              // Incremented on openCommit to discard stale responses
        focusedIndex: 0,            // Index into the visibleEntries array for keyboard nav
        filterText: "",             // Current filter text
        filterDebounceTimer: null,  // Debounce timer for filter input
        prevVisiblePaths: new Set(),// Tracks previous render's paths for animation
        loading: false,             // True while root tree is loading
        breadcrumbPath: "",         // Current breadcrumb navigation path
        statusIndex: new Map(),     // path -> { code, category } from working tree status
        viewMode: "tree",           // "tree" or "diff" - controls which view is shown
    };

    async function fetchTree(treeHash) {
        const response = await fetch(apiUrl(`/tree/${treeHash}`));
        if (!response.ok) {
            throw new Error(`Failed to fetch tree ${treeHash}: ${response.status}`);
        }
        return response.json();
    }

    async function fetchBlame(commitHash, dirPath) {
        const url = apiUrl(`/tree/blame/${commitHash}?path=${encodeURIComponent(dirPath)}`);
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`Failed to fetch blame for ${dirPath}: ${response.status}`);
        }
        return response.json();
    }

    /**
     * Build a flat array of visible entries based on expanded directories.
     * Each entry includes ARIA sibling info (setSize, posInSet) for accessibility.
     * When filterText is set, only matching entries and their ancestors are returned.
     */
    function buildVisibleEntries() {
        const visible = [];

        if (!state.rootTreeHash) {
            return visible;
        }

        // Recursively walk from root
        function walk(treeHash, parentPath, depth) {
            const tree = state.treeCache.get(treeHash);
            if (!tree) return;

            // Sort entries: directories first (alphabetically), then files (alphabetically)
            const entries = [...tree.entries].sort((a, b) => {
                const aIsDir = a.type === "tree";
                const bIsDir = b.type === "tree";
                if (aIsDir && !bIsDir) return -1;
                if (!aIsDir && bIsDir) return 1;
                return a.name.localeCompare(b.name);
            });

            const setSize = entries.length;

            for (let pos = 0; pos < entries.length; pos++) {
                const entry = entries[pos];
                const entryPath = parentPath ? `${parentPath}/${entry.name}` : entry.name;
                const isDir = entry.type === "tree";

                const visibleEntry = {
                    path: entryPath,
                    name: entry.name,
                    depth,
                    isDir,
                    treeHash: isDir ? entry.hash : null,
                    blobHash: !isDir ? entry.hash : null,
                    mode: entry.mode,
                    blame: null,
                    setSize,
                    posInSet: pos + 1,
                };

                // Check if we have blame data for this entry
                const blameKey = `${state.commitHash}:${parentPath}`;
                const blameData = state.blameCache.get(blameKey);
                if (blameData && blameData.entries && blameData.entries[entry.name]) {
                    visibleEntry.blame = blameData.entries[entry.name];
                }

                visible.push(visibleEntry);

                // If this is a directory and it's expanded, recurse
                if (isDir && state.expandedDirs.has(entryPath)) {
                    walk(entry.hash, entryPath, depth + 1);
                }
            }
        }

        walk(state.rootTreeHash, "", 0);

        // Apply filter
        if (state.filterText) {
            const needle = state.filterText.toLowerCase();
            const matchedPaths = new Set();

            for (const entry of visible) {
                if (entry.name.toLowerCase().includes(needle)) {
                    matchedPaths.add(entry.path);
                    // Add all ancestor paths
                    let p = entry.path;
                    while (p.includes("/")) {
                        p = p.substring(0, p.lastIndexOf("/"));
                        matchedPaths.add(p);
                    }
                }
            }

            return visible.filter(e => matchedPaths.has(e.path));
        }

        return visible;
    }

    function renderToolbar() {
        const toolbar = document.createElement("div");
        toolbar.className = "file-explorer-toolbar";

        const collapseBtn = document.createElement("button");
        collapseBtn.title = "Collapse All";
        collapseBtn.innerHTML = COLLAPSE_ALL_SVG;
        collapseBtn.addEventListener("click", collapseAll);
        toolbar.appendChild(collapseBtn);

        const expandBtn = document.createElement("button");
        expandBtn.title = "Expand All";
        expandBtn.innerHTML = EXPAND_ALL_SVG;
        expandBtn.addEventListener("click", expandAll);
        toolbar.appendChild(expandBtn);

        const filterWrap = document.createElement("div");
        filterWrap.className = "file-explorer-filter";

        const searchIcon = document.createElement("span");
        searchIcon.className = "file-explorer-filter-icon";
        searchIcon.innerHTML = SEARCH_SVG;
        filterWrap.appendChild(searchIcon);

        const input = document.createElement("input");
        input.type = "text";
        input.placeholder = "Filter files...";
        input.value = state.filterText;
        input.addEventListener("input", (e) => {
            clearTimeout(state.filterDebounceTimer);
            state.filterDebounceTimer = setTimeout(() => {
                state.filterText = e.target.value;
                state.focusedIndex = 0;
                render();
                // Re-focus input after render
                const newInput = el.querySelector(".file-explorer-filter input");
                if (newInput) {
                    newInput.focus();
                    newInput.selectionStart = newInput.selectionEnd = newInput.value.length;
                }
            }, 150);
        });
        input.addEventListener("keydown", (e) => {
            if (e.key === "Escape") {
                e.preventDefault();
                state.filterText = "";
                state.focusedIndex = 0;
                render();
                const tree = el.querySelector(".file-explorer-tree");
                if (tree) tree.focus();
            }
        });
        filterWrap.appendChild(input);

        if (state.filterText) {
            const clearBtn = document.createElement("button");
            clearBtn.className = "file-explorer-filter-clear";
            clearBtn.innerHTML = CLEAR_SVG;
            clearBtn.addEventListener("click", () => {
                state.filterText = "";
                state.focusedIndex = 0;
                render();
            });
            filterWrap.appendChild(clearBtn);
        }

        toolbar.appendChild(filterWrap);
        return toolbar;
    }

    function renderBreadcrumbs() {
        if (!state.breadcrumbPath) return null;

        const bar = document.createElement("div");
        bar.className = "file-explorer-breadcrumbs";

        const segments = state.breadcrumbPath.split("/");

        // Root segment
        const rootBtn = document.createElement("button");
        rootBtn.className = "file-breadcrumb";
        rootBtn.textContent = "/";
        rootBtn.addEventListener("click", () => {
            state.breadcrumbPath = "";
            render();
        });
        bar.appendChild(rootBtn);

        for (let i = 0; i < segments.length; i++) {
            const sep = document.createElement("span");
            sep.className = "file-breadcrumb-sep";
            sep.textContent = "\u203A";
            bar.appendChild(sep);

            const segPath = segments.slice(0, i + 1).join("/");
            const isLast = i === segments.length - 1;

            if (isLast) {
                const current = document.createElement("span");
                current.className = "file-breadcrumb-current";
                current.textContent = segments[i];
                bar.appendChild(current);
            } else {
                const btn = document.createElement("button");
                btn.className = "file-breadcrumb";
                btn.textContent = segments[i];
                btn.addEventListener("click", () => {
                    state.breadcrumbPath = segPath;
                    render();
                    scrollToPath(segPath);
                });
                bar.appendChild(btn);
            }
        }

        return bar;
    }

    /** Scroll to and focus a specific path in the tree. */
    function scrollToPath(path) {
        const row = el.querySelector(`[data-path="${CSS.escape(path)}"]`);
        if (row) {
            row.scrollIntoView({ block: "nearest" });
            const entries = buildVisibleEntries();
            const idx = entries.findIndex(e => e.path === path);
            if (idx >= 0) {
                state.focusedIndex = idx;
                renderFocusUpdate(entries);
            }
        }
    }

    function renderEmptyState() {
        const empty = document.createElement("div");
        empty.className = "file-explorer-empty";

        const icon = document.createElement("div");
        icon.className = "file-explorer-empty-icon";
        icon.innerHTML = EMPTY_FOLDER_SVG;
        empty.appendChild(icon);

        const title = document.createElement("div");
        title.className = "file-explorer-empty-title";
        title.textContent = "No commit selected";
        empty.appendChild(title);

        const hint = document.createElement("div");
        hint.className = "file-explorer-empty-hint";
        hint.textContent = "Click a commit\u2019s tree icon to browse files";
        empty.appendChild(hint);

        return empty;
    }

    function renderSkeletonRows() {
        const container = document.createElement("div");
        container.className = "file-explorer-tree";

        const widths = [60, 80, 45, 70, 55, 65];
        for (let i = 0; i < 6; i++) {
            const row = document.createElement("div");
            row.className = "explorer-skeleton";
            row.style.paddingLeft = `${8 + (i > 2 ? 16 : 0)}px`;

            const iconSkel = document.createElement("div");
            iconSkel.className = "explorer-skeleton-icon";
            iconSkel.style.animationDelay = `${i * 0.1}s`;
            row.appendChild(iconSkel);

            const bar = document.createElement("div");
            bar.className = "explorer-skeleton-bar";
            bar.style.width = `${widths[i]}%`;
            bar.style.animationDelay = `${i * 0.1}s`;
            row.appendChild(bar);

            container.appendChild(row);
        }

        return container;
    }

    /** For directories, returns a status indicator if any descendant has status. */
    function getStatusForPath(entryPath, isDir) {
        if (state.statusIndex.size === 0) return null;

        if (!isDir) {
            return state.statusIndex.get(entryPath) || null;
        }

        // For directories: check if any descendant has status
        const prefix = entryPath + "/";
        for (const [path] of state.statusIndex) {
            if (path.startsWith(prefix)) {
                return { isDirIndicator: true };
            }
        }
        return null;
    }

    /**
     * Only calls diffView.open() when the commit has changed, to avoid
     * resetting diff state on re-renders triggered by unrelated updates.
     */
    function renderDiffMode() {
        el.innerHTML = "";

        const header = document.createElement("div");
        header.className = "file-explorer-header";
        const commitInfo = document.createElement("div");
        commitInfo.className = "file-explorer-commit";
        const shortHash = state.commitHash.substring(0, 7);
        const firstLine = state.commitMessage ? state.commitMessage.split("\n")[0] : "";
        commitInfo.textContent = `Commit: ${shortHash} - "${firstLine}"`;
        header.appendChild(commitInfo);

        const toggleBtn = document.createElement("button");
        toggleBtn.className = "file-explorer-toggle";
        toggleBtn.innerHTML = TREE_SVG + " Show Tree";
        toggleBtn.title = "Toggle between tree and diff view";
        toggleBtn.setAttribute("aria-label", "Toggle between tree and diff view");
        toggleBtn.addEventListener("click", () => {
            state.viewMode = "tree";
            diffView.close();
            render();
        });
        header.appendChild(toggleBtn);

        el.appendChild(header);

        if (!diffView.isOpen() || diffView.getCommitHash() !== state.commitHash) {
            diffView.open(state.commitHash, state.commitMessage);
        }

        el.appendChild(diffView.el);
    }

    function render() {
        const currentEntries = el.querySelectorAll(".explorer-entry[data-path]");
        const prevPaths = new Set();
        currentEntries.forEach(row => prevPaths.add(row.getAttribute("data-path")));
        state.prevVisiblePaths = prevPaths;

        el.innerHTML = "";

        if (!state.commitHash) {
            el.appendChild(renderEmptyState());
            return;
        }

        if (state.viewMode === "diff") {
            renderDiffMode();
            return;
        }

        const header = document.createElement("div");
        header.className = "file-explorer-header";
        const commitInfo = document.createElement("div");
        commitInfo.className = "file-explorer-commit";
        const shortHash = state.commitHash.substring(0, 7);
        const firstLine = state.commitMessage ? state.commitMessage.split("\n")[0] : "";
        commitInfo.textContent = `Commit: ${shortHash} - "${firstLine}"`;
        header.appendChild(commitInfo);

        const toggleBtn = document.createElement("button");
        toggleBtn.className = "file-explorer-toggle";
        toggleBtn.innerHTML = DIFF_SVG + " Show Diff";
        toggleBtn.title = "Toggle between tree and diff view";
        toggleBtn.setAttribute("aria-label", "Toggle between tree and diff view");
        toggleBtn.addEventListener("click", () => {
            state.viewMode = "diff";
            render();
        });
        header.appendChild(toggleBtn);

        el.appendChild(header);

        el.appendChild(renderToolbar());

        const breadcrumbs = renderBreadcrumbs();
        if (breadcrumbs) {
            el.appendChild(breadcrumbs);
        }

        if (state.loading) {
            el.appendChild(renderSkeletonRows());
            el.appendChild(contentViewer.el);
            return;
        }

        const treeContainer = document.createElement("div");
        treeContainer.className = "file-explorer-tree";
        treeContainer.setAttribute("role", "tree");
        treeContainer.setAttribute("aria-label", "File explorer");
        treeContainer.setAttribute("tabindex", "0");

        const visibleEntries = buildVisibleEntries();

        if (state.focusedIndex >= visibleEntries.length) {
            state.focusedIndex = Math.max(0, visibleEntries.length - 1);
        }

        if (visibleEntries[state.focusedIndex]) {
            treeContainer.setAttribute("aria-activedescendant",
                pathToId(visibleEntries[state.focusedIndex].path));
        }

        for (let i = 0; i < visibleEntries.length; i++) {
            const entry = visibleEntries[i];
            const row = document.createElement("div");
            row.className = "explorer-entry";
            row.id = pathToId(entry.path);
            row.setAttribute("data-path", entry.path);
            row.style.paddingLeft = `${8 + entry.depth * 16}px`;

            if (!state.prevVisiblePaths.has(entry.path)) {
                row.classList.add("is-new");
            }

            row.setAttribute("role", "treeitem");
            row.setAttribute("aria-level", String(entry.depth + 1));
            row.setAttribute("aria-setsize", String(entry.setSize));
            row.setAttribute("aria-posinset", String(entry.posInSet));
            if (entry.isDir) {
                row.setAttribute("aria-expanded",
                    state.expandedDirs.has(entry.path) ? "true" : "false");
            }

            if (i === state.focusedIndex) {
                row.classList.add("is-focused");
            }
            if (state.selectedFile && state.selectedFile.path === entry.path) {
                row.classList.add("is-selected");
            }

            if (entry.isDir) {
                const chevron = document.createElement("span");
                chevron.className = "explorer-chevron";
                chevron.innerHTML = CHEVRON_SVG;
                if (state.expandedDirs.has(entry.path)) {
                    chevron.classList.add("is-expanded");
                }
                row.appendChild(chevron);
            } else {
                const spacer = document.createElement("span");
                spacer.className = "explorer-chevron-spacer";
                row.appendChild(spacer);
            }

            const icon = document.createElement("span");
            icon.className = "explorer-icon";
            icon.innerHTML = entry.isDir ? FOLDER_SVG : getFileIcon(entry.name);
            row.appendChild(icon);

            const name = document.createElement("span");
            name.className = "explorer-name";
            name.textContent = entry.name;
            row.appendChild(name);

            const statusInfo = getStatusForPath(entry.path, entry.isDir);
            if (statusInfo) {
                if (statusInfo.isDirIndicator) {
                    const dot = document.createElement("span");
                    dot.className = "explorer-dir-indicator";
                    row.appendChild(dot);
                } else {
                    const badge = document.createElement("span");
                    let badgeCategory = statusInfo.category;
                    if (statusInfo.code === "D") badgeCategory = "deleted";
                    badge.className = `explorer-status explorer-status--${badgeCategory}`;
                    badge.textContent = statusInfo.code;
                    row.appendChild(badge);
                }
            }

            const blameContainer = document.createElement("span");
            blameContainer.className = "explorer-blame";

            if (entry.blame) {
                const age = document.createElement("span");
                age.className = "explorer-blame-age";
                age.textContent = formatRelativeTime(entry.blame.when);
                blameContainer.appendChild(age);

                const hash = document.createElement("span");
                hash.className = "explorer-blame-hash";
                hash.textContent = entry.blame.commitHash.substring(0, 7);
                blameContainer.appendChild(hash);
            } else {
                const skeleton = document.createElement("span");
                skeleton.className = "explorer-blame-skeleton";
                blameContainer.appendChild(skeleton);
            }

            row.appendChild(blameContainer);

            row.addEventListener("click", () => {
                state.focusedIndex = i;
                if (entry.isDir) {
                    handleDirClick(entry);
                } else {
                    handleFileClick(entry);
                }
            });

            treeContainer.appendChild(row);
        }

        treeContainer.addEventListener("keydown", (e) => handleKeyDown(e, visibleEntries));

        el.appendChild(treeContainer);
        el.appendChild(contentViewer.el);

        const focusedRow = treeContainer.querySelector(".is-focused");
        if (focusedRow) {
            focusedRow.scrollIntoView({ block: "nearest" });
        }
    }

    /**
     * Keyboard handler following W3C APG TreeView spec.
     * https://www.w3.org/WAI/ARIA/apg/patterns/treeview/
     */
    function handleKeyDown(e, entries) {
        if (entries.length === 0) return;

        const current = entries[state.focusedIndex];
        if (!current) return;

        switch (e.key) {
            case "ArrowDown":
                e.preventDefault();
                if (state.focusedIndex < entries.length - 1) {
                    state.focusedIndex++;
                    renderFocusUpdate(entries);
                }
                break;

            case "ArrowUp":
                e.preventDefault();
                if (state.focusedIndex > 0) {
                    state.focusedIndex--;
                    renderFocusUpdate(entries);
                }
                break;

            case "ArrowRight":
                e.preventDefault();
                if (current.isDir && !state.expandedDirs.has(current.path)) {
                    handleDirClick(current);
                } else if (current.isDir && state.expandedDirs.has(current.path)) {
                    // Move focus to first child
                    if (state.focusedIndex < entries.length - 1) {
                        state.focusedIndex++;
                        renderFocusUpdate(entries);
                    }
                }
                break;

            case "ArrowLeft":
                e.preventDefault();
                if (current.isDir && state.expandedDirs.has(current.path)) {
                    handleDirClick(current);
                } else {
                    // Move focus to parent directory
                    const parentPath = current.path.includes("/")
                        ? current.path.substring(0, current.path.lastIndexOf("/"))
                        : "";
                    const parentIndex = entries.findIndex(e => e.path === parentPath);
                    if (parentIndex >= 0) {
                        state.focusedIndex = parentIndex;
                        renderFocusUpdate(entries);
                    }
                }
                break;

            case "Enter":
            case " ":
                e.preventDefault();
                if (current.isDir) {
                    handleDirClick(current);
                } else {
                    handleFileClick(current);
                }
                break;

            case "Home":
                e.preventDefault();
                state.focusedIndex = 0;
                renderFocusUpdate(entries);
                break;

            case "End":
                e.preventDefault();
                state.focusedIndex = entries.length - 1;
                renderFocusUpdate(entries);
                break;

            case "*":
                e.preventDefault();
                expandSiblings(current, entries);
                break;

            case "/":
                e.preventDefault();
                const filterInput = el.querySelector(".file-explorer-filter input");
                if (filterInput) filterInput.focus();
                break;
        }
    }

    /** Lightweight focus update without full re-render. */
    function renderFocusUpdate(entries) {
        const prevFocused = el.querySelector(".explorer-entry.is-focused");
        if (prevFocused) prevFocused.classList.remove("is-focused");

        const rows = el.querySelectorAll(".explorer-entry");
        const row = rows[state.focusedIndex];
        if (row) {
            row.classList.add("is-focused");
            row.scrollIntoView({ block: "nearest" });
        }

        const treeContainer = el.querySelector(".file-explorer-tree");
        if (treeContainer && entries[state.focusedIndex]) {
            treeContainer.setAttribute("aria-activedescendant",
                pathToId(entries[state.focusedIndex].path));
        }
    }

    function expandSiblings(current, entries) {
        const parentPath = current.path.includes("/")
            ? current.path.substring(0, current.path.lastIndexOf("/"))
            : "";
        const siblings = entries.filter(e =>
            e.depth === current.depth &&
            e.isDir &&
            !state.expandedDirs.has(e.path) &&
            (e.path.includes("/")
                ? e.path.substring(0, e.path.lastIndexOf("/")) === parentPath
                : parentPath === "")
        );
        Promise.all(siblings.map(s => handleDirClick(s)));
    }

    function collapseAll() {
        state.expandedDirs.clear();
        state.expandedDirs.add("");
        state.breadcrumbPath = "";
        state.focusedIndex = 0;
        render();
    }

    function expandAll() {
        function walkAndExpand(treeHash, parentPath) {
            const tree = state.treeCache.get(treeHash);
            if (!tree) return;
            for (const entry of tree.entries) {
                if (entry.type === "tree") {
                    const entryPath = parentPath ? `${parentPath}/${entry.name}` : entry.name;
                    state.expandedDirs.add(entryPath);
                    if (state.treeCache.has(entry.hash)) {
                        walkAndExpand(entry.hash, entryPath);
                    }
                }
            }
        }
        if (state.rootTreeHash) {
            walkAndExpand(state.rootTreeHash, "");
        }
        render();
    }

    async function handleDirClick(entry) {
        const isExpanded = state.expandedDirs.has(entry.path);

        if (isExpanded) {
            state.expandedDirs.delete(entry.path);
            if (state.breadcrumbPath === entry.path ||
                state.breadcrumbPath.startsWith(entry.path + "/")) {
                const parent = entry.path.includes("/")
                    ? entry.path.substring(0, entry.path.lastIndexOf("/"))
                    : "";
                state.breadcrumbPath = parent;
            }
            render();
            const entries = buildVisibleEntries();
            const idx = entries.findIndex(e => e.path === entry.path);
            if (idx >= 0) state.focusedIndex = idx;
        } else {
            state.expandedDirs.add(entry.path);
            state.breadcrumbPath = entry.path;

            if (!state.treeCache.has(entry.treeHash)) {
                try {
                    const gen = state.generation;
                    const tree = await fetchTree(entry.treeHash);
                    if (state.generation !== gen) return;
                    state.treeCache.set(entry.treeHash, tree);
                } catch (err) {
                    console.error("Failed to fetch tree:", err);
                    state.expandedDirs.delete(entry.path);
                    return;
                }
            }

            render();

            const entries = buildVisibleEntries();
            const idx = entries.findIndex(e => e.path === entry.path);
            if (idx >= 0) state.focusedIndex = idx;

            const blameKey = `${state.commitHash}:${entry.path}`;
            if (!state.blameCache.has(blameKey)) {
                try {
                    const gen = state.generation;
                    const blameData = await fetchBlame(state.commitHash, entry.path);
                    if (state.generation !== gen) return;
                    state.blameCache.set(blameKey, blameData);
                    render();
                } catch (err) {
                    console.error("Failed to fetch blame:", err);
                }
            }
        }
    }

    function handleFileClick(entry) {
        state.selectedFile = { path: entry.path, blobHash: entry.blobHash };
        const parentPath = entry.path.includes("/")
            ? entry.path.substring(0, entry.path.lastIndexOf("/"))
            : "";
        state.breadcrumbPath = parentPath;
        for (const child of el.children) {
            if (child !== contentViewer.el) {
                child.style.display = "none";
            }
        }
        contentViewer.open(entry.blobHash, entry.name);
    }

    async function openCommit(commit) {
        state.generation++;
        state.commitHash = commit.hash;
        state.commitMessage = commit.message;
        state.rootTreeHash = commit.tree;
        state.expandedDirs.clear();
        state.treeCache.clear();
        state.blameCache.clear();
        state.selectedFile = null;
        state.focusedIndex = 0;
        state.filterText = "";
        state.breadcrumbPath = "";
        state.loading = true;
        state.viewMode = "tree";

        state.expandedDirs.add("");

        if (diffView.isOpen()) {
            diffView.close();
        }

        render();

        try {
            const gen = state.generation;
            const rootTree = await fetchTree(commit.tree);
            if (state.generation !== gen) return;
            state.treeCache.set(commit.tree, rootTree);
        } catch (err) {
            console.error("Failed to fetch root tree:", err);
            state.loading = false;
            render();
            return;
        }

        state.loading = false;
        render();

        const blameKey = `${state.commitHash}:`;
        try {
            const gen = state.generation;
            const blameData = await fetchBlame(state.commitHash, "");
            if (state.generation !== gen) return;
            state.blameCache.set(blameKey, blameData);
            render();
        } catch (err) {
            console.error("Failed to fetch root blame:", err);
        }
    }

    function updateWorkingTreeStatus(status) {
        state.statusIndex.clear();
        if (!status) return;

        const categorize = (files, category) => {
            if (!files) return;
            for (const file of files) {
                state.statusIndex.set(file.path, {
                    code: file.statusCode,
                    category,
                });
            }
        };

        categorize(status.staged, "staged");
        categorize(status.modified, "modified");
        categorize(status.untracked, "untracked");

        if (state.commitHash && !state.loading && state.viewMode === "tree") {
            render();
        }
    }

    render();

    return {
        el,
        openCommit,
        updateWorkingTreeStatus,
    };
}

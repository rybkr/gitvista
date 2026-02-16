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

import { createFileContentViewer } from "./fileContentViewer.js";

// SVG icons
const FOLDER_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M1.5 3C1.5 2.44772 1.94772 2 2.5 2H6.29289C6.42551 2 6.55268 2.05268 6.64645 2.14645L7.85355 3.35355C7.94732 3.44732 8.07449 3.5 8.20711 3.5H13.5C14.0523 3.5 14.5 3.94772 14.5 4.5V12.5C14.5 13.0523 14.0523 13.5 13.5 13.5H2.5C1.94772 13.5 1.5 13.0523 1.5 12.5V3Z" fill="currentColor" opacity="0.2"/>
    <path d="M2.5 2H6.29289C6.42551 2 6.55268 2.05268 6.64645 2.14645L7.85355 3.35355C7.94732 3.44732 8.07449 3.5 8.20711 3.5H13.5C14.0523 3.5 14.5 3.94772 14.5 4.5V12.5C14.5 13.0523 14.0523 13.5 13.5 13.5H2.5C1.94772 13.5 1.5 13.0523 1.5 12.5V3C1.5 2.44772 1.94772 2 2.5 2Z" stroke="currentColor" stroke-width="1.2"/>
</svg>`;

const FILE_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 2C3.44772 2 3 2.44772 3 3V13C3 13.5523 3.44772 14 4 14H12C12.5523 14 13 13.5523 13 13V6L9 2H4Z" fill="currentColor" opacity="0.2"/>
    <path d="M9 2V6H13M4 2H9L13 6V13C13 13.5523 12.5523 14 12 14H4C3.44772 14 3 13.5523 3 13V3C3 2.44772 3.44772 2 4 2Z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const CHEVRON_SVG = `<svg width="10" height="10" viewBox="0 0 16 16" fill="none">
    <path d="M6 4l4 4-4 4" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

/**
 * Format a date as a relative time string (e.g., "3d ago", "2mo ago").
 */
function formatRelativeTime(dateStr) {
    const date = new Date(dateStr);
    const now = Date.now();
    const diffMs = now - date.getTime();
    const diffMin = Math.floor(diffMs / 60000);
    if (diffMin < 1) return "now";
    if (diffMin < 60) return `${diffMin}m ago`;
    const diffHr = Math.floor(diffMin / 60);
    if (diffHr < 24) return `${diffHr}h ago`;
    const diffDay = Math.floor(diffHr / 24);
    if (diffDay < 30) return `${diffDay}d ago`;
    const diffMon = Math.floor(diffDay / 30);
    if (diffMon < 12) return `${diffMon}mo ago`;
    const diffYr = Math.floor(diffDay / 365);
    return `${diffYr}y ago`;
}

/** Convert a file path to a valid DOM id for ARIA references. */
function pathToId(path) {
    return `explorer-item-${path.replace(/[^a-zA-Z0-9]/g, "-") || "root"}`;
}

export function createFileExplorer() {
    const el = document.createElement("div");
    el.className = "file-explorer";

    // File content viewer (managed separately, shown when a file is clicked)
    const contentViewer = createFileContentViewer();
    contentViewer.onBack(() => {
        state.selectedFile = null;
        contentViewer.close();
        render();
    });

    // State
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
    };

    /**
     * Fetch tree data from API. Returns { hash, entries: [] }.
     */
    async function fetchTree(treeHash) {
        const response = await fetch(`/api/tree/${treeHash}`);
        if (!response.ok) {
            throw new Error(`Failed to fetch tree ${treeHash}: ${response.status}`);
        }
        return response.json();
    }

    /**
     * Fetch blame data for a directory at a given commit.
     * Returns { entries: { filename: BlameEntry } }.
     */
    async function fetchBlame(commitHash, dirPath) {
        const url = `/api/tree/blame/${commitHash}?path=${encodeURIComponent(dirPath)}`;
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`Failed to fetch blame for ${dirPath}: ${response.status}`);
        }
        return response.json();
    }

    /**
     * Build a flat array of visible entries based on expanded directories.
     * Each entry includes ARIA sibling info (setSize, posInSet) for accessibility.
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
        return visible;
    }

    /**
     * Render the file explorer UI with full ARIA tree semantics.
     */
    function render() {
        el.innerHTML = "";

        if (!state.commitHash) {
            const placeholder = document.createElement("div");
            placeholder.className = "file-explorer-placeholder";
            placeholder.textContent = "Click a commit's tree icon to browse files.";
            el.appendChild(placeholder);
            return;
        }

        // Header
        const header = document.createElement("div");
        header.className = "file-explorer-header";
        const commitInfo = document.createElement("div");
        commitInfo.className = "file-explorer-commit";
        const shortHash = state.commitHash.substring(0, 7);
        const firstLine = state.commitMessage ? state.commitMessage.split("\n")[0] : "";
        commitInfo.textContent = `Commit: ${shortHash} - "${firstLine}"`;
        header.appendChild(commitInfo);
        el.appendChild(header);

        // Tree container with ARIA tree role
        const treeContainer = document.createElement("div");
        treeContainer.className = "file-explorer-tree";
        treeContainer.setAttribute("role", "tree");
        treeContainer.setAttribute("aria-label", "File explorer");
        treeContainer.setAttribute("tabindex", "0");

        const visibleEntries = buildVisibleEntries();

        // Clamp focusedIndex to valid range
        if (state.focusedIndex >= visibleEntries.length) {
            state.focusedIndex = Math.max(0, visibleEntries.length - 1);
        }

        // Set aria-activedescendant to the focused entry
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

            // ARIA treeitem attributes
            row.setAttribute("role", "treeitem");
            row.setAttribute("aria-level", String(entry.depth + 1));
            row.setAttribute("aria-setsize", String(entry.setSize));
            row.setAttribute("aria-posinset", String(entry.posInSet));
            if (entry.isDir) {
                row.setAttribute("aria-expanded",
                    state.expandedDirs.has(entry.path) ? "true" : "false");
            }

            // Focus and selection state
            if (i === state.focusedIndex) {
                row.classList.add("is-focused");
            }
            if (state.selectedFile && state.selectedFile.path === entry.path) {
                row.classList.add("is-selected");
            }

            // Chevron (only for directories)
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

            // Icon
            const icon = document.createElement("span");
            icon.className = "explorer-icon";
            icon.innerHTML = entry.isDir ? FOLDER_SVG : FILE_SVG;
            row.appendChild(icon);

            // Name
            const name = document.createElement("span");
            name.className = "explorer-name";
            name.textContent = entry.name;
            row.appendChild(name);

            // Blame annotations
            const blameContainer = document.createElement("span");
            blameContainer.className = "explorer-blame";

            const age = document.createElement("span");
            age.className = "explorer-blame-age";
            age.textContent = entry.blame ? formatRelativeTime(entry.blame.when) : "...";
            blameContainer.appendChild(age);

            const hash = document.createElement("span");
            hash.className = "explorer-blame-hash";
            hash.textContent = entry.blame ? entry.blame.commitHash.substring(0, 7) : "";
            blameContainer.appendChild(hash);

            row.appendChild(blameContainer);

            // Click handler — also updates focus
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

        // Keyboard navigation (W3C APG TreeView pattern)
        treeContainer.addEventListener("keydown", (e) => handleKeyDown(e, visibleEntries));

        el.appendChild(treeContainer);
        el.appendChild(contentViewer.el);

        // Scroll focused item into view after full re-render
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
        }
    }

    /**
     * Lightweight focus update without full re-render.
     * Moves the .is-focused class and updates aria-activedescendant.
     */
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

    /**
     * Expand all sibling directories at the same level (* key).
     */
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

    /**
     * Handle directory click: toggle expand/collapse, fetch tree data if needed,
     * then lazily fetch blame data.
     */
    async function handleDirClick(entry) {
        const isExpanded = state.expandedDirs.has(entry.path);

        if (isExpanded) {
            state.expandedDirs.delete(entry.path);
            render();
            // Preserve focus on the collapsed directory
            const entries = buildVisibleEntries();
            const idx = entries.findIndex(e => e.path === entry.path);
            if (idx >= 0) state.focusedIndex = idx;
        } else {
            state.expandedDirs.add(entry.path);

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

            // Preserve focus on the expanded directory
            const entries = buildVisibleEntries();
            const idx = entries.findIndex(e => e.path === entry.path);
            if (idx >= 0) state.focusedIndex = idx;

            // Lazily fetch blame data in background
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

    /**
     * Handle file click: show file content viewer.
     */
    function handleFileClick(entry) {
        state.selectedFile = { path: entry.path, blobHash: entry.blobHash };
        for (const child of el.children) {
            if (child !== contentViewer.el) {
                child.style.display = "none";
            }
        }
        contentViewer.open(entry.blobHash, entry.name);
    }

    /**
     * Open a commit and load its root tree.
     * @param {Object} commit - Commit object with { hash, tree, message, ... }
     */
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

        state.expandedDirs.add("");

        try {
            const gen = state.generation;
            const rootTree = await fetchTree(commit.tree);
            if (state.generation !== gen) return;
            state.treeCache.set(commit.tree, rootTree);
        } catch (err) {
            console.error("Failed to fetch root tree:", err);
            render();
            return;
        }

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

    function getEl() {
        return el;
    }

    // Initial render
    render();

    return {
        el,
        openCommit,
        getEl,
    };
}

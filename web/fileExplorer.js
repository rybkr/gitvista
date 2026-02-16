/**
 * File explorer component — a tree-based file browser with expand/collapse,
 * lazy blame annotations, and file content viewing.
 *
 * Architecture:
 * - Flat list rendering: builds visibleEntries array from expanded directories
 * - Lazy fetching: tree data cached on expand, blame data fetched async
 * - Stale response handling: generation counter discards outdated responses
 * - File viewing: switches to file content viewer when file is clicked
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
     * Each entry: { path, name, depth, isDir, treeHash, blobHash, mode, blame }.
     */
    function buildVisibleEntries() {
        const visible = [];

        if (!state.rootTreeHash) {
            return visible;
        }

        // Recursively walk from root
        function walk(treeHash, parentPath, depth) {
            const tree = state.treeCache.get(treeHash);
            if (!tree) {
                // Tree not loaded yet (should not happen for root, but can for lazy-loaded dirs)
                return;
            }

            // Sort entries: directories first (alphabetically), then files (alphabetically)
            const entries = [...tree.entries].sort((a, b) => {
                const aIsDir = a.type === "tree";
                const bIsDir = b.type === "tree";
                if (aIsDir && !bIsDir) return -1;
                if (!aIsDir && bIsDir) return 1;
                return a.name.localeCompare(b.name);
            });

            for (const entry of entries) {
                const entryPath = parentPath ? `${parentPath}/${entry.name}` : entry.name;
                const isDir = entry.type === "tree";

                // Build entry object
                const visibleEntry = {
                    path: entryPath,
                    name: entry.name,
                    depth,
                    isDir,
                    treeHash: isDir ? entry.hash : null,
                    blobHash: !isDir ? entry.hash : null,
                    mode: entry.mode,
                    blame: null, // Will be populated lazily
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
     * Render the file explorer UI.
     */
    function render() {
        el.innerHTML = "";

        // Header: show commit info or placeholder
        const header = document.createElement("div");
        header.className = "file-explorer-header";

        if (!state.commitHash) {
            const placeholder = document.createElement("div");
            placeholder.className = "file-explorer-placeholder";
            placeholder.textContent = "Click a commit's tree icon to browse files.";
            el.appendChild(placeholder);
            return;
        }

        const commitInfo = document.createElement("div");
        commitInfo.className = "file-explorer-commit";
        const shortHash = state.commitHash.substring(0, 7);
        const firstLine = state.commitMessage ? state.commitMessage.split("\n")[0] : "";
        commitInfo.textContent = `Commit: ${shortHash} - "${firstLine}"`;
        header.appendChild(commitInfo);
        el.appendChild(header);

        // Tree list
        const treeContainer = document.createElement("div");
        treeContainer.className = "file-explorer-tree";

        const visibleEntries = buildVisibleEntries();

        for (const entry of visibleEntries) {
            const row = document.createElement("div");
            row.className = "explorer-entry";

            // Apply indentation
            row.style.paddingLeft = `${8 + entry.depth * 16}px`;

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
                // Spacer for files (no chevron)
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

            // Blame annotations (age and hash)
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

            // Selection state
            if (state.selectedFile && state.selectedFile.path === entry.path) {
                row.classList.add("is-selected");
            }

            // Click handler
            row.addEventListener("click", () => {
                if (entry.isDir) {
                    handleDirClick(entry);
                } else {
                    handleFileClick(entry);
                }
            });

            treeContainer.appendChild(row);
        }

        el.appendChild(treeContainer);
        el.appendChild(contentViewer.el);
    }

    /**
     * Handle directory click: toggle expand/collapse, fetch tree data if needed,
     * then lazily fetch blame data.
     */
    async function handleDirClick(entry) {
        const isExpanded = state.expandedDirs.has(entry.path);

        if (isExpanded) {
            // Collapse
            state.expandedDirs.delete(entry.path);
            render();
        } else {
            // Expand
            state.expandedDirs.add(entry.path);

            // Fetch tree data if not cached
            if (!state.treeCache.has(entry.treeHash)) {
                try {
                    const gen = state.generation;
                    const tree = await fetchTree(entry.treeHash);
                    if (state.generation !== gen) return; // Discard stale response
                    state.treeCache.set(entry.treeHash, tree);
                } catch (err) {
                    console.error("Failed to fetch tree:", err);
                    state.expandedDirs.delete(entry.path); // Revert expansion
                    return;
                }
            }

            // Re-render to show children
            render();

            // Lazily fetch blame data in background
            const blameKey = `${state.commitHash}:${entry.path}`;
            if (!state.blameCache.has(blameKey)) {
                try {
                    const gen = state.generation;
                    const blameData = await fetchBlame(state.commitHash, entry.path);
                    if (state.generation !== gen) return; // Discard stale response
                    state.blameCache.set(blameKey, blameData);
                    render(); // Re-render to show blame annotations
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
        // Hide tree and header, show content viewer
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
        // Increment generation to discard stale responses
        state.generation++;

        // Reset state
        state.commitHash = commit.hash;
        state.commitMessage = commit.message;
        state.rootTreeHash = commit.tree;
        state.expandedDirs.clear();
        state.treeCache.clear();
        state.blameCache.clear();
        state.selectedFile = null;

        // Auto-expand root
        state.expandedDirs.add("");

        // Fetch root tree
        try {
            const gen = state.generation;
            const rootTree = await fetchTree(commit.tree);
            if (state.generation !== gen) return; // Discard stale response
            state.treeCache.set(commit.tree, rootTree);
        } catch (err) {
            console.error("Failed to fetch root tree:", err);
            render();
            return;
        }

        // Render tree
        render();

        // Lazily fetch root blame data
        const blameKey = `${state.commitHash}:`;
        try {
            const gen = state.generation;
            const blameData = await fetchBlame(state.commitHash, "");
            if (state.generation !== gen) return; // Discard stale response
            state.blameCache.set(blameKey, blameData);
            render(); // Re-render to show blame annotations
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

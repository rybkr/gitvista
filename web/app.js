import { logger } from "./logger.js";
import { initThemeToggle } from "./themeToggle.js";
import { createGraphController as createGraph } from "./graph/graphController.js";
import { startBackend } from "./backend.js";
import { createSidebar } from "./sidebar.js";
import { createInfoBar } from "./infoBar.js";
import { createIndexView } from "./indexView.js";
import { createFileExplorer } from "./fileExplorer.js";
import { createStagingView } from "./stagingView.js";
import { createAnalyticsView } from "./analyticsView.js";
import { showToast } from "./toast.js";
import { createKeyboardShortcuts } from "./keyboardShortcuts.js";
import { createKeyboardHelp } from "./keyboardHelp.js";
import { createSearch } from "./search.js";
import { createGraphFilters, loadFilterState } from "./graphFilters.js";
import { setApiBase } from "./apiBase.js";
import { createRepoLanding } from "./repoLanding.js";
import { setConnectionState as setErrorConnectionState, setRepositoryAvailable } from "./errorState.js";
import { createConnectionBanner } from "./connectionBanner.js";
import { createRepoUnavailableOverlay } from "./repoUnavailableOverlay.js";

const COMMIT_HASH_RE = /^[0-9a-f]{40}$/i;
const REPO_HASH_RE = /^repo\/([^/]+)(?:\/([0-9a-f]{40}))?$/i;

/** Parses the URL hash. Returns { repoId, commitHash } or null. */
function parseHash() {
    const fragment = location.hash.slice(1);
    if (!fragment) return null;

    // SaaS: #repo/{id} or #repo/{id}/{commitHash}
    const m = REPO_HASH_RE.exec(fragment);
    if (m) return { repoId: m[1], commitHash: m[2] || null };

    // Local: #{commitHash}
    if (COMMIT_HASH_RE.test(fragment)) return { repoId: null, commitHash: fragment };

    return null;
}

document.addEventListener("DOMContentLoaded", async () => {
    logger.info("Bootstrapping frontend");

    initThemeToggle();

    const root = document.querySelector("#root");
    if (!root) {
        logger.error("Root element not found");
        return;
    }

    // ── Detect server mode ───────────────────────────────────────────────
    let mode = "local";
    try {
        const resp = await fetch("/api/config");
        if (resp.ok) {
            const config = await resp.json();
            mode = config.mode || "local";
        }
    } catch {
        // Default to local on failure
    }
    logger.info("Server mode", mode);

    if (mode === "local") {
        bootstrapGraph(root, null);
    } else {
        // SaaS mode: check hash for an existing repo selection
        const parsed = parseHash();
        if (parsed?.repoId) {
            setApiBase(`/api/repos/${parsed.repoId}`);
            bootstrapGraph(root, parsed.repoId);
        } else {
            showLanding(root);
        }

        window.addEventListener("hashchange", () => {
            const p = parseHash();
            if (p?.repoId) {
                setApiBase(`/api/repos/${p.repoId}`);
                clearRoot(root);
                bootstrapGraph(root, p.repoId);
            } else {
                clearRoot(root);
                showLanding(root);
            }
        });
    }
});

/** Removes sidebar elements and empties the root for a fresh view. */
function clearRoot(root) {
    const parent = root.parentElement;
    // Remove sidebar activity bar and panel if present
    parent.querySelectorAll(".activity-bar, .sidebar-panel").forEach((el) => el.remove());
    // Remove status dot
    document.querySelectorAll("[data-gv-status-dot]").forEach((el) => el.remove());
    root.innerHTML = "";
}

/** Shows the SaaS landing page. */
function showLanding(root) {
    document.title = "GitVista";
    const landing = createRepoLanding({
        onRepoSelect: (id) => {
            landing.destroy();
            location.hash = `repo/${id}`;
        },
    });
    root.appendChild(landing.el);
}

/** Bootstraps the graph view (works for both local and SaaS modes). */
function bootstrapGraph(root, repoId) {
    const statusDot = document.createElement("div");
    statusDot.setAttribute("data-gv-status-dot", "");
    statusDot.title = "Connecting...";
    statusDot.style.cssText = `
        position: fixed; bottom: 16px; right: 16px;
        width: 10px; height: 10px; border-radius: 50%;
        background: #8b949e; z-index: 9999; transition: background 300ms ease;
    `;
    document.body.appendChild(statusDot);

    const banner = createConnectionBanner();
    document.body.appendChild(banner.el);

    const overlay = createRepoUnavailableOverlay({ repoId });
    document.body.appendChild(overlay.el);

    const styleEl = document.createElement("style");
    styleEl.textContent = `
        @keyframes _gv-pulse {
            0%, 100% { box-shadow: 0 0 0 0 rgba(26,127,55,0.5); }
            50%       { box-shadow: 0 0 0 5px rgba(26,127,55,0); }
        }
        @keyframes _gv-amber-pulse {
            0%, 100% { box-shadow: 0 0 0 0 rgba(154,103,0,0.5); }
            50%       { box-shadow: 0 0 0 5px rgba(154,103,0,0); }
        }
    `;
    document.head.appendChild(styleEl);

    function setConnectionState(state) {
        if (state === "connected") {
            statusDot.style.background = "#1a7f37";
            statusDot.style.animation = "_gv-pulse 2s ease infinite";
            statusDot.title = "Connected";
        } else if (state === "reconnecting") {
            statusDot.style.background = "#9a6700";
            statusDot.style.animation = "_gv-amber-pulse 1s ease infinite";
            statusDot.title = "Reconnecting...";
        } else {
            statusDot.style.background = "#d1242f";
            statusDot.style.animation = "none";
            statusDot.title = "Disconnected";
        }
    }

    const infoBar = createInfoBar();
    const indexView = createIndexView();
    const fileExplorer = createFileExplorer();
    const stagingView = createStagingView();
    const analyticsView = createAnalyticsView({
        getCommits: () => graph.getCommits(),
        getTags: () => graph.getTags?.() ?? new Map(),
    });

    const repoTabContent = document.createElement("div");
    repoTabContent.style.display = "flex";
    repoTabContent.style.flexDirection = "column";
    repoTabContent.style.flex = "1";
    repoTabContent.style.overflow = "hidden";

    // In SaaS mode, add a "Back to repos" button at the top of the sidebar
    if (repoId) {
        const backBtn = document.createElement("button");
        backBtn.className = "back-to-repos";
        backBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
            <path d="M10 4L6 8l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg> Back to repos`;
        backBtn.addEventListener("click", () => {
            location.hash = "";
        });
        repoTabContent.appendChild(backBtn);
    }

    repoTabContent.appendChild(infoBar.el);
    repoTabContent.appendChild(indexView.el);

    const sidebar = createSidebar([
        { name: "repository", icon: "", tooltip: "Repository", content: repoTabContent },
        { name: "file-explorer", icon: "", tooltip: "File Explorer", content: fileExplorer.el },
        { name: "three-zones", tooltip: "Lifecycle", content: stagingView.el },
        { name: "analytics", tooltip: "Analytics", content: analyticsView.el },
    ]);
    root.parentElement.insertBefore(sidebar.activityBar, root);
    root.parentElement.insertBefore(sidebar.panel, root);

    let initialDeltaApplied = false;

    const graph = createGraph(root, {
        onCommitTreeClick: (commit) => {
            if (sidebar.getActivePanel() === "file-explorer") {
                fileExplorer.openCommit(commit);
            }
        },
        onCommitSelect: (hash) => {
            if (repoId) {
                // SaaS: preserve repo prefix in hash
                if (hash) {
                    history.replaceState(null, "", `#repo/${repoId}/${hash}`);
                } else {
                    history.replaceState(null, "", `#repo/${repoId}`);
                }
            } else {
                // Local mode
                if (hash) {
                    history.replaceState(null, "", "#" + hash);
                } else {
                    history.replaceState(null, "", location.pathname);
                }
            }
        },
    });

    // ── Canvas Toolbar ──────────────────────────────────────────────────────
    const canvasToolbar = document.createElement("div");
    canvasToolbar.className = "canvas-toolbar";

    const searchContainer = document.createElement("div");
    searchContainer.className = "search-container";
    canvasToolbar.appendChild(searchContainer);

    const search = createSearch(searchContainer, {
        getBranches: () => graph.getBranches(),
        getCommits: () => graph.getCommits(),
        getCommitCount: () => graph.getCommitCount(),
        onSearch: ({ searchState }) => {
            graph.setSearchState(searchState ?? null);
        },
    });

    const graphControlsEl = root.querySelector(".graph-controls");
    if (graphControlsEl) {
        canvasToolbar.appendChild(graphControlsEl);
    }

    const canvasEl = root.querySelector("canvas");
    if (canvasEl) {
        root.insertBefore(canvasToolbar, canvasEl);
    }

    // ── Filter popover ─────────────────────────────────────────────────
    const graphFilters = createGraphFilters({
        initialState: loadFilterState(),
        onChange: (filterState) => {
            graph.setFilterState(filterState);
        },
    });
    canvasToolbar.insertBefore(graphFilters.el, searchContainer);

    graph.setFilterState(graphFilters.getState());

    const keyboardHelp = createKeyboardHelp();

    createKeyboardShortcuts({
        onJumpToHead: () => graph.centerOnCommit(graph.getHeadHash()),
        onFocusSearch: () => {
            search.focus();
        },
        onToggleHelp: () => keyboardHelp.toggle(),
        onDismiss: () => {
            keyboardHelp.hide();
            search.clear();
        },
        onNavigateNext: () => graph.navigateCommits("next"),
        onNavigatePrev: () => graph.navigateCommits("prev"),
    });

    let currentBranchName = "";
    let repoName = "";

    function updateTitle() {
        if (currentBranchName && repoName) {
            document.title = `${currentBranchName} — ${repoName} — GitVista`;
        } else if (repoName) {
            document.title = `${repoName} — GitVista`;
        } else {
            document.title = "GitVista";
        }
    }

    /** Extract a permalink commit hash for restore. */
    function getPermalinkHash() {
        const parsed = parseHash();
        return parsed?.commitHash || null;
    }

    startBackend({
        logger,
        onConnectionStateChange: (state, attempt) => {
            setConnectionState(state);
            setErrorConnectionState(state, attempt);
        },
        onDelta: (delta) => {
            graph.applyDelta(delta);
            analyticsView.update();

            graphFilters.updateBranches(graph.getBranches());

            const addedCount = delta.addedCommits?.length ?? 0;
            if (addedCount > 0 && currentBranchName) {
                const branchName = (delta.addedBranches && Object.keys(delta.addedBranches).length > 0)
                    ? Object.keys(delta.addedBranches)[0]
                    : currentBranchName;
                const label = addedCount === 1
                    ? `1 new commit on ${branchName}`
                    : `${addedCount} new commits on ${branchName}`;
                showToast(label, { duration: 5000 });
            }

            if (!initialDeltaApplied) {
                initialDeltaApplied = true;
                const permalinkHash = getPermalinkHash();
                if (permalinkHash) {
                    setTimeout(() => {
                        graph.selectAndCenter(permalinkHash);
                    }, 80);
                }
            }
        },
        onStatus: (status) => {
            indexView.update(status);
            fileExplorer.updateWorkingTreeStatus(status);
            stagingView.update(status);
        },
        onHead: (headInfo) => {
            infoBar.updateHead(headInfo);
            if (headInfo.branchName) {
                currentBranchName = headInfo.branchName;
                updateTitle();
            }
            graph.setHeadHash(headInfo?.hash ?? null);
        },
        onRepoMetadata: (metadata) => {
            setRepositoryAvailable(true);
            infoBar.update(metadata);
            repoName = metadata.name || "";
            if (metadata.currentBranch) {
                currentBranchName = metadata.currentBranch;
            }
            updateTitle();
        },
    }).catch((error) => {
        logger.error("Backend bootstrap failed", error);
    });
}

import { logger } from "./logger.js";
import { initThemeToggle } from "./themeToggle.js";
import { createGraphController as createGraph } from "./graph/graphController.js";
import { startBackend } from "./backend.js";
import { createWorkbench } from "./workbench.js";
import { createIndexView } from "./indexView.js";
import { createFileExplorer } from "./fileExplorer.js";
import { createStagingView } from "./stagingView.js";
import { createAnalyticsView } from "./analyticsView.js";
import { createMergePreviewView } from "./mergePreviewView.js";
import { showToast } from "./toast.js";
import { createKeyboardShortcuts } from "./keyboardShortcuts.js";
import { createKeyboardHelp } from "./keyboardHelp.js";
import { createSearch } from "./search.js";
import { createGraphFilters, loadFilterState } from "./graphFilters.js";
import { setApiBase, apiUrl } from "./apiBase.js";
import { apiFetch } from "./apiFetch.js";
import { createRepoLanding } from "./repoLanding.js";
import { setConnectionState as setErrorConnectionState, setRepositoryAvailable } from "./errorState.js";
import { createConnectionBanner } from "./connectionBanner.js";
import { createRepoUnavailableOverlay } from "./repoUnavailableOverlay.js";
import { createGraphBreadcrumb } from "./graphBreadcrumb.js";
import { createGraphMinimap } from "./graphMinimap.js";
import { createGraphSettings } from "./graphSettings.js";
import { loadSettings } from "./graphSettingsDefaults.js";
import { createTelemetryHud, telemetryStore } from "./telemetry.js";

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
    const statusIndicator = document.createElement("div");
    statusIndicator.className = "gv-connection-indicator";
    statusIndicator.setAttribute("data-gv-status-dot", "");
    statusIndicator.dataset.state = "connecting";
    statusIndicator.setAttribute("role", "status");
    statusIndicator.setAttribute("aria-live", "polite");

    const statusLight = document.createElement("span");
    statusLight.className = "gv-connection-indicator__light";
    statusLight.setAttribute("aria-hidden", "true");

    const statusText = document.createElement("span");
    statusText.className = "gv-connection-indicator__text";
    statusText.textContent = "Connecting";

    statusIndicator.appendChild(statusLight);
    statusIndicator.appendChild(statusText);
    document.body.appendChild(statusIndicator);

    const banner = createConnectionBanner();
    document.body.appendChild(banner.el);

    const overlay = createRepoUnavailableOverlay({ repoId });
    document.body.appendChild(overlay.el);

    function setConnectionState(state) {
        if (state === "connected") {
            statusIndicator.dataset.state = "connected";
            statusText.textContent = "Connected";
            statusIndicator.title = "Connected";
        } else if (state === "reconnecting") {
            statusIndicator.dataset.state = "reconnecting";
            statusText.textContent = "Reconnecting";
            statusIndicator.title = "Reconnecting...";
        } else {
            statusIndicator.dataset.state = "disconnected";
            statusText.textContent = "Disconnected";
            statusIndicator.title = "Disconnected";
        }
    }

    const indexView = createIndexView();
    const fileExplorer = createFileExplorer();
    const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
    const fetchGraphCommits = async (hashes) => {
        if (!Array.isArray(hashes) || hashes.length === 0) return [];
        const unique = [...new Set(hashes.filter((h) => typeof h === "string" && h.length > 0))];
        if (unique.length === 0) return [];
        const batches = [];
        const CHUNK = 50; // smaller batches reduce 414/429 risks in busy sessions
        for (let i = 0; i < unique.length; i += CHUNK) {
            batches.push(unique.slice(i, i + CHUNK));
        }

        const all = [];
        for (const batch of batches) {
            const params = new URLSearchParams();
            params.set("hashes", batch.join(","));

            let lastError = null;
            for (let attempt = 0; attempt < 3; attempt++) {
                const resp = await apiFetch(apiUrl(`/graph/commits?${params.toString()}`));
                if (resp.ok) {
                    const payload = await resp.json();
                    if (Array.isArray(payload?.commits)) {
                        all.push(...payload.commits);
                    }
                    lastError = null;
                    break;
                }

                const body = await resp.text().catch(() => "");
                lastError = new Error(
                    `Failed to fetch graph commits (status ${resp.status}) ${body.slice(0, 160)}`,
                );
                const retryable =
                    resp.status === 429 ||
                    resp.status === 500 ||
                    resp.status === 502 ||
                    resp.status === 503 ||
                    resp.status === 504;
                if (!retryable || attempt === 2) break;
                await sleep(120 * (attempt + 1));
            }

            if (lastError) {
                logger.error("Graph commit hydration failed", lastError);
                throw lastError;
            }
        }
        return all;
    };
    const stagingView = createStagingView();
    const analyticsView = createAnalyticsView({
        getCommits: () => graph.getCommits(),
        getTags: () => graph.getTags?.() ?? new Map(),
        fetchGraphCommits,
        fetchAnalytics: async ({ period, start, end } = {}) => {
            const params = new URLSearchParams();
            if (typeof start === "string" && start && typeof end === "string" && end) {
                params.set("start", start);
                params.set("end", end);
            } else {
                const p = typeof period === "string" && period ? period : "all";
                params.set("period", p);
            }
            const resp = await apiFetch(apiUrl(`/analytics?${params.toString()}`));
            if (!resp.ok) throw new Error("Failed to fetch analytics");
            return resp.json();
        },
        fetchDiffStats: async ({ limit } = {}) => {
            const query = Number.isFinite(limit) && limit > 0 ? `?limit=${Math.floor(limit)}` : "";
            const resp = await apiFetch(apiUrl("/commits/diffstats" + query));
            if (!resp.ok) {
                telemetryStore.recordDiffStatsRequest(limit, false);
                throw new Error("Failed to fetch diff stats");
            }
            telemetryStore.recordDiffStatsRequest(limit, true);
            return resp.json();
        },
    });

    const mergePreviewView = createMergePreviewView({
        getBranches: () => graph.getBranches(),
        onPreviewResult: (preview) => {
            graph.setMergePreview(preview);
        },
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

    repoTabContent.appendChild(indexView.el);

    let graph = null;
    let graphFirstShown = false;

    const graphHost = document.createElement("div");
    graphHost.className = "graph-host";

    const workbench = createWorkbench([
        {
            name: "graph",
            tooltip: "Graph",
            content: graphHost,
            onShow: () => {
                graph?.refreshPalette?.();
                graph?.refreshViewport?.();
                graph?.rebuildVisibleGraph?.();
                if (!graphFirstShown) {
                    graphFirstShown = true;
                    const head = graph?.getHeadHash?.();
                    if (head) graph.centerOnCommit(head);
                }
            },
        },
        { name: "repository", tooltip: "Repository", content: repoTabContent },
        { name: "file-explorer", tooltip: "File Explorer", content: fileExplorer.el },
        { name: "three-zones", tooltip: "Lifecycle", content: stagingView.el },
        {
            name: "analytics",
            tooltip: "Analytics",
            content: analyticsView.el,
            onShow: () => {
                analyticsView.resetToDefaultPeriod();
                preloadAnalyticsOnce();
                analyticsView.update();
            },
        },
        { name: "compare", tooltip: "Compare", content: mergePreviewView.el },
    ]);
    root.appendChild(workbench.el);

    let initialBootstrapApplied = false;
    let analyticsPreloadStarted = false;

    function preloadAnalyticsOnce() {
        if (analyticsPreloadStarted) return;
        analyticsPreloadStarted = true;
        analyticsView.preload().catch(() => {
            // Keep startup resilient even if analytics preloading fails.
        });
    }

    // Warm analytics payloads as part of repo bootstrap.
    preloadAnalyticsOnce();

    graph = createGraph(graphHost, {
        fetchGraphCommits,
        onCommitTreeClick: (commit) => {
            if (workbench.isViewVisible("file-explorer")) {
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
            // Update breadcrumb on commit selection.
            refreshBreadcrumb();
        },
    });

    graph.refreshPalette?.();

    const telemetryHud = createTelemetryHud({
        getGraphTelemetry: () => graph.getTelemetrySnapshot?.() ?? null,
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
        getTags: () => graph.getTags?.() ?? new Map(),
        fetchDiffStats: async ({ limit } = {}) => {
            const query = Number.isFinite(limit) && limit > 0 ? `?limit=${Math.floor(limit)}` : "";
            const resp = await apiFetch(apiUrl("/commits/diffstats" + query));
            if (!resp.ok) {
                telemetryStore.recordDiffStatsRequest(limit, false);
                throw new Error("Failed to fetch diff stats");
            }
            telemetryStore.recordDiffStatsRequest(limit, true);
            return resp.json();
        },
        onSearch: ({ searchState }) => {
            graph.setSearchState(searchState ?? null);
            search.clearPosition();
        },
    });

    const graphControlsEl = graphHost.querySelector(".graph-controls");
    if (graphControlsEl) {
        canvasToolbar.appendChild(graphControlsEl);
    }

    const canvasEl = graphHost.querySelector("canvas");
    if (canvasEl) {
        canvasEl.parentElement.insertBefore(canvasToolbar, canvasEl);
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

    // ── Breadcrumb bar ────────────────────────────────────────────────
    const breadcrumb = createGraphBreadcrumb({
        onBranchClick: (branch) => {
            const tip = graph.getBranches().get(branch);
            if (tip) graph.selectAndCenter(tip);
        },
    });
    const canvasEl2 = graphHost.querySelector("canvas");
    if (canvasEl2) {
        canvasEl2.parentElement.insertBefore(breadcrumb.el, canvasEl2);
    }

    /** Helper to refresh breadcrumb from current graph state. */
    function refreshBreadcrumb() {
        const pos = graph.getNavigationPosition();
        if (!pos.selectedHash) {
            breadcrumb.update({ hash: null });
            return;
        }
        const commit = graph.getCommits().get(pos.selectedHash);
        // Find branch pointing at this commit
        let branch = null;
        for (const [name, hash] of graph.getBranches().entries()) {
            if (hash === pos.selectedHash && !name.startsWith("refs/remotes/")) {
                branch = name;
                break;
            }
        }
        breadcrumb.update({
            branch,
            hash: pos.selectedHash,
            message: commit?.message?.split("\n")[0] ?? null,
            index: pos.index,
            total: pos.total,
        });
    }

    // ── Minimap ───────────────────────────────────────────────────────
    const minimap = createGraphMinimap({
        getNodes: () => graph.getNodes(),
        getLinks: () => graph.getLinks(),
        getZoomTransform: () => graph.getZoomTransform(),
        getViewport: () => graph.getViewport(),
        onJump: (x, y) => graph.zoomTo(x, y),
    });
    graphHost.appendChild(minimap.el);

    // Hook minimap rendering into the graph render loop.
    graph.setMinimapCallback(() => minimap.render());

    // ── Settings overlay ──────────────────────────────────────────────
    const graphSettings = createGraphSettings({
        initialSettings: loadSettings(),
        onChange: (partial) => graph.setGraphSettings(partial),
        getBranches: () => graph.getBranches(),
        getLayoutMode: () => graph.getLayoutMode(),
    });
    canvasToolbar.appendChild(graphSettings.triggerEl);
    graphHost.appendChild(graphSettings.overlayEl);
    canvasToolbar.appendChild(telemetryHud.triggerEl);
    graphHost.appendChild(telemetryHud.panelEl);

    // Apply initial settings to the graph controller.
    graph.setGraphSettings(loadSettings());

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
            graph.clearIsolation();
            if (graphSettings.isVisible()) graphSettings.hide();
            if (telemetryHud.isVisible()) telemetryHud.hide();
        },
        onNavigateNext: () => {
            graph.navigateCommits("next");
            refreshBreadcrumb();
        },
        onNavigatePrev: () => {
            graph.navigateCommits("prev");
            refreshBreadcrumb();
        },
        onSearchResultNext: () => {
            const pos = graph.navigateSearchResults("next");
            if (pos) search.updatePosition(pos.index, pos.total);
            refreshBreadcrumb();
        },
        onSearchResultPrev: () => {
            const pos = graph.navigateSearchResults("prev");
            if (pos) search.updatePosition(pos.index, pos.total);
            refreshBreadcrumb();
        },
        onToggleSettings: () => graphSettings.toggle(),
    });

    /** If the file explorer has no commit loaded, open HEAD. */
    function openHeadInExplorerIfEmpty() {
        if (fileExplorer.hasCommit()) return;
        const headHash = graph.getHeadHash();
        if (!headHash) return;
        const commit = graph.getCommits().get(headHash);
        if (commit) fileExplorer.openCommit(commit);
    }

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
        onSummary: (summary) => {
            telemetryStore.recordSummary(summary);
            graph.applySummary(summary);
            analyticsView.update();
            preloadAnalyticsOnce();

            graphFilters.updateBranches(graph.getBranches());
            mergePreviewView.updateBranches();
            if (graphSettings.isVisible()) graphSettings.updateBranches();

            if (!initialBootstrapApplied) {
                initialBootstrapApplied = true;
                const permalinkHash = getPermalinkHash();
                if (permalinkHash) {
                    setTimeout(() => {
                        graph.selectAndCenter(permalinkHash);
                    }, 80);
                }
                openHeadInExplorerIfEmpty();
            }
        },
        onDelta: (delta) => {
            telemetryStore.recordDelta(delta);
            graph.applyDelta(delta);
            analyticsView.update();
            preloadAnalyticsOnce();

            graphFilters.updateBranches(graph.getBranches());
            mergePreviewView.updateBranches();
            if (graphSettings.isVisible()) graphSettings.updateBranches();

            // If a selected branch tip moved, refresh the merge preview.
            if (delta.amendedBranches) {
                const selected = mergePreviewView.getSelectedBranches();
                const amended = Object.keys(delta.amendedBranches);
                if (amended.includes(selected.ours) || amended.includes(selected.theirs)) {
                    mergePreviewView.refresh();
                }
            }

            const addedCount = delta.addedCommits?.length ?? 0;
            if (addedCount > 0 && currentBranchName && !delta.bootstrap) {
                const branchName = (delta.addedBranches && Object.keys(delta.addedBranches).length > 0)
                    ? Object.keys(delta.addedBranches)[0]
                    : currentBranchName;
                const label = addedCount === 1
                    ? `1 new commit on ${branchName}`
                    : `${addedCount} new commits on ${branchName}`;
                showToast(label, { duration: 5000 });
            }

            if (!initialBootstrapApplied) {
                initialBootstrapApplied = true;
                const permalinkHash = getPermalinkHash();
                if (permalinkHash) {
                    setTimeout(() => {
                        graph.selectAndCenter(permalinkHash);
                    }, 80);
                }
                openHeadInExplorerIfEmpty();
            }
        },
        onStatus: (status) => {
            indexView.updateStatus(status);
            fileExplorer.updateWorkingTreeStatus(status);
            stagingView.update(status);
        },
        onHead: (headInfo) => {
            indexView.updateHead(headInfo);
            if (headInfo.branchName) {
                currentBranchName = headInfo.branchName;
                updateTitle();
            }
            graph.setHeadHash(headInfo?.hash ?? null);
            openHeadInExplorerIfEmpty();
            refreshBreadcrumb();
        },
        onRepoMetadata: (metadata) => {
            setRepositoryAvailable(true);
            indexView.update(metadata);
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

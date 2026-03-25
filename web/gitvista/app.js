import { logger } from "../logger.js";
import { createGraphController as createGraph } from "../graph/graphController.js";
import { startBackend } from "../backend.js";
import { createWorkbench } from "../workbench.js";
import { createIndexView } from "../indexView.js";
import { createStagingView } from "../stagingView.js";
import { showToast } from "../toast.js";
import { createKeyboardShortcuts } from "../keyboardShortcuts.js";
import { createKeyboardHelp } from "../keyboardHelp.js";
import { createSearch } from "../search.js";
import { createGraphFilters, loadFilterState } from "../graphFilters.js";
import { apiUrl } from "../apiBase.js";
import { apiFetch } from "../apiFetch.js";
import { setConnectionState as setErrorConnectionState, setRepositoryAvailable } from "../errorState.js";
import { createConnectionBanner } from "../connectionBanner.js";
import { createRepoUnavailableOverlay } from "../repoUnavailableOverlay.js";
import { createGraphBreadcrumb } from "../graphBreadcrumb.js";
import { createGraphMinimap } from "../graphMinimap.js";
import { createGraphSettings } from "../graphSettings.js";
import { loadSettings } from "../graphSettingsDefaults.js";
import { createTelemetryHud, telemetryStore } from "../telemetry.js";

export function cleanupActiveView(cleanup) {
    if (typeof cleanup === "function") {
        cleanup();
    }
}

export function clearRoot(root) {
    // Remove status dot
    document.querySelectorAll("[data-gv-status-dot]").forEach((el) => el.remove());
    root.innerHTML = "";
}

/** Bootstraps the graph view (works for both local and hosted modes). */
export function bootstrapGraph(root, options = {}) {
    const {
        repoId = null,
        repoPathBuilder = null,
        navigateToPath = null,
        replacePath = null,
        parsePermalinkHash = null,
        parseLaunchTarget = null,
        productName = "GitVista",
    } = options;
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

    const overlay = createRepoUnavailableOverlay({
        repoId,
        onNavigateHome: repoId && navigateToPath ? () => navigateToPath("/") : null,
    });
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
    const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
    const GRAPH_COMMITS_CHUNK = 250;
    const fetchGraphCommits = async (hashes) => {
        if (!Array.isArray(hashes) || hashes.length === 0) return [];
        const unique = [...new Set(hashes.filter((h) => typeof h === "string" && h.length > 0))];
        if (unique.length === 0) return [];
        const batches = [];
        for (let i = 0; i < unique.length; i += GRAPH_COMMITS_CHUNK) {
            batches.push(unique.slice(i, i + GRAPH_COMMITS_CHUNK));
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
    const fetchDiffStats = async ({ limit } = {}) => {
        const query = Number.isFinite(limit) && limit > 0 ? `?limit=${Math.floor(limit)}` : "";
        const resp = await apiFetch(apiUrl("/commits/diffstats" + query));
        if (!resp.ok) {
            telemetryStore.recordDiffStatsRequest(limit, false);
            throw new Error("Failed to fetch diff stats");
        }
        telemetryStore.recordDiffStatsRequest(limit, true);
        return resp.json();
    };
    let workbench = null;
    const stagingView = createStagingView({
        onSelectFile: ({ path }) => {
            workbench?.focusView?.("file-explorer");
            ensureFileExplorerLoaded()
                .then(async (view) => {
                    if (!view) return;
                    const headHash = graph?.getHeadHash?.();
                    const headCommit = headHash ? graph?.getCommits?.()?.get(headHash) : null;
                    if (headCommit) {
                        await view.openCommit(headCommit);
                    }
                    await view.openFilePath(path);
                })
                .catch(() => {
                    // Keep Lifecycle usable even if the explorer fails to load.
                });
        },
        onOpenInExplorer: (payload) => {
            pendingExplorerDiff = payload;
            workbench?.focusView?.("file-explorer");
            ensureFileExplorerLoaded()
                .then(async (view) => {
                    if (!view) return;
                    const headHash = graph?.getHeadHash?.();
                    const headCommit = headHash ? graph?.getCommits?.()?.get(headHash) : null;
                    if (headCommit) {
                        await view.openCommit(headCommit);
                    }
                    await view.openExternalDiff(payload);
                    pendingExplorerDiff = null;
                })
                .catch(() => {
                    // Keep Lifecycle usable even if the explorer fails to load.
                });
        },
    });

    const repoTabContent = document.createElement("div");
    repoTabContent.style.display = "flex";
    repoTabContent.style.flexDirection = "column";
    repoTabContent.style.flex = "1";
    repoTabContent.style.overflow = "hidden";

    // In hosted mode, add a "Back to repos" button at the top of the sidebar
    if (repoId) {
        const backBtn = document.createElement("button");
        backBtn.className = "back-to-repos";
        backBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
            <path d="M10 4L6 8l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg> Back to repos`;
        backBtn.addEventListener("click", () => {
            navigateToPath?.("/");
        });
        repoTabContent.appendChild(backBtn);
    }

    repoTabContent.appendChild(indexView.el);

    const createLazyHost = (message) => {
        const host = document.createElement("div");
        host.className = "lazy-panel-host";
        const loading = document.createElement("div");
        loading.className = "lazy-panel-loading";
        loading.textContent = message;
        host.appendChild(loading);
        return {
            host,
            setLoaded(contentEl) {
                if (contentEl.parentElement !== host) {
                    host.innerHTML = "";
                    host.appendChild(contentEl);
                }
            },
        };
    };

    function createDeferredViewLoader(load) {
        let instance = null;
        let inflight = null;
        return {
            get() {
                return instance;
            },
            ensure() {
                if (instance) return Promise.resolve(instance);
                if (inflight) return inflight;
                inflight = Promise.resolve()
                    .then(load)
                    .then((next) => {
                        if (next) instance = next;
                        return instance;
                    })
                    .finally(() => {
                        inflight = null;
                    });
                return inflight;
            },
        };
    }

    let graph = null;
    let graphFirstShown = false;
    let latestStatus = null;
    let pendingExplorerCommit = null;
    let pendingExplorerDiff = null;
    let disposed = false;
    let backendSession = null;

    const graphHost = document.createElement("div");
    graphHost.className = "graph-host";
    const fileExplorerPanel = createLazyHost("Loading file explorer…");
    const analyticsPanel = createLazyHost("Loading analytics…");
    const comparePanel = createLazyHost("Loading merge preview…");

    const fetchAnalytics = async ({ period, start, end } = {}) => {
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
    };

    const fileExplorerLoader = createDeferredViewLoader(() =>
        import("../fileExplorer.js")
            .then(({ createFileExplorer }) => {
                if (disposed) return null;
                const view = createFileExplorer();
                fileExplorerPanel.setLoaded(view.el);
                if (latestStatus) view.updateWorkingTreeStatus(latestStatus);
                if (pendingExplorerCommit) {
                    view.openCommit(pendingExplorerCommit);
                    pendingExplorerCommit = null;
                }
                if (pendingExplorerDiff) {
                    view.openExternalDiff(pendingExplorerDiff);
                    pendingExplorerDiff = null;
                }
                return view;
            })
    );

    const analyticsLoader = createDeferredViewLoader(() =>
        import("../analyticsView.js")
            .then(({ createAnalyticsView }) => {
                if (disposed) return null;
                const view = createAnalyticsView({
                    getCommits: () => graph?.getCommits?.() ?? new Map(),
                    getTags: () => graph?.getTags?.() ?? new Map(),
                    fetchGraphCommits,
                    fetchAnalytics,
                    fetchDiffStats,
                });
                analyticsPanel.setLoaded(view.el);
                return view;
            })
    );

    const mergePreviewLoader = createDeferredViewLoader(() =>
        import("../mergePreviewView.js")
            .then(({ createMergePreviewView }) => {
                if (disposed) return null;
                const view = createMergePreviewView({
                    getBranches: () => graph?.getBranches?.() ?? new Map(),
                    onPreviewResult: (preview) => {
                        graph?.setMergePreview(preview);
                    },
                });
                comparePanel.setLoaded(view.el);
                view.updateBranches();
                return view;
            })
    );

    const ensureFileExplorerLoaded = () => fileExplorerLoader.ensure();
    const ensureAnalyticsLoaded = () => analyticsLoader.ensure();
    const ensureMergePreviewLoaded = () => mergePreviewLoader.ensure();

    workbench = createWorkbench([
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
        {
            name: "file-explorer",
            tooltip: "File Explorer",
            content: fileExplorerPanel.host,
            onShow: () => {
                ensureFileExplorerLoaded().catch(() => {
                    // Keep tab open; load errors are surfaced by view retries.
                });
            },
        },
        { name: "three-zones", tooltip: "Lifecycle", content: stagingView.el },
        {
            name: "analytics",
            tooltip: "Analytics",
            content: analyticsPanel.host,
            onShow: () => {
                ensureAnalyticsLoaded()
                    .then((view) => {
                        if (!view) return;
                        preloadAnalyticsOnce();
                        requestAnimationFrame(() => view.update());
                    })
                    .catch(() => {
                        // Keep tab open; view shows its own recoverable errors.
                    });
            },
        },
        {
            name: "compare",
            tooltip: "Compare",
            content: comparePanel.host,
            onShow: () => {
                ensureMergePreviewLoaded()
                    .then((view) => {
                        view?.updateBranches?.();
                    })
                    .catch(() => {
                        // Keep tab open; view shows its own recoverable errors.
                    });
            },
        },
    ]);
    root.appendChild(workbench.el);

    let initialBootstrapApplied = false;
    let analyticsPreloadStarted = false;

    function preloadAnalyticsOnce() {
        const analyticsView = analyticsLoader.get();
        if (analyticsPreloadStarted || !analyticsView) return;
        analyticsPreloadStarted = true;
        analyticsView.preload().catch(() => {
            // Keep startup resilient even if analytics preloading fails.
        });
    }

    graph = createGraph(graphHost, {
        fetchGraphCommits,
        onCommitTreeClick: (commit) => {
            if (workbench.isViewVisible("file-explorer")) {
                ensureFileExplorerLoaded().then((view) => {
                    if (!view) return;
                    view.openCommit(commit);
                }).catch(() => {
                    // Ignore load failures here; tab remains interactive.
                });
            }
        },
        onCommitSelect: (hash) => {
            if (repoId) {
                replacePath?.(repoPathBuilder ? repoPathBuilder(repoId, hash) : (hash ? `/repo/${repoId}/${hash}` : `/repo/${repoId}`));
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
        fetchDiffStats,
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
        getCommitCount: () => graph.getCommitCount(),
    });
    canvasToolbar.appendChild(graphSettings.triggerEl);
    graphHost.appendChild(graphSettings.overlayEl);
    canvasToolbar.appendChild(telemetryHud.triggerEl);
    graphHost.appendChild(telemetryHud.panelEl);

    // Apply initial settings to the graph controller.
    graph.setGraphSettings(loadSettings());

    const keyboardHelp = createKeyboardHelp();

    const keyboardShortcuts = createKeyboardShortcuts({
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
        const fileExplorer = fileExplorerLoader.get();
        if (fileExplorer?.hasCommit?.()) return;
        const headHash = graph.getHeadHash();
        if (!headHash) return;
        const commit = graph.getCommits().get(headHash);
        if (!commit) return;
        if (fileExplorer) {
            fileExplorer.openCommit(commit);
            return;
        }
        pendingExplorerCommit = commit;
    }

    let currentBranchName = "";
    let repoName = "";

    function updateTitle() {
        if (currentBranchName && repoName) {
            document.title = `${currentBranchName} — ${repoName} — ${productName}`;
        } else if (repoName) {
            document.title = `${repoName} — ${productName}`;
        } else {
            document.title = productName;
        }
    }

    /** Extract a permalink commit hash for restore. */
    function getPermalinkHash() {
        if (typeof parsePermalinkHash !== "function") return null;
        return parsePermalinkHash();
    }

    function getLaunchTarget() {
        if (typeof parseLaunchTarget !== "function") return { path: null, commitHash: null };
        return parseLaunchTarget() || { path: null, commitHash: null };
    }

    function openInitialPath(path, commitHash) {
        if (!path) return;

        workbench?.focusView?.("file-explorer");
        ensureFileExplorerLoaded()
            .then(async (view) => {
                if (!view) return;

                let resolvedCommit = null;
                if (commitHash) {
                    resolvedCommit = graph?.getCommits?.()?.get(commitHash) || null;
                    if (!resolvedCommit) {
                        const fetched = await fetchGraphCommits([commitHash]);
                        resolvedCommit = Array.isArray(fetched) && fetched.length > 0 ? fetched[0] : null;
                    }
                }

                if (!resolvedCommit) {
                    const headHash = graph?.getHeadHash?.();
                    resolvedCommit = headHash ? graph?.getCommits?.()?.get(headHash) || null : null;
                }

                if (resolvedCommit) {
                    await view.openCommit(resolvedCommit);
                }
                await view.openFilePath(path);
            })
            .catch(() => {
                // Keep startup resilient even if focused file loading fails.
            });
    }

    function syncBranchDependentViews() {
        const mergePreviewView = mergePreviewLoader.get();
        graphFilters.updateBranches(graph.getBranches());
        mergePreviewView?.updateBranches?.();
        graphSettings.refresh();
    }

    function applyInitialSelectionOnce() {
        if (initialBootstrapApplied) return;
        initialBootstrapApplied = true;
        const permalinkHash = getPermalinkHash();
        const launchTarget = getLaunchTarget();
        if (permalinkHash) {
            setTimeout(() => {
                graph.selectAndCenter(permalinkHash);
            }, 80);
        }
        if (launchTarget.path) {
            setTimeout(() => {
                openInitialPath(launchTarget.path, launchTarget.commitHash || permalinkHash);
            }, 120);
        }
        openHeadInExplorerIfEmpty();
    }

    const toBootstrapDelta = (chunk) => {
        if (!chunk) return null;
        return {
            addedCommits: Array.isArray(chunk.commits) ? chunk.commits : [],
            addedBranches: chunk.branches || {},
            headHash: chunk.headHash || "",
            bootstrap: true,
            bootstrapComplete: false,
        };
    };

    const toBootstrapCompleteDelta = (payload) => {
        if (!payload) return null;
        return {
            headHash: payload.headHash || "",
            tags: payload.tags || {},
            stashes: Array.isArray(payload.stashes) ? payload.stashes : [],
            bootstrap: true,
            bootstrapComplete: true,
        };
    };

    backendSession = startBackend({
        logger,
        onConnectionStateChange: (state, attempt) => {
            setConnectionState(state);
            setErrorConnectionState(state, attempt);
        },
        onBootstrapChunk: (chunk) => {
            const analyticsView = analyticsLoader.get();
            const delta = toBootstrapDelta(chunk);
            if (!delta) return;
            graph.applyDelta(delta);
            analyticsView?.update?.();
            preloadAnalyticsOnce();
            syncBranchDependentViews();
        },
        onBootstrapComplete: (payload) => {
            const analyticsView = analyticsLoader.get();
            const delta = toBootstrapCompleteDelta(payload);
            if (!delta) return;
            graph.applyDelta(delta);
            analyticsView?.update?.();
            preloadAnalyticsOnce();
            syncBranchDependentViews();
            applyInitialSelectionOnce();
        },
        onDelta: (delta) => {
            const analyticsView = analyticsLoader.get();
            const mergePreviewView = mergePreviewLoader.get();
            graph.applyDelta(delta);
            analyticsView?.update?.();
            preloadAnalyticsOnce();
            syncBranchDependentViews();

            // If a selected branch tip moved, refresh the merge preview.
            if (delta.amendedBranches && mergePreviewView) {
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

            applyInitialSelectionOnce();
        },
        onStatus: (status) => {
            const fileExplorer = fileExplorerLoader.get();
            latestStatus = status;
            indexView.updateStatus(status);
            fileExplorer?.updateWorkingTreeStatus?.(status);
            stagingView.updateStatus(status);
        },
        onHead: (headInfo) => {
            indexView.updateHead(headInfo);
            stagingView.updateHead(headInfo);
            if (headInfo.branchName) {
                currentBranchName = headInfo.branchName;
                updateTitle();
            }
            graph.setHeadHash(headInfo?.hash ?? null);
            openHeadInExplorerIfEmpty();
            refreshBreadcrumb();
        },
        onRepoMetadata: (metadata) => {
            graph.resetData?.();
            setRepositoryAvailable(true);
            indexView.update(metadata);
            stagingView.updateHead({
                branchName: metadata.currentBranch || "",
                isDetached: Boolean(metadata.headDetached),
                upstream: metadata.upstream || null,
            });
            repoName = metadata.name || "";
            if (metadata.currentBranch) {
                currentBranchName = metadata.currentBranch;
            }
            updateTitle();
        },
    });

    if (disposed) {
        backendSession?.destroy?.();
        backendSession = null;
    }

    return () => {
        if (disposed) {
            return;
        }
        disposed = true;
        backendSession?.destroy?.();
        backendSession = null;
        keyboardShortcuts.destroy();
        keyboardHelp.destroy?.();
        search.destroy?.();
        graphFilters.destroy?.();
        graphSettings.destroy?.();
        telemetryHud.destroy?.();
        graph.setMinimapCallback?.(null);
        minimap.destroy?.();
        graph.destroy?.();
        stagingView.destroy?.();
        workbench?.destroy?.();
        banner.destroy?.();
        overlay.destroy?.();
        statusIndicator.remove();
    };
}

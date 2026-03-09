/**
 * @fileoverview Primary controller orchestrating the Git graph visualization.
 * Wires together state, D3 simulation, rendering, tooltips, and interactions.
 */

import { select, zoom as d3Zoom } from "/vendor/d3-minimal.js";
import { TooltipManager } from "../tooltips/index.js";
import {
    BRANCH_NODE_OFFSET_X,
    BRANCH_NODE_OFFSET_Y,
    BRANCH_NODE_RADIUS,
    DRAG_ACTIVATION_DISTANCE,
    LANE_MARGIN,
    LANE_VERTICAL_STEP,
    LANE_WIDTH,
    LINK_DISTANCE,
    NODE_RADIUS,
    TAG_NODE_OFFSET_X,
    TAG_NODE_OFFSET_Y,
    TAG_NODE_RADIUS,
    ZOOM_MAX,
    ZOOM_MIN,
} from "./constants.js";
import { GraphRenderer } from "./rendering/graphRenderer.js";
import { ForceStrategy } from "./layout/forceStrategy.js";
import { LaneStrategy } from "./layout/laneStrategy.js";
import { buildFilterPredicate } from "./filterPredicate.js";
import { buildPalette } from "./utils/palette.js";
import { getCommitTimestamp } from "./utils/time.js";
import { createGraphState, setZoomTransform } from "./state/graphState.js";
import { CommitIndex } from "./state/commitIndex.js";
import { ViewportWindow } from "./state/viewportWindow.js";
import { NodeMaterializer } from "./state/nodeMaterializer.js";
import { loadSettings, saveSettings, getDefaults } from "../graphSettingsDefaults.js";

/**
 * Creates and initializes the graph controller instance.
 *
 * @param {HTMLElement} rootElement DOM node that hosts the canvas.
 * @param {{
 *   fetchGraphCommits?: (hashes: string[]) => Promise<import("./types.js").GraphCommit[]>,
  *   centerAnchorYFraction?: number,
 *   detailThresholds?: { message?: number, author?: number, date?: number },
 *   initialZoomScale?: number,
 *   initialLayoutMode?: "force" | "lane",
 *   onCommitTreeClick?: (commit: import("./types.js").GraphCommit) => void,
 *   onCommitSelect?: (hash: string | null) => void,
 *   showRefDecorators?: boolean,
 *   showControls?: boolean,
 * }} [options] Optional callbacks.
 * @returns {{
 *   applyDelta(delta: unknown): void,
 *   applySummary(summary: unknown): void,
 *   centerOnCommit(hash: string): void,
 *   navigateCommits(direction: 'prev' | 'next'): void,
 *   getTelemetrySnapshot?: () => Object,
 *   destroy(): void,
 * }} Public graph API.
 */
/** Returns a random value in the range [-range/2, range/2]. */
const jitter = (range) => (Math.random() - 0.5) * range;
const FORCE_MODE_MAX_COMMITS = 10000;

export function createGraphController(rootElement, options = {}) {
    const canvas = document.createElement("canvas");
    rootElement.appendChild(canvas);

    const state = createGraphState();
    const { commits, branches, nodes, links } = state;

    // Graph settings (physics + scope) — loaded from localStorage.
    state.graphSettings = loadSettings();
    let minimapCallback = null;

    let zoomTransform = state.zoomTransform;
    let dragState = null;
    let isDraggingNode = false;
    const pointerHandlers = {};

    let viewportWidth = 0;
    let viewportHeight = 0;

    let selectedHash = null;
    let sortedCommitCache = null;
    let searchResultCache = null;   // filtered subset matching search
    let searchResultIndex = -1;     // current position (-1 = unset)
    let rafId = null;
    const hydrationPending = new Set();
    const hydrationInflight = new Set();
    let hydrationFlushTimer = null;
    let searchHydrationEntries = [];
    let searchHydrationCursor = 0;
    let searchHydrationTimer = null;

    // Create both layout strategies
    const forceStrategy = new ForceStrategy({
        viewportWidth,
        viewportHeight,
        onTick: tick,
        onSettle: handleForceSettle,
    });
    const laneStrategy = new LaneStrategy({ onTick: tick });

    // Restore layout mode from localStorage, default to "force"
    const STORAGE_KEY_LAYOUT_MODE = "gitvista-layout-mode";
    const savedMode = localStorage.getItem(STORAGE_KEY_LAYOUT_MODE);
    const requestedMode = options.initialLayoutMode === "lane" ? "lane"
        : options.initialLayoutMode === "force" ? "force" : null;
    const initialMode = requestedMode || (savedMode === "lane" ? "lane" : "force");
    const centerAnchorYFraction = Number.isFinite(options.centerAnchorYFraction)
        ? Math.max(0, Math.min(1, options.centerAnchorYFraction))
        : 0.25;
    const initialZoomScale = Number.isFinite(options.initialZoomScale)
        ? Math.max(ZOOM_MIN, Math.min(ZOOM_MAX, options.initialZoomScale))
        : null;
    const showRefDecorators = options.showRefDecorators !== false;
    const showControls = options.showControls !== false;
    let layoutStrategy = initialMode === "lane" ? laneStrategy : forceStrategy;
    state.layoutMode = initialMode;

    // ── Lazy loading pipeline (lane mode + force Phase B) ──────────────
    const commitIndex = new CommitIndex();
    const viewportWindow = new ViewportWindow(commitIndex);
    const nodeMaterializer = new NodeMaterializer();
    let lazyLoadingActive = initialMode === "lane";
    /** @type {Map<string, {x: number, y: number}>|null} Temporary position seed for layout switching */
    let _savedPositions = null;
    let renderCount = 0;
    let lastViewportEntryCount = 0;
    let hydrationFetched = 0;
    let hydrationErrors = 0;

    const zoom = d3Zoom()
        .filter((event) => !isDraggingNode || event.type === "wheel")
        .scaleExtent([ZOOM_MIN, ZOOM_MAX])
        .on("zoom", (event) => {
            if (event.sourceEvent) {
                layoutStrategy.disableAutoCenter();
            }
            zoomTransform = event.transform;
            setZoomTransform(state, zoomTransform);
            if (lazyLoadingActive) {
                const { entries, changed } = viewportWindow.update(
                    zoomTransform, viewportWidth, viewportHeight,
                );
                if (changed) rematerializeFromViewport(entries);
            }
            render();
        });

    canvas.style.cursor = "default";

    // Build controls toolbar with mode toggle and rebalance
    const controls = document.createElement("div");
    controls.className = "graph-controls";

    const forceBtn = document.createElement("button");
    forceBtn.textContent = "Force";
    forceBtn.setAttribute("aria-label", "Switch to force-directed layout");
    if (initialMode === "force") {
        forceBtn.classList.add("is-active");
    }

    const laneBtn = document.createElement("button");
    laneBtn.textContent = "Lanes";
    laneBtn.setAttribute("aria-label", "Switch to lane-based layout");
    if (initialMode === "lane") {
        laneBtn.classList.add("is-active");
    }

    const rebalanceBtn = document.createElement("button");
    rebalanceBtn.textContent = "Rebalance";
    rebalanceBtn.setAttribute("aria-label", "Rebalance force-directed layout");
    rebalanceBtn.disabled = !layoutStrategy.supportsRebalance;

    controls.appendChild(forceBtn);
    controls.appendChild(laneBtn);
    controls.appendChild(rebalanceBtn);

    const updateForceButtonAvailability = () => {
        const forceAllowed = commits.size < FORCE_MODE_MAX_COMMITS;
        forceBtn.disabled = !forceAllowed;
        if (forceAllowed) {
            forceBtn.title = "Switch to force-directed layout";
        } else {
            forceBtn.title = `Force layout is available only for repositories under ${FORCE_MODE_MAX_COMMITS.toLocaleString()} commits`;
        }
    };
    updateForceButtonAvailability();

    // "Jump to HEAD" button — centers the view on the current HEAD commit.
    const headBtn = document.createElement("button");
    headBtn.textContent = "\u2302 HEAD";
    headBtn.title = "Jump to HEAD commit (G then H)";
    headBtn.addEventListener("click", () => {
        centerOnCommit(state.headHash || null);
    });
    controls.appendChild(headBtn);

    if (showControls) {
        rootElement.appendChild(controls);
    }

    /**
     * Switch between force-directed and lane-based layout modes.
     *
     * @param {"force" | "lane"} newMode The layout mode to switch to.
     */
    const switchLayout = (newMode) => {
        if (state.layoutMode === newMode) {
            return; // Already in this mode
        }
        if (newMode === "force" && commits.size >= FORCE_MODE_MAX_COMMITS) {
            return;
        }

        // Clear lane isolation when switching modes
        state.isolatedLanePosition = null;

        // When switching from lane → force, save lane positions so that
        // createCommitNode can place nodes where they were in lane mode
        // instead of spawning them randomly.  The nodes/links arrays are
        // cleared so D3 doesn't choke on stale link references.
        if (newMode === "force" && lazyLoadingActive) {
            _savedPositions = new Map();
            for (const entry of commitIndex.getAllEntries()) {
                _savedPositions.set(entry.hash, { x: entry.x, y: entry.y });
            }
        }

        // Deactivate current strategy
        layoutStrategy.deactivate();
        // Clear nodes/links when switching to force — D3 forceLink chokes
        // on stale link objects that reference old node indices.  When
        // switching to lane mode, keep existing nodes so the lane strategy
        // can animate the transition from the current positions.
        if (newMode === "force") {
            nodes.length = 0;
            links.length = 0;
        }

        // Switch to new strategy
        if (newMode === "lane") {
            layoutStrategy = laneStrategy;
            lazyLoadingActive = true;
            queueSearchHydration(state.searchState);
        } else {
            layoutStrategy = forceStrategy;
            lazyLoadingActive = false;
            nodeMaterializer.clear();
            clearSearchHydrationScan();
        }

        // Update state and localStorage
        state.layoutMode = newMode;
        localStorage.setItem(STORAGE_KEY_LAYOUT_MODE, newMode);

        // Update button active states
        if (newMode === "force") {
            forceBtn.classList.add("is-active");
            laneBtn.classList.remove("is-active");
        } else {
            laneBtn.classList.add("is-active");
            forceBtn.classList.remove("is-active");
        }
        updateForceButtonAvailability();

        // Enable/disable rebalance button based on strategy support
        rebalanceBtn.disabled = !layoutStrategy.supportsRebalance;

        // Activate new strategy with current graph state
        const viewport = { width: viewportWidth, height: viewportHeight };
        const activateOptions = _savedPositions ? { skipTimelineLayout: true } : undefined;
        layoutStrategy.activate(nodes, links, commits, branches, viewport, activateOptions);

        // Force immediate reposition
        updateGraph();
        _savedPositions = null;
        render();
    };

    // Wire button click handlers
    forceBtn.addEventListener("click", () => switchLayout("force"));
    laneBtn.addEventListener("click", () => switchLayout("lane"));
    rebalanceBtn.addEventListener("click", () => {
        if (layoutStrategy.supportsRebalance) {
            layoutStrategy.rebalance();
            snapDecoratorNodesForLaneDrag();
            render();
        }
    });

    const tooltipManager = new TooltipManager(canvas);
    const renderer = new GraphRenderer(canvas, buildPalette(canvas), {
        detailThresholds: options.detailThresholds,
    });

    const updateTooltipPosition = () => {
        tooltipManager.updatePosition(zoomTransform);
    };
    const hideTooltip = () => {
        tooltipManager.hideAll();
        render();
    };
    const showTooltip = (node) => {
        if (lazyLoadingActive && node?.type === "commit" && node.hash) {
            enqueueHydration([node.hash, ...(node.commit?.parents ?? [])]);
        }
        tooltipManager.show(node, zoomTransform);
        render();
    };

    const toGraphCoordinates = (event) => {
        const rect = canvas.getBoundingClientRect();
        const point = [event.clientX - rect.left, event.clientY - rect.top];
        const [x, y] = zoomTransform.invert(point);
        return { x, y };
    };

    const PICK_RADIUS_COMMIT = NODE_RADIUS + 4;
    const PICK_RADIUS_BRANCH = BRANCH_NODE_RADIUS + 6;
    const PICK_RADIUS_TAG = TAG_NODE_RADIUS + 6;

    const findNodeAt = (x, y, type) => {
        let bestNode = null;
        let bestDist = Infinity;

        for (const node of nodes) {
            if (node.type === "ghost-merge") continue;
            if (type && node.type !== type) {
                continue;
            }
            const dx = x - node.x;
            const dy = y - node.y;
            const distSq = dx * dx + dy * dy;
            let radius;
            if (node.type === "branch") {
                radius = PICK_RADIUS_BRANCH;
            } else if (node.type === "tag") {
                radius = PICK_RADIUS_TAG;
            } else {
                radius = PICK_RADIUS_COMMIT;
            }
            if (distSq <= radius * radius && distSq < bestDist) {
                bestDist = distSq;
                bestNode = node;
            }
        }

        return bestNode;
    };

    const translateViewTo = (x, y) => {
        const vw = viewportWidth || canvas.width;
        const vh = viewportHeight || canvas.height;
        if (initialZoomScale && Math.abs((zoomTransform?.k ?? 1) - initialZoomScale) > 0.01) {
            select(canvas).call(zoom.scaleTo, initialZoomScale);
        }
        select(canvas).call(
            zoom.translateTo,
            x,
            y,
            [vw / 2, vh * centerAnchorYFraction],
        );
    };

    const centerOnLatestCommit = () => {
        // Prefer the currently selected commit, then HEAD, then the layout
        // strategy's best guess (latest by timestamp).
        const preferredHash = selectedHash || state.headHash;
        if (preferredHash) {
            const targetNode = nodes.find(
                (n) => n.type === "commit" && n.hash === preferredHash,
            );
            if (targetNode) {
                if (state.layoutMode === "lane") {
                    translateViewTo(targetNode.x, targetNode.y);
                } else {
                    select(canvas).call(zoom.translateTo, targetNode.x, targetNode.y);
                }
                return;
            }
            // In lazy mode, the commit may not be materialized — use commitIndex
            if (lazyLoadingActive) {
                const entry = commitIndex.getByHash(preferredHash);
                if (entry) {
                    translateViewTo(entry.x, entry.y);
                    return;
                }
            }
        }

        const target = layoutStrategy.findCenterTarget(nodes);
        if (target) {
            select(canvas).call(zoom.translateTo, target.x, target.y);
        }
    };

    function commitNeedsHydration(commit) {
        // Missing commit in the map should also trigger hydration so we can
        // recover from stale summaries/bootstrap ordering.
        if (!commit) return true;
        const hasMessage = typeof commit.message === "string" && commit.message.length > 0;
        const hasTree = typeof commit.tree === "string" && commit.tree.length > 0;
        const hasAuthorIdentity =
            !!(commit.author?.name || commit.author?.Name || commit.author?.email || commit.author?.Email);
        // Lightweight bootstrap stubs contain topology + timestamps only.
        return !hasMessage && !hasTree && !hasAuthorIdentity;
    }

    function mergeCommitData(existing, incoming) {
        if (!existing) return incoming;
        if (!incoming) return existing;

        const merged = { ...existing, ...incoming };
        merged.parents = Array.isArray(incoming.parents) && incoming.parents.length > 0
            ? incoming.parents
            : (Array.isArray(existing.parents) ? existing.parents : []);

        const existingAuthor = existing.author ?? {};
        const incomingAuthor = incoming.author ?? {};
        merged.author = {
            ...existingAuthor,
            ...incomingAuthor,
            name: incomingAuthor.name || existingAuthor.name || "",
            email: incomingAuthor.email || existingAuthor.email || "",
            when: incomingAuthor.when || existingAuthor.when || "",
        };

        const existingCommitter = existing.committer ?? {};
        const incomingCommitter = incoming.committer ?? {};
        merged.committer = {
            ...existingCommitter,
            ...incomingCommitter,
            name: incomingCommitter.name || existingCommitter.name || "",
            email: incomingCommitter.email || existingCommitter.email || "",
            when: incomingCommitter.when || existingCommitter.when || "",
        };

        if (!incoming.message && existing.message) {
            merged.message = existing.message;
        }
        if (!incoming.tree && existing.tree) {
            merged.tree = existing.tree;
        }
        return merged;
    }

    function searchQueryNeedsHydration(query) {
        if (!query) return false;
        return (
            (query.textTerms?.length ?? 0) > 0 ||
            (query.negatedTextTerms?.length ?? 0) > 0 ||
            (query.authors?.length ?? 0) > 0 ||
            (query.negatedAuthors?.length ?? 0) > 0 ||
            (query.messages?.length ?? 0) > 0 ||
            (query.negatedMessages?.length ?? 0) > 0
        );
    }

    function clearSearchHydrationScan() {
        searchHydrationEntries = [];
        searchHydrationCursor = 0;
        if (searchHydrationTimer !== null) {
            clearTimeout(searchHydrationTimer);
            searchHydrationTimer = null;
        }
    }

    function pumpSearchHydration() {
        searchHydrationTimer = null;
        if (!options.fetchGraphCommits || searchHydrationCursor >= searchHydrationEntries.length) {
            return;
        }

        const enqueue = [];
        let scanned = 0;
        const SCAN_BUDGET = 2000;
        while (searchHydrationCursor < searchHydrationEntries.length && scanned < SCAN_BUDGET) {
            const entry = searchHydrationEntries[searchHydrationCursor++];
            scanned++;
            const commit = commits.get(entry.hash);
            if (commitNeedsHydration(commit)) {
                enqueue.push(entry.hash);
            }
        }
        if (enqueue.length > 0) {
            enqueueHydration(enqueue);
        }
        if (searchHydrationCursor < searchHydrationEntries.length) {
            searchHydrationTimer = setTimeout(pumpSearchHydration, 25);
        }
    }

    function queueSearchHydration(searchState) {
        if (!lazyLoadingActive || !options.fetchGraphCommits) {
            clearSearchHydrationScan();
            return;
        }
        if (!searchState?.query || !searchQueryNeedsHydration(searchState.query)) {
            clearSearchHydrationScan();
            return;
        }
        // Hydrate commit metadata in the background so global search/counts converge.
        searchHydrationEntries = commitIndex.getAllEntries();
        searchHydrationCursor = 0;
        if (searchHydrationTimer === null) {
            searchHydrationTimer = setTimeout(pumpSearchHydration, 0);
        }
    }

    function enqueueHydration(hashes) {
        if (!lazyLoadingActive || !options.fetchGraphCommits || !Array.isArray(hashes) || hashes.length === 0) return;
        for (const hash of hashes) {
            if (!hash || hydrationInflight.has(hash)) continue;
            const commit = commits.get(hash);
            if (!commitNeedsHydration(commit)) continue;
            hydrationPending.add(hash);
        }
        scheduleHydrationFlush();
    }

    function scheduleHydrationFlush() {
        if (!options.fetchGraphCommits || hydrationPending.size === 0 || hydrationFlushTimer !== null) {
            return;
        }
        hydrationFlushTimer = setTimeout(flushHydrationBatch, 50);
    }

    async function flushHydrationBatch() {
        hydrationFlushTimer = null;
        if (!options.fetchGraphCommits || hydrationPending.size === 0) return;

        const batch = [];
        for (const hash of hydrationPending) {
            hydrationPending.delete(hash);
            hydrationInflight.add(hash);
            batch.push(hash);
            if (batch.length >= 500) break;
        }
        if (batch.length === 0) return;

        try {
            const hydrated = await options.fetchGraphCommits(batch);
            let updated = 0;
            let structureChanged = false;
            const followupHydration = [];
            for (const commit of hydrated ?? []) {
                if (!commit?.hash) continue;
                const existing = commits.get(commit.hash);
                if (!existing) {
                    structureChanged = true;
                }
                const mergedCommit = mergeCommitData(existing, commit);
                commits.set(commit.hash, mergedCommit);
                for (const node of nodes) {
                    if (node.type === "commit" && node.hash === commit.hash) {
                        node.commit = mergedCommit;
                    }
                }
                for (const parent of mergedCommit?.parents ?? []) {
                    followupHydration.push(parent);
                }
                updated++;
            }
            hydrationFetched += updated;
            if (updated > 0) {
                if (followupHydration.length > 0) {
                    enqueueHydration(followupHydration);
                }
                if (structureChanged) {
                    updateGraph();
                }
                // Search/filter matchers depend on commit message/author fields.
                if (state.searchState || state.filterState?.focusBranch) {
                    applyDimmingFromPredicate();
                }
                render();
            }
        } catch {
            // Keep UI responsive; failed hydrations can be retried by future viewport updates.
            hydrationErrors++;
        } finally {
            for (const hash of batch) {
                hydrationInflight.delete(hash);
            }
            if (hydrationPending.size > 0) {
                scheduleHydrationFlush();
            }
        }
    }

    /**
     * Centers the viewport on the commit node with the given hash.
     * If the hash is null/undefined or not found among current nodes, falls back
     * to centering on the latest (HEAD) commit instead.
     *
     * @param {string | null} hash 40-character commit hash to center on.
     */
    const centerOnCommit = (hash) => {
        if (hash) {
            // Check materialized nodes first
            const target = nodes.find((n) => n.type === "commit" && n.hash === hash);
            if (target) {
                layoutStrategy.disableAutoCenter();
                translateViewTo(target.x, target.y);
                return;
            }
            // In lazy mode, the commit may be off-screen — use commitIndex
            if (lazyLoadingActive) {
                const entry = commitIndex.getByHash(hash);
                if (entry) {
                    layoutStrategy.disableAutoCenter();
                    translateViewTo(entry.x, entry.y);
                    // The zoom handler will fire → ViewportWindow re-queries → rematerializes
                    return;
                }
            }
        }
        // Graceful fallback: center on whatever the latest commit is.
        centerOnLatestCommit();
    };

    /**
     * Selects a commit by hash, shows its tooltip, fires onCommitSelect, and
     * centers the viewport on it.  Used by keyboard navigation and permalink restore.
     *
     * @param {string} hash 40-character commit hash to select.
     */
    const selectAndCenterCommit = (hash) => {
        let node = nodes.find((n) => n.type === "commit" && n.hash === hash);
        // In lazy mode, force-materialize if not currently in viewport
        if (!node && lazyLoadingActive) {
            const entry = commitIndex.getByHash(hash);
            const commit = commits.get(hash);
            if (entry && commit) {
                node = nodeMaterializer.forceMaterialize(hash, entry, commit);
            }
        }
        if (!node) {
            return;
        }
        enqueueHydration([hash, ...(node.commit?.parents ?? [])]);
        selectedHash = hash;
        showTooltip(node);
        options.onCommitSelect?.(hash);
        centerOnCommit(hash);

        // Keep search-result index in sync when navigating via click or J/K.
        if (searchResultCache) {
            const idx = searchResultCache.findIndex((n) => n.hash === hash);
            searchResultIndex = idx; // -1 if not a search result
        }
    };

    /**
     * Moves the selection to the next or previous commit in chronological order.
     * Commits are sorted newest-first (matching the timeline layout), so:
     *   - 'next' moves toward newer commits (lower index).
     *   - 'prev' moves toward older commits (higher index).
     * If nothing is selected, the HEAD commit is selected as the starting point.
     *
     * @param {'prev' | 'next'} direction Direction to navigate.
     */
    const navigateCommits = (direction) => {
        // In lazy mode, build sorted cache from commitIndex (all commits, not just materialized)
        if (!sortedCommitCache) {
            if (lazyLoadingActive) {
                // Use commitIndex entries sorted by Y (chronological in lane mode)
                // but we need commit data for timestamp-based sort
                const allEntries = commitIndex.getAllEntries();
                sortedCommitCache = allEntries
                    .map(e => ({ hash: e.hash, commit: commits.get(e.hash) }))
                    .filter(e => e.commit)
                    .sort((a, b) => {
                        const aTime = getCommitTimestamp(a.commit);
                        const bTime = getCommitTimestamp(b.commit);
                        if (aTime === bTime) return a.hash.localeCompare(b.hash);
                        return bTime - aTime;
                    });
            } else {
                sortedCommitCache = nodes
                    .filter((n) => n.type === "commit")
                    .sort((a, b) => {
                        const aTime = getCommitTimestamp(a.commit);
                        const bTime = getCommitTimestamp(b.commit);
                        if (aTime === bTime) return a.hash.localeCompare(b.hash);
                        return bTime - aTime;
                    });
            }
        }

        const commitNodes = sortedCommitCache;
        if (commitNodes.length === 0) {
            return;
        }

        // Determine the current position in the sorted array.
        let currentIndex = commitNodes.findIndex((n) => n.hash === selectedHash);

        if (currentIndex === -1) {
            // Nothing selected: start from HEAD (or the first node when HEAD is absent).
            const headIndex = state.headHash
                ? commitNodes.findIndex((n) => n.hash === state.headHash)
                : -1;
            currentIndex = headIndex !== -1 ? headIndex : 0;
            selectAndCenterCommit(commitNodes[currentIndex].hash);
            return;
        }

        // 'next' = newer = lower index in newest-first array.
        // 'prev' = older = higher index in newest-first array.
        const delta = direction === "next" ? -1 : 1;
        const nextIndex = currentIndex + delta;

        if (nextIndex < 0 || nextIndex >= commitNodes.length) {
            // Already at the boundary; no-op.
            return;
        }

        selectAndCenterCommit(commitNodes[nextIndex].hash);
    };

    /**
     * Moves the selection to the next or previous search result.
     * Only operates when a search matcher is active; otherwise returns null.
     *
     * @param {'next' | 'prev'} direction
     * @returns {{ index: number, total: number } | null}
     */
    const navigateSearchResults = (direction) => {
        if (!state.searchState?.matcher) return null;

        // Lazily build sortedCommitCache (same sort as navigateCommits).
        if (!sortedCommitCache) {
            if (lazyLoadingActive) {
                const allEntries = commitIndex.getAllEntries();
                sortedCommitCache = allEntries
                    .map(e => ({ hash: e.hash, commit: commits.get(e.hash) }))
                    .filter(e => e.commit)
                    .sort((a, b) => {
                        const aTime = getCommitTimestamp(a.commit);
                        const bTime = getCommitTimestamp(b.commit);
                        if (aTime === bTime) return a.hash.localeCompare(b.hash);
                        return bTime - aTime;
                    });
            } else {
                sortedCommitCache = nodes
                    .filter((n) => n.type === "commit")
                    .sort((a, b) => {
                        const aTime = getCommitTimestamp(a.commit);
                        const bTime = getCommitTimestamp(b.commit);
                        if (aTime === bTime) return a.hash.localeCompare(b.hash);
                        return bTime - aTime;
                    });
            }
        }

        // Lazily build searchResultCache by filtering with the active matcher.
        if (!searchResultCache) {
            searchResultCache = sortedCommitCache.filter((n) =>
                state.searchState.matcher(n.commit),
            );
            searchResultIndex = -1;
        }

        const results = searchResultCache;
        if (results.length === 0) return null;

        // On first call: if current selection is a result, start there.
        if (searchResultIndex === -1) {
            const currentIdx = results.findIndex((n) => n.hash === selectedHash);
            if (currentIdx !== -1) {
                searchResultIndex = currentIdx;
            } else {
                // Start at first result.
                searchResultIndex = 0;
                selectAndCenterCommit(results[0].hash);
                return { index: 0, total: results.length };
            }
        }

        // Step with wrapping.
        const delta = direction === "next" ? 1 : -1;
        searchResultIndex =
            (searchResultIndex + delta + results.length) % results.length;

        selectAndCenterCommit(results[searchResultIndex].hash);
        return { index: searchResultIndex, total: results.length };
    };

    // Wire Prev/Next buttons now that navigateCommits is defined.
    tooltipManager.tooltips.commit.setNavigate(navigateCommits);

    const releaseDrag = () => {
        if (!dragState) {
            return;
        }

        const current = dragState;

        // Delegate to layout strategy for drag end handling
        layoutStrategy.handleDragEnd(current.node);

        if (canvas.releasePointerCapture) {
            try {
                canvas.releasePointerCapture(current.pointerId);
            } catch {
                // ignore release failures (pointer already released)
            }
        }

        dragState = null;
        isDraggingNode = false;
        canvas.style.cursor = "default";
    };

    const handlePointerDown = (event) => {
        if (event.button !== 0) {
            return;
        }

        const { x, y } = toGraphCoordinates(event);

        // Lane header drag detection — check before node hit-test
        if (state.layoutMode === "lane") {
            const hit = laneStrategy.findLaneHeaderAt(x, y);
            if (hit) {
                event.stopImmediatePropagation();
                event.preventDefault();
                isDraggingNode = true;
                dragState = {
                    node: null,
                    pointerId: event.pointerId,
                    startX: x,
                    startY: y,
                    dragged: true,
                    laneDrag: true,
                    segmentHashes: hit.segmentHashes,
                    sourceLaneDisplay: hit.displayLane,
                    currentLaneDisplay: hit.displayLane,
                };
                canvas.style.cursor = "grabbing";
                try {
                    canvas.setPointerCapture(event.pointerId);
                } catch {
                    // ignore
                }
                return;
            }
        }

        // Lane body click → toggle isolation
        if (state.layoutMode === "lane") {
            const bodyHit = laneStrategy.findLaneBodyAt(x, y);
            if (bodyHit) {
                // Check that we're not clicking on a node
                const nodeAtClick = findNodeAt(x, y);
                if (!nodeAtClick) {
                    state.isolatedLanePosition = (state.isolatedLanePosition === bodyHit.position) ? null : bodyHit.position;
                    rebuildAndApplyPredicate();
                    return;
                }
            }
        }

        const targetNode = findNodeAt(x, y);

        if (!targetNode) {
            hideTooltip();
            // Clicking empty canvas clears lane isolation
            if (state.isolatedLanePosition !== null) {
                state.isolatedLanePosition = null;
                rebuildAndApplyPredicate();
            }
            return;
        }

        layoutStrategy.disableAutoCenter();

        event.stopImmediatePropagation();
        event.preventDefault();

        // Record drag intent but don't call handleDrag() yet — that happens
        // once the pointer moves past DRAG_ACTIVATION_DISTANCE.  This avoids
        // reheating the simulation and triggering a Phase B → A rebuild on a
        // simple click.  Tooltip/selection is deferred to pointerup so that
        // drags don't flash the tooltip.
        isDraggingNode = true;
        dragState = {
            node: targetNode,
            pointerId: event.pointerId,
            startX: event.clientX,
            startY: event.clientY,
            dragged: false,
        };

        try {
            canvas.setPointerCapture(event.pointerId);
        } catch {
            // ignore when pointer capture fails (browser limitations)
        }
    };

    // Pending hover check scheduled via rAF — avoids running the O(n) node scan
    // on every pixel of mouse movement (can be 60+ events/sec on modern hardware).
    let pendingHoverX = 0;
    let pendingHoverY = 0;
    let hoverRafId = null;

    const handlePointerMove = (event) => {
        if (dragState && event.pointerId === dragState.pointerId) {
            event.preventDefault();
            const { x, y } = toGraphCoordinates(event);

            // Lane header drag — move segment as pointer crosses lane boundaries
            if (dragState.laneDrag) {
                const currentDisplay = laneStrategy.findPositionAtX(x);
                if (currentDisplay !== dragState.currentLaneDisplay) {
                    laneStrategy.moveSegment(dragState.segmentHashes, currentDisplay);
                    dragState.currentLaneDisplay = currentDisplay;
                    // Re-snap branch/tag decorator nodes to new commit positions
                    snapDecoratorNodesForLaneDrag();
                }
                canvas.style.cursor = "grabbing";
                render();
                return;
            }

            if (!dragState.dragged) {
                // Compare in screen pixels so zoom level doesn't affect
                // the drag activation threshold.
                const distance = Math.hypot(
                    event.clientX - dragState.startX,
                    event.clientY - dragState.startY,
                );
                if (distance > DRAG_ACTIVATION_DISTANCE) {
                    dragState.dragged = true;
                    hideTooltip();
                }
            }

            if (dragState.dragged) {
                // Delegate drag handling to layout strategy
                const needsRender = layoutStrategy.handleDrag(dragState.node, x, y);
                if (needsRender) {
                    render();
                }
            }
            return;
        }

        // Capture coordinates immediately (event object may be reused by the browser).
        const { x, y } = toGraphCoordinates(event);
        pendingHoverX = x;
        pendingHoverY = y;

        // Throttle the O(n) hit-test to at most once per animation frame.
        if (hoverRafId !== null) return;
        hoverRafId = requestAnimationFrame(() => {
            hoverRafId = null;

            // Lane header hover cursor + title tooltip for full branch name
            if (state.layoutMode === "lane" && !dragState) {
                const hit = laneStrategy.findLaneHeaderAt(pendingHoverX, pendingHoverY);
                if (hit) {
                    state.hoverNode = null;
                    canvas.style.cursor = "grab";
                    canvas.title = hit.displayName || hit.branchOwner || hit.tipHash || "";
                    render();
                    return;
                }
            }

            const hit = findNodeAt(pendingHoverX, pendingHoverY);
            if (hit !== state.hoverNode || canvas.style.cursor === "grab") {
                state.hoverNode = hit;
                canvas.style.cursor = hit ? "pointer" : "default";
                canvas.title = "";
                render();
            }
        });
    };

    const handlePointerUp = (event) => {
        if (dragState && event.pointerId === dragState.pointerId) {
            const wasDragged = dragState.dragged;
            const clickedNode = dragState.node;
            releaseDrag();

            // Show tooltip / update selection only on click (no drag).
            if (!wasDragged && clickedNode) {
                const currentTarget = tooltipManager.getTargetData();
                if (tooltipManager.isVisible() && currentTarget === clickedNode) {
                    hideTooltip();
                    selectedHash = null;
                    options.onCommitSelect?.(null);
                } else {
                    showTooltip(clickedNode);
                    if (clickedNode.type === "commit") {
                        if (lazyLoadingActive) {
                            enqueueHydration([clickedNode.hash, ...(clickedNode.commit?.parents ?? [])]);
                        }
                        selectedHash = clickedNode.hash;
                        options.onCommitSelect?.(clickedNode.hash);
                    }
                    if (clickedNode.type === "commit" && clickedNode.commit && options.onCommitTreeClick) {
                        options.onCommitTreeClick(clickedNode.commit);
                    }
                }
            }
        }
    };

    let palette = buildPalette(canvas);
    let removeThemeWatcher = null;

    select(canvas).call(zoom).on("dblclick.zoom", null);

    const resize = () => {
        // Read the canvas's own flex-computed size (not the parent's) to avoid
        // a feedback loop: #root is a column flex container whose height is
        // determined by body's cross-axis stretch.  Reading parent.clientHeight
        // and writing it back as canvas.style.height would make #root grow on
        // every call.  Fall back to window dimensions on the very first call
        // before the browser has completed layout.
        const cssWidth = canvas.clientWidth || window.innerWidth;
        const cssHeight = canvas.clientHeight || window.innerHeight;
        const dpr = window.devicePixelRatio || 1;

        // Guard against invalid dimensions that would put canvas in error state
        if (cssWidth <= 0 || cssHeight <= 0 || !isFinite(cssWidth) || !isFinite(cssHeight)) {
            return;
        }

        const physicalWidth = Math.round(cssWidth * dpr);
        const physicalHeight = Math.round(cssHeight * dpr);

        // Prevent exceeding browser canvas size limits (typically 32767px)
        const MAX_CANVAS_DIMENSION = 32767;
        if (physicalWidth > MAX_CANVAS_DIMENSION || physicalHeight > MAX_CANVAS_DIMENSION) {
            return;
        }

        viewportWidth = cssWidth;
        viewportHeight = cssHeight;

        // Set the drawing buffer size (high-DPI).  Do NOT set
        // canvas.style.width/height — the CSS rules (flex: 1; width: 100%)
        // handle display sizing and must not be overridden by inline styles.
        canvas.width = physicalWidth;
        canvas.height = physicalHeight;

        layoutStrategy.updateViewport(cssWidth, cssHeight);
        render();

        // In lane/lazy mode, viewport changes must rematerialize the visible
        // commit window; otherwise an off-screen init can leave stale subsets.
        if (lazyLoadingActive) {
            viewportWindow.invalidate();
            const { entries, changed } = viewportWindow.update(
                zoomTransform,
                viewportWidth,
                viewportHeight,
            );
            if (changed) {
                rematerializeFromViewport(entries);
            } else {
                // Keep hydration progressing even if the window key is unchanged.
                const hashesToHydrate = [];
                for (const entry of entries) {
                    hashesToHydrate.push(entry.hash);
                    const commit = commits.get(entry.hash);
                    for (const parent of commit?.parents ?? []) {
                        hashesToHydrate.push(parent);
                    }
                }
                enqueueHydration(hashesToHydrate);
            }
        }
    };

    window.addEventListener("resize", resize);

    // Watch for container size changes (e.g. sidebar drag resize)
    let resizeObserver = null;
    const parent = canvas.parentElement;
    if (parent && typeof ResizeObserver !== "undefined") {
        resizeObserver = new ResizeObserver(resize);
        resizeObserver.observe(parent);
    }

    resize();

    const refreshPalette = () => {
        palette = buildPalette(canvas);
        renderer.updatePalette(palette);
        render();
    };

    const themeWatcher = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (themeWatcher) {
        if (themeWatcher.addEventListener) {
            themeWatcher.addEventListener("change", refreshPalette);
            removeThemeWatcher = () =>
                themeWatcher.removeEventListener("change", refreshPalette);
        } else if (themeWatcher.addListener) {
            themeWatcher.addListener(refreshPalette);
            removeThemeWatcher = () => themeWatcher.removeListener(refreshPalette);
        }
    }

    // Also watch for manual theme toggle (data-theme attribute on <html>).
    const themeAttrObserver = new MutationObserver(refreshPalette);
    themeAttrObserver.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ["data-theme"],
    });

    Object.assign(pointerHandlers, {
        down: handlePointerDown,
        move: handlePointerMove,
        up: handlePointerUp,
        cancel: handlePointerUp,
    });

    canvas.addEventListener("pointerdown", pointerHandlers.down);
    canvas.addEventListener("pointermove", pointerHandlers.move);
    canvas.addEventListener("pointerup", pointerHandlers.up);
    canvas.addEventListener("pointercancel", pointerHandlers.cancel);

    /**
     * Reconciles commit nodes by comparing existing nodes against current commits.
     * Creates new nodes for added commits, reuses existing nodes, and detects structural changes.
     *
     * @param {Map<string, import("./types.js").GraphNode>} existingNodes Map of hash -> existing commit nodes.
     * @param {Map<string, import("./types.js").GraphCommit>} commits Current commits from state.
     * @returns {{nodes: import("./types.js").GraphNode[], changed: boolean}} Reconciled nodes and change flag.
     */
    function reconcileCommitNodes(existingNodes, commits) {
        const nextNodes = [];
        let changed = existingNodes.size !== commits.size;

        // Build stash lookups so we can tag nodes during reconciliation.
        const stashHashes = new Map();
        for (const s of state.stashes ?? []) {
            if (s?.hash) stashHashes.set(s.hash, s.message);
        }
        const stashInternalHashes = new Set();
        const stashInternalKinds = new Map(); // hash → "index" | "untracked"
        for (const s of state.stashes ?? []) {
            const commit = commits.get(s?.hash);
            if (commit?.parents) {
                for (let i = 1; i < commit.parents.length; i++) {
                    stashInternalHashes.add(commit.parents[i]);
                    stashInternalKinds.set(commit.parents[i], i === 1 ? "index" : "untracked");
                }
            }
        }

        for (const commit of commits.values()) {
            const parentNode = (commit.parents ?? [])
                .map((parentHash) => existingNodes.get(parentHash))
                .find((node) => node);
            let grandparentNode = null;
            if (parentNode && commit.parents?.[0]) {
                const pc = commits.get(commit.parents[0]);
                grandparentNode = (pc?.parents ?? [])
                    .map((gph) => existingNodes.get(gph))
                    .find((n) => n) ?? null;
            }
            const node =
                existingNodes.get(commit.hash) ??
                createCommitNode(commit.hash, parentNode, grandparentNode);
            node.type = "commit";
            node.hash = commit.hash;
            node.commit = commit;
            node.isStash = stashHashes.has(commit.hash);
            node.stashMessage = stashHashes.get(commit.hash) ?? null;
            node.isStashInternal = stashInternalHashes.has(commit.hash);
            node.stashInternalKind = stashInternalKinds.get(commit.hash) ?? null;
            node.radius = node.radius ?? NODE_RADIUS;
            nextNodes.push(node);
            if (!existingNodes.has(commit.hash)) {
                changed = true;
            }
        }

        return { nodes: nextNodes, changed };
    }

    /**
     * Builds parent-child links between commits based on commit parent relationships.
     *
     * @param {Map<string, import("./types.js").GraphCommit>} commits Current commits from state.
     * @param {Set<string>} commitHashes Set of valid commit hashes (nodes that exist).
     * @returns {Array<{source: string, target: string}>} Array of link objects.
     */
    function buildLinks(commits, commitHashes) {
        const links = [];
        for (const commit of commits.values()) {
            if (!commit?.hash) {
                continue;
            }
            for (const parentHash of commit.parents ?? []) {
                if (!commitHashes.has(parentHash)) {
                    continue;
                }
                links.push({
                    source: parentHash,
                    target: commit.hash,
                });
            }
        }
        return links;
    }

    /**
     * Reconciles branch nodes by comparing existing nodes against current branches.
     * Creates new nodes for added branches, updates targets, and prepares alignment data.
     *
     * @param {Map<string, import("./types.js").GraphNode>} existingNodes Map of branchName -> existing branch nodes.
     * @param {Map<string, string>} branches Current branches from state (name -> target hash).
     * @param {Map<string, import("./types.js").GraphNode>} commitNodeByHash Map of hash -> commit node.
     * @returns {{nodes: import("./types.js").GraphNode[], links: Array<{source: import("./types.js").GraphNode, target: import("./types.js").GraphNode, kind: string}>, alignments: Array<{branchNode: import("./types.js").GraphNode, targetNode: import("./types.js").GraphNode}>, changed: boolean}} Reconciled data.
     */
    function reconcileBranchNodes(existingNodes, branches, commitNodeByHash) {
        const nextNodes = [];
        const branchLinks = [];
        const alignments = [];
        let changed = existingNodes.size !== branches.size;

        for (const [branchName, targetHash] of branches.entries()) {
            const targetNode = commitNodeByHash.get(targetHash);
            if (!targetNode) {
                continue;
            }

            let branchNode = existingNodes.get(branchName);
            const isNewNode = !branchNode;
            if (!branchNode) {
                branchNode = createBranchNode(branchName, targetNode);
            }

            const previousHash = branchNode.targetHash;
            branchNode.type = "branch";
            branchNode.branch = branchName;
            branchNode.targetHash = targetHash;
            if (isNewNode) {
                // In lane mode there is no continuous simulation tick to
                // drive the spawn animation, so start fully visible.
                branchNode.spawnPhase = lazyLoadingActive ? 1 : 0;
                changed = true;
            } else if (previousHash !== targetHash) {
                changed = true;
            }

            nextNodes.push(branchNode);
            branchLinks.push({
                source: branchNode,
                target: targetNode,
                kind: "branch",
            });
            alignments.push({ branchNode, targetNode });
        }

        return { nodes: nextNodes, links: branchLinks, alignments, changed };
    }

    /**
     * Reconciles tag nodes by comparing existing nodes against current tags.
     * Creates new nodes for added tags, updates targets, and prepares alignment data.
     *
     * @param {Map<string, import("./types.js").GraphNode>} existingNodes Map of tagName -> existing tag nodes.
     * @param {Map<string, string>} tags Current tags from state (name -> target hash).
     * @param {Map<string, import("./types.js").GraphNode>} commitNodeByHash Map of hash -> commit node.
     * @returns {{nodes: import("./types.js").GraphNode[], links: Array, alignments: Array, changed: boolean}}
     */
    function reconcileTagNodes(existingNodes, tags, commitNodeByHash) {
        const nextNodes = [];
        const tagLinks = [];
        const alignments = [];
        let changed = existingNodes.size !== tags.size;

        for (const [tagName, targetHash] of tags.entries()) {
            const targetNode = commitNodeByHash.get(targetHash);
            if (!targetNode) {
                continue;
            }

            let tagNode = existingNodes.get(tagName);
            const isNewNode = !tagNode;
            if (!tagNode) {
                tagNode = createTagNode(tagName, targetNode);
            }

            const previousHash = tagNode.targetHash;
            tagNode.type = "tag";
            tagNode.tag = tagName;
            tagNode.targetHash = targetHash;
            if (isNewNode) {
                tagNode.spawnPhase = lazyLoadingActive ? 1 : 0;
                changed = true;
            } else if (previousHash !== targetHash) {
                changed = true;
            }

            nextNodes.push(tagNode);
            tagLinks.push({
                source: tagNode,
                target: targetNode,
                kind: "tag",
            });
            alignments.push({ tagNode, targetNode });
        }

        return { nodes: nextNodes, links: tagLinks, alignments, changed };
    }

    /**
     * Rebuilds the compound filter predicate from the current searchQuery and
     * filterState stored on state, then applies dimming to all current nodes.
     * Call this whenever either field changes.
     */
    function rebuildAndApplyPredicate() {
        applyDimmingFromPredicate();
        render();
    }

    /**
     * Iterates all nodes and sets node.dimmed according to the active compound
     * predicate (A2 search + A3 structural filters).  When no criteria are active,
     * buildFilterPredicate returns null and all nodes are un-dimmed without looping.
     *
     * This is called after every delta so that branch/commit data inside the
     * predicate closure is always current (the BFS reachable set rebuilds here).
     */
    /** Rate at which dimPhase converges toward dimTarget per frame (~200ms transition at 60fps). */
    const DIM_LERP_RATE = 0.08;

    /**
     * Advances dimPhase toward dimTarget for all nodes. Returns true if any
     * transition is still in flight (needs another animation frame).
     */
    function advanceDimTransitions() {
        let anyActive = false;
        for (const node of nodes) {
            if (typeof node.dimPhase !== "number") continue;
            const target = node.dimTarget ?? 0;
            if (node.dimPhase !== target) {
                const diff = target - node.dimPhase;
                if (Math.abs(diff) < 0.01) {
                    node.dimPhase = target;
                } else {
                    node.dimPhase += diff * DIM_LERP_RATE;
                    node.dimPhase = Math.max(0, Math.min(1, node.dimPhase));
                    anyActive = true;
                }
            }
        }
        return anyActive;
    }

    function applyDimmingFromPredicate() {
        // Rebuild predicate so it captures the latest branches/commits/stashes.
        state.filterPredicate = buildFilterPredicate(
            state.searchState,
            state.filterState,
            branches,
            commits,
            state.stashes,
            state.headHash || null,
            state.isolatedLanePosition,
            laneStrategy._segments,
            state.graphSettings?.scope,
        );
        const predicate = state.filterPredicate;
        for (const node of nodes) {
            // Set the target dim state. dimPhase is lerped per-frame toward this
            // target in advanceDimTransitions() for smooth opacity transitions.
            node.dimTarget = predicate !== null ? (predicate(node) ? 0 : 1) : 0;
            if (typeof node.dimPhase !== "number") {
                node.dimPhase = node.dimTarget;
            }
            // Preserve boolean for backward-compat (getCommitCount, etc.)
            node.dimmed = node.dimTarget === 1;
        }
    }

    /**
     * Main graph update dispatcher. Routes to the appropriate pipeline:
     * - Lane mode: always lazy (viewport-materialized nodes)
     * - Force mode: Phase A (sim-only) → settles → Phase B (lazy)
     * - Fallback: eager (all nodes)
     */
    function updateGraph() {
        sortedCommitCache = null;
        searchResultCache = null;
        if (state.layoutMode === "lane") {
            if (!lazyLoadingActive) lazyLoadingActive = true;
            updateGraphLazy();
        } else {
            // Force mode uses the eager path — all commit, branch, and tag
            // nodes are always present so the simulation has full context.
            updateGraphEager();
        }
    }

    /**
     * Eager graph update — creates GraphNode objects for ALL commits.
     * Used in force mode and as the original code path.
     */
    function updateGraphEager() {
        const existingCommitNodes = new Map();
        const existingBranchNodes = new Map();
        const existingTagNodes = new Map();

        for (const node of nodes) {
            if (node.type === "branch" && node.branch) {
                existingBranchNodes.set(node.branch, node);
            } else if (node.type === "tag" && node.tag) {
                existingTagNodes.set(node.tag, node);
            } else if (node.type === "commit" && node.hash) {
                existingCommitNodes.set(node.hash, node);
            }
        }

        const commitReconciliation = reconcileCommitNodes(
            existingCommitNodes,
            commits,
        );
        const commitHashes = new Set(
            commitReconciliation.nodes.map((node) => node.hash),
        );
        const commitLinks = buildLinks(commits, commitHashes);
        const previousLinkCount = links.length;

        const commitNodeByHash = new Map(
            commitReconciliation.nodes.map((node) => [node.hash, node]),
        );
            const branchReconciliation = reconcileBranchNodes(
                existingBranchNodes,
                branches,
                commitNodeByHash,
            );
        const tagReconciliation = reconcileTagNodes(
            existingTagNodes,
            state.tags,
            commitNodeByHash,
        );

        const allNodes = [
            ...commitReconciliation.nodes,
            ...branchReconciliation.nodes,
            ...tagReconciliation.nodes,
        ];
        const allLinks = [
            ...commitLinks,
            ...branchReconciliation.links,
            ...tagReconciliation.links,
        ];

        nodes.splice(0, nodes.length, ...allNodes);
        links.splice(0, links.length, ...allLinks);

        if (dragState && !dragState.laneDrag && !nodes.includes(dragState.node)) {
            releaseDrag();
        }

        const currentTarget = tooltipManager.getTargetData();
        if (currentTarget && !nodes.includes(currentTarget)) {
            hideTooltip();
        }

        const linkStructureChanged = previousLinkCount !== allLinks.length;
        const structureChanged =
            commitReconciliation.changed ||
            branchReconciliation.changed ||
            tagReconciliation.changed ||
            linkStructureChanged;

        // Delegate layout updates to the strategy
        layoutStrategy.updateGraph(
            nodes,
            links,
            commits,
            branches,
            { width: viewportWidth, height: viewportHeight },
            structureChanged,
        );

        // Snap branch and tag nodes AFTER layout so laneIndex/x/y are set
        snapBranchesToTargets(branchReconciliation.alignments);
        snapTagsToTargets(tagReconciliation.alignments);

        // Inject ghost merge node
        injectGhostMergeNode(commitNodeByHash);

        // Apply the active compound predicate (A2 search + A3 filters) to set
        // node.dimmed.  This runs after layout so lane segments are rebuilt and
        // newly-created nodes are correctly included in isolation predicates.
        applyDimmingFromPredicate();

        // Center on latest commit if auto-centering is requested
        if (layoutStrategy.shouldAutoCenter()) {
            centerOnLatestCommit();
        }

    }

    /**
     * Handles the force simulation convergence event.
     * Force mode uses the eager path so no Phase B transition is needed.
     * The onSettle callback is still wired so the settled flag and position
     * snapshot are captured inside ForceStrategy.
     */
    function handleForceSettle() {
        // No-op
    }

    /**
     * Builds links for a subset of materialized commit nodes.
     * Only includes links where both endpoints are materialized.
     *
     * @param {Map<string, Object>} commits Full commit data map.
     * @param {Set<string>} materializedHashes Hashes of materialized nodes.
     * @returns {Array<{source: string, target: string}>}
     */
    function buildLinksForMaterialized(commits, materializedHashes) {
        const result = [];
        for (const hash of materializedHashes) {
            const commit = commits.get(hash);
            if (!commit?.parents) continue;
            for (const parentHash of commit.parents) {
                if (materializedHashes.has(parentHash)) {
                    result.push({ source: parentHash, target: hash });
                }
            }
        }
        return result;
    }

    /**
     * Lazy graph update — runs laneStrategy on pseudo-nodes, then materializes
     * only the viewport-visible subset as real GraphNode objects.
     */
    function updateGraphLazy() {
        // 1. Build pseudo-nodes from commits + stash data
        const stashSet = new Set((state.stashes ?? []).map(s => s?.hash).filter(Boolean));
        const stashInternalSet = new Set();
        const stashInternalKinds = new Map();
        for (const s of state.stashes ?? []) {
            const commit = commits.get(s?.hash);
            if (commit?.parents) {
                for (let i = 1; i < commit.parents.length; i++) {
                    stashInternalSet.add(commit.parents[i]);
                    stashInternalKinds.set(commit.parents[i], i === 1 ? "index" : "untracked");
                }
            }
        }

        const pseudoNodes = [];
        for (const [hash, commit] of commits) {
            pseudoNodes.push({
                type: "commit",
                hash,
                isStash: stashSet.has(hash),
                isStashInternal: stashInternalSet.has(hash),
                stashInternalKind: stashInternalKinds.get(hash) ?? null,
            });
        }

        // 2. Run laneStrategy on pseudo-nodes (computes layout into internal maps)
        const viewport = { width: viewportWidth, height: viewportHeight };
        layoutStrategy.updateGraph(pseudoNodes, [], commits, branches, viewport, true);

        // 3. Build CommitIndex from laneStrategy position data
        commitIndex.rebuild(commits, layoutStrategy.getPositionData(), state.stashes);

        // 4. Query ViewportWindow
        viewportWindow.invalidate();
        const { entries } = viewportWindow.update(zoomTransform, viewportWidth, viewportHeight);
        lastViewportEntryCount = entries.length;
        const hashesToHydrate = [];
        for (const entry of entries) {
            hashesToHydrate.push(entry.hash);
            const commit = commits.get(entry.hash);
            for (const parent of commit?.parents ?? []) {
                hashesToHydrate.push(parent);
            }
        }
        enqueueHydration(hashesToHydrate);

        // 5. Materialize nodes for visible commits
        const { nodes: commitNodes } = nodeMaterializer.synchronize(entries, commits);

        // 6. Build links for materialized subgraph only
        const materializedHashes = new Set(commitNodes.map(n => n.hash));
        const commitLinks = buildLinksForMaterialized(commits, materializedHashes);

        // 7. Reconcile branch/tag nodes (only for branches pointing at materialized commits)
        const existingBranchNodes = new Map();
        const existingTagNodes = new Map();
        for (const node of nodes) {
            if (node.type === "branch" && node.branch) {
                existingBranchNodes.set(node.branch, node);
            } else if (node.type === "tag" && node.tag) {
                existingTagNodes.set(node.tag, node);
            }
        }

        const commitNodeByHash = new Map(commitNodes.map(n => [n.hash, n]));
        const branchReconciliation = reconcileBranchNodes(
            existingBranchNodes,
            branches,
            commitNodeByHash,
        );
        const tagReconciliation = reconcileTagNodes(
            existingTagNodes,
            state.tags,
            commitNodeByHash,
        );

        // 8. Assemble state.nodes and state.links
            const allNodes = [
                ...commitNodes,
                ...(showRefDecorators ? branchReconciliation.nodes : []),
                ...(showRefDecorators ? tagReconciliation.nodes : []),
            ];
            const allLinks = [
                ...commitLinks,
                ...(showRefDecorators ? branchReconciliation.links : []),
                ...(showRefDecorators ? tagReconciliation.links : []),
            ];

        nodes.splice(0, nodes.length, ...allNodes);
        links.splice(0, links.length, ...allLinks);

        if (dragState && !dragState.laneDrag && !nodes.includes(dragState.node)) {
            releaseDrag();
        }

        const currentTarget = tooltipManager.getTargetData();
        if (currentTarget && !nodes.includes(currentTarget)) {
            hideTooltip();
        }

        // Snap decorators AFTER materialization so positions are set
        if (showRefDecorators) {
            snapBranchesToTargets(branchReconciliation.alignments);
            snapTagsToTargets(tagReconciliation.alignments);
        }

        // Inject ghost merge node
        injectGhostMergeNode(commitNodeByHash);

        // Apply dimming
        applyDimmingFromPredicate();

        // Center on latest commit if auto-centering is requested
        if (layoutStrategy.shouldAutoCenter()) {
            centerOnLatestCommit();
        }
    }

    /**
     * Re-materializes nodes when viewport changes during scroll/zoom in lazy mode.
     * Only rebuilds the commit node set and links; does NOT re-run layout.
     *
     * @param {import("./state/commitIndex.js").CommitEntry[]} entries Visible entries from ViewportWindow.
     */
    function rematerializeFromViewport(entries) {
        lastViewportEntryCount = entries.length;
        const hashesToHydrate = [];
        for (const entry of entries) {
            hashesToHydrate.push(entry.hash);
            const commit = commits.get(entry.hash);
            for (const parent of commit?.parents ?? []) {
                hashesToHydrate.push(parent);
            }
        }
        enqueueHydration(hashesToHydrate);

        const { nodes: commitNodes, added, removed } = nodeMaterializer.synchronize(entries, commits);
        if (added.length === 0 && removed.length === 0) return;

        // Rebuild links for new materialized set
        const materializedHashes = new Set(commitNodes.map(n => n.hash));
        const commitLinks = buildLinksForMaterialized(commits, materializedHashes);

        // Reconcile branch/tag nodes for currently-materialized commits
        const existingBranchNodes = new Map();
        const existingTagNodes = new Map();
        for (const node of nodes) {
            if (node.type === "branch" && node.branch) {
                existingBranchNodes.set(node.branch, node);
            } else if (node.type === "tag" && node.tag) {
                existingTagNodes.set(node.tag, node);
            }
        }

        const commitNodeByHash = new Map(commitNodes.map(n => [n.hash, n]));
        const branchReconciliation = reconcileBranchNodes(
            existingBranchNodes,
            branches,
            commitNodeByHash,
        );
        const tagReconciliation = reconcileTagNodes(
            existingTagNodes,
            state.tags,
            commitNodeByHash,
        );

        // Assemble
        const allNodes = [
            ...commitNodes,
            ...(showRefDecorators ? branchReconciliation.nodes : []),
            ...(showRefDecorators ? tagReconciliation.nodes : []),
        ];
        const allLinks = [
            ...commitLinks,
            ...(showRefDecorators ? branchReconciliation.links : []),
            ...(showRefDecorators ? tagReconciliation.links : []),
        ];

        nodes.splice(0, nodes.length, ...allNodes);
        links.splice(0, links.length, ...allLinks);

        // Snap decorators
        if (showRefDecorators) {
            snapBranchesToTargets(branchReconciliation.alignments);
            snapTagsToTargets(tagReconciliation.alignments);
        }

        // Inject ghost merge node
        injectGhostMergeNode(commitNodeByHash);

        // Re-apply dimming for new nodes
        applyDimmingFromPredicate();
    }

    /**
     * Injects ghost merge node into the nodes/links arrays if mergePreview is active.
     *
     * @param {Map<string, Object>} commitNodeByHash Map of hash → commit node.
     */
    function injectGhostMergeNode(commitNodeByHash) {
        if (!state.mergePreview) return;

        const mp = state.mergePreview;
        const oursNode = commitNodeByHash.get(mp.oursHash);
        const theirsNode = commitNodeByHash.get(mp.theirsHash);
        if (!oursNode || !theirsNode) return;

        const useLane = state.layoutMode === "lane" &&
            oursNode.laneIndex !== undefined;
        let ghostX, ghostY;
        if (useLane) {
            ghostX = oursNode.x;
            const commitYs = nodes
                .filter(n => n.type === "commit")
                .map(n => n.y)
                .sort((a, b) => a - b);
            const minY = commitYs[0] ?? oursNode.y;
            const step = commitYs.length >= 2
                ? (commitYs[commitYs.length - 1] - commitYs[0]) /
                  (commitYs.length - 1)
                : LANE_VERTICAL_STEP;
            ghostY = minY - step;
        } else {
            ghostX = (oursNode.x + theirsNode.x) / 2;
            ghostY = Math.min(oursNode.y, theirsNode.y) - 60;
        }
        const ghostNode = {
            type: "ghost-merge",
            hash: "__ghost_merge__",
            x: ghostX,
            y: ghostY,
            fx: ghostX,
            fy: ghostY,
        };
        if (useLane) {
            ghostNode.laneIndex = oursNode.laneIndex;
            ghostNode.laneColor = oursNode.laneColor;
        }
        nodes.push(ghostNode);
        links.push({ source: ghostNode, target: oursNode, kind: "ghost" });
        links.push({ source: ghostNode, target: theirsNode, kind: "ghost" });
    }

    /**
     * Creates a new commit node at a best-guess position.
     *
     * @param {string} hash Commit hash.
     * @param {{x: number, y: number}|null} anchorPos Parent node position.
     * @param {{x: number, y: number}|null} [awayFrom] Grandparent position — new node
     *   spawns at LINK_DISTANCE from anchorPos in the direction away from awayFrom.
     */
    function createCommitNode(hash, anchorPos, awayFrom) {
        // Use saved positions from a prior layout (e.g. lane → force switch)
        const saved = _savedPositions?.get(hash);
        if (saved) {
            return {
                type: "commit",
                hash,
                x: saved.x,
                y: saved.y,
                vx: 0,
                vy: 0,
            };
        }

        if (anchorPos) {
            if (awayFrom) {
                const dx = anchorPos.x - awayFrom.x;
                const dy = anchorPos.y - awayFrom.y;
                const dist = Math.hypot(dx, dy);
                if (dist > 1) {
                    const scale = LINK_DISTANCE / dist;
                    return {
                        type: "commit",
                        hash,
                        x: anchorPos.x + dx * scale + jitter(4),
                        y: anchorPos.y + dy * scale + jitter(4),
                        vx: 0,
                        vy: 0,
                    };
                }
            }
            return {
                type: "commit",
                hash,
                x: anchorPos.x + jitter(6),
                y: anchorPos.y + jitter(6),
                vx: 0,
                vy: 0,
            };
        }

        const centerX = (viewportWidth || canvas.width) / 2;
        const centerY = (viewportHeight || canvas.height) / 2;
        const maxRadius =
            Math.min(
                viewportWidth || canvas.width,
                viewportHeight || canvas.height,
            ) * 0.18;
        const radius = Math.random() * maxRadius;
        const angle = Math.random() * Math.PI * 2;

        return {
            type: "commit",
            hash,
            x: centerX + Math.cos(angle) * radius + jitter(35),
            y: centerY + Math.sin(angle) * radius + jitter(35),
            vx: 0,
            vy: 0,
        };
    }

    function snapBranchesToTargets(pairs) {
        // Track per-commit branch count for stacking in lane mode
        const perCommitCount = new Map();

        for (const pair of pairs) {
            if (!pair) continue;
            const { branchNode, targetNode } = pair;
            if (!branchNode || !targetNode) {
                continue;
            }

            if (state.layoutMode === "lane") {
                // In lane mode, center pills below the commit to stay
                // within the lane column and avoid overlapping headers.
                const key = targetNode.hash ?? (targetNode.laneIndex ?? 0);
                const index = perCommitCount.get(key) || 0;
                perCommitCount.set(key, index + 1);

                branchNode.x = targetNode.x;
                branchNode.y = targetNode.y + NODE_RADIUS + 14 + index * 25;
                branchNode.maxPillWidth = LANE_WIDTH - 12;
            } else {
                // Force mode: small jitter is fine since simulation will settle
                const baseX = targetNode.x ?? 0;
                const baseY = targetNode.y ?? 0;
                const jitter = (range) => (Math.random() - 0.5) * range;
                branchNode.x = baseX - BRANCH_NODE_OFFSET_X + jitter(2);
                branchNode.y = baseY + jitter(BRANCH_NODE_OFFSET_Y);
            }

            branchNode.vx = 0;
            branchNode.vy = 0;
        }
    }

    function createBranchNode(branchName, targetNode) {
        if (targetNode) {
            return {
                type: "branch",
                branch: branchName,
                targetHash: targetNode.hash ?? null,
                x: (targetNode.x ?? 0) - BRANCH_NODE_OFFSET_X + jitter(4),
                y: (targetNode.y ?? 0) + jitter(BRANCH_NODE_OFFSET_Y),
                vx: 0,
                vy: 0,
            };
        }

        const baseX = (viewportWidth || canvas.width) / 2;
        const baseY = (viewportHeight || canvas.height) / 2;

        return {
            type: "branch",
            branch: branchName,
            targetHash: null,
            x: baseX - BRANCH_NODE_OFFSET_X + jitter(6),
            y: baseY + jitter(BRANCH_NODE_OFFSET_Y),
            vx: 0,
            vy: 0,
        };
    }

    function createTagNode(tagName, targetNode) {
        if (targetNode) {
            return {
                type: "tag",
                tag: tagName,
                targetHash: targetNode.hash ?? null,
                x: (targetNode.x ?? 0) + TAG_NODE_OFFSET_X + jitter(4),
                y: (targetNode.y ?? 0) + jitter(TAG_NODE_OFFSET_Y),
                vx: 0,
                vy: 0,
            };
        }

        const baseX = (viewportWidth || canvas.width) / 2;
        const baseY = (viewportHeight || canvas.height) / 2;

        return {
            type: "tag",
            tag: tagName,
            targetHash: null,
            x: baseX + TAG_NODE_OFFSET_X + jitter(6),
            y: baseY + jitter(TAG_NODE_OFFSET_Y),
            vx: 0,
            vy: 0,
        };
    }

    /**
     * Re-snaps branch and tag decorator nodes to their target commits
     * after a lane swap. Used during lane drag to keep decorators aligned.
     */
    function snapDecoratorNodesForLaneDrag() {
        const commitMap = new Map();
        for (const n of nodes) {
            if (n.type === "commit") commitMap.set(n.hash, n);
        }
        // Pass 1: position branch pills below commits
        const bCount = new Map();
        for (const n of nodes) {
            if (n.type !== "branch" || !n.targetHash) continue;
            const t = commitMap.get(n.targetHash);
            if (!t) continue;
            const i = bCount.get(t.hash) || 0;
            bCount.set(t.hash, i + 1);
            n.x = t.x;
            n.y = t.y + NODE_RADIUS + 14 + i * 25;
            n.maxPillWidth = LANE_WIDTH - 12;
        }
        // Pass 2: position tags below branch pills
        const tCount = new Map();
        for (const n of nodes) {
            if (n.type !== "tag" || !n.targetHash) continue;
            const t = commitMap.get(n.targetHash);
            if (!t) continue;
            const bc = bCount.get(t.hash) || 0;
            const i = tCount.get(t.hash) || 0;
            tCount.set(t.hash, i + 1);
            n.x = t.x;
            n.y = t.y + NODE_RADIUS + 14 + bc * 25 + (bc > 0 ? 4 : 0) + i * 25;
        }
        // Pass 3: snap ghost merge node to ours lane
        if (state.mergePreview) {
            const ours = commitMap.get(state.mergePreview.oursHash);
            if (ours) {
                for (const n of nodes) {
                    if (n.type !== "ghost-merge") continue;
                    n.x = ours.x;
                    n.fx = ours.x;
                    n.laneIndex = ours.laneIndex;
                    n.laneColor = ours.laneColor;
                    break;
                }
            }
        }
    }

    function snapTagsToTargets(pairs) {
        // Track per-commit tag count for stacking in lane mode
        const perCommitCount = new Map();

        // Count branch pills per commit so tags stack below them
        let branchesPerCommit = null;
        if (state.layoutMode === "lane") {
            branchesPerCommit = new Map();
            for (const n of nodes) {
                if (n.type === "branch" && n.targetHash) {
                    branchesPerCommit.set(n.targetHash, (branchesPerCommit.get(n.targetHash) || 0) + 1);
                }
            }
        }

        for (const pair of pairs) {
            if (!pair) continue;
            const { tagNode, targetNode } = pair;
            if (!tagNode || !targetNode) {
                continue;
            }

            if (state.layoutMode === "lane") {
                const key = targetNode.hash ?? (targetNode.laneIndex ?? 0);
                const index = perCommitCount.get(key) || 0;
                perCommitCount.set(key, index + 1);

                const bc = branchesPerCommit.get(targetNode.hash) || 0;
                const branchStackH = bc * 25 + (bc > 0 ? 4 : 0);
                tagNode.x = targetNode.x;
                tagNode.y = targetNode.y + NODE_RADIUS + 14 + branchStackH + index * 25;
            } else {
                // Force mode: small jitter offset from target commit
                const baseX = targetNode.x ?? 0;
                const baseY = targetNode.y ?? 0;
                const j = (range) => (Math.random() - 0.5) * range;
                tagNode.x = baseX + TAG_NODE_OFFSET_X + j(2);
                tagNode.y = baseY + j(TAG_NODE_OFFSET_Y);
            }

            tagNode.vx = 0;
            tagNode.vy = 0;
        }
    }

    function render() {
        if (rafId !== null) return;
        rafId = requestAnimationFrame(() => {
            rafId = null;
            renderCount++;
            // Advance dim transitions before rendering so the renderer sees
            // the updated dimPhase values for smooth opacity animation.
            const dimTransitionsActive = advanceDimTransitions();
            renderer.render({
                nodes,
                links,
                zoomTransform,
                viewportWidth,
                viewportHeight,
                tooltipManager,
                headHash: state.headHash,
                hoverNode: state.hoverNode,
                tags: state.tags,
                layoutMode: state.layoutMode,
                laneInfo: state.layoutMode === "lane" ? laneStrategy.getLaneInfo() : [],
                mergePreview: state.mergePreview,
            });
            // Notify minimap after every main canvas render.
            if (minimapCallback) {
                minimapCallback();
            }
            // Keep the animation loop alive while transitions are in flight,
            // even if the D3 simulation has cooled.
            if (dimTransitionsActive) {
                render();
            }
        });
    }

    function tick() {
        render();
    }

    function destroy() {
        if (hydrationFlushTimer !== null) {
            clearTimeout(hydrationFlushTimer);
            hydrationFlushTimer = null;
        }
        if (rafId !== null) {
            cancelAnimationFrame(rafId);
            rafId = null;
        }
        if (hoverRafId !== null) {
            cancelAnimationFrame(hoverRafId);
            hoverRafId = null;
        }
        window.removeEventListener("resize", resize);
        resizeObserver?.disconnect();
        select(canvas).on(".zoom", null);
        // Deactivate both strategies and clean up lazy loading
        forceStrategy.deactivate();
        laneStrategy.deactivate();
        nodeMaterializer.clear();
        clearSearchHydrationScan();
        lazyLoadingActive = false;
        removeThemeWatcher?.();
        themeAttrObserver.disconnect();
        releaseDrag();
        canvas.removeEventListener("pointerdown", pointerHandlers.down);
        canvas.removeEventListener("pointermove", pointerHandlers.move);
        canvas.removeEventListener("pointerup", pointerHandlers.up);
        canvas.removeEventListener("pointercancel", pointerHandlers.cancel);
        if (showControls) {
            controls.remove();
        }
        tooltipManager.destroy();
    }

    function applyDelta(delta) {
        if (!delta) {
            return;
        }

        for (const commit of delta.addedCommits || []) {
            if (commit?.hash) {
                const existing = commits.get(commit.hash);
                commits.set(commit.hash, mergeCommitData(existing, commit));
            }
        }
        for (const commit of delta.deletedCommits || []) {
            if (commit?.hash) {
                commits.delete(commit.hash);
            }
        }

        for (const [name, hash] of Object.entries(delta.addedBranches || {})) {
            if (name && hash) {
                branches.set(name, hash);
            }
        }
        for (const [name, hash] of Object.entries(
            delta.amendedBranches || {},
        )) {
            if (name && hash) {
                branches.set(name, hash);
            }
        }
        for (const name of Object.keys(delta.deletedBranches || {})) {
            branches.delete(name);
        }

        // Sync HEAD, tags, and stashes from every delta.
        if (delta.headHash) {
            state.headHash = delta.headHash;
            enqueueHydration([delta.headHash]);
        }
        if (delta.tags) {
            state.tags = new Map(Object.entries(delta.tags));
        }
        if (Array.isArray(delta.stashes)) {
            state.stashes = delta.stashes;
        }
        updateForceButtonAvailability();
        if (state.layoutMode === "force" && commits.size >= FORCE_MODE_MAX_COMMITS) {
            switchLayout("lane");
        }

        // Bootstrap deltas arrive in batches. Applying layout reconciliation on
        // every batch is too expensive for very large repositories (tens of
        // thousands of commits), so ingest commits incrementally and build the
        // graph once on bootstrapComplete.
        if (delta.bootstrap && !delta.bootstrapComplete) {
            return;
        }

        updateGraph();
    }

    function applySummary(summary) {
        if (!summary || !Array.isArray(summary.skeleton)) {
            return;
        }

        const previousCommits = new Map(commits);
        commits.clear();
        branches.clear();
        state.tags = new Map();
        state.stashes = [];
        nodes.splice(0, nodes.length);
        links.splice(0, links.length);
        nodeMaterializer.clear();
        sortedCommitCache = null;
        searchResultCache = null;

        for (const item of summary.skeleton) {
            const hash = item?.hash ?? item?.h;
            if (!hash) continue;
            const parents = Array.isArray(item?.parents) ? item.parents : (Array.isArray(item?.p) ? item.p : []);
            const unix = Number.isFinite(item?.timestamp) ? item.timestamp : item?.t;
            const when = Number.isFinite(unix) && unix > 0 ? new Date(unix * 1000).toISOString() : "";

            const skeletonCommit = {
                hash,
                parents,
                author: { when },
                committer: { when },
                branchLabel: item?.branchLabel || "",
                branchLabelSource: item?.branchLabelSource || "",
            };
            const existing = previousCommits.get(hash);
            commits.set(hash, mergeCommitData(existing, skeletonCommit));
        }

        // If a stale summary omits commits we've already materialized,
        // keep them until a future delta/snapshot reconciles explicitly.
        for (const [hash, commit] of previousCommits.entries()) {
            if (!commits.has(hash)) {
                commits.set(hash, commit);
            }
        }

        for (const [name, hash] of Object.entries(summary.branches || {})) {
            if (name && hash) {
                branches.set(name, hash);
            }
        }

        state.tags = new Map(Object.entries(summary.tags || {}));
        state.stashes = Array.isArray(summary.stashes) ? summary.stashes : [];
        state.headHash = summary.headHash || "";
        updateForceButtonAvailability();

        if (state.layoutMode === "force" && commits.size >= FORCE_MODE_MAX_COMMITS) {
            switchLayout("lane");
            return;
        }
        updateGraph();
    }

    // Activate the layout strategy with initial empty state
    layoutStrategy.activate(
        nodes,
        links,
        commits,
        branches,
        { width: viewportWidth, height: viewportHeight },
    );

    return {
        applyDelta,
        applySummary,
        destroy,
        /**
         * Centers the viewport on the commit with the given hash.
         * Falls back to the latest commit when the hash is not found.
         *
         * @param {string | null} hash Commit hash to center on.
         */
        centerOnCommit,
        /**
         * Moves the selection one step in chronological order.
         *
         * @param {'prev' | 'next'} direction 'prev' = older, 'next' = newer.
         */
        navigateCommits,
        /**
         * Moves the selection to the next/previous search result (N / Shift+N).
         * Returns { index, total } for badge updates, or null if no active search.
         *
         * @param {'next' | 'prev'} direction
         * @returns {{ index: number, total: number } | null}
         */
        navigateSearchResults,
        /**
         * Selects the commit with the given hash, shows its tooltip, fires
         * onCommitSelect, and centers the viewport on it.
         *
         * @param {string} hash Commit hash to select.
         */
        selectAndCenter: selectAndCenterCommit,
        /** Returns the current HEAD hash or null when unknown. */
        getHeadHash: () => state.headHash || null,
        /**
         * Updates the tracked HEAD commit hash.
         * Called by app.js from the onHead WebSocket callback so the HEAD button
         * and G→H shortcut always refer to the correct commit.
         *
         * @param {string | null} hash Current HEAD commit hash.
         */
        setHeadHash: (hash) => {
            state.headHash = hash || "";
            if (state.headHash) {
                enqueueHydration([state.headHash]);
            }
        },
        /**
         * Updates the active structured search state and immediately re-applies
         * the compound filter predicate (A2 search + A3 structural filters).
         * Pass null to clear the search while keeping other filters active.
         *
         * The searchState object is produced by search.js from searchQuery.js
         * and carries both the parsed query and the compiled matcher function.
         *
         * @param {{ query: import("./types.js").SearchQuery, matcher: ((commit: import("./types.js").GraphCommit) => boolean) | null } | null} searchState
         */
        setSearchState: (searchState) => {
            state.searchState = searchState ?? null;
            searchResultCache = null;
            searchResultIndex = -1;
            queueSearchHydration(state.searchState);
            rebuildAndApplyPredicate();
        },
        /**
         * Returns the number of commit nodes currently NOT dimmed (i.e. passing
         * all active search and structural filters).  Used by the search UI to
         * display "N / M" result counts.
         *
         * @returns {{ matching: number, total: number }}
         */
        getCommitCount: () => {
            if (lazyLoadingActive) {
                // In lazy mode, count against all commits via commitIndex
                const allTotal = commitIndex.size;
                if (!state.filterPredicate) {
                    return { matching: allTotal, total: allTotal, pendingHydration: 0 };
                }
                // Evaluate predicate against lightweight objects from commitIndex
                let matching = 0;
                let pendingHydration = 0;
                for (const entry of commitIndex.getAllEntries()) {
                    const commit = commits.get(entry.hash);
                    if (commitNeedsHydration(commit)) pendingHydration++;
                    const pseudoNode = {
                        type: "commit",
                        hash: entry.hash,
                        commit,
                        isStash: entry.isStash,
                        isStashInternal: entry.isStashInternal,
                        stashInternalKind: entry.stashInternalKind,
                    };
                    if (state.filterPredicate(pseudoNode)) matching++;
                }
                return { matching, total: allTotal, pendingHydration };
            }
            let total = 0;
            let matching = 0;
            for (const node of nodes) {
                if (node.type !== "commit") continue;
                total++;
                if (!node.dimmed) matching++;
            }
            return { matching, total, pendingHydration: 0 };
        },
        /**
         * Updates the A3 structural filter state and immediately re-applies the
         * compound predicate.  Called by graphFilters.js onChange callback.
         *
         * @param {{ hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }} filterState
         */
        setFilterState: (filterState) => {
            state.filterState = { ...filterState };
            searchResultCache = null;
            searchResultIndex = -1;
            rebuildAndApplyPredicate();
        },
        /**
         * Returns the live branches Map so the filter panel can populate its
         * branch-focus dropdown with current branch names without going through
         * a full delta cycle.
         *
         * @returns {Map<string, string>} branch-name → commit-hash map.
         */
        getBranches: () => branches,
        /**
         * Provides access to the live commits Map for external components such
         * as the search component that need to scan commit data without going
         * through the full applyDelta pathway.
         *
         * @returns {Map<string, import("./types.js").GraphCommit>}
         */
        getCommits: () => commits,
        /**
         * Returns the live tags Map for external components (e.g. analytics)
         * that need tag-to-commit mappings.
         *
         * @returns {Map<string, string>} tag-name → commit-hash map.
         */
        getTags: () => state.tags,
        /**
         * Clears the lane isolation filter (if active) and re-applies the
         * compound predicate.  Called by the global Escape handler.
         *
         * @returns {boolean} True if isolation was active and cleared.
         */
        clearIsolation: () => {
            if (state.isolatedLanePosition === null) return false;
            state.isolatedLanePosition = null;
            rebuildAndApplyPredicate();
            return true;
        },
        /**
         * Sets or clears the merge preview ghost node on the graph.
         * When preview is non-null, a ghost diamond node appears between the
         * two branch tips. Pass null to remove it.
         *
         * @param {{ oursHash: string, theirsHash: string, mergeBaseHash: string } | null} preview
         */
        setMergePreview: (preview) => {
            state.mergePreview = preview || null;
            updateGraph();
            render();
        },
        /** Returns the live nodes array for the minimap. */
        getNodes: () => nodes,
        /** Returns the live links array for the minimap. */
        getLinks: () => links,
        /** Returns the current D3 zoom transform. */
        getZoomTransform: () => zoomTransform,
        /** Returns current viewport dimensions. */
        getViewport: () => ({ width: viewportWidth, height: viewportHeight }),
        /**
         * Returns navigation position info for the breadcrumb bar.
         * @returns {{ selectedHash: string|null, headHash: string|null, index: number, total: number, layoutMode: string }}
         */
        getNavigationPosition: () => {
            if (!sortedCommitCache) {
                if (lazyLoadingActive) {
                    const allEntries = commitIndex.getAllEntries();
                    sortedCommitCache = allEntries
                        .map(e => ({ hash: e.hash, commit: commits.get(e.hash) }))
                        .filter(e => e.commit)
                        .sort((a, b) => {
                            const aTime = getCommitTimestamp(a.commit);
                            const bTime = getCommitTimestamp(b.commit);
                            if (aTime === bTime) return a.hash.localeCompare(b.hash);
                            return bTime - aTime;
                        });
                } else {
                    sortedCommitCache = nodes
                        .filter((n) => n.type === "commit")
                        .sort((a, b) => {
                            const aTime = getCommitTimestamp(a.commit);
                            const bTime = getCommitTimestamp(b.commit);
                            if (aTime === bTime) return a.hash.localeCompare(b.hash);
                            return bTime - aTime;
                        });
                }
            }
            const total = sortedCommitCache.length;
            let index = -1;
            if (selectedHash) {
                index = sortedCommitCache.findIndex((n) => n.hash === selectedHash);
            }
            return {
                selectedHash,
                headHash: state.headHash || null,
                index: index >= 0 ? index + 1 : -1,
                total,
                layoutMode: state.layoutMode,
            };
        },
        /**
         * Registers a callback invoked after every render for minimap updates.
         * @param {Function} cb
         */
        setMinimapCallback: (cb) => {
            minimapCallback = cb;
        },
        getTelemetrySnapshot: () => ({
            layoutMode: state.layoutMode,
            commitsCount: commits.size,
            commitIndexSize: commitIndex.size,
            materializedCommits: nodeMaterializer.getMaterializedNodes().length,
            viewportEntries: lastViewportEntryCount,
            hydrationPending: hydrationPending.size,
            hydrationInflight: hydrationInflight.size,
            hydrationFetched,
            hydrationErrors,
            nodesCount: nodes.length,
            linksCount: links.length,
            renderCount,
        }),
        /**
         * Updates graph settings (physics + scope), persists to localStorage,
         * and applies changes live.
         * @param {{ scope?: object, physics?: object }} settings
         */
        setGraphSettings: (settings) => {
            if (settings.physics) {
                state.graphSettings.physics = { ...state.graphSettings.physics, ...settings.physics };
                if (state.layoutMode === "force") {
                    forceStrategy.applyPhysics(state.graphSettings.physics);
                }
            }
            if (settings.scope) {
                state.graphSettings.scope = { ...state.graphSettings.scope, ...settings.scope };
                rebuildAndApplyPredicate();
            }
            saveSettings(state.graphSettings);
        },
        /** Re-reads CSS theme variables and re-renders the graph canvas. */
        refreshPalette: () => {
            refreshPalette();
        },
        /** Recomputes canvas size from layout and re-renders. */
        refreshViewport: () => {
            resize();
        },
        /** Rebuilds graph state against current viewport and active layout mode. */
        rebuildVisibleGraph: () => {
            updateGraph();
            render();
        },
        /** Returns current layout mode ("force" or "lane"). */
        getLayoutMode: () => state.layoutMode,
        /** Programmatic access to the zoom behavior for minimap click-to-jump. */
        zoomTo: (x, y) => {
            select(canvas).call(zoom.translateTo, x, y);
        },
    };
}

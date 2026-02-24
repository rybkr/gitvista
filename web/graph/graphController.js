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
    LANE_WIDTH,
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
import { buildPalette } from "./utils/palette.js";
import { getCommitTimestamp } from "./utils/time.js";
import { createGraphState, setZoomTransform } from "./state/graphState.js";

// ── A3 Filter helpers ─────────────────────────────────────────────────────────
//
// These module-level functions are pure and have no dependency on controller
// closure state, making them easy to unit-test and reason about in isolation.

/**
 * Returns true when the given commit node is tracked only by remote-tracking
 * branches (i.e. every branch pointing at it starts with "refs/remotes/").
 *
 * A commit is kept visible if it has at least one local branch pointer OR if no
 * branch points at it at all (orphaned commits stay visible to keep the graph
 * connected and navigable).
 *
 * @param {import("./types.js").GraphNode} node Commit node being evaluated.
 * @param {Map<string, string>} branches Live branch map (name → hash).
 * @returns {boolean} True when the commit is exclusively remote-tracked.
 */
function isExclusivelyRemote(node, branches) {
    if (node.type !== "commit") return false;
    let hasAnyBranch = false;
    let hasLocalBranch = false;
    for (const [name, hash] of branches.entries()) {
        if (hash !== node.hash) continue;
        hasAnyBranch = true;
        if (!name.startsWith("refs/remotes/")) {
            hasLocalBranch = true;
            break;
        }
    }
    // If no branch points at this commit, don't hide it — it could be a
    // reachable ancestor of a visible local branch.
    return hasAnyBranch && !hasLocalBranch;
}

/**
 * Returns true when the given commit node corresponds to a stash entry.
 * Git stores stashes under refs/stash (a single ref pointing to the most
 * recent stash commit) or as reflogs under refs/stash@{n}.  We detect stashes
 * by checking state.stashes first, then falling back to branch-name heuristics.
 *
 * @param {import("./types.js").GraphNode} node Commit node being evaluated.
 * @param {Map<string, string>} branches Live branch map.
 * @param {Array<{hash: string}>} stashes Live stash list from delta.
 * @returns {boolean} True when the commit is a stash entry.
 */
function isStashCommit(node, branches, stashes) {
    if (node.type !== "commit") return false;
    const hash = node.hash;
    // Check explicit stash list from server delta — most reliable path.
    if (Array.isArray(stashes)) {
        for (const stash of stashes) {
            if (stash?.hash === hash) return true;
        }
    }
    // Fallback: detect via stash branch refs.
    for (const [name, targetHash] of branches.entries()) {
        if (targetHash !== hash) continue;
        if (name === "refs/stash" || name.startsWith("stash@{")) return true;
    }
    return false;
}

/**
 * Performs a BFS from the tip commit of a focused branch and returns a Set of
 * all reachable commit hashes.  O(n) in the number of reachable commits;
 * typically < 1 ms for 1000-commit histories on modern hardware.
 *
 * @param {string} branchTipHash Hash the focused branch points at.
 * @param {Map<string, import("./types.js").GraphCommit>} commits All known commits.
 * @returns {Set<string>} Hashes of all commits reachable from the tip (inclusive).
 */
function getReachableCommits(branchTipHash, commits) {
    const reachable = new Set();
    const queue = [branchTipHash];
    while (queue.length > 0) {
        const hash = queue.pop();
        if (!hash || reachable.has(hash)) continue;
        reachable.add(hash);
        const commit = commits.get(hash);
        if (!commit) continue;
        for (const parent of commit.parents ?? []) {
            if (!reachable.has(parent)) {
                queue.push(parent);
            }
        }
    }
    return reachable;
}

/**
 * Builds a compound predicate that combines the structured search matcher with
 * the A3 structural filters.  Returns null when no criteria are active, which
 * lets the dimming pass be skipped entirely on the common (no filter) path.
 *
 * AND semantics: a node must pass every active criterion to remain fully
 * visible.  Failing nodes are dimmed (not removed) so the graph stays
 * connected and the user can still orient themselves.
 *
 * @param {{ query: import("./types.js").SearchQuery, matcher: ((commit: import("./types.js").GraphCommit) => boolean) | null } | null} searchState Structured search state from search.js.
 * @param {{ hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }} filterState
 * @param {Map<string, string>} branches Live branch map.
 * @param {Map<string, import("./types.js").GraphCommit>} commits All known commits.
 * @param {Array<{hash: string}>} stashes Live stash list from delta.
 * @param {number|null} isolatedLanePosition When non-null, only commits at this lane position pass.
 * @param {Array<Object>} segments Segments array from laneStrategy (used for isolation).
 * @returns {((node: import("./types.js").GraphNode) => boolean) | null}
 */
function buildFilterPredicate(searchState, filterState, branches, commits, stashes, isolatedLanePosition, segments) {
    // The matcher is null when the query is empty (parseSearchQuery sets isEmpty).
    const matcher = searchState?.matcher ?? null;
    const hasSearch = matcher !== null;
    const { hideRemotes, hideMerges, hideStashes, focusBranch } = filterState;
    const hasIsolation = isolatedLanePosition !== null && isolatedLanePosition !== undefined;
    const hasAnyFilter = hideRemotes || hideMerges || hideStashes || !!focusBranch || hasIsolation;

    // Short-circuit: nothing active → caller skips the dimming loop entirely.
    if (!hasSearch && !hasAnyFilter) return null;

    // Pre-compute isolated hashes set from segments at the given position.
    let isolatedHashes = null;
    if (hasIsolation && Array.isArray(segments)) {
        isolatedHashes = new Set();
        for (const seg of segments) {
            if (seg.position === isolatedLanePosition) {
                for (const h of seg.hashes) isolatedHashes.add(h);
            }
        }
    }

    // Pre-compute the reachable set once via BFS (not per-node).
    let reachableSet = null;
    if (focusBranch) {
        const tipHash = branches.get(focusBranch);
        reachableSet = tipHash ? getReachableCommits(tipHash, commits) : new Set();
    }

    return (node) => {
        // Branch label nodes: only apply the remote-ref filter; all other
        // filters operate on commit data which branch nodes don't carry.
        if (node.type === "branch") {
            if (hideRemotes && node.branch?.startsWith("refs/remotes/")) return false;
            return true;
        }

        // Tag nodes always pass — they have no commit data to filter on.
        if (node.type === "tag") {
            return true;
        }

        // ── A2: structured search matcher ─────────────────────────────────────
        // Delegates all field matching (text, author, hash, date, merge, branch)
        // to the compiled predicate from searchQuery.js.
        if (hasSearch) {
            const commit = node.commit;
            if (!commit) return false;
            if (!matcher(commit)) return false;
        }

        // ── A3: structural filters ────────────────────────────────────────────
        if (hideRemotes && isExclusivelyRemote(node, branches)) return false;
        if (hideMerges && (node.commit?.parents?.length ?? 0) > 1) return false;
        if (hideStashes && isStashCommit(node, branches, stashes)) return false;
        if (reachableSet !== null && !reachableSet.has(node.hash)) return false;

        // Lane isolation: only commits in segments at the isolated position pass.
        if (isolatedHashes !== null && !isolatedHashes.has(node.hash)) return false;

        return true;
    };
}

/**
 * Creates and initializes the graph controller instance.
 *
 * @param {HTMLElement} rootElement DOM node that hosts the canvas.
 * @param {{
 *   onCommitTreeClick?: (commit: import("./types.js").GraphCommit) => void,
 *   onCommitSelect?: (hash: string | null) => void,
 * }} [options] Optional callbacks.
 * @returns {{
 *   applyDelta(delta: unknown): void,
 *   centerOnCommit(hash: string): void,
 *   navigateCommits(direction: 'prev' | 'next'): void,
 *   destroy(): void,
 * }} Public graph API.
 */
/** Returns a random value in the range [-range/2, range/2]. */
const jitter = (range) => (Math.random() - 0.5) * range;

export function createGraphController(rootElement, options = {}) {
    const canvas = document.createElement("canvas");
    rootElement.appendChild(canvas);

    const state = createGraphState();
    const { commits, branches, nodes, links } = state;

    let zoomTransform = state.zoomTransform;
    let dragState = null;
    let isDraggingNode = false;
    const pointerHandlers = {};

    let viewportWidth = 0;
    let viewportHeight = 0;

    let selectedHash = null;
    let sortedCommitCache = null;
    let rafId = null;

    // Create both layout strategies
    const forceStrategy = new ForceStrategy({
        viewportWidth,
        viewportHeight,
        onTick: tick,
    });
    const laneStrategy = new LaneStrategy({ onTick: tick });

    // Restore layout mode from localStorage, default to "force"
    const STORAGE_KEY_LAYOUT_MODE = "gitvista-layout-mode";
    const savedMode = localStorage.getItem(STORAGE_KEY_LAYOUT_MODE);
    const initialMode = savedMode === "lane" ? "lane" : "force";
    let layoutStrategy = initialMode === "lane" ? laneStrategy : forceStrategy;
    state.layoutMode = initialMode;

    const zoom = d3Zoom()
        .filter((event) => !isDraggingNode || event.type === "wheel")
        .scaleExtent([ZOOM_MIN, ZOOM_MAX])
        .on("zoom", (event) => {
            if (event.sourceEvent) {
                layoutStrategy.disableAutoCenter();
            }
            zoomTransform = event.transform;
            setZoomTransform(state, zoomTransform);
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

    // "Jump to HEAD" button — centers the view on the current HEAD commit.
    const headBtn = document.createElement("button");
    headBtn.textContent = "\u2302 HEAD";
    headBtn.title = "Jump to HEAD commit (G then H)";
    headBtn.addEventListener("click", () => {
        centerOnCommit(state.headHash || null);
    });
    controls.appendChild(headBtn);

    rootElement.appendChild(controls);

    /**
     * Switch between force-directed and lane-based layout modes.
     *
     * @param {"force" | "lane"} newMode The layout mode to switch to.
     */
    const switchLayout = (newMode) => {
        if (state.layoutMode === newMode) {
            return; // Already in this mode
        }

        // Clear lane isolation when switching modes
        state.isolatedLanePosition = null;

        // Deactivate current strategy
        layoutStrategy.deactivate();

        // Switch to new strategy
        if (newMode === "lane") {
            layoutStrategy = laneStrategy;
        } else {
            layoutStrategy = forceStrategy;
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

        // Enable/disable rebalance button based on strategy support
        rebalanceBtn.disabled = !layoutStrategy.supportsRebalance;

        // Activate new strategy with current graph state
        const viewport = { width: viewportWidth, height: viewportHeight };
        layoutStrategy.activate(nodes, links, commits, branches, viewport);

        // Force immediate reposition
        updateGraph();
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
    const renderer = new GraphRenderer(canvas, buildPalette(canvas));

    const updateTooltipPosition = () => {
        tooltipManager.updatePosition(zoomTransform);
    };
    const hideTooltip = () => {
        tooltipManager.hideAll();
        render();
    };
    const showTooltip = (node) => {
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

    const centerOnLatestCommit = () => {
        // Prefer centering on HEAD when available — the "latest commit by
        // timestamp" heuristic used by layout strategies often picks a
        // different commit, leaving the view stranded in the middle of history.
        if (state.headHash) {
            const headNode = nodes.find(
                (n) => n.type === "commit" && n.hash === state.headHash,
            );
            if (headNode) {
                select(canvas).call(zoom.translateTo, headNode.x, headNode.y);
                return;
            }
        }

        const target = layoutStrategy.findCenterTarget(nodes);
        if (target) {
            select(canvas).call(zoom.translateTo, target.x, target.y);
        }
    };

    /**
     * Centers the viewport on the commit node with the given hash.
     * If the hash is null/undefined or not found among current nodes, falls back
     * to centering on the latest (HEAD) commit instead.
     *
     * @param {string | null} hash 40-character commit hash to center on.
     */
    const centerOnCommit = (hash) => {
        if (hash) {
            const target = nodes.find((n) => n.type === "commit" && n.hash === hash);
            if (target) {
                layoutStrategy.disableAutoCenter();
                select(canvas).call(zoom.translateTo, target.x, target.y);
                return;
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
        const node = nodes.find((n) => n.type === "commit" && n.hash === hash);
        if (!node) {
            return;
        }
        selectedHash = hash;
        showTooltip(node);
        options.onCommitSelect?.(hash);
        centerOnCommit(hash);
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
        if (!sortedCommitCache) {
            sortedCommitCache = nodes
                .filter((n) => n.type === "commit")
                .sort((a, b) => {
                    const aTime = getCommitTimestamp(a.commit);
                    const bTime = getCommitTimestamp(b.commit);
                    if (aTime === bTime) return a.hash.localeCompare(b.hash);
                    return bTime - aTime;
                });
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

        // Show/hide tooltip for selection feedback
        const currentTarget = tooltipManager.getTargetData();
        if (tooltipManager.isVisible() && currentTarget === targetNode) {
            hideTooltip();
            // Deselect: clear permalink and selection state.
            selectedHash = null;
            options.onCommitSelect?.(null);
        } else {
            showTooltip(targetNode);
            // Track selection for permalink and keyboard navigation.
            if (targetNode.type === "commit") {
                selectedHash = targetNode.hash;
                options.onCommitSelect?.(targetNode.hash);
            }
        }

        // If clicking on a commit node, also open the file explorer
        if (targetNode.type === "commit" && targetNode.commit && options.onCommitTreeClick) {
            options.onCommitTreeClick(targetNode.commit);
        }

        event.stopImmediatePropagation();
        event.preventDefault();

        isDraggingNode = true;
        dragState = {
            node: targetNode,
            pointerId: event.pointerId,
            startX: x,
            startY: y,
            dragged: false,
        };

        // Delegate initial drag position to layout strategy
        layoutStrategy.handleDrag(targetNode, x, y);

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
                const distance = Math.hypot(
                    x - dragState.startX,
                    y - dragState.startY,
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
                    canvas.title = hit.branchOwner || hit.tipHash || "";
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
            releaseDrag();
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

        for (const commit of commits.values()) {
            const parentNode = (commit.parents ?? [])
                .map((parentHash) => existingNodes.get(parentHash))
                .find((node) => node);
            const node =
                existingNodes.get(commit.hash) ??
                createCommitNode(commit.hash, parentNode);
            node.type = "commit";
            node.hash = commit.hash;
            node.commit = commit;
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
                branchNode.spawnPhase = 0;
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
                tagNode.spawnPhase = 0;
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
    function applyDimmingFromPredicate() {
        // Rebuild predicate so it captures the latest branches/commits/stashes.
        state.filterPredicate = buildFilterPredicate(
            state.searchState,
            state.filterState,
            branches,
            commits,
            state.stashes,
            state.isolatedLanePosition,
            laneStrategy._segments,
        );
        const predicate = state.filterPredicate;
        for (const node of nodes) {
            // null predicate = no active filter → show everything at full opacity.
            node.dimmed = predicate !== null ? !predicate(node) : false;
        }
    }

    /**
     * Main graph update orchestrator. Reconciles nodes and links, updates simulation state,
     * and triggers layout adjustments.
     */
    function updateGraph() {
        sortedCommitCache = null; // Invalidate on every structural update
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

        // Apply the active compound predicate (A2 search + A3 filters) to set
        // node.dimmed.  This runs after reconciliation so newly-created nodes are
        // included, and rebuilds the predicate so BFS data is never stale.
        applyDimmingFromPredicate();

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

        // Center on latest commit if auto-centering is requested
        if (layoutStrategy.shouldAutoCenter()) {
            centerOnLatestCommit();
        }
    }

    function createCommitNode(hash, anchorNode) {
        const centerX = (viewportWidth || canvas.width) / 2;
        const centerY = (viewportHeight || canvas.height) / 2;
        const maxRadius =
            Math.min(
                viewportWidth || canvas.width,
                viewportHeight || canvas.height,
            ) * 0.18;
        const radius = Math.random() * maxRadius;
        const angle = Math.random() * Math.PI * 2;

        if (anchorNode) {
            return {
                type: "commit",
                hash,
                x: anchorNode.x + jitter(6),
                y: anchorNode.y + jitter(6),
                vx: 0,
                vy: 0,
            };
        }

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
                // In lane mode, offset branch pills to the right of the commit
                // so they don't overlap with the centered lane header labels.
                const laneX = targetNode.x;

                // Stack multiple branches on the same commit vertically
                const key = targetNode.hash ?? (targetNode.laneIndex ?? 0);
                const index = perCommitCount.get(key) || 0;
                perCommitCount.set(key, index + 1);

                const stackOffset = index * (BRANCH_NODE_RADIUS * 2.5 + 2);
                branchNode.x = laneX + BRANCH_NODE_OFFSET_X;
                branchNode.y = targetNode.y - BRANCH_NODE_RADIUS - 8 - stackOffset;
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
        const bCount = new Map();
        const tCount = new Map();
        for (const n of nodes) {
            if (n.type === "branch" && n.targetHash) {
                const t = commitMap.get(n.targetHash);
                if (!t) continue;
                const i = bCount.get(t.hash) || 0;
                bCount.set(t.hash, i + 1);
                n.x = t.x + BRANCH_NODE_OFFSET_X;
                n.y = t.y - BRANCH_NODE_RADIUS - 8 - i * (BRANCH_NODE_RADIUS * 2.5 + 2);
            } else if (n.type === "tag" && n.targetHash) {
                const t = commitMap.get(n.targetHash);
                if (!t) continue;
                const i = tCount.get(t.hash) || 0;
                tCount.set(t.hash, i + 1);
                n.x = t.x;
                n.y = t.y + TAG_NODE_RADIUS + 8 + i * (TAG_NODE_RADIUS * 2.5 + 2);
            }
        }
    }

    function snapTagsToTargets(pairs) {
        // Track per-commit tag count for stacking in lane mode
        const perCommitCount = new Map();

        for (const pair of pairs) {
            if (!pair) continue;
            const { tagNode, targetNode } = pair;
            if (!tagNode || !targetNode) {
                continue;
            }

            if (state.layoutMode === "lane") {
                // In lane mode, use commit's X directly (already display-aware)
                const laneX = targetNode.x;

                // Stack multiple tags on the same commit vertically
                const key = targetNode.hash ?? (targetNode.laneIndex ?? 0);
                const index = perCommitCount.get(key) || 0;
                perCommitCount.set(key, index + 1);

                const stackOffset = index * (TAG_NODE_RADIUS * 2.5 + 2);
                tagNode.x = laneX;
                tagNode.y = targetNode.y + TAG_NODE_RADIUS + 8 + stackOffset;
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
            });
        });
    }

    function tick() {
        // Auto-centering is handled in updateGraph now
        if (layoutStrategy.shouldAutoCenter()) {
            centerOnLatestCommit();
        }
        render();
    }

    function destroy() {
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
        // Deactivate both strategies to clean up resources
        forceStrategy.deactivate();
        laneStrategy.deactivate();
        removeThemeWatcher?.();
        themeAttrObserver.disconnect();
        releaseDrag();
        canvas.removeEventListener("pointerdown", pointerHandlers.down);
        canvas.removeEventListener("pointermove", pointerHandlers.move);
        canvas.removeEventListener("pointerup", pointerHandlers.up);
        canvas.removeEventListener("pointercancel", pointerHandlers.cancel);
        controls.remove();
        tooltipManager.destroy();
    }

    function applyDelta(delta) {
        if (!delta) {
            return;
        }

        for (const commit of delta.addedCommits || []) {
            if (commit?.hash) {
                commits.set(commit.hash, commit);
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
        }
        if (delta.tags) {
            state.tags = new Map(Object.entries(delta.tags));
        }
        if (Array.isArray(delta.stashes)) {
            state.stashes = delta.stashes;
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
            let total = 0;
            let matching = 0;
            for (const node of nodes) {
                if (node.type !== "commit") continue;
                total++;
                if (!node.dimmed) matching++;
            }
            return { matching, total };
        },
        /**
         * Updates the A3 structural filter state and immediately re-applies the
         * compound predicate.  Called by graphFilters.js onChange callback.
         *
         * @param {{ hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }} filterState
         */
        setFilterState: (filterState) => {
            state.filterState = { ...filterState };
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
    };
}

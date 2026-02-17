/**
 * @fileoverview Primary controller orchestrating the Git graph visualization.
 * Wires together state, D3 simulation, rendering, tooltips, and interactions.
 */

import * as d3 from "https://cdn.jsdelivr.net/npm/d3@7.9.0/+esm";
import { TooltipManager } from "../tooltips/index.js";
import {
    BRANCH_NODE_OFFSET_X,
    BRANCH_NODE_OFFSET_Y,
    BRANCH_NODE_RADIUS,
    DRAG_ACTIVATION_DISTANCE,
    NODE_RADIUS,
    ZOOM_MAX,
    ZOOM_MIN,
    TREE_ICON_SIZE,
    TREE_ICON_OFFSET,
} from "./constants.js";
import { GraphRenderer } from "./rendering/graphRenderer.js";
import { ForceStrategy } from "./layout/forceStrategy.js";
import { LaneStrategy } from "./layout/laneStrategy.js";
import { buildPalette } from "./utils/palette.js";
import { createGraphState, setZoomTransform } from "./state/graphState.js";

/**
 * Creates and initializes the graph controller instance.
 *
 * @param {HTMLElement} rootElement DOM node that hosts the canvas.
 * @param {{onCommitTreeClick?: (commit: import("./types.js").GraphCommit) => void}} [options] Optional callbacks.
 * @returns {{ applyDelta(delta: unknown): void, destroy(): void }} Public graph API.
 */
export function createGraphController(rootElement, options = {}) {
    const canvas = document.createElement("canvas");
    const context = canvas.getContext("2d", { alpha: false });
    canvas.factor = window.devicePixelRatio || 1;
    rootElement.appendChild(canvas);

    const state = createGraphState();
    const { commits, branches, nodes, links } = state;

    let zoomTransform = state.zoomTransform;
    let dragState = null;
    let isDraggingNode = false;
    const pointerHandlers = {};

    let viewportWidth = 0;
    let viewportHeight = 0;

    // Create both layout strategies
    const forceStrategy = new ForceStrategy({
        viewportWidth,
        viewportHeight,
        onTick: tick,
    });
    const laneStrategy = new LaneStrategy();

    // Restore layout mode from localStorage, default to "force"
    const STORAGE_KEY_LAYOUT_MODE = "gitvista-layout-mode";
    const savedMode = localStorage.getItem(STORAGE_KEY_LAYOUT_MODE);
    const initialMode = savedMode === "lane" ? "lane" : "force";
    let layoutStrategy = initialMode === "lane" ? laneStrategy : forceStrategy;
    state.layoutMode = initialMode;

    const zoom = d3
        .zoom()
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
    };

    // Wire button click handlers
    forceBtn.addEventListener("click", () => switchLayout("force"));
    laneBtn.addEventListener("click", () => switchLayout("lane"));
    rebalanceBtn.addEventListener("click", () => {
        if (layoutStrategy.supportsRebalance) {
            layoutStrategy.rebalance();
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

    /**
     * Hit-tests whether the given graph coordinates land within a commit node's tree icon.
     * Returns the commit node if the icon is clicked, null otherwise.
     *
     * @param {number} x Graph X coordinate.
     * @param {number} y Graph Y coordinate.
     * @returns {import("./types.js").GraphNodeCommit | null} Commit node if tree icon clicked.
     */
    const findTreeIconAt = (x, y) => {
        for (const node of nodes) {
            if (node.type !== "commit" || !node.commit?.tree) {
                continue;
            }

            const iconSize = TREE_ICON_SIZE;
            const offsetX = node.radius + TREE_ICON_OFFSET;
            const offsetY = -(node.radius + TREE_ICON_OFFSET);
            const iconX = node.x + offsetX;
            const iconY = node.y + offsetY;

            // Check if click is within the folder icon bounds (with some padding for easier clicking)
            const padding = 2;
            if (
                x >= iconX - padding &&
                x <= iconX + iconSize + padding &&
                y >= iconY - iconSize * 0.2 - padding &&
                y <= iconY + iconSize * 0.6 + padding
            ) {
                return node;
            }
        }
        return null;
    };

    const centerOnLatestCommit = () => {
        const target = layoutStrategy.findCenterTarget(nodes);
        if (target) {
            // d3.select(canvas).call(...) translates view to center on target coordinates.
            d3.select(canvas).call(zoom.translateTo, target.x, target.y);
        }
    };

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
    };

    const handlePointerDown = (event) => {
        if (event.button !== 0) {
            return;
        }

        const { x, y } = toGraphCoordinates(event);

        const targetNode = findNodeAt(x, y);

        if (!targetNode) {
            hideTooltip();
            return;
        }

        layoutStrategy.disableAutoCenter();

        // Show/hide tooltip for selection feedback
        const currentTarget = tooltipManager.getTargetData();
        if (tooltipManager.isVisible() && currentTarget === targetNode) {
            hideTooltip();
        } else {
            showTooltip(targetNode);
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

    const handlePointerMove = (event) => {
        if (dragState && event.pointerId === dragState.pointerId) {
            event.preventDefault();
            const { x, y } = toGraphCoordinates(event);

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
    };

    const handlePointerUp = (event) => {
        if (dragState && event.pointerId === dragState.pointerId) {
            releaseDrag();
        }
    };

    let palette = buildPalette(canvas);
    let removeThemeWatcher = null;

    d3.select(canvas).call(zoom).on("dblclick.zoom", null);

    const resize = () => {
        const parent = canvas.parentElement;
        const cssWidth =
            (parent?.clientWidth ?? window.innerWidth) || window.innerWidth;
        const cssHeight =
            (parent?.clientHeight ?? window.innerHeight) || window.innerHeight;
        const dpr = window.devicePixelRatio || 1;

        viewportWidth = cssWidth;
        viewportHeight = cssHeight;

        canvas.width = Math.round(cssWidth * dpr);
        canvas.height = Math.round(cssHeight * dpr);
        canvas.style.width = `${cssWidth}px`;
        canvas.style.height = `${cssHeight}px`;

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

    const themeWatcher = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (themeWatcher) {
        const handler = () => {
            palette = buildPalette(canvas);
            renderer.updatePalette(palette);
            render();
        };
        if (themeWatcher.addEventListener) {
            themeWatcher.addEventListener("change", handler);
            removeThemeWatcher = () =>
                themeWatcher.removeEventListener("change", handler);
        } else if (themeWatcher.addListener) {
            themeWatcher.addListener(handler);
            removeThemeWatcher = () => themeWatcher.removeListener(handler);
        }
    }

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

    function updateGraph() {
        const existingCommitNodes = new Map();
        const existingBranchNodes = new Map();

        for (const node of nodes) {
            if (node.type === "branch" && node.branch) {
                existingBranchNodes.set(node.branch, node);
            } else if (node.type === "commit" && node.hash) {
                existingCommitNodes.set(node.hash, node);
            }
        }

        const nextCommitNodes = [];
        let commitStructureChanged = existingCommitNodes.size !== commits.size;
        for (const commit of commits.values()) {
            const parentNode = (commit.parents ?? [])
                .map((parentHash) => existingCommitNodes.get(parentHash))
                .find((node) => node);
            const node =
                existingCommitNodes.get(commit.hash) ??
                createCommitNode(commit.hash, parentNode);
            node.type = "commit";
            node.hash = commit.hash;
            node.commit = commit;
            node.radius = node.radius ?? NODE_RADIUS;
            nextCommitNodes.push(node);
            if (!existingCommitNodes.has(commit.hash)) {
                commitStructureChanged = true;
            }
        }

        const commitHashes = new Set(nextCommitNodes.map((node) => node.hash));
        const previousLinkCount = links.length;
        const nextLinks = [];
        for (const commit of commits.values()) {
            if (!commit?.hash) {
                continue;
            }
            for (const parentHash of commit.parents ?? []) {
                if (!commitHashes.has(parentHash)) {
                    continue;
                }
                nextLinks.push({
                    source: commit.hash,
                    target: parentHash,
                });
            }
        }

        const commitNodeByHash = new Map(
            nextCommitNodes.map((node) => [node.hash, node]),
        );
        const nextBranchNodes = [];
        const pendingBranchAlignments = [];
        let branchStructureChanged = existingBranchNodes.size !== branches.size;
        for (const [branchName, targetHash] of branches.entries()) {
            const targetNode = commitNodeByHash.get(targetHash);
            if (!targetNode) {
                continue;
            }

            let branchNode = existingBranchNodes.get(branchName);
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
                branchStructureChanged = true;
            } else if (previousHash !== targetHash) {
                branchStructureChanged = true;
            }

            nextBranchNodes.push(branchNode);
            nextLinks.push({
                source: branchNode,
                target: targetNode,
                kind: "branch",
            });

            pendingBranchAlignments.push({ branchNode, targetNode });
        }

        nodes.splice(0, nodes.length, ...nextCommitNodes, ...nextBranchNodes);
        links.splice(0, links.length, ...nextLinks);

        if (dragState && !nodes.includes(dragState.node)) {
            releaseDrag();
        }

        const currentTarget = tooltipManager.getTargetData();
        if (currentTarget && !nodes.includes(currentTarget)) {
            hideTooltip();
        }

        // Snap branch nodes to their target commits before updating layout strategy
        snapBranchesToTargets(pendingBranchAlignments);

        const linkStructureChanged = previousLinkCount !== nextLinks.length;
        const structureChanged =
            commitStructureChanged ||
            branchStructureChanged ||
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
        const jitter = () => (Math.random() - 0.5) * 35;

        if (anchorNode) {
            const offsetJitter = () => (Math.random() - 0.5) * 6;
            return {
                type: "commit",
                hash,
                x: anchorNode.x + offsetJitter(),
                y: anchorNode.y + offsetJitter(),
                vx: 0,
                vy: 0,
            };
        }

        return {
            type: "commit",
            hash,
            x: centerX + Math.cos(angle) * radius + jitter(),
            y: centerY + Math.sin(angle) * radius + jitter(),
            vx: 0,
            vy: 0,
        };
    }

    function snapBranchesToTargets(pairs) {
        for (const pair of pairs) {
            if (!pair) continue;
            const { branchNode, targetNode } = pair;
            if (!branchNode || !targetNode) {
                continue;
            }

            const baseX = targetNode.x ?? 0;
            const baseY = targetNode.y ?? 0;
            const jitter = (range) => (Math.random() - 0.5) * range;

            branchNode.x = baseX - BRANCH_NODE_OFFSET_X + jitter(2);
            branchNode.y = baseY + jitter(BRANCH_NODE_OFFSET_Y);
            branchNode.vx = 0;
            branchNode.vy = 0;
        }
    }

    function createBranchNode(branchName, targetNode) {
        if (targetNode) {
            const jitter = (range) => (Math.random() - 0.5) * range;
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
        const jitterFallback = (range) => (Math.random() - 0.5) * range;

        return {
            type: "branch",
            branch: branchName,
            targetHash: null,
            x: baseX - BRANCH_NODE_OFFSET_X + jitterFallback(6),
            y: baseY + jitterFallback(BRANCH_NODE_OFFSET_Y),
            vx: 0,
            vy: 0,
        };
    }

    function render() {
        renderer.render({
            nodes,
            links,
            zoomTransform,
            viewportWidth,
            viewportHeight,
            tooltipManager,
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
        window.removeEventListener("resize", resize);
        resizeObserver?.disconnect();
        d3.select(canvas).on(".zoom", null);
        // Deactivate both strategies to clean up resources
        forceStrategy.deactivate();
        laneStrategy.deactivate();
        removeThemeWatcher?.();
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
    };
}

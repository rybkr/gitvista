/**
 * @fileoverview Primary controller orchestrating the Git graph visualization.
 * Wires together state, D3 simulation, rendering, tooltips, and interactions.
 */

import * as d3 from "https://cdn.jsdelivr.net/npm/d3@7.9.0/+esm";
import { TooltipManager } from "../tooltips/index.js";
import {
    ALPHA_DECAY,
    BRANCH_NODE_OFFSET_X,
    BRANCH_NODE_OFFSET_Y,
    BRANCH_NODE_RADIUS,
    CHARGE_STRENGTH,
    COLLISION_RADIUS,
    DRAG_ACTIVATION_DISTANCE,
    DRAG_ALPHA_TARGET,
    LINK_DISTANCE,
    LINK_STRENGTH,
    NODE_RADIUS,
    VELOCITY_DECAY,
    ZOOM_MAX,
    ZOOM_MIN,
    TREE_ICON_SIZE,
    TREE_ICON_OFFSET,
} from "./constants.js";
import { GraphRenderer } from "./rendering/graphRenderer.js";
import { LayoutManager } from "./layout/layoutManager.js";
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
    let initialLayoutComplete = false;

    let viewportWidth = 0;
    let viewportHeight = 0;

    const simulation = d3
        .forceSimulation(nodes)
        .velocityDecay(VELOCITY_DECAY)
        .alphaDecay(ALPHA_DECAY)
        .force("charge", d3.forceManyBody().strength(CHARGE_STRENGTH))
        .force("collision", d3.forceCollide().radius(COLLISION_RADIUS))
        .force(
            "link",
            d3
                .forceLink(links)
                .id((d) => d.id ?? d.hash)
                .distance(LINK_DISTANCE)
                .strength(LINK_STRENGTH),
        )
        .on("tick", tick);

    const layoutManager = new LayoutManager(
        simulation,
        viewportWidth,
        viewportHeight,
    );

    const zoom = d3
        .zoom()
        .filter((event) => !isDraggingNode || event.type === "wheel")
        .scaleExtent([ZOOM_MIN, ZOOM_MAX])
        .on("zoom", (event) => {
            if (event.sourceEvent) {
                layoutManager.disableAutoCenter();
            }
            zoomTransform = event.transform;
            setZoomTransform(state, zoomTransform);
            render();
        });

    canvas.style.cursor = "default";

    const controls = document.createElement("div");
    controls.className = "graph-controls";

    const rebalanceBtn = document.createElement("button");
    rebalanceBtn.textContent = "Rebalance";
    rebalanceBtn.addEventListener("click", () => {
        for (const node of nodes) {
            node.fx = null;
            node.fy = null;
        }
        simulation.alpha(0.8).restart();
    });
    controls.appendChild(rebalanceBtn);
    rootElement.appendChild(controls);

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
        const latest = layoutManager.findLatestCommit(nodes);
        if (latest) {
            // d3.select(canvas).call(...) translates view to center on target coordinates.
            d3.select(canvas).call(zoom.translateTo, latest.x, latest.y);
        }
    };

    const releaseDrag = () => {
        if (!dragState) {
            return;
        }

        const current = dragState;
        current.node.fx = null;
        current.node.fy = null;
        current.node.vx = 0;
        current.node.vy = 0;

        if (canvas.releasePointerCapture) {
            try {
                canvas.releasePointerCapture(current.pointerId);
            } catch {
                // ignore release failures (pointer already released)
            }
        }

        dragState = null;
        isDraggingNode = false;
        simulation.alphaTarget(0);
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

        layoutManager.disableAutoCenter();

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

        targetNode.fx = x;
        targetNode.fy = y;
        targetNode.vx = 0;
        targetNode.vy = 0;

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
            dragState.node.fx = x;
            dragState.node.fy = y;
            dragState.node.vx = 0;
            dragState.node.vy = 0;
            dragState.node.x = x;
            dragState.node.y = y;

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
                simulation.alphaTarget(DRAG_ALPHA_TARGET).restart();
            }
            render();
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

        canvas.width = physicalWidth;
        canvas.height = physicalHeight;
        canvas.style.width = `${cssWidth}px`;
        canvas.style.height = `${cssHeight}px`;

        layoutManager.updateViewport(cssWidth, cssHeight);
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
                    source: commit.hash,
                    target: parentHash,
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
     * Applies reconciled nodes and links to the D3 simulation and handles layout logic.
     * Manages initial layout, auto-centering, and simulation restart behavior.
     *
     * @param {boolean} structureChanged Whether any nodes or links were added/removed/changed.
     * @param {boolean} initialComplete Whether the initial layout has been completed.
     * @param {boolean} commitStructureChanged Whether commit nodes changed.
     * @param {import("./types.js").GraphNode[]} allNodes Combined commit + branch nodes.
     * @param {Array} allLinks Combined commit + branch links.
     * @param {Array<{branchNode: import("./types.js").GraphNode, targetNode: import("./types.js").GraphNode}>} branchAlignments Pairs for positioning.
     */
    function applySimulationUpdate(
        structureChanged,
        initialComplete,
        commitStructureChanged,
        allNodes,
        allLinks,
        branchAlignments,
    ) {
        const hasCommits = allNodes.some((node) => node.type === "commit");

        if (!initialComplete && hasCommits) {
            layoutManager.applyTimelineLayout(allNodes);
            snapBranchesToTargets(branchAlignments);
            layoutManager.requestAutoCenter();
            centerOnLatestCommit();
            initialLayoutComplete = true;
            layoutManager.restartSimulation(1.0);
        } else {
            snapBranchesToTargets(branchAlignments);
            if (commitStructureChanged) {
                layoutManager.requestAutoCenter();
            }
        }

        layoutManager.boostSimulation(structureChanged);
    }

    /**
     * Main graph update orchestrator. Reconciles nodes and links, updates simulation state,
     * and triggers layout adjustments.
     */
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

        const allNodes = [
            ...commitReconciliation.nodes,
            ...branchReconciliation.nodes,
        ];
        const allLinks = [...commitLinks, ...branchReconciliation.links];

        nodes.splice(0, nodes.length, ...allNodes);
        links.splice(0, links.length, ...allLinks);

        if (dragState && !nodes.includes(dragState.node)) {
            releaseDrag();
        }

        const currentTarget = tooltipManager.getTargetData();
        if (currentTarget && !nodes.includes(currentTarget)) {
            hideTooltip();
        }

        simulation.nodes(nodes);
        simulation.force("link").links(links);

        const linkStructureChanged = previousLinkCount !== allLinks.length;
        const structureChanged =
            commitReconciliation.changed ||
            branchReconciliation.changed ||
            linkStructureChanged;

        applySimulationUpdate(
            structureChanged,
            initialLayoutComplete,
            commitReconciliation.changed,
            allNodes,
            allLinks,
            branchReconciliation.alignments,
        );
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
        if (layoutManager.shouldAutoCenter()) {
            centerOnLatestCommit();
            layoutManager.checkAutoCenterStop(simulation.alpha());
        }
        render();
    }

    function destroy() {
        window.removeEventListener("resize", resize);
        resizeObserver?.disconnect();
        d3.select(canvas).on(".zoom", null);
        simulation.stop();
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

    return {
        applyDelta,
        destroy,
    };
}

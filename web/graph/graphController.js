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
    TREE_NODE_OFFSET_X,
    TREE_NODE_OFFSET_Y,
    CHARGE_STRENGTH,
    COLLISION_RADIUS,
    DRAG_ACTIVATION_DISTANCE,
    LINK_DISTANCE,
    LINK_STRENGTH,
    NODE_RADIUS,
    ZOOM_MAX,
    ZOOM_MIN,
} from "./constants.js";
import { GraphRenderer } from "./rendering/graphRenderer.js";
import { LayoutManager } from "./layout/layoutManager.js";
import { buildPalette } from "./utils/palette.js";
import { createGraphState, setZoomTransform } from "./state/graphState.js";

/**
 * Creates and initializes the graph controller instance.
 *
 * @param {HTMLElement} rootElement DOM node that hosts the canvas.
 * @returns {{ applyDelta(delta: unknown): void, destroy(): void }} Public graph API.
 */
export function createGraphController(rootElement) {
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
    let currentTreeCommitHash = null;

    const simulation = d3
        .forceSimulation(nodes)
        .force("charge", d3.forceManyBody().strength(CHARGE_STRENGTH))
        .force("center", d3.forceCenter(0, 0))
        .force("collision", d3.forceCollide().radius(COLLISION_RADIUS))
        .force(
            "link",
            d3
                .forceLink(links)
                .id((d) => d.hash)
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

    const tooltipManager = new TooltipManager(canvas);
    const renderer = new GraphRenderer(canvas, buildPalette(canvas));

    const updateTooltipPosition = () => {
        tooltipManager.updatePosition(zoomTransform);
    };
    const hideTooltip = () => {
        if (currentTreeCommitHash) {
            removeTreeNodeForCommit(currentTreeCommitHash);
            currentTreeCommitHash = null;
        }
        tooltipManager.hideAll();
        render();
    };
    const showTooltip = (node) => {
        tooltipManager.show(node, zoomTransform);
        render();
    };
    async function fetchTree(treeHash) {
        try {
            const response = await fetch(`/api/tree/${treeHash}`);
            if (!response.ok) {
                throw new Error(
                    `Failed to fetch tree: ${response.status} ${response.statusText}`,
                );
            }
            return await response.json();
        } catch (error) {
            console.error("Error fetching tree:", error);
            throw error;
        }
    }

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
            const radius =
                node.type === "branch"
                    ? PICK_RADIUS_BRANCH
                    : PICK_RADIUS_COMMIT;
            if (distSq <= radius * radius && distSq < bestDist) {
                bestDist = distSq;
                bestNode = node;
            }
        }

        return bestNode;
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
        const targetNode = findNodeAt(x, y, "commit") ?? findNodeAt(x, y);

        if (!targetNode) {
            hideTooltip();
            return;
        }

        layoutManager.disableAutoCenter();

        const currentTarget = tooltipManager.getTargetData();
        if (tooltipManager.isVisible() && currentTarget === targetNode) {
            hideTooltip();
        } else {
            if (
                currentTreeCommitHash &&
                currentTreeCommitHash !== targetNode.hash
            ) {
                removeTreeNodeForCommit(currentTreeCommitHash);
            }

            showTooltip(targetNode);

            if (targetNode.type === "commit") {
                currentTreeCommitHash = targetNode.hash;
                loadTreeNodeForCommit(targetNode).catch((error) => {
                    console.error("Error loading tree node:", error);
                    currentTreeCommitHash = null; // Reset on error
                });
            }
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
                simulation.alphaTarget(0.4).restart();
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

        viewportWidth = cssWidth;
        viewportHeight = cssHeight;

        canvas.width = Math.round(cssWidth * dpr);
        canvas.height = Math.round(cssHeight * dpr);
        canvas.style.width = `${cssWidth}px`;
        canvas.style.height = `${cssHeight}px`;

        layoutManager.updateViewport(cssWidth, cssHeight);
        layoutManager.restartSimulation(1.0);
        render();
    };

    window.addEventListener("resize", resize);
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
        const existingTreeNodes = new Map();

        for (const node of nodes) {
            if (node.type === "branch" && node.branch) {
                existingBranchNodes.set(node.branch, node);
            } else if (node.type === "commit" && node.hash) {
                existingCommitNodes.set(node.hash, node);
            } else if (node.type === "tree" && node.hash) {
                existingTreeNodes.set(node.hash, node);
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

        const nextTreeNodes = [];
        for (const [treeHash, treeNode] of existingTreeNodes.entries()) {
            nextTreeNodes.push(treeNode);
        }
        nodes.splice(
            0,
            nodes.length,
            ...nextCommitNodes,
            ...nextBranchNodes,
            ...nextTreeNodes,
        );

        const existingTreeLinks = links.filter((link) => link.kind === "tree");
        const allLinks = [...nextLinks, ...existingTreeLinks];
        links.splice(0, links.length, ...allLinks);

        if (dragState && !nodes.includes(dragState.node)) {
            releaseDrag();
        }

        const currentTarget = tooltipManager.getTargetData();
        if (currentTarget && !nodes.includes(currentTarget)) {
            hideTooltip();
        }

        if (currentTreeCommitHash) {
            const commitStillExists = nodes.some(
                (node) =>
                    node.type === "commit" &&
                    node.hash === currentTreeCommitHash,
            );
            if (!commitStillExists) {
                removeTreeNodeForCommit(currentTreeCommitHash);
                currentTreeCommitHash = null;
            }
        }

        simulation.nodes(nodes);
        simulation.force("link").links(links);

        const linkStructureChanged = previousLinkCount !== nextLinks.length;
        const structureChanged =
            commitStructureChanged ||
            branchStructureChanged ||
            linkStructureChanged;
        const hasCommits = nextCommitNodes.length > 0;

        if (!initialLayoutComplete && hasCommits) {
            layoutManager.applyTimelineLayout(nodes);
            snapBranchesToTargets(pendingBranchAlignments);
            layoutManager.requestAutoCenter();
            centerOnLatestCommit();
            initialLayoutComplete = true;
            layoutManager.restartSimulation(1.0);
        } else {
            snapBranchesToTargets(pendingBranchAlignments);
            if (commitStructureChanged) {
                layoutManager.requestAutoCenter();
            }
        }

        layoutManager.boostSimulation(structureChanged);
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

            branchNode.x = baseX + BRANCH_NODE_OFFSET_X + jitter(2);
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
                x: (targetNode.x ?? 0) + BRANCH_NODE_OFFSET_X + jitter(4),
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
            x: baseX + BRANCH_NODE_OFFSET_X + jitterFallback(6),
            y: baseY + jitterFallback(BRANCH_NODE_OFFSET_Y),
            vx: 0,
            vy: 0,
        };
    }

    function createTreeNode(treeHash, commitNode) {
        const jitter = (range) => (Math.random() - 0.5) * range;
        return {
            type: "tree",
            hash: treeHash,
            commitHash: commitNode.hash,
            x: (commitNode.x ?? 0) + TREE_NODE_OFFSET_X + jitter(4),
            y: (commitNode.y ?? 0) + jitter(BRANCH_NODE_OFFSET_Y),
            vx: 0,
            vy: 0,
        };
    }

    async function loadTreeNodeForCommit(commitNode) {
        if (!commitNode || !commitNode.commit || !commitNode.commit.tree) {
            return null;
        }
        const treeHash = commitNode.commit.tree;

        const existingTree = nodes.find(
            (node) => node.type === "tree" && node.hash === treeHash,
        );
        if (existingTree) {
            return existingTree;
        }

        try {
            const treeData = await fetchTree(treeHash);
            const treeNode = createTreeNode(treeHash, commitNode);
            treeNode.tree = treeData;
            treeNode.spawnPhase = 0;
            nodes.push(treeNode);

            const treeLink = {
                source: commitNode,
                target: treeNode,
                kind: "tree",
            };
            links.push(treeLink);

            simulation.nodes(nodes);
            simulation.force("link").links(links);

            layoutManager.boostSimulation(true);
            render();
            return treeNode;
        } catch (error) {
            console.error(`Failed to load tree ${treeHash}:`, error);
            return null;
        }
    }

    function removeTreeNodeForCommit(commitHash) {
        if (!commitHash) {
            return;
        }

        const commitNode = nodes.find(
            (node) => node.type === "commit" && node.hash === commitHash,
        );
        if (!commitNode || !commitNode.commit || !commitNode.commit.tree) {
            return;
        }
        const treeHash = commitNode.commit.tree;

        const treeNodeIndex = nodes.findIndex(
            (node) => node.type === "tree" && node.hash === treeHash,
        );
        if (treeNodeIndex !== -1) {
            nodes.splice(treeNodeIndex, 1);
        }

        const treeLinkIndex = links.findIndex(
            (link) =>
                link.kind === "tree" &&
                link.target &&
                link.target.hash === treeHash,
        );
        if (treeLinkIndex !== -1) {
            links.splice(treeLinkIndex, 1);
        }

        simulation.nodes(nodes);
        simulation.force("link").links(links);
        render();
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
        d3.select(canvas).on(".zoom", null);
        simulation.stop();
        removeThemeWatcher?.();
        releaseDrag();
        canvas.removeEventListener("pointerdown", pointerHandlers.down);
        canvas.removeEventListener("pointermove", pointerHandlers.move);
        canvas.removeEventListener("pointerup", pointerHandlers.up);
        canvas.removeEventListener("pointercancel", pointerHandlers.cancel);
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

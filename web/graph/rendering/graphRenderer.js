/**
 * @fileoverview Canvas renderer for the Git graph visualization.
 * Responsible for painting nodes, links, and highlight treatment.
 */

import {
    ARROW_LENGTH,
    ARROW_WIDTH,
    BRANCH_NODE_CORNER_RADIUS,
    BRANCH_NODE_PADDING_X,
    BRANCH_NODE_PADDING_Y,
    BRANCH_NODE_RADIUS,
    HIGHLIGHT_NODE_RADIUS,
    HIGHLIGHT_MERGE_NODE_RADIUS,
    LABEL_FONT,
    LABEL_PADDING,
    LINK_THICKNESS,
    MERGE_NODE_RADIUS,
    NODE_RADIUS,
    NODE_SHADOW_BLUR,
    NODE_SHADOW_OFFSET_Y,
    TREE_ICON_SIZE,
    TREE_ICON_OFFSET,
} from "../constants.js";
import { shortenHash } from "../../utils/format.js";

/**
 * Renders graph nodes and links to a 2D canvas context.
 */
export class GraphRenderer {
    /**
     * @param {HTMLCanvasElement} canvas Canvas element receiving render output.
     * @param {import("../types.js").GraphPalette} palette Color palette derived from CSS variables.
     */
    constructor(canvas, palette) {
        this.canvas = canvas;
        this.ctx = canvas.getContext("2d", { alpha: false });
        this.palette = palette;
    }

    /**
     * Renders the entire scene based on provided state.
     *
     * @param {{nodes: import("../types.js").GraphNode[], links: Array<{source: string | import("../types.js").GraphNode, target: string | import("../types.js").GraphNode, kind?: string}>, zoomTransform: import("d3").ZoomTransform, viewportWidth: number, viewportHeight: number, tooltipManager?: import("../../tooltips/index.js").TooltipManager}} state Graph state snapshot.
     */
    render(state) {
        const { nodes, links, zoomTransform, viewportWidth, viewportHeight } =
            state;
        const highlightKey = state.tooltipManager?.getHighlightKey();

        this.clear(viewportWidth, viewportHeight);
        this.setupTransform(zoomTransform);

        this.renderLinks(links, nodes);
        this.renderNodes(nodes, highlightKey);

        this.ctx.restore();
    }

    /**
     * Clears the canvas using the active palette background.
     *
     * @param {number} width Viewport width in CSS pixels.
     * @param {number} height Viewport height in CSS pixels.
     */
    clear(width, height) {
        const dpr = window.devicePixelRatio || 1;
        try {
            this.ctx.save();
            this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
            this.ctx.fillStyle = this.palette.background;
            this.ctx.fillRect(0, 0, width, height);
        } catch (e) {
            // Canvas is in error state - silently skip this frame
            // The error will be resolved when resize() sets valid dimensions
            return;
        }
    }

    /**
     * Applies translation and scaling based on the zoom transform.
     *
     * @param {import("d3").ZoomTransform} zoomTransform D3 zoom transform mapping graph to screen.
     */
    setupTransform(zoomTransform) {
        this.ctx.translate(zoomTransform.x, zoomTransform.y);
        this.ctx.scale(zoomTransform.k, zoomTransform.k);
    }

    /**
     * Updates the palette used for future render cycles.
     *
     * @param {import("../types.js").GraphPalette} palette Updated palette.
     */
    updatePalette(palette) {
        this.palette = palette;
    }

    /**
     * Backwards compatibility alias for historical misspelling.
     *
     * @param {import("../types.js").GraphPalette} palette Updated palette.
     */
    updatePallete(palette) {
        this.updatePalette(palette);
    }

    /**
     * Draws all links connecting commit and branch nodes.
     *
     * @param {Array<{source: string | import("../types.js").GraphNode, target: string | import("../types.js").GraphNode, kind?: string}>} links Link definitions from the force simulation.
     * @param {import("../types.js").GraphNode[]} nodes Node collection used to resolve string references.
     */
    renderLinks(links, nodes) {
        this.ctx.lineWidth = LINK_THICKNESS;

        for (const link of links) {
            const source = this.resolveNode(link.source, nodes);
            const target = this.resolveNode(link.target, nodes);
            if (!source || !target) continue;

            const warmup =
                typeof link.warmup === "number"
                    ? Math.max(0, Math.min(1, link.warmup))
                    : 1;
            const nextWarmup = warmup < 1 ? Math.min(1, warmup + 0.12) : 1;
            link.warmup = nextWarmup;
            if (warmup <= 0) {
                continue;
            }

            const prevAlpha = this.ctx.globalAlpha;
            this.ctx.globalAlpha = prevAlpha * warmup;
            this.renderLink(source, target, link.kind);
            this.ctx.globalAlpha = prevAlpha;
        }
    }

    /**
     * Resolves a link endpoint from a node or hash identifier.
     *
     * @param {string | import("../types.js").GraphNode} nodeOrHash Node instance or commit hash.
     * @param {import("../types.js").GraphNode[]} nodes Node collection for lookups.
     * @returns {import("../types.js").GraphNode | undefined} Matching node when found.
     */
    resolveNode(nodeOrHash, nodes) {
        return typeof nodeOrHash === "object"
            ? nodeOrHash
            : nodes.find((n) => n.hash === nodeOrHash);
    }

    /**
     * Draws a single directional link with an arrowhead.
     *
     * @param {import("../types.js").GraphNode} source Source node.
     * @param {import("../types.js").GraphNode} target Target node.
     * @param {boolean} isBranch True when the arrow represents a branch link.
     */
    renderLink(source, target, linkKind) {
        const dx = target.x - source.x;
        const dy = target.y - source.y;
        const distance = Math.sqrt(dx * dx + dy * dy);
        if (distance === 0) {
            return;
        }

        const color = linkKind === "branch"
            ? this.palette.branchLink
            : this.palette.link;

        const targetRadius = target.type === "branch"
            ? BRANCH_NODE_RADIUS
            : NODE_RADIUS;

        this.renderArrow(source, target, dx, dy, distance, targetRadius, color);
    }

    /**
     * Renders a link shaft and arrowhead given vector math between endpoints.
     *
     * @param {import("../types.js").GraphNode} source Source node.
     * @param {import("../types.js").GraphNode} target Target node.
     * @param {number} dx Delta X between source and target.
     * @param {number} dy Delta Y between source and target.
     * @param {number} distance Euclidean distance between nodes.
     * @param {number} targetRadius Radius of the target node for arrow placement.
     * @param {string} color Stroke and fill color for the arrow.
     */
    renderArrow(source, target, dx, dy, distance, targetRadius, color) {
        const arrowBase = Math.max(
            (distance - targetRadius - ARROW_LENGTH) / distance,
            0,
        );
        const arrowTip = Math.max((distance - targetRadius) / distance, 0);

        const shaftEndX = source.x + dx * arrowBase;
        const shaftEndY = source.y + dy * arrowBase;
        const arrowTipX = source.x + dx * arrowTip;
        const arrowTipY = source.y + dy * arrowTip;

        this.ctx.strokeStyle = color;
        this.ctx.beginPath();
        this.ctx.moveTo(source.x, source.y);
        this.ctx.lineTo(shaftEndX, shaftEndY);
        this.ctx.stroke();

        this.ctx.save();
        this.ctx.translate(arrowTipX, arrowTipY);
        this.ctx.rotate(Math.atan2(dy, dx));
        this.ctx.beginPath();
        this.ctx.moveTo(0, 0);
        this.ctx.lineTo(-ARROW_LENGTH, ARROW_WIDTH / 2);
        this.ctx.lineTo(-ARROW_LENGTH, -ARROW_WIDTH / 2);
        this.ctx.closePath();
        this.ctx.fillStyle = color;
        this.ctx.fill();
        this.ctx.restore();
    }

    /**
     * Draws commit and branch nodes, honoring highlight states.
     *
     * @param {import("../types.js").GraphNode[]} nodes Collection of nodes to render.
     * @param {string|null} highlightKey Hash or branch name for the highlighted node.
     */
    renderNodes(nodes, highlightKey) {
        for (const node of nodes) {
            if (node.type === "commit") {
                this.renderCommitNode(node, highlightKey);
            }
        }
        for (const node of nodes) {
            if (node.type === "branch") {
                this.renderBranchNode(node, highlightKey);
            }
        }
    }

    /**
     * Draws a commit node including adaptive highlighting.
     *
     * @param {import("../types.js").GraphNodeCommit} node Commit node to paint.
     * @param {string|null} highlightKey Current highlight identifier.
     */
    renderCommitNode(node, highlightKey) {
        const isHighlighted = highlightKey && node.hash === highlightKey;
        const isMerge = (node.commit?.parents?.length ?? 0) >= 2;

        const baseRadius = isMerge ? MERGE_NODE_RADIUS : NODE_RADIUS;
        const highlightRadius = isMerge
            ? HIGHLIGHT_MERGE_NODE_RADIUS
            : HIGHLIGHT_NODE_RADIUS;
        const currentRadius = node.radius ?? baseRadius;
        const targetRadius = isHighlighted ? highlightRadius : baseRadius;
        node.radius = currentRadius + (targetRadius - currentRadius) * 0.25;

        const spawnProgress =
            typeof node.spawnPhase === "number" ? node.spawnPhase : 1;
        const easedSpawn =
            spawnProgress * spawnProgress * (3 - 2 * spawnProgress);
        const nextSpawn =
            spawnProgress < 1 ? Math.min(1, spawnProgress + 0.12) : 1;
        if (nextSpawn >= 1) {
            delete node.spawnPhase;
        } else {
            node.spawnPhase = nextSpawn;
        }

        const spawnAlpha = Math.max(0, Math.min(1, easedSpawn));
        const radiusScale = 0.55 + 0.45 * spawnAlpha;
        const drawRadius = node.radius * radiusScale;

        const previousAlpha = this.ctx.globalAlpha;
        this.ctx.globalAlpha = previousAlpha * (spawnAlpha || 0.01);
        if (isHighlighted) {
            if (isMerge) {
                this.renderHighlightedMerge(node, drawRadius);
            } else {
                this.renderHighlightedCommit(node, drawRadius);
            }
        } else {
            if (isMerge) {
                this.renderNormalMerge(node, drawRadius);
            } else {
                this.renderNormalCommit(node, drawRadius);
            }
        }
        this.ctx.globalAlpha = previousAlpha;

        this.renderCommitLabel(node, spawnAlpha);
    }

    /**
     * Renders a non-highlighted commit node.
     *
     * @param {import("../types.js").GraphNodeCommit} node Commit node to paint.
     */
    renderNormalCommit(node, radius) {
        this.ctx.fillStyle = this.palette.node;
        this.applyShadow();
        this.ctx.beginPath();
        this.ctx.arc(node.x, node.y, radius, 0, Math.PI * 2);
        this.ctx.fill();
        this.clearShadow();
    }

    /**
     * Renders a highlighted commit node with glow and stroke treatments.
     *
     * @param {import("../types.js").GraphNodeCommit} node Commit node to paint.
     */
    renderHighlightedCommit(node, radius) {
        this.ctx.save();
        this.ctx.fillStyle = this.palette.nodeHighlightGlow;
        this.ctx.globalAlpha = 0.35;
        this.ctx.beginPath();
        this.ctx.arc(node.x, node.y, radius + 7, 0, Math.PI * 2);
        this.ctx.fill();
        this.ctx.restore();

        const gradient = this.ctx.createRadialGradient(
            node.x,
            node.y,
            radius * 0.2,
            node.x,
            node.y,
            radius,
        );
        gradient.addColorStop(0, this.palette.nodeHighlightCore);
        gradient.addColorStop(0.7, this.palette.nodeHighlight);
        gradient.addColorStop(1, this.palette.nodeHighlightRing);

        this.ctx.fillStyle = gradient;
        this.applyShadow();
        this.ctx.beginPath();
        this.ctx.arc(node.x, node.y, radius, 0, Math.PI * 2);
        this.ctx.fill();
        this.clearShadow();

        this.ctx.save();
        this.ctx.lineWidth = 1.25;
        this.ctx.strokeStyle = this.palette.nodeHighlight;
        this.ctx.globalAlpha = 0.8;
        this.ctx.beginPath();
        this.ctx.arc(node.x, node.y, radius + 1.8, 0, Math.PI * 2);
        this.ctx.stroke();
        this.ctx.restore();
    }

    /**
     * Renders a non-highlighted merge commit as a diamond.
     *
     * @param {import("../types.js").GraphNodeCommit} node Merge commit node.
     * @param {number} radius Half-diagonal of the diamond.
     */
    renderNormalMerge(node, radius) {
        this.ctx.fillStyle = this.palette.mergeNode;
        this.applyShadow();
        this.drawDiamond(node.x, node.y, radius);
        this.ctx.fill();
        this.clearShadow();
    }

    /**
     * Renders a highlighted merge commit diamond with glow and stroke.
     *
     * @param {import("../types.js").GraphNodeCommit} node Merge commit node.
     * @param {number} radius Half-diagonal of the diamond.
     */
    renderHighlightedMerge(node, radius) {
        this.ctx.save();
        this.ctx.fillStyle = this.palette.mergeHighlightGlow;
        this.ctx.globalAlpha = 0.35;
        this.drawDiamond(node.x, node.y, radius + 7);
        this.ctx.fill();
        this.ctx.restore();

        const gradient = this.ctx.createRadialGradient(
            node.x,
            node.y,
            radius * 0.2,
            node.x,
            node.y,
            radius,
        );
        gradient.addColorStop(0, this.palette.mergeHighlightCore);
        gradient.addColorStop(0.7, this.palette.mergeHighlight);
        gradient.addColorStop(1, this.palette.mergeHighlightRing);

        this.ctx.fillStyle = gradient;
        this.applyShadow();
        this.drawDiamond(node.x, node.y, radius);
        this.ctx.fill();
        this.clearShadow();

        this.ctx.save();
        this.ctx.lineWidth = 1.25;
        this.ctx.strokeStyle = this.palette.mergeHighlight;
        this.ctx.globalAlpha = 0.8;
        this.drawDiamond(node.x, node.y, radius + 1.8);
        this.ctx.stroke();
        this.ctx.restore();
    }

    /**
     * Draws the text label alongside a commit node.
     *
     * @param {import("../types.js").GraphNodeCommit} node Commit node to annotate.
     */
    renderCommitLabel(node, spawnAlpha = 1) {
        if (!node.commit?.hash) return;

        const text = shortenHash(node.commit.hash);

        this.ctx.save();
        this.ctx.font = LABEL_FONT;
        this.ctx.textBaseline = "middle";
        this.ctx.textAlign = "left";

        const offset = node.radius + LABEL_PADDING;
        const labelX = node.x + offset;
        const labelY = node.y;

        this.ctx.lineWidth = 3;
        this.ctx.lineJoin = "round";
        this.ctx.strokeStyle = this.palette.labelHalo;
        this.ctx.globalAlpha = 0.9 * spawnAlpha;
        this.ctx.strokeText(text, labelX, labelY);

        this.ctx.globalAlpha = spawnAlpha;
        this.ctx.fillStyle = this.palette.labelText;
        this.ctx.fillText(text, labelX, labelY);

        this.ctx.restore();
    }

    /**
     * Renders a small folder icon at the top-right of a commit node.
     * This icon indicates the commit has a tree that can be browsed.
     *
     * @param {import("../types.js").GraphNodeCommit} node Commit node to annotate with icon.
     */
    renderTreeIcon(node) {
        if (!node.commit?.tree) return;

        const iconSize = TREE_ICON_SIZE;
        const offsetX = node.radius + TREE_ICON_OFFSET;
        const offsetY = -(node.radius + TREE_ICON_OFFSET);
        const ix = node.x + offsetX;
        const iy = node.y + offsetY;

        // Small folder shape
        this.ctx.fillStyle = this.palette.treeNode;
        this.ctx.beginPath();
        this.ctx.moveTo(ix, iy);
        this.ctx.lineTo(ix + iconSize * 0.4, iy);
        this.ctx.lineTo(ix + iconSize * 0.5, iy - iconSize * 0.2);
        this.ctx.lineTo(ix + iconSize, iy - iconSize * 0.2);
        this.ctx.lineTo(ix + iconSize, iy + iconSize * 0.6);
        this.ctx.lineTo(ix, iy + iconSize * 0.6);
        this.ctx.closePath();
        this.ctx.fill();
    }

    /**
     * Renders a branch node pill with text.
     *
     * @param {import("../types.js").GraphNodeBranch} node Branch node to paint.
     * @param {string|null} highlightKey Current highlight identifier.
     */
    renderBranchNode(node, highlightKey) {
        const isHighlighted = highlightKey && node.branch === highlightKey;
        const text = node.branch ?? "";

        const spawnProgress =
            typeof node.spawnPhase === "number" ? node.spawnPhase : 1;
        const easedSpawn =
            spawnProgress * spawnProgress * (3 - 2 * spawnProgress);
        const nextSpawn =
            spawnProgress < 1 ? Math.min(1, spawnProgress + 0.12) : 1;
        if (nextSpawn >= 1) {
            delete node.spawnPhase;
        } else {
            node.spawnPhase = nextSpawn;
        }
        const spawnAlpha = Math.max(0, Math.min(1, easedSpawn));
        const scale = 0.7 + 0.3 * spawnAlpha;

        this.ctx.save();
        const previousAlpha = this.ctx.globalAlpha;
        this.ctx.globalAlpha = previousAlpha * (spawnAlpha || 0.01);
        this.ctx.translate(node.x, node.y);
        this.ctx.scale(scale, scale);
        this.ctx.translate(-node.x, -node.y);
        this.ctx.font = LABEL_FONT;
        this.ctx.textBaseline = "middle";
        this.ctx.textAlign = "center";

        const metrics = this.ctx.measureText(text);
        const textHeight = metrics.actualBoundingBoxAscent ?? 9;
        const width = metrics.width + BRANCH_NODE_PADDING_X * 2;
        const height = textHeight + BRANCH_NODE_PADDING_Y * 2;

        this.drawRoundedRect(
            node.x - width / 2,
            node.y - height / 2,
            width,
            height,
            BRANCH_NODE_CORNER_RADIUS,
        );

        this.ctx.fillStyle = isHighlighted
            ? this.palette.branchHighlight
            : this.palette.branchNode;
        this.applyShadow();
        this.ctx.fill();
        this.clearShadow();
        const baseLineWidth = isHighlighted ? 2 : 1.5;
        this.ctx.lineWidth = baseLineWidth / scale;
        this.ctx.strokeStyle = isHighlighted
            ? this.palette.branchHighlightRing
            : this.palette.branchNodeBorder;
        this.ctx.stroke();

        this.ctx.fillStyle = this.palette.branchLabelText;
        this.ctx.fillText(text, node.x, node.y);
        this.ctx.globalAlpha = previousAlpha;
        this.ctx.restore();
    }


    /**
     * Draws a rounded rectangle path for branch nodes.
     *
     * @param {number} x Starting X coordinate.
     * @param {number} y Starting Y coordinate.
     * @param {number} width Rectangle width.
     * @param {number} height Rectangle height.
     * @param {number} radius Corner radius.
     */
    drawRoundedRect(x, y, width, height, radius) {
        const r = Math.max(0, Math.min(radius, Math.min(width, height) / 2));
        this.ctx.beginPath();
        this.ctx.moveTo(x + r, y);
        this.ctx.lineTo(x + width - r, y);
        this.ctx.quadraticCurveTo(x + width, y, x + width, y + r);
        this.ctx.lineTo(x + width, y + height - r);
        this.ctx.quadraticCurveTo(
            x + width,
            y + height,
            x + width - r,
            y + height,
        );
        this.ctx.lineTo(x + r, y + height);
        this.ctx.quadraticCurveTo(x, y + height, x, y + height - r);
        this.ctx.lineTo(x, y + r);
        this.ctx.quadraticCurveTo(x, y, x + r, y);
        this.ctx.closePath();
    }

    /**
     * Applies a drop shadow to subsequent fill operations.
     */
    applyShadow() {
        this.ctx.shadowBlur = NODE_SHADOW_BLUR;
        this.ctx.shadowColor = this.palette.nodeShadow;
        this.ctx.shadowOffsetX = 0;
        this.ctx.shadowOffsetY = NODE_SHADOW_OFFSET_Y;
    }

    /**
     * Clears shadow state so it doesn't bleed into strokes or labels.
     */
    clearShadow() {
        this.ctx.shadowBlur = 0;
        this.ctx.shadowColor = "transparent";
        this.ctx.shadowOffsetX = 0;
        this.ctx.shadowOffsetY = 0;
    }

    /**
     * Draws a diamond (rotated square) path centered on the given coordinates.
     *
     * @param {number} cx Center X.
     * @param {number} cy Center Y.
     * @param {number} radius Half-diagonal of the diamond.
     */
    drawDiamond(cx, cy, radius) {
        this.ctx.beginPath();
        this.ctx.moveTo(cx, cy - radius);
        this.ctx.lineTo(cx + radius, cy);
        this.ctx.lineTo(cx, cy + radius);
        this.ctx.lineTo(cx - radius, cy);
        this.ctx.closePath();
    }
}

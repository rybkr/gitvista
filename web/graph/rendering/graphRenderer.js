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
    TAG_NODE_COLOR,
    TAG_NODE_BORDER_COLOR,
    TAG_NODE_RADIUS,
    COMMIT_MESSAGE_ZOOM_THRESHOLD,
    COMMIT_MESSAGE_MAX_CHARS,
    COMMIT_MESSAGE_FONT,
    COMMIT_AUTHOR_ZOOM_THRESHOLD,
    COMMIT_DATE_ZOOM_THRESHOLD,
    COMMIT_DETAIL_FONT,
    HOVER_GLOW_EXTRA_RADIUS,
    HOVER_GLOW_OPACITY,
    LANE_CORNER_RADIUS,
    LANE_VERTICAL_STEP,
    LANE_WIDTH,
    LANE_MARGIN,
    LANE_HEADER_FONT,
} from "../constants.js";
import { shortenHash } from "../../utils/format.js";
import { getAuthorColor } from "../../utils/colors.js";
import { relativeTime } from "../utils/time.js";

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
        const headHash = state.headHash ?? "";
        const hoverNode = state.hoverNode ?? null;
        const tags = state.tags ?? new Map();

        const laneInfo = state.laneInfo ?? [];

        this.clear(viewportWidth, viewportHeight);
        this.setupTransform(zoomTransform);

        if (laneInfo.length > 0) {
            this.renderLaneBackgrounds(laneInfo, viewportHeight, zoomTransform);
        }

        this.renderLinks(links, nodes);
        const layoutMode = state.layoutMode ?? "force";
        this.renderNodes(nodes, highlightKey, zoomTransform, headHash, hoverNode, tags, layoutMode);

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
     * Renders semi-transparent vertical background strips and branch name headers
     * for each lane in lane layout mode.
     *
     * @param {Array<{index: number, color: string, branchName: string, segments: Array<{minY: number, maxY: number}>, minY: number, maxY: number}>} laneInfo Lane metadata.
     * @param {number} viewportHeight Viewport height in CSS pixels.
     * @param {import("d3").ZoomTransform} zoomTransform Current zoom transform.
     */
    renderLaneBackgrounds(laneInfo, viewportHeight, zoomTransform) {
        const k = zoomTransform.k;
        // Convert viewport bounds into graph coordinates for the vertical extent
        const topY = -zoomTransform.y / k;
        const bottomY = topY + viewportHeight / k;

        for (const lane of laneInfo) {
            const cx = LANE_MARGIN + lane.index * LANE_WIDTH;
            const halfW = LANE_WIDTH / 2 - 4;
            const pad = LANE_VERTICAL_STEP / 2;
            const segments = lane.segments ?? [{ minY: lane.minY, maxY: lane.maxY }];

            // Draw background strip and label per segment
            for (const seg of segments) {
                const stripTop = Math.max(topY, seg.minY - pad);
                const stripBottom = Math.min(bottomY, seg.maxY + pad);
                if (stripTop >= stripBottom) continue;

                // Background — rounded ends where the segment isn't clipped
                this.ctx.save();
                this.ctx.globalAlpha = 0.06;
                this.ctx.fillStyle = lane.color;
                const rTop = stripTop <= topY ? 0 : halfW;
                const rBottom = stripBottom >= bottomY ? 0 : halfW;
                this.ctx.beginPath();
                this.ctx.roundRect(
                    cx - halfW, stripTop,
                    halfW * 2, stripBottom - stripTop,
                    [rTop, rTop, rBottom, rBottom],
                );
                this.ctx.fill();
                this.ctx.restore();

                // Branch name label — pinned to top of visible area but
                // clamped within this segment's vertical extent
                const segName = seg.branchName || "";
                if (segName) {
                    const labelTop = seg.minY - pad;
                    const labelBottom = seg.maxY + pad - 24 / k;
                    const headerY = Math.max(labelTop, Math.min(topY + 18 / k, labelBottom));
                    this.ctx.save();
                    this.ctx.font = LANE_HEADER_FONT;
                    this.ctx.textAlign = "center";
                    this.ctx.textBaseline = "top";

                    // Halo for readability
                    this.ctx.lineWidth = 3;
                    this.ctx.lineJoin = "round";
                    this.ctx.strokeStyle = this.palette.labelHalo;
                    this.ctx.globalAlpha = 0.9;
                    this.ctx.strokeText(segName, cx, headerY);

                    // Text
                    this.ctx.globalAlpha = 0.75;
                    this.ctx.fillStyle = lane.color;
                    this.ctx.fillText(segName, cx, headerY);
                    this.ctx.restore();
                }
            }
        }
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
     * Draws all links connecting commit and branch nodes.
     *
     * @param {Array<{source: string | import("../types.js").GraphNode, target: string | import("../types.js").GraphNode, kind?: string}>} links Link definitions from the force simulation.
     * @param {import("../types.js").GraphNode[]} nodes Node collection used to resolve string references.
     */
    renderLinks(links, nodes) {
        this.ctx.lineWidth = LINK_THICKNESS;

        // Build a hash-to-node lookup map once per frame: O(n) instead of
        // O(n) per link endpoint via the previous linear scan.
        const nodeMap = new Map();
        for (const node of nodes) {
            if (node.hash) nodeMap.set(node.hash, node);
            if (node.branch) nodeMap.set(node.branch, node);
            if (node.tag) nodeMap.set(node.tag, node);
        }

        for (const link of links) {
            const source = typeof link.source === "object"
                ? link.source
                : nodeMap.get(link.source);
            const target = typeof link.target === "object"
                ? link.target
                : nodeMap.get(link.target);
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

            // Dim links whose commit endpoints are both dimmed — fades non-matching
            // sub-graphs without removing them from the force simulation.
            // Branch and tag links are always rendered at full opacity so labels
            // remain legible during a search.
            const isDimmedLink = link.kind !== "branch" && link.kind !== "tag" &&
                (source.dimmed || target.dimmed);
            const dimAlpha = isDimmedLink ? 0.15 : 1;

            const prevAlpha = this.ctx.globalAlpha;
            this.ctx.globalAlpha = prevAlpha * warmup * dimAlpha;
            this.renderLink(source, target, link.kind);
            this.ctx.globalAlpha = prevAlpha;
        }
    }

    /**
     * Draws a single directional link with an arrowhead.
     * Supports lane-aware coloring and stepped paths for cross-lane connections.
     *
     * @param {import("../types.js").GraphNode} source Source node.
     * @param {import("../types.js").GraphNode} target Target node.
     * @param {string} linkKind Link kind ("branch" or undefined).
     */
    renderLink(source, target, linkKind) {
        const dx = target.x - source.x;
        const dy = target.y - source.y;
        const distance = Math.sqrt(dx * dx + dy * dy);
        if (distance === 0) {
            return;
        }

        // Determine link color: branch/tag links use their palette color,
        // commit links use lane color if present, otherwise default palette
        let color;
        if (linkKind === "branch") {
            color = this.palette.branchLink;
        } else if (linkKind === "tag") {
            color = this.palette.tagLink;
        } else {
            color = source.laneColor || this.palette.link;
        }

        let targetRadius;
        if (target.type === "branch") {
            targetRadius = BRANCH_NODE_RADIUS;
        } else if (target.type === "tag") {
            targetRadius = TAG_NODE_RADIUS;
        } else {
            targetRadius = NODE_RADIUS;
        }

        // Check if this is a cross-lane connection (lane mode only)
        const isCrossLane =
            source.laneIndex !== undefined &&
            target.laneIndex !== undefined &&
            source.laneIndex !== target.laneIndex;

        if (isCrossLane) {
            this.renderSteppedArrow(source, target, targetRadius, color);
        } else {
            this.renderArrow(source, target, dx, dy, distance, targetRadius, color);
        }
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
     * Renders a stepped path for cross-lane connections with an arrowhead.
     * The path consists of three segments:
     * 1. Vertical line from source downward
     * 2. Horizontal line across lanes
     * 3. Vertical line to target with arrowhead
     *
     * For octopus merges (multiple parents), stagger the midpoint Y to avoid overlap.
     *
     * @param {import("../types.js").GraphNode} source Source node.
     * @param {import("../types.js").GraphNode} target Target node.
     * @param {number} targetRadius Radius of the target node for arrow placement.
     * @param {string} color Stroke and fill color for the arrow.
     */
    renderSteppedArrow(source, target, targetRadius, color) {
        // Determine direction: target may be above or below source
        const goingDown = target.y >= source.y;
        const dir = goingDown ? 1 : -1;

        // Calculate midpoint Y with stagger for octopus merges
        // Use lane indices (small bounded integers) for deterministic stagger
        const sourceLane = source.laneIndex ?? 0;
        const targetLane = target.laneIndex ?? 0;
        const goingRight = target.x > source.x;
        const stagger = ((sourceLane * 7 + targetLane * 13) % 11 - 5) * (goingRight ? 2 : 1);
        // Right-going arrows: turn right after the source node so they
        // don't pass through intermediate same-lane commits.
        // Left-going arrows (merges): turn halfway between the target
        // (merge node) and its previous node in the lane, giving the
        // rounded corners room to breathe.
        const midY = goingRight
            ? source.y + dir * (NODE_RADIUS + 10) + stagger
            : target.y - dir * (LANE_VERTICAL_STEP / 2) + stagger;
        const endY = target.y - dir * (targetRadius + ARROW_LENGTH);

        // Clamp corner radius so it doesn't exceed half the available
        // vertical or horizontal span (avoids visual artefacts on tight paths).
        const maxV = Math.min(
            Math.abs(midY - source.y),
            Math.abs(endY - midY),
        ) / 2;
        const maxH = Math.abs(target.x - source.x) / 2;
        const r = Math.max(0, Math.min(LANE_CORNER_RADIUS, maxV, maxH));

        // Draw the stepped path with rounded corners via arcTo
        this.ctx.strokeStyle = color;
        this.ctx.beginPath();
        this.ctx.moveTo(source.x, source.y);
        this.ctx.arcTo(source.x, midY, target.x, midY, r);
        this.ctx.arcTo(target.x, midY, target.x, endY, r);
        this.ctx.lineTo(target.x, endY);
        this.ctx.stroke();

        // Draw arrowhead pointing toward target
        const arrowTipY = target.y - dir * targetRadius;
        this.ctx.save();
        this.ctx.translate(target.x, arrowTipY);
        this.ctx.rotate(goingDown ? Math.PI / 2 : -Math.PI / 2);
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
    renderNodes(nodes, highlightKey, zoomTransform, headHash, hoverNode, tags, layoutMode) {
        // Build a reverse map: commit hash -> array of tag names pointing at it.
        const tagsByCommit = new Map();
        if (tags) {
            for (const [tagName, commitHash] of tags) {
                const existing = tagsByCommit.get(commitHash);
                if (existing) {
                    existing.push(tagName);
                } else {
                    tagsByCommit.set(commitHash, [tagName]);
                }
            }
        }

        for (const node of nodes) {
            if (node.type === "commit") {
                this.renderCommitNode(node, highlightKey, zoomTransform, headHash, hoverNode, layoutMode);
            }
        }
        for (const node of nodes) {
            if (node.type === "branch") {
                this.renderBranchNode(node, highlightKey);
            }
        }
        for (const node of nodes) {
            if (node.type === "tag") {
                this.renderTagNode(node, highlightKey);
            }
        }
    }

    /**
     * Draws a commit node including adaptive highlighting.
     *
     * @param {import("../types.js").GraphNodeCommit} node Commit node to paint.
     * @param {string|null} highlightKey Current highlight identifier.
     */
    renderCommitNode(node, highlightKey, zoomTransform, headHash, hoverNode, layoutMode) {
        const isHighlighted = highlightKey && node.hash === highlightKey;
        const isHead = headHash && node.hash === headHash;
        const isHovered = hoverNode && node === hoverNode;
        const isMerge = (node.commit?.parents?.length ?? 0) >= 2;
        // node.dimmed is set by applyDimmingFromPredicate() in graphController
        // when a search/filter is active. We reduce alpha to 15% so non-matching
        // commits recede without being removed from the D3 simulation.
        const isDimmed = node.dimmed === true;

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

        // Compound alpha: context alpha × spawn fade-in × dimming multiplier.
        // Dimmed nodes are drawn at 15% — visible enough to preserve graph
        // topology without competing with full-opacity matching commits.
        const previousAlpha = this.ctx.globalAlpha;
        const dimMultiplier = isDimmed ? 0.15 : 1;
        this.ctx.globalAlpha = previousAlpha * (spawnAlpha || 0.01) * dimMultiplier;
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

        // Hover glow ring — suppressed for dimmed nodes so the glow doesn't
        // punch through the 15% alpha and confuse the user.
        if (isHovered && !isHighlighted && !isDimmed) {
            this.ctx.save();
            this.ctx.globalAlpha = previousAlpha * HOVER_GLOW_OPACITY * spawnAlpha;
            this.ctx.fillStyle = "#ffffff";
            this.ctx.beginPath();
            this.ctx.arc(node.x, node.y, drawRadius + HOVER_GLOW_EXTRA_RADIUS, 0, Math.PI * 2);
            this.ctx.fill();
            this.ctx.restore();
        }

        // HEAD accent ring — rendered even when dimmed so HEAD is identifiable
        // during search. Its alpha is scaled by dimMultiplier for consistency.
        if (isHead) {
            this.ctx.save();
            this.ctx.globalAlpha = previousAlpha * spawnAlpha * 0.45 * dimMultiplier;
            this.ctx.lineWidth = 1.5;
            this.ctx.strokeStyle = this.palette.nodeHighlight;
            this.ctx.beginPath();
            this.ctx.arc(node.x, node.y, drawRadius + 3.5, 0, Math.PI * 2);
            this.ctx.stroke();
            this.ctx.restore();
        }

        // Skip label rendering for dimmed nodes — labels at 15% opacity would
        // clutter the view without adding navigational value.
        if (!isDimmed) {
            this.renderCommitLabel(node, spawnAlpha, zoomTransform, layoutMode);
        }
    }

    /**
     * Renders a non-highlighted commit node.
     * Uses lane color if available (lane mode), otherwise falls back to palette.
     *
     * @param {import("../types.js").GraphNodeCommit} node Commit node to paint.
     */
    renderNormalCommit(node, radius) {
        // Lane mode: use lane color. Force mode: use author-derived color.
        const authorEmail = node.commit?.author?.email;
        this.ctx.fillStyle = node.laneColor
            || (authorEmail ? getAuthorColor(authorEmail) : this.palette.node);
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
        const authorEmail = node.commit?.author?.email;
        this.ctx.fillStyle = node.laneColor
            || (authorEmail ? getAuthorColor(authorEmail) : this.palette.mergeNode);
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
    renderCommitLabel(node, spawnAlpha = 1, zoomTransform, layoutMode) {
        if (!node.commit?.hash) return;

        const text = shortenHash(node.commit.hash);
        const zoomK = zoomTransform?.k ?? 1;

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

        // Progressive detail: each tier adds text below the previous, tracking Y with detailY.
        let detailY = labelY + 14;

        // Tier 1 (zoom >= 1.5): first line of commit message
        if (zoomK >= COMMIT_MESSAGE_ZOOM_THRESHOLD && node.commit.message) {
            const firstLine = node.commit.message.split("\n")[0].trim();
            const maxChars = layoutMode === "lane" ? 30 : COMMIT_MESSAGE_MAX_CHARS;
            const truncated = firstLine.length > maxChars
                ? firstLine.slice(0, maxChars) + "…"
                : firstLine;
            this.ctx.font = COMMIT_MESSAGE_FONT;
            this.ctx.globalAlpha = 0.65 * spawnAlpha;
            this.ctx.fillStyle = this.palette.labelText;
            this.ctx.fillText(truncated, labelX, detailY);
            detailY += 13;
        }

        // Tier 2 (zoom >= 2.0): author name
        if (zoomK >= COMMIT_AUTHOR_ZOOM_THRESHOLD && node.commit.author?.name) {
            this.ctx.font = COMMIT_DETAIL_FONT;
            this.ctx.lineWidth = 3;
            this.ctx.lineJoin = "round";
            this.ctx.strokeStyle = this.palette.labelHalo;
            this.ctx.globalAlpha = 0.50 * spawnAlpha;
            this.ctx.strokeText(node.commit.author.name, labelX, detailY);
            this.ctx.fillStyle = this.palette.labelText;
            this.ctx.fillText(node.commit.author.name, labelX, detailY);
            detailY += 12;
        }

        // Tier 3 (zoom >= 3.0): relative commit date
        if (zoomK >= COMMIT_DATE_ZOOM_THRESHOLD) {
            const rel = relativeTime(node.commit.author?.when);
            if (rel) {
                this.ctx.font = COMMIT_DETAIL_FONT;
                this.ctx.lineWidth = 3;
                this.ctx.lineJoin = "round";
                this.ctx.strokeStyle = this.palette.labelHalo;
                this.ctx.globalAlpha = 0.40 * spawnAlpha;
                this.ctx.strokeText(rel, labelX, detailY);
                this.ctx.fillStyle = this.palette.labelText;
                this.ctx.fillText(rel, labelX, detailY);
            }
        }

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
     * Renders a tag node pill with text, mirroring renderBranchNode.
     *
     * @param {import("../types.js").GraphNodeTag} node Tag node to paint.
     * @param {string|null} highlightKey Current highlight identifier.
     */
    renderTagNode(node, highlightKey) {
        const text = `⌂ ${node.tag ?? ""}`;

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

        this.ctx.fillStyle = TAG_NODE_COLOR;
        this.applyShadow();
        this.ctx.fill();
        this.clearShadow();
        this.ctx.lineWidth = 1.5 / scale;
        this.ctx.strokeStyle = TAG_NODE_BORDER_COLOR;
        this.ctx.stroke();

        this.ctx.fillStyle = "#1a1a1a";
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

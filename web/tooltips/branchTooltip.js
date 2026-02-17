/**
 * @fileoverview Branch tooltip implementation for the Git graph UI.
 * Renders branch name and target commit hash details.
 */

import { Tooltip, createTooltipElement } from "./baseTooltip.js";
import { shortenHash } from "../utils/format.js";

/**
 * Tooltip that presents branch metadata.
 */
export class BranchTooltip extends Tooltip {
    /**
     * @param {HTMLCanvasElement} canvas Canvas that anchors tooltip positioning.
     */
    constructor(canvas) {
        super(canvas);
    }

    /**
     * Builds the DOM structure for branch tooltips.
     *
     * @returns {HTMLDivElement} Tooltip root element appended to the document body.
     */
    createElement() {
        const tooltip = /** @type {HTMLDivElement} */ (
            createTooltipElement("div", this.getClassName())
        );
        tooltip.hidden = true;

        this.nameEl = createTooltipElement("div", "branch-tooltip-name");
        this.targetEl = createTooltipElement("div", "branch-tooltip-target");

        tooltip.append(this.nameEl, this.targetEl);
        document.body.appendChild(tooltip);
        return tooltip;
    }

    /**
     * @returns {string} CSS class scoped to branch tooltips.
     */
    getClassName() {
        return "branch-tooltip";
    }

    /**
     * @param {import("../graph/types.js").GraphNodeBranch} node Potential branch node.
     * @returns {boolean} True when the node represents a branch with metadata.
     */
    validate(node) {
        return node && node.type === "branch" && node.branch;
    }

    /**
     * Populates tooltip with branch name and target hash.
     *
     * @param {import("../graph/types.js").GraphNodeBranch} node Branch node data.
     */
    buildContent(node) {
        this.nameEl.textContent = node.branch;
        this.targetEl.textContent = shortenHash(node.targetHash);
    }

    /**
     * @param {import("../graph/types.js").GraphNodeBranch} node Branch node used for anchoring.
     * @returns {{x: number, y: number}} Logical coordinates for tooltip placement.
     */
    getTargetPosition(node) {
        return { x: node.x, y: node.y };
    }

    /**
     * @returns {{x: number, y: number}} Tooltip offset relative to branch node.
     */
    getOffset() {
        return { x: 20, y: -10 };
    }

    /**
     * @returns {string|null} Branch name used to highlight corresponding node.
     */
    getHighlightKey() {
        return this.targetData?.branch || null;
    }
}


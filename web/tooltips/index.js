/**
 * @fileoverview Tooltip manager and registrations for the Git graph UI.
 * Coordinates specialized tooltips and exposes highlight state helpers.
 */

import { CommitTooltip } from "./commitTooltip.js";
import { BranchTooltip } from "./branchTooltip.js";

/**
 * Central coordinator that dispatches tooltip rendering based on node type.
 */
export class TooltipManager {
    /**
     * @param {HTMLCanvasElement} canvas Canvas used for coordinate conversions.
     */
    constructor(canvas) {
        this.canvas = canvas;
        this.tooltips = {
            commit: new CommitTooltip(canvas),
            branch: new BranchTooltip(canvas),
        };
        this.activeTooltip = null;
    }

    /**
     * Shows a tooltip for the provided graph node.
     *
     * @param {import("../graph/types.js").GraphNode} node Graph node detected by interaction logic.
     * @param {import("d3").ZoomTransform} zoomTransform D3 zoom transform describing current view.
     * @returns {boolean} True when a tooltip was displayed.
     */
    show(node, zoomTransform) {
        const type = node?.type;
        if (!type || !this.tooltips[type]) {
            this.hideAll();
            return false;
        }

        for (const [key, tooltip] of Object.entries(this.tooltips)) {
            if (key !== type) {
                tooltip.hide();
            }
        }

        const success = this.tooltips[type].show(node, zoomTransform);
        this.activeTooltip = success ? this.tooltips[type] : null;
        return success;
    }

    /**
     * Hides all registered tooltips.
     */
    hideAll() {
        for (const tooltip of Object.values(this.tooltips)) {
            tooltip.hide();
        }
        this.activeTooltip = null;
    }

    /**
     * Updates currently visible tooltip position.
     *
     * @param {import("d3").ZoomTransform} zoomTransform D3 zoom transform describing current view.
     */
    updatePosition(zoomTransform) {
        if (this.activeTooltip) {
            this.activeTooltip.updatePosition(zoomTransform);
        }
    }

    /**
     * @returns {string|null} Highlight key for the active tooltip or null.
     */
    getHighlightKey() {
        return this.activeTooltip?.getHighlightKey() || null;
    }

    /**
     * @returns {boolean} True when a tooltip is currently visible.
     */
    isVisible() {
        return this.activeTooltip?.visible || false;
    }

    /**
     * Determines whether the provided node corresponds to the highlighted entity.
     *
     * @param {import("../graph/types.js").GraphNode} node Graph node to compare.
     * @returns {boolean} True when the node matches the active tooltip highlight key.
     */
    isHighlighted(node) {
        const highlightKey = this.getHighlightKey();
        if (!highlightKey) {
            return false;
        }

        if (node.type === "commit") {
            return node.hash === highlightKey;
        }
        if (node.type === "branch") {
            return node.branch === highlightKey;
        }
        if (node.type === "tree") {
            return node.hash === highlightKey;
        }

        return false;
    }

    /**
     * @returns {unknown} Raw data currently used by the active tooltip.
     */
    getTargetData() {
        return this.activeTooltip?.targetData || null;
    }

    /**
     * Destroys all tooltip instances and detaches their DOM elements.
     */
    destroy() {
        for (const tooltip of Object.values(this.tooltips)) {
            tooltip.destroy();
        }
    }
}

export { CommitTooltip } from "./commitTooltip.js";
export { BranchTooltip } from "./branchTooltip.js";

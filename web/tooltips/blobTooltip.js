/**
 * @fileoverview Blob tooltip implementation for the Git graph UI.
 * Renders blob entry name, hash, and mode details.
 */

import { Tooltip, createTooltipElement } from "./baseTooltip.js";

/**
 * Tooltip that presents blob (file) metadata.
 */
export class BlobTooltip extends Tooltip {
    /**
     * @param {HTMLCanvasElement} canvas Canvas that anchors tooltip positioning.
     */
    constructor(canvas) {
        super(canvas);
    }

    /**
     * Builds the DOM structure for blob tooltips.
     *
     * @returns {HTMLDivElement} Tooltip root element appended to the document body.
     */
    createElement() {
        const tooltip = /** @type {HTMLDivElement} */ (
            createTooltipElement("div", this.getClassName())
        );
        tooltip.hidden = true;

        this.nameEl = createTooltipElement("div", "blob-tooltip-name");
        this.hashEl = createTooltipElement("div", "blob-tooltip-hash");
        this.modeEl = createTooltipElement("div", "blob-tooltip-mode");

        tooltip.append(this.nameEl, this.hashEl, this.modeEl);
        document.body.appendChild(tooltip);
        return tooltip;
    }

    /**
     * @returns {string} CSS class scoped to blob tooltips.
     */
    getClassName() {
        return "blob-tooltip";
    }

    /**
     * @param {import("../graph/types.js").GraphNodeBlob} node Potential blob node.
     * @returns {boolean} True when the node represents a blob with metadata.
     */
    validate(node) {
        return node && node.type === "blob" && node.entryName;
    }

    /**
     * Populates tooltip with blob entry name, hash, and mode.
     *
     * @param {import("../graph/types.js").GraphNodeBlob} node Blob node data.
     */
    buildContent(node) {
        this.nameEl.textContent = node.entryName;
        this.hashEl.textContent = node.hash;
        this.modeEl.textContent = node.mode ? `mode ${node.mode}` : "";
    }

    /**
     * @param {import("../graph/types.js").GraphNodeBlob} node Blob node used for anchoring.
     * @returns {{x: number, y: number}} Logical coordinates for tooltip placement.
     */
    getTargetPosition(node) {
        return { x: node.x, y: node.y };
    }

    /**
     * @returns {{x: number, y: number}} Tooltip offset relative to blob node.
     */
    getOffset() {
        return { x: 18, y: -10 };
    }

    /**
     * @returns {string|null} Blob ID used to highlight corresponding node.
     */
    getHighlightKey() {
        return this.targetData?.id || null;
    }
}

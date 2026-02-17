/**
 * @fileoverview Commit tooltip implementation for the Git graph UI.
 * Renders commit metadata within a structured tooltip layout.
 */

import { Tooltip, createTooltipElement } from "./baseTooltip.js";

/**
 * Tooltip that displays commit details such as hash, author, and message.
 */
export class CommitTooltip extends Tooltip {
    /**
     * @param {HTMLCanvasElement} canvas Canvas that anchors tooltip positioning.
     */
    constructor(canvas) {
        super(canvas);
    }

    /**
     * Builds the DOM structure comprising header and message sections.
     *
     * @returns {HTMLDivElement} Tooltip root element appended to the document body.
     */
    createElement() {
        const tooltip = /** @type {HTMLDivElement} */ (
            createTooltipElement("div", this.getClassName())
        );
        tooltip.hidden = true;

        this.headerEl = createTooltipElement("div", "commit-tooltip-header");
        this.hashEl = createTooltipElement("code", "commit-tooltip-hash");
        this.metaEl = createTooltipElement("div", "commit-tooltip-meta");

        this.headerEl.append(this.hashEl, this.metaEl);

        this.messageEl = createTooltipElement("pre", "commit-tooltip-message");

        tooltip.append(this.headerEl, this.messageEl);
        document.body.appendChild(tooltip);
        return tooltip;
    }

    /**
     * @returns {string} CSS class scoped to commit tooltips.
     */
    getClassName() {
        return "commit-tooltip";
    }

    /**
     * @param {import("../graph/types.js").GraphNodeCommit} node Potential commit node.
     * @returns {boolean} True when the node represents a commit with data.
     */
    validate(node) {
        return node && node.type === "commit" && node.commit;
    }

    /**
     * Populates tooltip with commit hash, author metadata, and message.
     *
     * @param {import("../graph/types.js").GraphNodeCommit} node Commit node data.
     */
    buildContent(node) {
        const commit = node.commit;

        this.hashEl.textContent = commit.hash;

        const metaParts = [];
        if (commit.author?.name) {
            metaParts.push(commit.author.name);
        }
        if (commit.author?.when) {
            const date = new Date(commit.author.when);
            metaParts.push(date.toLocaleString());
        }
        this.metaEl.textContent = metaParts.join(" • ");

        this.messageEl.textContent = commit.message || "(no message)";
    }

    /**
     * @param {import("../graph/types.js").GraphNodeCommit} node Commit node used for anchoring.
     * @returns {{x: number, y: number}} Logical coordinates for tooltip placement.
     */
    getTargetPosition(node) {
        return { x: node.x, y: node.y };
    }

    /**
	 * @returns {{x: number, y: number}} Tooltip offset relative to commit node.
	 */
	getOffset() {
		return { x: 24, y: -12 };
	}

	/**
     * @returns {string|null} Hash used to highlight corresponding commit node.
     */
    getHighlightKey() {
        return this.targetData?.hash || null;
    }
}


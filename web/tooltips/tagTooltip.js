/**
 * @fileoverview Tag tooltip implementation for the Git graph UI.
 * Renders tag name and target commit hash details with amber accent styling.
 */

import { Tooltip, createTooltipElement } from "./baseTooltip.js";
import { shortenHash } from "../utils/format.js";

/**
 * Tooltip that presents tag metadata.
 */
export class TagTooltip extends Tooltip {
    /**
     * @param {HTMLCanvasElement} canvas Canvas that anchors tooltip positioning.
     */
    constructor(canvas) {
        super(canvas);
    }

    /**
     * Builds the DOM structure for tag tooltips.
     *
     * @returns {HTMLDivElement} Tooltip root element appended to the document body.
     */
    createElement() {
        const tooltip = /** @type {HTMLDivElement} */ (
            createTooltipElement("div", this.getClassName())
        );
        tooltip.hidden = true;

        // Name row: tag name + copy button
        this.nameRowEl = createTooltipElement("div", "tag-tooltip-name-row");
        this.nameEl = createTooltipElement("div", "tag-tooltip-name");
        this.copyBtn = createTooltipElement("button", "tag-tooltip-copy");
        this.copyBtn.title = "Copy tag name";
        this.copyBtn.setAttribute("aria-label", "Copy tag name");
        this.copyBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
            <rect x="5" y="5" width="9" height="10" rx="1.5" stroke="currentColor" stroke-width="1.4"/>
            <path d="M3 11H2.5A1.5 1.5 0 0 1 1 9.5v-8A1.5 1.5 0 0 1 2.5 0h8A1.5 1.5 0 0 1 12 1.5V2" stroke="currentColor" stroke-width="1.4"/>
        </svg>`;
        this.nameRowEl.append(this.nameEl, this.copyBtn);

        this.targetEl = createTooltipElement("div", "tag-tooltip-target");

        tooltip.append(this.nameRowEl, this.targetEl);
        document.body.appendChild(tooltip);

        this._wireCopyButton();
        return tooltip;
    }

    /** Attaches clipboard copy handler with brief checkmark feedback. */
    _wireCopyButton() {
        const COPY_SVG = this.copyBtn.innerHTML;
        this.copyBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            const tag = this.targetData?.tag;
            if (!tag) return;
            navigator.clipboard.writeText(tag).then(() => {
                this.copyBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
                    <path d="M2 8l4 4 8-8" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>`;
                this.copyBtn.title = "Copied!";
                setTimeout(() => {
                    this.copyBtn.innerHTML = COPY_SVG;
                    this.copyBtn.title = "Copy tag name";
                }, 1500);
            }).catch(() => { /* clipboard unavailable, silently ignore */ });
        });
    }

    /**
     * @returns {string} CSS class scoped to tag tooltips.
     */
    getClassName() {
        return "tag-tooltip";
    }

    /**
     * @param {import("../graph/types.js").GraphNodeTag} node Potential tag node.
     * @returns {boolean} True when the node represents a tag with metadata.
     */
    validate(node) {
        return node && node.type === "tag" && node.tag;
    }

    /**
     * Populates tooltip with tag name and target hash.
     *
     * @param {import("../graph/types.js").GraphNodeTag} node Tag node data.
     */
    buildContent(node) {
        this.nameEl.textContent = node.tag;
        this.targetEl.textContent = shortenHash(node.targetHash);
    }

    /**
     * @param {import("../graph/types.js").GraphNodeTag} node Tag node used for anchoring.
     * @returns {{x: number, y: number}} Logical coordinates for tooltip placement.
     */
    getTargetPosition(node) {
        return { x: node.x, y: node.y };
    }

    /**
     * @returns {{x: number, y: number}} Tooltip offset relative to tag node.
     */
    getOffset() {
        return { x: 20, y: -10 };
    }

    /**
     * @returns {string|null} Tag name used to highlight corresponding node.
     */
    getHighlightKey() {
        return this.targetData?.tag || null;
    }
}

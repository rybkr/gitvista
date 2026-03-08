/**
 * @fileoverview Branch tooltip implementation for the Git graph UI.
 * Renders branch name, target commit hash, and remote/local badge.
 */

import { Tooltip, createTooltipElement } from "./baseTooltip.js";
import { shortenHash } from "../utils/format.js";
import { friendlyBranchName } from "../graph/utils/refs.js";

/**
 * Tooltip that presents branch metadata with copy-to-clipboard support.
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

        // Name row: branch name + copy button
        this.nameRowEl = createTooltipElement("div", "branch-tooltip-name-row");
        this.nameEl = createTooltipElement("div", "branch-tooltip-name");
        this.copyBtn = createTooltipElement("button", "branch-tooltip-copy");
        this.copyBtn.title = "Copy branch name";
        this.copyBtn.setAttribute("aria-label", "Copy branch name");
        this.copyBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
            <rect x="5" y="5" width="9" height="10" rx="1.5" stroke="currentColor" stroke-width="1.4"/>
            <path d="M3 11H2.5A1.5 1.5 0 0 1 1 9.5v-8A1.5 1.5 0 0 1 2.5 0h8A1.5 1.5 0 0 1 12 1.5V2" stroke="currentColor" stroke-width="1.4"/>
        </svg>`;
        this.nameRowEl.append(this.nameEl, this.copyBtn);

        // Badge row: remote/local indicator
        this.badgeEl = createTooltipElement("span", "branch-tooltip-badge");

        this.targetEl = createTooltipElement("div", "branch-tooltip-target");

        tooltip.append(this.nameRowEl, this.badgeEl, this.targetEl);
        document.body.appendChild(tooltip);

        this._wireCopyButton();
        return tooltip;
    }

    /** Attaches clipboard copy handler with brief checkmark feedback. */
    _wireCopyButton() {
        const COPY_SVG = this.copyBtn.innerHTML;
        this.copyBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            const branch = this.targetData?.branch;
            if (!branch) return;
            navigator.clipboard.writeText(branch).then(() => {
                this.copyBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
                    <path d="M2 8l4 4 8-8" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>`;
                this.copyBtn.title = "Copied!";
                setTimeout(() => {
                    this.copyBtn.innerHTML = COPY_SVG;
                    this.copyBtn.title = "Copy branch name";
                }, 1500);
            }).catch(() => { /* clipboard unavailable, silently ignore */ });
        });
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
     * Populates tooltip with branch name, badge, and target hash.
     *
     * @param {import("../graph/types.js").GraphNodeBranch} node Branch node data.
     */
    buildContent(node) {
        this.nameEl.textContent = friendlyBranchName(node.branch);
        this.targetEl.textContent = shortenHash(node.targetHash);

        const isRemote = node.branch?.startsWith("refs/remotes/");
        this.badgeEl.textContent = isRemote ? "remote" : "local";
        this.badgeEl.className = isRemote
            ? "branch-tooltip-badge branch-tooltip-badge--remote"
            : "branch-tooltip-badge branch-tooltip-badge--local";
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

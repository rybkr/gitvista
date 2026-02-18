/**
 * @fileoverview Commit tooltip implementation for the Git graph UI.
 * Renders commit metadata within a structured tooltip layout.
 * Includes Prev/Next navigation buttons wired via setNavigate().
 */

import { Tooltip, createTooltipElement } from "./baseTooltip.js";
import { formatRelativeTime } from "../utils/format.js";

/**
 * Tooltip that displays commit details such as hash, author, and message.
 *
 * Navigation support: call `setNavigate(fn)` after construction to wire the
 * Prev and Next buttons. The callback receives `'prev'` or `'next'` and is
 * expected to call `navigateCommits()` on the graph controller.
 */
export class CommitTooltip extends Tooltip {
    /**
     * @param {HTMLCanvasElement} canvas Canvas that anchors tooltip positioning.
     */
    constructor(canvas) {
        super(canvas);
        // _navigate is set post-construction via setNavigate().
        this._navigate = null;
    }

    /**
     * Wires the Prev/Next buttons to a navigation callback.
     * Must be called after construction; this design avoids the super()-before-this
     * constraint when passing callbacks through the base class constructor.
     *
     * @param {(direction: 'prev' | 'next') => void} fn Callback invoked on button click.
     */
    setNavigate(fn) {
        this._navigate = fn;
        // Reflect the new state on already-created buttons.
        if (this.prevBtn) {
            this.prevBtn.disabled = false;
            this.nextBtn.disabled = false;
        }
    }

    /**
     * Builds the DOM structure: header (hash + meta), message body, and nav buttons.
     *
     * @returns {HTMLDivElement} Tooltip root element appended to the document body.
     */
    createElement() {
        const tooltip = /** @type {HTMLDivElement} */ (
            createTooltipElement("div", this.getClassName())
        );
        tooltip.hidden = true;

        this.headerEl = createTooltipElement("div", "commit-tooltip-header");

        // Hash row: abbreviated hash + copy button
        this.hashRowEl = createTooltipElement("div", "commit-tooltip-hash-row");
        this.hashRowEl.style.cssText = "display:flex;align-items:center;gap:6px;";
        this.hashEl = createTooltipElement("code", "commit-tooltip-hash");
        this.copyBtn = createTooltipElement("button", "commit-tooltip-copy");
        this.copyBtn.title = "Copy full hash";
        this.copyBtn.setAttribute("aria-label", "Copy commit hash");
        this.copyBtn.style.cssText = `
            background:none;border:none;cursor:pointer;padding:2px;
            color:var(--text-secondary,#656d76);display:flex;align-items:center;
        `;
        this.copyBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
            <rect x="5" y="5" width="9" height="10" rx="1.5" stroke="currentColor" stroke-width="1.4"/>
            <path d="M3 11H2.5A1.5 1.5 0 0 1 1 9.5v-8A1.5 1.5 0 0 1 2.5 0h8A1.5 1.5 0 0 1 12 1.5V2" stroke="currentColor" stroke-width="1.4"/>
        </svg>`;
        this.hashRowEl.append(this.hashEl, this.copyBtn);

        this.metaEl = createTooltipElement("div", "commit-tooltip-meta");
        this.headerEl.append(this.hashRowEl, this.metaEl);

        this.messageEl = createTooltipElement("pre", "commit-tooltip-message");

        // Navigation row — CSS restores pointer-events so buttons are clickable
        // despite the parent tooltip being pointer-events: none.
        this.navEl = createTooltipElement("div", "commit-tooltip-nav");

        this.prevBtn = /** @type {HTMLButtonElement} */ (
            createTooltipElement("button", "commit-tooltip-nav-btn")
        );
        this.prevBtn.textContent = "\u2190 Prev";
        this.prevBtn.type = "button";
        this.prevBtn.disabled = true; // enabled once setNavigate() is called

        this.nextBtn = /** @type {HTMLButtonElement} */ (
            createTooltipElement("button", "commit-tooltip-nav-btn")
        );
        this.nextBtn.textContent = "Next \u2192";
        this.nextBtn.type = "button";
        this.nextBtn.disabled = true; // enabled once setNavigate() is called

        this.prevBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            this._navigate?.("prev");
        });
        this.nextBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            this._navigate?.("next");
        });

        this.navEl.append(this.prevBtn, this.nextBtn);
        tooltip.append(this.headerEl, this.messageEl, this.navEl);
        document.body.appendChild(tooltip);

        this._wireCopyButton();
        return tooltip;
    }

    /** Attaches clipboard copy handler with brief checkmark feedback. */
    _wireCopyButton() {
        const COPY_SVG = this.copyBtn.innerHTML;
        this.copyBtn.addEventListener("click", (e) => {
            e.stopPropagation();
            const hash = this.targetData?.commit?.hash;
            if (!hash) return;
            navigator.clipboard.writeText(hash).then(() => {
                this.copyBtn.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
                    <path d="M2 8l4 4 8-8" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                </svg>`;
                this.copyBtn.title = "Copied!";
                setTimeout(() => {
                    this.copyBtn.innerHTML = COPY_SVG;
                    this.copyBtn.title = "Copy full hash";
                }, 1500);
            }).catch(() => { /* clipboard unavailable, silently ignore */ });
        });
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
     * Also updates button enabled state based on whether a navigate callback is wired.
     *
     * @param {import("../graph/types.js").GraphNodeCommit} node Commit node data.
     */
    buildContent(node) {
        const commit = node.commit;

        this.hashEl.textContent = commit.hash.slice(0, 7);
        this.hashEl.title = commit.hash;

        const metaParts = [];
        if (commit.author?.name) {
            metaParts.push(commit.author.name);
        }
        if (commit.author?.when) {
            const relative = formatRelativeTime(commit.author.when);
            const absolute = new Date(commit.author.when).toLocaleString();
            metaParts.push(`${relative} (${absolute})`);
        }
        this.metaEl.textContent = metaParts.join(" \u2022 ");

        this.messageEl.textContent = commit.message || "(no message)";

        // Keep button state consistent: enabled when navigate is wired.
        const hasNav = typeof this._navigate === "function";
        this.prevBtn.disabled = !hasNav;
        this.nextBtn.disabled = !hasNav;
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
     * @returns {string|null} Hash used to highlight the corresponding commit node.
     */
    getHighlightKey() {
        return this.targetData?.hash || null;
    }
}


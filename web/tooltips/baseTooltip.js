/**
 * @fileoverview Shared tooltip base class and helper utilities for the Git graph UI.
 * Encapsulates DOM element handling and positioning logic common to tooltips.
 */

/**
 * Creates an HTML element with the provided tag name and class.
 *
 * @param {string} tagName The HTML tag to instantiate.
 * @param {string} [className] Optional CSS class to assign to the element.
 * @returns {HTMLElement} A newly created DOM node ready for configuration.
 */
export function createTooltipElement(tagName, className) {
    const element = document.createElement(tagName);

    if (className) {
        element.className = className;
    }

    return element;
}

/**
 * Base tooltip class responsible for DOM lifecycle management and positioning.
 */
export class Tooltip {
    /**
     * Creates a tooltip bound to a canvas element.
     *
     * @param {HTMLCanvasElement} canvas Canvas used to derive positioning.
     */
    constructor(canvas) {
        this.canvas = canvas;
        this.element = this.createElement();
        this.visible = false;
        this.targetData = null;
    }

    /**
     * Constructs the tooltip container placed in the document body.
     *
     * @returns {HTMLDivElement} Container element displayed when the tooltip is visible.
     */
    createElement() {
        const tooltip = /** @type {HTMLDivElement} */ (
            createTooltipElement("div", this.getClassName())
        );

        tooltip.hidden = true;
        document.body.appendChild(tooltip);
        return tooltip;
    }

    /**
     * Displays the tooltip for the provided data if valid.
     *
     * @param {unknown} data Node or entity to represent.
     * @param {import("d3").ZoomTransform} zoomTransform D3 zoom transform used for positioning.
     * @returns {boolean} True when content is shown successfully.
     */
    show(data, zoomTransform) {
        if (!this.validate(data)) {
            this.hide();
            return false;
        }

        this.targetData = data;
        this.buildContent(data);
        this.visible = true;
        this.element.hidden = false;
        this.element.style.display = "flex";
        this.element.style.opacity = "1";
        this.updatePosition(zoomTransform);
        return true;
    }

    /**
     * Hides the tooltip and clears internal references.
     */
    hide() {
        if (!this.visible) {
            return;
        }

        this.element.hidden = true;
        this.element.style.display = "none";
        this.element.style.opacity = "0";
        this.visible = false;
        this.targetData = null;
        this.onHide();
    }

    /**
     * Moves the tooltip element relative to the zoomed canvas coordinate.
     *
     * @param {import("d3").ZoomTransform} zoomTransform D3 zoom transform describing current view.
     */
    updatePosition(zoomTransform) {
        if (!this.visible || !this.targetData) {
            return;
        }

        const { x, y } = this.getTargetPosition(this.targetData);
        // zoomTransform.apply([x, y]) -> projects logical graph coordinates to screen coordinates.
        const [tx, ty] = zoomTransform.apply([x, y]);
        const canvasRect = this.canvas.getBoundingClientRect();

        const offset = this.getOffset();
        const left = canvasRect.left + tx + offset.x;
        const top = canvasRect.top + ty + offset.y;

        this.element.style.transform = `translate(${left}px, ${top}px)`;
    }

    /**
     * Removes the tooltip element from the document.
     */
    destroy() {
        this.element.remove();
    }

    /**
     * Returns the CSS class name applied to the tooltip container.
     *
     * @returns {string} Tooltip class name.
     */
    getClassName() {
        return "graph-tooltip";
    }

    /**
     * Verifies the provided data is suitable for display.
     *
     * @param {unknown} data Candidate data payload.
     * @returns {boolean} True when the tooltip should render.
     */
    validate(data) {
        return !!data;
    }

    /**
     * Populates tooltip content using the current target data.
     *
     * @param {unknown} _data Current tooltip payload.
     */
    // eslint-disable-next-line class-methods-use-this, no-unused-vars
    buildContent(_data) { }

    /**
     * Returns the logical graph coordinates for the tooltip anchor.
     *
     * @param {unknown} _data Current tooltip payload.
     * @returns {{x: number, y: number}} Position in graph space.
     */
    // eslint-disable-next-line class-methods-use-this, no-unused-vars
    getTargetPosition(_data) {
        return { x: 0, y: 0 };
    }

    /**
     * Provides pixel offsets applied after coordinate conversion.
     *
     * @returns {{x: number, y: number}} Offset in screen pixels.
     */
    // eslint-disable-next-line class-methods-use-this
    getOffset() {
        return { x: 0, y: 0 };
    }

    /**
     * Hook invoked when the tooltip transitions to hidden state.
     */
    // eslint-disable-next-line class-methods-use-this
    onHide() { }

    /**
     * Returns identifier used to highlight matching entities on the graph.
     *
     * @returns {string|null} Highlight key or null when unavailable.
     */
    // eslint-disable-next-line class-methods-use-this
    getHighlightKey() {
        return null;
    }
}


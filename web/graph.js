/**
 * @fileoverview Entry point for the Git graph visualization.
 * Exposes the public factory that wires up the modular graph controller.
 */

import { createGraphController } from "./graph/graphController.js";

/**
 * Creates the graph experience within the provided root element.
 *
 * @param {HTMLElement} rootElement Container that will host the graph canvas.
 * @param {object} [options] Options forwarded to the graph controller.
 * @param {function} [options.onCommitTreeClick] Called when a commit's tree icon is clicked.
 * @param {function} [options.onCommitSelect] Called with the selected commit hash (or null).
 * @returns {{
 *   applyDelta(delta: unknown): void,
 *   centerOnCommit(hash: string | null): void,
 *   navigateCommits(direction: 'prev' | 'next'): void,
 *   selectAndCenter(hash: string): void,
 *   getHeadHash(): string | null,
 *   setHeadHash(hash: string | null): void,
 *   destroy(): void,
 * }} Public graph API surface.
 */
export function createGraph(rootElement, options) {
    return createGraphController(rootElement, options);
}


import { createGraphController } from "./graph/graphController.js";

/** Thin wrapper that creates and exposes the graph controller. */
export function createGraph(rootElement, options) {
    return createGraphController(rootElement, options);
}


/**
 * @fileoverview Factory helpers for instantiating and mutating graph state.
 * Provides a central definition for shared state across controller modules.
 */

import * as d3 from "https://cdn.jsdelivr.net/npm/d3@7.9.0/+esm";

/**
 * Creates the default graph state container.
 *
 * @returns {import("../types.js").GraphState} Initialized graph state object.
 */
export function createGraphState() {
	return {
		commits: new Map(),
		branches: new Map(),
		nodes: [],
		links: [],
		zoomTransform: d3.zoomIdentity,
		layoutMode: "force", // Current layout mode: "force" or "lane"
		searchQuery: "",
		filterState: { hideRemotes: false, hideMerges: false, hideStashes: false, focusBranch: "" },
		filterPredicate: null, // Derived: compiled from searchQuery + filterState; not serializable
		stashes: [],
		hoverNode: null,
		headHash: "",
		tags: new Map(),
	};
}

/**
 * Records the current zoom transform on the state object.
 *
 * @param {import("../types.js").GraphState} state Graph state being updated.
 * @param {import("d3").ZoomTransform} transform D3 zoom transform emitted by zoom behavior.
 */
export function setZoomTransform(state, transform) {
	state.zoomTransform = transform;
}

/**
 * @fileoverview Factory helpers for instantiating and mutating graph state.
 * Provides a central definition for shared state across controller modules.
 */

import { zoomIdentity } from "/vendor/d3-minimal.js";

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
		zoomTransform: zoomIdentity,
		layoutMode: "force", // Current layout mode: "force" or "lane"
		// Structured search state produced by searchQuery.js.
		// null means no active search; object contains { query, matcher }.
		searchState: null,
		filterState: { hideRemotes: false, hideMerges: false, hideStashes: false, focusBranch: "" },
		filterPredicate: null, // Derived: compiled from searchState + filterState; not serializable
		stashes: [],
		hoverNode: null,
		headHash: "",
		tags: new Map(),
		isolatedLanePosition: null,
		mergePreview: null,
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

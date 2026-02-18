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
		headHash: "",
		tags: new Map(),      // tag name -> commit hash
		stashes: [],          // StashEntry[]
		hoverNode: null,      // node under the pointer (no click required)
		// A2: text search query string, or "" when no search is active.
		searchQuery: "",
		// A3: structural filter toggles. The controller initialises this from
		// localStorage via graphFilters.loadFilterState() immediately after
		// createGraphState() is called, so the default here is "show everything".
		filterState: {
			hideRemotes: false,
			hideMerges: false,
			hideStashes: false,
			focusBranch: "",
		},
		// Compound predicate built from searchQuery + filterState by
		// buildFilterPredicate in graphController.  null = no active filter.
		filterPredicate: null, // ((node) => boolean) | null
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

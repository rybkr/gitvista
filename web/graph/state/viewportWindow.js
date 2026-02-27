/**
 * @fileoverview Viewport query with hysteresis for lazy node materialization.
 * Given a D3 zoom transform, computes which CommitIndex entries should be
 * materialized, using buffering and hysteresis to avoid thrashing.
 */

export class ViewportWindow {
	/**
	 * @param {import("./commitIndex.js").CommitIndex} commitIndex
	 * @param {Object} [opts]
	 * @param {number} [opts.bufferFactor=1.5] Extra viewport heights to buffer above/below.
	 * @param {number} [opts.hysteresisMargin=200] Graph-space pixels inside last query bounds before re-query.
	 */
	constructor(commitIndex, { bufferFactor = 1.5, hysteresisMargin = 200 } = {}) {
		this._commitIndex = commitIndex;
		this._bufferFactor = bufferFactor;
		this._hysteresisMargin = hysteresisMargin;

		/** @type {import("./commitIndex.js").CommitEntry[]} */
		this._cachedEntries = [];
		this._lastQueryYMin = -Infinity;
		this._lastQueryYMax = Infinity;
		this._valid = false;
	}

	/**
	 * Recompute the visible entry set based on the current zoom transform.
	 *
	 * @param {Object} zoomTransform D3 zoom transform ({ x, y, k }).
	 * @param {number} viewportWidth CSS pixels.
	 * @param {number} viewportHeight CSS pixels.
	 * @returns {{ entries: import("./commitIndex.js").CommitEntry[], changed: boolean }}
	 */
	update(zoomTransform, viewportWidth, viewportHeight) {
		const k = zoomTransform.k || 1;

		// Convert viewport edges to graph-space Y
		const displayYMin = -zoomTransform.y / k;
		const displayYMax = (-zoomTransform.y + viewportHeight) / k;
		const viewportH = viewportHeight / k;

		// Check hysteresis: if the display edges are well within the last
		// query bounds, skip the re-query.
		if (this._valid) {
			const margin = this._hysteresisMargin;
			if (
				displayYMin > this._lastQueryYMin + margin &&
				displayYMax < this._lastQueryYMax - margin
			) {
				return { entries: this._cachedEntries, changed: false };
			}
		}

		// Compute buffered query range
		const buffer = viewportH * this._bufferFactor;
		const queryYMin = displayYMin - buffer;
		const queryYMax = displayYMax + buffer;

		this._cachedEntries = this._commitIndex.queryYRange(queryYMin, queryYMax);
		this._lastQueryYMin = queryYMin;
		this._lastQueryYMax = queryYMax;
		this._valid = true;

		return { entries: this._cachedEntries, changed: true };
	}

	/**
	 * Force a fresh query on the next update() call.
	 * Call after layout recompute or delta application.
	 */
	invalidate() {
		this._valid = false;
	}

	/**
	 * Returns the current visible set without recomputing.
	 * @returns {import("./commitIndex.js").CommitEntry[]}
	 */
	getCurrentEntries() {
		return this._cachedEntries;
	}
}

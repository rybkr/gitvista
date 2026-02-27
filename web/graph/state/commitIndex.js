/**
 * @fileoverview Y-sorted position index for all commits in lane mode.
 * Supports O(log N) Y-range queries via binary search, enabling the
 * viewport window to efficiently determine which commits are visible.
 */

import { LANE_COLORS } from "../constants.js";

/**
 * @typedef {Object} CommitEntry
 * @property {string} hash
 * @property {number} x
 * @property {number} y
 * @property {number} laneIndex
 * @property {string} laneColor
 * @property {string} segmentId
 * @property {boolean} isStash
 * @property {boolean} isStashInternal
 * @property {string|null} stashInternalKind
 * @property {string|null} stashMessage
 */

export class CommitIndex {
	constructor() {
		/** @type {CommitEntry[]} Sorted by Y ascending */
		this._entries = [];
		/** @type {Map<string, CommitEntry>} Hash → entry for O(1) lookup */
		this._byHash = new Map();
	}

	/**
	 * Rebuilds the index from laneStrategy position data.
	 *
	 * @param {Map<string, Object>} commits Full commit data map.
	 * @param {Object} positionData From laneStrategy.getPositionData().
	 * @param {Array<Object>} stashes Stash list from state.
	 */
	rebuild(commits, positionData, stashes) {
		const {
			transitionTargetPositions,
			commitToLane,
			commitToSegmentId,
		} = positionData;

		// Build stash lookups
		const stashMessages = new Map();
		const stashInternalHashes = new Set();
		const stashInternalKinds = new Map();
		for (const s of stashes ?? []) {
			if (s?.hash) stashMessages.set(s.hash, s.message);
		}
		for (const s of stashes ?? []) {
			const commit = commits.get(s?.hash);
			if (commit?.parents) {
				for (let i = 1; i < commit.parents.length; i++) {
					stashInternalHashes.add(commit.parents[i]);
					stashInternalKinds.set(commit.parents[i], i === 1 ? "index" : "untracked");
				}
			}
		}

		this._entries = [];
		this._byHash.clear();

		for (const [hash, pos] of transitionTargetPositions) {
			const laneIndex = commitToLane.get(hash) ?? 0;
			const entry = {
				hash,
				x: pos.x,
				y: pos.y,
				laneIndex,
				laneColor: LANE_COLORS[laneIndex % LANE_COLORS.length],
				segmentId: commitToSegmentId.get(hash) ?? "",
				isStash: stashMessages.has(hash),
				isStashInternal: stashInternalHashes.has(hash),
				stashInternalKind: stashInternalKinds.get(hash) ?? null,
				stashMessage: stashMessages.get(hash) ?? null,
			};
			this._entries.push(entry);
			this._byHash.set(hash, entry);
		}

		// Sort by Y ascending for binary search
		this._entries.sort((a, b) => a.y - b.y);
	}

	/**
	 * Binary search range query for entries within [yMin, yMax].
	 * O(log N + K) where K is the number of results.
	 *
	 * @param {number} yMin
	 * @param {number} yMax
	 * @returns {CommitEntry[]}
	 */
	queryYRange(yMin, yMax) {
		const entries = this._entries;
		const len = entries.length;
		if (len === 0) return [];

		// Binary search for first entry with y >= yMin
		let lo = 0, hi = len;
		while (lo < hi) {
			const mid = (lo + hi) >>> 1;
			if (entries[mid].y < yMin) lo = mid + 1;
			else hi = mid;
		}
		const start = lo;

		// Collect entries until y > yMax
		const result = [];
		for (let i = start; i < len; i++) {
			if (entries[i].y > yMax) break;
			result.push(entries[i]);
		}
		return result;
	}

	/**
	 * O(1) lookup by hash.
	 * @param {string} hash
	 * @returns {CommitEntry|null}
	 */
	getByHash(hash) {
		return this._byHash.get(hash) ?? null;
	}

	/** @returns {number} Total entry count */
	get size() {
		return this._entries.length;
	}

	/**
	 * @returns {{minY: number, maxY: number}|null}
	 */
	getBounds() {
		if (this._entries.length === 0) return null;
		return {
			minY: this._entries[0].y,
			maxY: this._entries[this._entries.length - 1].y,
		};
	}

	/**
	 * Returns the full sorted array. Used by search and minimap.
	 * @returns {CommitEntry[]}
	 */
	getAllEntries() {
		return this._entries;
	}
}

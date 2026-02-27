/**
 * @fileoverview Node lifecycle manager for lazy commit materialization.
 * Creates/recycles GraphNode objects as commits enter/leave the viewport.
 * Object pooling reduces GC pressure for smooth scrolling.
 */

import { NODE_RADIUS } from "../constants.js";

export class NodeMaterializer {
	/**
	 * @param {Object} [opts]
	 * @param {number} [opts.poolCapacity=500] Max recycled objects to keep in pool.
	 */
	constructor({ poolCapacity = 500 } = {}) {
		this._poolCapacity = poolCapacity;
		/** @type {Object[]} Recycled node objects */
		this._pool = [];
		/** @type {Map<string, Object>} Hash → materialized GraphNode */
		this._active = new Map();
	}

	/**
	 * Synchronize materialized nodes with the desired visible set.
	 *
	 * @param {import("./commitIndex.js").CommitEntry[]} visibleEntries Entries to materialize.
	 * @param {Map<string, Object>} commits Full commit data map.
	 * @returns {{ nodes: Object[], added: string[], removed: string[] }}
	 */
	synchronize(visibleEntries, commits) {
		const desiredHashes = new Set(visibleEntries.map(e => e.hash));
		const added = [];
		const removed = [];

		// Evict nodes no longer in the desired set
		for (const [hash, node] of this._active) {
			if (!desiredHashes.has(hash)) {
				this._evict(node);
				this._active.delete(hash);
				removed.push(hash);
			}
		}

		// Materialize or update desired entries
		for (const entry of visibleEntries) {
			let node = this._active.get(entry.hash);
			if (!node) {
				node = this._acquire();
				node.dimPhase = 0;
				node.dimTarget = 0;
				this._active.set(entry.hash, node);
				added.push(entry.hash);
			}
			// Apply entry data
			node.type = "commit";
			node.hash = entry.hash;
			node.x = entry.x;
			node.y = entry.y;
			node.vx = 0;
			node.vy = 0;
			node.laneIndex = entry.laneIndex;
			node.laneColor = entry.laneColor;
			node.isStash = entry.isStash;
			node.isStashInternal = entry.isStashInternal;
			node.stashInternalKind = entry.stashInternalKind;
			node.stashMessage = entry.stashMessage;
			node.radius = node.radius ?? NODE_RADIUS;
			// Attach commit data
			node.commit = commits.get(entry.hash) ?? null;
		}

		return {
			nodes: Array.from(this._active.values()),
			added,
			removed,
		};
	}

	/**
	 * O(1) lookup for an existing materialized node.
	 * @param {string} hash
	 * @returns {Object|null}
	 */
	getNode(hash) {
		return this._active.get(hash) ?? null;
	}

	/**
	 * Force-materialize a single commit (e.g. for navigation to off-screen commit).
	 *
	 * @param {string} hash
	 * @param {import("./commitIndex.js").CommitEntry} entry
	 * @param {Object} commit
	 * @returns {Object} The materialized GraphNode.
	 */
	forceMaterialize(hash, entry, commit) {
		let node = this._active.get(hash);
		if (!node) {
			node = this._acquire();
			node.dimPhase = 0;
			node.dimTarget = 0;
			this._active.set(hash, node);
		}
		node.type = "commit";
		node.hash = hash;
		node.x = entry.x;
		node.y = entry.y;
		node.vx = 0;
		node.vy = 0;
		node.laneIndex = entry.laneIndex;
		node.laneColor = entry.laneColor;
		node.isStash = entry.isStash;
		node.isStashInternal = entry.isStashInternal;
		node.stashInternalKind = entry.stashInternalKind;
		node.stashMessage = entry.stashMessage;
		node.radius = node.radius ?? NODE_RADIUS;
		node.commit = commit ?? null;
		return node;
	}

	/**
	 * Returns a snapshot array of all currently materialized nodes.
	 * @returns {Object[]}
	 */
	getMaterializedNodes() {
		return Array.from(this._active.values());
	}

	/**
	 * Evict all active nodes and return them to the pool.
	 * Called on mode switch to force → lane or vice versa.
	 */
	clear() {
		for (const node of this._active.values()) {
			this._evict(node);
		}
		this._active.clear();
	}

	/**
	 * Acquire a node object from the pool or create a new one.
	 * @returns {Object}
	 */
	_acquire() {
		if (this._pool.length > 0) {
			return this._pool.pop();
		}
		return {
			type: "commit",
			hash: "",
			x: 0,
			y: 0,
			vx: 0,
			vy: 0,
			commit: null,
			radius: NODE_RADIUS,
		};
	}

	/**
	 * Return a node to the pool after clearing references.
	 * @param {Object} node
	 */
	_evict(node) {
		node.commit = null;
		node.hash = "";
		node.isStash = false;
		node.isStashInternal = false;
		node.stashInternalKind = null;
		node.stashMessage = null;
		delete node.spawnPhase;
		delete node.dimPhase;
		delete node.dimTarget;
		delete node.dimmed;
		if (this._pool.length < this._poolCapacity) {
			this._pool.push(node);
		}
	}
}

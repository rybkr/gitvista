/**
 * @fileoverview Lane-based layout strategy for Git commit graph.
 * Implements git-style swimlane layout with chronological ordering.
 *
 * Conforms to LayoutStrategy interface defined in layoutStrategy.js.
 *
 * Algorithm:
 * 1. Walk first-parent chains from branch tips (main first) to claim ownership
 * 2. Assign remaining commits (merge parents) via column-reuse
 * 3. Position nodes: Y = chronological, X = lane index
 * 4. Animate smooth transitions between modes
 */

import {
	LANE_WIDTH,
	LANE_MARGIN,
	LANE_VERTICAL_STEP,
	LANE_TRANSITION_DURATION,
	LANE_COLORS,
	LANE_HEADER_HEIGHT,
	TIMELINE_PADDING,
	TIMELINE_MARGIN,
} from "../constants.js";
import { getCommitTimestamp } from "../utils/time.js";

/**
 * Lane-based layout strategy implementation.
 * @implements {LayoutStrategy}
 */
export class LaneStrategy {
	constructor(options = {}) {
		/** @type {Function|null} Callback invoked each animation frame (mirrors ForceStrategy pattern) */
		this._onTick = options.onTick || null;

		/** @type {Map<string, number>} Map from commit hash to lane index */
		this.commitToLane = new Map();

		/** @type {number|null} Animation frame ID for transitions */
		this.animationFrameId = null;

		/** @type {number} Timestamp when transition animation started */
		this.transitionStartTime = 0;

		/** @type {Map<string, {x: number, y: number}>} Start positions for transition */
		this.transitionStartPositions = new Map();

		/** @type {Map<string, {x: number, y: number}>} Target positions for transition */
		this.transitionTargetPositions = new Map();

		/** @type {boolean} Whether we're currently animating a transition */
		this.isTransitioning = false;

		/** @type {number} Viewport height for layout calculations */
		this.viewportHeight = 0;

		/** @type {Array<{index: number, color: string, branchName: string, minY: number, maxY: number}>} */
		this._laneInfo = [];

		/** @type {Array<number>} Permutation mapping logical lane index → display position */
		this.laneOrder = [];

		/** @type {Map<string, Object>|null} Cached commits map for swap recomputation */
		this._commits = null;

		/** @type {Map<string, string>|null} Cached branches map for swap recomputation */
		this._branches = null;

		/** @type {Array<string>} Branch name owning each logical lane (index → name) */
		this._laneOwners = [];
	}

	/**
	 * @type {boolean}
	 */
	get supportsRebalance() {
		return false; // Deterministic layout doesn't need rebalancing
	}

	/**
	 * Activate the lane layout strategy.
	 * Computes lane assignments and starts transition animation from current positions.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Array<Object>} links Array of graph links
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 * @param {Map<string, string>} branches Map of branch name to target hash
	 * @param {Object} viewport Current viewport state
	 */
	activate(nodes, links, commits, branches, viewport) {
		this.nodes = nodes; // Store reference for cleanup in deactivate()
		this._commits = commits;
		this._branches = branches;
		this.viewportHeight = viewport.height || 800;

		// Compute lane assignments
		this.assignLanes(nodes, commits, branches);

		// Compute target positions for all nodes
		this.computeTargetPositions(nodes, commits);

		// Build lane info for rendering backgrounds and headers
		this._buildLaneInfo(nodes, branches, commits);

		// Store current positions as start positions
		this.transitionStartPositions.clear();
		for (const node of nodes) {
			if (node.type === "commit") {
				this.transitionStartPositions.set(node.hash, { x: node.x, y: node.y });
			}
		}

		// Start transition animation
		this.startTransition();
	}

	/**
	 * Deactivate the lane layout strategy.
	 * Cancels any pending animations and cleans up resources.
	 * Removes lane-specific properties from nodes to ensure clean state.
	 */
	deactivate() {
		this.stopTransition();
		this.commitToLane.clear();
		this.transitionStartPositions.clear();
		this.transitionTargetPositions.clear();
		this.laneOrder = [];
		this._commits = null;
		this._branches = null;
		this._laneOwners = [];

		// Clear lane-specific properties from shared node objects
		if (this.nodes) {
			for (const node of this.nodes) {
				delete node.laneColor;
				delete node.laneIndex;
			}
			this.nodes = null;
		}
	}

	/**
	 * Update the layout when graph data changes.
	 * Recomputes lane assignments and positions for new/changed commits.
	 *
	 * @param {Array<Object>} nodes Updated nodes array
	 * @param {Array<Object>} links Updated links array
	 * @param {Map<string, Object>} commits Updated commits map
	 * @param {Map<string, string>} branches Updated branches map
	 * @param {Object} viewport Current viewport state
	 */
	updateGraph(nodes, links, commits, branches, viewport) {
		this._commits = commits;
		this._branches = branches;
		this.viewportHeight = viewport.height || 800;

		// Recompute lane assignments
		this.assignLanes(nodes, commits, branches);

		// Update target positions
		this.computeTargetPositions(nodes, commits);

		// Rebuild lane info for rendering
		this._buildLaneInfo(nodes, branches, commits);

		// Apply positions immediately (no transition for incremental updates)
		this.applyTargetPositions(nodes);
	}

	/**
	 * Handle node drag interaction.
	 * Lane layout is deterministic, so dragging is disabled.
	 *
	 * @param {Object} node The node being dragged
	 * @param {number} x New x position
	 * @param {number} y New y position
	 * @returns {boolean} Always returns false (drag not supported)
	 */
	handleDrag(node, x, y) {
		return false; // Dragging disabled in lane mode
	}

	/**
	 * Handle end of node drag interaction.
	 *
	 * @param {Object} node The node that was dragged
	 */
	handleDragEnd(node) {
		// No-op: dragging is disabled
	}

	/**
	 * Animation frame tick callback.
	 * Updates transition animation progress and interpolates node positions.
	 *
	 * @returns {boolean} True if render needed, false otherwise
	 */
	tick() {
		if (!this.isTransitioning || !this.nodes) {
			return false;
		}

		const elapsed = performance.now() - this.transitionStartTime;
		const progress = Math.min(1, elapsed / LANE_TRANSITION_DURATION);

		// Interpolate node positions during transition
		this.interpolatePositions(this.nodes, progress);

		// Check if animation complete
		if (progress >= 1) {
			this.stopTransition();
			return true; // One final render
		}

		return true; // Render needed during animation
	}

	/**
	 * Rebalance the layout (not supported for lane strategy).
	 */
	rebalance() {
		// No-op: lane layout is deterministic
	}

	/**
	 * Find the logical center position for viewport centering.
	 * Returns the position of the latest commit (top of timeline).
	 *
	 * @param {Array<Object>} nodes Current nodes array
	 * @returns {{x: number, y: number}|null} Center coordinates or null
	 */
	findCenterTarget(nodes) {
		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return null;

		// Find newest commit (smallest y value in our top-down layout)
		let latest = commitNodes[0];
		let bestTime = getCommitTimestamp(latest.commit);

		for (const node of commitNodes) {
			const time = getCommitTimestamp(node.commit);
			if (time > bestTime) {
				bestTime = time;
				latest = node;
			}
		}

		return { x: latest.x, y: latest.y };
	}

	/**
	 * Assigns commits to lanes using a first-parent ownership heuristic.
	 *
	 * Phase 1 — First-parent chains:
	 *   Walk the first-parent chain from each branch tip in priority order
	 *   (main/master/trunk first, then alphabetical). Each chain claims its
	 *   commits for a dedicated lane. Walking stops when a commit already
	 *   claimed by a higher-priority branch is reached, so shared ancestors
	 *   (like the init commit) are attributed to the most important branch.
	 *
	 * Phase 2 — Remaining commits:
	 *   Any commits not on a first-parent chain (merge parents reachable only
	 *   via non-first-parent edges) are assigned via column-reuse: lowest
	 *   free lane, with active-lane tracking for convergence.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 * @param {Map<string, string>} branches Map of branch name to target hash
	 */
	assignLanes(nodes, commits, branches) {
		this.commitToLane.clear();

		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return;

		const commitHashes = new Set(commitNodes.map((n) => n.hash));

		// --- Phase 1: First-parent chain ownership ---
		const sortedBranches = this._prioritizeBranches(branches);
		const commitOwner = new Map(); // hash → lane index
		let nextLane = 0;
		const newLaneOwners = [];

		for (const [branchName, tipHash] of sortedBranches) {
			let current = tipHash;
			const chain = [];

			// Follow first-parent links until we reach a commit already owned
			while (current && commitHashes.has(current) && !commitOwner.has(current)) {
				chain.push(current);
				const commit = commits.get(current);
				current = commit?.parents?.[0] ?? null;
			}

			if (chain.length === 0) continue;

			const lane = nextLane++;
			newLaneOwners.push(branchName);
			for (const hash of chain) {
				commitOwner.set(hash, lane);
				this.commitToLane.set(hash, lane);
			}
		}

		// --- Phase 2: Assign remaining commits (merge parents, orphans) ---
		const ordered = [...commitNodes].sort((a, b) => {
			const aTime = getCommitTimestamp(commits.get(a.hash));
			const bTime = getCommitTimestamp(commits.get(b.hash));
			if (aTime === bTime) return a.hash.localeCompare(b.hash);
			return bTime - aTime;
		});

		// Active lanes track which hash each column expects next (for convergence)
		const activeLanes = new Array(nextLane).fill(null);

		for (const node of ordered) {
			const hash = node.hash;
			const commit = commits.get(hash);
			const parents = commit?.parents || [];

			let lane;

			if (this.commitToLane.has(hash)) {
				// Already assigned by phase 1
				lane = this.commitToLane.get(hash);
			} else {
				// Check if an active lane expects this commit
				lane = -1;
				for (let i = 0; i < activeLanes.length; i++) {
					if (activeLanes[i] === hash) {
						lane = i;
						break;
					}
				}
				if (lane === -1) {
					lane = this._findFreeLane(activeLanes);
				}
				this.commitToLane.set(hash, lane);
			}

			// Free any other active lanes that also expected this commit (convergence)
			for (let i = 0; i < activeLanes.length; i++) {
				if (i !== lane && activeLanes[i] === hash) {
					activeLanes[i] = null;
				}
			}

			if (parents.length === 0) {
				// Root commit — free the lane
				activeLanes[lane] = null;
			} else {
				const firstParent = parents[0];

				// First parent continues this lane only if it isn't owned by a different lane
				if (!commitOwner.has(firstParent) || commitOwner.get(firstParent) === lane) {
					activeLanes[lane] = firstParent;
				} else {
					activeLanes[lane] = null; // Parent belongs to another branch's lane
				}

				// Merge parents (non-first) get new/reused lanes
				for (let i = 1; i < parents.length; i++) {
					const parentHash = parents[i];
					if (!commitHashes.has(parentHash)) continue;
					if (commitOwner.has(parentHash)) continue; // Already owned

					let alreadyExpected = false;
					for (let j = 0; j < activeLanes.length; j++) {
						if (activeLanes[j] === parentHash) {
							alreadyExpected = true;
							break;
						}
					}
					if (!alreadyExpected) {
						const mergeLane = this._findFreeLane(activeLanes);
						activeLanes[mergeLane] = parentHash;
					}
				}
			}
		}

		// Apply lane assignments to nodes
		for (const node of commitNodes) {
			const laneIndex = this.commitToLane.get(node.hash) ?? 0;
			node.laneIndex = laneIndex;
			node.laneColor = LANE_COLORS[laneIndex % LANE_COLORS.length];
		}

		// Rebuild display order permutation, preserving user's drag reordering
		const maxLane = Math.max(0, ...Array.from(this.commitToLane.values()));
		const newLength = maxLane + 1;

		// Pad newLaneOwners for Phase-2 lanes (no named branch owner)
		while (newLaneOwners.length < newLength) {
			newLaneOwners.push("");
		}

		const prevOwners = this._laneOwners;
		const prevOrder = this.laneOrder;

		if (prevOwners.length === 0 || prevOrder.length === 0) {
			// First activation — identity permutation
			this.laneOrder = Array.from({ length: newLength }, (_, i) => i);
		} else {
			// Map old branch names to their previous display positions
			const branchToOldDisplay = new Map();
			for (let i = 0; i < prevOwners.length; i++) {
				if (prevOwners[i] && i < prevOrder.length) {
					branchToOldDisplay.set(prevOwners[i], prevOrder[i]);
				}
			}

			const newOrder = new Array(newLength);
			const usedPositions = new Set();

			// First pass: assign known branches their old display positions
			for (let i = 0; i < newLength; i++) {
				const branch = newLaneOwners[i];
				if (branch && branchToOldDisplay.has(branch)) {
					const oldDisplay = branchToOldDisplay.get(branch);
					if (oldDisplay < newLength && !usedPositions.has(oldDisplay)) {
						newOrder[i] = oldDisplay;
						usedPositions.add(oldDisplay);
					}
				}
			}

			// Second pass: fill remaining lanes with unused display positions
			let nextFree = 0;
			for (let i = 0; i < newLength; i++) {
				if (newOrder[i] !== undefined) continue;
				while (usedPositions.has(nextFree)) nextFree++;
				newOrder[i] = nextFree;
				usedPositions.add(nextFree);
				nextFree++;
			}

			this.laneOrder = newOrder;
		}

		this._laneOwners = newLaneOwners;
	}

	/**
	 * Finds the lowest free (null) slot in the active lanes array.
	 * Appends a new slot if none are free.
	 *
	 * @param {Array<string|null>} activeLanes Active lane tracking array
	 * @returns {number} Index of the free lane
	 */
	_findFreeLane(activeLanes) {
		for (let i = 0; i < activeLanes.length; i++) {
			if (activeLanes[i] === null) return i;
		}
		activeLanes.push(null);
		return activeLanes.length - 1;
	}

	/**
	 * Maps a logical lane index to its current display position.
	 * @param {number} logicalIndex Logical lane index from commitToLane.
	 * @returns {number} Display position index.
	 */
	getDisplayIndex(logicalIndex) {
		return this.laneOrder[logicalIndex] ?? logicalIndex;
	}

	/**
	 * Returns branches sorted by priority for lane assignment.
	 * main/master/trunk gets first priority (lane 0), then alphabetical.
	 *
	 * @param {Map<string, string>} branches Map of branch name to target hash
	 * @returns {Array<[string, string]>} Sorted [name, hash] pairs
	 */
	_prioritizeBranches(branches) {
		if (!branches || branches.size === 0) return [];

		const mainNames = ["main", "master", "trunk"];
		const result = [];
		let mainEntry = null;

		for (const name of mainNames) {
			if (branches.has(name)) {
				mainEntry = name;
				result.push([name, branches.get(name)]);
				break;
			}
		}

		// Fallback: use the first branch if no main/master/trunk
		if (!mainEntry && branches.size > 0) {
			const [name, hash] = branches.entries().next().value;
			mainEntry = name;
			result.push([name, hash]);
		}

		// Remaining branches alphabetically
		const others = [];
		for (const [name, hash] of branches.entries()) {
			if (name !== mainEntry) {
				others.push([name, hash]);
			}
		}
		others.sort((a, b) => a[0].localeCompare(b[0]));
		result.push(...others);

		return result;
	}

	/**
	 * Returns lane metadata for rendering lane backgrounds and headers.
	 * @returns {Array<{index: number, color: string, branchName: string, minY: number, maxY: number}>}
	 */
	getLaneInfo() {
		return this._laneInfo;
	}

	/**
	 * Returns the display lane index at a given X coordinate in graph space.
	 * @param {number} graphX X coordinate in graph space.
	 * @returns {number} Display lane index (may be out of range — caller must clamp).
	 */
	findLaneAtX(graphX) {
		return Math.round((graphX - LANE_MARGIN) / LANE_WIDTH);
	}

	/**
	 * Hit-tests a graph-space point against all lane header bar rectangles.
	 * Returns an object with the display lane index and the segment's commit
	 * hashes, or null if the point isn't inside any header.
	 * @param {number} graphX X coordinate in graph space.
	 * @param {number} graphY Y coordinate in graph space.
	 * @returns {{displayLane: number, segmentHashes: Set<string>}|null}
	 */
	findLaneHeaderAt(graphX, graphY) {
		const pad = LANE_VERTICAL_STEP / 2;
		const halfW = LANE_WIDTH / 2 - 4;

		for (const lane of this._laneInfo) {
			const cx = LANE_MARGIN + lane.index * LANE_WIDTH;
			if (graphX < cx - halfW || graphX > cx + halfW) continue;

			const segments = lane.segments ?? [];
			for (const seg of segments) {
				const barY = seg.minY - pad;
				if (graphY >= barY && graphY <= barY + LANE_HEADER_HEIGHT) {
					return { displayLane: lane.index, segmentHashes: seg.hashes };
				}
			}
		}
		return null;
	}

	/**
	 * Swaps two entire display columns.
	 * Only the display positions in laneOrder are exchanged — logical lane
	 * assignments and colors stay the same, so no overlaps can occur.
	 * @param {Set<string>} segmentHashes Commit hashes used to identify the source column.
	 * @param {number} targetDisplay Target display column index.
	 */
	moveSegment(segmentHashes, targetDisplay) {
		// Determine the source logical lane from one of the dragged commits
		const sampleHash = segmentHashes.values().next().value;
		const sourceLogical = this.commitToLane.get(sampleHash);
		if (sourceLogical === undefined) return;

		// Find the logical lane that currently maps to the target display position
		let targetLogical = -1;
		for (let i = 0; i < this.laneOrder.length; i++) {
			if (this.laneOrder[i] === targetDisplay) {
				targetLogical = i;
				break;
			}
		}
		if (targetLogical === -1 || targetLogical === sourceLogical) return;

		// Swap display positions — all segments in each column move together
		const tmp = this.laneOrder[sourceLogical];
		this.laneOrder[sourceLogical] = this.laneOrder[targetLogical];
		this.laneOrder[targetLogical] = tmp;

		// Recompute positions and lane info
		this.computeTargetPositions(this.nodes, this._commits);
		this._buildLaneInfo(this.nodes, this._branches, this._commits);
		this.applyTargetPositions(this.nodes);
	}

	/**
	 * Builds lane info array from current lane assignments and target positions.
	 * Maps lanes to branch names by checking which branch tips occupy which lanes.
	 * Splits lanes into contiguous segments based on parent-child connectivity
	 * so that merged branches get distinct background strips.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, string>} branches Map of branch name to target hash
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 */
	_buildLaneInfo(nodes, branches, commits) {
		// Collect per-lane commit hashes and Y positions
		/** @type {Map<number, {hashes: Set<string>, yByHash: Map<string, number>}>} */
		const laneCommits = new Map();

		for (const node of nodes) {
			if (node.type !== "commit") continue;
			const laneIndex = this.commitToLane.get(node.hash);
			if (laneIndex === undefined) continue;

			const target = this.transitionTargetPositions.get(node.hash);
			const y = target ? target.y : node.y;

			if (!laneCommits.has(laneIndex)) {
				laneCommits.set(laneIndex, { hashes: new Set(), yByHash: new Map() });
			}
			const lc = laneCommits.get(laneIndex);
			lc.hashes.add(node.hash);
			lc.yByHash.set(node.hash, y);
		}

		// Global hash→Y lookup (across all lanes) for resolving fork/merge points
		const hashToY = new Map();
		for (const lc of laneCommits.values()) {
			for (const [h, y] of lc.yByHash) hashToY.set(h, y);
		}

		// Build segments per lane using parent-child connectivity (union-find)
		const laneData = new Map();

		for (const [laneIndex, lc] of laneCommits.entries()) {
			// Union-find for grouping connected commits in this lane
			const parent = new Map();
			for (const h of lc.hashes) parent.set(h, h);

			const find = (a) => {
				while (parent.get(a) !== a) {
					parent.set(a, parent.get(parent.get(a)));
					a = parent.get(a);
				}
				return a;
			};
			const union = (a, b) => {
				const ra = find(a), rb = find(b);
				if (ra !== rb) parent.set(ra, rb);
			};

			// Union commits that are direct parent-child within the same lane
			for (const hash of lc.hashes) {
				const commit = commits?.get(hash);
				if (!commit?.parents) continue;
				for (const ph of commit.parents) {
					if (lc.hashes.has(ph)) {
						union(hash, ph);
					}
				}
			}

			// Group into connected components (collect hashes per group)
			const groups = new Map();
			for (const hash of lc.hashes) {
				const root = find(hash);
				if (!groups.has(root)) groups.set(root, []);
				groups.get(root).push(hash);
			}

			// Build segments from groups, extending to fork/merge points
			const segments = [];
			for (const groupHashes of groups.values()) {
				let minY = Infinity, maxY = -Infinity;
				for (const h of groupHashes) {
					const y = lc.yByHash.get(h);
					if (y < minY) minY = y;
					if (y > maxY) maxY = y;
				}
				const groupSet = new Set(groupHashes);

				// Extend to fork point: find cross-lane parents of this
				// segment's commits (the source of the right-going arrow
				// that spawned this branch).
				for (const h of groupHashes) {
					const commit = commits?.get(h);
					if (!commit?.parents) continue;
					for (const ph of commit.parents) {
						if (!groupSet.has(ph) && hashToY.has(ph)) {
							const py = hashToY.get(ph);
							if (py > maxY) maxY = py;
							if (py < minY) minY = py;
						}
					}
				}

				// Extend to merge point: find cross-lane children that
				// have a parent in this segment (the target of the
				// left-going arrow merging this branch back).
				for (const node of nodes) {
					if (node.type !== "commit") continue;
					if (groupSet.has(node.hash)) continue;
					const commit = commits?.get(node.hash);
					if (!commit?.parents) continue;
					for (const ph of commit.parents) {
						if (groupSet.has(ph) && hashToY.has(node.hash)) {
							const cy = hashToY.get(node.hash);
							if (cy < minY) minY = cy;
							if (cy > maxY) maxY = cy;
						}
					}
				}

				segments.push({ minY, maxY, hashes: groupSet, branchName: "" });
			}
			segments.sort((a, b) => a.minY - b.minY);

			const allY = [...lc.yByHash.values()];
			laneData.set(laneIndex, {
				index: this.getDisplayIndex(laneIndex),
				color: LANE_COLORS[laneIndex % LANE_COLORS.length],
				branchName: "",
				segments,
				minY: Math.min(...allY),
				maxY: Math.max(...allY),
			});
		}

		// Map branch tips to the specific segment they belong to
		if (branches) {
			for (const [branchName, targetHash] of branches.entries()) {
				const laneIndex = this.commitToLane.get(targetHash);
				if (laneIndex === undefined) continue;
				const info = laneData.get(laneIndex);
				if (!info) continue;

				// Strip refs/heads/ and refs/remotes/ prefixes for display
				let displayName = branchName;
				if (displayName.startsWith("refs/heads/")) {
					displayName = displayName.slice("refs/heads/".length);
				} else if (displayName.startsWith("refs/remotes/")) {
					displayName = displayName.slice("refs/remotes/".length);
				}

				// Find the segment containing this branch tip
				const seg = info.segments.find(s => s.hashes.has(targetHash));
				if (seg && !seg.branchName) {
					seg.branchName = displayName;
				}
				// Also set lane-level name for the first branch (header fallback)
				if (!info.branchName) {
					info.branchName = displayName;
				}
			}
		}

		this._laneInfo = Array.from(laneData.values()).sort((a, b) => a.index - b.index);
	}

	/**
	 * Computes target positions for all commit nodes.
	 * Y-axis: chronological (newest at top)
	 * X-axis: lane-based (LANE_MARGIN + laneIndex * LANE_WIDTH)
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 */
	computeTargetPositions(nodes, commits) {
		this.transitionTargetPositions.clear();

		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return;

		// Sort commits chronologically (newest first)
		const ordered = [...commitNodes].sort((a, b) => {
			const aTime = getCommitTimestamp(commits.get(a.hash));
			const bTime = getCommitTimestamp(commits.get(b.hash));
			if (aTime === bTime) {
				return a.hash.localeCompare(b.hash);
			}
			return bTime - aTime; // Reversed: newer commits first
		});

		// Calculate vertical spacing
		const count = ordered.length;
		const span = Math.max(1, count - 1);
		const baseStep = LANE_VERTICAL_STEP;
		const desiredLength = span * baseStep + TIMELINE_PADDING;
		const available = Math.max(desiredLength, this.viewportHeight - TIMELINE_MARGIN * 2);
		const step = span === 0 ? 0 : available / span;
		const startY = Math.max(TIMELINE_MARGIN, (this.viewportHeight - available) / 2);

		// Position each commit
		ordered.forEach((node, index) => {
			const laneIndex = this.commitToLane.get(node.hash) || 0;
			const x = LANE_MARGIN + this.getDisplayIndex(laneIndex) * LANE_WIDTH;
			const y = span === 0 ? startY : startY + step * index;

			this.transitionTargetPositions.set(node.hash, { x, y });
		});
	}

	/**
	 * Applies target positions immediately to nodes (no animation).
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 */
	applyTargetPositions(nodes) {
		for (const node of nodes) {
			if (node.type !== "commit") continue;

			const target = this.transitionTargetPositions.get(node.hash);
			if (target) {
				node.x = target.x;
				node.y = target.y;
				node.vx = 0;
				node.vy = 0;
			}
		}
	}

	/**
	 * Starts the transition animation and kicks off the rAF render loop.
	 */
	startTransition() {
		this.isTransitioning = true;
		this.transitionStartTime = performance.now();
		this._runTransitionFrame();
	}

	/**
	 * Drives the transition animation via requestAnimationFrame.
	 * Calls tick() to interpolate positions, then onTick() to trigger rendering.
	 */
	_runTransitionFrame() {
		if (!this.isTransitioning) return;

		const needsRender = this.tick();
		if (needsRender && this._onTick) {
			this._onTick();
		}

		if (this.isTransitioning) {
			this.animationFrameId = requestAnimationFrame(() => this._runTransitionFrame());
		}
	}

	/**
	 * Stops the transition animation.
	 */
	stopTransition() {
		this.isTransitioning = false;
		if (this.animationFrameId !== null) {
			cancelAnimationFrame(this.animationFrameId);
			this.animationFrameId = null;
		}
	}

	/**
	 * Interpolates positions during transition animation.
	 * Called by external animation loop with current progress.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {number} progress Animation progress (0 to 1)
	 */
	interpolatePositions(nodes, progress) {
		// Easing function: ease-in-out cubic
		const eased = progress < 0.5
			? 4 * progress * progress * progress
			: 1 - Math.pow(-2 * progress + 2, 3) / 2;

		for (const node of nodes) {
			if (node.type !== "commit") continue;

			const start = this.transitionStartPositions.get(node.hash);
			const target = this.transitionTargetPositions.get(node.hash);

			if (start && target) {
				node.x = start.x + (target.x - start.x) * eased;
				node.y = start.y + (target.y - start.y) * eased;
				node.vx = 0;
				node.vy = 0;
			}
		}
	}

	/**
	 * Update viewport dimensions.
	 * Lane layout needs viewport height for positioning calculations.
	 *
	 * @param {number} width New viewport width.
	 * @param {number} height New viewport height.
	 */
	updateViewport(width, height) {
		this.viewportHeight = height;
	}

	/**
	 * Check if auto-centering should be active.
	 * Lane layout doesn't use auto-centering.
	 *
	 * @returns {boolean} Always returns false.
	 */
	shouldAutoCenter() {
		return false;
	}

	/**
	 * Disable auto-centering.
	 * No-op for lane layout since it doesn't auto-center.
	 */
	disableAutoCenter() {
		// No-op: lane layout doesn't auto-center
	}
}

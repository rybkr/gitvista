/**
 * @fileoverview Lane-based layout strategy for Git commit graph.
 * Implements git-style swimlane layout with chronological ordering.
 *
 * Conforms to LayoutStrategy interface defined in layoutStrategy.js.
 *
 * Algorithm:
 * 1. Walk first-parent chains from branch tips (main first) to claim ownership
 * 2. Assign remaining commits (merge parents) via column-reuse
 * 3. Build segments from connected components within each lane
 * 4. Assign each segment an independent position on a number line (0 = main)
 * 5. Position nodes: Y = chronological, X = segment position
 * 6. Animate smooth transitions between modes
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
 * Extracts a branch name from a merge commit message.
 * Handles standard patterns produced by git, GitHub, and custom messages:
 *   "Merge branch 'feature/foo'"
 *   "Merge branch 'feature/foo' into main"
 *   "Merge remote-tracking branch 'origin/feature/foo'"
 *   "Merge pull request #123 from user/branch-name"
 *   "Merge feature/foo: description"
 *   "Merge dev into main: description"
 *
 * @param {string} message Commit message (first line).
 * @returns {string} Extracted branch name, or "" if no pattern matches.
 */
function parseMergeBranchName(message) {
	const first = message.split("\n")[0];

	// "Merge remote-tracking branch 'origin/name'" — strip remote prefix
	const remoteMatch = first.match(/^Merge remote-tracking branch '([^']+)'/);
	if (remoteMatch) {
		const raw = remoteMatch[1];
		const slashIdx = raw.indexOf("/");
		return slashIdx >= 0 ? raw.slice(slashIdx + 1) : raw;
	}

	// "Merge branch 'name'" — keep the full branch name (no stripping)
	const branchMatch = first.match(/^Merge branch '([^']+)'/);
	if (branchMatch) return branchMatch[1];

	// "Merge pull request #N from user/branch"
	const prMatch = first.match(/^Merge pull request #\d+ from [^/]+\/(.+)/);
	if (prMatch) return prMatch[1].trim();

	// Custom merge messages: "Merge feature/foo: ..." or "Merge dev into main"
	const customMatch = first.match(/^Merge ([\w][\w./-]*\w)(?:\s+into\b|\s*:|$)/);
	if (customMatch) return customMatch[1];

	return "";
}

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

		/** @type {Array<Object>} Lane info for rendering */
		this._laneInfo = [];

		/** @type {Map<string, Object>|null} Cached commits map for recomputation */
		this._commits = null;

		/** @type {Map<string, string>|null} Cached branches map for recomputation */
		this._branches = null;

		/** @type {Array<string>} Branch name owning each logical lane (index → name) */
		this._laneOwners = [];

		/** @type {Map<string, number>|null} Phase 1 commit hash → lane index */
		this._phase1Commits = null;

		/** @type {Array<Object>} Computed segments with position info */
		this._segments = [];

		/** @type {Map<string, number>} Persisted segment ID → position (survives graph updates) */
		this._segmentPositions = new Map();

		/** @type {Map<string, number>} Commit hash → Y coordinate */
		this._yPositions = new Map();

		/** @type {Map<string, string>} Commit hash → segment ID */
		this._commitToSegmentId = new Map();
	}

	/**
	 * @type {boolean}
	 */
	get supportsRebalance() {
		return true;
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

		// 1. Compute lane assignments
		this.assignLanes(nodes, commits, branches);

		// 2. Compute Y positions
		this._computeYPositions(nodes, commits);

		// 3. Build segments
		this._buildSegments(nodes, commits);

		// 4. Assign segment positions
		this._assignSegmentPositions();

		// 5. Compute X positions → transitionTargetPositions
		this._computeXPositions(nodes);

		// 6. Build lane info for rendering
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
		this._commits = null;
		this._branches = null;
		this._laneOwners = [];
		this._phase1Commits = null;
		this._segments = [];
		this._segmentPositions.clear();
		this._yPositions.clear();
		this._commitToSegmentId.clear();

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

		// 1. Recompute lane assignments
		this.assignLanes(nodes, commits, branches);

		// 2. Compute Y positions
		this._computeYPositions(nodes, commits);

		// 3. Build segments
		this._buildSegments(nodes, commits);

		// 4. Assign segment positions (restores previous positions)
		this._assignSegmentPositions();

		// 5. Compute X positions
		this._computeXPositions(nodes);

		// 6. Rebuild lane info for rendering
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
	 * Rebalance the layout by clearing all user-customized segment positions
	 * and recomputing the default spiral assignment (1, -1, 2, -2, ...).
	 */
	rebalance() {
		if (!this.nodes) return;

		this._segmentPositions.clear();
		this._assignSegmentPositions();
		this._computeXPositions(this.nodes);
		this._buildLaneInfo(this.nodes, this._branches, this._commits);
		this.applyTargetPositions(this.nodes);
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

		// Save Phase 1 ownership so _buildSegments can distinguish
		// Phase 1 commits from Phase 2 commits that reused the same lane.
		this._phase1Commits = commitOwner;

		// Apply lane assignments to nodes
		for (const node of commitNodes) {
			const laneIndex = this.commitToLane.get(node.hash) ?? 0;
			node.laneIndex = laneIndex;
			node.laneColor = LANE_COLORS[laneIndex % LANE_COLORS.length];
		}

		// Pad laneOwners for Phase-2 lanes (no named branch owner)
		const maxLane = Math.max(0, ...Array.from(this.commitToLane.values()));
		while (newLaneOwners.length < maxLane + 1) {
			newLaneOwners.push("");
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
	 * @returns {Array<Object>}
	 */
	getLaneInfo() {
		return this._laneInfo;
	}

	/**
	 * Converts a number-line position to an X coordinate.
	 * @param {number} position Integer position on the number line.
	 * @returns {number} X coordinate in graph space.
	 */
	positionToX(position) {
		return LANE_MARGIN + position * LANE_WIDTH;
	}

	/**
	 * Returns the nearest number-line position at a given X coordinate.
	 * @param {number} graphX X coordinate in graph space.
	 * @returns {number} Integer position on the number line (may be negative).
	 */
	findPositionAtX(graphX) {
		return Math.round((graphX - LANE_MARGIN) / LANE_WIDTH);
	}

	/**
	 * Hit-tests a graph-space point against all segment header bar rectangles.
	 * Returns an object with the segment position and commit hashes,
	 * or null if the point isn't inside any header.
	 * @param {number} graphX X coordinate in graph space.
	 * @param {number} graphY Y coordinate in graph space.
	 * @returns {{displayLane: number, segmentHashes: Set<string>}|null}
	 */
	findLaneHeaderAt(graphX, graphY) {
		const pad = LANE_VERTICAL_STEP / 2;
		const halfW = LANE_WIDTH / 2 - 4;

		for (const lane of this._laneInfo) {
			const cx = this.positionToX(lane.position);
			if (graphX < cx - halfW || graphX > cx + halfW) continue;

			const segments = lane.segments ?? [];
			for (const seg of segments) {
				const barY = seg.minY - pad;
				if (graphY >= barY && graphY <= barY + LANE_HEADER_HEIGHT) {
					return { displayLane: lane.position, segmentHashes: seg.hashes, branchOwner: seg.branchOwner || "", tipHash: seg.tipHash || "" };
				}
			}
		}
		return null;
	}

	/**
	 * Hit-tests a graph-space point against lane body regions (below headers).
	 * Returns an object with the lane position and merged commit hashes,
	 * or null if the point isn't inside any lane body.
	 * @param {number} graphX X coordinate in graph space.
	 * @param {number} graphY Y coordinate in graph space.
	 * @returns {{position: number, hashes: Set<string>}|null}
	 */
	findLaneBodyAt(graphX, graphY) {
		const halfW = LANE_WIDTH / 2 - 4;

		for (const lane of this._laneInfo) {
			const cx = this.positionToX(lane.position);
			if (graphX < cx - halfW || graphX > cx + halfW) continue;

			const segments = lane.segments ?? [];
			for (const seg of segments) {
				if (graphY >= seg.coreMinY && graphY <= seg.coreMaxY) {
					// Merge all segment hashes at this position
					const allHashes = new Set();
					for (const s of segments) {
						for (const h of s.hashes) allHashes.add(h);
					}
					return { position: lane.position, hashes: allHashes };
				}
			}
		}
		return null;
	}

	/**
	 * Moves a segment to a target position on the number line.
	 * Position 0 is locked to the main branch and cannot be displaced.
	 * Y-overlapping segments at the target position are pushed away.
	 *
	 * @param {Set<string>} segmentHashes Commit hashes identifying the segment.
	 * @param {number} targetPosition Target position on the number line.
	 */
	moveSegment(segmentHashes, targetPosition) {
		// Can't displace main at position 0
		if (targetPosition === 0) return;

		// Find the source segment
		const sampleHash = segmentHashes.values().next().value;
		const segId = this._commitToSegmentId.get(sampleHash);
		if (!segId) return;

		const sourceSeg = this._segments.find((s) => s.id === segId);
		if (!sourceSeg) return;

		// Can't move main
		if (sourceSeg.isMain) return;

		const oldPosition = sourceSeg.position;
		if (oldPosition === targetPosition) return;

		// Set the segment's position to target
		sourceSeg.position = targetPosition;
		this._segmentPositions.set(sourceSeg.id, targetPosition);

		// Push Y-overlapping segments at the target position
		const direction = Math.sign(oldPosition - targetPosition);
		for (const other of this._segments) {
			if (other === sourceSeg) continue;
			if (other.position !== targetPosition) continue;
			if (!this._segmentsOverlapY(sourceSeg, other)) continue;
			this._pushSegment(other, direction || 1);
		}

		// Recompute X positions and lane info
		this._computeXPositions(this.nodes);
		this._buildLaneInfo(this.nodes, this._branches, this._commits);
		this.applyTargetPositions(this.nodes);
	}

	/**
	 * Recursively pushes a segment in the given direction to resolve collisions.
	 * Skips position 0 (reserved for main).
	 *
	 * @param {Object} segment The segment to push.
	 * @param {number} direction +1 or -1.
	 */
	_pushSegment(segment, direction) {
		if (segment.isMain) return; // Never push main

		let candidate = segment.position + direction;
		// Skip position 0 (reserved for main)
		if (candidate === 0) candidate += direction;

		// Check for Y-overlapping segments at the candidate position
		for (const other of this._segments) {
			if (other === segment) continue;
			if (other.position !== candidate) continue;
			if (!this._segmentsOverlapY(segment, other)) continue;
			// Recurse: push the blocking segment further
			this._pushSegment(other, direction);
		}

		segment.position = candidate;
		this._segmentPositions.set(segment.id, candidate);
	}

	/**
	 * Tests whether two segments overlap in Y (using extended ranges).
	 * @param {Object} a First segment.
	 * @param {Object} b Second segment.
	 * @returns {boolean}
	 */
	_segmentsOverlapY(a, b) {
		return a.extMinY <= b.extMaxY && b.extMinY <= a.extMaxY;
	}

	/**
	 * Computes Y positions for all commit nodes.
	 * Extracts the chronological sort + topological correction from computeTargetPositions.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 */
	_computeYPositions(nodes, commits) {
		this._yPositions.clear();

		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return;

		// Chronological sort with topological correction
		const commitHashes = new Set(commitNodes.map((n) => n.hash));

		const childrenOf = new Map();
		const parentCount = new Map();
		for (const hash of commitHashes) {
			childrenOf.set(hash, []);
			parentCount.set(hash, 0);
		}
		for (const hash of commitHashes) {
			const commit = commits.get(hash);
			for (const ph of commit?.parents ?? []) {
				if (commitHashes.has(ph)) {
					childrenOf.get(ph).push(hash);
					parentCount.set(hash, parentCount.get(hash) + 1);
				}
			}
		}

		// Initialize effective timestamps, then propagate via Kahn's from roots
		const effectiveTime = new Map();
		const queue = [];
		for (const hash of commitHashes) {
			effectiveTime.set(hash, getCommitTimestamp(commits.get(hash)));
		}
		for (const [hash, count] of parentCount) {
			if (count === 0) queue.push(hash);
		}
		while (queue.length > 0) {
			const hash = queue.shift();
			const parentET = effectiveTime.get(hash);
			for (const ch of childrenOf.get(hash)) {
				effectiveTime.set(ch, Math.max(effectiveTime.get(ch), parentET + 1));
				const remaining = parentCount.get(ch) - 1;
				parentCount.set(ch, remaining);
				if (remaining === 0) queue.push(ch);
			}
		}

		const ordered = [...commitNodes].sort((a, b) => {
			const ea = effectiveTime.get(a.hash) ?? 0;
			const eb = effectiveTime.get(b.hash) ?? 0;
			if (ea !== eb) return eb - ea;
			return a.hash.localeCompare(b.hash);
		});

		// Calculate vertical spacing
		const count = ordered.length;
		const span = Math.max(1, count - 1);
		const baseStep = LANE_VERTICAL_STEP;
		const desiredLength = span * baseStep + TIMELINE_PADDING;
		const available = Math.max(desiredLength, this.viewportHeight - TIMELINE_MARGIN * 2);
		const step = span === 0 ? 0 : available / span;
		const startY = Math.max(TIMELINE_MARGIN, (this.viewportHeight - available) / 2);

		ordered.forEach((node, index) => {
			const y = span === 0 ? startY : startY + step * index;
			this._yPositions.set(node.hash, y);
		});
	}

	/**
	 * Builds segments from connected components within each lane.
	 * Uses union-find to group commits that are parent-child within the same lane.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 */
	_buildSegments(nodes, commits) {
		this._segments = [];
		this._commitToSegmentId.clear();

		// Collect per-lane commit hashes
		const laneCommits = new Map();
		for (const node of nodes) {
			if (node.type !== "commit") continue;
			const laneIndex = this.commitToLane.get(node.hash);
			if (laneIndex === undefined) continue;
			if (!laneCommits.has(laneIndex)) {
				laneCommits.set(laneIndex, new Set());
			}
			laneCommits.get(laneIndex).add(node.hash);
		}

		// Global hash→Y lookup for extending to fork/merge points
		const hashToY = this._yPositions;

		for (const [laneIndex, hashes] of laneCommits.entries()) {
			// Union-find
			const parent = new Map();
			for (const h of hashes) parent.set(h, h);

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
			for (const hash of hashes) {
				const commit = commits?.get(hash);
				if (!commit?.parents) continue;
				for (const ph of commit.parents) {
					if (hashes.has(ph)) {
						union(hash, ph);
					}
				}
			}

			// Group into connected components
			const groups = new Map();
			for (const hash of hashes) {
				const root = find(hash);
				if (!groups.has(root)) groups.set(root, []);
				groups.get(root).push(hash);
			}

			// Build segments from groups
			const laneOwner = this._laneOwners[laneIndex] || "";
			const isMain = laneIndex === 0;

			for (const groupHashes of groups.values()) {
				// Only inherit the lane's branch name if this segment actually
				// contains Phase 1 commits for this lane.  Phase 2 commits that
				// reused a freed lane slot should NOT inherit the old name.
				const ownsLane = groupHashes.some(
					(h) => this._phase1Commits?.get(h) === laneIndex,
				);
				const branchOwner = ownsLane ? laneOwner : "";
				let minY = Infinity, maxY = -Infinity;
				for (const h of groupHashes) {
					const y = hashToY.get(h);
					if (y !== undefined) {
						if (y < minY) minY = y;
						if (y > maxY) maxY = y;
					}
				}
				const coreMinY = minY, coreMaxY = maxY;
				const hashSet = new Set(groupHashes);

				// Extend to fork/merge points for background rendering
				let extMinY = minY, extMaxY = maxY;

				// Extend to fork point
				for (const h of groupHashes) {
					const commit = commits?.get(h);
					if (!commit?.parents) continue;
					for (const ph of commit.parents) {
						if (!hashSet.has(ph) && hashToY.has(ph)) {
							const py = hashToY.get(ph);
							if (py > extMaxY) extMaxY = py;
							if (py < extMinY) extMinY = py;
						}
					}
				}

				// Extend to merge point
				for (const node of nodes) {
					if (node.type !== "commit") continue;
					if (hashSet.has(node.hash)) continue;
					const commit = commits?.get(node.hash);
					if (!commit?.parents) continue;
					for (const ph of commit.parents) {
						if (hashSet.has(ph) && hashToY.has(node.hash)) {
							const cy = hashToY.get(node.hash);
							if (cy < extMinY) extMinY = cy;
							if (cy > extMaxY) extMaxY = cy;
						}
					}
				}

				// Find tip hash (newest commit) for segment ID
				let tipHash = groupHashes[0];
				let tipY = hashToY.get(tipHash) ?? Infinity;
				for (const h of groupHashes) {
					const y = hashToY.get(h) ?? Infinity;
					if (y < tipY) {
						tipY = y;
						tipHash = h;
					}
				}

				const id = branchOwner + ":" + tipHash;

				// Resolve the tip commit's timestamp for age badges
				const tipCommit = commits?.get(tipHash);
				const tipTimestamp = tipCommit?.author?.when ?? null;

				const segment = {
					id,
					laneIndex,
					hashes: hashSet,
					minY,
					maxY,
					coreMinY,
					coreMaxY,
					extMinY,
					extMaxY,
					position: 0, // will be assigned in _assignSegmentPositions
					isMain,
					branchOwner,
					tipHash,
					tipTimestamp,
					color: LANE_COLORS[laneIndex % LANE_COLORS.length],
				};

				this._segments.push(segment);

				for (const h of hashSet) {
					this._commitToSegmentId.set(h, id);
				}
			}
		}

		// Sort segments by minY for consistent processing
		this._segments.sort((a, b) => a.minY - b.minY);

		// Infer branch names for unnamed segments from merge commit messages.
		// When a merge commit outside the segment has a parent inside it, its
		// message often contains the original branch name (e.g. "Merge branch
		// 'feature/foo'" or "Merge pull request #N from user/branch").
		for (const seg of this._segments) {
			if (seg.branchOwner) continue;

			for (const node of nodes) {
				if (node.type !== "commit") continue;
				if (seg.hashes.has(node.hash)) continue;
				const commit = commits?.get(node.hash);
				if (!commit?.parents || commit.parents.length < 2) continue;
				const mergesIntoSeg = commit.parents.some((ph) => seg.hashes.has(ph));
				if (!mergesIntoSeg) continue;

				const msg = commit.message || "";
				const name = parseMergeBranchName(msg);
				if (name) {
					seg.branchOwner = name;
					break;
				}
			}
		}
	}

	/**
	 * Assigns positions to segments on the number line.
	 * Restores previous positions by branch owner name, assigns new segments
	 * outward from 0, and resolves collisions.
	 */
	_assignSegmentPositions() {
		// Build a map from branch owner to previous positions for restoration
		const prevPositionsByOwner = new Map();
		for (const [segId, pos] of this._segmentPositions) {
			const colonIdx = segId.indexOf(":");
			const owner = colonIdx >= 0 ? segId.slice(0, colonIdx) : "";
			if (owner) {
				// Store all previous positions for this owner
				if (!prevPositionsByOwner.has(owner)) {
					prevPositionsByOwner.set(owner, []);
				}
				prevPositionsByOwner.get(owner).push(pos);
			}
		}

		// Clear old positions map — will rebuild
		this._segmentPositions.clear();

		// 1. Main segments → always position 0
		for (const seg of this._segments) {
			if (seg.isMain) {
				seg.position = 0;
				this._segmentPositions.set(seg.id, 0);
			}
		}

		// 2. Restore previous positions by branch owner name
		const unmatched = [];
		for (const seg of this._segments) {
			if (seg.isMain) continue;

			const owner = seg.branchOwner;
			if (owner && prevPositionsByOwner.has(owner)) {
				const positions = prevPositionsByOwner.get(owner);
				if (positions.length > 0) {
					seg.position = positions.shift();
					this._segmentPositions.set(seg.id, seg.position);
					continue;
				}
			}
			unmatched.push(seg);
		}

		// 3. Assign positions to unmatched segments via spiral from 0
		for (const seg of unmatched) {
			seg.position = this._assignInitialPosition(seg);
			this._segmentPositions.set(seg.id, seg.position);
		}

		// 4. Resolve any collisions introduced by restoration
		this._resolveAllCollisions();
	}

	/**
	 * Assigns an initial position for a new segment by spiraling outward from 0.
	 * Tries 1, -1, 2, -2, ... skipping positions occupied by Y-overlapping segments.
	 *
	 * @param {Object} segment The segment needing a position.
	 * @returns {number} Assigned position.
	 */
	_assignInitialPosition(segment) {
		for (let dist = 1; dist < 100; dist++) {
			for (const sign of [1, -1]) {
				const candidate = dist * sign;
				// Position 0 is reserved for main
				if (candidate === 0) continue;

				let occupied = false;
				for (const other of this._segments) {
					if (other === segment) continue;
					if (other.position !== candidate) continue;
					if (this._segmentsOverlapY(segment, other)) {
						occupied = true;
						break;
					}
				}
				if (!occupied) return candidate;
			}
		}
		return this._segments.length; // fallback
	}

	/**
	 * Resolves all Y-overlapping collisions at the same position.
	 * Scans all pairs and pushes colliding segments outward.
	 */
	_resolveAllCollisions() {
		let changed = true;
		let iterations = 0;
		while (changed && iterations < 50) {
			changed = false;
			iterations++;
			for (let i = 0; i < this._segments.length; i++) {
				for (let j = i + 1; j < this._segments.length; j++) {
					const a = this._segments[i];
					const b = this._segments[j];
					if (a.position !== b.position) continue;
					if (a.isMain && b.isMain) continue;
					if (!this._segmentsOverlapY(a, b)) continue;

					// Push the non-main one away
					const toPush = b.isMain ? a : (a.isMain ? b : b);
					const direction = toPush.position > 0 ? 1 : (toPush.position < 0 ? -1 : 1);
					this._pushSegment(toPush, direction);
					changed = true;
				}
			}
		}
	}

	/**
	 * Computes X positions for all commit nodes based on their segment positions.
	 * Combines with Y positions to fill transitionTargetPositions.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 */
	_computeXPositions(nodes) {
		this.transitionTargetPositions.clear();

		for (const node of nodes) {
			if (node.type !== "commit") continue;
			const y = this._yPositions.get(node.hash);
			if (y === undefined) continue;

			const segId = this._commitToSegmentId.get(node.hash);
			let position = 0;
			if (segId) {
				const seg = this._segments.find((s) => s.id === segId);
				if (seg) position = seg.position;
			}

			const x = this.positionToX(position);
			this.transitionTargetPositions.set(node.hash, { x, y });
		}
	}

	/**
	 * Builds lane info array for rendering backgrounds and headers.
	 * Each segment becomes its own entry with position-based X placement.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, string>} branches Map of branch name to target hash
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 */
	_buildLaneInfo(nodes, branches, commits) {
		const pad = LANE_VERTICAL_STEP / 2;
		const result = [];

		// Group segments by position for overlap clipping
		const segsByPosition = new Map();
		for (const seg of this._segments) {
			if (!segsByPosition.has(seg.position)) {
				segsByPosition.set(seg.position, []);
			}
			segsByPosition.get(seg.position).push(seg);
		}

		for (const [position, segs] of segsByPosition) {
			// Sort segments at this position by minY
			segs.sort((a, b) => a.minY - b.minY);

			// Build rendering segments with overlap clipping
			const renderSegments = [];
			for (const seg of segs) {
				let renderMinY = seg.extMinY;
				let renderMaxY = seg.extMaxY;

				renderSegments.push({
					minY: renderMinY,
					maxY: renderMaxY,
					coreMinY: seg.coreMinY,
					coreMaxY: seg.coreMaxY,
					hashes: seg.hashes,
					color: seg.color,
					branchOwner: seg.branchOwner,
					tipHash: seg.id.split(":")[1] || "",
					tipTimestamp: seg.tipTimestamp ?? null,
				});
			}

			// Clip adjacent segment backgrounds at midpoints
			for (let i = 0; i < renderSegments.length - 1; i++) {
				const cur = renderSegments[i];
				const next = renderSegments[i + 1];
				if (cur.maxY + pad > next.minY - pad) {
					const mid = (cur.coreMaxY + next.coreMinY) / 2;
					cur.maxY = Math.max(cur.coreMaxY, mid - pad);
					next.minY = Math.min(next.coreMinY, mid + pad);
				}
			}

			// Find overall Y range for this position
			let minY = Infinity, maxY = -Infinity;
			for (const seg of segs) {
				for (const h of seg.hashes) {
					const y = this._yPositions.get(h);
					if (y !== undefined) {
						if (y < minY) minY = y;
						if (y > maxY) maxY = y;
					}
				}
			}

			// Use the color from the first segment at this position
			const color = segs[0].color;

			result.push({
				position,
				color,
				segments: renderSegments,
				minY,
				maxY,
			});
		}

		result.sort((a, b) => a.position - b.position);
		this._laneInfo = result;
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

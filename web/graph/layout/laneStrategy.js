/**
 * @fileoverview Lane-based layout strategy for Git commit graph.
 * Implements git-style swimlane layout with chronological ordering.
 *
 * Conforms to LayoutStrategy interface defined in layoutStrategy.js.
 *
 * Algorithm:
 * 1. Sort commits chronologically (newest first)
 * 2. Assign main branch (main/master/trunk) first-parent chain to lane 0
 * 3. Assign remaining branches to lowest available lanes
 * 4. Assign orphan commits (unreachable from branches) to rightmost lanes
 * 5. Position nodes: Y = chronological, X = lane index
 * 6. Animate smooth transitions between modes
 */

import {
	LANE_WIDTH,
	LANE_MARGIN,
	LANE_TRANSITION_DURATION,
	LANE_COLORS,
	LINK_DISTANCE,
	TIMELINE_SPACING,
	TIMELINE_PADDING,
	TIMELINE_MARGIN,
} from "../constants.js";
import { getCommitTimestamp } from "../utils/time.js";

/**
 * Lane-based layout strategy implementation.
 * @implements {LayoutStrategy}
 */
export class LaneStrategy {
	constructor() {
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
		this.viewportHeight = viewport.height || 800;

		// Compute lane assignments
		this.assignLanes(nodes, commits, branches);

		// Compute target positions for all nodes
		this.computeTargetPositions(nodes, commits);

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
	 */
	deactivate() {
		this.stopTransition();
		this.commitToLane.clear();
		this.transitionStartPositions.clear();
		this.transitionTargetPositions.clear();
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
		this.viewportHeight = viewport.height || 800;

		// Recompute lane assignments
		this.assignLanes(nodes, commits, branches);

		// Update target positions
		this.computeTargetPositions(nodes, commits);

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
	 * Updates transition animation progress.
	 *
	 * @returns {boolean} True if render needed, false otherwise
	 */
	tick() {
		if (!this.isTransitioning) {
			return false;
		}

		const elapsed = performance.now() - this.transitionStartTime;
		const progress = Math.min(1, elapsed / LANE_TRANSITION_DURATION);

		// Easing function: ease-in-out cubic
		const eased = progress < 0.5
			? 4 * progress * progress * progress
			: 1 - Math.pow(-2 * progress + 2, 3) / 2;

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
	 * Assigns commits to lanes using topological sort and branch tracking.
	 *
	 * Algorithm:
	 * 1. Build parent-child adjacency maps
	 * 2. Identify main branch and assign its first-parent chain to lane 0
	 * 3. Assign remaining branches to lowest available lanes
	 * 4. Assign orphan commits to rightmost lanes
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 * @param {Map<string, string>} branches Map of branch name to target hash
	 */
	assignLanes(nodes, commits, branches) {
		this.commitToLane.clear();

		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return;

		// Build parent-child relationships
		const childrenMap = new Map(); // hash -> array of child hashes
		for (const node of commitNodes) {
			const commit = commits.get(node.hash);
			if (!commit?.parents) continue;

			for (const parentHash of commit.parents) {
				if (!childrenMap.has(parentHash)) {
					childrenMap.set(parentHash, []);
				}
				childrenMap.get(parentHash).push(node.hash);
			}
		}

		// Find main branch (main, master, or trunk)
		const mainBranchNames = ["main", "master", "trunk"];
		let mainBranchHash = null;
		for (const name of mainBranchNames) {
			if (branches.has(name)) {
				mainBranchHash = branches.get(name);
				break;
			}
		}

		// If no main branch found, use the first branch
		if (!mainBranchHash && branches.size > 0) {
			mainBranchHash = branches.values().next().value;
		}

		// Track which commits have been assigned
		const assigned = new Set();

		// Assign main branch first-parent chain to lane 0
		if (mainBranchHash) {
			this.assignBranchChain(mainBranchHash, 0, commits, assigned, true);
		}

		// Assign remaining branches to lowest available lanes
		let currentLane = 1;
		for (const [branchName, targetHash] of branches) {
			if (assigned.has(targetHash)) continue;

			// Find lowest available lane for this branch
			const lane = this.findLowestAvailableLane(targetHash, commits, assigned);
			this.assignBranchChain(targetHash, lane, commits, assigned, true);
			currentLane = Math.max(currentLane, lane + 1);
		}

		// Assign orphan commits (not reachable from any branch)
		for (const node of commitNodes) {
			if (!assigned.has(node.hash)) {
				this.commitToLane.set(node.hash, currentLane);
				assigned.add(node.hash);
				currentLane++;
			}
		}

		// Apply lane assignments to nodes
		for (const node of commitNodes) {
			const laneIndex = this.commitToLane.get(node.hash) || 0;
			node.laneIndex = laneIndex;
			node.laneColor = LANE_COLORS[laneIndex % LANE_COLORS.length];
		}
	}

	/**
	 * Assigns a branch's commit chain to a specific lane.
	 * Follows first-parent chain for linear history.
	 *
	 * @param {string} startHash Starting commit hash
	 * @param {number} lane Lane index to assign
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 * @param {Set<string>} assigned Set of already-assigned commit hashes
	 * @param {boolean} firstParentOnly Whether to follow only first parent
	 */
	assignBranchChain(startHash, lane, commits, assigned, firstParentOnly) {
		let current = startHash;
		const visited = new Set();

		while (current && !assigned.has(current) && !visited.has(current)) {
			visited.add(current);
			this.commitToLane.set(current, lane);
			assigned.add(current);

			const commit = commits.get(current);
			if (!commit?.parents || commit.parents.length === 0) break;

			// Follow first parent for linear history
			current = firstParentOnly ? commit.parents[0] : null;
		}
	}

	/**
	 * Finds the lowest available lane for a commit chain.
	 * Checks for collisions with already-assigned commits.
	 *
	 * @param {string} startHash Starting commit hash
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 * @param {Set<string>} assigned Set of already-assigned commit hashes
	 * @returns {number} Lowest available lane index
	 */
	findLowestAvailableLane(startHash, commits, assigned) {
		// Build set of lanes used by already-assigned commits
		const usedLanes = new Set();
		for (const [hash, lane] of this.commitToLane) {
			usedLanes.add(lane);
		}

		// Find first available lane
		let lane = 0;
		while (usedLanes.has(lane)) {
			lane++;
		}
		return lane;
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
		const baseStep = LINK_DISTANCE * TIMELINE_SPACING;
		const desiredLength = span * baseStep + TIMELINE_PADDING;
		const available = Math.max(desiredLength, this.viewportHeight - TIMELINE_MARGIN * 2);
		const step = span === 0 ? 0 : available / span;
		const startY = Math.max(TIMELINE_MARGIN, (this.viewportHeight - available) / 2);

		// Position each commit
		ordered.forEach((node, index) => {
			const laneIndex = this.commitToLane.get(node.hash) || 0;
			const x = LANE_MARGIN + laneIndex * LANE_WIDTH;
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
	 * Starts the transition animation.
	 */
	startTransition() {
		this.isTransitioning = true;
		this.transitionStartTime = performance.now();
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

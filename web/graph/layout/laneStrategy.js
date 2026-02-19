/**
 * @fileoverview Lane-based layout strategy for Git commit graph.
 * Implements git-style swimlane layout with chronological ordering.
 *
 * Conforms to LayoutStrategy interface defined in layoutStrategy.js.
 *
 * Algorithm:
 * 1. Sort commits chronologically (newest first)
 * 2. Seed lane 0 with the main branch tip
 * 3. Process commits in order — each inherits the lane of its child
 * 4. Merge parents get new/reused lanes; lanes freed on convergence
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
		this.nodes = nodes; // Store reference for cleanup in deactivate()
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
	 * Removes lane-specific properties from nodes to ensure clean state.
	 */
	deactivate() {
		this.stopTransition();
		this.commitToLane.clear();
		this.transitionStartPositions.clear();
		this.transitionTargetPositions.clear();

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
	 * Assigns commits to lanes using a topological column-reuse algorithm.
	 *
	 * Processes commits chronologically (newest first) and maintains a set of
	 * "active lanes" — columns that track which parent hash they expect next.
	 * Each commit inherits the lane of its child, and lanes are freed and
	 * reused when branches converge, keeping the graph compact.
	 *
	 * Algorithm:
	 * 1. Seed lane 0 with the main branch tip
	 * 2. Process commits newest-first
	 * 3. Each commit takes the lane that expects it (or the lowest free lane)
	 * 4. First parent inherits the lane; merge parents get new/reused lanes
	 * 5. When multiple lanes converge on one commit, extras are freed
	 *
	 * @param {Array<Object>} nodes Array of graph nodes
	 * @param {Map<string, Object>} commits Map of commit hash to commit data
	 * @param {Map<string, string>} branches Map of branch name to target hash
	 */
	assignLanes(nodes, commits, branches) {
		this.commitToLane.clear();

		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return;

		// Sort commits chronologically (newest first)
		const ordered = [...commitNodes].sort((a, b) => {
			const aTime = getCommitTimestamp(commits.get(a.hash));
			const bTime = getCommitTimestamp(commits.get(b.hash));
			if (aTime === bTime) return a.hash.localeCompare(b.hash);
			return bTime - aTime;
		});

		// Active lanes: index = column, value = hash expected next (or null if free)
		const activeLanes = [];

		// Seed lane 0 with main branch tip so it always occupies the leftmost column
		const mainBranchNames = ["main", "master", "trunk"];
		let mainBranchHash = null;
		for (const name of mainBranchNames) {
			if (branches.has(name)) {
				mainBranchHash = branches.get(name);
				break;
			}
		}
		if (!mainBranchHash && branches.size > 0) {
			mainBranchHash = branches.values().next().value;
		}
		if (mainBranchHash) {
			activeLanes.push(mainBranchHash);
		}

		for (const node of ordered) {
			const hash = node.hash;
			const commit = commits.get(hash);
			const parents = commit?.parents || [];

			// Find all lanes expecting this commit
			let lane = -1;
			const convergingLanes = [];
			for (let i = 0; i < activeLanes.length; i++) {
				if (activeLanes[i] === hash) {
					if (lane === -1) {
						lane = i;
					} else {
						convergingLanes.push(i);
					}
				}
			}

			if (lane === -1) {
				// New branch tip — find lowest free lane
				lane = this._findFreeLane(activeLanes);
			}

			// Free converging lanes (branches merging into this commit)
			for (const cl of convergingLanes) {
				activeLanes[cl] = null;
			}

			this.commitToLane.set(hash, lane);

			if (parents.length === 0) {
				// Root commit — free the lane
				activeLanes[lane] = null;
			} else {
				// First parent continues this lane
				activeLanes[lane] = parents[0];

				// Additional parents (merge sources) get new/reused lanes
				for (let i = 1; i < parents.length; i++) {
					const parentHash = parents[i];
					// Only allocate if no lane already expects this parent
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

/**
 * @fileoverview Force-directed layout strategy using D3 force simulation.
 * Implements the LayoutStrategy interface defined in layoutStrategy.js.
 *
 * This strategy uses D3's physics simulation to position nodes organically,
 * with forces for charge repulsion, collision avoidance, and link distance.
 * Combines force simulation with timeline-based initial positioning for
 * chronological commit ordering.
 */

import * as d3 from "https://cdn.jsdelivr.net/npm/d3@7.9.0/+esm";
import {
	ALPHA_DECAY,
	CHARGE_STRENGTH,
	COLLISION_RADIUS,
	DRAG_ALPHA_TARGET,
	LINK_DISTANCE,
	LINK_STRENGTH,
	VELOCITY_DECAY,
} from "../constants.js";
import { LayoutManager } from "./layoutManager.js";

/**
 * Force-directed layout strategy.
 * Conforms to the LayoutStrategy interface.
 */
export class ForceStrategy {
	/**
	 * @param {Object} options Configuration options.
	 * @param {number} options.viewportWidth Initial viewport width in pixels.
	 * @param {number} options.viewportHeight Initial viewport height in pixels.
	 * @param {Function} options.onTick Callback invoked on each simulation tick.
	 */
	constructor(options = {}) {
		this.viewportWidth = options.viewportWidth || 0;
		this.viewportHeight = options.viewportHeight || 0;
		this.onTick = options.onTick || (() => {});

		this.simulation = null;
		this.layoutManager = null;
		this.initialLayoutComplete = false;

		// Arrays managed by the controller, we receive references
		this.nodes = [];
		this.links = [];
	}

	/**
	 * @type {boolean}
	 * ForceStrategy supports rebalancing by clearing fx/fy and reheating the simulation.
	 */
	get supportsRebalance() {
		return true;
	}

	/**
	 * Activate the force strategy with current graph data.
	 * Creates D3 simulation and applies initial timeline layout.
	 *
	 * @param {Array<Object>} nodes Array of graph nodes (commits and branches).
	 * @param {Array<Object>} links Array of graph links (parent-child relationships).
	 * @param {Map<string, Object>} commits Map of commit hash to commit data.
	 * @param {Map<string, Object>} branches Map of branch name to target hash.
	 * @param {Object} viewport Current viewport state {width, height}.
	 */
	activate(nodes, links, commits, branches, viewport) {
		this.nodes = nodes;
		this.links = links;

		// Clear any lane-specific properties from a previous lane layout
		for (const node of this.nodes) {
			delete node.laneColor;
			delete node.laneIndex;
		}

		// Update viewport dimensions from current state
		if (viewport) {
			this.viewportWidth = viewport.width || this.viewportWidth;
			this.viewportHeight = viewport.height || this.viewportHeight;
		}

		// Create D3 force simulation
		this.simulation = d3
			.forceSimulation(this.nodes)
			.velocityDecay(VELOCITY_DECAY)
			.alphaDecay(ALPHA_DECAY)
			.force("charge", d3.forceManyBody().strength(CHARGE_STRENGTH))
			.force("collision", d3.forceCollide().radius(COLLISION_RADIUS))
			.force(
				"link",
				d3
					.forceLink(this.links)
					.id((d) => d.id ?? d.hash)
					.distance(LINK_DISTANCE)
					.strength(LINK_STRENGTH),
			)
			.on("tick", () => this.tick());

		// Create layout manager for timeline positioning and viewport management
		this.layoutManager = new LayoutManager(
			this.simulation,
			this.viewportWidth,
			this.viewportHeight,
		);

		this.initialLayoutComplete = false;
	}

	/**
	 * Deactivate the strategy and clean up resources.
	 * Stops the simulation but leaves nodes in their final positions.
	 */
	deactivate() {
		if (this.simulation) {
			this.simulation.stop();
			this.simulation = null;
		}
		this.layoutManager = null;
		this.nodes = [];
		this.links = [];
	}

	/**
	 * Update the graph in response to data changes.
	 * Applies timeline layout on first update, then incrementally updates simulation.
	 *
	 * @param {Array<Object>} nodes Updated nodes array.
	 * @param {Array<Object>} links Updated links array.
	 * @param {Map<string, Object>} commits Updated commits map.
	 * @param {Map<string, Object>} branches Updated branches map.
	 * @param {Object} viewport Current viewport state.
	 * @param {boolean} structureChanged Whether the graph structure changed.
	 */
	updateGraph(nodes, links, commits, branches, viewport, structureChanged) {
		if (!this.simulation || !this.layoutManager) {
			return;
		}

		this.nodes = nodes;
		this.links = links;

		// Update simulation with new nodes and links
		this.simulation.nodes(this.nodes);
		this.simulation.force("link").links(this.links);

		// Check if we have commits to layout
		const commitNodes = this.nodes.filter((n) => n.type === "commit");
		const hasCommits = commitNodes.length > 0;

		// On first update with commits, apply timeline layout and center
		if (!this.initialLayoutComplete && hasCommits) {
			this.layoutManager.applyTimelineLayout(this.nodes);
			this.layoutManager.requestAutoCenter();
			this.initialLayoutComplete = true;
			this.layoutManager.restartSimulation(1.0);
		} else {
			// Incremental update: boost simulation if structure changed
			this.layoutManager.boostSimulation(structureChanged);
			if (structureChanged && hasCommits) {
				this.layoutManager.requestAutoCenter();
			}
		}
	}

	/**
	 * Handle node drag interaction.
	 * Sets fixed position (fx/fy) and reheats simulation.
	 *
	 * @param {Object} node The node being dragged.
	 * @param {number} x New x position in graph coordinates.
	 * @param {number} y New y position in graph coordinates.
	 * @returns {boolean} Always true (handled and render needed).
	 */
	handleDrag(node, x, y) {
		if (!this.simulation) {
			return false;
		}

		// Set fixed position and zero velocity
		node.fx = x;
		node.fy = y;
		node.vx = 0;
		node.vy = 0;
		node.x = x;
		node.y = y;

		// Reheat simulation gently during drag
		this.simulation.alphaTarget(DRAG_ALPHA_TARGET).restart();

		return true; // Request render
	}

	/**
	 * Handle end of node drag interaction.
	 * Clears fx/fy to release the node back to the simulation.
	 *
	 * @param {Object} node The node that was dragged.
	 */
	handleDragEnd(node) {
		if (!this.simulation) {
			return;
		}

		// Release node back to simulation by clearing fixed position
		node.fx = null;
		node.fy = null;
		node.vx = 0;
		node.vy = 0;

		// Stop alpha target (let simulation cool naturally)
		this.simulation.alphaTarget(0);
	}

	/**
	 * Animation frame tick callback.
	 * Invokes the onTick callback and checks for auto-centering.
	 *
	 * @returns {boolean} Always true (force simulation always needs rendering).
	 */
	tick() {
		if (!this.simulation || !this.layoutManager) {
			return false;
		}

		// Check if we should auto-center on latest commit
		if (this.layoutManager.shouldAutoCenter()) {
			// Auto-centering is handled by the controller via findCenterTarget
			// We just check if we should stop auto-centering
			this.layoutManager.checkAutoCenterStop(this.simulation.alpha());
		}

		// Invoke the tick callback provided by the controller
		this.onTick();

		// Always render during force simulation
		return true;
	}

	/**
	 * Reset layout to default state.
	 * Clears all fixed positions and reheats the simulation.
	 */
	rebalance() {
		if (!this.simulation) {
			return;
		}

		// Clear all fixed positions
		for (const node of this.nodes) {
			node.fx = null;
			node.fy = null;
		}

		// Reheat simulation to full alpha
		this.simulation.alpha(0.8).restart();
	}

	/**
	 * Find the logical center position for viewport centering.
	 * Returns the position of the latest (newest) commit node.
	 *
	 * @param {Array<Object>} nodes Current nodes array.
	 * @returns {{x: number, y: number}|null} Latest commit position or null.
	 */
	findCenterTarget(nodes) {
		if (!this.layoutManager) {
			return null;
		}

		const latest = this.layoutManager.findLatestCommit(nodes);
		if (latest) {
			return { x: latest.x, y: latest.y };
		}

		return null;
	}

	/**
	 * Update viewport dimensions.
	 * Called when the canvas or container is resized.
	 *
	 * @param {number} width New viewport width.
	 * @param {number} height New viewport height.
	 */
	updateViewport(width, height) {
		this.viewportWidth = width;
		this.viewportHeight = height;

		if (this.layoutManager) {
			this.layoutManager.updateViewport(width, height);
		}
	}

	/**
	 * Check if auto-centering is currently active.
	 *
	 * @returns {boolean} True if auto-centering is active.
	 */
	shouldAutoCenter() {
		return this.layoutManager?.shouldAutoCenter() ?? false;
	}

	/**
	 * Disable auto-centering (called when user interacts with viewport).
	 */
	disableAutoCenter() {
		this.layoutManager?.disableAutoCenter();
	}
}

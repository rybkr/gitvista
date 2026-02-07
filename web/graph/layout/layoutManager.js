/**
 * @fileoverview Layout controller for the Git graph visualization.
 * Manages force simulation configuration and timeline positioning.
 */

import {
	LINK_DISTANCE,
	TIMELINE_AUTO_CENTER_ALPHA,
	TIMELINE_MARGIN,
	TIMELINE_PADDING,
	TIMELINE_SPACING,
} from "../constants.js";
import { getCommitTimestamp } from "../utils/time.js";

/**
 * Drives layout-specific behavior for the graph.
 */
export class LayoutManager {
	/**
	 * @param {import("d3").Simulation} simulation D3 force simulation instance.
	 * @param {number} viewportWidth Initial viewport width.
	 * @param {number} viewportHeight Initial viewport height.
	 */
	constructor(simulation, viewportWidth, viewportHeight) {
		this.simulation = simulation;
		this.viewportWidth = viewportWidth;
		this.viewportHeight = viewportHeight;
		this.autoCenter = false;
	}

	/**
	 * Updates the viewport dimensions and re-centers the simulation.
	 *
	 * @param {number} width Viewport width in CSS pixels.
	 * @param {number} height Viewport height in CSS pixels.
	 */
	updateViewport(width, height) {
		this.viewportWidth = width;
		this.viewportHeight = height;
	}

	/**
	 * Applies timeline layout by positioning commits chronologically.
	 *
	 * @param {import("../types.js").GraphNode[]} nodes Collection of nodes in the simulation.
	 */
	applyTimelineLayout(nodes) {
		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return;

		const ordered = this.sortCommitsByTime(commitNodes);
		const spacing = this.calculateTimelineSpacing(commitNodes);
		this.positionNodesVertically(ordered, spacing);
	}

	/**
	 * @param {import("../types.js").GraphNodeCommit[]} nodes Commit nodes to sort.
	 * @returns {import("../types.js").GraphNodeCommit[]} Sorted commit nodes.
	 */
	sortCommitsByTime(nodes) {
		return [...nodes].sort((a, b) => {
			const aTime = getCommitTimestamp(a.commit);
			const bTime = getCommitTimestamp(b.commit);
			if (aTime === bTime) {
				return a.hash.localeCompare(b.hash);
			}
			return aTime - bTime;
		});
	}

	/**
	 * Computes spacing information for timeline placement.
	 *
	 * @param {import("../types.js").GraphNodeCommit[]} nodes Commit node collection.
	 * @returns {{start: number, step: number, span: number}} Calculated spacing values.
	 */
	calculateTimelineSpacing(nodes) {
		const count = nodes.length;
		const span = Math.max(1, count - 1);
		if (span === 0) {
			return { start: this.viewportHeight / 2, step: 0, span };
		}
		const baseStep = LINK_DISTANCE * TIMELINE_SPACING;
		const desiredLength = span * baseStep + TIMELINE_PADDING;
		const available =
			Math.max(desiredLength, this.viewportHeight - TIMELINE_MARGIN * 2);
		const step = available / span;
		const start = Math.max(
			TIMELINE_MARGIN,
			(this.viewportHeight - available) / 2,
		);

		return { start, step, span };
	}

	/**
	 * Places commit nodes along the Y axis using computed spacing.
	 *
	 * @param {import("../types.js").GraphNodeCommit[]} ordered Sorted commit nodes.
	 * @param {{start: number, step: number, span: number}} spacing Timeline spacing info.
	 */
	positionNodesVertically(ordered, spacing) {
		const { start, step, span } = spacing;
		const centerX = this.viewportWidth / 2;

		ordered.forEach((node, index) => {
			node.x = centerX;
			node.y = span === 0 ? start : start + step * index;
			node.vx = 0;
			node.vy = 0;
		});
	}

	/**
	 * Finds the newest commit node (largest timestamp) for centering.
	 *
	 * @param {import("../types.js").GraphNode[]} nodes Collection of nodes in the simulation.
	 * @returns {import("../types.js").GraphNodeCommit | null} Latest commit node when found.
	 */
	findLatestCommit(nodes) {
		const commitNodes = nodes.filter((n) => n.type === "commit");
		if (commitNodes.length === 0) return null;

		let latest = commitNodes[0];
		let bestTime = getCommitTimestamp(latest.commit);

		for (const node of commitNodes) {
			const time = getCommitTimestamp(node.commit);
			if (time > bestTime || (time === bestTime && node.y > latest.y)) {
				bestTime = time;
				latest = node;
			}
		}

		return latest;
	}

	/**
	 * @returns {boolean} True when auto-centering should continue.
	 */
	shouldAutoCenter() {
		return this.autoCenter;
	}

	/**
	 * Disables auto-centering for timeline mode.
	 */
	disableAutoCenter() {
		this.autoCenter = false;
	}

	/**
	 * Stops auto-centering when simulation cools below threshold.
	 *
	 * @param {number} alpha Current simulation alpha value.
	 */
	checkAutoCenterStop(alpha) {
		if (this.autoCenter && alpha < TIMELINE_AUTO_CENTER_ALPHA) {
			this.autoCenter = false;
		}
	}

	/**
	 * Requests that the layout auto-center on the newest commit.
	 */
	requestAutoCenter() {
		this.autoCenter = true;
	}

	/**
	 * Restarts the simulation with a target alpha.
	 *
	 * @param {number} alpha Desired alpha value.
	 */
	restartSimulation(alpha = 0.3) {
		this.simulation.alpha(alpha).restart();
		this.simulation.alphaTarget(0);
	}

	/**
	 * Gently warms the simulation for tree expand/collapse operations.
	 * Uses a much lower alpha than boostSimulation to avoid scattering the graph.
	 */
	boostForTreeExpand() {
		const currentAlpha = this.simulation.alpha();
		this.simulation.alpha(Math.max(currentAlpha, 0.04)).restart();
		this.simulation.alphaTarget(0);
	}

	/**
	 * Boosts simulation alpha when graph structure changes.
	 *
	 * @param {boolean} structureChanged True when nodes or links changed materially.
	 */
	boostSimulation(structureChanged) {
		const currentAlpha = this.simulation.alpha();
		const desiredAlpha = structureChanged ? 0.15 : 0.05;
		const nextAlpha = Math.max(currentAlpha, desiredAlpha);
		this.simulation.alpha(nextAlpha).restart();
		this.simulation.alphaTarget(0);
	}
}


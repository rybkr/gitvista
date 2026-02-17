/**
 * LayoutStrategy Interface
 *
 * Defines the contract that all layout strategy implementations must follow.
 * Layout strategies control how nodes are positioned in the graph visualization,
 * handle user interactions like dragging, and manage animation frame updates.
 *
 * Two implementations:
 * - ForceStrategy: D3 force simulation with physics-based layout
 * - LaneStrategy: Git-style swimlane layout with chronological ordering
 *
 * @fileoverview JSDoc interface definition for layout strategies (documentation only)
 */

/**
 * @typedef {Object} LayoutStrategy
 *
 * @property {boolean} supportsRebalance - Whether this strategy supports the rebalance action.
 *   - ForceStrategy: true (can reheat simulation and reset alpha)
 *   - LaneStrategy: false (deterministic layout doesn't need rebalancing)
 *
 * @property {ActivateFn} activate - Called when this strategy becomes the active layout.
 *   Initialize the layout with current graph data and viewport state.
 *   For ForceStrategy: create D3 simulation, register tick callback, set up forces.
 *   For LaneStrategy: compute initial lane assignments and positions.
 *
 * @property {DeactivateFn} deactivate - Called when this strategy is deactivated.
 *   Clean up resources: stop simulations, remove event handlers, clear timers.
 *   Should leave nodes in their final positions for smooth transitions.
 *
 * @property {UpdateGraphFn} updateGraph - Called when graph data changes (delta applied).
 *   Update the layout with new/modified/removed nodes, links, commits, or branches.
 *   For incremental updates: adjust layout minimally to preserve mental map.
 *   For full rebuilds: recompute entire layout from scratch.
 *
 * @property {HandleDragFn} handleDrag - Called on every pointer move during node drag.
 *   Update node position in response to user dragging.
 *   Returns true if the strategy handled the drag (and needs a render).
 *   ForceStrategy: update fx/fy and reheat simulation.
 *   LaneStrategy: may constrain to lanes or allow free positioning.
 *
 * @property {HandleDragEndFn} handleDragEnd - Called when user releases dragged node.
 *   Finalize drag operation, potentially snapping to grid or releasing constraints.
 *   ForceStrategy: may clear fx/fy to resume simulation, or keep pinned.
 *   LaneStrategy: may snap to nearest lane or commit position.
 *
 * @property {TickFn} tick - Called each animation frame by requestAnimationFrame loop.
 *   Update positions if the layout is animating (e.g., force simulation running).
 *   Returns true if a render is needed this frame, false to skip rendering.
 *   Allows strategy to control when expensive canvas redraws happen.
 *
 * @property {RebalanceFn} rebalance - Reset layout to default state (if supported).
 *   Only called when supportsRebalance is true and user clicks rebalance button.
 *   ForceStrategy: reheat simulation to full alpha, center graph.
 *   LaneStrategy: N/A (not applicable, deterministic layout).
 *
 * @property {FindCenterTargetFn} findCenterTarget - Find the logical center position.
 *   Called when user requests "center on graph" or needs to focus viewport.
 *   Returns {x, y} coordinates to center on, or null if no meaningful center exists.
 *   ForceStrategy: may return centroid of all nodes.
 *   LaneStrategy: may return HEAD commit position or timeline midpoint.
 */

/**
 * Activate the layout strategy with current graph data.
 *
 * Called when:
 * - User switches to this layout strategy in the UI
 * - Initial graph load (for the default strategy)
 *
 * The strategy should initialize its internal state, set up any simulations or
 * data structures, and perform an initial layout pass. Node positions should be
 * updated in place on the provided nodes array.
 *
 * @callback ActivateFn
 * @param {Array<Object>} nodes - Array of node objects with {id, x, y, ...}
 *   Nodes represent commits and may have existing x/y from previous strategy.
 * @param {Array<Object>} links - Array of link objects with {source, target}
 *   Links represent parent-child relationships between commits.
 * @param {Map<string, Object>} commits - Map of commit hash to commit data
 *   Includes timestamp, message, parents, tree, etc.
 * @param {Map<string, Object>} branches - Map of branch name to {target, type}
 *   Includes all refs (branches, tags, HEAD) pointing to commits.
 * @param {Object} viewport - Current viewport state {x, y, scale}
 *   Viewport transform for coordinate system (may be used for initial positioning).
 * @returns {void}
 *
 * @example
 * // ForceStrategy activation
 * activate(nodes, links, commits, branches, viewport) {
 *   this.simulation = d3.forceSimulation(nodes)
 *     .force("link", d3.forceLink(links))
 *     .force("charge", d3.forceManyBody())
 *     .on("tick", () => this.onTick());
 *   this.simulation.alpha(1).restart();
 * }
 */

/**
 * Deactivate the layout strategy and clean up resources.
 *
 * Called when:
 * - User switches to a different layout strategy
 * - Graph component is being destroyed
 *
 * The strategy should stop any running simulations, cancel timers, remove event
 * handlers, and release resources. Node positions should remain stable so the
 * next strategy can transition smoothly.
 *
 * @callback DeactivateFn
 * @returns {void}
 *
 * @example
 * // ForceStrategy deactivation
 * deactivate() {
 *   if (this.simulation) {
 *     this.simulation.stop();
 *     this.simulation = null;
 *   }
 * }
 */

/**
 * Update the layout in response to graph data changes.
 *
 * Called when:
 * - RepositoryDelta is received via WebSocket
 * - New commits, branches, or refs are added
 * - Existing nodes or links are removed
 *
 * The strategy should incrementally update its layout, preserving the mental map
 * where possible. For small changes (few new commits), avoid full recomputation.
 * For large changes (branch rebase, history rewrite), a full rebuild may be needed.
 *
 * @callback UpdateGraphFn
 * @param {Array<Object>} nodes - Updated nodes array (may include new nodes)
 * @param {Array<Object>} links - Updated links array (may include new links)
 * @param {Map<string, Object>} commits - Updated commits map
 * @param {Map<string, Object>} branches - Updated branches map
 * @param {Object} viewport - Current viewport state
 * @returns {void}
 *
 * @example
 * // ForceStrategy incremental update
 * updateGraph(nodes, links, commits, branches, viewport) {
 *   // Update simulation with new nodes/links
 *   this.simulation.nodes(nodes);
 *   this.simulation.force("link").links(links);
 *   // Reheat simulation gently for new additions
 *   this.simulation.alpha(0.3).restart();
 * }
 */

/**
 * Handle node drag interaction.
 *
 * Called continuously during drag operations as the pointer moves.
 * The strategy decides how to respond: update positions, reheat simulation,
 * constrain to lanes, etc.
 *
 * Return true if the strategy handled the drag and a render is needed.
 * Return false if the strategy ignored the drag (rare, but possible if disabled).
 *
 * @callback HandleDragFn
 * @param {Object} node - The node being dragged
 * @param {number} x - New x position in graph coordinates
 * @param {number} y - New y position in graph coordinates
 * @returns {boolean} True if handled and render needed, false otherwise
 *
 * @example
 * // ForceStrategy drag handling
 * handleDrag(node, x, y) {
 *   node.fx = x;
 *   node.fy = y;
 *   this.simulation.alpha(0.3).restart(); // Reheat simulation
 *   return true; // Request render
 * }
 */

/**
 * Handle end of node drag interaction.
 *
 * Called once when the user releases the pointer after dragging a node.
 * The strategy can finalize positions, snap to grid, release constraints,
 * or continue animating.
 *
 * @callback HandleDragEndFn
 * @param {Object} node - The node that was dragged
 * @returns {void}
 *
 * @example
 * // ForceStrategy drag end (keep pinned)
 * handleDragEnd(node) {
 *   // Keep fx/fy set - node stays pinned where user left it
 *   // Simulation will continue around it
 * }
 *
 * @example
 * // Alternative: release node to simulation
 * handleDragEnd(node) {
 *   node.fx = null;
 *   node.fy = null;
 *   this.simulation.alpha(0.3).restart();
 * }
 */

/**
 * Animation frame tick callback.
 *
 * Called each frame by the main render loop (typically 60fps via requestAnimationFrame).
 * The strategy should update any time-based animations or simulation steps.
 *
 * Return true if node positions changed and a canvas render is needed.
 * Return false if nothing changed (saves expensive canvas redraws).
 *
 * @callback TickFn
 * @returns {boolean} True if render needed, false to skip this frame
 *
 * @example
 * // ForceStrategy tick (simulation running)
 * tick() {
 *   // D3 simulation already updated node positions via its own tick
 *   // Check if simulation is still running
 *   return this.simulation.alpha() > this.simulation.alphaMin();
 * }
 *
 * @example
 * // LaneStrategy tick (no animation)
 * tick() {
 *   return false; // Deterministic layout, no per-frame updates
 * }
 */

/**
 * Reset layout to default/rebalanced state.
 *
 * Only called when supportsRebalance is true and the user clicks a rebalance button.
 * The strategy should reset to its default configuration, clear any accumulated
 * drift or instability, and recompute a clean layout.
 *
 * @callback RebalanceFn
 * @returns {void}
 *
 * @example
 * // ForceStrategy rebalance
 * rebalance() {
 *   // Clear all position constraints
 *   this.simulation.nodes().forEach(n => { n.fx = null; n.fy = null; });
 *   // Reheat simulation to full alpha
 *   this.simulation.alpha(1).restart();
 * }
 */

/**
 * Find the logical center position for viewport centering.
 *
 * Called when the user wants to center the viewport on the graph or needs
 * to focus on a meaningful position (e.g., initial load, "zoom to fit").
 *
 * Return {x, y} coordinates in graph space, or null if no center can be determined
 * (e.g., empty graph).
 *
 * @callback FindCenterTargetFn
 * @param {Array<Object>} nodes - Current nodes array
 * @returns {{x: number, y: number}|null} Center coordinates or null
 *
 * @example
 * // ForceStrategy: centroid of all nodes
 * findCenterTarget(nodes) {
 *   if (nodes.length === 0) return null;
 *   const sumX = nodes.reduce((sum, n) => sum + n.x, 0);
 *   const sumY = nodes.reduce((sum, n) => sum + n.y, 0);
 *   return { x: sumX / nodes.length, y: sumY / nodes.length };
 * }
 *
 * @example
 * // LaneStrategy: HEAD commit position
 * findCenterTarget(nodes) {
 *   const headNode = nodes.find(n => n.isHead);
 *   return headNode ? { x: headNode.x, y: headNode.y } : null;
 * }
 */

// This file is a JSDoc-only interface definition. No runtime code is exported.
// Implementations should document their conformance to this interface in their file headers.

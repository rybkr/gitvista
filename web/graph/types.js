/**
 * @fileoverview Shared JSDoc typedefs describing graph structures.
 * Enables strong tooling support across modularized graph code.
 */

/**
 * @typedef {Object} GraphSignature
 * @property {string} [name] Author or committer name.
 * @property {string} [email] Author or committer email.
 * @property {string} [when] ISO timestamp for modern payloads.
 * @property {string} [Name] Legacy field for name casing discrepancies.
 * @property {string} [Email] Legacy field for email casing discrepancies.
 * @property {string} [When] ISO timestamp for legacy payloads.
 */

/**
 * @typedef {Object} GraphCommit
 * @property {string} hash Commit SHA identifier.
 * @property {string} [message] Commit message body.
 * @property {GraphSignature} [author] Author metadata.
 * @property {GraphSignature} [committer] Committer metadata.
 * @property {string[]} [parents] Array of parent commit hashes.
 */

/**
 * @typedef {Object} GraphNodeBase
 * @property {number} x X coordinate in graph space.
 * @property {number} y Y coordinate in graph space.
 * @property {number} vx Velocity along the X axis.
 * @property {number} vy Velocity along the Y axis.
 */

/**
 * @typedef {GraphNodeBase & {
 *   type: "commit",
 *   hash: string,
 *   commit?: GraphCommit,
 *   radius?: number
 * }} GraphNodeCommit
 */

/**
 * @typedef {GraphNodeBase & {
 *   type: "branch",
 *   branch: string,
 *   targetHash: string | null
 * }} GraphNodeBranch
 */

/**
 * Tree and blob nodes are no longer rendered in the graph.
 * Tree access is now handled via a clickable icon on commit nodes
 * that opens the file browser sidebar.
 *
 * @typedef {GraphNodeBase & {
 *   type: "tree",
 *   hash: string,
 *   commitHash?: string,
 *   tree?: object,
 *   entryName?: string,
 *   expanded?: boolean,
 *   childIds?: string[],
 *   id?: string
 * }} GraphNodeTree
 */

/**
 * @typedef {GraphNodeBase & {
 *   type: "blob",
 *   hash: string,
 *   entryName: string,
 *   parentTreeHash: string,
 *   mode?: string,
 *   id: string
 * }} GraphNodeBlob
 */

/**
 * @typedef {GraphNodeCommit | GraphNodeBranch} GraphNode
 */

/**
 * @typedef {Object} GraphPalette
 * @property {string} background Canvas background color.
 * @property {string} node Default node color.
 * @property {string} link Link stroke color.
 * @property {string} labelText Commit label text color.
 * @property {string} labelHalo Halo color drawn behind labels.
 * @property {string} branchNode Branch node fill color.
 * @property {string} branchNodeBorder Branch node stroke color.
 * @property {string} branchLabelText Branch label text color.
 * @property {string} branchLink Branch link color.
 * @property {string} treeNode Tree node fill color.
 * @property {string} treeNodeBorder Tree node border color.
 * @property {string} treeLabelText Tree label text color.
 * @property {string} treeLink Tree link color.
 * @property {string} nodeHighlight Node highlight fill color.
 * @property {string} nodeHighlightGlow Glow color for highlighted nodes.
 * @property {string} nodeHighlightCore Inner highlight color for commits.
 * @property {string} nodeHighlightRing Ring color for highlighted nodes.
 * @property {string} mergeNode Merge commit node fill color.
 * @property {string} mergeHighlight Merge commit highlight fill color.
 * @property {string} mergeHighlightGlow Glow color for highlighted merge commits.
 * @property {string} mergeHighlightCore Inner highlight color for merge commits.
 * @property {string} mergeHighlightRing Ring color for highlighted merge commits.
 * @property {string} nodeShadow Shadow color for all node types.
 * @property {string} blobNode Blob node fill color.
 * @property {string} blobNodeBorder Blob node border color.
 * @property {string} blobLabelText Blob label text color.
 */

/**
 * @typedef {Object} GraphState
 * @property {Map<string, GraphCommit>} commits Map of commit hash to commit data.
 * @property {Map<string, string>} branches Map of branch name to target hash.
 * @property {GraphNode[]} nodes Collection of nodes rendered on the canvas.
 * @property {Array<{source: string | GraphNode, target: string | GraphNode, kind?: string}>} links Force simulation link definitions.
 * @property {import("d3").ZoomTransform} zoomTransform Current D3 zoom transform.
 */


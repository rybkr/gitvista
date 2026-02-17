/**
 * @fileoverview Palette helpers for reading CSS custom properties.
 * Ensures the graph renderer responds to theme changes.
 */

/**
 * Builds a palette object from computed styles of the provided element.
 *
 * @param {HTMLElement} element Canvas element whose CSS variables define theme colors.
 * @returns {import("../types.js").GraphPalette} Palette consumed by the renderer.
 */
export function buildPalette(element) {
    const styles = getComputedStyle(element);

    const read = (name, fallback) =>
        styles.getPropertyValue(name)?.trim() || fallback;

    return {
        background: read("--surface-color", "#ffffff"),
        node: read("--node-color", "#0969da"),
        link: read("--link-color", "#d0d7de"),
        labelText: read("--label-text-color", "#1f2328"),
        labelHalo: read("--label-halo-color", "rgba(255, 255, 255, 0.92)"),
        branchNode: read("--branch-node-color", "#8250df"),
        branchNodeBorder: read("--branch-node-border-color", "#6639ba"),
        branchLabelText: read("--branch-label-text-color", "#ffffff"),
        branchLink: read("--branch-link-color", "#8250df"),
        branchHighlight: read("--branch-highlight-color", "#a78bfa"),
        branchHighlightGlow: read(
            "--branch-highlight-glow",
            "rgba(167, 139, 250, 0.5)",
        ),
        branchHighlightCore: read("--branch-highlight-core", "#f3f0ff"),
        branchHighlightRing: read("--branch-highlight-ring", "#a78bfa"),
        nodeHighlight: read("--node-highlight-color", "#0969da"),
        nodeHighlightGlow: read(
            "--node-highlight-glow",
            "rgba(9, 105, 218, 0.35)",
        ),
        treeNode: read("--tree-node-color", "#bf8700"),
        treeNodeBorder: read("--tree-node-border-color", "#9a6700"),
        treeLabelText: read("--tree-label-text-color", "#ffffff"),
        treeLink: read("--tree-link-color", "#bf8700"),
        nodeHighlightCore: read("--node-highlight-core", "#ddf4ff"),
        nodeHighlightRing: read("--node-highlight-ring", "#0969da"),
        mergeNode: read("--merge-node-color", "#1a7f37"),
        mergeHighlight: read("--merge-highlight-color", "#1a7f37"),
        mergeHighlightGlow: read(
            "--merge-highlight-glow",
            "rgba(26, 127, 55, 0.35)",
        ),
        mergeHighlightCore: read("--merge-highlight-core", "#ddf8e4"),
        mergeHighlightRing: read("--merge-highlight-ring", "#1a7f37"),
        nodeShadow: read("--node-shadow-color", "rgba(27, 31, 36, 0.12)"),
        blobNode: read("--blob-node-color", "#0969da"),
        blobNodeBorder: read("--blob-node-border-color", "#0550ae"),
        blobLabelText: read("--blob-label-text-color", "#ffffff"),
    };
}

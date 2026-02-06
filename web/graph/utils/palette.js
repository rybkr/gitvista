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
        node: read("--node-color", "#3273dc"),
        link: read("--link-color", "#c4c9cf"),
        labelText: read("--label-text-color", "#24292f"),
        labelHalo: read("--label-halo-color", "rgba(248, 248, 246, 0.92)"),
        branchNode: read("--branch-node-color", "#6f42c1"),
        branchNodeBorder: read("--branch-node-border-color", "#59339d"),
        branchLabelText: read("--branch-label-text-color", "#ffffff"),
        branchLink: read("--branch-link-color", "#6f42c1"),
        nodeHighlight: read("--node-highlight-color", "#2563eb"),
        nodeHighlightGlow: read(
            "--node-highlight-glow",
            "rgba(59, 130, 246, 0.40)",
        ),
        treeNode: read("--tree-node-color", "#fb8500"),
        treeNodeBorder: read("--tree-node-border-color", "#d97706"),
        treeLabelText: read("--tree-label-text-color", "#ffffff"),
        treeLink: read("--tree-link-color", "#fb8500"),
        nodeHighlightCore: read("--node-highlight-core", "#dbeafe"),
        nodeHighlightRing: read("--node-highlight-ring", "#2563eb"),
        mergeNode: read("--merge-node-color", "#0e7c6b"),
        nodeShadow: read("--node-shadow-color", "rgba(0, 0, 0, 0.18)"),
    };
}

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
        link: read("--link-color", "#afb8c1"),
        labelText: read("--label-text-color", "#24292f"),
        labelHalo: read("--label-halo-color", "rgba(246, 248, 250, 0.9)"),
        branchNode: read("--branch-node-color", "#6f42c1"),
        branchNodeBorder: read("--branch-node-border-color", "#59339d"),
        branchLabelText: read("--branch-label-text-color", "#ffffff"),
        branchLink: read("--branch-link-color", "#6f42c1"),
        nodeHighlight: read("--node-highlight-color", "#1f6feb"),
        nodeHighlightGlow: read(
            "--node-highlight-glow",
            "rgba(79, 140, 255, 0.45)",
        ),
        treeNode: read("--tree-node-color", "#fb8500"), // Orange
        treeNodeBorder: read("--tree-node-border-color", "#d97706"),
        treeLabelText: read("--tree-label-text-color", "#ffffff"),
        treeLink: read("--tree-link-color", "#fb8500"),
        nodeHighlight: read("--node-highlight-color", "#1f6feb"),
        nodeHighlightGlow: read(
            "--node-highlight-glow",
            "rgba(79, 140, 255, 0.45)",
        ),
        nodeHighlightCore: read("--node-highlight-core", "#dbe9ff"),
        nodeHighlightRing: read("--node-highlight-ring", "#1f6feb"),
    };
}

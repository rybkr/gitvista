/**
 * @fileoverview Numeric and stylistic constants for the Git graph renderer.
 * Centralizes configuration values to simplify tuning and reuse.
 */

export const NODE_RADIUS = 6;
export const LINK_DISTANCE = 50;
export const LINK_STRENGTH = 0.4;
export const CHARGE_STRENGTH = -110;
export const COLLISION_RADIUS = 14;
export const LINK_THICKNESS = NODE_RADIUS * 0.32;
export const ARROW_LENGTH = NODE_RADIUS * 2;
export const ARROW_WIDTH = NODE_RADIUS * 1.35;
export const HOVER_RADIUS = 12;
export const DRAG_ACTIVATION_DISTANCE = 4;
export const CLICK_TOLERANCE = 6;
export const TIMELINE_SPACING = 0.95;
export const TIMELINE_PADDING = 160;
export const TIMELINE_FALLBACK_GAP = 320;
export const TIMELINE_MARGIN = 40;
export const TIMELINE_AUTO_CENTER_ALPHA = 0.12;
export const LABEL_FONT =
    "12px ui-monospace, SFMono-Regular, SFMono, Menlo, Monaco, Consolas, Liberation Mono, Courier New, monospace";
export const LABEL_PADDING = 9;
export const ZOOM_MIN = 0.25;
export const ZOOM_MAX = 4;
export const BRANCH_NODE_PADDING_X = 10;
export const BRANCH_NODE_PADDING_Y = 6;
export const BRANCH_NODE_CORNER_RADIUS = 6;
export const BRANCH_NODE_OFFSET_X = 28;
export const BRANCH_NODE_OFFSET_Y = 6;
export const BRANCH_NODE_RADIUS = 18;
export const TREE_NODE_SIZE = 10;
export const TREE_NODE_PADDING_X = 8;
export const TREE_NODE_PADDING_Y = 6;
export const TREE_NODE_CORNER_RADIUS = 3;
export const TREE_NODE_HIGHLIGHT_SIZE = 13;
export const TREE_NODE_OFFSET_X = -28;
export const TREE_NODE_OFFSET_Y = 6;
export const TOOLTIP_OFFSET_X = 18;
export const TOOLTIP_OFFSET_Y = -24;
export const HIGHLIGHT_NODE_RADIUS = NODE_RADIUS + 2.5;


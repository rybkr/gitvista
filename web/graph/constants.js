/**
 * @fileoverview Numeric and stylistic constants for the Git graph renderer.
 * Centralizes configuration values to simplify tuning and reuse.
 */

export const NODE_RADIUS = 6;

/*
 * Force-simulation physics — tuned for compact layout with fast settling.
 * Reduced repulsion + weaker links + higher damping keeps clusters tight;
 * COLLISION_RADIUS prevents node overlap, and labels appear at zoom >= 1.5×
 * (DETAIL_THRESHOLDS) so density is acceptable at default zoom.
 */
export const LINK_DISTANCE = 50;
export const LINK_STRENGTH = 0.4;
export const CHARGE_STRENGTH = -110;
export const COLLISION_RADIUS = 14;
export const VELOCITY_DECAY = 0.55;
export const ALPHA_DECAY = 0.02;
export const DRAG_ALPHA_TARGET = 0.12;
export const LINK_THICKNESS = NODE_RADIUS * 0.32;
export const ARROW_LENGTH = NODE_RADIUS * 2;
export const ARROW_WIDTH = NODE_RADIUS * 1.35;
export const HOVER_RADIUS = 12;
export const DRAG_ACTIVATION_DISTANCE = 4;
export const CLICK_TOLERANCE = 6;
export const TIMELINE_SPACING = 0.95;
export const TIMELINE_PADDING = 160;
export const TIMELINE_MARGIN = 40;
export const TIMELINE_AUTO_CENTER_ALPHA = 0.12;
export const LABEL_FONT =
    "12px ui-monospace, SFMono-Regular, SF Mono, Menlo, Consolas, Liberation Mono, monospace";
export const LABEL_PADDING = 9;
export const ZOOM_MIN = 0.25;
export const ZOOM_MAX = 4;
export const BRANCH_NODE_PADDING_X = 10;
export const BRANCH_NODE_PADDING_Y = 6;
export const BRANCH_NODE_CORNER_RADIUS = 6;
export const BRANCH_NODE_OFFSET_X = 28;
export const BRANCH_NODE_OFFSET_Y = 6;
export const BRANCH_NODE_RADIUS = 18;
export const TOOLTIP_OFFSET_X = 18;
export const TOOLTIP_OFFSET_Y = -24;
export const HIGHLIGHT_NODE_RADIUS = NODE_RADIUS + 2.5;
export const MERGE_NODE_RADIUS = 7;
export const HIGHLIGHT_MERGE_NODE_RADIUS = 9.5;
export const NODE_SHADOW_BLUR = 6;
export const NODE_SHADOW_OFFSET_Y = 2;

// Tree icon constants (displayed on commit nodes instead of separate tree nodes)
export const TREE_ICON_SIZE = 7;
export const TREE_ICON_OFFSET = 2;

// Lane layout constants for lane-based positioning strategy
export const LANE_WIDTH = 120; // Pixels between lane centers
export const LANE_MARGIN = 60; // Left margin for lane 0
export const LANE_VERTICAL_STEP = 70; // Dedicated vertical spacing for lane mode
export const LANE_TRANSITION_DURATION = 300; // ms for mode switch animation
export const LANE_CORNER_RADIUS = 14; // Rounded corner radius for cross-lane stepped paths
export const LANE_HEADER_HEIGHT = 28; // Screen-space height of the lane header bar (px)
export const LANE_ARROW_CASING = 5; // Width of background-colored casing behind cross-lane arrows
export const LANE_COLORS = [
    "#3a86c4",  // Azure
    "#8b6cb5",  // Wisteria
    "#3d9970",  // Eucalyptus
    "#cc8c3c",  // Amber
    "#c4584a",  // Terra cotta
    "#5a7fb5",  // Slate
    "#a06090",  // Mauve
    "#6a9a4a",  // Moss
];

// Tag node styling
export const TAG_NODE_COLOR = "#ffd33d";
export const TAG_NODE_BORDER_COLOR = "#9a6700";
export const TAG_NODE_OFFSET_X = 28;
export const TAG_NODE_OFFSET_Y = 6;
export const TAG_NODE_RADIUS = 18;

// Progressive detail: commit message, author, and date zoom thresholds
export const COMMIT_MESSAGE_ZOOM_THRESHOLD = 1.5;
export const COMMIT_MESSAGE_MAX_CHARS = 60;
export const COMMIT_MESSAGE_FONT = "11px ui-monospace, SFMono-Regular, SF Mono, Menlo, Consolas, Liberation Mono, monospace";
export const COMMIT_AUTHOR_ZOOM_THRESHOLD = 2.0;
export const COMMIT_DATE_ZOOM_THRESHOLD = 3.0;
export const COMMIT_DETAIL_FONT = "10px -apple-system, BlinkMacSystemFont, 'Segoe UI', Helvetica, Arial, sans-serif";

// Hover glow effect
export const HOVER_GLOW_EXTRA_RADIUS = 4;
export const HOVER_GLOW_OPACITY = 0.25;


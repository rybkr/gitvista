/**
 * @fileoverview Author color utilities for the Git graph visualization.
 * Maps author email addresses to consistent, visually distinct colors
 * using a curated palette that harmonizes with GitVista's teal (#0ea5e9)
 * design system.
 */

/** Memoized map from email string to computed color string. */
const colorCache = new Map();

/**
 * Curated author color palette — 16 hues chosen for:
 * - Visual distinction from each other at small node sizes
 * - Harmony with the teal (#0ea5e9) accent and both light/dark themes
 * - Avoidance of pure red/green (diff semantics) and the exact teal accent
 *
 * Each entry is [hue, saturation%, lightness%] tuned for canvas rendering.
 */
const AUTHOR_PALETTE = [
    [205, 72, 58],  // cerulean blue
    [340, 65, 60],  // rose
    [162, 55, 48],  // sea green
    [28,  75, 58],  // tangerine
    [270, 52, 62],  // soft violet
    [48,  72, 52],  // golden amber
    [190, 60, 52],  // teal-cyan
    [315, 48, 58],  // orchid
    [95,  45, 48],  // olive sage
    [15,  68, 56],  // coral
    [240, 48, 62],  // periwinkle
    [145, 50, 45],  // jade
    [355, 58, 55],  // crimson-rose
    [55,  58, 48],  // chartreuse gold
    [285, 42, 58],  // lavender
    [175, 52, 48],  // deep cyan
];

/**
 * Hashes a string using the djb2 algorithm, producing an unsigned 32-bit integer.
 *
 * @param {string} str Input string to hash.
 * @returns {number} Non-negative 32-bit integer hash value.
 */
function djb2Hash(str) {
    let hash = 5381;
    for (let i = 0; i < str.length; i++) {
        hash = ((hash << 5) + hash + str.charCodeAt(i)) >>> 0;
    }
    return hash;
}

/**
 * Returns a deterministic HSL color string for a given author email.
 * Selects from a curated 16-color palette for visual harmony, then
 * applies a small hue jitter (+/- 8 degrees) so that authors who map
 * to the same palette slot are still distinguishable.
 *
 * Results are memoized so repeated calls for the same email are O(1).
 *
 * @param {string} email Author email address.
 * @returns {string} CSS HSL color string.
 */
export function getAuthorColor(email) {
    if (!email) return "hsl(205, 72%, 58%)";

    const cached = colorCache.get(email);
    if (cached !== undefined) return cached;

    const hash = djb2Hash(email);
    const [h, s, l] = AUTHOR_PALETTE[hash % AUTHOR_PALETTE.length];
    // Jitter: use upper bits for a -8..+8 degree hue offset
    const jitter = ((hash >>> 16) % 17) - 8;
    const color = `hsl(${h + jitter}, ${s}%, ${l}%)`;

    colorCache.set(email, color);
    return color;
}

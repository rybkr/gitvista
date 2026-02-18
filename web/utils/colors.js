/**
 * @fileoverview Author color utilities for the Git graph visualization.
 * Maps author email addresses to consistent, visually distinct colors
 * suitable for rendering on dark backgrounds.
 */

/** Memoized map from email string to computed HSL color string. */
const colorCache = new Map();

/**
 * Hashes a string using the djb2 algorithm, producing an unsigned 32-bit integer.
 * djb2 is fast and has good distribution for short strings like email addresses.
 *
 * @param {string} str Input string to hash.
 * @returns {number} Non-negative 32-bit integer hash value.
 */
function djb2Hash(str) {
    let hash = 5381;
    for (let i = 0; i < str.length; i++) {
        // hash * 33 XOR charCode, kept unsigned with >>> 0
        hash = ((hash << 5) + hash + str.charCodeAt(i)) >>> 0;
    }
    return hash;
}

/**
 * Returns a deterministic HSL color string for a given author email.
 * The hue is derived from the email hash so each author gets a unique color.
 * Saturation (65%) and lightness (55%) are fixed for readability on dark
 * backgrounds and sufficient contrast against white labels.
 *
 * Results are memoized so repeated calls for the same email are O(1).
 *
 * @param {string} email Author email address.
 * @returns {string} CSS color string in the form `hsl(H, 65%, 55%)`.
 */
export function getAuthorColor(email) {
    if (!email) {
        // Fall back to a neutral blue when no email is available.
        return "hsl(220, 65%, 55%)";
    }

    const cached = colorCache.get(email);
    if (cached !== undefined) {
        return cached;
    }

    const hash = djb2Hash(email);
    // Map the full 32-bit unsigned range onto 0–359 degrees.
    const hue = hash % 360;
    const color = `hsl(${hue}, 65%, 55%)`;

    colorCache.set(email, color);
    return color;
}

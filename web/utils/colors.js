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

/**
 * Converts an HSL triplet to RGB.
 *
 * @param {number} h Hue in degrees (0–360).
 * @param {number} s Saturation percentage (0–100).
 * @param {number} l Lightness percentage (0–100).
 * @returns {{ r: number, g: number, b: number }} RGB values (0–255).
 */
function hslToRgb(h, s, l) {
    s /= 100;
    l /= 100;
    const a = s * Math.min(l, 1 - l);
    const f = (n) => {
        const k = (n + h / 30) % 12;
        return l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1));
    };
    return {
        r: Math.round(f(0) * 255),
        g: Math.round(f(8) * 255),
        b: Math.round(f(4) * 255),
    };
}

/**
 * Converts RGB values to an HSL triplet.
 *
 * @param {number} r Red (0–255).
 * @param {number} g Green (0–255).
 * @param {number} b Blue (0–255).
 * @returns {{ h: number, s: number, l: number }} Hue (0–360), saturation (0–100), lightness (0–100).
 */
function rgbToHsl(r, g, b) {
    r /= 255;
    g /= 255;
    b /= 255;
    const max = Math.max(r, g, b);
    const min = Math.min(r, g, b);
    const l = (max + min) / 2;
    if (max === min) return { h: 0, s: 0, l: l * 100 };
    const d = max - min;
    const s = l > 0.5 ? d / (2 - max - min) : d / (max + min);
    let h;
    if (max === r) h = ((g - b) / d + (g < b ? 6 : 0)) / 6;
    else if (max === g) h = ((b - r) / d + 2) / 6;
    else h = ((r - g) / d + 4) / 6;
    return { h: h * 360, s: s * 100, l: l * 100 };
}

/**
 * Parses a CSS color string (hex or hsl) into HSL and RGB components.
 *
 * @param {string} color CSS color string (#hex or hsl(...)).
 * @returns {{ h: number, s: number, l: number, r: number, g: number, b: number }}
 */
function parseColor(color) {
    const hslMatch = color.match(
        /hsl\(\s*([\d.]+)\s*,\s*([\d.]+)%?\s*,\s*([\d.]+)%?\s*\)/,
    );
    if (hslMatch) {
        const h = parseFloat(hslMatch[1]);
        const s = parseFloat(hslMatch[2]);
        const l = parseFloat(hslMatch[3]);
        const { r, g, b } = hslToRgb(h, s, l);
        return { h, s, l, r, g, b };
    }
    const hex = color.replace("#", "");
    const fullHex =
        hex.length === 3 ? hex.split("").map((c) => c + c).join("") : hex;
    const r = parseInt(fullHex.substring(0, 2), 16);
    const g = parseInt(fullHex.substring(2, 4), 16);
    const b = parseInt(fullHex.substring(4, 6), 16);
    const hsl = rgbToHsl(r, g, b);
    return { ...hsl, r, g, b };
}

/** Memoized map from "color|isDark" to computed highlight object. */
const highlightCache = new Map();

/**
 * Computes highlight color variants (glow, core, highlight, ring) for an
 * arbitrary base color. Used so that a node's highlight treatment matches
 * its actual rendered color rather than a fixed palette value.
 *
 * @param {string} baseColor CSS color string (hex or hsl).
 * @param {boolean} isDark Whether the current theme is dark.
 * @returns {{ glow: string, core: string, highlight: string, ring: string }}
 */
export function computeHighlightColors(baseColor, isDark) {
    const key = `${baseColor}|${isDark ? 1 : 0}`;
    const cached = highlightCache.get(key);
    if (cached) return cached;

    const { h, s, r, g, b } = parseColor(baseColor);
    const result = {
        glow: `rgba(${r}, ${g}, ${b}, 0.3)`,
        core: isDark
            ? `hsl(${h}, ${Math.min(s, 50)}%, 13%)`
            : `hsl(${h}, ${Math.min(s, 60)}%, 96%)`,
        highlight: baseColor,
        ring: baseColor,
    };
    highlightCache.set(key, result);
    return result;
}

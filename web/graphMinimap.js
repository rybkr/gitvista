/**
 * @fileoverview Minimap canvas overlay showing a simplified graph overview
 * with a viewport rectangle. Supports click-to-jump navigation.
 */

const MINIMAP_WIDTH = 200;
const MINIMAP_HEIGHT = 140;
const MINIMAP_PADDING = 10;

/**
 * Creates the minimap component.
 *
 * @param {{
 *   getNodes: () => Array,
 *   getLinks: () => Array,
 *   getZoomTransform: () => object,
 *   getViewport: () => { width: number, height: number },
 *   onJump: (x: number, y: number) => void,
 * }} deps
 * @returns {{ el: HTMLElement, render: () => void, destroy: () => void }}
 */
export function createGraphMinimap(deps) {
    const container = document.createElement("div");
    container.className = "graph-minimap";

    const canvas = document.createElement("canvas");
    const dpr = window.devicePixelRatio || 1;
    canvas.width = MINIMAP_WIDTH * dpr;
    canvas.height = MINIMAP_HEIGHT * dpr;
    canvas.style.width = MINIMAP_WIDTH + "px";
    canvas.style.height = MINIMAP_HEIGHT + "px";
    container.appendChild(canvas);

    const ctx = canvas.getContext("2d");

    // Throttle: only render every 3rd call for performance.
    let frameCount = 0;

    // Cached bounding box + scale for click mapping.
    let cachedBounds = null;
    let cachedScale = 1;
    let cachedOffsetX = 0;
    let cachedOffsetY = 0;

    function computeBounds(nodes) {
        let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
        for (const n of nodes) {
            if (n.type === "branch" || n.type === "tag" || n.type === "ghost-merge") continue;
            if (typeof n.x !== "number" || typeof n.y !== "number") continue;
            if (n.x < minX) minX = n.x;
            if (n.x > maxX) maxX = n.x;
            if (n.y < minY) minY = n.y;
            if (n.y > maxY) maxY = n.y;
        }
        if (!isFinite(minX)) return null;
        return { minX, minY, maxX, maxY };
    }

    function render() {
        frameCount++;
        if (frameCount % 3 !== 0) return;

        const nodes = deps.getNodes();
        const links = deps.getLinks();
        const transform = deps.getZoomTransform();
        const viewport = deps.getViewport();

        if (!nodes || nodes.length === 0) return;

        const bounds = computeBounds(nodes);
        if (!bounds) return;
        cachedBounds = bounds;

        const bw = bounds.maxX - bounds.minX || 1;
        const bh = bounds.maxY - bounds.minY || 1;

        const drawW = MINIMAP_WIDTH * dpr;
        const drawH = MINIMAP_HEIGHT * dpr;
        const pad = MINIMAP_PADDING * dpr;
        const usableW = drawW - pad * 2;
        const usableH = drawH - pad * 2;

        const scale = Math.min(usableW / bw, usableH / bh);
        cachedScale = scale;
        cachedOffsetX = pad + (usableW - bw * scale) / 2;
        cachedOffsetY = pad + (usableH - bh * scale) / 2;

        const toMX = (x) => (x - bounds.minX) * scale + cachedOffsetX;
        const toMY = (y) => (y - bounds.minY) * scale + cachedOffsetY;

        ctx.clearRect(0, 0, drawW, drawH);

        // Draw links
        ctx.strokeStyle = getComputedStyle(document.documentElement)
            .getPropertyValue("--text-secondary").trim() || "#8b8f96";
        ctx.globalAlpha = 0.2;
        ctx.lineWidth = 1 * dpr;
        ctx.beginPath();
        for (const link of links) {
            const src = typeof link.source === "object" ? link.source : null;
            const tgt = typeof link.target === "object" ? link.target : null;
            if (!src || !tgt) continue;
            if (src.type === "branch" || src.type === "tag") continue;
            if (tgt.type === "branch" || tgt.type === "tag") continue;
            ctx.moveTo(toMX(src.x), toMY(src.y));
            ctx.lineTo(toMX(tgt.x), toMY(tgt.y));
        }
        ctx.stroke();
        ctx.globalAlpha = 1;

        // Draw nodes as 2px dots
        const nodeColor = getComputedStyle(document.documentElement)
            .getPropertyValue("--node-color").trim() || "#58a6ff";
        ctx.fillStyle = nodeColor;
        const dotR = 1.5 * dpr;
        for (const n of nodes) {
            if (n.type !== "commit") continue;
            if (typeof n.x !== "number" || typeof n.y !== "number") continue;
            const mx = toMX(n.x);
            const my = toMY(n.y);
            ctx.beginPath();
            ctx.arc(mx, my, n.dimmed ? dotR * 0.6 : dotR, 0, Math.PI * 2);
            ctx.globalAlpha = n.dimmed ? 0.3 : 0.8;
            ctx.fill();
        }
        ctx.globalAlpha = 1;

        // Draw viewport rectangle
        if (transform && viewport.width > 0 && viewport.height > 0) {
            // The visible area in graph coordinates
            const inverted = transform.invert([0, 0]);
            const invertedBR = transform.invert([viewport.width, viewport.height]);

            const vx1 = toMX(inverted[0]);
            const vy1 = toMY(inverted[1]);
            const vx2 = toMX(invertedBR[0]);
            const vy2 = toMY(invertedBR[1]);

            const vw = vx2 - vx1;
            const vh = vy2 - vy1;

            const strokeColor = getComputedStyle(document.documentElement)
                .getPropertyValue("--minimap-viewport-stroke").trim() || "#58a6ff";
            const fillColor = getComputedStyle(document.documentElement)
                .getPropertyValue("--minimap-viewport-fill").trim() || "rgba(88,166,255,0.08)";

            ctx.fillStyle = fillColor;
            ctx.fillRect(vx1, vy1, vw, vh);

            ctx.strokeStyle = strokeColor;
            ctx.lineWidth = 1.5 * dpr;
            ctx.strokeRect(vx1, vy1, vw, vh);
        }
    }

    // Click-to-jump
    function handleClick(event) {
        if (!cachedBounds) return;
        const rect = canvas.getBoundingClientRect();
        const mx = (event.clientX - rect.left) * dpr;
        const my = (event.clientY - rect.top) * dpr;

        // Map minimap coords back to graph coords
        const gx = (mx - cachedOffsetX) / cachedScale + cachedBounds.minX;
        const gy = (my - cachedOffsetY) / cachedScale + cachedBounds.minY;

        deps.onJump(gx, gy);
    }

    // Drag support on minimap
    let dragging = false;

    function handlePointerDown(event) {
        dragging = true;
        canvas.setPointerCapture(event.pointerId);
        handleClick(event);
    }

    function handlePointerMove(event) {
        if (!dragging) return;
        handleClick(event);
    }

    function handlePointerUp() {
        dragging = false;
    }

    canvas.addEventListener("pointerdown", handlePointerDown);
    canvas.addEventListener("pointermove", handlePointerMove);
    canvas.addEventListener("pointerup", handlePointerUp);
    canvas.addEventListener("pointercancel", handlePointerUp);

    function destroy() {
        canvas.removeEventListener("pointerdown", handlePointerDown);
        canvas.removeEventListener("pointermove", handlePointerMove);
        canvas.removeEventListener("pointerup", handlePointerUp);
        canvas.removeEventListener("pointercancel", handlePointerUp);
        container.remove();
    }

    return { el: container, render, destroy };
}

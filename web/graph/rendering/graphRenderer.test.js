import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { GraphRenderer } from "./graphRenderer.js";

function makeCtx() {
    return {
        restoreCalls: 0,
        restore() {
            this.restoreCalls += 1;
        },
    };
}

function makeRenderer() {
    const ctx = makeCtx();
    const canvas = { getContext: () => ctx };
    const palette = { background: "#000", gridDot: "#111", isDark: true };
    const renderer = new GraphRenderer(canvas, palette);
    return { renderer, ctx };
}

describe("GraphRenderer.render", () => {
    it("passes computed viewport culling bounds into link/node rendering", () => {
        const { renderer } = makeRenderer();

        renderer.clear = () => {};
        renderer.renderDotGrid = () => {};
        renderer.setupTransform = () => {};

        let linkBounds = null;
        let nodeBounds = null;
        let layoutMode = null;
        renderer.renderLinks = (_links, _nodes, vpBounds) => {
            linkBounds = vpBounds;
        };
        renderer.renderNodes = (
            _nodes,
            _highlightKey,
            _zoomTransform,
            _headHash,
            _hoverNode,
            _tags,
            mode,
            vpBounds,
        ) => {
            layoutMode = mode;
            nodeBounds = vpBounds;
        };
        renderer.renderLaneBackgrounds = () => {};
        renderer.renderLaneHeaders = () => {};
        renderer.renderStickyHeaders = () => {};

        renderer.render({
            nodes: [],
            links: [],
            zoomTransform: { x: 100, y: -50, k: 2 },
            viewportWidth: 800,
            viewportHeight: 600,
        });

        const expected = { left: -250, top: -175, right: 550, bottom: 525 };
        assert.deepEqual(linkBounds, expected);
        assert.deepEqual(nodeBounds, expected);
        assert.equal(layoutMode, "force");
    });

    it("renders lane overlays only when lane info exists", () => {
        const { renderer, ctx } = makeRenderer();

        renderer.clear = () => {};
        renderer.renderDotGrid = () => {};
        renderer.setupTransform = () => {};
        renderer.renderLinks = () => {};
        renderer.renderNodes = () => {};

        let backgroundCalls = 0;
        let headersCalls = 0;
        let stickyCalls = 0;
        renderer.renderLaneBackgrounds = () => { backgroundCalls += 1; };
        renderer.renderLaneHeaders = () => { headersCalls += 1; };
        renderer.renderStickyHeaders = () => { stickyCalls += 1; };

        renderer.render({
            nodes: [],
            links: [],
            zoomTransform: { x: 0, y: 0, k: 1 },
            viewportWidth: 100,
            viewportHeight: 80,
            laneInfo: [],
        });
        assert.equal(backgroundCalls, 0);
        assert.equal(headersCalls, 0);
        assert.equal(stickyCalls, 0);

        renderer.render({
            nodes: [],
            links: [],
            zoomTransform: { x: 0, y: 0, k: 1 },
            viewportWidth: 100,
            viewportHeight: 80,
            laneInfo: [{ position: 0, color: "#fff", minY: 0, maxY: 10 }],
        });
        assert.equal(backgroundCalls, 1);
        assert.equal(headersCalls, 1);
        assert.equal(stickyCalls, 1);
        assert.equal(ctx.restoreCalls, 2);
    });
});

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { CommitIndex } from "./commitIndex.js";

describe("CommitIndex", () => {
    it("indexes lane position data and annotates stash + stash-internal commits", () => {
        const index = new CommitIndex();
        const commits = new Map([
            ["stash", { hash: "stash", parents: ["base", "idx", "utr"] }],
            ["base", { hash: "base", parents: [] }],
            ["idx", { hash: "idx", parents: [] }],
            ["utr", { hash: "utr", parents: [] }],
        ]);
        const positionData = {
            transitionTargetPositions: new Map([
                ["base", { x: 10, y: 100 }],
                ["stash", { x: 20, y: 40 }],
                ["idx", { x: 30, y: 70 }],
                ["utr", { x: 40, y: 130 }],
            ]),
            commitToLane: new Map([["stash", 2]]),
            commitToSegmentId: new Map([["stash", "s-1"]]),
        };
        const stashes = [{ hash: "stash", message: "WIP work" }];

        index.rebuild(commits, positionData, stashes);

        assert.equal(index.size, 4);
        assert.equal(index.getByHash("stash")?.isStash, true);
        assert.equal(index.getByHash("stash")?.stashMessage, "WIP work");
        assert.equal(index.getByHash("idx")?.isStashInternal, true);
        assert.equal(index.getByHash("idx")?.stashInternalKind, "index");
        assert.equal(index.getByHash("utr")?.isStashInternal, true);
        assert.equal(index.getByHash("utr")?.stashInternalKind, "untracked");

        // queryYRange is inclusive and based on Y-sorted entries.
        const inRange = index.queryYRange(40, 100).map((e) => e.hash);
        assert.deepEqual(inRange, ["stash", "idx", "base"]);
    });

    it("rebuildFromPositions creates entries with neutral lane metadata", () => {
        const index = new CommitIndex();
        const commits = new Map([["a", { hash: "a", parents: [] }]]);
        const stashes = [{ hash: "a", message: "stash A" }];
        const positions = new Map([["a", { x: 1, y: 2 }]]);

        index.rebuildFromPositions(positions, commits, stashes);
        const entry = index.getByHash("a");

        assert.equal(entry?.laneIndex, 0);
        assert.equal(entry?.laneColor, "");
        assert.equal(entry?.segmentId, "");
        assert.equal(entry?.isStash, true);
        assert.equal(entry?.stashMessage, "stash A");
    });
});

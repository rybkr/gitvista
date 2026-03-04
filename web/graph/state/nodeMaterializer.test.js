import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { NodeMaterializer } from "./nodeMaterializer.js";

function makeEntry(hash, overrides = {}) {
    return {
        hash,
        x: 10,
        y: 20,
        laneIndex: 0,
        laneColor: "#abc",
        segmentId: "seg",
        isStash: false,
        isStashInternal: false,
        stashInternalKind: null,
        stashMessage: null,
        ...overrides,
    };
}

describe("NodeMaterializer", () => {
    it("hydrates visible commits and evicts commits that leave the viewport", () => {
        const materializer = new NodeMaterializer({ poolCapacity: 8 });
        const commits = new Map([
            ["a", { hash: "a", message: "A" }],
            ["b", { hash: "b", message: "B" }],
            ["c", { hash: "c", message: "C" }],
        ]);

        const first = materializer.synchronize(
            [makeEntry("a", { x: 1 }), makeEntry("b", { x: 2 })],
            commits,
        );
        assert.deepEqual(first.added.sort(), ["a", "b"]);
        assert.deepEqual(first.removed, []);
        assert.equal(materializer.getNode("a")?.commit?.message, "A");
        assert.equal(materializer.getNode("b")?.commit?.message, "B");

        // Simulate stale transient state on a live node; eviction should clear it.
        const oldB = materializer.getNode("b");
        oldB.dimmed = true;
        oldB.spawnPhase = 0.7;

        const second = materializer.synchronize([makeEntry("c", { x: 3 })], commits);
        assert.deepEqual(second.removed.sort(), ["a", "b"]);
        assert.deepEqual(second.added, ["c"]);
        assert.equal(materializer.getNode("a"), null);
        assert.equal(materializer.getNode("b"), null);
        assert.equal(materializer.getNode("c")?.commit?.message, "C");
        assert.equal(materializer.getNode("c")?.dimmed, undefined);
        assert.equal(materializer.getNode("c")?.spawnPhase, undefined);
    });

    it("forceMaterialize creates or updates a node with the given commit payload", () => {
        const materializer = new NodeMaterializer();
        const entry = makeEntry("x", { x: 42, y: 77, laneIndex: 2, laneColor: "#00f" });

        const node = materializer.forceMaterialize("x", entry, { hash: "x", message: "first" });
        assert.equal(node.hash, "x");
        assert.equal(node.x, 42);
        assert.equal(node.y, 77);
        assert.equal(node.commit?.message, "first");
        assert.equal(materializer.getMaterializedNodes().length, 1);

        const updated = materializer.forceMaterialize(
            "x",
            makeEntry("x", { x: 50, y: 90, laneIndex: 3, laneColor: "#f00" }),
            { hash: "x", message: "updated" },
        );
        assert.equal(updated, node);
        assert.equal(updated.x, 50);
        assert.equal(updated.laneIndex, 3);
        assert.equal(updated.commit?.message, "updated");
    });
});

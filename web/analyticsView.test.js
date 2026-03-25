import { describe, it } from "node:test";
import assert from "node:assert/strict";

import { computeReworkRate } from "./analyticsView.js";

describe("computeReworkRate", () => {
    it("tracks rework in linear time without counting same-timestamp commits as prior work", () => {
        const commits = new Map([
            ["a", { author: { when: "2026-01-01T00:00:00Z" } }],
            ["b", { author: { when: "2026-01-08T00:00:00Z" } }],
            ["c", { author: { when: "2026-01-08T00:00:00Z" } }],
            ["d", { author: { when: "2026-01-15T00:00:00Z" } }],
            ["e", { author: { when: "2026-02-20T00:00:00Z" } }],
        ]);

        const diffStats = new Map([
            ["a", { files: ["a.txt"] }],
            ["b", { files: ["a.txt", "b.txt"] }],
            ["c", { files: ["b.txt"] }],
            ["d", { files: ["b.txt"] }],
            ["e", { files: ["a.txt"] }],
        ]);

        const result = computeReworkRate(commits, diffStats, 0);

        assert.equal(result.weeks.length, 4);
        assert.equal(result.weeks[0].rate, 0);
        assert.ok(Math.abs(result.weeks[1].rate - (100 / 3)) < 0.001);
        assert.equal(result.weeks[2].rate, 100);
        assert.equal(result.weeks[3].rate, 0);
        assert.ok(Math.abs(result.avgRate - (100 / 3)) < 0.001);
    });
});

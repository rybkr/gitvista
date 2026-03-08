import { describe, it, mock } from "node:test";
import assert from "node:assert/strict";

import { buildFilterPredicate } from "./filterPredicate.js";

function makeCommit(hash, when, parents = []) {
    return {
        id: hash,
        hash,
        parents,
        author: { when },
        committer: { when },
    };
}

describe("buildFilterPredicate", () => {
    it("filters out commits older than the configured time window", () => {
        const now = Date.parse("2026-03-07T12:00:00.000Z");
        const dateNow = mock.method(Date, "now", () => now);

        try {
            const recentCommit = makeCommit("recent", "2026-03-05T12:00:00.000Z");
            const oldCommit = makeCommit("old", "2026-02-20T12:00:00.000Z");
            const commits = new Map([
                [recentCommit.hash, recentCommit],
                [oldCommit.hash, oldCommit],
            ]);

            const predicate = buildFilterPredicate(
                null,
                { hideRemotes: false, hideMerges: false, hideStashes: false, focusBranch: "" },
                new Map(),
                commits,
                [],
                null,
                null,
                [],
                { timeWindow: "7d", depthLimit: Infinity, branchRules: {} },
            );

            assert.equal(typeof predicate, "function");
            assert.equal(predicate({ type: "commit", hash: recentCommit.hash, commit: recentCommit }), true);
            assert.equal(predicate({ type: "commit", hash: oldCommit.hash, commit: oldCommit }), false);
        } finally {
            dateNow.mock.restore();
        }
    });
});

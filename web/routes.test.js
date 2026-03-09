import { describe, it } from "node:test";
import assert from "node:assert/strict";

import { parseHostedPath, parseLocalHash } from "./routes.js";

describe("parseHostedPath", () => {
    it("parses hosted landing and docs routes", () => {
        assert.deepEqual(parseHostedPath("/"), { page: "landing", repoId: null, commitHash: null });
        assert.deepEqual(parseHostedPath("/docs"), { page: "docs", repoId: null, commitHash: null });
        assert.deepEqual(parseHostedPath("/docs/"), { page: "docs", repoId: null, commitHash: null });
    });

    it("parses hosted repo routes", () => {
        assert.deepEqual(parseHostedPath("/repo/example"), {
            page: "repo",
            repoId: "example",
            commitHash: null,
        });
        assert.deepEqual(parseHostedPath("/repo/example/1234567890abcdef1234567890abcdef12345678"), {
            page: "repo",
            repoId: "example",
            commitHash: "1234567890abcdef1234567890abcdef12345678",
        });
    });

    it("falls back to landing for unknown paths", () => {
        assert.deepEqual(parseHostedPath("/unknown"), { page: "landing", repoId: null, commitHash: null });
    });
});

describe("parseLocalHash", () => {
    it("parses local commit hashes", () => {
        assert.deepEqual(parseLocalHash("#1234567890abcdef1234567890abcdef12345678"), {
            page: "repo",
            repoId: null,
            commitHash: "1234567890abcdef1234567890abcdef12345678",
        });
    });

    it("returns null commitHash for empty and invalid fragments", () => {
        assert.deepEqual(parseLocalHash(""), { page: "repo", repoId: null, commitHash: null });
        assert.deepEqual(parseLocalHash("#docs"), { page: "repo", repoId: null, commitHash: null });
    });
});

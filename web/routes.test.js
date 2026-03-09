import { describe, it } from "node:test";
import assert from "node:assert/strict";

import { parseHostedPath, parseLocalHash } from "./routes.js";

describe("parseHostedPath", () => {
    it("parses hosted landing and docs routes", () => {
        assert.deepEqual(parseHostedPath("/"), { page: "landing", repoId: null, commitHash: null, docsSection: null });
        assert.deepEqual(parseHostedPath("/install"), { page: "install", repoId: null, commitHash: null, docsSection: null });
        assert.deepEqual(parseHostedPath("/docs"), { page: "docs", repoId: null, commitHash: null, docsSection: null });
        assert.deepEqual(parseHostedPath("/docs/"), { page: "docs", repoId: null, commitHash: null, docsSection: null });
    });

    it("parses hosted docs section routes", () => {
        assert.deepEqual(parseHostedPath("/docs/hosted"), {
            page: "docs",
            repoId: null,
            commitHash: null,
            docsSection: "hosted",
        });
        assert.deepEqual(parseHostedPath("/docs/limits/"), {
            page: "docs",
            repoId: null,
            commitHash: null,
            docsSection: "limits",
        });
    });

    it("parses hosted repo routes", () => {
        assert.deepEqual(parseHostedPath("/repo/example"), {
            page: "repo",
            repoId: "example",
            commitHash: null,
            docsSection: null,
        });
        assert.deepEqual(parseHostedPath("/repo/example/1234567890abcdef1234567890abcdef12345678"), {
            page: "repo",
            repoId: "example",
            commitHash: "1234567890abcdef1234567890abcdef12345678",
            docsSection: null,
        });
    });

    it("falls back to landing for unknown paths", () => {
        assert.deepEqual(parseHostedPath("/unknown"), { page: "landing", repoId: null, commitHash: null, docsSection: null });
    });
});

describe("parseLocalHash", () => {
    it("parses local commit hashes", () => {
        assert.deepEqual(parseLocalHash("#1234567890abcdef1234567890abcdef12345678"), {
            page: "repo",
            repoId: null,
            commitHash: "1234567890abcdef1234567890abcdef12345678",
            docsSection: null,
        });
    });

    it("returns null commitHash for empty and invalid fragments", () => {
        assert.deepEqual(parseLocalHash(""), { page: "repo", repoId: null, commitHash: null, docsSection: null });
        assert.deepEqual(parseLocalHash("#docs"), { page: "repo", repoId: null, commitHash: null, docsSection: null });
    });
});

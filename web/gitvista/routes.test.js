import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { parseHostedPath } from "./routes.js";

describe("parseHostedPath", () => {
    it("parses the hosted landing path", () => {
        assert.deepEqual(parseHostedPath("/"), {
            page: "landing",
            repoId: null,
            commitHash: null,
            docsSection: null,
        });
    });

    it("parses repo loading paths", () => {
        assert.deepEqual(parseHostedPath("/repo/abc123/loading"), {
            page: "repo-loading",
            repoId: "abc123",
            commitHash: null,
            docsSection: null,
        });
    });

    it("parses hosted repo commit paths", () => {
        const hash = "0123456789abcdef0123456789abcdef01234567";
        assert.deepEqual(parseHostedPath(`/repo/abc123/${hash}`), {
            page: "repo",
            repoId: "abc123",
            commitHash: hash,
            docsSection: null,
        });
    });
});

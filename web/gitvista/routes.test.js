import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { buildHostedRepoApiBase, buildHostedRepoLoadingPath, buildHostedRepoPath, parseHostedPath } from "./routes.js";

describe("parseHostedPath", () => {
    it("parses the hosted landing path", () => {
        assert.deepEqual(parseHostedPath("/"), {
            page: "landing",
            accountSlug: "personal",
            repoId: null,
            commitHash: null,
            docsSection: null,
        });
    });

    it("parses account-scoped repo loading paths", () => {
        assert.deepEqual(parseHostedPath("/a/acme/r/abc123/loading"), {
            page: "repo-loading",
            accountSlug: "acme",
            repoId: "abc123",
            commitHash: null,
            docsSection: null,
        });
    });

    it("parses hosted repo commit paths", () => {
        const hash = "0123456789abcdef0123456789abcdef01234567";
        assert.deepEqual(parseHostedPath(`/a/acme/r/abc123/${hash}`), {
            page: "repo",
            accountSlug: "acme",
            repoId: "abc123",
            commitHash: hash,
            docsSection: null,
        });
    });

    it("keeps legacy hosted repo paths on the default account", () => {
        assert.deepEqual(parseHostedPath("/repo/abc123/loading"), {
            page: "repo-loading",
            accountSlug: "personal",
            repoId: "abc123",
            commitHash: null,
            docsSection: null,
        });
    });
});

describe("hosted route builders", () => {
    it("builds account-scoped repo paths", () => {
        assert.equal(buildHostedRepoPath("acme", "repo1"), "/a/acme/r/repo1");
        assert.equal(buildHostedRepoLoadingPath("acme", "repo1"), "/a/acme/r/repo1/loading");
        assert.equal(buildHostedRepoApiBase("acme", "repo1"), "/api/accounts/acme/repos/repo1");
    });
});

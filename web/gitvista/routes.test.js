import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { parseLocalHash, parseLocalLaunchTarget } from "./routes.js";

describe("parseLocalHash", () => {
    it("parses empty hashes as the repository view", () => {
        assert.deepEqual(parseLocalHash(""), {
            page: "repo",
            repoId: null,
            commitHash: null,
            docsSection: null,
        });
    });

    it("parses commit hashes from the URL fragment", () => {
        const hash = "0123456789abcdef0123456789abcdef01234567";
        assert.deepEqual(parseLocalHash(`#${hash}`), {
            page: "repo",
            repoId: null,
            commitHash: hash,
            docsSection: null,
        });
    });

    it("ignores non-commit fragments", () => {
        assert.deepEqual(parseLocalHash("#not-a-commit"), {
            page: "repo",
            repoId: null,
            commitHash: null,
            docsSection: null,
        });
    });
});

describe("parseLocalLaunchTarget", () => {
    it("parses file launch targets and commit hashes", () => {
        const hash = "0123456789abcdef0123456789abcdef01234567";
        assert.deepEqual(parseLocalLaunchTarget("?path=src/main.js", `#${hash}`), {
            path: "src/main.js",
            commitHash: hash,
        });
    });

    it("returns nulls when launch params are absent", () => {
        assert.deepEqual(parseLocalLaunchTarget("", ""), {
            path: null,
            commitHash: null,
        });
    });
});

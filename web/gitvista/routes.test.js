import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { parseHashFragment, parseLaunchTarget } from "./routes.js";

describe("parseHashFragment", () => {
    it("returns null when the fragment is empty", () => {
        assert.deepEqual(parseHashFragment(""), {
            commitHash: null,
        });
    });

    it("parses commit hashes from the URL fragment", () => {
        const hash = "0123456789abcdef0123456789abcdef01234567";
        assert.deepEqual(parseHashFragment(`#${hash}`), {
            commitHash: hash,
        });
    });

    it("ignores non-commit fragments", () => {
        assert.deepEqual(parseHashFragment("#not-a-commit"), {
            commitHash: null,
        });
    });
});

describe("parseLaunchTarget", () => {
    it("parses file launch targets and commit hashes", () => {
        const hash = "0123456789abcdef0123456789abcdef01234567";
        assert.deepEqual(parseLaunchTarget("?path=src/main.js", `#${hash}`), {
            path: "src/main.js",
            commitHash: hash,
        });
    });

    it("returns nulls when launch params are absent", () => {
        assert.deepEqual(parseLaunchTarget("", ""), {
            path: null,
            commitHash: null,
        });
    });
});

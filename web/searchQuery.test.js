/**
 * @fileoverview Tests for the search query parser and matcher.
 *
 * Run with: node --test web/searchQuery.test.js
 */

import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { parseSearchQuery, createSearchMatcher } from "./searchQuery.js";

// ── Helpers ──────────────────────────────────────────────────────────────────

/** Creates a minimal commit object for matcher testing. */
function makeCommit(overrides = {}) {
    return {
        hash: "aabbccdd00112233445566778899aabbccddeeff",
        message: "Initial commit",
        author: { name: "Alice", email: "alice@example.com", when: "2025-06-15T12:00:00Z" },
        parents: [],
        ...overrides,
    };
}

// ── parseSearchQuery ─────────────────────────────────────────────────────────

describe("parseSearchQuery", () => {
    describe("empty / blank input", () => {
        it("returns isEmpty for empty string", () => {
            const q = parseSearchQuery("");
            assert.equal(q.isEmpty, true);
        });

        it("returns isEmpty for whitespace-only", () => {
            const q = parseSearchQuery("   ");
            assert.equal(q.isEmpty, true);
        });

        it("returns isEmpty for null", () => {
            const q = parseSearchQuery(null);
            assert.equal(q.isEmpty, true);
        });

        it("returns isEmpty for undefined", () => {
            const q = parseSearchQuery(undefined);
            assert.equal(q.isEmpty, true);
        });
    });

    describe("bare text terms", () => {
        it("parses a single word", () => {
            const q = parseSearchQuery("fix");
            assert.deepEqual(q.textTerms, ["fix"]);
            assert.equal(q.isEmpty, false);
        });

        it("parses multiple words as separate terms", () => {
            const q = parseSearchQuery("fix bug");
            assert.deepEqual(q.textTerms, ["fix", "bug"]);
        });

        it("lowercases text terms", () => {
            const q = parseSearchQuery("FIX BUG");
            assert.deepEqual(q.textTerms, ["fix", "bug"]);
        });

        it("treats unrecognized qualifier-like tokens as bare text", () => {
            const q = parseSearchQuery("foo:bar");
            assert.deepEqual(q.textTerms, ["foo:bar"]);
            assert.equal(q.errors.length, 0);
        });
    });

    describe("negated bare text", () => {
        it("parses -word as negated text term", () => {
            const q = parseSearchQuery("-fix");
            assert.deepEqual(q.negatedTextTerms, ["fix"]);
            assert.deepEqual(q.textTerms, []);
        });

        it("handles mixed positive and negated terms", () => {
            const q = parseSearchQuery("bug -wip");
            assert.deepEqual(q.textTerms, ["bug"]);
            assert.deepEqual(q.negatedTextTerms, ["wip"]);
        });

        it("does not treat lone dash as negated term", () => {
            const q = parseSearchQuery("-");
            // "-" alone is length 1, won't trigger negation, treated as bare text
            assert.deepEqual(q.textTerms, ["-"]);
            assert.deepEqual(q.negatedTextTerms, []);
        });
    });

    describe("author: qualifier", () => {
        it("parses author: into authors array", () => {
            const q = parseSearchQuery("author:alice");
            assert.deepEqual(q.authors, ["alice"]);
            assert.equal(q.isEmpty, false);
        });

        it("lowercases author values", () => {
            const q = parseSearchQuery("author:Alice");
            assert.deepEqual(q.authors, ["alice"]);
        });

        it("supports multiple author: qualifiers (OR)", () => {
            const q = parseSearchQuery("author:alice author:bob");
            assert.deepEqual(q.authors, ["alice", "bob"]);
        });

        it("supports quoted values with spaces", () => {
            const q = parseSearchQuery('author:"Jane Doe"');
            assert.deepEqual(q.authors, ["jane doe"]);
        });

        it("ignores empty author: value", () => {
            const q = parseSearchQuery("author:");
            assert.deepEqual(q.authors, []);
            assert.equal(q.isEmpty, true);
        });

        it("parses -author: as negated", () => {
            const q = parseSearchQuery("-author:bot");
            assert.deepEqual(q.negatedAuthors, ["bot"]);
            assert.deepEqual(q.authors, []);
        });
    });

    describe("hash: qualifier", () => {
        it("parses hash: into hashes array", () => {
            const q = parseSearchQuery("hash:abc123");
            assert.deepEqual(q.hashes, ["abc123"]);
        });

        it("lowercases hash values", () => {
            const q = parseSearchQuery("hash:ABC123");
            assert.deepEqual(q.hashes, ["abc123"]);
        });

        it("parses -hash: as negated", () => {
            const q = parseSearchQuery("-hash:abc");
            assert.deepEqual(q.negatedHashes, ["abc"]);
            assert.deepEqual(q.hashes, []);
        });
    });

    describe("after: / before: qualifiers", () => {
        it("parses after: with ISO date", () => {
            const q = parseSearchQuery("after:2024-01-15");
            assert.ok(q.after instanceof Date);
            assert.equal(q.after.toISOString().startsWith("2024-01-15"), true);
        });

        it("parses before: with ISO date", () => {
            const q = parseSearchQuery("before:2024-06-30");
            assert.ok(q.before instanceof Date);
            assert.equal(q.before.toISOString().startsWith("2024-06-30"), true);
        });

        it("parses relative date 7d", () => {
            const q = parseSearchQuery("after:7d");
            assert.ok(q.after instanceof Date);
            const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
            // Allow 1 second tolerance for test execution time
            assert.ok(Math.abs(q.after.getTime() - sevenDaysAgo) < 1000);
        });

        it("parses relative date 2w", () => {
            const q = parseSearchQuery("after:2w");
            assert.ok(q.after instanceof Date);
            const twoWeeksAgo = Date.now() - 14 * 24 * 60 * 60 * 1000;
            assert.ok(Math.abs(q.after.getTime() - twoWeeksAgo) < 1000);
        });

        it("parses relative date 3m", () => {
            const q = parseSearchQuery("after:3m");
            assert.ok(q.after instanceof Date);
        });

        it("parses relative date 1y", () => {
            const q = parseSearchQuery("after:1y");
            assert.ok(q.after instanceof Date);
        });

        it("reports error for invalid date", () => {
            const q = parseSearchQuery("after:not-a-date");
            assert.equal(q.after, null);
            assert.equal(q.errors.length, 1);
            assert.ok(q.errors[0].message.includes("Invalid date"));
        });

        it("reports error for negated after:", () => {
            const q = parseSearchQuery("-after:7d");
            assert.equal(q.after, null);
            assert.equal(q.errors.length, 1);
            assert.ok(q.errors[0].message.includes("not supported"));
        });

        it("reports error for negated before:", () => {
            const q = parseSearchQuery("-before:7d");
            assert.equal(q.before, null);
            assert.equal(q.errors.length, 1);
            assert.ok(q.errors[0].message.includes("not supported"));
        });

        it("supports year-only ISO date", () => {
            const q = parseSearchQuery("after:2024");
            assert.ok(q.after instanceof Date);
        });

        it("supports year-month ISO date", () => {
            const q = parseSearchQuery("after:2024-06");
            assert.ok(q.after instanceof Date);
        });
    });

    describe("merge: qualifier", () => {
        it("parses merge:only", () => {
            const q = parseSearchQuery("merge:only");
            assert.equal(q.merge, "only");
            assert.equal(q.negateMerge, false);
        });

        it("parses merge:exclude", () => {
            const q = parseSearchQuery("merge:exclude");
            assert.equal(q.merge, "exclude");
        });

        it("reports error for unknown merge value", () => {
            const q = parseSearchQuery("merge:foo");
            assert.equal(q.merge, null);
            assert.equal(q.errors.length, 1);
            assert.ok(q.errors[0].message.includes("merge:only"));
        });

        it("parses -merge:only with negation flag", () => {
            const q = parseSearchQuery("-merge:only");
            assert.equal(q.merge, "only");
            assert.equal(q.negateMerge, true);
        });
    });

    describe("branch: qualifier", () => {
        it("parses branch: value", () => {
            const q = parseSearchQuery("branch:main");
            assert.equal(q.branch, "main");
            assert.equal(q.negateBranch, false);
        });

        it("parses -branch: with negation", () => {
            const q = parseSearchQuery("-branch:feature");
            assert.equal(q.branch, "feature");
            assert.equal(q.negateBranch, true);
        });
    });

    describe("message: qualifier", () => {
        it("parses message: into messages array", () => {
            const q = parseSearchQuery("message:refactor");
            assert.deepEqual(q.messages, ["refactor"]);
        });

        it("lowercases message values", () => {
            const q = parseSearchQuery("message:REFACTOR");
            assert.deepEqual(q.messages, ["refactor"]);
        });

        it("parses -message: as negated", () => {
            const q = parseSearchQuery("-message:wip");
            assert.deepEqual(q.negatedMessages, ["wip"]);
            assert.deepEqual(q.messages, []);
        });

        it("supports quoted multi-word messages", () => {
            const q = parseSearchQuery('message:"fix bug"');
            assert.deepEqual(q.messages, ["fix bug"]);
        });
    });

    describe("tag: qualifier", () => {
        it("parses tag: into tags array", () => {
            const q = parseSearchQuery("tag:v1.0");
            assert.deepEqual(q.tags, ["v1.0"]);
        });

        it("parses -tag: as negated", () => {
            const q = parseSearchQuery("-tag:rc");
            assert.deepEqual(q.negatedTags, ["rc"]);
            assert.deepEqual(q.tags, []);
        });
    });

    describe("file: qualifier", () => {
        it("parses file: into files array", () => {
            const q = parseSearchQuery("file:main.go");
            assert.deepEqual(q.files, ["main.go"]);
        });

        it("lowercases file values", () => {
            const q = parseSearchQuery("file:README.md");
            assert.deepEqual(q.files, ["readme.md"]);
        });

        it("parses -file: as negated", () => {
            const q = parseSearchQuery("-file:test.go");
            assert.deepEqual(q.negatedFiles, ["test.go"]);
            assert.deepEqual(q.files, []);
        });
    });

    describe("path: qualifier", () => {
        it("parses path: into paths array", () => {
            const q = parseSearchQuery("path:internal/server");
            assert.deepEqual(q.paths, ["internal/server"]);
        });

        it("parses -path: as negated", () => {
            const q = parseSearchQuery("-path:vendor");
            assert.deepEqual(q.negatedPaths, ["vendor"]);
            assert.deepEqual(q.paths, []);
        });
    });

    describe("combined qualifiers", () => {
        it("parses author + after together", () => {
            const q = parseSearchQuery("author:alice after:7d");
            assert.deepEqual(q.authors, ["alice"]);
            assert.ok(q.after instanceof Date);
            assert.equal(q.isEmpty, false);
        });

        it("parses mixed qualifiers and bare text", () => {
            const q = parseSearchQuery("fix author:alice hash:abc");
            assert.deepEqual(q.textTerms, ["fix"]);
            assert.deepEqual(q.authors, ["alice"]);
            assert.deepEqual(q.hashes, ["abc"]);
        });

        it("parses negated and positive of same qualifier", () => {
            const q = parseSearchQuery("author:alice -author:bot");
            assert.deepEqual(q.authors, ["alice"]);
            assert.deepEqual(q.negatedAuthors, ["bot"]);
        });
    });

    describe("tokenizer edge cases", () => {
        it("handles double-quoted bare token", () => {
            const q = parseSearchQuery('"multi word"');
            assert.deepEqual(q.textTerms, ["multi word"]);
        });

        it("handles extra whitespace between tokens", () => {
            const q = parseSearchQuery("  fix   bug  ");
            assert.deepEqual(q.textTerms, ["fix", "bug"]);
        });

        it("handles unclosed quote gracefully", () => {
            const q = parseSearchQuery('"unclosed');
            assert.deepEqual(q.textTerms, ["unclosed"]);
        });

        it("handles qualifier with quoted value containing spaces", () => {
            const q = parseSearchQuery('author:"Jane Doe" message:"fix bug"');
            assert.deepEqual(q.authors, ["jane doe"]);
            assert.deepEqual(q.messages, ["fix bug"]);
        });
    });

    describe("isEmpty semantics", () => {
        it("is true when only errors exist (no valid criteria)", () => {
            const q = parseSearchQuery("after:garbage");
            assert.equal(q.errors.length, 1);
            assert.equal(q.isEmpty, true);
        });

        it("is false when negated terms exist", () => {
            const q = parseSearchQuery("-wip");
            assert.equal(q.isEmpty, false);
        });

        it("is false when only negated qualifiers exist", () => {
            const q = parseSearchQuery("-author:bot");
            assert.equal(q.isEmpty, false);
        });
    });
});

// ── createSearchMatcher ──────────────────────────────────────────────────────

describe("createSearchMatcher", () => {
    describe("null matcher for empty queries", () => {
        it("returns null for empty query", () => {
            const q = parseSearchQuery("");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher, null);
        });
    });

    describe("bare text matching", () => {
        it("matches message substring", () => {
            const q = parseSearchQuery("initial");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("is case-insensitive on message", () => {
            const q = parseSearchQuery("INITIAL");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("matches author name", () => {
            const q = parseSearchQuery("alice");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("matches author email", () => {
            const q = parseSearchQuery("example.com");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("matches hash prefix", () => {
            const q = parseSearchQuery("aabbcc");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("rejects non-matching text", () => {
            const q = parseSearchQuery("nonexistent");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });

        it("requires ALL text terms to match (AND)", () => {
            const q = parseSearchQuery("alice initial");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);

            const q2 = parseSearchQuery("alice nonexistent");
            const matcher2 = createSearchMatcher(q2);
            assert.equal(matcher2(makeCommit()), false);
        });
    });

    describe("negated text matching", () => {
        it("excludes commits matching negated term in message", () => {
            const q = parseSearchQuery("-initial");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });

        it("excludes commits matching negated term in author", () => {
            const q = parseSearchQuery("-alice");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });

        it("keeps commits not matching negated term", () => {
            const q = parseSearchQuery("-nonexistent");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });
    });

    describe("author: qualifier matching", () => {
        it("matches by author name", () => {
            const q = parseSearchQuery("author:alice");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("matches by author email", () => {
            const q = parseSearchQuery("author:example.com");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("rejects non-matching author", () => {
            const q = parseSearchQuery("author:bob");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });

        it("ORs multiple author: qualifiers", () => {
            const q = parseSearchQuery("author:alice author:bob");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true); // alice matches
            assert.equal(matcher(makeCommit({ author: { name: "Bob", email: "bob@test.com", when: "2025-01-01T00:00:00Z" } })), true);
            assert.equal(matcher(makeCommit({ author: { name: "Charlie", email: "c@test.com", when: "2025-01-01T00:00:00Z" } })), false);
        });

        it("excludes with -author:", () => {
            const q = parseSearchQuery("-author:alice");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
            assert.equal(matcher(makeCommit({ author: { name: "Bob", email: "bob@test.com", when: "2025-01-01T00:00:00Z" } })), true);
        });
    });

    describe("hash: qualifier matching", () => {
        it("matches hash prefix", () => {
            const q = parseSearchQuery("hash:aabbcc");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("rejects non-matching prefix", () => {
            const q = parseSearchQuery("hash:ffffff");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });

        it("excludes with -hash:", () => {
            const q = parseSearchQuery("-hash:aabbcc");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });
    });

    describe("after: / before: matching", () => {
        const oldCommit = makeCommit({ author: { name: "Alice", email: "a@test.com", when: "2020-01-01T00:00:00Z" } });
        const newCommit = makeCommit({ author: { name: "Alice", email: "a@test.com", when: "2025-06-15T00:00:00Z" } });

        it("after: filters out old commits", () => {
            const q = parseSearchQuery("after:2024-01-01");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(oldCommit), false);
            assert.equal(matcher(newCommit), true);
        });

        it("before: filters out new commits", () => {
            const q = parseSearchQuery("before:2022-01-01");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(oldCommit), true);
            assert.equal(matcher(newCommit), false);
        });

        it("after: + before: creates a date range", () => {
            const q = parseSearchQuery("after:2019-01-01 before:2021-01-01");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(oldCommit), true);
            assert.equal(matcher(newCommit), false);
        });

        it("handles commit with no author date", () => {
            const q = parseSearchQuery("after:2024-01-01");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit({ author: { name: "X" } })), false);
        });
    });

    describe("message: qualifier matching", () => {
        it("matches message substring", () => {
            const q = parseSearchQuery("message:initial");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);
        });

        it("does NOT match author (unlike bare text)", () => {
            const q = parseSearchQuery("message:alice");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });

        it("excludes with -message:", () => {
            const q = parseSearchQuery("-message:initial");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), false);
        });
    });

    describe("merge: qualifier matching", () => {
        const normalCommit = makeCommit({ parents: ["aaa"] });
        const mergeCommit = makeCommit({ parents: ["aaa", "bbb"] });

        it("merge:only keeps only merge commits", () => {
            const q = parseSearchQuery("merge:only");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(normalCommit), false);
            assert.equal(matcher(mergeCommit), true);
        });

        it("merge:exclude removes merge commits", () => {
            const q = parseSearchQuery("merge:exclude");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(normalCommit), true);
            assert.equal(matcher(mergeCommit), false);
        });

        it("-merge:only inverts to exclude behavior", () => {
            const q = parseSearchQuery("-merge:only");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(normalCommit), true);
            assert.equal(matcher(mergeCommit), false);
        });

        it("-merge:exclude inverts to only behavior", () => {
            const q = parseSearchQuery("-merge:exclude");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(normalCommit), false);
            assert.equal(matcher(mergeCommit), true);
        });

        it("treats zero-parent commit as non-merge", () => {
            const q = parseSearchQuery("merge:only");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit({ parents: [] })), false);
        });
    });

    describe("tag: qualifier matching", () => {
        const tags = new Map([
            ["v1.0.0", "aabbccdd00112233445566778899aabbccddeeff"],
            ["v2.0.0-rc1", "1111111111111111111111111111111111111111"],
        ]);

        it("matches commit pointed at by matching tag", () => {
            const q = parseSearchQuery("tag:v1.0");
            const matcher = createSearchMatcher(q, new Map(), new Map(), tags);
            assert.equal(matcher(makeCommit()), true);
        });

        it("rejects commit not pointed at by matching tag", () => {
            const q = parseSearchQuery("tag:v3");
            const matcher = createSearchMatcher(q, new Map(), new Map(), tags);
            assert.equal(matcher(makeCommit()), false);
        });

        it("excludes with -tag:", () => {
            const q = parseSearchQuery("-tag:v1.0");
            const matcher = createSearchMatcher(q, new Map(), new Map(), tags);
            assert.equal(matcher(makeCommit()), false); // v1.0 tag points at this commit
        });

        it("matches tag substring", () => {
            const q = parseSearchQuery("tag:rc");
            const matcher = createSearchMatcher(q, new Map(), new Map(), tags);
            assert.equal(matcher(makeCommit({ hash: "1111111111111111111111111111111111111111" })), true);
        });
    });

    describe("branch: qualifier matching", () => {
        const commits = new Map([
            ["aaa", makeCommit({ hash: "aaa", parents: ["bbb"] })],
            ["bbb", makeCommit({ hash: "bbb", parents: [] })],
            ["ccc", makeCommit({ hash: "ccc", parents: [] })],
        ]);
        const branches = new Map([["main", "aaa"]]);

        it("matches commits reachable from branch", () => {
            const q = parseSearchQuery("branch:main");
            const matcher = createSearchMatcher(q, branches, commits);
            assert.equal(matcher(commits.get("aaa")), true);
            assert.equal(matcher(commits.get("bbb")), true); // reachable via parent
            assert.equal(matcher(commits.get("ccc")), false); // unreachable
        });

        it("negated branch excludes reachable commits", () => {
            const q = parseSearchQuery("-branch:main");
            const matcher = createSearchMatcher(q, branches, commits);
            assert.equal(matcher(commits.get("aaa")), false);
            assert.equal(matcher(commits.get("bbb")), false);
            assert.equal(matcher(commits.get("ccc")), true);
        });

        it("unknown branch matches nothing", () => {
            const q = parseSearchQuery("branch:nonexistent");
            const matcher = createSearchMatcher(q, branches, commits);
            assert.equal(matcher(commits.get("aaa")), false);
        });

        it("resolves refs/heads/ prefix", () => {
            const branchesWithRef = new Map([["refs/heads/develop", "aaa"]]);
            const q = parseSearchQuery("branch:develop");
            const matcher = createSearchMatcher(q, branchesWithRef, commits);
            assert.equal(matcher(commits.get("aaa")), true);
        });
    });

    describe("file: qualifier matching", () => {
        const fileIndex = new Map([
            ["aabbccdd00112233445566778899aabbccddeeff", ["internal/server/handlers.go", "web/app.js"]],
        ]);

        it("matches by basename", () => {
            const q = parseSearchQuery("file:handlers.go");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), fileIndex);
            assert.equal(matcher(makeCommit()), true);
        });

        it("does not match by directory component", () => {
            const q = parseSearchQuery("file:server");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), fileIndex);
            assert.equal(matcher(makeCommit()), false);
        });

        it("rejects commit with no file data", () => {
            const q = parseSearchQuery("file:handlers.go");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), new Map());
            assert.equal(matcher(makeCommit()), false);
        });

        it("excludes with -file:", () => {
            const q = parseSearchQuery("-file:handlers.go");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), fileIndex);
            assert.equal(matcher(makeCommit()), false);
        });
    });

    describe("path: qualifier matching", () => {
        const fileIndex = new Map([
            ["aabbccdd00112233445566778899aabbccddeeff", ["internal/server/handlers.go", "web/app.js"]],
        ]);

        it("matches by directory prefix", () => {
            const q = parseSearchQuery("path:internal/server");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), fileIndex);
            assert.equal(matcher(makeCommit()), true);
        });

        it("matches exact file path", () => {
            const q = parseSearchQuery("path:web/app.js");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), fileIndex);
            assert.equal(matcher(makeCommit()), true);
        });

        it("rejects non-matching path", () => {
            const q = parseSearchQuery("path:cmd");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), fileIndex);
            assert.equal(matcher(makeCommit()), false);
        });

        it("excludes with -path:", () => {
            const q = parseSearchQuery("-path:internal");
            const matcher = createSearchMatcher(q, new Map(), new Map(), new Map(), fileIndex);
            assert.equal(matcher(makeCommit()), false);
        });
    });

    describe("AND logic across qualifiers", () => {
        it("requires both author: and message: to match", () => {
            const q = parseSearchQuery("author:alice message:initial");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);

            const q2 = parseSearchQuery("author:bob message:initial");
            const matcher2 = createSearchMatcher(q2);
            assert.equal(matcher2(makeCommit()), false);
        });

        it("combines text term with qualifier", () => {
            const q = parseSearchQuery("initial author:alice");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit()), true);

            const q2 = parseSearchQuery("nonexistent author:alice");
            const matcher2 = createSearchMatcher(q2);
            assert.equal(matcher2(makeCommit()), false);
        });
    });

    describe("edge cases", () => {
        it("handles null commit", () => {
            const q = parseSearchQuery("fix");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(null), false);
        });

        it("handles commit with missing fields", () => {
            const q = parseSearchQuery("fix");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher({ hash: "", message: null, author: null, parents: null }), false);
        });

        it("handles PascalCase author fields (Go backend format)", () => {
            const q = parseSearchQuery("author:alice");
            const matcher = createSearchMatcher(q);
            const commit = makeCommit({ author: { Name: "Alice", Email: "alice@test.com", When: "2025-01-01T00:00:00Z" } });
            assert.equal(matcher(commit), true);
        });

        it("handles commit with no parents field for merge check", () => {
            const q = parseSearchQuery("merge:only");
            const matcher = createSearchMatcher(q);
            assert.equal(matcher(makeCommit({ parents: undefined })), false);
        });
    });
});

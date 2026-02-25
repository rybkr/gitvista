/**
 * @fileoverview Pure query parser and commit matcher for the GitVista search system.
 *
 * Converts raw search strings into structured SearchQuery objects and compiles
 * them into efficient predicate functions. Entirely side-effect-free — no DOM,
 * no network, no localStorage access.
 *
 * Supported qualifiers (case-insensitive):
 *   author:<value>       — match author name or email (substring, OR among multiples)
 *   hash:<value>         — match commit hash prefix (OR among multiples)
 *   after:<date>         — commits whose author date is after the given date
 *   before:<date>        — commits whose author date is before the given date
 *   merge:only           — show only merge commits (2+ parents)
 *   merge:exclude        — exclude merge commits
 *   branch:<name>        — commits reachable from the named branch (BFS)
 *   message:<value>      — match commit message only (substring, OR among multiples)
 *   tag:<value>          — commits pointed at by a tag matching the value (substring)
 *   file:<name>          — commits that touched a file with matching basename
 *   path:<prefix>        — commits that touched files under a directory prefix
 *
 * Negation: any qualifier or bare term can be prefixed with `-` to invert it.
 *   -author:bot          — exclude commits by authors matching "bot"
 *   -merge:only          — equivalent to merge:exclude
 *   -fix                 — exclude commits matching "fix" in message/author/hash
 * Negating date qualifiers (-after:, -before:) is not supported and produces a
 * parse error.
 *
 * Bare (unqualified) tokens are treated as message/author/hash substrings.
 * Multiple values for the same qualifier are OR'd; different qualifiers AND.
 * Unrecognized qualifier prefixes are treated as bare text (forgiving parser).
 */

// ── Types ──────────────────────────────────────────────────────────────────────

/**
 * @typedef {Object} ParseError
 * @property {string} token The raw token that caused the issue.
 * @property {string} message Human-readable explanation with guidance.
 */

/**
 * @typedef {Object} SearchQuery
 * @property {string} raw Original unmodified input string.
 * @property {string[]} textTerms Bare text terms (implicit message/author/hash search).
 * @property {string[]} negatedTextTerms Negated bare text terms.
 * @property {string[]} authors Normalized author: qualifier values.
 * @property {string[]} negatedAuthors Negated author: qualifier values.
 * @property {string[]} hashes Normalized hash: qualifier values.
 * @property {string[]} negatedHashes Negated hash: qualifier values.
 * @property {Date | null} after Resolved lower date bound, or null.
 * @property {Date | null} before Resolved upper date bound, or null.
 * @property {'only' | 'exclude' | null} merge Merge commit filter mode.
 * @property {boolean} negateMerge When true, inverts the merge filter logic.
 * @property {string | null} branch Branch name for reachability filtering.
 * @property {boolean} negateBranch When true, inverts branch reachability.
 * @property {string[]} messages Positive message: qualifier values.
 * @property {string[]} negatedMessages Negated message: qualifier values.
 * @property {string[]} tags Positive tag: qualifier patterns.
 * @property {string[]} negatedTags Negated tag: qualifier patterns.
 * @property {string[]} files Positive file: qualifier values (basename match).
 * @property {string[]} negatedFiles Negated file: qualifier values.
 * @property {string[]} paths Positive path: qualifier values (directory prefix match).
 * @property {string[]} negatedPaths Negated path: qualifier values.
 * @property {ParseError[]} errors Parse-time warnings for malformed qualifiers.
 * @property {boolean} isEmpty True when no meaningful criteria are present.
 */

// ── Date parsing ───────────────────────────────────────────────────────────────

/**
 * Parses a user-supplied date string into a Date object.
 * Supports ISO dates ("2024-01-15") and relative shorthand ("7d", "2w", "3m", "1y").
 * Returns null for unrecognized formats rather than throwing.
 *
 * @param {string} raw The raw date string from the qualifier value.
 * @returns {Date | null}
 */
function parseDate(raw) {
    const trimmed = raw.trim();
    if (!trimmed) return null;

    // Relative date: <number><unit> where unit is d/w/m/y (case-insensitive)
    const relativeMatch = trimmed.match(/^(\d+)([dwmy])$/i);
    if (relativeMatch) {
        const amount = parseInt(relativeMatch[1], 10);
        const unit = relativeMatch[2].toLowerCase();
        const now = Date.now();
        let offsetMs;
        switch (unit) {
            case "d": offsetMs = amount * 24 * 60 * 60 * 1000; break;
            case "w": offsetMs = amount * 7 * 24 * 60 * 60 * 1000; break;
            case "m": offsetMs = amount * 30 * 24 * 60 * 60 * 1000; break;
            case "y": offsetMs = amount * 365 * 24 * 60 * 60 * 1000; break;
            default: return null;
        }
        return new Date(now - offsetMs);
    }

    // ISO date or datetime: delegate to Date constructor.
    // We only accept strings that look like dates to avoid accidentally parsing
    // bare numbers (e.g. "2024" would parse as a year — acceptable) vs garbage.
    const isoMatch = trimmed.match(/^\d{4}(-\d{2}(-\d{2}(T[\d:]+Z?)?)?)?$/);
    if (isoMatch) {
        const d = new Date(trimmed);
        return isNaN(d.getTime()) ? null : d;
    }

    return null;
}

// ── Tokenizer ─────────────────────────────────────────────────────────────────

/**
 * Tokenizes a raw query string, respecting double-quoted strings.
 * Returns an array of raw token strings (quotes stripped from quoted tokens).
 *
 * Examples:
 *   'author:"Jane Doe" fix'      → ["author:Jane Doe", "fix"]
 *   'after:7d branch:main'       → ["after:7d", "branch:main"]
 *   '"multi word" hash:abc'      → ["multi word", "hash:abc"]
 *
 * @param {string} raw
 * @returns {string[]}
 */
function tokenize(raw) {
    const tokens = [];
    let i = 0;
    const len = raw.length;

    while (i < len) {
        // Skip leading whitespace
        while (i < len && raw[i] === " ") i++;
        if (i >= len) break;

        // Detect qualifier prefix before a quoted value: key:"quoted value"
        // Scan for a colon to find if this is a qualifier token.
        let token = "";

        if (raw[i] === '"') {
            // Purely quoted token: no qualifier prefix
            i++; // consume opening quote
            while (i < len && raw[i] !== '"') {
                token += raw[i++];
            }
            if (i < len) i++; // consume closing quote
        } else {
            // Read until whitespace or end, handling inline-quoted qualifier values.
            // For author:"Jane Doe": accumulates "author:" then strips the quotes and
            // appends "Jane Doe", yielding the token "author:Jane Doe".
            while (i < len && raw[i] !== " ") {
                if (raw[i] === '"') {
                    i++; // consume opening quote
                    while (i < len && raw[i] !== '"') {
                        token += raw[i++];
                    }
                    if (i < len) i++; // consume closing quote
                } else {
                    token += raw[i++];
                }
            }
        }

        if (token.length > 0) {
            tokens.push(token);
        }
    }

    return tokens;
}

// ── Known qualifiers ──────────────────────────────────────────────────────────

/** Set of recognized qualifier prefixes (lowercase, without colon). */
const KNOWN_QUALIFIERS = new Set(["author", "hash", "after", "before", "merge", "branch", "message", "tag", "file", "path"]);

// ── parseSearchQuery ──────────────────────────────────────────────────────────

/**
 * Parses a raw search string into a structured SearchQuery.
 *
 * @param {string} raw
 * @returns {SearchQuery}
 */
export function parseSearchQuery(raw) {
    const trimmed = (raw ?? "").trim();

    /** @type {SearchQuery} */
    const query = {
        raw: trimmed,
        textTerms: [],
        negatedTextTerms: [],
        authors: [],
        negatedAuthors: [],
        hashes: [],
        negatedHashes: [],
        after: null,
        before: null,
        merge: null,
        negateMerge: false,
        branch: null,
        negateBranch: false,
        messages: [],
        negatedMessages: [],
        tags: [],
        negatedTags: [],
        files: [],
        negatedFiles: [],
        paths: [],
        negatedPaths: [],
        errors: [],
        isEmpty: false,
    };

    if (!trimmed) {
        query.isEmpty = true;
        return query;
    }

    const tokens = tokenize(trimmed);

    for (const token of tokens) {
        // ── Detect negation prefix ──────────────────────────────────────────
        let negated = false;
        let workingToken = token;

        if (workingToken.startsWith("-") && workingToken.length > 1) {
            const rest = workingToken.slice(1);
            const colonIdx = rest.indexOf(":");
            if (colonIdx > 0) {
                const maybeQualifier = rest.slice(0, colonIdx).toLowerCase();
                if (KNOWN_QUALIFIERS.has(maybeQualifier)) {
                    negated = true;
                    workingToken = rest;
                }
            } else if (!rest.includes(":")) {
                // Bare negated text: "-word" (no colon)
                negated = true;
                workingToken = rest;
            }
        }

        // ── Check for qualifier: prefix ─────────────────────────────────────
        const colonIdx = workingToken.indexOf(":");
        if (colonIdx > 0) {
            const qualifier = workingToken.slice(0, colonIdx).toLowerCase();
            const value = workingToken.slice(colonIdx + 1);

            if (KNOWN_QUALIFIERS.has(qualifier)) {
                switch (qualifier) {
                    case "author":
                        if (value) {
                            if (negated) query.negatedAuthors.push(value.toLowerCase());
                            else query.authors.push(value.toLowerCase());
                        }
                        break;
                    case "hash":
                        if (value) {
                            if (negated) query.negatedHashes.push(value.toLowerCase());
                            else query.hashes.push(value.toLowerCase());
                        }
                        break;
                    case "after":
                        if (negated) {
                            query.errors.push({
                                token: token,
                                message: `Negating date qualifiers is not supported — use before: instead`,
                            });
                        } else if (value) {
                            const parsed = parseDate(value);
                            if (parsed) {
                                query.after = parsed;
                            } else {
                                query.errors.push({
                                    token: token,
                                    message: `Invalid date "${value}" — use ISO (2024-01-15) or relative (7d, 2w, 3m, 1y)`,
                                });
                            }
                        }
                        break;
                    case "before":
                        if (negated) {
                            query.errors.push({
                                token: token,
                                message: `Negating date qualifiers is not supported — use after: instead`,
                            });
                        } else if (value) {
                            const parsed = parseDate(value);
                            if (parsed) {
                                query.before = parsed;
                            } else {
                                query.errors.push({
                                    token: token,
                                    message: `Invalid date "${value}" — use ISO (2024-01-15) or relative (7d, 2w, 3m, 1y)`,
                                });
                            }
                        }
                        break;
                    case "merge":
                        if (value === "only" || value === "exclude") {
                            query.merge = value;
                            query.negateMerge = negated;
                        } else {
                            query.errors.push({
                                token: token,
                                message: `Unknown merge filter "${value}" — use merge:only or merge:exclude`,
                            });
                        }
                        break;
                    case "branch":
                        if (value) {
                            query.branch = value;
                            query.negateBranch = negated;
                        }
                        break;
                    case "message":
                        if (value) {
                            if (negated) query.negatedMessages.push(value.toLowerCase());
                            else query.messages.push(value.toLowerCase());
                        }
                        break;
                    case "tag":
                        if (value) {
                            if (negated) query.negatedTags.push(value.toLowerCase());
                            else query.tags.push(value.toLowerCase());
                        }
                        break;
                    case "file":
                        if (value) {
                            if (negated) query.negatedFiles.push(value.toLowerCase());
                            else query.files.push(value.toLowerCase());
                        }
                        break;
                    case "path":
                        if (value) {
                            if (negated) query.negatedPaths.push(value.toLowerCase());
                            else query.paths.push(value.toLowerCase());
                        }
                        break;
                }
                continue;
            }
            // Unrecognized qualifier (e.g. "foo:bar") → treat as bare text (forgiving)
        }

        // Bare text term (positive or negated)
        const lower = workingToken.toLowerCase();
        if (lower) {
            if (negated) query.negatedTextTerms.push(lower);
            else query.textTerms.push(lower);
        }
    }

    // Consider empty if nothing meaningful was parsed (errors alone don't count)
    const hasContent =
        query.textTerms.length > 0 ||
        query.negatedTextTerms.length > 0 ||
        query.authors.length > 0 ||
        query.negatedAuthors.length > 0 ||
        query.hashes.length > 0 ||
        query.negatedHashes.length > 0 ||
        query.after !== null ||
        query.before !== null ||
        query.merge !== null ||
        query.branch !== null ||
        query.messages.length > 0 ||
        query.negatedMessages.length > 0 ||
        query.tags.length > 0 ||
        query.negatedTags.length > 0 ||
        query.files.length > 0 ||
        query.negatedFiles.length > 0 ||
        query.paths.length > 0 ||
        query.negatedPaths.length > 0;

    query.isEmpty = !hasContent;
    return query;
}

// ── BFS reachability cache ────────────────────────────────────────────────────

/**
 * Simple per-invocation cache for BFS reachability sets, keyed by branch tip
 * hash. Because createSearchMatcher is called on every search change (not per
 * node), rebuilding the cache on every call is acceptable — it is only computed
 * once per query, not once per node.
 *
 * We intentionally do NOT keep a module-level persistent cache because branch
 * tips change over time (new commits) and the matcher lifetime is shorter than
 * the data lifetime.
 */

/**
 * Performs a BFS from the given tip hash and returns the set of all reachable
 * commit hashes (inclusive of tip). O(n) in reachable commits.
 *
 * @param {string} tipHash
 * @param {Map<string, import("./graph/types.js").GraphCommit>} commits
 * @returns {Set<string>}
 */
function buildReachableSet(tipHash, commits) {
    const reachable = new Set();
    const queue = [tipHash];
    while (queue.length > 0) {
        const hash = queue.pop();
        if (!hash || reachable.has(hash)) continue;
        reachable.add(hash);
        const commit = commits.get(hash);
        if (!commit) continue;
        for (const parent of commit.parents ?? []) {
            if (!reachable.has(parent)) queue.push(parent);
        }
    }
    return reachable;
}

// ── createSearchMatcher ────────────────────────────────────────────────────────

/**
 * Compiles a SearchQuery into a predicate function that returns true when a
 * commit satisfies all active criteria.
 *
 * Returns null when the query is empty (no criteria), which lets the caller
 * skip the predicate loop entirely on the common case.
 *
 * Multiple values for the same qualifier are OR'd (e.g. two author: tokens
 * mean "match either author"). Different qualifiers are AND'd (e.g. author:
 * and after: both must pass). Negated qualifiers exclude matching commits.
 *
 * @param {SearchQuery} query Parsed query.
 * @param {Map<string, string>} [branches] Live branch map (name → hash). Required for branch: qualifier.
 * @param {Map<string, import("./graph/types.js").GraphCommit>} [commits] All known commits. Required for branch: qualifier.
 * @param {Map<string, string>} [tags] Tag map (name → commit hash). Required for tag: qualifier.
 * @param {Map<string, string[]>} [fileIndex] Commit hash → file paths touched. Required for file:/path: qualifiers.
 * @returns {((commit: import("./graph/types.js").GraphCommit) => boolean) | null}
 */
export function createSearchMatcher(query, branches, commits, tags, fileIndex) {
    if (query.isEmpty) return null;

    // Pre-compute the branch reachability set once (not per-commit).
    let reachableSet = null;
    if (query.branch !== null && branches && commits) {
        const branchName = query.branch;
        const tipHash =
            branches.get(branchName) ??
            branches.get("refs/heads/" + branchName) ??
            branches.get("refs/remotes/" + branchName) ??
            null;
        reachableSet = tipHash ? buildReachableSet(tipHash, commits) : new Set();
    }

    // Pre-compute tag → commit hash sets for tag: qualifier.
    let taggedCommits = null;
    let negatedTaggedCommits = null;
    if (query.tags.length > 0 || query.negatedTags.length > 0) {
        if (tags) {
            if (query.tags.length > 0) {
                taggedCommits = new Set();
                for (const [tagName, commitHash] of tags) {
                    const lowerTag = tagName.toLowerCase();
                    if (query.tags.some((t) => lowerTag.includes(t))) {
                        taggedCommits.add(commitHash);
                    }
                }
            }
            if (query.negatedTags.length > 0) {
                negatedTaggedCommits = new Set();
                for (const [tagName, commitHash] of tags) {
                    const lowerTag = tagName.toLowerCase();
                    if (query.negatedTags.some((t) => lowerTag.includes(t))) {
                        negatedTaggedCommits.add(commitHash);
                    }
                }
            }
        } else {
            // No tags data — positive tag: matches nothing; negated tag: excludes nothing
            if (query.tags.length > 0) taggedCommits = new Set();
        }
    }

    /**
     * Extracts author timestamp as a Unix epoch (ms). Handles both camelCase and
     * PascalCase field names sent by the Go backend.
     *
     * @param {import("./graph/types.js").GraphCommit} commit
     * @returns {number | null}
     */
    function getAuthorTimestamp(commit) {
        const when = commit.author?.when ?? commit.author?.When ?? null;
        if (!when) return null;
        const ms = new Date(when).getTime();
        return isNaN(ms) ? null : ms;
    }

    return function matchCommit(commit) {
        if (!commit) return false;

        // ── Text terms (implicit message/author/hash search) ──────────────────
        // All positive text terms must match at least one field (AND among terms).
        if (query.textTerms.length > 0) {
            const msg = (commit.message ?? "").toLowerCase();
            const authorName = (commit.author?.name ?? commit.author?.Name ?? "").toLowerCase();
            const authorEmail = (commit.author?.email ?? commit.author?.Email ?? "").toLowerCase();
            const hash = (commit.hash ?? "").toLowerCase();

            for (const term of query.textTerms) {
                const inMsg = msg.includes(term);
                const inName = authorName.includes(term);
                const inEmail = authorEmail.includes(term);
                const inHash = hash.startsWith(term);
                if (!inMsg && !inName && !inEmail && !inHash) return false;
            }
        }

        // Negated text terms: exclude if ANY negated term matches any field.
        if (query.negatedTextTerms.length > 0) {
            const msg = (commit.message ?? "").toLowerCase();
            const authorName = (commit.author?.name ?? commit.author?.Name ?? "").toLowerCase();
            const authorEmail = (commit.author?.email ?? commit.author?.Email ?? "").toLowerCase();
            const hash = (commit.hash ?? "").toLowerCase();

            for (const term of query.negatedTextTerms) {
                if (msg.includes(term) || authorName.includes(term) || authorEmail.includes(term) || hash.startsWith(term)) {
                    return false;
                }
            }
        }

        // ── author: (OR among values) ─────────────────────────────────────────
        if (query.authors.length > 0) {
            const authorName = (commit.author?.name ?? commit.author?.Name ?? "").toLowerCase();
            const authorEmail = (commit.author?.email ?? commit.author?.Email ?? "").toLowerCase();
            const matchesAny = query.authors.some(
                (a) => authorName.includes(a) || authorEmail.includes(a),
            );
            if (!matchesAny) return false;
        }

        // -author: any match → exclude
        if (query.negatedAuthors.length > 0) {
            const authorName = (commit.author?.name ?? commit.author?.Name ?? "").toLowerCase();
            const authorEmail = (commit.author?.email ?? commit.author?.Email ?? "").toLowerCase();
            const matchesAny = query.negatedAuthors.some(
                (a) => authorName.includes(a) || authorEmail.includes(a),
            );
            if (matchesAny) return false;
        }

        // ── hash: (OR among values) ───────────────────────────────────────────
        if (query.hashes.length > 0) {
            const hash = (commit.hash ?? "").toLowerCase();
            const matchesAny = query.hashes.some((h) => hash.startsWith(h));
            if (!matchesAny) return false;
        }

        // -hash: any match → exclude
        if (query.negatedHashes.length > 0) {
            const hash = (commit.hash ?? "").toLowerCase();
            const matchesAny = query.negatedHashes.some((h) => hash.startsWith(h));
            if (matchesAny) return false;
        }

        // ── after: (inclusive of the day boundary) ────────────────────────────
        if (query.after !== null) {
            const ts = getAuthorTimestamp(commit);
            if (ts === null || ts < query.after.getTime()) return false;
        }

        // ── before: (inclusive of the day boundary) ───────────────────────────
        if (query.before !== null) {
            const ts = getAuthorTimestamp(commit);
            if (ts === null || ts > query.before.getTime()) return false;
        }

        // ── message: (OR among values) ────────────────────────────────────────
        if (query.messages.length > 0) {
            const msg = (commit.message ?? "").toLowerCase();
            const matchesAny = query.messages.some((m) => msg.includes(m));
            if (!matchesAny) return false;
        }

        // -message: any match → exclude
        if (query.negatedMessages.length > 0) {
            const msg = (commit.message ?? "").toLowerCase();
            const matchesAny = query.negatedMessages.some((m) => msg.includes(m));
            if (matchesAny) return false;
        }

        // ── merge: only | exclude (with negation inversion) ───────────────────
        if (query.merge !== null) {
            const parentCount = commit.parents?.length ?? 0;
            const isMerge = parentCount > 1;
            let mergeMode = query.merge;
            // Negation inverts: -merge:only → exclude, -merge:exclude → only
            if (query.negateMerge) {
                mergeMode = mergeMode === "only" ? "exclude" : "only";
            }
            if (mergeMode === "only" && !isMerge) return false;
            if (mergeMode === "exclude" && isMerge) return false;
        }

        // ── tag: (commit must be pointed at by a matching tag) ────────────────
        if (taggedCommits !== null) {
            if (!taggedCommits.has(commit.hash)) return false;
        }

        // -tag: exclude commits pointed at by a matching tag
        if (negatedTaggedCommits !== null) {
            if (negatedTaggedCommits.has(commit.hash)) return false;
        }

        // ── file: (OR among values) — exact basename match ────────────────────
        if (query.files.length > 0) {
            const commitFiles = fileIndex?.get(commit.hash);
            if (!commitFiles) return false;
            const basenames = commitFiles.map((f) => {
                const slash = f.lastIndexOf("/");
                return (slash >= 0 ? f.slice(slash + 1) : f).toLowerCase();
            });
            const matchesAny = query.files.some((f) => basenames.some((b) => b === f));
            if (!matchesAny) return false;
        }

        // -file: any match → exclude
        if (query.negatedFiles.length > 0) {
            const commitFiles = fileIndex?.get(commit.hash);
            if (commitFiles) {
                const basenames = commitFiles.map((f) => {
                    const slash = f.lastIndexOf("/");
                    return (slash >= 0 ? f.slice(slash + 1) : f).toLowerCase();
                });
                const matchesAny = query.negatedFiles.some((f) => basenames.some((b) => b === f));
                if (matchesAny) return false;
            }
        }

        // ── path: (OR among values) — directory prefix match ──────────────────
        if (query.paths.length > 0) {
            const commitFiles = fileIndex?.get(commit.hash);
            if (!commitFiles) return false;
            const lowerPaths = commitFiles.map((f) => f.toLowerCase());
            const matchesAny = query.paths.some((p) =>
                lowerPaths.some((f) => f === p || f.startsWith(p + "/")),
            );
            if (!matchesAny) return false;
        }

        // -path: any match → exclude
        if (query.negatedPaths.length > 0) {
            const commitFiles = fileIndex?.get(commit.hash);
            if (commitFiles) {
                const lowerPaths = commitFiles.map((f) => f.toLowerCase());
                const matchesAny = query.negatedPaths.some((p) =>
                    lowerPaths.some((f) => f === p || f.startsWith(p + "/")),
                );
                if (matchesAny) return false;
            }
        }

        // ── branch: reachability (with negation inversion) ────────────────────
        if (reachableSet !== null) {
            if (query.negateBranch) {
                if (reachableSet.has(commit.hash)) return false;
            } else {
                if (!reachableSet.has(commit.hash)) return false;
            }
        }

        return true;
    };
}

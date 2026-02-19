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
 *
 * Bare (unqualified) tokens are treated as message/author/hash substrings.
 * Multiple values for the same qualifier are OR'd; different qualifiers AND.
 * Unrecognized qualifier prefixes are treated as bare text (forgiving parser).
 */

// ── Types ──────────────────────────────────────────────────────────────────────

/**
 * @typedef {Object} SearchQuery
 * @property {string} raw Original unmodified input string.
 * @property {string[]} textTerms Bare text terms (implicit message/author/hash search).
 * @property {string[]} authors Normalized author: qualifier values.
 * @property {string[]} hashes Normalized hash: qualifier values.
 * @property {Date | null} after Resolved lower date bound, or null.
 * @property {Date | null} before Resolved upper date bound, or null.
 * @property {'only' | 'exclude' | null} merge Merge commit filter mode.
 * @property {string | null} branch Branch name for reachability filtering.
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
const KNOWN_QUALIFIERS = new Set(["author", "hash", "after", "before", "merge", "branch"]);

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
        authors: [],
        hashes: [],
        after: null,
        before: null,
        merge: null,
        branch: null,
        isEmpty: false,
    };

    if (!trimmed) {
        query.isEmpty = true;
        return query;
    }

    const tokens = tokenize(trimmed);

    for (const token of tokens) {
        // Check for qualifier: prefix
        const colonIdx = token.indexOf(":");
        if (colonIdx > 0) {
            const qualifier = token.slice(0, colonIdx).toLowerCase();
            const value = token.slice(colonIdx + 1);

            if (KNOWN_QUALIFIERS.has(qualifier)) {
                switch (qualifier) {
                    case "author":
                        if (value) query.authors.push(value.toLowerCase());
                        break;
                    case "hash":
                        if (value) query.hashes.push(value.toLowerCase());
                        break;
                    case "after":
                        // Last after: wins if multiple are specified
                        query.after = parseDate(value) ?? query.after;
                        break;
                    case "before":
                        query.before = parseDate(value) ?? query.before;
                        break;
                    case "merge":
                        if (value === "only") query.merge = "only";
                        else if (value === "exclude") query.merge = "exclude";
                        else query.textTerms.push(token.toLowerCase()); // unrecognized value → bare text
                        break;
                    case "branch":
                        // Last branch: wins
                        if (value) query.branch = value;
                        break;
                }
                continue;
            }
            // Unrecognized qualifier (e.g. "foo:bar") → treat as bare text (forgiving)
        }

        // Bare text term
        const lower = token.toLowerCase();
        if (lower) query.textTerms.push(lower);
    }

    // Consider empty if nothing meaningful was parsed
    const hasContent =
        query.textTerms.length > 0 ||
        query.authors.length > 0 ||
        query.hashes.length > 0 ||
        query.after !== null ||
        query.before !== null ||
        query.merge !== null ||
        query.branch !== null;

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
 * and after: both must pass).
 *
 * @param {SearchQuery} query Parsed query.
 * @param {Map<string, string>} [branches] Live branch map (name → hash). Required for branch: qualifier.
 * @param {Map<string, import("./graph/types.js").GraphCommit>} [commits] All known commits. Required for branch: qualifier.
 * @returns {((commit: import("./graph/types.js").GraphCommit) => boolean) | null}
 */
export function createSearchMatcher(query, branches, commits) {
    if (query.isEmpty) return null;

    // Pre-compute the branch reachability set once (not per-commit).
    let reachableSet = null;
    if (query.branch !== null && branches && commits) {
        // Resolve the branch name to a tip hash. Attempt both bare name and
        // refs/heads/<name> so users can write "branch:main" without the prefix.
        const branchName = query.branch;
        const tipHash =
            branches.get(branchName) ??
            branches.get("refs/heads/" + branchName) ??
            branches.get("refs/remotes/" + branchName) ??
            null;
        reachableSet = tipHash ? buildReachableSet(tipHash, commits) : new Set();
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
        // All text terms must match at least one of the commit fields (AND among terms).
        if (query.textTerms.length > 0) {
            const msg = (commit.message ?? "").toLowerCase();
            const authorName = (commit.author?.name ?? commit.author?.Name ?? "").toLowerCase();
            const authorEmail = (commit.author?.email ?? commit.author?.Email ?? "").toLowerCase();
            const hash = (commit.hash ?? "").toLowerCase();

            for (const term of query.textTerms) {
                const inMsg = msg.includes(term);
                const inName = authorName.includes(term);
                const inEmail = authorEmail.includes(term);
                // Hash prefix match: only check start of hash (canonical Git behavior)
                const inHash = hash.startsWith(term);
                if (!inMsg && !inName && !inEmail && !inHash) return false;
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

        // ── hash: (OR among values) ───────────────────────────────────────────
        if (query.hashes.length > 0) {
            const hash = (commit.hash ?? "").toLowerCase();
            const matchesAny = query.hashes.some((h) => hash.startsWith(h));
            if (!matchesAny) return false;
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

        // ── merge: only | exclude ─────────────────────────────────────────────
        if (query.merge !== null) {
            const parentCount = commit.parents?.length ?? 0;
            const isMerge = parentCount > 1;
            if (query.merge === "only" && !isMerge) return false;
            if (query.merge === "exclude" && isMerge) return false;
        }

        // ── branch: reachability ──────────────────────────────────────────────
        if (reachableSet !== null) {
            if (!reachableSet.has(commit.hash)) return false;
        }

        return true;
    };
}

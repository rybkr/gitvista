/**
 * @fileoverview Pure graph filter predicate builder used by the graph controller.
 */

/**
 * Returns true when the given commit node is tracked only by remote-tracking
 * branches (i.e. every branch pointing at it starts with "refs/remotes/").
 *
 * @param {import("./types.js").GraphNode} node
 * @param {Map<string, string>} branches
 * @returns {boolean}
 */
function isExclusivelyRemote(node, branches) {
    if (node.type !== "commit") return false;
    let hasAnyBranch = false;
    let hasLocalBranch = false;
    for (const [name, hash] of branches.entries()) {
        if (hash !== node.hash) continue;
        hasAnyBranch = true;
        if (!name.startsWith("refs/remotes/")) {
            hasLocalBranch = true;
            break;
        }
    }
    return hasAnyBranch && !hasLocalBranch;
}

/**
 * Returns true when the given commit node corresponds to a stash entry.
 *
 * @param {import("./types.js").GraphNode} node
 * @param {Map<string, string>} branches
 * @param {Array<{hash: string}>} stashes
 * @returns {boolean}
 */
function isStashCommit(node, branches, stashes) {
    if (node.type !== "commit") return false;
    if (node.isStash || node.isStashInternal) return true;
    const hash = node.hash;
    if (Array.isArray(stashes)) {
        for (const stash of stashes) {
            if (stash?.hash === hash) return true;
        }
    }
    for (const [name, targetHash] of branches.entries()) {
        if (targetHash !== hash) continue;
        if (name === "refs/stash" || name.startsWith("stash@{")) return true;
    }
    return false;
}

/**
 * Performs a BFS from the tip commit of a focused branch.
 *
 * @param {string} branchTipHash
 * @param {Map<string, import("./types.js").GraphCommit>} commits
 * @returns {Set<string>}
 */
function getReachableCommits(branchTipHash, commits) {
    const reachable = new Set();
    const queue = [branchTipHash];
    while (queue.length > 0) {
        const hash = queue.pop();
        if (!hash || reachable.has(hash)) continue;
        reachable.add(hash);
        const commit = commits.get(hash);
        if (!commit) continue;
        for (const parent of commit.parents ?? []) {
            if (!reachable.has(parent)) {
                queue.push(parent);
            }
        }
    }
    return reachable;
}

/**
 * Builds the compound node predicate for graph dimming/filtering.
 *
 * @param {{ query: import("./types.js").SearchQuery, matcher: ((commit: import("./types.js").GraphCommit) => boolean) | null } | null} searchState
 * @param {{ hideRemotes: boolean, hideMerges: boolean, hideStashes: boolean, focusBranch: string }} filterState
 * @param {Map<string, string>} branches
 * @param {Map<string, import("./types.js").GraphCommit>} commits
 * @param {Array<{hash: string}>} stashes
 * @param {string|null} headHash
 * @param {number|null} isolatedLanePosition
 * @param {Array<Object>} segments
 * @param {object} scopeSettings
 * @returns {((node: import("./types.js").GraphNode) => boolean) | null}
 */
export function buildFilterPredicate(
    searchState,
    filterState,
    branches,
    commits,
    stashes,
    headHash,
    isolatedLanePosition,
    segments,
    scopeSettings,
) {
    const matcher = searchState?.matcher ?? null;
    const hasSearch = matcher !== null;
    const { hideRemotes, hideMerges, hideStashes, focusBranch } = filterState;
    const hasIsolation = isolatedLanePosition !== null && isolatedLanePosition !== undefined;

    const scope = scopeSettings || {};
    const hasDepthLimit = typeof scope.depthLimit === "number" && scope.depthLimit !== Infinity;
    const hasTimeWindow = scope.timeWindow && scope.timeWindow !== "all";
    const branchRules = scope.branchRules && typeof scope.branchRules === "object"
        ? scope.branchRules
        : {};
    const hasBranchRules = Object.values(branchRules).some((value) => value === false);
    const hasScope = hasDepthLimit || hasTimeWindow || hasBranchRules;
    const hasAnyFilter = hideRemotes || hideMerges || hideStashes || !!focusBranch || hasIsolation || hasScope;

    if (!hasSearch && !hasAnyFilter) return null;

    let isolatedHashes = null;
    if (hasIsolation && Array.isArray(segments)) {
        isolatedHashes = new Set();
        for (const seg of segments) {
            if (seg.position === isolatedLanePosition) {
                for (const h of seg.hashes) isolatedHashes.add(h);
            }
        }
    }

    let reachableSet = null;
    if (focusBranch) {
        const tipHash = branches.get(focusBranch);
        reachableSet = tipHash ? getReachableCommits(tipHash, commits) : new Set();
    }

    let branchScopedHashes = null;
    if (hasBranchRules) {
        branchScopedHashes = new Set();
        const queue = [];
        for (const [name, hash] of branches.entries()) {
            if (!hash) continue;
            if (branchRules[name] === false) continue;
            if (
                name !== "HEAD" &&
                !name.startsWith("refs/heads/") &&
                !name.startsWith("refs/remotes/")
            ) {
                continue;
            }
            queue.push(hash);
        }
        while (queue.length > 0) {
            const hash = queue.pop();
            if (!hash || branchScopedHashes.has(hash)) continue;
            branchScopedHashes.add(hash);
            const commit = commits.get(hash);
            if (!commit) continue;
            for (const parent of commit.parents ?? []) {
                if (!branchScopedHashes.has(parent)) {
                    queue.push(parent);
                }
            }
        }
    }

    let depthMap = null;
    if (hasDepthLimit) {
        depthMap = new Map();
        let depthStartHash = headHash || "";
        if (!depthStartHash) {
            for (const [name, hash] of branches.entries()) {
                if (name === "HEAD" || name === "refs/heads/main" || name === "refs/heads/master") {
                    depthStartHash = hash;
                    if (name === "HEAD") break;
                }
            }
        }
        if (depthStartHash) {
            const queue = [{ hash: depthStartHash, depth: 0 }];
            while (queue.length > 0) {
                const { hash, depth } = queue.shift();
                if (!hash || depthMap.has(hash)) continue;
                depthMap.set(hash, depth);
                const commit = commits.get(hash);
                if (!commit) continue;
                for (const parent of commit.parents ?? []) {
                    if (!depthMap.has(parent)) {
                        queue.push({ hash: parent, depth: depth + 1 });
                    }
                }
            }
        }
    }

    let timeCutoff = 0;
    if (hasTimeWindow) {
        const now = Date.now();
        const windows = { "7d": 7, "30d": 30, "90d": 90, "1y": 365 };
        const days = windows[scope.timeWindow];
        if (days) timeCutoff = now - days * 86400000;
    }

    return (node) => {
        if (node.type === "branch") {
            if (hideRemotes && node.branch?.startsWith("refs/remotes/")) return false;
            if (branchRules[node.branch] === false) return false;
            return true;
        }

        if (node.type === "tag") {
            return true;
        }

        if (hasSearch) {
            const commit = node.commit;
            if (!commit) return false;
            if (!matcher(commit)) return false;
        }

        if (hideRemotes && isExclusivelyRemote(node, branches)) return false;
        if (hideMerges && (node.commit?.parents?.length ?? 0) > 1) return false;
        if (hideStashes && isStashCommit(node, branches, stashes)) return false;
        if (reachableSet !== null && !reachableSet.has(node.hash)) return false;
        if (branchScopedHashes !== null && !branchScopedHashes.has(node.hash)) return false;
        if (isolatedHashes !== null && !isolatedHashes.has(node.hash)) return false;

        if (depthMap !== null && node.type === "commit") {
            const depth = depthMap.get(node.hash);
            if (depth === undefined || depth > scope.depthLimit) return false;
        }

        if (timeCutoff > 0 && node.type === "commit") {
            const ts =
                Date.parse(
                    node.commit?.committer?.when ??
                    node.commit?.author?.when ??
                    node.commit?.committer?.When ??
                    node.commit?.author?.When ??
                    0,
                ) || 0;
            if (ts < timeCutoff) return false;
        }

        return true;
    };
}

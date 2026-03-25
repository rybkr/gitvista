const COMMIT_HASH_RE = /^[0-9a-f]{40}$/i;
export function parseHashFragment(hash) {
    const fragment = typeof hash === "string" ? hash.replace(/^#/, "") : "";
    return { commitHash: COMMIT_HASH_RE.test(fragment) ? fragment : null };
}

export function parseLaunchTarget(search, hash) {
    const params = new URLSearchParams(typeof search === "string" ? search : "");
    const path = params.get("path") || null;
    const commitHash = parseHashFragment(hash).commitHash;
    return { path, commitHash };
}

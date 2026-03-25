const COMMIT_HASH_RE = /^[0-9a-f]{40}$/i;
export function parseLocalHash(hash) {
    const fragment = typeof hash === "string" ? hash.replace(/^#/, "") : "";
    if (!fragment) return { page: "repo", repoId: null, commitHash: null, docsSection: null };
    if (COMMIT_HASH_RE.test(fragment)) return { page: "repo", repoId: null, commitHash: fragment, docsSection: null };
    return { page: "repo", repoId: null, commitHash: null, docsSection: null };
}

export function parseLocalLaunchTarget(search, hash) {
    const params = new URLSearchParams(typeof search === "string" ? search : "");
    const path = params.get("path") || null;
    const commitHash = parseLocalHash(hash)?.commitHash || null;
    return { path, commitHash };
}

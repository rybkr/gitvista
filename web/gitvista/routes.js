const COMMIT_HASH_RE = /^[0-9a-f]{40}$/i;
const REPO_PATH_RE = /^\/repo\/([^/]+)(?:\/([0-9a-f]{40}))?\/?$/i;
const REPO_LOADING_PATH_RE = /^\/repo\/([^/]+)\/loading\/?$/i;
const DOCS_PATH_RE = /^\/docs\/([^/]+)\/?$/i;

export function parseHostedPath(pathname) {
    const normalized = normalizePathname(pathname);
    if (normalized === "/") return { page: "landing", repoId: null, commitHash: null, docsSection: null };
    if (normalized === "/docs") return { page: "docs", repoId: null, commitHash: null, docsSection: null };

    const docsMatch = DOCS_PATH_RE.exec(normalized);
    if (docsMatch) {
        return { page: "docs", repoId: null, commitHash: null, docsSection: docsMatch[1] || null };
    }

    const loadingMatch = REPO_LOADING_PATH_RE.exec(normalized);
    if (loadingMatch) {
        return { page: "repo-loading", repoId: loadingMatch[1], commitHash: null, docsSection: null };
    }

    const match = REPO_PATH_RE.exec(normalized);
    if (match) {
        return { page: "repo", repoId: match[1], commitHash: match[2] || null, docsSection: null };
    }

    return { page: "landing", repoId: null, commitHash: null, docsSection: null };
}

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

function normalizePathname(pathname) {
    if (typeof pathname !== "string" || pathname.trim() === "") return "/";
    const trimmed = pathname.trim();
    if (trimmed === "/") return "/";
    return trimmed.endsWith("/") ? trimmed.slice(0, -1) : trimmed;
}

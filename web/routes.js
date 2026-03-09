const COMMIT_HASH_RE = /^[0-9a-f]{40}$/i;
const REPO_PATH_RE = /^\/repo\/([^/]+)(?:\/([0-9a-f]{40}))?\/?$/i;

export function parseHostedPath(pathname) {
    const normalized = normalizePathname(pathname);
    if (normalized === "/") return { page: "landing", repoId: null, commitHash: null };
    if (normalized === "/docs") return { page: "docs", repoId: null, commitHash: null };

    const match = REPO_PATH_RE.exec(normalized);
    if (match) {
        return { page: "repo", repoId: match[1], commitHash: match[2] || null };
    }

    return { page: "landing", repoId: null, commitHash: null };
}

export function parseLocalHash(hash) {
    const fragment = typeof hash === "string" ? hash.replace(/^#/, "") : "";
    if (!fragment) return { page: "repo", repoId: null, commitHash: null };
    if (COMMIT_HASH_RE.test(fragment)) return { page: "repo", repoId: null, commitHash: fragment };
    return { page: "repo", repoId: null, commitHash: null };
}

function normalizePathname(pathname) {
    if (typeof pathname !== "string" || pathname.trim() === "") return "/";
    const trimmed = pathname.trim();
    if (trimmed === "/") return "/";
    return trimmed.endsWith("/") ? trimmed.slice(0, -1) : trimmed;
}

const COMMIT_HASH_RE = /^[0-9a-f]{40}$/i;
const ACCOUNT_REPO_PATH_RE = /^\/a\/([^/]+)\/r\/([^/]+)(?:\/([0-9a-f]{40}))?\/?$/i;
const ACCOUNT_REPO_LOADING_PATH_RE = /^\/a\/([^/]+)\/r\/([^/]+)\/loading\/?$/i;
const REPO_PATH_RE = /^\/repo\/([^/]+)(?:\/([0-9a-f]{40}))?\/?$/i;
const REPO_LOADING_PATH_RE = /^\/repo\/([^/]+)\/loading\/?$/i;
const DOCS_PATH_RE = /^\/docs\/([^/]+)\/?$/i;
export const DEFAULT_HOSTED_ACCOUNT_SLUG = "personal";

export function parseHostedPath(pathname) {
    const normalized = normalizePathname(pathname);
    if (normalized === "/") return { page: "landing", accountSlug: DEFAULT_HOSTED_ACCOUNT_SLUG, repoId: null, commitHash: null, docsSection: null };
    if (normalized === "/docs") return { page: "docs", accountSlug: DEFAULT_HOSTED_ACCOUNT_SLUG, repoId: null, commitHash: null, docsSection: null };

    const docsMatch = DOCS_PATH_RE.exec(normalized);
    if (docsMatch) {
        return { page: "docs", accountSlug: DEFAULT_HOSTED_ACCOUNT_SLUG, repoId: null, commitHash: null, docsSection: docsMatch[1] || null };
    }

    const accountLoadingMatch = ACCOUNT_REPO_LOADING_PATH_RE.exec(normalized);
    if (accountLoadingMatch) {
        return { page: "repo-loading", accountSlug: accountLoadingMatch[1], repoId: accountLoadingMatch[2], commitHash: null, docsSection: null };
    }

    const accountMatch = ACCOUNT_REPO_PATH_RE.exec(normalized);
    if (accountMatch) {
        return { page: "repo", accountSlug: accountMatch[1], repoId: accountMatch[2], commitHash: accountMatch[3] || null, docsSection: null };
    }

    const loadingMatch = REPO_LOADING_PATH_RE.exec(normalized);
    if (loadingMatch) {
        return { page: "repo-loading", accountSlug: DEFAULT_HOSTED_ACCOUNT_SLUG, repoId: loadingMatch[1], commitHash: null, docsSection: null };
    }

    const match = REPO_PATH_RE.exec(normalized);
    if (match) {
        return { page: "repo", accountSlug: DEFAULT_HOSTED_ACCOUNT_SLUG, repoId: match[1], commitHash: match[2] || null, docsSection: null };
    }

    return { page: "landing", accountSlug: DEFAULT_HOSTED_ACCOUNT_SLUG, repoId: null, commitHash: null, docsSection: null };
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

export function buildHostedRepoPath(accountSlug, repoId, commitHash = null) {
    const base = `/a/${accountSlug || DEFAULT_HOSTED_ACCOUNT_SLUG}/r/${repoId}`;
    return commitHash ? `${base}/${commitHash}` : base;
}

export function buildHostedRepoLoadingPath(accountSlug, repoId) {
    return `${buildHostedRepoPath(accountSlug, repoId)}/loading`;
}

export function buildHostedRepoApiBase(accountSlug, repoId) {
    return `/api/accounts/${accountSlug || DEFAULT_HOSTED_ACCOUNT_SLUG}/repos/${repoId}`;
}

export function buildHostedReposApiPath(accountSlug) {
    return `/api/accounts/${accountSlug || DEFAULT_HOSTED_ACCOUNT_SLUG}/repos`;
}

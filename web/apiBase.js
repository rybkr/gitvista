let base = "/api"; // local mode default
let repoToken = "";

export function getRepoToken() {
    return repoToken;
}

export function apiUrl(path) {
    return `${base}${path}`;
}

export function wsUrl() {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    const url = new URL(`${protocol}://${location.host}${base}/ws`);
    if (repoToken) {
        url.searchParams.set("access_token", repoToken);
    }
    return url.toString();
}

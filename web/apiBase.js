export function apiUrl(path) {
    return `/api${path}`;
}

export function wsUrl() {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    return new URL(`${protocol}://${location.host}/api/ws`).toString();
}

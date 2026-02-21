let base = "/api"; // local mode default

export function setApiBase(newBase) {
    base = newBase;
}

export function getApiBase() {
    return base;
}

export function apiUrl(path) {
    return `${base}${path}`;
}

export function wsUrl() {
    const protocol = location.protocol === "https:" ? "wss" : "ws";
    return `${protocol}://${location.host}${base}/ws`;
}

const STORAGE_KEY = "gitvista-hosted-access";

function readAll() {
    try {
        const raw = sessionStorage.getItem(STORAGE_KEY);
        if (!raw) return {};
        const parsed = JSON.parse(raw);
        return parsed && typeof parsed === "object" ? parsed : {};
    } catch {
        return {};
    }
}

function writeAll(value) {
    try {
        sessionStorage.setItem(STORAGE_KEY, JSON.stringify(value));
    } catch {
        // Ignore storage failures and continue with in-memory navigation only.
    }
}

export function saveHostedRepoAccess({ id, url, accessToken }) {
    if (!id || !accessToken) return;
    const current = readAll();
    current[id] = {
        id,
        url: typeof url === "string" ? url : "",
        accessToken,
        savedAt: Date.now(),
    };
    writeAll(current);
}

export function getHostedRepoAccess(id) {
    if (!id) return null;
    const current = readAll();
    const entry = current[id];
    if (!entry || typeof entry.accessToken !== "string" || entry.accessToken === "") {
        return null;
    }
    return entry;
}

export function listHostedRepoAccess() {
    return Object.values(readAll())
        .filter((entry) => entry && entry.id && entry.accessToken)
        .sort((a, b) => Number(b.savedAt || 0) - Number(a.savedAt || 0));
}

export function removeHostedRepoAccess(id) {
    if (!id) return;
    const current = readAll();
    delete current[id];
    writeAll(current);
}

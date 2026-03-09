function isPrimaryPlainClick(event) {
    return !(
        event.defaultPrevented ||
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey
    );
}

export function bindHostedPathNavigation(link, path, navigateToPath) {
    link.href = path;
    if (typeof navigateToPath !== "function" || !path.startsWith("/")) return;

    link.addEventListener("click", (event) => {
        if (!isPrimaryPlainClick(event)) return;
        event.preventDefault();
        navigateToPath(path);
    });
}

export function bindHashScroll(link, hash, { root = document, updateHistory = true } = {}) {
    link.href = hash;
    link.addEventListener("click", (event) => {
        if (!isPrimaryPlainClick(event)) return;
        event.preventDefault();
        scrollToHashTarget(root, hash, { updateHistory });
    });
}

export function scrollToHashTarget(root, hash, { updateHistory = true } = {}) {
    if (typeof hash !== "string" || !hash.startsWith("#")) return false;
    const target = root.querySelector(hash);
    if (!target) return false;

    target.scrollIntoView({ behavior: "smooth", block: "start" });
    if (updateHistory) {
        history.replaceState(null, "", hash);
    }
    return true;
}


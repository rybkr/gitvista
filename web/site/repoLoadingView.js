import { buildHostedRepoApiBase, buildHostedRepoLoadingPath, buildHostedRepoPath } from "../gitvista/routes.js";
import { createHostedFooter, createHostedTopbar } from "./hostedChrome.js";
import { getHostedRepoAccess } from "./hostedAccess.js";

function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function describePhase(phase) {
    if (!phase) return "Preparing repository";
    return phase
        .replace(/[_-]+/g, " ")
        .replace(/\b\w/g, (match) => match.toUpperCase());
}

async function fetchRepoStatus(accountSlug, repoId, signal) {
    const access = getHostedRepoAccess(repoId);
    if (!access?.accessToken) {
        throw new Error("This browser no longer has access to that repository.");
    }
    const resp = await fetch(`${buildHostedRepoApiBase(accountSlug, repoId)}/status`, {
        signal,
        headers: { "X-GitVista-Repo-Token": access.accessToken },
    });
    if (!resp.ok) {
        let message = `Repository status request failed (${resp.status})`;
        try {
            const text = (await resp.text()).trim();
            if (text) message = text;
        } catch {
            // Ignore body parse failures.
        }
        throw new Error(message);
    }
    return resp.json();
}

export function createRepoLoadingView({ accountSlug, repoId, navigateToPath, replacePath, onRouteChange, initialStatus = null } = {}) {
    const controller = new AbortController();
    let progressStream = null;
    let reconnectTimer = null;
    let redirected = false;

    const el = createElement("div", "repo-loading");
    const chrome = createElement("div", "repo-loading__chrome");
    chrome.appendChild(createHostedTopbar({
        activePath: "/",
        navigateToPath,
        navItems: [
            { label: "Home", path: "/" },
            { label: "Docs", path: "/docs" },
        ],
    }));

    const content = createElement("div", "repo-loading__content");
    const hero = createElement("section", "repo-loading__hero");
    const heroCopy = createElement("div", "repo-loading__hero-copy");
    heroCopy.appendChild(createElement("p", "repo-loading__eyebrow", "Opening Repository"));
    heroCopy.appendChild(createElement("h1", "repo-loading__title", "Getting this public repo ready."));
    heroCopy.appendChild(createElement("p", "repo-loading__lede", "GitVista is cloning the repository and preparing the graph view. You will move into the full tool automatically as soon as it is ready."));

    const heroMeta = createElement("div", "repo-loading__hero-meta");
    const repoLabel = createElement("span", "repo-loading__repo-id", initialStatus?.displayName || repoId);
    const stateBadge = createElement("span", "repo-loading__badge");
    heroMeta.appendChild(repoLabel);
    heroMeta.appendChild(stateBadge);
    heroCopy.appendChild(heroMeta);

    const panel = createElement("aside", "repo-loading__panel");
    panel.appendChild(createElement("div", "repo-loading__panel-kicker", "Progress"));
    const phaseLabel = createElement("p", "repo-loading__phase", "Preparing repository");
    phaseLabel.setAttribute("role", "status");
    phaseLabel.setAttribute("aria-live", "polite");
    panel.appendChild(phaseLabel);

    const percentLabel = createElement("p", "repo-loading__percent", "0%");
    panel.appendChild(percentLabel);

    const progressBar = createElement("div", "repo-loading__progress-bar");
    const progressFill = createElement("div", "repo-loading__progress-fill");
    progressBar.appendChild(progressFill);
    panel.appendChild(progressBar);

    const detail = createElement("p", "repo-loading__detail", "Connecting to the repo manager.");
    panel.appendChild(detail);

    const actions = createElement("div", "repo-loading__actions");
    const homeLink = createElement("a", "repo-loading__action", "Back Home");
    homeLink.href = "/";
    homeLink.addEventListener("click", (event) => {
        event.preventDefault();
        navigateToPath?.("/");
    });
    actions.appendChild(homeLink);

    const retryButton = createElement("button", "repo-loading__action repo-loading__action--secondary", "Retry Status Check");
    retryButton.type = "button";
    retryButton.hidden = true;
    retryButton.addEventListener("click", () => {
        retryButton.hidden = true;
        detail.textContent = "Checking repository status again.";
        void refreshStatus();
    });
    actions.appendChild(retryButton);
    panel.appendChild(actions);

    hero.appendChild(heroCopy);
    hero.appendChild(panel);
    content.appendChild(hero);
    content.appendChild(createHostedFooter());
    el.appendChild(chrome);
    el.appendChild(content);

    function clearReconnectTimer() {
        if (reconnectTimer !== null) {
            window.clearTimeout(reconnectTimer);
            reconnectTimer = null;
        }
    }

    function closeProgressStream() {
        if (!progressStream) return;
        progressStream.close();
        progressStream = null;
    }

    function redirectToRepo() {
        if (redirected) return;
        redirected = true;
        closeProgressStream();
        clearReconnectTimer();
        replacePath?.(buildHostedRepoPath(accountSlug, repoId));
        onRouteChange?.();
    }

    function renderStatus(status) {
        const state = status?.state || "pending";
        const percent = Math.max(0, Math.min(100, Number(status?.percent) || 0));
        repoLabel.textContent = status?.displayName || repoId;
        stateBadge.className = `repo-loading__badge repo-loading__badge--${state}`;
        stateBadge.textContent = state;
        phaseLabel.textContent = describePhase(status?.phase);
        percentLabel.textContent = state === "ready" ? "100%" : `${percent}%`;
        progressFill.style.width = `${state === "ready" ? 100 : percent}%`;
        detail.className = "repo-loading__detail";
        retryButton.hidden = true;

        if (state === "ready") {
            detail.textContent = "Repository ready. Launching the graph view.";
            return;
        }

        if (state === "error") {
            detail.className = "repo-loading__detail repo-loading__detail--error";
            detail.textContent = status?.error || "The repository could not be prepared.";
            retryButton.hidden = false;
            return;
        }

        detail.textContent = percent > 0
            ? "Downloading objects and preparing the commit graph."
            : "Waiting for the clone job to report progress.";
    }

    function scheduleReconnect() {
        clearReconnectTimer();
        reconnectTimer = window.setTimeout(() => {
            void refreshStatus();
        }, 1200);
    }

    function startProgressStream() {
        closeProgressStream();
        const access = getHostedRepoAccess(repoId);
        if (!access?.accessToken) {
            renderStatus({
                state: "error",
                error: "This browser no longer has access to that repository.",
                phase: "",
                percent: 0,
            });
            return;
        }
        const url = new URL(`${buildHostedRepoApiBase(accountSlug, repoId)}/progress`, window.location.origin);
        url.searchParams.set("access_token", access.accessToken);
        progressStream = new EventSource(url.toString());

        progressStream.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                if (data.done) {
                    const terminalStatus = {
                        state: data.state || "ready",
                        error: data.error || "",
                        phase: "",
                        percent: data.state === "ready" ? 100 : 0,
                    };
                    renderStatus(terminalStatus);
                    closeProgressStream();
                    if (terminalStatus.state === "ready") {
                        redirectToRepo();
                    }
                    return;
                }

                renderStatus({
                    state: "cloning",
                    phase: data.phase || "",
                    percent: data.percent || 0,
                    error: "",
                });
            } catch {
                // Ignore malformed events.
            }
        };

        progressStream.onerror = () => {
            closeProgressStream();
            scheduleReconnect();
        };
    }

    async function refreshStatus() {
        closeProgressStream();
        clearReconnectTimer();

        try {
            const status = await fetchRepoStatus(accountSlug, repoId, controller.signal);
            if (controller.signal.aborted) return;

            renderStatus(status);
            if (status.state === "ready") {
                redirectToRepo();
                return;
            }
            if (status.state === "error") return;

            if (location.pathname !== buildHostedRepoLoadingPath(accountSlug, repoId)) {
                replacePath?.(buildHostedRepoLoadingPath(accountSlug, repoId));
            }
            startProgressStream();
        } catch (error) {
            if (controller.signal.aborted) return;
            renderStatus({
                state: "error",
                error: error.message || "Unable to load repository status.",
                phase: "",
                percent: 0,
            });
        }
    }

    renderStatus(initialStatus || { state: "pending", phase: "", percent: 0, error: "" });
    if (initialStatus?.state === "ready") {
        redirectToRepo();
    } else if (initialStatus?.state === "error") {
        retryButton.hidden = false;
    } else {
        void refreshStatus();
    }

    return {
        el,
        destroy() {
            controller.abort();
            closeProgressStream();
            clearReconnectTimer();
        },
    };
}

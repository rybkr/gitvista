/**
 * Repository unavailable overlay.
 *
 * Full-screen semi-transparent overlay shown when the backend cannot serve
 * repository data. Subscribes to errorState.repositoryAvailable.
 *
 * The overlay does NOT destroy the underlying DOM — the graph and sidebar
 * remain intact so recovery is instant.
 */

import { subscribe, getState, setRepositoryAvailable } from "./errorState.js";
import { apiUrl } from "./apiBase.js";

export function createRepoUnavailableOverlay({ repoId } = {}) {
    const el = document.createElement("div");
    el.className = "repo-unavailable-overlay";
    el.setAttribute("role", "alertdialog");
    el.setAttribute("aria-modal", "true");
    el.setAttribute("aria-label", "Repository unavailable");
    el.style.display = "none";

    const card = document.createElement("div");
    card.className = "repo-unavailable-card";

    const icon = document.createElement("div");
    icon.className = "repo-unavailable-icon";
    icon.textContent = "\u26A0";
    card.appendChild(icon);

    const title = document.createElement("h2");
    title.className = "repo-unavailable-title";
    title.textContent = "Repository Unavailable";
    el.setAttribute("aria-labelledby", "repo-unavail-title");
    title.id = "repo-unavail-title";
    card.appendChild(title);

    const desc = document.createElement("p");
    desc.className = "repo-unavailable-desc";
    desc.textContent =
        "The repository could not be loaded. This may happen if the repo path is invalid or the server is restarting.";
    card.appendChild(desc);

    const retryBtn = document.createElement("button");
    retryBtn.className = "repo-unavailable-retry";
    retryBtn.textContent = "Retry Connection";
    card.appendChild(retryBtn);

    if (repoId) {
        const backLink = document.createElement("a");
        backLink.className = "repo-unavailable-back";
        backLink.href = "#";
        backLink.textContent = "Back to repositories";
        backLink.addEventListener("click", (e) => {
            e.preventDefault();
            location.hash = "";
        });
        card.appendChild(backLink);
    }

    el.appendChild(card);

    retryBtn.addEventListener("click", async () => {
        retryBtn.disabled = true;
        retryBtn.textContent = "Retrying\u2026";
        try {
            const resp = await fetch(apiUrl("/repository"));
            if (resp.ok) {
                setRepositoryAvailable(true);
            }
        } catch {
            // Stay on overlay
        }
        retryBtn.disabled = false;
        retryBtn.textContent = "Retry Connection";
    });

    subscribe((state) => {
        if (state.repositoryAvailable) {
            el.style.display = "none";
        } else {
            el.style.display = "flex";
            retryBtn.focus();
        }
    });

    // Initial check
    if (!getState().repositoryAvailable) {
        el.style.display = "flex";
    }

    return { el };
}

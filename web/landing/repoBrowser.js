const DELETE_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const ARROW_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M3 8h10M9 4l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function normalizeRepoUrl(url) {
    return typeof url === "string" ? url.replace(/\/+$/, "") : "";
}

export function createRepoBrowser({ featuredRepos, onRepoSelect }) {
    let repos = [];
    let destroyed = false;
    const activeStreams = new Map();
    const featuredState = new Map();

    const heroRecentListEl = createElement("div", "repo-landing__list repo-landing__list--hero");
    const featuredSectionEl = createElement("section", "repo-landing__section repo-landing__featured");
    featuredSectionEl.id = "featured";

    featuredSectionEl.appendChild(createElement("p", "repo-landing__eyebrow", "Fast-start examples"));
    featuredSectionEl.appendChild(createElement("h2", "repo-landing__section-title", "Open a live repository and inspect how the history is shaped."));
    featuredSectionEl.appendChild(createElement("p", "repo-landing__section-subtitle", "These repos are preloaded to make the first interaction immediate. Use them to see branch movement, merges, and commit-level diffs before pasting your own URL."));

    const featuredGrid = createElement("div", "repo-landing__featured-grid");
    featuredSectionEl.appendChild(featuredGrid);

    function destroyStream(id) {
        if (!activeStreams.has(id)) return;
        activeStreams.get(id).close();
        activeStreams.delete(id);
    }

    function startProgressStream(id, onUpdate) {
        if (activeStreams.has(id)) return;

        const es = new EventSource(`/api/repos/${id}/progress`);
        activeStreams.set(id, es);

        es.onmessage = (event) => {
            if (destroyed) {
                destroyStream(id);
                return;
            }

            try {
                const data = JSON.parse(event.data);
                if (data.done) {
                    onUpdate?.({ id, state: data.state, error: data.error || "", phase: "", percent: 0 });
                    destroyStream(id);
                    return;
                }

                onUpdate?.({ id, state: "cloning", error: "", phase: data.phase || "", percent: data.percent || 0 });
            } catch {
                // Ignore malformed events.
            }
        };

        es.onerror = () => {
            destroyStream(id);
            fetch(`/api/repos/${id}/status`)
                .then((resp) => resp.ok ? resp.json() : null)
                .then((data) => {
                    if (destroyed || !data) return;
                    onUpdate?.({
                        id: data.id || id,
                        state: data.state,
                        error: data.error || "",
                        phase: data.phase || "",
                        percent: data.percent || 0,
                    });
                })
                .catch(() => {});
        };
    }

    function addOrUpdateRepo(repo) {
        const idx = repos.findIndex((entry) => entry.id === repo.id);
        if (idx >= 0) {
            repos[idx] = { ...repos[idx], ...repo };
            return;
        }
        repos.push(repo);
    }

    function createProgressEl(phase, percent) {
        const progressContainer = createElement("div", "repo-landing__clone-progress");
        progressContainer.appendChild(createElement("span", "repo-landing__card-progress", `${phase} ${percent}%`));

        const progressBar = createElement("div", "repo-landing__progress-bar");
        const progressFill = createElement("div", "repo-landing__progress-fill");
        progressFill.style.width = `${percent}%`;
        progressBar.appendChild(progressFill);
        progressContainer.appendChild(progressBar);
        return progressContainer;
    }

    function renderFeaturedCard(entry) {
        const state = featuredState.get(entry.url) || { id: null, state: "pending", error: null, phase: "", percent: 0 };
        const existing = featuredGrid.querySelector(`[data-url="${entry.url}"]`);
        const card = existing || createElement("div", "repo-landing__card");
        card.className = "repo-landing__card";
        card.dataset.url = entry.url;

        if (state.state === "ready") {
            card.classList.add("repo-landing__card--ready");
        } else if (state.state === "cloning" || state.state === "pending") {
            card.classList.add("repo-landing__card--loading");
        }

        card.replaceChildren();

        const header = createElement("div", "repo-landing__card-header");
        header.appendChild(createElement("span", "repo-landing__card-name", entry.name));
        header.appendChild(createElement("span", `repo-landing__badge repo-landing__badge--${state.state}`, state.state));

        const desc = createElement("p", "repo-landing__card-desc", entry.description);
        const metaText = state.state === "ready"
            ? "Ready now. Open the graph view and inspect commit flow."
            : "Preparing graph data and commit history.";
        const meta = createElement("p", "repo-landing__card-meta", metaText);
        const action = createElement("div", "repo-landing__card-action");

        if (state.state === "ready") {
            const cta = createElement("button", "repo-landing__card-cta");
            cta.innerHTML = `Open Live View ${ARROW_SVG}`;
            cta.addEventListener("click", (event) => {
                event.stopPropagation();
                onRepoSelect(state.id);
            });
            action.appendChild(cta);
        } else if (state.state === "error") {
            if (state.error) {
                action.appendChild(createElement("span", "repo-landing__item-error", state.error));
            }
            const retry = createElement("button", "repo-landing__card-cta", "Retry");
            retry.addEventListener("click", (event) => {
                event.stopPropagation();
                initFeaturedRepo(entry);
            });
            action.appendChild(retry);
        } else if (state.state === "cloning" && state.percent > 0) {
            action.appendChild(createProgressEl(state.phase, state.percent));
        } else {
            action.appendChild(createElement("span", "repo-landing__card-progress", state.state === "cloning" ? "Cloning..." : "Loading..."));
        }

        card.appendChild(header);
        card.appendChild(desc);
        card.appendChild(meta);
        card.appendChild(action);

        card.onclick = state.state === "ready" ? () => onRepoSelect(state.id) : null;
        if (!existing) featuredGrid.appendChild(card);
    }

    function renderUserRepos() {
        heroRecentListEl.replaceChildren();

        const featuredIds = new Set();
        for (const state of featuredState.values()) {
            if (state.id) featuredIds.add(state.id);
        }
        const userRepos = repos.filter((repo) => !featuredIds.has(repo.id));
        if (userRepos.length === 0) return;

        heroRecentListEl.appendChild(createElement("h3", "repo-landing__list-heading", "Your recent repositories"));
        for (const repo of userRepos) {
            const item = createElement("div", "repo-landing__item");
            if (repo.state === "ready") item.classList.add("repo-landing__item--ready");

            const info = createElement("div", "repo-landing__item-info");
            info.appendChild(createElement("span", "repo-landing__item-url", repo.url || repo.id));
            info.appendChild(createElement("span", `repo-landing__badge repo-landing__badge--${repo.state}`, repo.state));

            if (repo.error) {
                info.appendChild(createElement("span", "repo-landing__item-error", repo.error));
            }
            if (repo.state === "cloning" && repo.percent > 0) {
                info.appendChild(createProgressEl(repo.phase, repo.percent));
            }

            const deleteBtn = createElement("button", "repo-landing__delete-btn");
            deleteBtn.title = "Remove repository";
            deleteBtn.innerHTML = DELETE_SVG;
            deleteBtn.addEventListener("click", (event) => {
                event.stopPropagation();
                deleteRepo(repo.id);
            });

            item.appendChild(info);
            item.appendChild(deleteBtn);
            if (repo.state === "ready") {
                item.addEventListener("click", () => onRepoSelect(repo.id));
            }
            heroRecentListEl.appendChild(item);
        }
    }

    async function createRepo(url) {
        const resp = await fetch("/api/repos", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ url }),
        });
        if (!resp.ok) {
            const text = await resp.text();
            throw new Error(text || `HTTP ${resp.status}`);
        }
        return resp.json();
    }

    async function initFeaturedRepo(entry) {
        featuredState.set(entry.url, { id: null, state: "pending", error: null, phase: "", percent: 0 });
        renderFeaturedCard(entry);

        try {
            const repo = await createRepo(entry.url);
            if (destroyed) return;

            featuredState.set(entry.url, {
                id: repo.id,
                state: repo.state,
                error: null,
                phase: repo.phase || "",
                percent: repo.percent || 0,
            });
            renderFeaturedCard(entry);

            if (repo.state === "cloning" || repo.state === "pending") {
                startProgressStream(repo.id, (update) => {
                    if (destroyed) return;
                    featuredState.set(entry.url, {
                        id: update.id,
                        state: update.state,
                        error: update.error,
                        phase: update.phase || "",
                        percent: update.percent || 0,
                    });
                    renderFeaturedCard(entry);
                });
            }
        } catch (error) {
            if (destroyed) return;
            featuredState.set(entry.url, { id: null, state: "error", error: error.message, phase: "", percent: 0 });
            renderFeaturedCard(entry);
        }
    }

    async function deleteRepo(id) {
        try {
            await fetch(`/api/repos/${id}`, { method: "DELETE" });
            repos = repos.filter((repo) => repo.id !== id);
            destroyStream(id);
            renderUserRepos();
        } catch {
            // Ignore delete failures.
        }
    }

    async function openRepository(url) {
        const normalized = normalizeRepoUrl(url);
        const featuredEntry = featuredRepos.find((entry) => normalizeRepoUrl(entry.url) === normalized);
        if (featuredEntry && featuredState.has(featuredEntry.url)) {
            return { kind: "featured", element: getFeaturedCard(featuredEntry.url) };
        }

        const repo = await createRepo(url);
        if (destroyed) return { kind: "destroyed", element: null };

        for (const [featuredUrl, state] of featuredState.entries()) {
            if (state.id === repo.id) {
                return { kind: "featured", element: getFeaturedCard(featuredUrl) };
            }
        }

        addOrUpdateRepo(repo);
        renderUserRepos();

        if (repo.state === "cloning" || repo.state === "pending") {
            startProgressStream(repo.id, (update) => {
                if (destroyed) return;
                addOrUpdateRepo({ ...update, url: repo.url });
                renderUserRepos();
            });
        }
        return { kind: "repo", element: heroRecentListEl };
    }

    function getFeaturedCard(url) {
        return featuredGrid.querySelector(`[data-url="${url}"]`);
    }

    async function loadInitialRepos() {
        featuredRepos.forEach((entry) => {
            featuredState.set(entry.url, { id: null, state: "pending", error: null, phase: "", percent: 0 });
            renderFeaturedCard(entry);
        });
        Promise.allSettled(featuredRepos.map((entry) => initFeaturedRepo(entry)));

        try {
            const resp = await fetch("/api/repos");
            if (!resp.ok) return;

            const list = await resp.json();
            if (destroyed) return;

            for (const repo of list) {
                addOrUpdateRepo(repo);
                if (repo.state === "cloning" || repo.state === "pending") {
                    startProgressStream(repo.id, (update) => {
                        if (destroyed) return;
                        addOrUpdateRepo(update);
                        renderUserRepos();
                    });
                }
            }
            renderUserRepos();
        } catch {
            // Silently fail initial load.
        }
    }

    function destroy() {
        destroyed = true;
        for (const id of activeStreams.keys()) {
            destroyStream(id);
        }
    }

    return {
        heroRecentListEl,
        featuredSectionEl,
        getFeaturedCard,
        loadInitialRepos,
        openRepository,
        destroy,
    };
}


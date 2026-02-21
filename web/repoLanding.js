const STATUS_POLL_INTERVAL = 2000;

const LOGO_SVG = `<svg width="32" height="32" viewBox="0 0 16 16" fill="none">
    <circle cx="8" cy="4" r="2" fill="currentColor"/>
    <circle cx="4" cy="12" r="2" fill="currentColor"/>
    <circle cx="12" cy="12" r="2" fill="currentColor"/>
    <path d="M8 6v2M6.5 10.5L8 8M9.5 10.5L8 8" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
</svg>`;

const DELETE_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

/**
 * Creates the SaaS-mode repo landing page.
 *
 * @param {Object} opts
 * @param {(repoId: string) => void} opts.onRepoSelect — called when user clicks a ready repo
 * @returns {{ el: HTMLElement, destroy: () => void }}
 */
export function createRepoLanding({ onRepoSelect }) {
    const el = document.createElement("div");
    el.className = "repo-landing";

    let repos = [];
    let pollTimers = new Map(); // id -> intervalId
    let destroyed = false;

    // ── Header ───────────────────────────────────────────────────────────
    const header = document.createElement("div");
    header.className = "repo-landing__header";

    const logo = document.createElement("div");
    logo.className = "repo-landing__logo";
    logo.innerHTML = LOGO_SVG;

    const title = document.createElement("h1");
    title.className = "repo-landing__title";
    title.textContent = "GitVista";

    const subtitle = document.createElement("p");
    subtitle.className = "repo-landing__subtitle";
    subtitle.textContent = "Paste a GitHub repository URL to visualize its commit graph";

    header.appendChild(logo);
    header.appendChild(title);
    header.appendChild(subtitle);

    // ── Input form ───────────────────────────────────────────────────────
    const form = document.createElement("form");
    form.className = "repo-landing__form";

    const input = document.createElement("input");
    input.type = "url";
    input.className = "repo-landing__input";
    input.placeholder = "https://github.com/owner/repo";
    input.required = true;

    const addBtn = document.createElement("button");
    addBtn.type = "submit";
    addBtn.className = "repo-landing__add-btn";
    addBtn.textContent = "Add";

    form.appendChild(input);
    form.appendChild(addBtn);

    const errorMsg = document.createElement("div");
    errorMsg.className = "repo-landing__error";

    form.addEventListener("submit", async (e) => {
        e.preventDefault();
        errorMsg.textContent = "";
        const url = input.value.trim();
        if (!url) return;

        addBtn.disabled = true;
        addBtn.textContent = "Adding...";

        try {
            const resp = await fetch("/api/repos", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ url }),
            });

            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(text || `HTTP ${resp.status}`);
            }

            const repo = await resp.json();
            input.value = "";

            // Add to list and start polling if cloning
            addOrUpdateRepo(repo);
            render();

            if (repo.state === "cloning") {
                startPolling(repo.id);
            }
        } catch (err) {
            errorMsg.textContent = err.message;
        } finally {
            addBtn.disabled = false;
            addBtn.textContent = "Add";
        }
    });

    // ── Repo list ────────────────────────────────────────────────────────
    const listContainer = document.createElement("div");
    listContainer.className = "repo-landing__list";

    function addOrUpdateRepo(repo) {
        const idx = repos.findIndex((r) => r.id === repo.id);
        if (idx >= 0) {
            repos[idx] = { ...repos[idx], ...repo };
        } else {
            repos.push(repo);
        }
    }

    function startPolling(id) {
        if (pollTimers.has(id)) return;

        const timerId = setInterval(async () => {
            if (destroyed) {
                clearInterval(timerId);
                return;
            }

            try {
                const resp = await fetch(`/api/repos/${id}/status`);
                if (!resp.ok) return;
                const data = await resp.json();

                addOrUpdateRepo({ id, state: data.state, error: data.error });
                render();

                if (data.state !== "cloning") {
                    clearInterval(timerId);
                    pollTimers.delete(id);
                }
            } catch {
                // Silently retry on next interval
            }
        }, STATUS_POLL_INTERVAL);

        pollTimers.set(id, timerId);
    }

    async function deleteRepo(id) {
        try {
            await fetch(`/api/repos/${id}`, { method: "DELETE" });
            repos = repos.filter((r) => r.id !== id);
            if (pollTimers.has(id)) {
                clearInterval(pollTimers.get(id));
                pollTimers.delete(id);
            }
            render();
        } catch {
            // Ignore delete failures
        }
    }

    function render() {
        listContainer.innerHTML = "";

        if (repos.length === 0) return;

        const heading = document.createElement("h2");
        heading.className = "repo-landing__list-heading";
        heading.textContent = "Repositories";
        listContainer.appendChild(heading);

        for (const repo of repos) {
            const item = document.createElement("div");
            item.className = "repo-landing__item";
            if (repo.state === "ready") {
                item.classList.add("repo-landing__item--ready");
            }

            const info = document.createElement("div");
            info.className = "repo-landing__item-info";

            const urlText = document.createElement("span");
            urlText.className = "repo-landing__item-url";
            urlText.textContent = repo.url || repo.id;
            info.appendChild(urlText);

            const badge = document.createElement("span");
            badge.className = `repo-landing__badge repo-landing__badge--${repo.state}`;
            badge.textContent = repo.state;
            info.appendChild(badge);

            if (repo.error) {
                const errText = document.createElement("span");
                errText.className = "repo-landing__item-error";
                errText.textContent = repo.error;
                info.appendChild(errText);
            }

            item.appendChild(info);

            const deleteBtn = document.createElement("button");
            deleteBtn.className = "repo-landing__delete-btn";
            deleteBtn.innerHTML = DELETE_SVG;
            deleteBtn.title = "Remove repository";
            deleteBtn.addEventListener("click", (e) => {
                e.stopPropagation();
                deleteRepo(repo.id);
            });
            item.appendChild(deleteBtn);

            if (repo.state === "ready") {
                item.addEventListener("click", () => {
                    onRepoSelect(repo.id);
                });
            }

            listContainer.appendChild(item);
        }
    }

    // ── Assemble ─────────────────────────────────────────────────────────
    el.appendChild(header);
    el.appendChild(form);
    el.appendChild(errorMsg);
    el.appendChild(listContainer);

    // Initial fetch of existing repos
    (async () => {
        try {
            const resp = await fetch("/api/repos");
            if (!resp.ok) return;
            const list = await resp.json();
            if (destroyed) return;

            for (const repo of list) {
                addOrUpdateRepo(repo);
                if (repo.state === "cloning") {
                    startPolling(repo.id);
                }
            }
            render();
        } catch {
            // Silently fail initial load
        }
    })();

    function destroy() {
        destroyed = true;
        for (const [, timerId] of pollTimers) {
            clearInterval(timerId);
        }
        pollTimers.clear();
    }

    return { el, destroy };
}

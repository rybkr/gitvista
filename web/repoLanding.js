const STATUS_POLL_INTERVAL = 2000;

const FEATURED_REPOS = [
    { url: "https://github.com/jqlang/jq", name: "jqlang/jq", description: "Command-line JSON processor" },
    { url: "https://github.com/expressjs/express", name: "expressjs/express", description: "Fast, unopinionated web framework for Node.js" },
];

const LOGO_SVG = `<svg width="48" height="48" viewBox="0 0 48 48" fill="none" xmlns="http://www.w3.org/2000/svg">
    <circle cx="24" cy="8" r="4" fill="var(--node-color)"/>
    <circle cx="24" cy="24" r="4" fill="var(--node-color)"/>
    <circle cx="24" cy="40" r="4" fill="var(--node-color)"/>
    <circle cx="36" cy="16" r="4" fill="var(--merge-node-color)"/>
    <circle cx="36" cy="32" r="4" fill="var(--merge-node-color)"/>
    <line x1="24" y1="12" x2="24" y2="20" stroke="var(--node-color)" stroke-width="2"/>
    <line x1="24" y1="28" x2="24" y2="36" stroke="var(--node-color)" stroke-width="2"/>
    <path d="M24 12 C24 14 30 14 36 16" stroke="var(--node-color)" stroke-width="2" fill="none"/>
    <line x1="36" y1="20" x2="36" y2="28" stroke="var(--merge-node-color)" stroke-width="2"/>
    <path d="M36 32 C30 34 24 34 24 36" stroke="var(--merge-node-color)" stroke-width="2" fill="none"/>
</svg>`;

const DELETE_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const COPY_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <rect x="5" y="5" width="8" height="8" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
    <path d="M3 11V3a1.5 1.5 0 011.5-1.5H11" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const CHECK_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M3 8.5l3.5 3.5 6.5-8" stroke="var(--success-color)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const GITHUB_SVG = `<svg width="18" height="18" viewBox="0 0 16 16" fill="currentColor">
    <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/>
</svg>`;

const ARROW_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M3 8h10M9 4l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
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
    let pollTimers = new Map();
    let destroyed = false;

    /** @type {Map<string, { id: string|null, state: string, error: string|null }>} keyed by URL */
    const featuredState = new Map();

    // ── Shared helpers ────────────────────────────────────────────────────

    function startPolling(id, onUpdate) {
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

                if (onUpdate) {
                    onUpdate({ id, state: data.state, error: data.error });
                }

                if (data.state !== "cloning" && data.state !== "pending") {
                    clearInterval(timerId);
                    pollTimers.delete(id);
                }
            } catch {
                // Silently retry on next interval
            }
        }, STATUS_POLL_INTERVAL);

        pollTimers.set(id, timerId);
    }

    function addOrUpdateRepo(repo) {
        const idx = repos.findIndex((r) => r.id === repo.id);
        if (idx >= 0) {
            repos[idx] = { ...repos[idx], ...repo };
        } else {
            repos.push(repo);
        }
    }

    // ── 1. Hero ───────────────────────────────────────────────────────────

    const hero = document.createElement("section");
    hero.className = "repo-landing__hero";

    const logo = document.createElement("div");
    logo.className = "repo-landing__logo";
    logo.innerHTML = LOGO_SVG;

    const title = document.createElement("h1");
    title.className = "repo-landing__title";
    title.textContent = "GitVista";

    const tagline = document.createElement("p");
    tagline.className = "repo-landing__tagline";
    tagline.textContent = "See what Git is actually doing.";

    const heroActions = document.createElement("div");
    heroActions.className = "repo-landing__hero-actions";

    const ctaPrimary = document.createElement("button");
    ctaPrimary.className = "repo-landing__cta-primary";
    ctaPrimary.textContent = "Explore a repo";
    ctaPrimary.addEventListener("click", () => {
        featuredSection.scrollIntoView({ behavior: "smooth" });
    });

    const ctaSecondary = document.createElement("button");
    ctaSecondary.className = "repo-landing__cta-secondary";
    ctaSecondary.textContent = "Install locally";
    ctaSecondary.addEventListener("click", () => {
        installSection.scrollIntoView({ behavior: "smooth" });
    });

    heroActions.appendChild(ctaPrimary);
    heroActions.appendChild(ctaSecondary);

    hero.appendChild(logo);
    hero.appendChild(title);
    hero.appendChild(tagline);
    hero.appendChild(heroActions);

    // ── 2. Featured Repos ─────────────────────────────────────────────────

    const featuredSection = document.createElement("section");
    featuredSection.className = "repo-landing__section repo-landing__featured";

    const featuredTitle = document.createElement("h2");
    featuredTitle.className = "repo-landing__section-title";
    featuredTitle.textContent = "Featured Repositories";

    const featuredSubtitle = document.createElement("p");
    featuredSubtitle.className = "repo-landing__section-subtitle";
    featuredSubtitle.textContent = "Pre-loaded and ready to explore";

    const featuredGrid = document.createElement("div");
    featuredGrid.className = "repo-landing__featured-grid";

    featuredSection.appendChild(featuredTitle);
    featuredSection.appendChild(featuredSubtitle);
    featuredSection.appendChild(featuredGrid);

    function renderFeaturedCard(entry) {
        const s = featuredState.get(entry.url) || { id: null, state: "pending", error: null };
        const existing = featuredGrid.querySelector(`[data-url="${entry.url}"]`);

        const card = existing || document.createElement("div");
        card.className = "repo-landing__card";
        card.dataset.url = entry.url;

        if (s.state === "ready") {
            card.classList.add("repo-landing__card--ready");
        } else if (s.state === "cloning" || s.state === "pending") {
            card.classList.add("repo-landing__card--loading");
        }

        card.innerHTML = "";

        const cardHeader = document.createElement("div");
        cardHeader.className = "repo-landing__card-header";

        const cardName = document.createElement("span");
        cardName.className = "repo-landing__card-name";
        cardName.textContent = entry.name;

        const badge = document.createElement("span");
        badge.className = `repo-landing__badge repo-landing__badge--${s.state}`;
        badge.textContent = s.state;

        cardHeader.appendChild(cardName);
        cardHeader.appendChild(badge);

        const cardDesc = document.createElement("p");
        cardDesc.className = "repo-landing__card-desc";
        cardDesc.textContent = entry.description;

        const cardAction = document.createElement("div");
        cardAction.className = "repo-landing__card-action";

        if (s.state === "ready") {
            const cta = document.createElement("button");
            cta.className = "repo-landing__card-cta";
            cta.innerHTML = `Explore ${ARROW_SVG}`;
            cta.addEventListener("click", (e) => {
                e.stopPropagation();
                onRepoSelect(s.id);
            });
            cardAction.appendChild(cta);
        } else if (s.state === "error") {
            const retry = document.createElement("button");
            retry.className = "repo-landing__card-cta";
            retry.textContent = "Retry";
            retry.addEventListener("click", (e) => {
                e.stopPropagation();
                initSingleFeatured(entry);
            });
            if (s.error) {
                const errText = document.createElement("span");
                errText.className = "repo-landing__item-error";
                errText.textContent = s.error;
                cardAction.appendChild(errText);
            }
            cardAction.appendChild(retry);
        } else {
            const progress = document.createElement("span");
            progress.className = "repo-landing__card-progress";
            progress.textContent = s.state === "cloning" ? "Cloning..." : "Loading...";
            cardAction.appendChild(progress);
        }

        card.appendChild(cardHeader);
        card.appendChild(cardDesc);
        card.appendChild(cardAction);

        if (s.state === "ready") {
            card.onclick = () => onRepoSelect(s.id);
        } else {
            card.onclick = null;
        }

        if (!existing) {
            featuredGrid.appendChild(card);
        }
    }

    async function initSingleFeatured(entry) {
        featuredState.set(entry.url, { id: null, state: "pending", error: null });
        renderFeaturedCard(entry);

        try {
            const resp = await fetch("/api/repos", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ url: entry.url }),
            });

            if (!resp.ok) {
                const text = await resp.text();
                throw new Error(text || `HTTP ${resp.status}`);
            }

            const repo = await resp.json();
            if (destroyed) return;

            featuredState.set(entry.url, { id: repo.id, state: repo.state, error: null });
            renderFeaturedCard(entry);

            if (repo.state === "cloning" || repo.state === "pending") {
                startPolling(repo.id, (update) => {
                    if (destroyed) return;
                    featuredState.set(entry.url, { id: update.id, state: update.state, error: update.error });
                    renderFeaturedCard(entry);
                });
            }
        } catch (err) {
            if (destroyed) return;
            featuredState.set(entry.url, { id: null, state: "error", error: err.message });
            renderFeaturedCard(entry);
        }
    }

    function initFeatured() {
        const promises = FEATURED_REPOS.map((entry) => initSingleFeatured(entry));
        Promise.allSettled(promises);
    }

    // ── 3. Try Your Own ───────────────────────────────────────────────────

    const tryOwnSection = document.createElement("section");
    tryOwnSection.className = "repo-landing__section repo-landing__tryown";

    const tryOwnTitle = document.createElement("h2");
    tryOwnTitle.className = "repo-landing__section-title";
    tryOwnTitle.textContent = "Visualize any public GitHub repository";

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
    addBtn.textContent = "Visualize";

    form.appendChild(input);
    form.appendChild(addBtn);

    const errorMsg = document.createElement("div");
    errorMsg.className = "repo-landing__error";

    form.addEventListener("submit", async (e) => {
        e.preventDefault();
        errorMsg.textContent = "";
        const url = input.value.trim();
        if (!url) return;

        // If the URL matches a featured repo, scroll to it instead
        const featuredEntry = FEATURED_REPOS.find((f) => f.url === url || f.url === url.replace(/\/$/, ""));
        if (featuredEntry && featuredState.has(featuredEntry.url)) {
            const card = featuredGrid.querySelector(`[data-url="${featuredEntry.url}"]`);
            if (card) {
                card.scrollIntoView({ behavior: "smooth" });
                card.style.outline = "2px solid var(--node-color)";
                setTimeout(() => { card.style.outline = ""; }, 1500);
                input.value = "";
                return;
            }
        }

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
            if (destroyed) return;
            input.value = "";

            // Check if this repo ID belongs to a featured repo
            for (const [fUrl, fState] of featuredState) {
                if (fState.id === repo.id) {
                    const card = featuredGrid.querySelector(`[data-url="${fUrl}"]`);
                    if (card) {
                        card.scrollIntoView({ behavior: "smooth" });
                        card.style.outline = "2px solid var(--node-color)";
                        setTimeout(() => { card.style.outline = ""; }, 1500);
                    }
                    return;
                }
            }

            addOrUpdateRepo(repo);
            renderUserRepos();

            if (repo.state === "cloning" || repo.state === "pending") {
                startPolling(repo.id, (update) => {
                    if (destroyed) return;
                    addOrUpdateRepo(update);
                    renderUserRepos();
                });
            }
        } catch (err) {
            errorMsg.textContent = err.message;
        } finally {
            addBtn.disabled = false;
            addBtn.textContent = "Visualize";
        }
    });

    const listContainer = document.createElement("div");
    listContainer.className = "repo-landing__list";

    tryOwnSection.appendChild(tryOwnTitle);
    tryOwnSection.appendChild(form);
    tryOwnSection.appendChild(errorMsg);
    tryOwnSection.appendChild(listContainer);

    async function deleteRepo(id) {
        try {
            await fetch(`/api/repos/${id}`, { method: "DELETE" });
            repos = repos.filter((r) => r.id !== id);
            if (pollTimers.has(id)) {
                clearInterval(pollTimers.get(id));
                pollTimers.delete(id);
            }
            renderUserRepos();
        } catch {
            // Ignore delete failures
        }
    }

    function renderUserRepos() {
        listContainer.innerHTML = "";

        // Filter out repos whose IDs match featured repos
        const featuredIds = new Set();
        for (const s of featuredState.values()) {
            if (s.id) featuredIds.add(s.id);
        }
        const userRepos = repos.filter((r) => !featuredIds.has(r.id));

        if (userRepos.length === 0) return;

        const heading = document.createElement("h3");
        heading.className = "repo-landing__list-heading";
        heading.textContent = "Your Repositories";
        listContainer.appendChild(heading);

        for (const repo of userRepos) {
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

    // ── 4. Install Locally ────────────────────────────────────────────────

    const installSection = document.createElement("section");
    installSection.className = "repo-landing__section repo-landing__install";

    const installTitle = document.createElement("h2");
    installTitle.className = "repo-landing__section-title";
    installTitle.textContent = "Run locally for real-time updates";

    const installSubtitle = document.createElement("p");
    installSubtitle.className = "repo-landing__section-subtitle";
    installSubtitle.textContent = "Watch commits appear the instant you make them. Local mode connects directly to your .git directory for zero-latency visualization.";

    const codeBlock = document.createElement("div");
    codeBlock.className = "repo-landing__code-block";

    const codeText = document.createElement("code");
    codeText.textContent = "go install github.com/AmoghShet/gitvista/cmd/vista@latest && vista";

    const copyBtn = document.createElement("button");
    copyBtn.className = "repo-landing__copy-btn";
    copyBtn.innerHTML = COPY_SVG;
    copyBtn.title = "Copy to clipboard";
    copyBtn.addEventListener("click", async () => {
        try {
            await navigator.clipboard.writeText(codeText.textContent);
            copyBtn.innerHTML = CHECK_SVG;
            setTimeout(() => { copyBtn.innerHTML = COPY_SVG; }, 2000);
        } catch {
            // Clipboard API not available
        }
    });

    codeBlock.appendChild(codeText);
    codeBlock.appendChild(copyBtn);

    installSection.appendChild(installTitle);
    installSection.appendChild(installSubtitle);
    installSection.appendChild(codeBlock);

    // ── 5. Footer ─────────────────────────────────────────────────────────

    const footer = document.createElement("footer");
    footer.className = "repo-landing__footer";

    const footerLinks = document.createElement("div");
    footerLinks.className = "repo-landing__footer-links";

    const ghLink = document.createElement("a");
    ghLink.className = "repo-landing__footer-link";
    ghLink.href = "https://github.com/AmoghShet/gitvista";
    ghLink.target = "_blank";
    ghLink.rel = "noopener noreferrer";
    ghLink.innerHTML = `${GITHUB_SVG} GitHub`;

    const ytLink = document.createElement("a");
    ytLink.className = "repo-landing__footer-link";
    ytLink.href = "https://youtube.com/@gitvista";
    ytLink.target = "_blank";
    ytLink.rel = "noopener noreferrer";
    ytLink.textContent = "YouTube";

    footerLinks.appendChild(ghLink);
    footerLinks.appendChild(ytLink);

    const license = document.createElement("span");
    license.className = "repo-landing__footer-license";
    license.textContent = "Apache 2.0";

    footer.appendChild(footerLinks);
    footer.appendChild(license);

    // ── Assemble ──────────────────────────────────────────────────────────

    el.appendChild(hero);
    el.appendChild(featuredSection);
    el.appendChild(tryOwnSection);
    el.appendChild(installSection);
    el.appendChild(footer);

    // ── Initialize ────────────────────────────────────────────────────────

    initFeatured();

    // Load existing user repos
    (async () => {
        try {
            const resp = await fetch("/api/repos");
            if (!resp.ok) return;
            const list = await resp.json();
            if (destroyed) return;

            for (const repo of list) {
                addOrUpdateRepo(repo);
                if (repo.state === "cloning" || repo.state === "pending") {
                    startPolling(repo.id, (update) => {
                        if (destroyed) return;
                        addOrUpdateRepo(update);
                        renderUserRepos();
                    });
                }
            }
            renderUserRepos();
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

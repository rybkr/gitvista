const FEATURED_REPOS = [
    { url: "https://github.com/rybkr/gitvista", name: "rybkr/gitvista", description: "Git history visualization for branches, diffs, and activity" },
    { url: "https://github.com/jqlang/jq", name: "jqlang/jq", description: "Command-line JSON processor" },
];
const HERO_PREVIEW = {
    path: "gitvista.io / repo / rybkr / gitvista",
    name: "GitVista",
    branch: "main",
    head: "HEAD lane mode",
    description: "Read the repository at branch level first, then drop into individual commits and diffs when you need proof.",
    metrics: [
        { label: "Commits", value: "14.2k", note: "history indexed" },
        { label: "Branches", value: "38", note: "refs in view" },
        { label: "Tags", value: "112", note: "release markers" },
    ],
    activity: [
        { hash: "50c9298", title: "Fix landing preview loading and layout behavior", meta: "main • just now", tone: "main" },
        { hash: "d6a11f2", title: "Stabilize hosted repo bootstrap after clone completion", meta: "preview-refresh • 12 min ago", tone: "branch" },
        { hash: "4bb2e9c", title: "Merge lane layout polish into landing worktree", meta: "merge from ui-pass", tone: "merge" },
        { hash: "9f90d8a", title: "Add repository overview cards and docs navigation", meta: "main • 34 min ago", tone: "main" },
    ],
    insights: [
        { label: "Graph-first", value: "Branch motion stays visible while you inspect commits." },
        { label: "Repo overview", value: "Head, refs, tags, and remotes stay in one rail." },
        { label: "Diff-ready", value: "Open a commit only when you already know why it matters." },
    ],
};

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
 * Creates the hosted-mode repo landing page.
 *
 * @param {Object} opts
 * @param {(repoId: string) => void} opts.onRepoSelect — called when user clicks a ready repo
 * @param {(path: string) => void} [opts.onNavigate] — called for hosted route navigation
 * @returns {{ el: HTMLElement, destroy: () => void }}
 */
export function createRepoLanding({ onRepoSelect, onNavigate }) {
    const el = document.createElement("div");
    el.className = "repo-landing";

    const chrome = document.createElement("div");
    chrome.className = "repo-landing__chrome";

    const content = document.createElement("div");
    content.className = "repo-landing__content";

    let repos = [];
    let activeStreams = new Map();
    let destroyed = false;
    let highlightTimer = null;

    /** @type {Map<string, { id: string|null, state: string, error: string|null, phase: string, percent: number }>} keyed by URL */
    const featuredState = new Map();

    function normalizeRepoUrl(url) {
        return typeof url === "string" ? url.replace(/\/+$/, "") : "";
    }

    function scrollToSection(target, { focus } = {}) {
        target?.scrollIntoView({ behavior: "smooth", block: "start" });
        if (focus) {
            window.setTimeout(() => {
                focus.focus();
                if (typeof focus.select === "function") {
                    focus.select();
                }
            }, 180);
        }
    }

    function highlightElementTemporarily(node) {
        if (!node) return;
        node.classList.add("repo-landing__highlighted");
        window.clearTimeout(highlightTimer);
        highlightTimer = window.setTimeout(() => {
            node.classList.remove("repo-landing__highlighted");
        }, 1600);
    }

    function bindHostedNavigation(link, path) {
        link.href = path;
        if (typeof onNavigate !== "function") return;
        link.addEventListener("click", (event) => {
            if (
                event.defaultPrevented ||
                event.button !== 0 ||
                event.metaKey ||
                event.ctrlKey ||
                event.shiftKey ||
                event.altKey
            ) {
                return;
            }
            event.preventDefault();
            onNavigate(path);
        });
    }

    // ── Shared helpers ────────────────────────────────────────────────────

    function startProgressStream(id, onUpdate) {
        if (activeStreams.has(id)) return;

        const es = new EventSource(`/api/repos/${id}/progress`);
        activeStreams.set(id, es);

        es.onmessage = (event) => {
            if (destroyed) {
                es.close();
                activeStreams.delete(id);
                return;
            }

            try {
                const data = JSON.parse(event.data);

                if (data.done) {
                    if (onUpdate) {
                        onUpdate({ id, state: data.state, error: data.error || "", phase: "", percent: 0 });
                    }
                    es.close();
                    activeStreams.delete(id);
                    return;
                }

                if (onUpdate) {
                    onUpdate({ id, state: "cloning", error: "", phase: data.phase || "", percent: data.percent || 0 });
                }
            } catch {
                // Ignore malformed events
            }
        };

        es.onerror = () => {
            es.close();
            activeStreams.delete(id);

            fetch(`/api/repos/${id}/status`)
                .then((resp) => resp.ok ? resp.json() : null)
                .then((data) => {
                    if (data && onUpdate && !destroyed) {
                        onUpdate({ id: data.id || id, state: data.state, error: data.error || "", phase: data.phase || "", percent: data.percent || 0 });
                    }
                })
                .catch(() => {});
        };
    }

    function addOrUpdateRepo(repo) {
        const idx = repos.findIndex((r) => r.id === repo.id);
        if (idx >= 0) {
            repos[idx] = { ...repos[idx], ...repo };
        } else {
            repos.push(repo);
        }
    }

    // ── 1. Header + Hero ─────────────────────────────────────────────────

    const topbar = document.createElement("header");
    topbar.className = "repo-landing__topbar";

    const topbarNav = document.createElement("nav");
    topbarNav.className = "repo-landing__topbar-nav";
    topbarNav.setAttribute("aria-label", "Primary");

    const brand = document.createElement("a");
    brand.className = "repo-landing__brand";
    brand.href = "#try";
    brand.setAttribute("aria-label", "GitVista home");
    brand.addEventListener("click", (event) => {
        event.preventDefault();
        scrollToSection(el.querySelector("#try"), { focus: input });
    });

    const brandMark = document.createElement("img");
    brandMark.className = "repo-landing__brand-mark";
    brandMark.src = "/favicon.svg";
    brandMark.alt = "";
    brandMark.setAttribute("aria-hidden", "true");

    const brandCopy = document.createElement("span");
    brandCopy.className = "repo-landing__brand-copy";
    brandCopy.innerHTML = `<strong>GitVista</strong><span>see what git is actually doing.</span>`;

    brand.appendChild(brandMark);
    brand.appendChild(brandCopy);
    topbarNav.appendChild(brand);

    const topbarLinks = document.createElement("div");
    topbarLinks.className = "repo-landing__topbar-links";

    const navItems = [
        { label: "Overview", targetId: "try" },
        { label: "Featured", targetId: "featured" },
        { label: "Local", targetId: "local" },
        { label: "Docs", href: "/docs" },
    ];

    for (const item of navItems) {
        const link = document.createElement("a");
        link.className = "repo-landing__nav-link";
        link.textContent = item.label;
        if (item.href) {
            bindHostedNavigation(link, item.href);
        } else if (item.external) {
            link.href = item.href;
            link.target = "_blank";
            link.rel = "noopener noreferrer";
        } else {
            link.href = `#${item.targetId}`;
            link.addEventListener("click", (event) => {
                event.preventDefault();
                const target = el.querySelector(`#${item.targetId}`);
                scrollToSection(target, item.targetId === "try" ? { focus: input } : undefined);
            });
        }
        topbarLinks.appendChild(link);
    }

    topbarNav.appendChild(topbarLinks);

    topbar.appendChild(topbarNav);

    const hero = document.createElement("section");
    hero.className = "repo-landing__hero";
    hero.id = "try";

    const heroCopy = document.createElement("div");
    heroCopy.className = "repo-landing__hero-copy";

    const title = document.createElement("h1");
    title.className = "repo-landing__title";
    title.textContent = "Bring your Git history into view at a glance.";

    const tagline = document.createElement("p");
    tagline.className = "repo-landing__tagline";
    tagline.textContent = "GitVista turns a repository into a readable branch graph, recent activity view, and commit-by-commit workspace. Start with a public GitHub URL in the browser, then switch to local mode when you need live updates from your own checkout.";

    const heroFormShell = document.createElement("div");
    heroFormShell.className = "repo-landing__hero-form-shell";

    const formLead = document.createElement("div");
    formLead.className = "repo-landing__hero-form-lead";
    formLead.innerHTML = `<strong>Try it now</strong><span>Open a public repository directly in the browser.</span>`;

    const form = document.createElement("form");
    form.className = "repo-landing__form";

    const input = document.createElement("input");
    input.type = "url";
    input.className = "repo-landing__input";
    input.placeholder = "https://github.com/owner/repo";
    input.required = true;
    input.setAttribute("aria-label", "GitHub repository URL");

    const addBtn = document.createElement("button");
    addBtn.type = "submit";
    addBtn.className = "repo-landing__add-btn";
    addBtn.textContent = "Open Repository";

    form.appendChild(input);
    form.appendChild(addBtn);

    const formMeta = document.createElement("div");
    formMeta.className = "repo-landing__hero-form-meta";

    const heroSupport = document.createElement("span");
    heroSupport.className = "repo-landing__hero-support";
    heroSupport.textContent = "Works best with public GitHub repositories.";

    const installShortcut = document.createElement("button");
    installShortcut.type = "button";
    installShortcut.className = "repo-landing__hero-link";
    installShortcut.textContent = "Prefer local mode?";
    installShortcut.addEventListener("click", () => {
        scrollToSection(installSection);
    });

    formMeta.appendChild(heroSupport);
    formMeta.appendChild(installShortcut);

    const errorMsg = document.createElement("div");
    errorMsg.className = "repo-landing__error";

    const listContainer = document.createElement("div");
    listContainer.className = "repo-landing__list repo-landing__list--hero";

    heroFormShell.appendChild(formLead);
    heroFormShell.appendChild(form);
    heroFormShell.appendChild(formMeta);
    heroFormShell.appendChild(errorMsg);
    heroFormShell.appendChild(listContainer);

    heroCopy.appendChild(title);
    heroCopy.appendChild(heroFormShell);

    const heroPreview = document.createElement("div");
    heroPreview.className = "repo-landing__hero-preview";
    heroPreview.setAttribute("aria-hidden", "true");

    const previewFrame = document.createElement("div");
    previewFrame.className = "repo-landing__preview-frame";

    const previewTopbar = document.createElement("div");
    previewTopbar.className = "repo-landing__preview-topbar";
    previewTopbar.innerHTML = `
        <div class="repo-landing__preview-dots">
            <span></span><span></span><span></span>
        </div>
        <div class="repo-landing__preview-path">gitvista.io / repo / rybkr / gitvista</div>
    `;
    const previewPath = previewTopbar.querySelector(".repo-landing__preview-path");

    const previewBody = document.createElement("div");
    previewBody.className = "repo-landing__preview-body";

    const previewGraph = document.createElement("div");
    previewGraph.className = "repo-landing__preview-graph";
    previewGraph.dataset.loading = "true";
    previewGraph.dataset.state = "loading";
    previewGraph.innerHTML = `
        <div class="repo-landing__preview-graph-header"></div>
        <div class="repo-landing__preview-graph-shell">
            <div class="repo-landing__preview-graph-loading">
                <span class="repo-landing__preview-graph-spinner" aria-hidden="true"></span>
                <span class="repo-landing__preview-graph-loading-text">Loading graph...</span>
            </div>
        </div>
    `;
    const previewGraphHeader = previewGraph.querySelector(".repo-landing__preview-graph-header");
    const previewGraphShell = previewGraph.querySelector(".repo-landing__preview-graph-shell");
    const previewGraphLoadingText = previewGraph.querySelector(".repo-landing__preview-graph-loading-text");
    const previewGraphCanvas = document.createElement("div");
    previewGraphCanvas.className = "repo-landing__preview-graph-canvas";
    previewGraphShell.appendChild(previewGraphCanvas);
    ensurePreviewGraphController();

    const previewSidebar = document.createElement("div");
    previewSidebar.className = "repo-landing__preview-sidebar";
    previewSidebar.dataset.loading = "true";
    previewSidebar.dataset.state = "loading";

    const previewSidebarHeader = document.createElement("div");
    previewSidebarHeader.className = "repo-landing__preview-sidebar-header";
    previewSidebarHeader.innerHTML = `
        <div class="repo-landing__preview-eyebrow">Repository overview</div>
        <strong>GitVista</strong>
        <div class="repo-landing__preview-pills">
            <span class="repo-landing__preview-pill repo-landing__preview-pill--branch">main</span>
            <span class="repo-landing__preview-pill repo-landing__preview-pill--head">HEAD loading</span>
        </div>
    `;
    const previewRepoName = previewSidebarHeader.querySelector("strong");
    const previewBranchPill = previewSidebarHeader.querySelector(".repo-landing__preview-pill--branch");
    const previewHeadPill = previewSidebarHeader.querySelector(".repo-landing__preview-pill--head");

    const previewSidebarMeta = document.createElement("div");
    previewSidebarMeta.className = "repo-landing__preview-sidebar-meta";
    previewSidebarMeta.innerHTML = `
        <div><strong>Description</strong><span>Loading repository metadata...</span></div>
    `;
    const previewDescription = previewSidebarMeta.querySelector("span");

    const previewList = document.createElement("div");
    previewList.className = "repo-landing__preview-list";
    previewList.innerHTML = `
        <div class="repo-landing__preview-item">
            <strong>Commits</strong>
            <span>...</span>
        </div>
        <div class="repo-landing__preview-item">
            <strong>Branches</strong>
            <span>...</span>
        </div>
        <div class="repo-landing__preview-item">
            <strong>Tags</strong>
            <span>...</span>
        </div>
    `;
    const [previewCommitItem, previewBranchItem, previewTagItem] = previewList.querySelectorAll(".repo-landing__preview-item span");

    previewSidebar.appendChild(previewSidebarHeader);
    previewSidebar.appendChild(previewSidebarMeta);
    previewSidebar.appendChild(previewList);

    previewBody.appendChild(previewSidebar);
    previewBody.appendChild(previewGraph);
    previewFrame.appendChild(previewTopbar);
    previewFrame.appendChild(previewBody);
    heroPreview.appendChild(previewFrame);

    hero.appendChild(heroCopy);
    hero.appendChild(heroPreview);

    const proofStrip = document.createElement("section");
    proofStrip.className = "repo-landing__proof-strip";
    proofStrip.innerHTML = `
        <div class="repo-landing__proof-item"><strong>Instant orientation</strong><span>Graph-first context before you inspect individual commits.</span></div>
        <div class="repo-landing__proof-item"><strong>Example repos ready</strong><span>Jump into curated public repos without waiting on setup.</span></div>
        <div class="repo-landing__proof-item"><strong>Local mode available</strong><span>Track your own <code>.git</code> directory when browser mode is not enough.</span></div>
    `;
    const proofIntro = document.createElement("p");
    proofIntro.className = "repo-landing__section-subtitle repo-landing__proof-intro";
    proofIntro.textContent = tagline.textContent;
    proofStrip.prepend(proofIntro);

    // ── 2. Featured Repos ─────────────────────────────────────────────────

    const featuredSection = document.createElement("section");
    featuredSection.className = "repo-landing__section repo-landing__featured";
    featuredSection.id = "featured";

    const featuredEyebrow = document.createElement("p");
    featuredEyebrow.className = "repo-landing__eyebrow";
    featuredEyebrow.textContent = "Fast-start examples";

    const featuredTitle = document.createElement("h2");
    featuredTitle.className = "repo-landing__section-title";
    featuredTitle.textContent = "Open a live repository and inspect how the history is shaped.";

    const featuredSubtitle = document.createElement("p");
    featuredSubtitle.className = "repo-landing__section-subtitle";
    featuredSubtitle.textContent = "These repos are preloaded to make the first interaction immediate. Use them to see branch movement, merges, and commit-level diffs before pasting your own URL.";

    const featuredGrid = document.createElement("div");
    featuredGrid.className = "repo-landing__featured-grid";

    featuredSection.appendChild(featuredEyebrow);
    featuredSection.appendChild(featuredTitle);
    featuredSection.appendChild(featuredSubtitle);
    featuredSection.appendChild(featuredGrid);

    function renderFeaturedCard(entry) {
        const s = featuredState.get(entry.url) || { id: null, state: "pending", error: null, phase: "", percent: 0 };
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

        const cardMeta = document.createElement("p");
        cardMeta.className = "repo-landing__card-meta";
        cardMeta.textContent = s.state === "ready"
            ? "Ready now. Open the graph view and inspect commit flow."
            : "Preparing graph data and commit history.";

        const cardAction = document.createElement("div");
        cardAction.className = "repo-landing__card-action";

        if (s.state === "ready") {
            const cta = document.createElement("button");
            cta.className = "repo-landing__card-cta";
            cta.innerHTML = `Open Live View ${ARROW_SVG}`;
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
        } else if (s.state === "cloning" && s.percent > 0) {
            const progressContainer = document.createElement("div");
            progressContainer.className = "repo-landing__clone-progress";

            const progressText = document.createElement("span");
            progressText.className = "repo-landing__card-progress";
            progressText.textContent = `${s.phase} ${s.percent}%`;
            progressContainer.appendChild(progressText);

            const progressBar = document.createElement("div");
            progressBar.className = "repo-landing__progress-bar";
            const progressFill = document.createElement("div");
            progressFill.className = "repo-landing__progress-fill";
            progressFill.style.width = `${s.percent}%`;
            progressBar.appendChild(progressFill);
            progressContainer.appendChild(progressBar);

            cardAction.appendChild(progressContainer);
        } else {
            const progress = document.createElement("span");
            progress.className = "repo-landing__card-progress";
            progress.textContent = s.state === "cloning" ? "Cloning..." : "Loading...";
            cardAction.appendChild(progress);
        }

        card.appendChild(cardHeader);
        card.appendChild(cardDesc);
        card.appendChild(cardMeta);
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
        featuredState.set(entry.url, { id: null, state: "pending", error: null, phase: "", percent: 0 });
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

            featuredState.set(entry.url, { id: repo.id, state: repo.state, error: null, phase: repo.phase || "", percent: repo.percent || 0 });
            renderFeaturedCard(entry);

            if (repo.state === "cloning" || repo.state === "pending") {
                startProgressStream(repo.id, (update) => {
                    if (destroyed) return;
                    featuredState.set(entry.url, { id: update.id, state: update.state, error: update.error, phase: update.phase || "", percent: update.percent || 0 });
                    renderFeaturedCard(entry);
                });
            }
        } catch (err) {
            if (destroyed) return;
            featuredState.set(entry.url, { id: null, state: "error", error: err.message, phase: "", percent: 0 });
            renderFeaturedCard(entry);
        }
    }

    function initFeatured() {
        const promises = FEATURED_REPOS.map((entry) => initSingleFeatured(entry));
        Promise.allSettled(promises);
    }

    function setSnapshotMetricValues({ commits, branches, tags }) {
        previewCommitItem.textContent = commits;
        previewBranchItem.textContent = branches;
        previewTagItem.textContent = tags;
    }

    function setSnapshotLoadingState(message = "Loading graph...") {
        previewSidebar.dataset.loading = "true";
        previewSidebar.dataset.state = "loading";
        previewGraph.dataset.loading = "true";
        previewGraph.dataset.state = "loading";
        previewGraphLoadingText.textContent = message;
    }

    function applySnapshotPendingState(repo = {}, progress = null) {
        if (destroyed) return;
        const remotePath = deriveRemotePath({ remotes: { origin: repo.url || SNAPSHOT_REPO_URL } });
        const phase = progress?.phase || repo.phase || "Preparing repository";
        const percent = Number(progress?.percent ?? repo.percent ?? 0);
        const progressLabel = percent > 0 ? `${phase} ${percent}%` : phase;

        previewRepoName.textContent = "GitVista";
        previewBranchPill.textContent = progressLabel;
        previewHeadPill.textContent = repo.state === "pending" ? "Queued" : "Loading";
        previewDescription.textContent = "Preparing the live GitVista snapshot for the landing page preview.";
        setSnapshotMetricValues({
            commits: percent > 0 ? `${percent}% cloned` : "Scanning history",
            branches: "Detecting refs",
            tags: "Detecting tags",
        });
        previewPath.textContent = `gitvista.io / repo / ${remotePath}`;
        setSnapshotLoadingState(progressLabel);
    }

    function setSnapshotErrorState(message = "GitVista snapshot is unavailable right now.") {
        if (destroyed) return;
        previewRepoName.textContent = "GitVista";
        previewBranchPill.textContent = "Snapshot unavailable";
        previewHeadPill.textContent = "Retry later";
        previewDescription.textContent = message;
        setSnapshotMetricValues({
            commits: "Unavailable",
            branches: "Unavailable",
            tags: "Unavailable",
        });
        previewSidebar.dataset.loading = "false";
        previewSidebar.dataset.state = "error";
        previewGraph.dataset.loading = "true";
        previewGraph.dataset.state = "error";
        previewGraphLoadingText.textContent = message;
    }

    function applySnapshotOverview(data) {
        if (!data || destroyed) return;
        const repoName = deriveRepoName(data);
        const branch = data.headDetached ? "Detached HEAD" : (formatBranchName(data.currentBranch) || "No branch");
        const head = typeof data.headHash === "string" && data.headHash ? data.headHash.slice(0, 7) : "unknown";
        const description = (data.description || "").trim() || "Git history visualization.";
        const remotePath = deriveRemotePath(data.remotes);

        previewRepoName.textContent = repoName;
        previewBranchPill.textContent = branch;
        previewHeadPill.textContent = `HEAD ${head}`;
        previewDescription.textContent = description;
        setSnapshotMetricValues({
            commits: `${Number(data.commitCount || 0).toLocaleString()} total`,
            branches: `${Number(data.branchCount || 0)} branches`,
            tags: `${Number(data.tagCount || 0)} tags`,
        });
        previewPath.textContent = `gitvista.io / repo / ${remotePath}`;
        previewSidebar.dataset.loading = "false";
        previewSidebar.dataset.state = "ready";
    }

    function buildSnapshotPreviewSummary(summary, limit = 8) {
        const visibleHashes = buildSnapshotCommitHashes(summary, limit);
        const visibleHashSet = new Set(visibleHashes);
        return {
            headHash: summary.headHash || visibleHashes[0] || "",
            branches: Object.fromEntries(
                Object.entries(summary.branches || {}).filter(([, hash]) => visibleHashSet.has(hash))
            ),
            tags: {},
            stashes: [],
            skeleton: (summary.skeleton || [])
                .filter((commit) => visibleHashSet.has(commit.h))
                .map((commit) => ({
                    h: commit.h,
                    p: (commit.p || []).filter((parentHash) => visibleHashSet.has(parentHash)),
                    t: commit.t,
                    branchLabel: commit.branchLabel || "",
                    branchLabelSource: commit.branchLabelSource || "",
                })),
        };
    }

    function ensurePreviewGraphController() {
        if (previewGraphController || destroyed) return previewGraphController;
        previewGraphController = createGraphController(previewGraphCanvas, {
            centerAnchorYFraction: 0.25,
            detailThresholds: {
                message: 1,
                author: 1,
                date: 1,
            },
            initialLayoutMode: "lane",
            showControls: false,
            showRefDecorators: false,
            fetchGraphCommits: async (hashes) => hashes
                .map((hash) => previewGraphCommitMap.get(hash))
                .filter(Boolean),
        });
        return previewGraphController;
    }

    function renderSnapshotGraph(summary, commits = []) {
        if (!summary?.skeleton?.length) {
            setSnapshotErrorState("GitVista snapshot loaded without graph data.");
            return;
        }
        previewGraphCommitMap = new Map(commits.map((commit) => [commit.hash, commit]));
        const controller = ensurePreviewGraphController();
        const previewSummary = buildSnapshotPreviewSummary(summary, 8);
        previewGraphHeader.innerHTML = "";
        previewGraph.dataset.loading = "false";
        previewGraph.dataset.state = "ready";
        controller?.applySummary(previewSummary);
        controller?.refreshViewport?.();
        controller?.centerOnCommit(previewSummary.headHash || null);
        window.requestAnimationFrame(() => {
            if (destroyed || !previewGraphController) return;
            previewGraphController.refreshViewport?.();
            previewGraphController.centerOnCommit(previewSummary.headHash || null);
        });
    }

    async function fetchSnapshotCommits(id, hashes) {
        const unique = [...new Set((hashes || []).filter((hash) => typeof hash === "string" && hash.length > 0))];
        if (unique.length === 0) return [];
        const params = new URLSearchParams();
        params.set("hashes", unique.join(","));
        const resp = await fetch(`/api/repos/${id}/graph/commits?${params.toString()}`);
        if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
        const payload = await resp.json();
        return Array.isArray(payload?.commits) ? payload.commits : [];
    }

    async function fetchSnapshotData(id) {
        const [repoResp, graphResp] = await Promise.all([
            fetch(`/api/repos/${id}/repository`),
            fetch(`/api/repos/${id}/graph/summary`),
        ]);
        if (!repoResp.ok) throw new Error(`HTTP ${repoResp.status}`);
        if (!graphResp.ok) throw new Error(`HTTP ${graphResp.status}`);
        const [repoData, graphData] = await Promise.all([repoResp.json(), graphResp.json()]);
        const visibleHashes = buildSnapshotCommitHashes(graphData, 8);
        const graphCommits = await fetchSnapshotCommits(id, visibleHashes).catch(() => []);
        applySnapshotOverview(repoData);
        renderSnapshotGraph(graphData, graphCommits);
    }

    async function fetchSnapshotDataWithRetry(id, attempts = 6) {
        let lastError = null;
        for (let attempt = 0; attempt < attempts; attempt++) {
            try {
                await fetchSnapshotData(id);
                return;
            } catch (error) {
                lastError = error;
                if (attempt >= attempts - 1) break;
                await new Promise((resolve) => window.setTimeout(resolve, 250 * (attempt + 1)));
            }
        }
        throw lastError || new Error("Snapshot data unavailable");
    }

    async function ensureSnapshotRepo(existingRepos = null) {
        const knownRepos = Array.isArray(existingRepos) ? existingRepos : repos;
        const existing = knownRepos.find((repo) => normalizeRepoUrl(repo.url) === normalizeRepoUrl(SNAPSHOT_REPO_URL));

        if (existing) {
            snapshotRepoId = existing.id;
            addOrUpdateRepo(existing);
            if (existing.state === "ready") {
                await fetchSnapshotDataWithRetry(existing.id).catch(() => {
                    setSnapshotErrorState("GitVista snapshot stats could not be loaded.");
                });
                return;
            }
            if (existing.state === "cloning" || existing.state === "pending") {
                applySnapshotPendingState(existing);
                startProgressStream(existing.id, async (update) => {
                    if (destroyed) return;
                    addOrUpdateRepo({ ...existing, ...update, url: existing.url });
                    applySnapshotPendingState(existing, update);
                    if (update.state === "ready") {
                        await fetchSnapshotDataWithRetry(existing.id).catch(() => {
                            setSnapshotErrorState("GitVista snapshot stats could not be loaded.");
                        });
                    }
                });
                return;
            }
        }

        const resp = await fetch("/api/repos", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ url: SNAPSHOT_REPO_URL }),
        });
        if (!resp.ok) return;

        const repo = await resp.json();
        if (destroyed) return;
        snapshotRepoId = repo.id;
        addOrUpdateRepo(repo);

        if (repo.state === "ready") {
            await fetchSnapshotDataWithRetry(repo.id).catch(() => {
                setSnapshotErrorState("GitVista snapshot stats could not be loaded.");
            });
            return;
        }

        if (repo.state === "cloning" || repo.state === "pending") {
            applySnapshotPendingState(repo);
            startProgressStream(repo.id, async (update) => {
                if (destroyed) return;
                addOrUpdateRepo({ ...repo, ...update, url: SNAPSHOT_REPO_URL });
                applySnapshotPendingState(repo, update);
                if (update.state === "ready") {
                    await fetchSnapshotDataWithRetry(repo.id).catch(() => {
                        setSnapshotErrorState("GitVista snapshot stats could not be loaded.");
                    });
                }
            });
            return;
        }

        setSnapshotErrorState("GitVista snapshot is not ready yet.");
    }

    function initSnapshotRepo(existingRepos = null) {
        if (!snapshotInitPromise) {
            setSnapshotLoadingState("Loading live GitVista snapshot...");
            snapshotInitPromise = ensureSnapshotRepo(existingRepos).catch(() => {
                setSnapshotErrorState("GitVista snapshot could not be initialized.");
            });
        }
        return snapshotInitPromise;
    }

    form.addEventListener("submit", async (e) => {
        e.preventDefault();
        errorMsg.textContent = "";
        const url = input.value.trim();
        if (!url) return;

        const featuredEntry = FEATURED_REPOS.find((f) => f.url === url || f.url === url.replace(/\/$/, ""));
        if (featuredEntry && featuredState.has(featuredEntry.url)) {
            const card = featuredGrid.querySelector(`[data-url="${featuredEntry.url}"]`);
            if (card) {
                scrollToSection(featuredSection);
                highlightElementTemporarily(card);
                input.value = "";
                return;
            }
        }

        addBtn.disabled = true;
        addBtn.textContent = "Opening...";

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

            for (const [fUrl, fState] of featuredState) {
                if (fState.id === repo.id) {
                    const card = featuredGrid.querySelector(`[data-url="${fUrl}"]`);
                    if (card) {
                        scrollToSection(featuredSection);
                        highlightElementTemporarily(card);
                    }
                    return;
                }
            }

            addOrUpdateRepo(repo);
            renderUserRepos();
            scrollToSection(heroFormShell);

            if (repo.state === "cloning" || repo.state === "pending") {
                startProgressStream(repo.id, (update) => {
                    if (destroyed) return;
                    addOrUpdateRepo({ ...update, url: repo.url });
                    renderUserRepos();
                });
            }
        } catch (err) {
            errorMsg.textContent = err.message;
        } finally {
            addBtn.disabled = false;
            addBtn.textContent = "Open Repository";
        }
    });

    async function deleteRepo(id) {
        try {
            await fetch(`/api/repos/${id}`, { method: "DELETE" });
            repos = repos.filter((r) => r.id !== id);
            if (activeStreams.has(id)) {
                activeStreams.get(id).close();
                activeStreams.delete(id);
            }
            renderUserRepos();
        } catch {
            // Ignore delete failures
        }
    }

    function renderUserRepos() {
        listContainer.innerHTML = "";

        const featuredIds = new Set();
        for (const s of featuredState.values()) {
            if (s.id) featuredIds.add(s.id);
        }
        const userRepos = repos.filter((r) => !featuredIds.has(r.id) && r.id !== snapshotRepoId);

        if (userRepos.length === 0) return;

        const heading = document.createElement("h3");
        heading.className = "repo-landing__list-heading";
        heading.textContent = "Your recent repositories";
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

            if (repo.state === "cloning" && repo.percent > 0) {
                const progressContainer = document.createElement("div");
                progressContainer.className = "repo-landing__clone-progress";

                const progressText = document.createElement("span");
                progressText.className = "repo-landing__card-progress";
                progressText.textContent = `${repo.phase} ${repo.percent}%`;
                progressContainer.appendChild(progressText);

                const progressBar = document.createElement("div");
                progressBar.className = "repo-landing__progress-bar";
                const progressFill = document.createElement("div");
                progressFill.className = "repo-landing__progress-fill";
                progressFill.style.width = `${repo.percent}%`;
                progressBar.appendChild(progressFill);
                progressContainer.appendChild(progressBar);

                info.appendChild(progressContainer);
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

    // ── 3. Install Locally ────────────────────────────────────────────────

    const installSection = document.createElement("section");
    installSection.className = "repo-landing__section repo-landing__install";
    installSection.id = "local";

    const installEyebrow = document.createElement("p");
    installEyebrow.className = "repo-landing__eyebrow";
    installEyebrow.textContent = "Local mode";

    const installTitle = document.createElement("h2");
    installTitle.className = "repo-landing__section-title";
    installTitle.textContent = "Run GitVista beside your repository when you need live local state.";

    const installSubtitle = document.createElement("p");
    installSubtitle.className = "repo-landing__section-subtitle";
    installSubtitle.textContent = "Browser mode is the fastest way to inspect a public repo. Local mode connects directly to your .git directory so new commits, staged changes, and diff views update the moment you make them.";

    const codeBlock = document.createElement("div");
    codeBlock.className = "repo-landing__code-block";

    const codeText = document.createElement("code");
    codeText.textContent = "go install github.com/rybkr/gitvista/cmd/vista@latest && vista";

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

    installSection.appendChild(installEyebrow);
    installSection.appendChild(installTitle);
    installSection.appendChild(installSubtitle);
    installSection.appendChild(codeBlock);

    // ── 4. Footer ─────────────────────────────────────────────────────────

    const footer = document.createElement("footer");
    footer.className = "repo-landing__footer";

    const footerBrand = document.createElement("div");
    footerBrand.className = "repo-landing__footer-brand";
    footerBrand.innerHTML = `<strong>GitVista</strong><span>See what Git is actually doing.</span>`;

    const footerLinks = document.createElement("div");
    footerLinks.className = "repo-landing__footer-links";

    const ghLink = document.createElement("a");
    ghLink.className = "repo-landing__footer-link";
    ghLink.href = "https://github.com/rybkr/gitvista";
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

    footer.appendChild(footerBrand);
    footer.appendChild(footerLinks);
    footer.appendChild(license);

    // ── Assemble ──────────────────────────────────────────────────────────

    chrome.appendChild(topbar);

    content.appendChild(hero);
    content.appendChild(proofStrip);
    content.appendChild(featuredSection);
    content.appendChild(installSection);
    content.appendChild(footer);

    el.appendChild(chrome);
    el.appendChild(content);

    // ── Initialize ────────────────────────────────────────────────────────

    initFeatured();

    (async () => {
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
            initSnapshotRepo(list);
            renderUserRepos();
        } catch {
            // Silently fail initial load
            initSnapshotRepo();
        }
    })();

    function destroy() {
        destroyed = true;
        window.clearTimeout(highlightTimer);
        previewGraphController?.destroy?.();
        previewGraphController = null;
        for (const [, es] of activeStreams) {
            es.close();
        }
        activeStreams.clear();
    }

    return { el, destroy };
}

const GITHUB_SVG = `<svg width="18" height="18" viewBox="0 0 16 16" fill="currentColor">
    <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/>
</svg>`;

const SECTIONS = [
    {
        id: "hosted",
        label: "Hosted Mode",
        title: "Open a public GitHub repository directly in the browser.",
        body: "Hosted mode is the fastest path into GitVista. Paste a public GitHub URL on the landing page and GitVista prepares a repository-backed workspace with the commit graph, repository overview, and diff views already wired up.",
        points: [
            "Best for quick inspection, demos, and sharing a repository view without asking someone to install anything.",
            "Preloaded examples are there to show the graph immediately while the rest of the product remains repo-accurate.",
            "If a repository is still being prepared, GitVista streams progress until the graph is ready.",
        ],
    },
    {
        id: "local",
        label: "Local Mode",
        title: "Run beside your checkout when you need zero-latency state.",
        body: "Local mode connects GitVista to your own repository so branch movement, staged changes, and diffs reflect what you are doing on disk. This is the right mode when you are actively working, not just inspecting history.",
        points: [
            "Install with `go install github.com/rybkr/gitvista/cmd/vista@latest && vista`.",
            "Use local mode when you care about staged changes, immediate refresh, and your unpushed work.",
            "The browser UI stays the same, but the data source is your local `.git` directory.",
        ],
    },
    {
        id: "views",
        label: "Views",
        title: "Read the repository from graph to diff without changing tools.",
        body: "GitVista is organized around orientation first. The graph gives branch shape, the repository overview summarizes the current state, and the commit and diff views let you drill down without losing context.",
        points: [
            "Graph view shows commit flow, merges, and branch movement.",
            "Repository overview highlights branch, HEAD, remotes, description, and recent tags.",
            "Diff and file views let you move from topology to exact file changes in one session.",
        ],
    },
    {
        id: "limits",
        label: "Limits",
        title: "Know the edges before you depend on them.",
        body: "GitVista is built to make Git behavior legible, but it still inherits the realities of repository size, remote accessibility, and the distinction between hosted and local execution.",
        points: [
            "Hosted mode is aimed at public GitHub repositories.",
            "Very large histories can take longer to prepare before the graph is available.",
            "For private repositories or sensitive local work, use local mode instead of expecting the hosted path to cover everything.",
        ],
    },
];

export function createDocsView() {
    const el = document.createElement("div");
    el.className = "repo-docs";

    const chrome = document.createElement("div");
    chrome.className = "repo-docs__chrome";

    const topbar = document.createElement("header");
    topbar.className = "repo-landing__topbar";

    const topbarNav = document.createElement("nav");
    topbarNav.className = "repo-landing__topbar-nav";
    topbarNav.setAttribute("aria-label", "Primary");

    const brand = document.createElement("a");
    brand.className = "repo-landing__brand";
    brand.href = "#";
    brand.setAttribute("aria-label", "GitVista home");

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

    const topbarLinks = document.createElement("div");
    topbarLinks.className = "repo-landing__topbar-links";

    const homeLink = document.createElement("a");
    homeLink.className = "repo-landing__nav-link";
    homeLink.href = "#";
    homeLink.textContent = "Home";

    const docsLink = document.createElement("a");
    docsLink.className = "repo-landing__nav-link repo-landing__nav-link--active";
    docsLink.href = "#docs";
    docsLink.textContent = "Docs";
    docsLink.setAttribute("aria-current", "page");

    topbarLinks.appendChild(homeLink);
    topbarLinks.appendChild(docsLink);

    topbarNav.appendChild(brand);
    topbarNav.appendChild(topbarLinks);
    topbar.appendChild(topbarNav);
    chrome.appendChild(topbar);

    const content = document.createElement("div");
    content.className = "repo-docs__content";

    const hero = document.createElement("section");
    hero.className = "repo-docs__hero";
    hero.innerHTML = `
        <div class="repo-docs__hero-copy">
            <p class="repo-docs__eyebrow">Product Docs</p>
            <h1 class="repo-docs__title">Start fast, understand the edges, and know when to switch modes.</h1>
            <p class="repo-docs__lede">GitVista is easiest to trust when the operating model is explicit. These docs cover how hosted mode behaves, when local mode is the better fit, and what the core views are designed to show.</p>
            <div class="repo-docs__hero-actions">
                <a class="repo-docs__primary-link" href="#hosted">Read the workflow</a>
                <a class="repo-docs__secondary-link" href="#">Open the landing page</a>
            </div>
        </div>
        <aside class="repo-docs__hero-panel" aria-label="Docs summary">
            <div class="repo-docs__panel-kicker">At a glance</div>
            <div class="repo-docs__panel-grid">
                <div><strong>Hosted</strong><span>Public GitHub repos in browser</span></div>
                <div><strong>Local</strong><span>Directly against your checkout</span></div>
                <div><strong>Views</strong><span>Graph, overview, commit, diff</span></div>
                <div><strong>Best use</strong><span>Orientation before file-level inspection</span></div>
            </div>
        </aside>
    `;

    const sectionRail = document.createElement("aside");
    sectionRail.className = "repo-docs__rail";
    sectionRail.setAttribute("aria-label", "Doc sections");

    const railLabel = document.createElement("p");
    railLabel.className = "repo-docs__rail-label";
    railLabel.textContent = "Sections";
    sectionRail.appendChild(railLabel);

    for (const section of SECTIONS) {
        const link = document.createElement("a");
        link.className = "repo-docs__rail-link";
        link.href = `#${section.id}`;
        link.textContent = section.label;
        sectionRail.appendChild(link);
    }

    const sections = document.createElement("div");
    sections.className = "repo-docs__sections";

    for (const section of SECTIONS) {
        const article = document.createElement("section");
        article.className = "repo-docs__section";
        article.id = section.id;

        const label = document.createElement("p");
        label.className = "repo-docs__section-label";
        label.textContent = section.label;

        const title = document.createElement("h2");
        title.className = "repo-docs__section-title";
        title.textContent = section.title;

        const body = document.createElement("p");
        body.className = "repo-docs__section-body";
        body.textContent = section.body;

        const list = document.createElement("ul");
        list.className = "repo-docs__point-list";

        for (const point of section.points) {
            const item = document.createElement("li");
            item.className = "repo-docs__point";
            item.textContent = point;
            list.appendChild(item);
        }

        article.appendChild(label);
        article.appendChild(title);
        article.appendChild(body);
        article.appendChild(list);
        sections.appendChild(article);
    }

    const help = document.createElement("section");
    help.className = "repo-docs__help";
    help.innerHTML = `
        <div>
            <p class="repo-docs__section-label">Need More</p>
            <h2 class="repo-docs__section-title">Use the product, then come back here when you hit a real question.</h2>
            <p class="repo-docs__section-body">This page is meant to remove ambiguity, not bury the product in prose. Start with a public repository from the landing page, then switch to local mode when you need live repository state.</p>
        </div>
        <div class="repo-docs__help-actions">
            <a class="repo-docs__primary-link" href="#">Open GitVista</a>
            <a class="repo-landing__footer-link" href="https://github.com/rybkr/gitvista" target="_blank" rel="noopener noreferrer">${GITHUB_SVG} GitHub</a>
        </div>
    `;

    content.appendChild(hero);
    content.appendChild(sectionRail);
    content.appendChild(sections);
    content.appendChild(help);

    el.appendChild(chrome);
    el.appendChild(content);

    return {
        el,
        destroy() {},
    };
}

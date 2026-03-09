const GITHUB_SVG = `<svg width="18" height="18" viewBox="0 0 16 16" fill="currentColor">
    <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/>
</svg>`;

export function createDocsView({ navigateToPath } = {}) {
    const controller = new AbortController();
    const el = document.createElement("div");
    el.className = "repo-docs";

    const chrome = document.createElement("div");
    chrome.className = "repo-docs__chrome";
    chrome.appendChild(createTopbar(bindHostedNavigation));

    const content = document.createElement("div");
    content.className = "repo-docs__content";

    const state = document.createElement("section");
    state.className = "repo-docs__state";
    state.textContent = "Loading docs…";
    content.appendChild(state);

    el.appendChild(chrome);
    el.appendChild(content);

    loadDocs();

    function bindHostedNavigation(link, path) {
        link.href = path;
        if (path.startsWith("#")) {
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
                const target = el.querySelector(path);
                if (!target) return;
                target.scrollIntoView({ behavior: "smooth", block: "start" });
                history.replaceState(null, "", path);
            });
            return;
        }

        if (typeof navigateToPath !== "function" || !path.startsWith("/")) return;
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
            navigateToPath(path);
        });
    }

    async function loadDocs() {
        try {
            const resp = await fetch("/api/docs", { signal: controller.signal });
            if (!resp.ok) throw new Error(`docs request failed: ${resp.status}`);
            const docs = await resp.json();
            if (controller.signal.aborted) return;
            renderDocs(content, docs, bindHostedNavigation);
        } catch (error) {
            if (controller.signal.aborted) return;
            state.className = "repo-docs__state repo-docs__state--error";
            state.textContent = "Docs are unavailable right now.";
            console.error(error);
        }
    }

    return {
        el,
        destroy() {
            controller.abort();
        },
    };
}

function createTopbar(bindHostedNavigation) {
    const topbar = document.createElement("header");
    topbar.className = "repo-landing__topbar";

    const topbarNav = document.createElement("nav");
    topbarNav.className = "repo-landing__topbar-nav";
    topbarNav.setAttribute("aria-label", "Primary");

    const brand = document.createElement("a");
    brand.className = "repo-landing__brand";
    brand.setAttribute("aria-label", "GitVista home");
    bindHostedNavigation(brand, "/");

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
    homeLink.textContent = "Home";
    bindHostedNavigation(homeLink, "/");

    const docsLink = document.createElement("a");
    docsLink.className = "repo-landing__nav-link repo-landing__nav-link--active";
    docsLink.textContent = "Docs";
    docsLink.setAttribute("aria-current", "page");
    bindHostedNavigation(docsLink, "/docs");

    topbarLinks.appendChild(homeLink);
    topbarLinks.appendChild(docsLink);
    topbarNav.appendChild(brand);
    topbarNav.appendChild(topbarLinks);
    topbar.appendChild(topbarNav);

    return topbar;
}

function renderDocs(content, docs, bindHostedNavigation) {
    content.innerHTML = "";

    const hero = document.createElement("section");
    hero.className = "repo-docs__hero";

    const heroCopy = document.createElement("div");
    heroCopy.className = "repo-docs__hero-copy";

    const eyebrow = document.createElement("p");
    eyebrow.className = "repo-docs__eyebrow";
    eyebrow.textContent = docs.eyebrow || "Product Docs";

    const title = document.createElement("h1");
    title.className = "repo-docs__title";
    title.textContent = docs.title || "GitVista Docs";

    const lede = document.createElement("p");
    lede.className = "repo-docs__lede";
    lede.textContent = docs.lede || "";

    const heroActions = document.createElement("div");
    heroActions.className = "repo-docs__hero-actions";
    const primaryLink = createDocsLink(docs.primaryCta, "repo-docs__primary-link", bindHostedNavigation);
    const secondaryLink = createDocsLink(docs.secondaryCta, "repo-docs__secondary-link", bindHostedNavigation);
    if (primaryLink) heroActions.appendChild(primaryLink);
    if (secondaryLink) heroActions.appendChild(secondaryLink);

    heroCopy.appendChild(eyebrow);
    heroCopy.appendChild(title);
    heroCopy.appendChild(lede);
    heroCopy.appendChild(heroActions);

    const heroPanel = document.createElement("aside");
    heroPanel.className = "repo-docs__hero-panel";
    heroPanel.setAttribute("aria-label", "Docs summary");

    const panelLabel = document.createElement("div");
    panelLabel.className = "repo-docs__panel-kicker";
    panelLabel.textContent = "At a glance";
    heroPanel.appendChild(panelLabel);

    const panelGrid = document.createElement("div");
    panelGrid.className = "repo-docs__panel-grid";
    for (const item of docs.summary || []) {
        const card = document.createElement("div");
        const strong = document.createElement("strong");
        strong.textContent = item.label;
        const span = document.createElement("span");
        span.textContent = item.value;
        card.appendChild(strong);
        card.appendChild(span);
        panelGrid.appendChild(card);
    }
    heroPanel.appendChild(panelGrid);

    hero.appendChild(heroCopy);
    hero.appendChild(heroPanel);

    const rail = document.createElement("aside");
    rail.className = "repo-docs__rail";
    rail.setAttribute("aria-label", "Doc sections");

    const railLabel = document.createElement("p");
    railLabel.className = "repo-docs__rail-label";
    railLabel.textContent = "Sections";
    rail.appendChild(railLabel);

    const sections = document.createElement("div");
    sections.className = "repo-docs__sections";

    for (const section of docs.sections || []) {
        const link = document.createElement("a");
        link.className = "repo-docs__rail-link";
        link.href = `#${section.id}`;
        link.textContent = section.label;
        rail.appendChild(link);

        const article = document.createElement("section");
        article.className = "repo-docs__section";
        article.id = section.id;

        const label = document.createElement("p");
        label.className = "repo-docs__section-label";
        label.textContent = section.label;

        const heading = document.createElement("h2");
        heading.className = "repo-docs__section-title";
        heading.textContent = section.title;

        const body = document.createElement("div");
        body.className = "repo-docs__section-body";
        renderMarkdown(body, section.content || "");

        article.appendChild(label);
        article.appendChild(heading);
        article.appendChild(body);
        sections.appendChild(article);
    }

    const help = document.createElement("section");
    help.className = "repo-docs__help";

    const helpCopy = document.createElement("div");
    const helpLabel = document.createElement("p");
    helpLabel.className = "repo-docs__section-label";
    helpLabel.textContent = docs.help?.label || "Need More";
    const helpTitle = document.createElement("h2");
    helpTitle.className = "repo-docs__section-title";
    helpTitle.textContent = docs.help?.title || "";
    const helpBody = document.createElement("p");
    helpBody.className = "repo-docs__section-body";
    helpBody.textContent = docs.help?.body || "";
    helpCopy.appendChild(helpLabel);
    helpCopy.appendChild(helpTitle);
    helpCopy.appendChild(helpBody);

    const helpActions = document.createElement("div");
    helpActions.className = "repo-docs__help-actions";
    const helpPrimaryLink = createDocsLink(docs.help?.primaryCta, "repo-docs__primary-link", bindHostedNavigation);
    if (helpPrimaryLink) helpActions.appendChild(helpPrimaryLink);

    const githubLink = document.createElement("a");
    githubLink.className = "repo-landing__footer-link";
    githubLink.href = "https://github.com/rybkr/gitvista";
    githubLink.target = "_blank";
    githubLink.rel = "noopener noreferrer";
    githubLink.innerHTML = `${GITHUB_SVG} GitHub`;
    helpActions.appendChild(githubLink);

    help.appendChild(helpCopy);
    help.appendChild(helpActions);

    content.appendChild(hero);
    content.appendChild(rail);
    content.appendChild(sections);
    content.appendChild(help);
}

function createDocsLink(cta, className, bindHostedNavigation) {
    if (!cta?.label || !cta?.href) return null;
    const link = document.createElement("a");
    link.className = className;
    link.textContent = cta.label;
    bindHostedNavigation(link, cta.href);
    return link;
}

function renderMarkdown(container, markdown) {
    const blocks = String(markdown)
        .trim()
        .split(/\n\s*\n/)
        .map((block) => block.trim())
        .filter(Boolean);

    for (const block of blocks) {
        const lines = block.split("\n").map((line) => line.trim()).filter(Boolean);
        if (lines.length > 0 && lines.every((line) => line.startsWith("- "))) {
            const list = document.createElement("ul");
            list.className = "repo-docs__point-list";
            for (const line of lines) {
                const item = document.createElement("li");
                item.className = "repo-docs__point";
                appendInlineContent(item, line.slice(2));
                list.appendChild(item);
            }
            container.appendChild(list);
            continue;
        }

        const paragraph = document.createElement("p");
        paragraph.className = "repo-docs__copy";
        appendInlineContent(paragraph, lines.join(" "));
        container.appendChild(paragraph);
    }
}

function appendInlineContent(parent, text) {
    const parts = String(text).split(/(`[^`]+`)/g).filter(Boolean);
    for (const part of parts) {
        if (part.startsWith("`") && part.endsWith("`")) {
            const code = document.createElement("code");
            code.textContent = part.slice(1, -1);
            parent.appendChild(code);
        } else {
            parent.appendChild(document.createTextNode(part));
        }
    }
}

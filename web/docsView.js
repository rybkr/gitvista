import { createHostedFooter, createHostedTopbar } from "./hostedChrome.js";
import { PRODUCT_INFO } from "./hostedProduct.js";
import { bindHostedPathNavigation } from "./hostedNavigation.js";

function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

export function createDocsView({ navigateToPath, activeSection = null } = {}) {
    const controller = new AbortController();
    const el = createElement("div", "repo-docs");
    const chrome = createElement("div", "repo-docs__chrome");
    chrome.appendChild(createHostedTopbar({
        activePath: "/docs",
        navigateToPath,
        navItems: [
            { label: "Home", path: "/" },
            { label: "Docs", path: "/docs" },
        ],
    }));

    const content = createElement("div", "repo-docs__content");
    const state = createElement("section", "repo-docs__state", "Loading docs…");
    content.appendChild(state);

    el.appendChild(chrome);
    el.appendChild(content);
    loadDocs();

    async function loadDocs() {
        try {
            const resp = await fetch("/api/docs", { signal: controller.signal });
            if (!resp.ok) throw new Error(`docs request failed: ${resp.status}`);
            const docs = await resp.json();
            if (controller.signal.aborted) return;
            renderDocs(content, docs, navigateToPath, activeSection);
        } catch (error) {
            if (controller.signal.aborted) return;
            state.className = "repo-docs__state repo-docs__state--error";
            state.textContent = "Docs are unavailable right now.";
            content.appendChild(createHostedFooter());
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

function renderDocs(content, docs, navigateToPath, activeSection) {
    content.replaceChildren();

    const selectedSection = activeSection
        ? (docs.sections || []).find((section) => section.id === activeSection) || null
        : null;

    if (activeSection && !selectedSection) {
        content.appendChild(renderDocsNotFound(navigateToPath));
        content.appendChild(createHostedFooter());
        return;
    }

    const hero = createElement("section", activeSection ? "repo-docs__hero" : "repo-docs__hero repo-docs__hero--overview");
    const heroCopy = createElement("div", "repo-docs__hero-copy");
    heroCopy.appendChild(createElement("p", "repo-docs__eyebrow", activeSection ? "Docs Section" : (docs.eyebrow || "Product Docs")));
    heroCopy.appendChild(createElement("h1", "repo-docs__title", activeSection ? selectedSection.title : (docs.title || `${PRODUCT_INFO.name} Docs`)));
    heroCopy.appendChild(createElement("p", "repo-docs__lede", activeSection ? `Part of ${PRODUCT_INFO.name} Docs.` : (docs.lede || "")));

    if (activeSection) {
        const context = createElement("div", "repo-docs__hero-meta");
        const overviewLink = createElement("a", "repo-docs__context-link", "Back to docs overview");
        bindHostedPathNavigation(overviewLink, "/docs", navigateToPath);
        context.appendChild(overviewLink);
        heroCopy.appendChild(context);
    }

    hero.appendChild(heroCopy);

    if (activeSection) {
        const heroPanel = createElement("aside", "repo-docs__hero-panel");
        heroPanel.setAttribute("aria-label", "Current docs section");
        heroPanel.appendChild(createElement("div", "repo-docs__panel-kicker", "Current section"));

        const panelGrid = createElement("div", "repo-docs__panel-grid");
        const panelItems = [
            { label: "Section", value: selectedSection.label },
            { label: "Path", value: getDocsSectionPath(selectedSection.id) },
            { label: "Docs Home", value: "/docs" },
        ];

        for (const item of panelItems) {
            const card = document.createElement("div");
            card.appendChild(createElement("strong", "", item.label));
            card.appendChild(createElement("span", "", item.value));
            panelGrid.appendChild(card);
        }
        heroPanel.appendChild(panelGrid);
        hero.appendChild(heroPanel);
    }

    const rail = createElement("aside", "repo-docs__rail");
    rail.setAttribute("aria-label", "Doc sections");
    rail.appendChild(createElement("p", "repo-docs__rail-label", "Sections"));

    const overviewLink = createElement("a", "repo-docs__rail-link", "Overview");
    if (!activeSection) overviewLink.setAttribute("aria-current", "page");
    bindHostedPathNavigation(overviewLink, "/docs", navigateToPath);
    rail.appendChild(overviewLink);

    const sections = createElement("div", activeSection ? "repo-docs__sections repo-docs__sections--single" : "repo-docs__sections repo-docs__sections--overview");
    for (const section of docs.sections || []) {
        const railLink = createElement("a", "repo-docs__rail-link", section.label);
        if (section.id === activeSection) railLink.setAttribute("aria-current", "page");
        bindHostedPathNavigation(railLink, getDocsSectionPath(section.id), navigateToPath);
        rail.appendChild(railLink);

        if (!activeSection || section.id === activeSection) {
            sections.appendChild(activeSection ? renderDocsSection(section) : renderDocsSectionCard(section, navigateToPath));
        }
    }

    const help = createElement("section", "repo-docs__help");
    const helpCopy = document.createElement("div");
    helpCopy.appendChild(createElement("p", "repo-docs__section-label", docs.help?.label || "Need More"));
    helpCopy.appendChild(createElement("h2", "repo-docs__section-title", docs.help?.title || ""));
    helpCopy.appendChild(createElement("p", "repo-docs__section-body", docs.help?.body || ""));
    const helpMeta = createElement("div", "repo-docs__help-meta");
    const githubLink = createElement("a", "repo-docs__context-link", "Project on GitHub");
    githubLink.href = PRODUCT_INFO.repositoryUrl;
    githubLink.target = "_blank";
    githubLink.rel = "noopener noreferrer";
    helpMeta.appendChild(githubLink);

    if (docs.help?.primaryCta?.label && docs.help?.primaryCta?.href) {
        const helpLink = createElement("a", "repo-docs__context-link", docs.help.primaryCta.label);
        bindHostedPathNavigation(helpLink, resolveDocsHref(docs.help.primaryCta.href), navigateToPath);
        helpMeta.appendChild(helpLink);
    }

    help.appendChild(helpCopy);
    help.appendChild(helpMeta);

    content.appendChild(hero);
    content.appendChild(rail);
    content.appendChild(sections);
    content.appendChild(help);
    content.appendChild(createHostedFooter());
}

function renderDocsSection(section, { headingTag = "h2" } = {}) {
    const article = createElement("section", "repo-docs__section");
    article.id = section.id;
    article.appendChild(createElement("p", "repo-docs__section-label", section.label));
    article.appendChild(createElement(headingTag, "repo-docs__section-title", section.title));

    const body = createElement("div", "repo-docs__section-body");
    renderMarkdown(body, section.content || "");
    article.appendChild(body);
    return article;
}

function renderDocsSectionCard(section, navigateToPath) {
    const article = createElement("section", "repo-docs__section repo-docs__section--card");
    const parsed = parseSectionContent(section.content || "");

    article.appendChild(createElement("p", "repo-docs__section-label", section.label));
    article.appendChild(createElement("h2", "repo-docs__section-title", section.title));

    if (parsed.summary) {
        article.appendChild(createElement("p", "repo-docs__section-preview", parsed.summary));
    }

    if (parsed.points.length > 0) {
        const list = createElement("ul", "repo-docs__point-list repo-docs__point-list--compact");
        for (const point of parsed.points.slice(0, 3)) {
            const item = createElement("li", "repo-docs__point");
            appendInlineContent(item, point);
            list.appendChild(item);
        }
        article.appendChild(list);
    }

    const footer = createElement("div", "repo-docs__section-footer");
    const link = createElement("a", "repo-docs__context-link", "Open section");
    bindHostedPathNavigation(link, getDocsSectionPath(section.id), navigateToPath);
    footer.appendChild(link);
    article.appendChild(footer);

    return article;
}

function renderDocsNotFound(navigateToPath) {
    const state = createElement("section", "repo-docs__state repo-docs__state--error");
    state.appendChild(createElement("h1", "repo-docs__title", "That docs page does not exist."));
    state.appendChild(createElement("p", "repo-docs__copy", "Use the overview to pick one of the supported docs sections."));

    const actions = createElement("div", "repo-docs__hero-actions");
    const overviewLink = createElement("a", "repo-docs__context-link", "Open Docs Overview");
    bindHostedPathNavigation(overviewLink, "/docs", navigateToPath);
    actions.appendChild(overviewLink);
    state.appendChild(actions);

    return state;
}

function resolveDocsHref(href) {
    if (typeof href !== "string" || href === "") return href;
    if (href.startsWith("#")) return getDocsSectionPath(href.slice(1));
    return href;
}

function getDocsSectionPath(sectionID) {
    return `/docs/${sectionID}`;
}

function parseSectionContent(markdown) {
    const blocks = String(markdown)
        .trim()
        .split(/\n\s*\n/)
        .map((block) => block.trim())
        .filter(Boolean);

    const paragraphs = [];
    const points = [];

    for (const block of blocks) {
        const lines = block.split("\n").map((line) => line.trim()).filter(Boolean);
        if (lines.length > 0 && lines.every((line) => line.startsWith("- "))) {
            for (const line of lines) points.push(line.slice(2));
            continue;
        }
        paragraphs.push(lines.join(" "));
    }

    return {
        summary: paragraphs[0] || "",
        paragraphs,
        points,
    };
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
            const list = createElement("ul", "repo-docs__point-list");
            for (const line of lines) {
                const item = createElement("li", "repo-docs__point");
                appendInlineContent(item, line.slice(2));
                list.appendChild(item);
            }
            container.appendChild(list);
            continue;
        }

        const paragraph = createElement("p", "repo-docs__copy");
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
            continue;
        }
        parent.appendChild(document.createTextNode(part));
    }
}

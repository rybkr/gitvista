import { createHostedTopbar } from "./hostedChrome.js";
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
        return;
    }

    const hero = createElement("section", "repo-docs__hero");
    const heroCopy = createElement("div", "repo-docs__hero-copy");
    heroCopy.appendChild(createElement("p", "repo-docs__eyebrow", activeSection ? "Docs Section" : (docs.eyebrow || "Product Docs")));
    heroCopy.appendChild(createElement("h1", "repo-docs__title", activeSection ? selectedSection.title : (docs.title || `${PRODUCT_INFO.name} Docs`)));
    heroCopy.appendChild(createElement("p", "repo-docs__lede", activeSection ? `Part of ${PRODUCT_INFO.name} Docs.` : (docs.lede || "")));

    const heroActions = createElement("div", "repo-docs__hero-actions");
    const primaryLink = createDocsLink(docs.primaryCta, "repo-docs__primary-link", navigateToPath);
    const secondaryLink = createDocsLink(docs.secondaryCta, "repo-docs__secondary-link", navigateToPath);
    if (primaryLink) heroActions.appendChild(primaryLink);
    if (secondaryLink) heroActions.appendChild(secondaryLink);
    heroCopy.appendChild(heroActions);

    const heroPanel = createElement("aside", "repo-docs__hero-panel");
    heroPanel.setAttribute("aria-label", activeSection ? "Current docs section" : "Docs summary");
    heroPanel.appendChild(createElement("div", "repo-docs__panel-kicker", activeSection ? "Current section" : "At a glance"));

    const panelGrid = createElement("div", "repo-docs__panel-grid");
    const panelItems = activeSection
        ? [
            { label: "Section", value: selectedSection.label },
            { label: "Path", value: getDocsSectionPath(selectedSection.id) },
            { label: "Docs Home", value: "/docs" },
        ]
        : (docs.summary || []);

    for (const item of panelItems) {
        const card = document.createElement("div");
        card.appendChild(createElement("strong", "", item.label));
        card.appendChild(createElement("span", "", item.value));
        panelGrid.appendChild(card);
    }
    heroPanel.appendChild(panelGrid);
    hero.appendChild(heroCopy);
    hero.appendChild(heroPanel);

    const rail = createElement("aside", "repo-docs__rail");
    rail.setAttribute("aria-label", "Doc sections");
    rail.appendChild(createElement("p", "repo-docs__rail-label", "Sections"));

    const overviewLink = createElement("a", "repo-docs__rail-link", "Overview");
    if (!activeSection) overviewLink.setAttribute("aria-current", "page");
    bindHostedPathNavigation(overviewLink, "/docs", navigateToPath);
    rail.appendChild(overviewLink);

    const sections = createElement("div", "repo-docs__sections");
    for (const section of docs.sections || []) {
        const railLink = createElement("a", "repo-docs__rail-link", section.label);
        if (section.id === activeSection) railLink.setAttribute("aria-current", "page");
        bindHostedPathNavigation(railLink, getDocsSectionPath(section.id), navigateToPath);
        rail.appendChild(railLink);

        if (!activeSection || section.id === activeSection) {
            sections.appendChild(renderDocsSection(section));
        }
    }

    const help = createElement("section", "repo-docs__help");
    const helpCopy = document.createElement("div");
    helpCopy.appendChild(createElement("p", "repo-docs__section-label", docs.help?.label || "Need More"));
    helpCopy.appendChild(createElement("h2", "repo-docs__section-title", docs.help?.title || ""));
    helpCopy.appendChild(createElement("p", "repo-docs__section-body", docs.help?.body || ""));

    const helpActions = createElement("div", "repo-docs__help-actions");
    const helpPrimaryLink = createDocsLink(docs.help?.primaryCta, "repo-docs__primary-link", navigateToPath);
    if (helpPrimaryLink) helpActions.appendChild(helpPrimaryLink);

    const githubLink = createElement("a", "repo-landing__footer-link", "GitHub");
    githubLink.href = PRODUCT_INFO.repositoryUrl;
    githubLink.target = "_blank";
    githubLink.rel = "noopener noreferrer";
    helpActions.appendChild(githubLink);

    help.appendChild(helpCopy);
    help.appendChild(helpActions);

    content.appendChild(hero);
    content.appendChild(rail);
    content.appendChild(sections);
    content.appendChild(help);
}

function createDocsLink(cta, className, navigateToPath) {
    if (!cta?.label || !cta?.href) return null;
    const link = createElement("a", className, cta.label);
    bindHostedPathNavigation(link, resolveDocsHref(cta.href), navigateToPath);
    return link;
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

function renderDocsNotFound(navigateToPath) {
    const state = createElement("section", "repo-docs__state repo-docs__state--error");
    state.appendChild(createElement("h1", "repo-docs__title", "That docs page does not exist."));
    state.appendChild(createElement("p", "repo-docs__copy", "Use the overview to pick one of the supported docs sections."));

    const actions = createElement("div", "repo-docs__hero-actions");
    const overviewLink = createElement("a", "repo-docs__primary-link", "Open Docs Overview");
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

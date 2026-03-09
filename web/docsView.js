import { createHostedTopbar } from "./hostedChrome.js";
import { PRODUCT_INFO } from "./hostedProduct.js";
import { bindHashScroll, bindHostedPathNavigation, scrollToHashTarget } from "./hostedNavigation.js";

function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

export function createDocsView({ navigateToPath } = {}) {
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
            renderDocs(content, docs, navigateToPath, el);
            if (location.hash) {
                requestAnimationFrame(() => {
                    scrollToHashTarget(el, location.hash);
                });
            }
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

function renderDocs(content, docs, navigateToPath, root) {
    content.replaceChildren();

    const hero = createElement("section", "repo-docs__hero");
    const heroCopy = createElement("div", "repo-docs__hero-copy");
    heroCopy.appendChild(createElement("p", "repo-docs__eyebrow", docs.eyebrow || "Product Docs"));
    heroCopy.appendChild(createElement("h1", "repo-docs__title", docs.title || `${PRODUCT_INFO.name} Docs`));
    heroCopy.appendChild(createElement("p", "repo-docs__lede", docs.lede || ""));

    const heroActions = createElement("div", "repo-docs__hero-actions");
    const primaryLink = createDocsLink(docs.primaryCta, "repo-docs__primary-link", navigateToPath, root);
    const secondaryLink = createDocsLink(docs.secondaryCta, "repo-docs__secondary-link", navigateToPath, root);
    if (primaryLink) heroActions.appendChild(primaryLink);
    if (secondaryLink) heroActions.appendChild(secondaryLink);
    heroCopy.appendChild(heroActions);

    const heroPanel = createElement("aside", "repo-docs__hero-panel");
    heroPanel.setAttribute("aria-label", "Docs summary");
    heroPanel.appendChild(createElement("div", "repo-docs__panel-kicker", "At a glance"));

    const panelGrid = createElement("div", "repo-docs__panel-grid");
    for (const item of docs.summary || []) {
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

    const sections = createElement("div", "repo-docs__sections");
    for (const section of docs.sections || []) {
        const railLink = createElement("a", "repo-docs__rail-link", section.label);
        bindHashScroll(railLink, `#${section.id}`, { root });
        rail.appendChild(railLink);

        const article = createElement("section", "repo-docs__section");
        article.id = section.id;
        article.appendChild(createElement("p", "repo-docs__section-label", section.label));
        article.appendChild(createElement("h2", "repo-docs__section-title", section.title));

        const body = createElement("div", "repo-docs__section-body");
        renderMarkdown(body, section.content || "");
        article.appendChild(body);
        sections.appendChild(article);
    }

    const help = createElement("section", "repo-docs__help");
    const helpCopy = document.createElement("div");
    helpCopy.appendChild(createElement("p", "repo-docs__section-label", docs.help?.label || "Need More"));
    helpCopy.appendChild(createElement("h2", "repo-docs__section-title", docs.help?.title || ""));
    helpCopy.appendChild(createElement("p", "repo-docs__section-body", docs.help?.body || ""));

    const helpActions = createElement("div", "repo-docs__help-actions");
    const helpPrimaryLink = createDocsLink(docs.help?.primaryCta, "repo-docs__primary-link", navigateToPath, root);
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

function createDocsLink(cta, className, navigateToPath, root) {
    if (!cta?.label || !cta?.href) return null;
    const link = createElement("a", className, cta.label);
    if (cta.href.startsWith("#")) {
        bindHashScroll(link, cta.href, { root });
        return link;
    }
    bindHostedPathNavigation(link, cta.href, navigateToPath);
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

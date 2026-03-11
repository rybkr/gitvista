import { createHostedFooter, createHostedTopbar } from "./hostedChrome.js";
import { bindHostedPathNavigation } from "./hostedNavigation.js";

const COPY_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <rect x="5" y="5" width="8" height="8" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
    <path d="M3 11V3a1.5 1.5 0 011.5-1.5H11" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
</svg>`;

const CHECK_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M3 8.5l3.5 3.5 6.5-8" stroke="var(--success-color)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

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

    if (activeSection) {
        content.appendChild(renderDocsPage(selectedSection, docs.sections || [], navigateToPath));
    } else {
        content.appendChild(renderDocsOverview(docs, navigateToPath));
    }
    content.appendChild(createHostedFooter());
}

function renderDocsOverview(docs, navigateToPath) {
    const layout = createElement("div", "repo-docs__page");
    layout.appendChild(renderDocsSidebar(docs.sections || [], null, navigateToPath));

    const main = createElement("div", "repo-docs__main");
    const header = createElement("section", "repo-docs__header");
    if (docs.eyebrow) {
        header.appendChild(createElement("p", "repo-docs__eyebrow", docs.eyebrow));
    }
    header.appendChild(createElement("h1", "repo-docs__title", docs.title || "Documentation"));
    if (docs.lede) {
        header.appendChild(createElement("p", "repo-docs__lede", docs.lede));
    }
    if ((docs.summary || []).length > 0) {
        header.appendChild(renderDocsSummary(docs.summary));
    }
    main.appendChild(header);

    const toc = createElement("section", "repo-docs__toc");
    toc.appendChild(createElement("h2", "repo-docs__toc-title", "Start with the path that matches the job"));
    const tocList = createElement("div", "repo-docs__toc-list");
    for (const section of docs.sections || []) {
        tocList.appendChild(renderDocsSectionCard(section, navigateToPath));
    }
    toc.appendChild(tocList);
    main.appendChild(toc);

    if (docs.help?.title || docs.help?.body) {
        main.appendChild(renderDocsHelp(docs.help, navigateToPath));
    }

    layout.appendChild(main);
    return layout;
}

function renderDocsPage(section, sections, navigateToPath) {
    const headings = extractSectionHeadings(section);
    const sectionIndex = sections.findIndex((entry) => entry.id === section.id);
    const previousSection = sectionIndex > 0 ? sections[sectionIndex - 1] : null;
    const nextSection = sectionIndex >= 0 && sectionIndex < sections.length - 1 ? sections[sectionIndex + 1] : null;
    const layout = createElement("div", "repo-docs__page");
    layout.appendChild(renderDocsSidebar(sections, section.id, navigateToPath, { headings }));

    const main = createElement("div", "repo-docs__main");
    const header = createElement("section", "repo-docs__header repo-docs__header--page");
    header.appendChild(renderDocsBreadcrumbs(section, navigateToPath));
    header.appendChild(createElement("h1", "repo-docs__title", section.title));
    const backLink = createElement("a", "repo-docs__context-link", "Back to overview");
    bindHostedPathNavigation(backLink, "/docs", navigateToPath);
    const headerMeta = createElement("div", "repo-docs__header-meta");
    headerMeta.appendChild(backLink);
    header.appendChild(headerMeta);
    main.appendChild(header);
    main.appendChild(renderDocsSection(section, { headingTag: "h2", navigateToPath }));
    if (sectionIndex >= 0) {
        main.appendChild(renderDocsPager({
            previousItem: previousSection
                ? { kind: "section", section: previousSection }
                : { kind: "overview", label: "Overview", href: "/docs" },
            nextItem: nextSection ? { kind: "section", section: nextSection } : null,
            navigateToPath,
        }));
    }
    layout.appendChild(main);

    return layout;
}

function renderDocsSidebar(sections, activeSectionID, navigateToPath, { headings = [] } = {}) {
    const sidebar = createElement("aside", "repo-docs__sidebar");
    sidebar.setAttribute("aria-label", "Documentation navigation");
    sidebar.appendChild(createElement("p", "repo-docs__sidebar-label", activeSectionID ? "Sections" : "Guide"));

    const nav = createElement("nav", "repo-docs__sidebar-nav");
    const overviewLink = createElement("a", "repo-docs__sidebar-link", "Overview");
    if (!activeSectionID) overviewLink.setAttribute("aria-current", "page");
    bindHostedPathNavigation(overviewLink, "/docs", navigateToPath);
    nav.appendChild(overviewLink);

    for (const entry of sections) {
        const navLink = createElement("a", "repo-docs__sidebar-link", entry.label);
        if (entry.id === activeSectionID) navLink.setAttribute("aria-current", "page");
        bindHostedPathNavigation(navLink, getDocsSectionPath(entry.id), navigateToPath);
        nav.appendChild(navLink);
    }

    sidebar.appendChild(nav);

    if (activeSectionID && headings.length > 0) {
        sidebar.appendChild(createElement("p", "repo-docs__sidebar-label repo-docs__sidebar-label--subnav", "On this page"));
        const headingNav = createElement("nav", "repo-docs__sidebar-subnav");
        for (const heading of headings) {
            const headingLink = createElement("a", "repo-docs__sidebar-sublink", heading.text);
            headingLink.href = `#${heading.id}`;
            headingNav.appendChild(headingLink);
        }
        sidebar.appendChild(headingNav);
    }

    return sidebar;
}

function renderDocsSection(section, { headingTag = "h2", navigateToPath = null } = {}) {
    const article = createElement("section", "repo-docs__section");
    article.id = section.id;

    const body = createElement("div", "repo-docs__section-body");
    if (section.contentHtml) {
        body.innerHTML = section.contentHtml;
        enhanceRenderedMarkdown(body, navigateToPath);
    } else {
        renderMarkdown(body, section.content || "");
    }
    article.appendChild(body);
    return article;
}

function renderDocsSectionCard(section, navigateToPath) {
    const { summary } = parseSectionContent(section.content || "");
    const article = createElement("a", "repo-docs__toc-item");
    bindHostedPathNavigation(article, getDocsSectionPath(section.id), navigateToPath);

    article.appendChild(createElement("span", "repo-docs__toc-item-kicker", section.label));
    article.appendChild(createElement("span", "repo-docs__toc-item-title", section.title || section.label));
    if (summary) {
        article.appendChild(createElement("span", "repo-docs__toc-item-summary", summary));
    }

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
    const blocks = parseMarkdownBlocks(markdown);
    const paragraphs = [];

    for (const block of blocks) {
        if (block.type !== "paragraph") continue;
        const lines = block.content.split("\n").map((line) => line.trim()).filter(Boolean);
        paragraphs.push(lines.join(" "));
    }

    return {
        summary: paragraphs[0] || "",
        paragraphs,
    };
}

function renderMarkdown(container, markdown) {
    const blocks = parseMarkdownBlocks(markdown);

    for (const block of blocks) {
        if (block.type === "ulist" || block.type === "olist") {
            const lines = block.content.split("\n").map((line) => line.trim()).filter(Boolean);
            const list = createElement(block.type === "olist" ? "ol" : "ul", "repo-docs__list");
            for (const line of lines) {
                const item = createElement("li", "repo-docs__list-item");
                appendInlineContent(item, stripListMarker(line));
                list.appendChild(item);
            }
            container.appendChild(list);
            continue;
        }

        if (block.type === "code") {
            const pre = createElement("pre", "repo-docs__code");
            const code = document.createElement("code");
            code.textContent = block.content;
            const copyBtn = createCopyButton(block.content);
            pre.appendChild(code);
            pre.appendChild(copyBtn);
            container.appendChild(pre);
            continue;
        }

        const paragraph = createElement("p", "repo-docs__copy");
        const lines = block.content.split("\n").map((line) => line.trim()).filter(Boolean);
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

function createCopyButton(text) {
    const btn = createElement("button", "repo-docs__copy-btn");
    btn.type = "button";
    btn.title = "Copy command";
    btn.setAttribute("aria-label", "Copy command");
    btn.innerHTML = COPY_SVG;
    btn.addEventListener("click", async () => {
        try {
            await navigator.clipboard.writeText(text);
            btn.innerHTML = CHECK_SVG;
            window.setTimeout(() => {
                btn.innerHTML = COPY_SVG;
            }, 2000);
        } catch {
            // Clipboard API may be unavailable.
        }
    });
    return btn;
}

function enhanceRenderedMarkdown(container, navigateToPath = null) {
    let headingIndex = 0;
    for (const heading of container.querySelectorAll("h1, h2, h3")) {
        headingIndex += 1;
        heading.classList.add("repo-docs__heading", `repo-docs__heading--${heading.tagName.toLowerCase()}`);
        if (!heading.id) {
            heading.id = `section-${headingIndex}`;
        }
    }

    for (const paragraph of container.querySelectorAll("p")) {
        paragraph.classList.add("repo-docs__copy");
    }

    for (const list of container.querySelectorAll("ul, ol")) {
        list.classList.add("repo-docs__list");
    }

    for (const item of container.querySelectorAll("li")) {
        item.classList.add("repo-docs__list-item");
    }

    for (const blockquote of container.querySelectorAll("blockquote")) {
        blockquote.classList.add("repo-docs__quote");
    }

    for (const rule of container.querySelectorAll("hr")) {
        rule.classList.add("repo-docs__rule");
    }

    for (const table of container.querySelectorAll("table")) {
        const wrap = createElement("div", "repo-docs__table-wrap");
        table.parentNode?.insertBefore(wrap, table);
        wrap.appendChild(table);
        table.classList.add("repo-docs__table");
    }

    for (const externalLink of container.querySelectorAll("a[href]")) {
        const href = externalLink.getAttribute("href") || "";
        externalLink.classList.add("repo-docs__inline-link");
        if (href === "/docs/cli") {
            externalLink.classList.remove("repo-docs__inline-link");
            externalLink.classList.add("repo-install__docs-link");
        }
        if (navigateToPath && href.startsWith("/")) {
            bindHostedPathNavigation(externalLink, href, navigateToPath);
            continue;
        }
        if (/^https?:\/\//.test(href)) {
            externalLink.setAttribute("target", "_blank");
            externalLink.setAttribute("rel", "noopener noreferrer");
        }
    }

    for (const pre of container.querySelectorAll("pre")) {
        if (pre.querySelector(".repo-docs__copy-btn")) continue;
        const code = pre.querySelector("code");
        if (!code) continue;
        pre.classList.add("repo-docs__code");
        const language = code.className.match(/language-([a-z0-9_-]+)/i)?.[1] || "";
        if (language) {
            pre.dataset.language = language;
        }
        pre.appendChild(createCopyButton(code.textContent || ""));
    }
}

function renderDocsSummary(items) {
    const grid = createElement("div", "repo-docs__summary");
    for (const item of items) {
        const card = createElement("article", "repo-docs__summary-item");
        card.appendChild(createElement("span", "repo-docs__summary-label", item.label || ""));
        card.appendChild(createElement("strong", "repo-docs__summary-value", item.value || ""));
        grid.appendChild(card);
    }
    return grid;
}

function renderDocsHelp(help, navigateToPath) {
    const section = createElement("section", "repo-docs__help");
    if (help.label) {
        section.appendChild(createElement("p", "repo-docs__eyebrow", help.label));
    }
    if (help.title) {
        section.appendChild(createElement("h2", "repo-docs__toc-title", help.title));
    }
    if (help.body) {
        section.appendChild(createElement("p", "repo-docs__lede repo-docs__lede--compact", help.body));
    }
    if (help.primaryCta?.href && help.primaryCta?.label) {
        const cta = createElement("a", "repo-docs__primary-link", help.primaryCta.label);
        bindHostedPathNavigation(cta, help.primaryCta.href, navigateToPath);
        section.appendChild(cta);
    }
    return section;
}

function renderDocsPager({ previousItem = null, nextItem = null, navigateToPath = null }) {
    const nav = createElement("nav", "repo-docs__pager");
    nav.setAttribute("aria-label", "Documentation pagination");

    if (previousItem) {
        nav.appendChild(renderDocsPagerLink({
            label: "Previous",
            item: previousItem,
            direction: "previous",
            navigateToPath,
        }));
    }

    if (nextItem) {
        nav.appendChild(renderDocsPagerLink({
            label: "Next",
            item: nextItem,
            direction: "next",
            navigateToPath,
        }));
    }

    return nav;
}

function renderDocsPagerLink({ label, item, direction, navigateToPath }) {
    const link = createElement("a", `repo-docs__pager-link repo-docs__pager-link--${direction}`);
    const href = item.kind === "overview" ? item.href : getDocsSectionPath(item.section.id);
    const title = item.kind === "overview" ? item.label : (item.section.label || item.section.title || "");
    bindHostedPathNavigation(link, href, navigateToPath);

    link.appendChild(createElement("span", "repo-docs__pager-label", label));
    link.appendChild(createElement("strong", "repo-docs__pager-title", title));

    return link;
}

function renderDocsBreadcrumbs(section, navigateToPath) {
    const nav = createElement("nav", "repo-docs__breadcrumbs");
    nav.setAttribute("aria-label", "Breadcrumb");

    const overview = createElement("a", "repo-docs__breadcrumb-link", "Docs");
    bindHostedPathNavigation(overview, "/docs", navigateToPath);
    nav.appendChild(overview);

    nav.appendChild(createElement("span", "repo-docs__breadcrumb-sep", "/"));
    nav.appendChild(createElement("span", "repo-docs__breadcrumb-current", section.label || section.title || "Section"));

    return nav;
}

function extractSectionHeadings(section) {
    if (typeof section?.contentHtml !== "string" || section.contentHtml.trim() === "") return [];
    const template = document.createElement("template");
    template.innerHTML = section.contentHtml;
    return [...template.content.querySelectorAll("h2, h3")]
        .filter((heading) => heading.id && heading.textContent?.trim())
        .map((heading) => ({
            id: heading.id,
            text: heading.textContent.trim(),
            level: heading.tagName.toLowerCase(),
        }));
}

function stripListMarker(line) {
    return String(line).replace(/^(?:- |\d+\.\s+)/, "");
}

function parseMarkdownBlocks(markdown) {
    const normalized = String(markdown || "").trim();
    if (!normalized) return [];

    const blocks = [];
    const lines = normalized.split("\n");
    let index = 0;

    while (index < lines.length) {
        const line = lines[index];
        const trimmed = line.trim();

        if (!trimmed) {
            index += 1;
            continue;
        }

        if (trimmed.startsWith("```")) {
            const codeLines = [];
            index += 1;
            while (index < lines.length && !lines[index].trim().startsWith("```")) {
                codeLines.push(lines[index]);
                index += 1;
            }
            if (index < lines.length) index += 1;
            blocks.push({ type: "code", content: codeLines.join("\n").trimEnd() });
            continue;
        }

        if (trimmed.startsWith("- ")) {
            const items = [];
            while (index < lines.length) {
                const current = lines[index].trim();
                if (!current.startsWith("- ")) break;
                items.push(current);
                index += 1;
            }
            blocks.push({ type: "ulist", content: items.join("\n") });
            continue;
        }

        if (/^\d+\.\s+/.test(trimmed)) {
            const items = [];
            while (index < lines.length) {
                const current = lines[index].trim();
                if (!/^\d+\.\s+/.test(current)) break;
                items.push(current);
                index += 1;
            }
            blocks.push({ type: "olist", content: items.join("\n") });
            continue;
        }

        const paragraphLines = [];
        while (index < lines.length) {
            const current = lines[index].trim();
            if (!current) {
                index += 1;
                break;
            }
            if (current.startsWith("```") || current.startsWith("- ") || /^\d+\.\s+/.test(current)) break;
            paragraphLines.push(current);
            index += 1;
        }
        if (paragraphLines.length > 0) {
            blocks.push({ type: "paragraph", content: paragraphLines.join("\n") });
        }
    }

    return blocks;
}

import { createHostedFooter, createHostedTopbar } from "./hostedChrome.js";
import { bindHostedPathNavigation } from "./hostedNavigation.js";
import { PRODUCT_INFO } from "./hostedProduct.js";

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

export function createInstallView({ navigateToPath } = {}) {
    const el = createElement("div", "repo-install");
    const chrome = createElement("div", "repo-install__chrome");
    chrome.appendChild(createHostedTopbar({
        activePath: "/install",
        navigateToPath,
        navItems: [
            { label: "Home", path: "/" },
            { label: "Install", path: "/install" },
            { label: "Docs", path: "/docs" },
        ],
    }));

    const content = createElement("div", "repo-install__content");
    const hero = createElement("section", "repo-install__hero");
    const heroCopy = createElement("div", "repo-install__hero-copy");
    heroCopy.appendChild(createElement("p", "repo-install__eyebrow", "Install GitVista"));
    heroCopy.appendChild(createElement("h1", "repo-install__title", "Run GitVista beside your repo."));
    heroCopy.appendChild(createElement("p", "repo-install__lede", "This page is the start of the local setup flow. For now, it just gives you the install command and the shortest path into local mode."));

    const heroMeta = createElement("div", "repo-install__hero-meta");
    const docsLink = createElement("a", "repo-install__context-link", "Read product docs");
    bindHostedPathNavigation(docsLink, "/docs", navigateToPath);
    heroMeta.appendChild(docsLink);
    heroCopy.appendChild(heroMeta);

    const commandCard = createElement("aside", "repo-install__command-card");
    commandCard.appendChild(createElement("div", "repo-install__panel-kicker", "Command"));
    commandCard.appendChild(createElement("p", "repo-install__command-label", "Install and launch"));

    const codeBlock = createElement("div", "repo-landing__code-block repo-install__code-block");
    const codeText = createElement("code", "", PRODUCT_INFO.installCommand);
    const copyBtn = createElement("button", "repo-landing__copy-btn");
    copyBtn.type = "button";
    copyBtn.title = "Copy install command";
    copyBtn.setAttribute("aria-label", "Copy install command");
    copyBtn.innerHTML = COPY_SVG;
    copyBtn.addEventListener("click", async () => {
        try {
            await navigator.clipboard.writeText(codeText.textContent || "");
            copyBtn.innerHTML = CHECK_SVG;
            window.setTimeout(() => {
                copyBtn.innerHTML = COPY_SVG;
            }, 2000);
        } catch {
            // Clipboard API may be unavailable.
        }
    });
    codeBlock.appendChild(codeText);
    codeBlock.appendChild(copyBtn);
    commandCard.appendChild(codeBlock);
    commandCard.appendChild(createElement("p", "repo-install__command-note", "After launch, point GitVista at a local checkout so commit, diff, and worktree state stay live."));

    const steps = createElement("section", "repo-install__steps");
    steps.appendChild(createElement("p", "repo-install__section-label", "Scaffold"));
    steps.appendChild(createElement("h2", "repo-install__section-title", "More setup detail will land here next."));
    steps.appendChild(createElement("p", "repo-install__section-body", "I’ve kept this intentionally light for now: one command, one destination, and enough framing to make the route real."));

    const stepGrid = createElement("div", "repo-install__step-grid");
    const cards = [
        ["1. Install", "Use the command above to add the local app."],
        ["2. Launch", "Start GitVista from your shell."],
        ["3. Open a repo", "Point it at a checkout and inspect live state."],
    ];
    for (const [title, body] of cards) {
        const card = createElement("article", "repo-install__step-card");
        card.appendChild(createElement("strong", "repo-install__step-title", title));
        card.appendChild(createElement("p", "repo-install__step-body", body));
        stepGrid.appendChild(card);
    }
    steps.appendChild(stepGrid);

    content.appendChild(hero);
    content.appendChild(commandCard);
    content.appendChild(steps);
    content.appendChild(createHostedFooter());

    el.appendChild(chrome);
    el.appendChild(content);

    return {
        el,
        destroy() {},
    };
}

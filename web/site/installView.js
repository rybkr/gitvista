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
    heroCopy.appendChild(createElement("h1", "repo-install__title", "Install the local app."));
    heroCopy.appendChild(createElement("p", "repo-install__lede", "Install GitVista, start it against a local repository, and inspect the graph, diffs, and current repository state."));
    hero.appendChild(heroCopy);

    const installBlock = createElement("section", "repo-install__install-block");
    installBlock.appendChild(createElement("p", "repo-install__section-label", "Install"));
    installBlock.appendChild(createElement("h2", "repo-install__section-title", "Install and launch"));
    installBlock.appendChild(createElement("p", "repo-install__section-body", "Requirements: a local Git checkout, shell access to run the install script, and a browser on the same machine."));

    installBlock.appendChild(createOrderedStep(
        "1",
        "Run the install script once to set up GitVista locally.",
        "This downloads and configures the local app so the CLI command is available in your shell.",
        PRODUCT_INFO.installCommand,
    ));
    installBlock.appendChild(createOrderedStep(
        "2",
        "Run GitVista from your shell.",
        "Pass the repository path directly so GitVista opens the checkout you want to inspect.",
        "git vista open -repo /path/to/repo",
    ));
    installBlock.appendChild(createOrderedStep(
        "3",
        "Analyze the repository.",
        "GitVista loads the branch graph, diffs, and current local state for the repository you opened.",
    ));

    const docsLink = createElement("a", "repo-install__docs-link", "Read the full docs");
    bindHostedPathNavigation(docsLink, "/docs", navigateToPath);
    installBlock.appendChild(docsLink);

    content.appendChild(hero);
    content.appendChild(installBlock);
    content.appendChild(createHostedFooter());

    el.appendChild(chrome);
    el.appendChild(content);

    return {
        el,
        destroy() {},
    };
}

function createOrderedStep(index, title, description, command = "") {
    const item = createElement("div", "repo-install__ordered-step");
    item.appendChild(createElement("span", "repo-install__ordered-index", index));
    const content = createElement("div", "repo-install__ordered-body");
    content.appendChild(createElement("p", "repo-install__ordered-title", title));
    content.appendChild(createElement("p", "repo-install__ordered-copy", description));
    item.appendChild(content);
    if (command) {
        item.appendChild(createCopyCodeBlock(command));
    }
    return item;
}

function createCopyCodeBlock(command) {
    const codeBlock = createElement("div", "repo-landing__code-block repo-install__code-block repo-install__step-code");
    const codeText = createElement("code", "", command);
    const copyBtn = createElement("button", "repo-landing__copy-btn");
    copyBtn.type = "button";
    copyBtn.title = "Copy command";
    copyBtn.setAttribute("aria-label", "Copy command");
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
    return codeBlock;
}

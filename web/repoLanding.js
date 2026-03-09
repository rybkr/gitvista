import { createHostedFooter, createHostedTopbar } from "./hostedChrome.js";
import { PRODUCT_INFO } from "./hostedProduct.js";
import { createHeroPreview } from "./landing/preview.js";
import { FEATURED_REPOS, HERO_PREVIEW } from "./landing/content.js";
import { createRepoBrowser } from "./landing/repoBrowser.js";

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

export function createRepoLanding({ onRepoSelect, onNavigate }) {
    const el = createElement("div", "repo-landing");
    const chrome = createElement("div", "repo-landing__chrome");
    const content = createElement("div", "repo-landing__content");
    let highlightTimer = null;
    let scrollCueObserver = null;

    function scrollToSection(target, { focus } = {}) {
        target?.scrollIntoView({ behavior: "smooth", block: "start" });
        if (focus) {
            window.setTimeout(() => {
                focus.focus();
                focus.select?.();
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

    const repoBrowser = createRepoBrowser({ featuredRepos: FEATURED_REPOS, onRepoSelect });

    const topbar = createHostedTopbar({
        activePath: "/",
        navigateToPath: onNavigate,
        brandAction: () => scrollToSection(hero, { focus: input }),
        navItems: [
            { label: "Home", path: "/" },
            { label: "Install", path: "/install" },
            { label: "Docs", path: "/docs" },
        ],
    });

    const hero = createElement("section", "repo-landing__hero");
    hero.id = "try";
    const heroCopy = createElement("div", "repo-landing__hero-copy");
    heroCopy.appendChild(createElement("h1", "repo-landing__title", "Git history stops being guesswork."));
    heroCopy.appendChild(createElement("p", "repo-landing__tagline", "Open any public GitHub repository in one clear view with the branch graph, recent activity, and commit diffs aligned from the start."));

    const heroFormShell = createElement("div", "repo-landing__hero-form-shell");
    const formLead = createElement("div", "repo-landing__hero-form-lead");
    formLead.appendChild(createElement("strong", "", "Try it here"));
    formLead.appendChild(createElement("span", "", "Drop in a public GitHub repository to trace branches, commits, and diffs in one readable view."));

    const form = createElement("form", "repo-landing__form");
    const input = createElement("input", "repo-landing__input");
    input.type = "url";
    input.placeholder = "https://github.com/owner/repo";
    input.required = true;
    input.setAttribute("aria-label", "GitHub repository URL");

    const addBtn = createElement("button", "repo-landing__add-btn", "Open");
    addBtn.type = "submit";
    form.appendChild(input);
    form.appendChild(addBtn);

    const formMeta = createElement("div", "repo-landing__hero-form-meta");
    formMeta.appendChild(createElement("span", "repo-landing__hero-support", "No setup for public repos. Use local mode for live working-tree changes."));

    const errorMsg = createElement("div", "repo-landing__error");

    heroFormShell.appendChild(formLead);
    heroFormShell.appendChild(form);
    heroFormShell.appendChild(formMeta);
    heroFormShell.appendChild(errorMsg);
    heroCopy.appendChild(heroFormShell);

    const heroPreview = createElement("div", "repo-landing__hero-preview");
    heroPreview.setAttribute("aria-hidden", "true");
    const heroPreviewFrame = createHeroPreview(HERO_PREVIEW);
    heroPreview.appendChild(heroPreviewFrame);

    const proofStrip = createElement("section", "repo-landing__proof-strip");
    proofStrip.id = "why";
    proofStrip.innerHTML = `
        <div class="repo-landing__proof-item"><strong>Instant orientation</strong><span>Graph-first context before you inspect individual commits.</span></div>
        <div class="repo-landing__proof-item"><strong>Example repos ready</strong><span>Jump into curated public repos without waiting on setup.</span></div>
        <div class="repo-landing__proof-item"><strong>Local mode available</strong><span>Track your own <code>.git</code> directory when browser mode is not enough.</span></div>
    `;
    const proofHeader = [
        createElement("p", "repo-landing__eyebrow", "Why GitVista"),
        createElement("h2", "repo-landing__section-title repo-landing__proof-title", "One readable view for branches, merges, and the commits between them."),
        createElement("p", "repo-landing__section-subtitle repo-landing__proof-intro", "Built for the moment when branch names, merge commits, and detached HEADs stop being legible in your head."),
    ];
    proofStrip.replaceChildren(...proofHeader, ...Array.from(proofStrip.children));

    const scrollCue = createElement("button", "repo-landing__scroll-cue");
    scrollCue.type = "button";
    scrollCue.setAttribute("aria-label", "Scroll to the next section");
    scrollCue.appendChild(createElement("span", "repo-landing__scroll-cue-label", "See More"));
    scrollCue.appendChild(createElement("span", "repo-landing__scroll-cue-arrow", ""));
    scrollCue.addEventListener("click", () => scrollToSection(proofStrip));

    hero.appendChild(heroCopy);
    hero.appendChild(heroPreview);
    hero.appendChild(scrollCue);

    if ("IntersectionObserver" in window) {
        scrollCueObserver = new IntersectionObserver(
            ([entry]) => {
                scrollCue.classList.toggle("repo-landing__scroll-cue--hidden", entry.intersectionRatio < 0.8);
            },
            {
                threshold: [0.8, 1],
            },
        );
        scrollCueObserver.observe(hero);
    }

    const installSection = createElement("section", "repo-landing__section repo-landing__install");
    installSection.id = "local";
    installSection.appendChild(createElement("p", "repo-landing__eyebrow", "Local mode"));
    installSection.appendChild(createElement("h2", "repo-landing__section-title", "Run GitVista beside your repository when you need live local state."));
    installSection.appendChild(createElement("p", "repo-landing__section-subtitle", "Browser mode is the fastest way to inspect a public repo. Local mode connects directly to your .git directory so new commits, staged changes, and diff views update the moment you make them."));

    const codeBlock = createElement("div", "repo-landing__code-block");
    const codeText = createElement("code", "", PRODUCT_INFO.installCommand);
    const copyBtn = createElement("button", "repo-landing__copy-btn");
    copyBtn.title = "Copy to clipboard";
    copyBtn.innerHTML = COPY_SVG;
    copyBtn.addEventListener("click", async () => {
        try {
            await navigator.clipboard.writeText(codeText.textContent);
            copyBtn.innerHTML = CHECK_SVG;
            setTimeout(() => {
                copyBtn.innerHTML = COPY_SVG;
            }, 2000);
        } catch {
            // Clipboard API not available.
        }
    });
    codeBlock.appendChild(codeText);
    codeBlock.appendChild(copyBtn);
    installSection.appendChild(codeBlock);

    const footer = createHostedFooter();

    form.addEventListener("submit", async (event) => {
        event.preventDefault();
        errorMsg.textContent = "";
        const url = input.value.trim();
        if (!url) return;

        addBtn.disabled = true;
        addBtn.textContent = "Opening...";
        try {
            const result = await repoBrowser.openRepository(url);
            input.value = "";
            if (result?.kind === "featured") {
                scrollToSection(repoBrowser.featuredSectionEl);
                highlightElementTemporarily(result.element);
            } else if (result?.kind === "repo") {
                scrollToSection(heroFormShell);
            }
        } catch (error) {
            errorMsg.textContent = error.message;
        } finally {
            addBtn.disabled = false;
            addBtn.textContent = "Open";
        }
    });

    chrome.appendChild(topbar);
    content.appendChild(hero);
    content.appendChild(proofStrip);
    content.appendChild(repoBrowser.featuredSectionEl);
    content.appendChild(installSection);
    content.appendChild(footer);
    el.appendChild(chrome);
    el.appendChild(content);

    repoBrowser.loadInitialRepos();

    return {
        el,
        destroy() {
            window.clearTimeout(highlightTimer);
            scrollCueObserver?.disconnect();
            heroPreviewFrame.destroy?.();
            repoBrowser.destroy();
        },
    };
}

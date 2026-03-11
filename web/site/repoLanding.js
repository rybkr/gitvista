import { createHostedFooter, createHostedTopbar } from "./hostedChrome.js";
import { bindHostedPathNavigation } from "./hostedNavigation.js";
import { createHeroPreview } from "../landing/preview.js";
import { FEATURED_REPOS, HERO_PREVIEW, PUBLIC_REPO_SAMPLES } from "../landing/content.js";
import { createRepoBrowser } from "../landing/repoBrowser.js";

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
    let sampleRotationTimer = null;
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
    const inputShell = createElement("div", "repo-landing__input-shell");
    const input = createElement("input", "repo-landing__input");
    input.type = "url";
    input.placeholder = "";
    input.required = true;
    input.setAttribute("aria-label", "GitHub repository URL");
    input.autocomplete = "off";

    const sampleHint = createElement("div", "repo-landing__input-sample");
    sampleHint.setAttribute("aria-hidden", "true");
    const sampleHintText = createElement("span", "repo-landing__input-sample-text");
    sampleHint.appendChild(sampleHintText);

    const addBtn = createElement("button", "repo-landing__add-btn", "Open");
    addBtn.type = "submit";
    inputShell.appendChild(input);
    inputShell.appendChild(sampleHint);
    form.appendChild(inputShell);
    form.appendChild(addBtn);

    const sampleUrls = PUBLIC_REPO_SAMPLES.length ? PUBLIC_REPO_SAMPLES : FEATURED_REPOS.map((repo) => repo.url);
    let sampleIndex = 0;

    const formMeta = createElement("div", "repo-landing__hero-form-meta");
    formMeta.appendChild(createElement("span", "repo-landing__hero-support", "No setup for public repos. Use the desktop app for live working-tree changes."));

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
        <div class="repo-landing__proof-item"><strong>See the shape of the repo</strong><span>Follow branches, merges, and commit flow in one view before you drill into individual diffs.</span></div>
        <div class="repo-landing__proof-item"><strong>Start instantly in the browser</strong><span>Open public GitHub repositories without setup and get useful context right away.</span></div>
        <div class="repo-landing__proof-item"><strong>Stay live when it matters</strong><span>Connect to your own <code>.git</code> directory when you need live working-tree and checkout updates.</span></div>
    `;
    const proofHeader = [
        createElement("p", "repo-landing__eyebrow", "Why GitVista"),
        createElement("h2", "repo-landing__section-title repo-landing__proof-title", "Git history becomes readable the moment you open it."),
        createElement("p", "repo-landing__section-subtitle repo-landing__proof-intro", "GitVista turns branch movement, merge points, and commit-level changes into one clear workflow for inspection."),
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
    installSection.appendChild(createElement("p", "repo-landing__eyebrow", "Desktop app"));
    installSection.appendChild(createElement("h2", "repo-landing__section-title", "Run GitVista beside your repository when you need live local state."));
    installSection.appendChild(createElement("p", "repo-landing__section-subtitle", "Browser mode is the fastest way to inspect a public repo. Head to the install page for the desktop app command and local setup flow."));

    const installLink = createElement("a", "repo-install__docs-link");
    installLink.textContent = "Open the setup page";
    bindHostedPathNavigation(installLink, "/docs/install", onNavigate);
    installSection.appendChild(installLink);

    const footer = createHostedFooter();

    function syncSampleVisibility() {
        const shouldHide = document.activeElement === input || Boolean(input.value.trim());
        sampleHint.classList.toggle("repo-landing__input-sample--hidden", shouldHide);
    }

    function updateSample(nextIndex) {
        if (!sampleUrls.length) return;
        sampleIndex = nextIndex % sampleUrls.length;
        const sampleUrl = sampleUrls[sampleIndex];
        sampleHintText.textContent = sampleUrl;
        sampleHint.title = sampleUrl;
        sampleHint.classList.remove("repo-landing__input-sample--animate");
        // Force the animation to restart for each repo sample.
        void sampleHint.offsetWidth;
        sampleHint.classList.add("repo-landing__input-sample--animate");
        syncSampleVisibility();
    }

    inputShell.addEventListener("click", () => input.focus());
    input.addEventListener("focus", syncSampleVisibility);
    input.addEventListener("blur", syncSampleVisibility);
    input.addEventListener("input", syncSampleVisibility);

    updateSample(sampleIndex);
    if (sampleUrls.length > 1) {
        sampleRotationTimer = window.setInterval(() => {
            updateSample(sampleIndex + 1);
        }, 2400);
    }

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
            syncSampleVisibility();
            if (result?.id) {
                onRepoSelect?.(result.id);
                return;
            }
            if (result?.kind === "featured") {
                scrollToSection(repoBrowser.featuredSectionEl);
                highlightElementTemporarily(result.element);
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
            window.clearInterval(sampleRotationTimer);
            scrollCueObserver?.disconnect();
            heroPreviewFrame.destroy?.();
            repoBrowser.destroy();
        },
    };
}

import { PRODUCT_INFO } from "./hostedProduct.js";
import { bindHostedPathNavigation } from "./hostedNavigation.js";

function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

const GITHUB_SVG = `<svg viewBox="0 0 16 16" width="14" height="14" fill="currentColor" aria-hidden="true">
    <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.5-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82A7.6 7.6 0 0 1 8 4.77c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/>
</svg>`;

function createBrandCopy() {
    const copy = createElement("span", "repo-landing__brand-copy");
    copy.appendChild(createElement("strong", "", PRODUCT_INFO.name));
    copy.appendChild(createElement("span", "", PRODUCT_INFO.tagline));
    return copy;
}

export function createHostedTopbar({ activePath = "/", navigateToPath, brandAction = null, navItems = [] } = {}) {
    const topbar = createElement("header", "repo-landing__topbar");
    const nav = createElement("nav", "repo-landing__topbar-nav");
    nav.setAttribute("aria-label", "Primary");

    const brand = createElement("a", "repo-landing__brand");
    brand.setAttribute("aria-label", `${PRODUCT_INFO.name} home`);
    if (typeof brandAction === "function") {
        brand.href = "/";
        brand.addEventListener("click", (event) => {
            event.preventDefault();
            brandAction();
        });
    } else {
        bindHostedPathNavigation(brand, "/", navigateToPath);
    }

    const brandMark = createElement("img", "repo-landing__brand-mark");
    brandMark.src = "/favicon.png";
    brandMark.alt = "";
    brandMark.setAttribute("aria-hidden", "true");

    brand.appendChild(brandMark);
    brand.appendChild(createBrandCopy());
    nav.appendChild(brand);

    const links = createElement("div", "repo-landing__topbar-links");
    for (const item of navItems) {
        const link = createElement("a", "repo-landing__nav-link", item.label);
        if (item.path) {
            bindHostedPathNavigation(link, item.path, navigateToPath);
            if (item.path === activePath) {
                link.classList.add("repo-landing__nav-link--active");
                link.setAttribute("aria-current", "page");
            }
        } else if (typeof item.onClick === "function") {
            link.href = item.href || "#";
            link.addEventListener("click", (event) => {
                event.preventDefault();
                item.onClick();
            });
        }
        links.appendChild(link);
    }

    nav.appendChild(links);
    topbar.appendChild(nav);
    return topbar;
}

export function createHostedFooter() {
    const footer = createElement("footer", "repo-landing__footer");
    const currentYear = new Date().getFullYear();

    const brand = createElement("div", "repo-landing__footer-brand");
    brand.appendChild(createElement("strong", "", PRODUCT_INFO.name));
    brand.appendChild(createElement("span", "", "See what Git is actually doing."));

    const meta = createElement("div", "repo-landing__footer-meta");
    meta.appendChild(createElement("span", "repo-landing__footer-meta-item", `Copyright © ${currentYear} ${PRODUCT_INFO.name}.`));
    meta.appendChild(createElement("span", "repo-landing__footer-meta-item repo-landing__footer-license", `Open source under ${PRODUCT_INFO.license}.`));
    brand.appendChild(meta);

    const links = createElement("div", "repo-landing__footer-links");
    const githubLink = createElement("a", "repo-landing__footer-link", "GitHub");
    githubLink.href = PRODUCT_INFO.repositoryUrl;
    githubLink.target = "_blank";
    githubLink.rel = "noopener noreferrer";
    githubLink.innerHTML = `${GITHUB_SVG}<span>GitHub</span>`;
    links.appendChild(githubLink);

    footer.appendChild(brand);
    footer.appendChild(links);
    return footer;
}

import { PRODUCT_INFO } from "./hostedProduct.js";
import { bindHostedPathNavigation } from "./hostedNavigation.js";

function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

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
    brandMark.src = "/favicon.svg";
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

    const brand = createElement("div", "repo-landing__footer-brand");
    brand.appendChild(createElement("strong", "", PRODUCT_INFO.name));
    brand.appendChild(createElement("span", "", "See what Git is actually doing."));

    const links = createElement("div", "repo-landing__footer-links");
    const githubLink = createElement("a", "repo-landing__footer-link", "GitHub");
    githubLink.href = PRODUCT_INFO.repositoryUrl;
    githubLink.target = "_blank";
    githubLink.rel = "noopener noreferrer";

    const youtubeLink = createElement("a", "repo-landing__footer-link", "YouTube");
    youtubeLink.href = PRODUCT_INFO.youtubeUrl;
    youtubeLink.target = "_blank";
    youtubeLink.rel = "noopener noreferrer";

    links.appendChild(githubLink);
    links.appendChild(youtubeLink);

    const license = createElement("span", "repo-landing__footer-license", PRODUCT_INFO.license);

    footer.appendChild(brand);
    footer.appendChild(links);
    footer.appendChild(license);
    return footer;
}


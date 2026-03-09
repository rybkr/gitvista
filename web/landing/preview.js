function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function resolvePreviewImageSrc(previewData) {
    const theme = document.documentElement.getAttribute("data-theme");
    if (theme === "dark") return previewData.images.dark;
    if (theme === "light") return previewData.images.light;

    const prefersDark = window.matchMedia?.("(prefers-color-scheme: dark)")?.matches ?? false;
    return prefersDark ? previewData.images.dark : previewData.images.light;
}

function createPreviewPicture(previewData) {
    const img = document.createElement("img");
    img.className = "repo-landing__preview-picture repo-landing__preview-image";
    img.src = resolvePreviewImageSrc(previewData);
    img.alt = previewData.alt;
    img.loading = "eager";
    img.decoding = "async";
    return img;
}

export function createHeroPreview(previewData) {
    const frame = createElement("figure", "repo-landing__preview-frame");
    const media = createElement("div", "repo-landing__preview-media");
    const image = createPreviewPicture(previewData);
    media.appendChild(image);
    frame.appendChild(media);

    const syncPreviewTheme = () => {
        image.src = resolvePreviewImageSrc(previewData);
    };

    const themeObserver = new MutationObserver(syncPreviewTheme);
    themeObserver.observe(document.documentElement, {
        attributes: true,
        attributeFilter: ["data-theme"],
    });

    const systemTheme = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (systemTheme) {
        if (typeof systemTheme.addEventListener === "function") {
            systemTheme.addEventListener("change", syncPreviewTheme);
        } else if (typeof systemTheme.addListener === "function") {
            systemTheme.addListener(syncPreviewTheme);
        }
    }

    frame.destroy = () => {
        themeObserver.disconnect();
        if (systemTheme) {
            if (typeof systemTheme.removeEventListener === "function") {
                systemTheme.removeEventListener("change", syncPreviewTheme);
            } else if (typeof systemTheme.removeListener === "function") {
                systemTheme.removeListener(syncPreviewTheme);
            }
        }
    };

    return frame;
}

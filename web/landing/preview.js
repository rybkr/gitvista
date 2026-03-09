function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function createPreviewPicture(previewData) {
    const picture = document.createElement("picture");
    picture.className = "repo-landing__preview-picture";

    const mobileDark = document.createElement("source");
    mobileDark.media = "(max-width: 760px) and (prefers-color-scheme: dark)";
    mobileDark.srcset = previewData.images.mobileDark;

    const mobileLight = document.createElement("source");
    mobileLight.media = "(max-width: 760px)";
    mobileLight.srcset = previewData.images.mobileLight;

    const desktopDark = document.createElement("source");
    desktopDark.media = "(prefers-color-scheme: dark)";
    desktopDark.srcset = previewData.images.desktopDark;

    const img = document.createElement("img");
    img.className = "repo-landing__preview-image";
    img.src = previewData.images.desktopLight;
    img.alt = previewData.alt;
    img.loading = "eager";
    img.decoding = "async";

    picture.appendChild(mobileDark);
    picture.appendChild(mobileLight);
    picture.appendChild(desktopDark);
    picture.appendChild(img);
    return picture;
}

function createChipRow(chips) {
    const row = createElement("div", "repo-landing__preview-chip-row");
    for (const chip of chips) {
        row.appendChild(createElement("span", "repo-landing__preview-chip", chip));
    }
    return row;
}

export function createHeroPreview(previewData) {
    const frame = createElement("figure", "repo-landing__preview-frame");

    const topbar = createElement("figcaption", "repo-landing__preview-topbar");
    const path = createElement("div", "repo-landing__preview-path", previewData.path);
    const badge = createElement("span", "repo-landing__preview-badge", previewData.badge);
    topbar.appendChild(path);
    topbar.appendChild(badge);

    const media = createElement("div", "repo-landing__preview-media");
    media.appendChild(createPreviewPicture(previewData));

    frame.appendChild(topbar);
    frame.appendChild(media);
    if (Array.isArray(previewData.chips) && previewData.chips.length > 0) {
        frame.appendChild(createChipRow(previewData.chips));
    }
    return frame;
}

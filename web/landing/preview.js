function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function createPreviewPicture(previewData) {
    const picture = document.createElement("picture");
    picture.className = "repo-landing__preview-picture";

    const stackedDark = document.createElement("source");
    stackedDark.media = "(max-width: 760px) and (prefers-color-scheme: dark)";
    stackedDark.srcset = previewData.images.desktopDark;

    const stackedLight = document.createElement("source");
    stackedLight.media = "(max-width: 760px)";
    stackedLight.srcset = previewData.images.desktopLight;

    const sideBySideDark = document.createElement("source");
    sideBySideDark.media = "(prefers-color-scheme: dark)";
    sideBySideDark.srcset = previewData.images.mobileDark;

    const img = document.createElement("img");
    img.className = "repo-landing__preview-image";
    img.src = previewData.images.mobileLight;
    img.alt = previewData.alt;
    img.loading = "eager";
    img.decoding = "async";

    picture.appendChild(stackedDark);
    picture.appendChild(stackedLight);
    picture.appendChild(sideBySideDark);
    picture.appendChild(img);
    return picture;
}

export function createHeroPreview(previewData) {
    const frame = createElement("figure", "repo-landing__preview-frame");
    const media = createElement("div", "repo-landing__preview-media");
    media.appendChild(createPreviewPicture(previewData));
    frame.appendChild(media);
    return frame;
}

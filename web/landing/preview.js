function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function createPreviewPicture(previewData) {
    const picture = document.createElement("picture");
    picture.className = "repo-landing__preview-picture";

    const darkSource = document.createElement("source");
    darkSource.media = "(prefers-color-scheme: dark)";
    darkSource.srcset = previewData.images.dark;

    const img = document.createElement("img");
    img.className = "repo-landing__preview-image";
    img.src = previewData.images.light;
    img.alt = previewData.alt;
    img.loading = "eager";
    img.decoding = "async";

    picture.appendChild(darkSource);
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

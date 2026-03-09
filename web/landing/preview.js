function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function createPreviewHeader({ kicker, title, body }) {
    const header = createElement("div", "repo-landing__preview-header");
    if (kicker) header.appendChild(createElement("span", "repo-landing__preview-kicker", kicker));
    if (title) header.appendChild(createElement("strong", "repo-landing__preview-heading", title));
    if (body) header.appendChild(createElement("p", "repo-landing__preview-copy", body));
    return header;
}

function createPreviewCommit(commit, tone) {
    const card = createElement(
        "article",
        `repo-landing__preview-commit repo-landing__preview-commit--${tone}${commit.emphasis === "active" ? " is-active" : ""}`,
    );
    card.appendChild(createElement("span", "repo-landing__preview-commit-hash", commit.hash));
    card.appendChild(createElement("strong", "repo-landing__preview-commit-title", commit.title));
    card.appendChild(createElement("span", "repo-landing__preview-commit-meta", commit.meta));
    return card;
}

function createPreviewLane(lane) {
    const laneEl = createElement("section", `repo-landing__preview-lane repo-landing__preview-lane--${lane.tone}`);
    laneEl.appendChild(createElement("span", "repo-landing__preview-lane-label", lane.label));

    const track = createElement("div", "repo-landing__preview-lane-track");
    for (const commit of lane.commits) {
        track.appendChild(createPreviewCommit(commit, lane.tone));
    }
    laneEl.appendChild(track);
    return laneEl;
}

function createPreviewGraph(graph) {
    const panel = createElement("section", "repo-landing__preview-graph-panel");
    panel.appendChild(createPreviewHeader({
        kicker: graph.kicker,
        title: graph.title,
        body: graph.summary,
    }));

    const lanes = createElement("div", "repo-landing__preview-lanes");
    for (const lane of graph.lanes) {
        lanes.appendChild(createPreviewLane(lane));
    }
    panel.appendChild(lanes);
    return panel;
}

function createFocusCard(focusCard) {
    const card = createElement("aside", "repo-landing__preview-focus");
    card.appendChild(createPreviewHeader({
        kicker: focusCard.kicker,
        title: focusCard.title,
        body: focusCard.summary,
    }));

    const pills = createElement("div", "repo-landing__preview-pill-row");
    for (const pill of focusCard.pills) {
        pills.appendChild(createElement("span", "repo-landing__preview-pill", pill));
    }
    card.appendChild(pills);
    return card;
}

function createChipRow(chips) {
    const row = createElement("div", "repo-landing__preview-chip-row");
    for (const chip of chips) {
        row.appendChild(createElement("span", "repo-landing__preview-chip", chip));
    }
    return row;
}

export function createHeroPreview(previewData) {
    const frame = createElement("div", "repo-landing__preview-frame");

    const topbar = createElement("div", "repo-landing__preview-topbar");
    const dots = createElement("div", "repo-landing__preview-dots");
    dots.appendChild(createElement("span"));
    dots.appendChild(createElement("span"));
    dots.appendChild(createElement("span"));
    topbar.appendChild(dots);
    topbar.appendChild(createElement("div", "repo-landing__preview-path", previewData.path));

    const body = createElement("div", "repo-landing__preview-body");
    body.appendChild(createPreviewGraph(previewData.graph));
    body.appendChild(createFocusCard(previewData.focusCard));

    frame.appendChild(topbar);
    frame.appendChild(body);
    if (Array.isArray(previewData.chips) && previewData.chips.length > 0) {
        frame.appendChild(createChipRow(previewData.chips));
    }
    return frame;
}

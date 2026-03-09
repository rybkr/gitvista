function createElement(tagName, className, text) {
    const el = document.createElement(tagName);
    if (className) el.className = className;
    if (typeof text === "string") el.textContent = text;
    return el;
}

function createPreviewPanelHeader({ kicker, title, body }) {
    const header = createElement("div", "repo-landing__preview-panel-header");
    header.appendChild(createElement("span", "repo-landing__preview-kicker", kicker));
    header.appendChild(createElement("strong", "", title));
    if (body) {
        header.appendChild(createElement("p", "repo-landing__preview-panel-copy", body));
    }
    return header;
}

function createPreviewProblem(problem) {
    const panel = createElement("section", "repo-landing__preview-panel repo-landing__preview-panel--problem");
    panel.appendChild(createPreviewPanelHeader(problem));

    const chaos = createElement("div", "repo-landing__preview-chaos");
    for (const line of problem.lines) {
        chaos.appendChild(createElement("div", "repo-landing__preview-chaos-row", line));
    }
    panel.appendChild(chaos);
    return panel;
}

function createPreviewGraphLane(lane) {
    const laneEl = createElement("div", `repo-landing__preview-lane repo-landing__preview-lane--${lane.tone}`);
    laneEl.appendChild(createElement("span", "repo-landing__preview-lane-label", lane.label));

    const commits = createElement("div", "repo-landing__preview-lane-commits");
    for (const commit of lane.commits) {
        const commitEl = createElement(
            "article",
            `repo-landing__preview-commit${commit.emphasis === "active" ? " repo-landing__preview-commit--active" : ""}`,
        );
        commitEl.appendChild(createElement("span", "repo-landing__preview-commit-hash", commit.hash));
        commitEl.appendChild(createElement("strong", "repo-landing__preview-commit-title", commit.title));
        commitEl.appendChild(createElement("span", "repo-landing__preview-commit-meta", commit.meta));
        commits.appendChild(commitEl);
    }

    laneEl.appendChild(commits);
    return laneEl;
}

function createPreviewGraph(graph) {
    const graphEl = createElement("section", "repo-landing__preview-graph");
    graphEl.appendChild(createPreviewPanelHeader({
        kicker: graph.label,
        title: "Branch movement first.",
        body: graph.summary,
    }));

    const lanes = createElement("div", "repo-landing__preview-lanes");
    for (const lane of graph.lanes) {
        lanes.appendChild(createPreviewGraphLane(lane));
    }
    graphEl.appendChild(lanes);
    return graphEl;
}

function createPreviewInspector(inspector) {
    const card = createElement("section", "repo-landing__preview-inspector");
    card.appendChild(createElement("span", "repo-landing__preview-label", inspector.label));
    card.appendChild(createElement("strong", "repo-landing__preview-inspector-title", inspector.title));
    card.appendChild(createElement("p", "repo-landing__preview-inspector-copy", inspector.summary));

    const pills = createElement("div", "repo-landing__preview-pill-row");
    for (const pill of inspector.pills) {
        pills.appendChild(createElement("span", "repo-landing__preview-pill", pill));
    }
    card.appendChild(pills);
    return card;
}

function createPreviewDiff(diff) {
    const diffEl = createElement("section", "repo-landing__preview-diff");

    const header = createElement("div", "repo-landing__preview-diff-header");
    const copy = createElement("div", "repo-landing__preview-diff-copy");
    copy.appendChild(createElement("span", "repo-landing__preview-label", diff.label));
    copy.appendChild(createElement("strong", "repo-landing__preview-diff-file", diff.file));
    header.appendChild(copy);

    const stats = createElement("div", "repo-landing__preview-diff-stats");
    for (const stat of diff.stats) {
        stats.appendChild(createElement("span", "repo-landing__preview-diff-stat", stat));
    }
    header.appendChild(stats);
    diffEl.appendChild(header);

    const excerpt = createElement("div", "repo-landing__preview-diff-excerpt");
    for (const line of diff.excerpt) {
        excerpt.appendChild(createElement("code", "repo-landing__preview-diff-line", line));
    }
    diffEl.appendChild(excerpt);
    return diffEl;
}

function createPreviewChecklist(items) {
    const list = createElement("div", "repo-landing__preview-checklist");
    for (const item of items) {
        const card = createElement("div", "repo-landing__preview-clarity-card");
        card.appendChild(createElement("strong", "", item.title));
        card.appendChild(createElement("span", "", item.body));
        list.appendChild(card);
    }
    return list;
}

function createPreviewChipRow(chips) {
    const row = createElement("div", "repo-landing__preview-chip-row");
    for (const chip of chips) {
        row.appendChild(createElement("span", "repo-landing__preview-chip", chip));
    }
    return row;
}

function createPreviewSolution(solution) {
    const panel = createElement("section", "repo-landing__preview-panel repo-landing__preview-panel--solution");
    panel.appendChild(createPreviewPanelHeader(solution));

    const canvas = createElement("div", "repo-landing__preview-canvas");
    canvas.appendChild(createPreviewGraph(solution.graph));

    const sidebar = createElement("div", "repo-landing__preview-sidebar");
    sidebar.appendChild(createPreviewInspector(solution.inspector));
    sidebar.appendChild(createPreviewDiff(solution.diff));
    canvas.appendChild(sidebar);

    panel.appendChild(canvas);
    panel.appendChild(createPreviewChecklist(solution.checklist));
    panel.appendChild(createPreviewChipRow(solution.chips));
    return panel;
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
    body.appendChild(createPreviewProblem(previewData.problem));
    body.appendChild(createPreviewSolution(previewData.solution));

    frame.appendChild(topbar);
    frame.appendChild(body);
    return frame;
}


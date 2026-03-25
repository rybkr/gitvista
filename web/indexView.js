function shortHash(hash) {
    if (!hash || typeof hash !== "string") return "unknown";
    return hash.slice(0, 8);
}

function sanitizeText(value, fallback = "Not available") {
    if (typeof value !== "string") return fallback;
    const trimmed = value.trim();
    return trimmed.length > 0 ? trimmed : fallback;
}

function splitRemoteUrl(url) {
    const value = sanitizeText(url, "unknown");
    const afterAt = value.includes("@") ? value.split("@").pop() : value;
    const compact = afterAt.replace(/^https?:\/\//, "");
    const withoutGit = compact.replace(/\.git$/, "");
    const [host, ...pathParts] = withoutGit.split(/[/:]/).filter(Boolean);

    return {
        host: host || "unknown",
        path: pathParts.join("/") || "repository",
        full: value,
    };
}

function setPill(el, label, tone) {
    el.textContent = label;
    el.className = tone ? `repo-pill repo-pill--${tone}` : "repo-pill";
}

function setMetricValue(el, value, title = "") {
    el.textContent = value;
    el.title = title || value;
}

export function createIndexView() {
    const el = document.createElement("div");
    el.className = "index-view";

    const hero = document.createElement("section");
    hero.className = "repo-overview-hero";

    const eyebrow = document.createElement("div");
    eyebrow.className = "repo-overview-eyebrow";
    eyebrow.textContent = "Repository overview";

    const title = document.createElement("h2");
    title.className = "repo-overview-title";
    title.textContent = "Repository";

    const pillRow = document.createElement("div");
    pillRow.className = "repo-overview-pills";

    const branchPill = document.createElement("span");
    setPill(branchPill, "No branch", "muted");

    const headPill = document.createElement("span");
    setPill(headPill, "HEAD unknown", "muted");

    pillRow.appendChild(branchPill);
    pillRow.appendChild(headPill);

    hero.appendChild(eyebrow);
    hero.appendChild(title);
    hero.appendChild(pillRow);

    const metricGrid = document.createElement("section");
    metricGrid.className = "repo-metric-grid";

    const commitValue = document.createElement("span");
    const branchValue = document.createElement("span");
    const tagValue = document.createElement("span");
    const remoteValue = document.createElement("span");

    const metricConfigs = [
        { label: "Commits", valueEl: commitValue },
        { label: "Branches", valueEl: branchValue },
        { label: "Tags", valueEl: tagValue },
        { label: "Remotes", valueEl: remoteValue },
    ];

    for (const metric of metricConfigs) {
        const card = document.createElement("article");
        card.className = "repo-metric-card";

        const cardLabel = document.createElement("span");
        cardLabel.className = "repo-metric-label";
        cardLabel.textContent = metric.label;

        metric.valueEl.className = "repo-metric-value";
        metric.valueEl.textContent = "0";

        card.appendChild(cardLabel);
        card.appendChild(metric.valueEl);
        metricGrid.appendChild(card);
    }

    const details = document.createElement("section");
    details.className = "repo-details-grid";

    const descriptionCard = document.createElement("article");
    descriptionCard.className = "repo-detail-card";

    const descriptionTitle = document.createElement("h3");
    descriptionTitle.className = "repo-detail-title";
    descriptionTitle.textContent = "Description";

    const descriptionBody = document.createElement("p");
    descriptionBody.className = "repo-description";
    descriptionBody.textContent = "No description";

    descriptionCard.appendChild(descriptionTitle);
    descriptionCard.appendChild(descriptionBody);

    const tagsCard = document.createElement("article");
    tagsCard.className = "repo-detail-card";

    const tagsTitle = document.createElement("h3");
    tagsTitle.className = "repo-detail-title";
    tagsTitle.textContent = "Recent tags";

    const tagsList = document.createElement("div");
    tagsList.className = "repo-tag-list";

    tagsCard.appendChild(tagsTitle);
    tagsCard.appendChild(tagsList);

    const remotesCard = document.createElement("article");
    remotesCard.className = "repo-detail-card";

    const remotesTitle = document.createElement("h3");
    remotesTitle.className = "repo-detail-title";
    remotesTitle.textContent = "Remotes";

    const remotesList = document.createElement("ul");
    remotesList.className = "repo-remote-list";

    remotesCard.appendChild(remotesTitle);
    remotesCard.appendChild(remotesList);

    details.appendChild(descriptionCard);
    details.appendChild(tagsCard);
    details.appendChild(remotesCard);

    el.appendChild(hero);
    el.appendChild(metricGrid);
    el.appendChild(details);

    let state = null;

    function render() {
        if (!state) return;

        const repoName = sanitizeText(state.name, "Repository");
        title.textContent = repoName;
        title.title = repoName;

        const branchName = sanitizeText(state.currentBranch, "");
        if (state.headDetached) {
            setPill(branchPill, "Detached HEAD", "warning");
        } else if (branchName) {
            setPill(branchPill, branchName, "branch");
        } else {
            setPill(branchPill, "No branch", "muted");
        }

        const hash = sanitizeText(state.headHash, "");
        setPill(headPill, hash ? `HEAD ${shortHash(hash)}` : "HEAD unknown", "head");

        setMetricValue(commitValue, String(state.commitCount ?? 0), "Total commits");
        setMetricValue(branchValue, String(state.branchCount ?? 0), "Total branches");
        setMetricValue(tagValue, String(state.tagCount ?? 0), "Total tags");
        const remoteCount = state.remotes ? Object.keys(state.remotes).length : 0;
        setMetricValue(remoteValue, String(remoteCount), "Configured remotes");

        descriptionBody.textContent = sanitizeText(state.description, "No description available.");

        tagsList.innerHTML = "";
        const tags = Array.isArray(state.tags) ? state.tags.slice(0, 10) : [];
        if (tags.length === 0) {
            const emptyTag = document.createElement("span");
            emptyTag.className = "repo-tag repo-tag--empty";
            emptyTag.textContent = "No tags";
            tagsList.appendChild(emptyTag);
        } else {
            for (const tag of tags) {
                const tagEl = document.createElement("span");
                tagEl.className = "repo-tag";
                tagEl.textContent = tag;
                tagEl.title = tag;
                tagsList.appendChild(tagEl);
            }
        }

        remotesList.innerHTML = "";
        const remotes = state.remotes ? Object.entries(state.remotes) : [];
        if (remotes.length === 0) {
            const emptyRow = document.createElement("li");
            emptyRow.className = "repo-remote repo-remote--empty";
            emptyRow.textContent = "No remotes configured";
            remotesList.appendChild(emptyRow);
        } else {
            for (const [name, url] of remotes) {
                const row = document.createElement("li");
                row.className = "repo-remote";

                const label = document.createElement("span");
                label.className = "repo-remote-name";
                label.textContent = name;

                const parsed = splitRemoteUrl(url);
                const host = document.createElement("span");
                host.className = "repo-remote-host";
                host.textContent = parsed.host;

                const path = document.createElement("span");
                path.className = "repo-remote-path";
                path.textContent = parsed.path;
                path.title = parsed.full;

                row.appendChild(label);
                row.appendChild(host);
                row.appendChild(path);
                remotesList.appendChild(row);
            }
        }
    }

    return {
        el,
        update(metadata) {
            if (!metadata) return;
            state = {
                ...state,
                ...metadata,
                tags: Array.isArray(metadata.tags) ? metadata.tags : state?.tags,
                remotes: metadata.remotes || state?.remotes,
            };
            render();
        },
        updateHead(headInfo) {
            if (!headInfo) return;
            state = {
                ...state,
                headHash: headInfo.hash,
                headDetached: headInfo.isDetached,
                currentBranch: headInfo.branchName,
                commitCount: headInfo.commitCount,
                branchCount: headInfo.branchCount,
                tagCount: headInfo.tagCount,
                description: headInfo.description,
                tags: Array.isArray(headInfo.recentTags) ? headInfo.recentTags : state?.tags,
                remotes: headInfo.remotes || state?.remotes,
            };
            render();
        },
    };
}

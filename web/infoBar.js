/**
 * InfoBar displays repository metadata in a collapsible section above the sidebar tabs.
 * Persists collapsed state to localStorage.
 */

const STORAGE_KEY = "gitvista-infobar";

export function createInfoBar() {
    const container = document.createElement("div");
    container.className = "info-bar";

    const savedState = localStorage.getItem(STORAGE_KEY);
    if (savedState === "collapsed") {
        container.classList.add("is-collapsed");
    }

    const header = document.createElement("button");
    header.className = "info-bar-header";
    header.setAttribute("aria-expanded", savedState !== "collapsed");
    header.addEventListener("click", () => {
        const isCollapsed = container.classList.toggle("is-collapsed");
        header.setAttribute("aria-expanded", !isCollapsed);
        localStorage.setItem(STORAGE_KEY, isCollapsed ? "collapsed" : "expanded");
    });

    const chevron = document.createElement("span");
    chevron.className = "info-bar-chevron";
    chevron.innerHTML = `<svg width="12" height="12" viewBox="0 0 12 12" fill="none" xmlns="http://www.w3.org/2000/svg">
        <path d="M4 3L7 6L4 9" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
    </svg>`;

    const title = document.createElement("span");
    title.className = "info-bar-title";
    title.textContent = "Repository Info";

    header.appendChild(chevron);
    header.appendChild(title);
    container.appendChild(header);

    const contentEl = document.createElement("div");
    contentEl.className = "info-bar-content";
    container.appendChild(contentEl);

    let lastMetadata = null;

    /**
     * Renders rows from a normalized descriptor, eliminating duplication
     * between the full-metadata and HEAD-only update paths.
     */
    function renderRows(descriptor) {
        contentEl.innerHTML = "";

        addRow(contentEl, "Repository", descriptor.repoName);

        if (descriptor.isDetached) {
            const hashShort = descriptor.headHash
                ? String(descriptor.headHash).substring(0, 7)
                : "unknown";
            addRow(contentEl, "HEAD", `(detached) ${hashShort}`, "detached");
        } else if (descriptor.branch) {
            addRow(contentEl, "Branch", descriptor.branch);
        }

        if (typeof descriptor.commitCount === "number") {
            addRow(contentEl, "Commits", String(descriptor.commitCount));
        }

        if (typeof descriptor.branchCount === "number") {
            addRow(contentEl, "Branches", String(descriptor.branchCount));
        }

        if (typeof descriptor.tagCount === "number" && descriptor.tagCount > 0) {
            const tags = descriptor.tags;
            const tagValue = tags && tags.length > 0
                ? `${descriptor.tagCount} (${tags.slice(0, 3).join(", ")}${tags.length > 3 ? "..." : ""})`
                : String(descriptor.tagCount);
            addRow(contentEl, "Tags", tagValue);
        }

        if (descriptor.description) {
            addRow(contentEl, "Description", descriptor.description);
        }

        if (descriptor.remotes && Object.keys(descriptor.remotes).length > 0) {
            for (const [name, url] of Object.entries(descriptor.remotes)) {
                addRow(contentEl, `Remote: ${name}`, url, "remote-url");
            }
        }
    }

    function update(data) {
        lastMetadata = data;
        renderRows({
            repoName: data.name || "Unknown",
            isDetached: data.headDetached,
            headHash: data.headHash,
            branch: data.currentBranch,
            commitCount: data.commitCount,
            branchCount: data.branchCount,
            tagCount: data.tagCount,
            tags: data.tags,
            description: data.description,
            remotes: data.remotes,
        });
    }

    function updateHead(headInfo) {
        renderRows({
            repoName: lastMetadata?.name || "Repository",
            isDetached: headInfo.isDetached,
            headHash: headInfo.hash,
            branch: headInfo.branchName,
            commitCount: headInfo.commitCount,
            branchCount: headInfo.branchCount,
            tagCount: headInfo.tagCount,
            tags: headInfo.recentTags,
            description: headInfo.description,
            remotes: headInfo.remotes,
        });
    }

    function addRow(parent, label, value, modifier) {
        const row = document.createElement("div");
        row.className = "info-bar-row";

        const labelEl = document.createElement("span");
        labelEl.className = "info-bar-label";
        labelEl.textContent = label;

        const valueEl = document.createElement("span");
        valueEl.className = modifier ? `info-bar-value info-bar-value--${modifier}` : "info-bar-value";
        valueEl.textContent = value;
        valueEl.title = value;

        row.appendChild(labelEl);
        row.appendChild(valueEl);
        parent.appendChild(row);
    }

    return {
        el: container,
        update,
        updateHead,
    };
}

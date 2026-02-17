/**
 * InfoBar displays repository metadata in a collapsible section above the sidebar tabs.
 * Persists collapsed state to localStorage.
 */

const STORAGE_KEY = "gitvista-infobar";

/**
 * Creates an info bar component.
 * @returns {{el: HTMLElement, update: Function, updateHead: Function}}
 */
export function createInfoBar() {
    const container = document.createElement("div");
    container.className = "info-bar";

    // Restore collapsed state from localStorage
    const savedState = localStorage.getItem(STORAGE_KEY);
    if (savedState === "collapsed") {
        container.classList.add("is-collapsed");
    }

    // Header with toggle button
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

    // Content section
    const content = document.createElement("div");
    content.className = "info-bar-content";
    container.appendChild(content);

    /**
     * Update the info bar with full repository metadata.
     * @param {Object} data - Repository metadata from /api/repository
     */
    function update(data) {
        content.innerHTML = "";

        // Repository name
        addRow(content, "Repository", data.name || "Unknown");

        // Current branch/HEAD with detached indicator
        if (data.headDetached) {
            const hashShort = data.headHash ? String(data.headHash).substring(0, 7) : "unknown";
            addRow(content, "HEAD", `(detached) ${hashShort}`, "detached");
        } else if (data.currentBranch) {
            addRow(content, "Branch", data.currentBranch);
        }

        // Commit count
        if (typeof data.commitCount === "number") {
            addRow(content, "Commits", String(data.commitCount));
        }

        // Branch count
        if (typeof data.branchCount === "number") {
            addRow(content, "Branches", String(data.branchCount));
        }

        // Tag count and recent tags
        if (typeof data.tagCount === "number" && data.tagCount > 0) {
            const tagValue = data.tags && data.tags.length > 0
                ? `${data.tagCount} (${data.tags.slice(0, 3).join(", ")}${data.tags.length > 3 ? "..." : ""})`
                : String(data.tagCount);
            addRow(content, "Tags", tagValue);
        }

        // Description (if set)
        if (data.description) {
            addRow(content, "Description", data.description);
        }

        // Remotes
        if (data.remotes && Object.keys(data.remotes).length > 0) {
            for (const [name, url] of Object.entries(data.remotes)) {
                addRow(content, `Remote: ${name}`, url, "remote-url");
            }
        }
    }

    /**
     * Update only the HEAD-related fields (for real-time updates via WebSocket).
     * @param {Object} headInfo - HeadInfo from WebSocket message
     */
    function updateHead(headInfo) {
        content.innerHTML = "";

        // Extract repo name from existing data or use a placeholder
        const repoName = content.dataset.repoName || "Repository";
        addRow(content, "Repository", repoName);

        // Current branch/HEAD
        if (headInfo.isDetached) {
            const hashShort = headInfo.hash ? headInfo.hash.substring(0, 7) : "unknown";
            addRow(content, "HEAD", `(detached) ${hashShort}`, "detached");
        } else if (headInfo.branchName) {
            addRow(content, "Branch", headInfo.branchName);
        }

        // Commit count
        if (typeof headInfo.commitCount === "number") {
            addRow(content, "Commits", String(headInfo.commitCount));
        }

        // Branch count
        if (typeof headInfo.branchCount === "number") {
            addRow(content, "Branches", String(headInfo.branchCount));
        }

        // Tag count and recent tags
        if (typeof headInfo.tagCount === "number" && headInfo.tagCount > 0) {
            const tagValue = headInfo.recentTags && headInfo.recentTags.length > 0
                ? `${headInfo.tagCount} (${headInfo.recentTags.slice(0, 3).join(", ")}${headInfo.recentTags.length > 3 ? "..." : ""})`
                : String(headInfo.tagCount);
            addRow(content, "Tags", tagValue);
        }

        // Description (if set)
        if (headInfo.description) {
            addRow(content, "Description", headInfo.description);
        }

        // Remotes
        if (headInfo.remotes && Object.keys(headInfo.remotes).length > 0) {
            for (const [name, url] of Object.entries(headInfo.remotes)) {
                addRow(content, `Remote: ${name}`, url, "remote-url");
            }
        }
    }

    /**
     * Helper to add a key-value row to the content.
     * @param {HTMLElement} parent - Parent element to append to
     * @param {string} label - Label text
     * @param {string} value - Value text
     * @param {string} [modifier] - Optional CSS modifier class
     */
    function addRow(parent, label, value, modifier) {
        const row = document.createElement("div");
        row.className = "info-bar-row";

        const labelEl = document.createElement("span");
        labelEl.className = "info-bar-label";
        labelEl.textContent = label;

        const valueEl = document.createElement("span");
        valueEl.className = modifier ? `info-bar-value info-bar-value--${modifier}` : "info-bar-value";
        valueEl.textContent = value;
        valueEl.title = value; // Tooltip for long values

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

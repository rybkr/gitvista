/**
 * @fileoverview Breadcrumb bar showing branch, commit hash, message, and position counter.
 * Sits between the toolbar and canvas for graph orientation.
 */

/**
 * Creates the breadcrumb bar component.
 *
 * @param {{ onBranchClick?: (branch: string) => void }} [options]
 * @returns {{ el: HTMLElement, update: (info: object) => void, destroy: () => void }}
 */
export function createGraphBreadcrumb(options = {}) {
    const el = document.createElement("div");
    el.className = "graph-breadcrumb";

    const branchEl = document.createElement("span");
    branchEl.className = "graph-breadcrumb__branch";

    const sep1 = document.createElement("span");
    sep1.className = "graph-breadcrumb__sep";
    sep1.textContent = "\u203A"; // ›

    const hashEl = document.createElement("span");
    hashEl.className = "graph-breadcrumb__hash";

    const msgEl = document.createElement("span");
    msgEl.className = "graph-breadcrumb__message";

    const posEl = document.createElement("span");
    posEl.className = "graph-breadcrumb__position";

    const emptyEl = document.createElement("span");
    emptyEl.className = "graph-breadcrumb--empty";
    emptyEl.textContent = "No commit selected";

    el.appendChild(emptyEl);

    branchEl.addEventListener("click", () => {
        const name = branchEl.dataset.branch;
        if (name && options.onBranchClick) {
            options.onBranchClick(name);
        }
    });

    let currentState = null;

    /**
     * Updates the breadcrumb display.
     *
     * @param {{
     *   branch?: string|null,
     *   hash?: string|null,
     *   message?: string|null,
     *   index?: number,
     *   total?: number,
     * }} info
     */
    function update(info) {
        currentState = info;
        el.innerHTML = "";

        if (!info.hash) {
            el.appendChild(emptyEl);
            return;
        }

        if (info.branch) {
            // Strip refs/heads/ prefix for display
            const displayBranch = info.branch.replace(/^refs\/heads\//, "");
            branchEl.textContent = displayBranch;
            branchEl.dataset.branch = info.branch;
            branchEl.title = info.branch;
            el.appendChild(branchEl);
            el.appendChild(sep1);
        }

        hashEl.textContent = info.hash.slice(0, 7);
        hashEl.title = info.hash;
        el.appendChild(hashEl);

        if (info.message) {
            const trimmed = info.message.length > 60
                ? info.message.slice(0, 57) + "\u2026"
                : info.message;
            msgEl.textContent = "\u201C" + trimmed + "\u201D";
            msgEl.title = info.message;
            el.appendChild(msgEl);
        }

        if (info.index > 0 && info.total > 0) {
            posEl.textContent = `Commit ${info.index} of ${info.total}`;
            el.appendChild(posEl);
        }
    }

    function destroy() {
        el.remove();
    }

    return { el, update, destroy };
}

/**
 * Three-way merge diff viewer for displaying conflict regions.
 *
 * Renders ThreeWayFileDiff regions with classified sections:
 * - context: unchanged lines shared by all three versions
 * - ours: lines changed only on our side
 * - theirs: lines changed only on their side
 * - conflict: overlapping changes from both sides with base shown
 *
 * Follows the established patterns from diffContentViewer.js.
 */

import { loadHighlightJs, getLanguageFromPath } from "./hljs.js";

const hljsReady = loadHighlightJs();

const BACK_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M10 4L6 8l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

export function createMergePreviewDiffViewer() {
    const el = document.createElement("div");
    el.className = "merge-diff-viewer";
    el.style.display = "none";

    let onBackCallback = null;

    function highlightLine(text, hljs, language) {
        if (!hljs || !language || !text) return null;
        try {
            return hljs.highlight(text, { language, ignoreIllegals: true }).value;
        } catch {
            return null;
        }
    }

    function createCodeLine(text, className, hljs, language) {
        const line = document.createElement("div");
        line.className = `merge-diff-line ${className}`;

        const content = document.createElement("span");
        content.className = "line-content";

        const highlighted = highlightLine(text, hljs, language);
        if (highlighted) {
            content.innerHTML = highlighted;
        } else {
            content.textContent = text;
        }

        line.appendChild(content);
        return line;
    }

    function renderRegionHeader(label, className) {
        const header = document.createElement("div");
        header.className = `merge-region-header ${className}`;
        header.textContent = label;
        return header;
    }

    function renderContextRegion(region, hljs, language) {
        const container = document.createElement("div");
        container.className = "merge-region merge-region--context";

        for (const line of region.baseLines) {
            container.appendChild(createCodeLine(line, "diff-line-context", hljs, language));
        }
        return container;
    }

    function renderOursRegion(region, hljs, language) {
        const container = document.createElement("div");
        container.className = "merge-region merge-region--ours";
        container.appendChild(renderRegionHeader("Ours", "merge-region-header--ours"));

        for (const line of region.baseLines) {
            container.appendChild(createCodeLine(line, "diff-line-delete", hljs, language));
        }
        for (const line of (region.oursLines || [])) {
            container.appendChild(createCodeLine(line, "diff-line-add", hljs, language));
        }
        return container;
    }

    function renderTheirsRegion(region, hljs, language) {
        const container = document.createElement("div");
        container.className = "merge-region merge-region--theirs";
        container.appendChild(renderRegionHeader("Theirs", "merge-region-header--theirs"));

        for (const line of region.baseLines) {
            container.appendChild(createCodeLine(line, "diff-line-delete", hljs, language));
        }
        for (const line of (region.theirsLines || [])) {
            container.appendChild(createCodeLine(line, "diff-line-theirs-add", hljs, language));
        }
        return container;
    }

    function renderConflictRegion(region, hljs, language) {
        const container = document.createElement("div");
        container.className = "merge-region merge-conflict-region";
        container.appendChild(renderRegionHeader("Conflict", "merge-region-header--conflict"));

        // Base sub-section
        if (region.baseLines && region.baseLines.length > 0) {
            const baseHeader = document.createElement("div");
            baseHeader.className = "merge-conflict-section-header merge-conflict-section-header--base";
            baseHeader.textContent = "Base";
            container.appendChild(baseHeader);

            for (const line of region.baseLines) {
                container.appendChild(createCodeLine(line, "diff-line-delete merge-diff-line--base", hljs, language));
            }
        }

        // Ours sub-section
        if (region.oursLines && region.oursLines.length > 0) {
            const oursHeader = document.createElement("div");
            oursHeader.className = "merge-conflict-section-header merge-conflict-section-header--ours";
            oursHeader.textContent = "Ours";
            container.appendChild(oursHeader);

            for (const line of region.oursLines) {
                container.appendChild(createCodeLine(line, "diff-line-add", hljs, language));
            }
        }

        // Theirs sub-section
        if (region.theirsLines && region.theirsLines.length > 0) {
            const theirsHeader = document.createElement("div");
            theirsHeader.className = "merge-conflict-section-header merge-conflict-section-header--theirs";
            theirsHeader.textContent = "Theirs";
            container.appendChild(theirsHeader);

            for (const line of region.theirsLines) {
                container.appendChild(createCodeLine(line, "diff-line-theirs-add", hljs, language));
            }
        }

        return container;
    }

    function renderStats(stats) {
        const bar = document.createElement("div");
        bar.className = "merge-diff-stats";

        if (stats.oursAdded || stats.oursDeleted) {
            const ours = document.createElement("span");
            ours.className = "merge-diff-stat merge-diff-stat--ours";
            ours.textContent = `Ours: +${stats.oursAdded} -${stats.oursDeleted}`;
            bar.appendChild(ours);
        }

        if (stats.theirsAdded || stats.theirsDeleted) {
            const theirs = document.createElement("span");
            theirs.className = "merge-diff-stat merge-diff-stat--theirs";
            theirs.textContent = `Theirs: +${stats.theirsAdded} -${stats.theirsDeleted}`;
            bar.appendChild(theirs);
        }

        if (stats.conflictRegions > 0) {
            const conflicts = document.createElement("span");
            conflicts.className = "merge-diff-stat merge-diff-stat--conflict";
            conflicts.textContent = `${stats.conflictRegions} conflict${stats.conflictRegions !== 1 ? "s" : ""}`;
            bar.appendChild(conflicts);
        }

        return bar;
    }

    async function show(data) {
        const hljs = await hljsReady;
        const language = getLanguageFromPath(data.path);

        el.style.display = "flex";
        el.innerHTML = "";

        // Back button
        const backBtn = document.createElement("button");
        backBtn.className = "diff-content-back";
        backBtn.innerHTML = BACK_SVG + " Back";
        backBtn.addEventListener("click", () => {
            if (onBackCallback) onBackCallback();
        });
        el.appendChild(backBtn);

        // File header
        const header = document.createElement("div");
        header.className = "diff-file-header";

        const conflictBadge = document.createElement("span");
        conflictBadge.className = "merge-preview-badge merge-status--conflict";
        conflictBadge.textContent = "!!";
        header.appendChild(conflictBadge);

        const pathEl = document.createElement("span");
        pathEl.className = "diff-file-path";
        pathEl.textContent = data.path;
        header.appendChild(pathEl);

        el.appendChild(header);

        // Stats
        if (data.stats) {
            el.appendChild(renderStats(data.stats));
        }

        // Content body
        const body = document.createElement("div");
        body.className = "diff-content-body";

        if (data.isBinary) {
            const msg = document.createElement("div");
            msg.className = "diff-binary-notice";
            msg.textContent = "Binary file — cannot display three-way diff";
            body.appendChild(msg);
        } else if (data.truncated) {
            const msg = document.createElement("div");
            msg.className = "diff-truncated-notice";
            msg.textContent = "File too large to display three-way diff";
            body.appendChild(msg);
        } else if (data.regions && data.regions.length > 0) {
            const regionsContainer = document.createElement("div");
            regionsContainer.className = "merge-diff-regions";

            for (const region of data.regions) {
                switch (region.type) {
                    case "context":
                        regionsContainer.appendChild(renderContextRegion(region, hljs, language));
                        break;
                    case "ours":
                        regionsContainer.appendChild(renderOursRegion(region, hljs, language));
                        break;
                    case "theirs":
                        regionsContainer.appendChild(renderTheirsRegion(region, hljs, language));
                        break;
                    case "conflict":
                        regionsContainer.appendChild(renderConflictRegion(region, hljs, language));
                        break;
                }
            }

            body.appendChild(regionsContainer);
        } else {
            const msg = document.createElement("div");
            msg.className = "merge-preview-message";
            msg.textContent = "No differences to display.";
            body.appendChild(msg);
        }

        el.appendChild(body);
    }

    function showLoading() {
        el.style.display = "flex";
        el.innerHTML = "";

        const loading = document.createElement("div");
        loading.className = "diff-content-loading";

        const spinner = document.createElement("div");
        spinner.className = "diff-loading-spinner";
        loading.appendChild(spinner);

        const text = document.createElement("div");
        text.className = "diff-loading-text";
        text.textContent = "Loading merge diff...";
        loading.appendChild(text);

        el.appendChild(loading);
    }

    function showError(message) {
        el.style.display = "flex";
        el.innerHTML = "";

        const errorEl = document.createElement("div");
        errorEl.className = "diff-content-error";
        errorEl.textContent = message;
        el.appendChild(errorEl);
    }

    function close() {
        el.style.display = "none";
        el.innerHTML = "";
    }

    function onBack(callback) {
        onBackCallback = callback;
    }

    return { el, show, showLoading, showError, close, onBack };
}

/**
 * Diff content viewer for displaying line-level unified diffs.
 *
 * Renders hunks with dual line number gutters (old/new), syntax highlighting
 * for added/deleted/context lines, and handles binary files and truncated diffs.
 * Follows the established ES module pattern from fileContentViewer.js.
 *
 * Expand-context: "↕ Expand context" buttons appear between hunks where hidden
 * lines exist. Each click re-fetches the diff with context increased by 5 lines.
 */

const BACK_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M10 4L6 8l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

// How many extra context lines each "Expand context" click adds
const CONTEXT_EXPAND_STEP = 5;

// Default context lines — must match the backend defaultContextLines constant
const DEFAULT_CONTEXT_LINES = 3;

export function createDiffContentViewer() {
    const el = document.createElement("div");
    el.className = "diff-content-viewer";
    el.style.display = "none"; // Hidden by default

    let onBackCallback = null;

    // State tracking for the currently displayed diff, used by expand-context
    let currentFetchUrl = null;       // URL used to fetch the current diff
    let currentContextLines = DEFAULT_CONTEXT_LINES;

    /**
     * Create a status badge element for a given file status.
     * Returns a DOM element (not an HTML string) to avoid XSS via file paths.
     */
    function createStatusBadge(status) {
        const badges = {
            added: { text: "A", cls: "diff-status-badge--added" },
            modified: { text: "M", cls: "diff-status-badge--modified" },
            deleted: { text: "D", cls: "diff-status-badge--deleted" },
            renamed: { text: "R", cls: "diff-status-badge--renamed" },
            copied: { text: "C", cls: "diff-status-badge--copied" },
        };
        const badge = badges[status] || { text: "?", cls: "" };
        const el = document.createElement("span");
        el.className = badge.cls ? `diff-status-badge ${badge.cls}` : "diff-status-badge";
        el.textContent = badge.text;
        return el;
    }

    /**
     * Render hunk header showing line ranges.
     * Format: @@ -oldStart,oldCount +newStart,newCount @@
     */
    function renderHunkHeader(hunk) {
        const header = document.createElement("div");
        header.className = "diff-hunk-header";
        header.textContent = `@@ -${hunk.oldStart},${hunk.oldLines} +${hunk.newStart},${hunk.newLines} @@`;
        return header;
    }

    /**
     * Render a single diff line with dual line number gutters.
     */
    function renderLine(line) {
        const lineEl = document.createElement("div");

        // Determine line type class
        let lineClass = "diff-line";
        if (line.type === "addition") {
            lineClass += " diff-line-add";
        } else if (line.type === "deletion") {
            lineClass += " diff-line-delete";
        } else {
            lineClass += " diff-line-context";
        }
        lineEl.className = lineClass;

        // Old line number (left gutter)
        const oldNum = document.createElement("span");
        oldNum.className = "line-num-old";
        oldNum.textContent = line.oldLine ? String(line.oldLine) : "";
        lineEl.appendChild(oldNum);

        // New line number (right gutter)
        const newNum = document.createElement("span");
        newNum.className = "line-num-new";
        newNum.textContent = line.newLine ? String(line.newLine) : "";
        lineEl.appendChild(newNum);

        // Line content
        const content = document.createElement("span");
        content.className = "line-content";
        content.textContent = line.content;
        lineEl.appendChild(content);

        return lineEl;
    }

    /**
     * Render a complete hunk with header and lines.
     */
    function renderHunk(hunk) {
        const hunkEl = document.createElement("div");
        hunkEl.className = "diff-hunk";

        // Hunk header
        hunkEl.appendChild(renderHunkHeader(hunk));

        // All lines in the hunk
        for (const line of hunk.lines) {
            hunkEl.appendChild(renderLine(line));
        }

        return hunkEl;
    }

    /**
     * Render an "expand context" button shown between two adjacent hunks when
     * there are hidden lines between them.  Clicking re-fetches the diff with
     * currentContextLines increased by CONTEXT_EXPAND_STEP.
     *
     * @param {number} hiddenCount - Number of lines hidden between the two hunks
     * @param {HTMLElement} hunksContainer - Container to replace on re-fetch
     * @param {Object} fileDiff - The current FileDiff being shown
     */
    function renderExpandButton(hiddenCount, hunksContainer, fileDiff) {
        const btn = document.createElement("button");
        btn.className = "diff-expand-context";
        btn.innerHTML = `\u2195 Show ${hiddenCount} hidden line${hiddenCount !== 1 ? "s" : ""} &mdash; Expand context`;
        btn.title = `Re-fetch with ${currentContextLines + CONTEXT_EXPAND_STEP} context lines`;

        btn.addEventListener("click", () => {
            if (!currentFetchUrl) return;

            // Increment context for this session
            currentContextLines += CONTEXT_EXPAND_STEP;

            // Build the new URL with the updated ?context= parameter
            const url = new URL(currentFetchUrl, window.location.origin);
            url.searchParams.set("context", String(currentContextLines));

            showLoading();
            fetch(url.toString())
                .then((res) => {
                    if (!res.ok) throw new Error(`HTTP ${res.status}`);
                    return res.json();
                })
                .then((newDiff) => {
                    // Preserve status from the original diff since working-tree
                    // diffs returned by the API may not carry it in the re-fetch
                    if (!newDiff.status) {
                        newDiff.status = fileDiff.status;
                    }
                    show(newDiff);
                })
                .catch((err) => {
                    showError(`Failed to expand context: ${err.message}`);
                });
        });

        return btn;
    }

    /**
     * Compute the number of source lines that fall between two consecutive hunks.
     * This is the gap in old-file line numbers between the end of hunkA and the
     * start of hunkB.  Returns 0 when the hunks are adjacent.
     */
    function hiddenLinesBetween(hunkA, hunkB) {
        const endOfA = hunkA.oldStart + hunkA.oldLines - 1;
        const startOfB = hunkB.oldStart;
        const gap = startOfB - endOfA - 1;
        return gap > 0 ? gap : 0;
    }

    /**
     * Display a FileDiff object.
     *
     * @param {Object} fileDiff - The file diff object from the API
     * @param {string} fileDiff.path - File path
     * @param {string} fileDiff.status - File status (added/modified/deleted)
     * @param {boolean} fileDiff.isBinary - Whether file is binary
     * @param {Array} fileDiff.hunks - Array of diff hunks
     * @param {boolean} fileDiff.truncated - Whether diff was truncated
     */
    function show(fileDiff) {
        el.style.display = "flex";
        el.innerHTML = "";

        // Back button
        const backBtn = document.createElement("button");
        backBtn.className = "diff-content-back";
        backBtn.innerHTML = BACK_SVG + " Back";
        backBtn.addEventListener("click", () => {
            if (onBackCallback) {
                onBackCallback();
            }
        });
        el.appendChild(backBtn);

        // File header with status badge and path — use DOM construction to avoid XSS
        const header = document.createElement("div");
        header.className = "diff-file-header";
        header.appendChild(createStatusBadge(fileDiff.status));
        const pathEl = document.createElement("span");
        pathEl.className = "diff-file-path";
        pathEl.textContent = fileDiff.path;
        header.appendChild(pathEl);
        el.appendChild(header);

        // Content body
        const body = document.createElement("div");
        body.className = "diff-content-body";

        // Handle special cases
        if (fileDiff.isBinary) {
            // Binary file notice
            const binaryMsg = document.createElement("div");
            binaryMsg.className = "diff-binary-notice";
            binaryMsg.textContent = "Binary file changed";
            body.appendChild(binaryMsg);
        } else if (fileDiff.status === "added") {
            // New file notice (still show hunks if available)
            const newFileMsg = document.createElement("div");
            newFileMsg.className = "diff-file-notice diff-file-notice--added";
            newFileMsg.textContent = "New file";
            body.appendChild(newFileMsg);
        } else if (fileDiff.status === "deleted") {
            // Deleted file notice (still show hunks if available)
            const deletedFileMsg = document.createElement("div");
            deletedFileMsg.className = "diff-file-notice diff-file-notice--deleted";
            deletedFileMsg.textContent = "Deleted file";
            body.appendChild(deletedFileMsg);
        } else if (fileDiff.status === "untracked") {
            // Untracked file — no diff against HEAD is available
            const untrackedMsg = document.createElement("div");
            untrackedMsg.className = "diff-file-notice diff-file-notice--untracked";
            untrackedMsg.textContent = "New file \u2014 not yet tracked";
            body.appendChild(untrackedMsg);
        }

        // Render hunks, inserting expand-context buttons between non-adjacent hunks
        if (!fileDiff.isBinary && fileDiff.hunks && fileDiff.hunks.length > 0) {
            const hunksContainer = document.createElement("div");
            hunksContainer.className = "diff-hunks";

            for (let i = 0; i < fileDiff.hunks.length; i++) {
                const hunk = fileDiff.hunks[i];

                // Insert an expand button before this hunk when there is a gap
                // between it and the previous one, indicating hidden context lines
                if (i > 0 && currentFetchUrl) {
                    const hidden = hiddenLinesBetween(fileDiff.hunks[i - 1], hunk);
                    if (hidden > 0) {
                        hunksContainer.appendChild(
                            renderExpandButton(hidden, hunksContainer, fileDiff)
                        );
                    }
                }

                hunksContainer.appendChild(renderHunk(hunk));
            }

            body.appendChild(hunksContainer);
        }

        // Truncation notice
        if (fileDiff.truncated) {
            const truncMsg = document.createElement("div");
            truncMsg.className = "diff-truncated-notice";
            truncMsg.textContent = "Diff truncated (too large to display completely)";
            body.appendChild(truncMsg);
        }

        el.appendChild(body);
    }

    /**
     * Show a loading indicator while a file diff is being fetched.
     */
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
        text.textContent = "Loading file diff...";
        loading.appendChild(text);

        el.appendChild(loading);
    }

    /**
     * Show an error message when a file diff fails to load.
     * @param {string} message - The error message to display
     */
    function showError(message) {
        el.style.display = "flex";
        el.innerHTML = "";

        const errorEl = document.createElement("div");
        errorEl.className = "diff-content-error";
        errorEl.textContent = message;
        el.appendChild(errorEl);
    }

    /**
     * Fetch and display a diff from a URL.
     * Tracks the URL so that expand-context buttons can re-fetch with more context.
     *
     * @param {string} url - Full URL for the diff API endpoint (without ?context=)
     */
    async function showFromUrl(url) {
        // Reset context depth for each new file view
        currentFetchUrl = url;
        currentContextLines = DEFAULT_CONTEXT_LINES;

        showLoading();
        try {
            const res = await fetch(url);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const fileDiff = await res.json();
            show(fileDiff);
        } catch (err) {
            showError(`Failed to load diff: ${err.message}`);
        }
    }

    function close() {
        el.style.display = "none";
        el.innerHTML = "";
        currentFetchUrl = null;
        currentContextLines = DEFAULT_CONTEXT_LINES;
    }

    // Alias retained for call sites that use "clear" semantics.
    const clear = close;

    /**
     * Register a callback for when the back button is clicked.
     */
    function onBack(callback) {
        onBackCallback = callback;
    }

    return {
        el,
        show,
        showFromUrl,
        showLoading,
        showError,
        close,
        clear,
        onBack,
    };
}

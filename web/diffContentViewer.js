/**
 * Diff content viewer for displaying line-level unified diffs.
 *
 * Renders hunks with dual line number gutters (old/new), syntax highlighting
 * for added/deleted/context lines, and handles binary files and truncated diffs.
 * Follows the established ES module pattern from fileContentViewer.js.
 */

const BACK_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M10 4L6 8l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

export function createDiffContentViewer() {
    const el = document.createElement("div");
    el.className = "diff-content-viewer";
    el.style.display = "none"; // Hidden by default

    let onBackCallback = null;
    let onFileSelectCallback = null;

    /**
     * Render a status badge based on file status.
     */
    function getStatusBadge(status) {
        const badges = {
            added: { text: "A", class: "diff-status-badge--added" },
            modified: { text: "M", class: "diff-status-badge--modified" },
            deleted: { text: "D", class: "diff-status-badge--deleted" },
            renamed: { text: "R", class: "diff-status-badge--renamed" },
            copied: { text: "C", class: "diff-status-badge--copied" },
        };
        const badge = badges[status] || { text: "?", class: "diff-status-badge" };
        return `<span class="diff-status-badge ${badge.class}">${badge.text}</span>`;
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

        // File header with status badge and path
        const header = document.createElement("div");
        header.className = "diff-file-header";
        header.innerHTML = `
            ${getStatusBadge(fileDiff.status)}
            <span class="diff-file-path">${fileDiff.path}</span>
        `;
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
        }

        // Render hunks
        if (!fileDiff.isBinary && fileDiff.hunks && fileDiff.hunks.length > 0) {
            const hunksContainer = document.createElement("div");
            hunksContainer.className = "diff-hunks";

            for (const hunk of fileDiff.hunks) {
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
     * Close the viewer and hide it (alias for clear).
     */
    function close() {
        el.style.display = "none";
        el.innerHTML = "";
    }

    /**
     * Clear the viewer and hide it.
     */
    function clear() {
        el.style.display = "none";
        el.innerHTML = "";
    }

    /**
     * Register a callback for when the back button is clicked.
     */
    function onBack(callback) {
        onBackCallback = callback;
    }

    /**
     * Register a callback for when user selects a different file.
     * (Optional feature for future multi-file navigation)
     */
    function onFileSelect(callback) {
        onFileSelectCallback = callback;
    }

    return {
        el,
        show,
        showLoading,
        showError,
        close,
        clear,
        onBack,
        onFileSelect,
    };
}

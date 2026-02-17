/**
 * File content viewer for displaying blob contents.
 *
 * Fetches blob data from the API and renders it with line numbers.
 * Handles binary files, truncation, loading states, and errors.
 * Extracted from fileBrowser.js for reuse across different contexts.
 */

import { getFileIcon } from "./fileIcons.js";

const BACK_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M10 4L6 8l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

export function createFileContentViewer() {
    const el = document.createElement("div");
    el.className = "file-content";
    el.style.display = "none"; // Hidden by default

    let onBackCallback = null;

    async function fetchBlob(blobHash) {
        const response = await fetch(`/api/blob/${blobHash}`);
        if (!response.ok) {
            throw new Error(`Failed to fetch blob ${blobHash}: ${response.status}`);
        }
        return response.json();
    }

    function formatSize(bytes) {
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    }

    async function open(blobHash, fileName) {
        el.style.display = "flex";
        el.innerHTML = '<div class="file-content-loading">Loading...</div>';

        try {
            const blobData = await fetchBlob(blobHash);

            el.innerHTML = "";

            // Back button
            const backBtn = document.createElement("button");
            backBtn.className = "file-content-back";
            backBtn.innerHTML = BACK_SVG + " Back";
            backBtn.addEventListener("click", () => {
                if (onBackCallback) {
                    onBackCallback();
                }
            });
            el.appendChild(backBtn);

            // Header with file metadata
            const header = document.createElement("div");
            header.className = "file-content-header";

            const fileNameEl = document.createElement("div");
            fileNameEl.className = "file-content-filename";
            const fileIconEl = document.createElement("span");
            fileIconEl.className = "explorer-icon";
            fileIconEl.innerHTML = getFileIcon(fileName || "");
            fileNameEl.appendChild(fileIconEl);
            fileNameEl.appendChild(document.createTextNode(fileName || "Untitled"));

            const meta = document.createElement("div");
            meta.className = "file-content-meta";
            meta.textContent = formatSize(blobData.size);

            header.appendChild(fileNameEl);
            header.appendChild(meta);
            el.appendChild(header);

            // Content body
            const body = document.createElement("div");
            body.className = "file-content-body";

            if (blobData.binary) {
                // Binary file notice
                const binaryMsg = document.createElement("div");
                binaryMsg.className = "file-content-binary";
                binaryMsg.textContent = `Binary file (${formatSize(blobData.size)})`;
                body.appendChild(binaryMsg);
            } else {
                // Text file with line numbers
                const lines = blobData.content.split("\n");
                for (let i = 0; i < lines.length; i++) {
                    const lineEl = document.createElement("div");
                    lineEl.className = "file-content-line";

                    const lineNum = document.createElement("span");
                    lineNum.className = "file-content-linenum";
                    lineNum.textContent = String(i + 1);

                    const lineText = document.createElement("span");
                    lineText.className = "file-content-text";
                    lineText.textContent = lines[i];

                    lineEl.appendChild(lineNum);
                    lineEl.appendChild(lineText);
                    body.appendChild(lineEl);
                }

                // Truncation notice if applicable
                if (blobData.truncated) {
                    const truncMsg = document.createElement("div");
                    truncMsg.className = "file-content-truncated";
                    truncMsg.textContent = "Content truncated (file too large)";
                    body.appendChild(truncMsg);
                }
            }

            el.appendChild(body);

        } catch (error) {
            el.innerHTML = "";
            const errorMsg = document.createElement("div");
            errorMsg.className = "file-content-error";
            errorMsg.textContent = `Error loading file: ${error.message}`;
            el.appendChild(errorMsg);
        }
    }

    function close() {
        el.style.display = "none";
        el.innerHTML = "";
    }

    function onBack(callback) {
        onBackCallback = callback;
    }

    return {
        el,
        open,
        close,
        onBack,
    };
}

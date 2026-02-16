const FOLDER_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M2 3.5C2 2.67157 2.67157 2 3.5 2H6.08579C6.351 2 6.60536 2.10536 6.79289 2.29289L8 3.5H12.5C13.3284 3.5 14 4.17157 14 5V12C14 12.8284 13.3284 13.5 12.5 13.5H3.5C2.67157 13.5 2 12.8284 2 12V3.5Z" fill="currentColor" opacity="0.3"/>
    <path d="M2 3.5C2 2.67157 2.67157 2 3.5 2H6.08579C6.351 2 6.60536 2.10536 6.79289 2.29289L8 3.5M14 5V12C14 12.8284 13.3284 13.5 12.5 13.5H3.5C2.67157 13.5 2 12.8284 2 12V5C2 4.17157 2.67157 3.5 3.5 3.5H12.5C13.3284 3.5 14 4.17157 14 5Z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const FILE_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M4 2C3.44772 2 3 2.44772 3 3V13C3 13.5523 3.44772 14 4 14H12C12.5523 14 13 13.5523 13 13V6L9 2H4Z" fill="currentColor" opacity="0.2"/>
    <path d="M9 2V6H13M4 2H9L13 6V13C13 13.5523 12.5523 14 12 14H4C3.44772 14 3 13.5523 3 13V3C3 2.44772 3.44772 2 4 2Z" stroke="currentColor" stroke-width="1.2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const BACK_SVG = `<svg width="14" height="14" viewBox="0 0 16 16" fill="none">
    <path d="M10 4L6 8l4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

export function createFileBrowser() {
    const el = document.createElement("div");
    el.className = "file-browser";

    const breadcrumbBar = document.createElement("div");
    breadcrumbBar.className = "file-breadcrumbs";

    const treeList = document.createElement("div");
    treeList.className = "file-tree";

    const contentView = document.createElement("div");
    contentView.className = "file-content";
    contentView.style.display = "none";

    el.appendChild(breadcrumbBar);
    el.appendChild(treeList);
    el.appendChild(contentView);

    // State
    let commitHash = null;
    let commitMessage = null;
    let treePath = []; // [{name, hash}] - stack of traversed tree nodes
    let treeCache = new Map(); // Map<hash, treeData>
    let currentView = "tree"; // "tree" or "content"

    function renderBreadcrumbs() {
        breadcrumbBar.innerHTML = "";

        if (!commitHash) {
            return;
        }

        // Root breadcrumb
        const rootCrumb = document.createElement("button");
        rootCrumb.className = "file-breadcrumb";
        rootCrumb.textContent = commitHash.substring(0, 7);
        rootCrumb.title = commitMessage || commitHash;
        rootCrumb.addEventListener("click", () => {
            treePath = [];
            currentView = "tree";
            renderTreeView();
        });
        breadcrumbBar.appendChild(rootCrumb);

        // Path breadcrumbs
        for (let i = 0; i < treePath.length; i++) {
            const sep = document.createElement("span");
            sep.className = "file-breadcrumb-sep";
            sep.textContent = "/";
            breadcrumbBar.appendChild(sep);

            const crumb = document.createElement("button");
            crumb.className = "file-breadcrumb";
            crumb.textContent = treePath[i].name;
            crumb.addEventListener("click", () => {
                treePath = treePath.slice(0, i + 1);
                currentView = "tree";
                renderTreeView();
            });
            breadcrumbBar.appendChild(crumb);
        }
    }

    async function fetchTree(treeHash) {
        if (treeCache.has(treeHash)) {
            return treeCache.get(treeHash);
        }

        const response = await fetch(`/api/tree/${treeHash}`);
        if (!response.ok) {
            throw new Error(`Failed to fetch tree ${treeHash}: ${response.status}`);
        }

        const treeData = await response.json();
        treeCache.set(treeHash, treeData);
        return treeData;
    }

    async function fetchBlob(blobHash) {
        const response = await fetch(`/api/blob/${blobHash}`);
        if (!response.ok) {
            throw new Error(`Failed to fetch blob ${blobHash}: ${response.status}`);
        }

        return response.json();
    }

    async function renderTreeView() {
        treeList.style.display = "block";
        contentView.style.display = "none";
        currentView = "tree";

        renderBreadcrumbs();

        treeList.innerHTML = "";

        const currentTreeHash = treePath.length === 0
            ? await getCommitRootTree()
            : treePath[treePath.length - 1].hash;

        if (!currentTreeHash) {
            treeList.innerHTML = '<div class="file-tree-empty">No tree data</div>';
            return;
        }

        try {
            const treeData = await fetchTree(currentTreeHash);
            const entries = treeData.entries || [];

            if (entries.length === 0) {
                treeList.innerHTML = '<div class="file-tree-empty">Empty directory</div>';
                return;
            }

            // Sort: folders first, then files, alphabetically within each group
            const folders = entries.filter(e => e.mode === "040000").sort((a, b) => a.name.localeCompare(b.name));
            const files = entries.filter(e => e.mode !== "040000").sort((a, b) => a.name.localeCompare(b.name));
            const sortedEntries = [...folders, ...files];

            for (const entry of sortedEntries) {
                const entryEl = document.createElement("div");
                entryEl.className = "file-tree-entry";

                const icon = document.createElement("span");
                icon.className = "file-tree-icon";
                icon.innerHTML = entry.mode === "040000" ? FOLDER_SVG : FILE_SVG;

                const name = document.createElement("span");
                name.className = "file-tree-name";
                name.textContent = entry.name;

                entryEl.appendChild(icon);
                entryEl.appendChild(name);

                entryEl.addEventListener("click", () => {
                    if (entry.mode === "040000") {
                        // Folder - navigate into it
                        treePath.push({ name: entry.name, hash: entry.hash });
                        renderTreeView();
                    } else {
                        // File - view its content
                        viewFileContent(entry);
                    }
                });

                treeList.appendChild(entryEl);
            }
        } catch (error) {
            treeList.innerHTML = `<div class="file-tree-empty">Error: ${error.message}</div>`;
        }
    }

    async function viewFileContent(entry) {
        currentView = "content";
        treeList.style.display = "none";
        contentView.style.display = "block";

        // Add file to breadcrumb path temporarily for display
        const filePathDisplay = [...treePath, { name: entry.name, hash: entry.hash }];
        breadcrumbBar.innerHTML = "";

        // Root breadcrumb
        const rootCrumb = document.createElement("button");
        rootCrumb.className = "file-breadcrumb";
        rootCrumb.textContent = commitHash.substring(0, 7);
        rootCrumb.title = commitMessage || commitHash;
        rootCrumb.addEventListener("click", () => {
            treePath = [];
            currentView = "tree";
            renderTreeView();
        });
        breadcrumbBar.appendChild(rootCrumb);

        // Path including file
        for (let i = 0; i < filePathDisplay.length; i++) {
            const sep = document.createElement("span");
            sep.className = "file-breadcrumb-sep";
            sep.textContent = "/";
            breadcrumbBar.appendChild(sep);

            const crumb = document.createElement("span");
            crumb.className = i < filePathDisplay.length - 1 ? "file-breadcrumb" : "file-breadcrumb-current";
            crumb.textContent = filePathDisplay[i].name;
            if (i < filePathDisplay.length - 1) {
                crumb.style.cursor = "pointer";
                crumb.addEventListener("click", () => {
                    treePath = treePath.slice(0, i + 1);
                    currentView = "tree";
                    renderTreeView();
                });
            }
            breadcrumbBar.appendChild(crumb);
        }

        contentView.innerHTML = '<div class="file-content-loading">Loading...</div>';

        try {
            const blobData = await fetchBlob(entry.hash);

            contentView.innerHTML = "";

            // Header with metadata
            const header = document.createElement("div");
            header.className = "file-content-header";

            const fileName = document.createElement("div");
            fileName.className = "file-content-filename";
            fileName.textContent = entry.name;

            const meta = document.createElement("div");
            meta.className = "file-content-meta";
            meta.textContent = formatSize(blobData.size);

            header.appendChild(fileName);
            header.appendChild(meta);
            contentView.appendChild(header);

            // Content body
            const body = document.createElement("div");
            body.className = "file-content-body";

            if (blobData.binary) {
                const binaryMsg = document.createElement("div");
                binaryMsg.className = "file-content-binary";
                binaryMsg.textContent = `Binary file (${formatSize(blobData.size)})`;
                body.appendChild(binaryMsg);
            } else {
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

                if (blobData.truncated) {
                    const truncMsg = document.createElement("div");
                    truncMsg.className = "file-content-truncated";
                    truncMsg.textContent = "Content truncated (file too large)";
                    body.appendChild(truncMsg);
                }
            }

            contentView.appendChild(body);

            // Add back button
            const backBtn = document.createElement("button");
            backBtn.className = "file-content-back";
            backBtn.innerHTML = BACK_SVG + " Back to tree";
            backBtn.addEventListener("click", () => {
                currentView = "tree";
                renderTreeView();
            });
            contentView.insertBefore(backBtn, header);

        } catch (error) {
            contentView.innerHTML = `<div class="file-content-error">Error loading file: ${error.message}</div>`;
        }
    }

    async function getCommitRootTree() {
        // Fetch commit data to get root tree hash
        // For now we'll fetch it from the repository endpoint or assume we have it
        // This is a simplified version - in practice we'd fetch commit data
        if (!commitHash) return null;

        try {
            const response = await fetch(`/api/repository`);
            if (!response.ok) return null;
            const repoData = await response.json();

            // Find the commit in our data
            // Since we don't have direct commit API, we need to traverse from refs
            // For initial implementation, we'll assume the commit tree is passed in openCommit
            return currentRootTree;
        } catch {
            return null;
        }
    }

    let currentRootTree = null;

    function openCommit(commit) {
        if (!commit) return;

        commitHash = commit.hash;
        commitMessage = commit.message || "";
        currentRootTree = commit.tree;
        treePath = [];
        treeCache.clear(); // Clear cache when switching commits
        currentView = "tree";

        renderTreeView();
    }

    function formatSize(bytes) {
        if (bytes < 1024) return `${bytes} B`;
        if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
        return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
    }

    return { el, openCommit };
}

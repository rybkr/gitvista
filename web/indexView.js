import { apiUrl } from "./apiBase.js";
import { createDiffContentViewer } from "./diffContentViewer.js";

const CHEVRON_SVG = `<svg width="12" height="12" viewBox="0 0 12 12" fill="none">
    <path d="M4 2l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const SECTIONS = [
    { key: "staged", label: "Staged", colorClass: "index-badge--staged" },
    { key: "modified", label: "Modified", colorClass: "index-badge--modified" },
    { key: "untracked", label: "Untracked", colorClass: "index-badge--untracked" },
];

/**
 * Build a single collapsible section (Staged / Modified / Untracked).
 *
 * @param {Object} cfg - Section config from SECTIONS array
 * @param {Function} onFileClick - Called with (file, sectionKey) when a list item is clicked
 */
function createSection(cfg, onFileClick) {
    const section = document.createElement("div");
    section.className = "index-section is-empty";

    const header = document.createElement("button");
    header.className = "index-section-header";

    const chevron = document.createElement("span");
    chevron.className = "index-chevron";
    chevron.innerHTML = CHEVRON_SVG;

    const title = document.createElement("span");
    title.className = "index-section-title";
    title.textContent = cfg.label;

    const badge = document.createElement("span");
    badge.className = `index-badge ${cfg.colorClass}`;
    badge.textContent = "0";

    header.appendChild(chevron);
    header.appendChild(title);
    header.appendChild(badge);

    const list = document.createElement("ul");
    list.className = "index-file-list";

    let collapsed = false;

    header.addEventListener("click", () => {
        collapsed = !collapsed;
        section.classList.toggle("is-collapsed", collapsed);
    });

    section.appendChild(header);
    section.appendChild(list);

    return {
        el: section,
        update(files) {
            const isEmpty = !files || files.length === 0;
            section.classList.toggle("is-empty", isEmpty);
            badge.textContent = isEmpty ? "0" : String(files.length);

            list.innerHTML = "";
            if (!isEmpty) {
                for (const file of files) {
                    const li = document.createElement("li");
                    li.className = "index-file";

                    const code = document.createElement("span");
                    code.className = "index-file-code";
                    code.textContent = file.statusCode;

                    const path = document.createElement("span");
                    path.className = "index-file-path";
                    path.textContent = file.path;
                    path.title = file.path;

                    li.appendChild(code);
                    li.appendChild(path);

                    // Modified and staged files are clickable — fetch their diff
                    if (cfg.key === "modified" || cfg.key === "staged") {
                        li.classList.add("index-file--clickable");
                        li.setAttribute("role", "button");
                        li.setAttribute("tabindex", "0");
                        li.title = `Click to view diff for ${file.path}`;

                        const handleClick = () => {
                            if (onFileClick) {
                                onFileClick(file, cfg.key);
                            }
                        };
                        li.addEventListener("click", handleClick);
                        // Keyboard accessibility: activate on Enter/Space
                        li.addEventListener("keydown", (e) => {
                            if (e.key === "Enter" || e.key === " ") {
                                e.preventDefault();
                                handleClick();
                            }
                        });
                    }

                    list.appendChild(li);
                }
            }
        },
    };
}

export function createIndexView() {
    const el = document.createElement("div");
    el.className = "index-view";

    // Diff viewer shown inline when a modified file is clicked
    const diffViewer = createDiffContentViewer();
    diffViewer.el.className += " index-diff-viewer";

    const heading = document.createElement("h3");
    heading.className = "index-heading";
    heading.textContent = "Working Tree";

    // Main content wrapper shown when the diff viewer is hidden
    const content = document.createElement("div");
    content.className = "index-content";
    content.appendChild(heading);

    // Wire up "Back" button in the diff viewer to return to the file list
    diffViewer.onBack(() => {
        diffViewer.close();
        content.style.display = "";
    });

    el.appendChild(content);
    el.appendChild(diffViewer.el);

    /**
     * Called when a file row is clicked. Fetches working-tree diff from the API
     * and renders it using diffContentViewer. Untracked files get a friendly notice
     * (the API returns status:"untracked" with empty hunks for those).
     */
    function handleFileClick(file) {
        // Hide the file list and show the diff viewer
        content.style.display = "none";

        const encodedPath = encodeURIComponent(file.path);
        const url = apiUrl(`/working-tree/diff?path=${encodedPath}`);

        // showFromUrl handles loading state, fetch, and rendering
        diffViewer.showFromUrl(url);
    }

    const sections = {};
    for (const cfg of SECTIONS) {
        const section = createSection(cfg, handleFileClick);
        sections[cfg.key] = section;
        content.appendChild(section.el);
    }

    return {
        el,
        update(status) {
            if (!status) return;
            sections.staged.update(status.staged);
            sections.modified.update(status.modified);
            sections.untracked.update(status.untracked);
        },
    };
}

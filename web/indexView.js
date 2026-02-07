const CHEVRON_SVG = `<svg width="12" height="12" viewBox="0 0 12 12" fill="none">
    <path d="M4 2l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const SECTIONS = [
    { key: "staged", label: "Staged", colorClass: "index-badge--staged" },
    { key: "modified", label: "Modified", colorClass: "index-badge--modified" },
    { key: "untracked", label: "Untracked", colorClass: "index-badge--untracked" },
];

function createSection(cfg) {
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
                    list.appendChild(li);
                }
            }
        },
    };
}

export function createIndexView() {
    const el = document.createElement("div");
    el.className = "index-view";

    const heading = document.createElement("h3");
    heading.className = "index-heading";
    heading.textContent = "Working Tree";
    el.appendChild(heading);

    const sections = {};
    for (const cfg of SECTIONS) {
        const section = createSection(cfg);
        sections[cfg.key] = section;
        el.appendChild(section.el);
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

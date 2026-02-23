/**
 * Three-Zone Git Staging View
 *
 * Visualises git's three conceptual areas — Working Directory, Staging Area,
 * and Repository (HEAD) — with animated transitions as files move between zones.
 */

const ZONE_ICON_WORKING = `<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
    <path d="M1.75 1h8.5c.966 0 1.75.784 1.75 1.75v5.5A1.75 1.75 0 0110.25 10H7.061l-2.574 2.573A1.458 1.458 0 012 11.543V10h-.25A1.75 1.75 0 010 8.25v-5.5C0 1.784.784 1 1.75 1zM1.5 2.75v5.5c0 .138.112.25.25.25h1a.75.75 0 01.75.75v2.19l2.72-2.72a.75.75 0 01.53-.22h3.5a.25.25 0 00.25-.25v-5.5a.25.25 0 00-.25-.25h-8.5a.25.25 0 00-.25.25z"/>
</svg>`;

const ZONE_ICON_STAGING = `<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
    <path d="M8.75.75a.75.75 0 00-1.5 0V2h-.984c-.305 0-.604.08-.869.23l-1.288.737A.25.25 0 013.984 3H1.75a.75.75 0 000 1.5h.428L.066 9.192a.75.75 0 00.154.838l.53-.53-.53.53v.001l.002.002.004.004.012.012a2.1 2.1 0 00.196.17c.14.11.364.26.678.4C1.727 10.92 2.59 11.25 4 11.25c1.41 0 2.273-.33 2.888-.632.314-.14.538-.29.679-.4a2.127 2.127 0 00.207-.182l.012-.012.004-.004.002-.002h.001l-.53-.53.53.53a.75.75 0 00.154-.838L5.822 4.5h.428a.25.25 0 00.125-.034l1.29-.736c.264-.152.563-.232.868-.232H9.5v1.633a.75.75 0 001.234.572l2.065-1.74a.75.75 0 000-1.147L10.734.076A.75.75 0 009.5.648V2h-.749zM4 5.25c-.498 0-.868.32-1.084.588L1.3 9.061l.007.001c.1.063.27.153.523.254.514.2 1.27.434 2.17.434.9 0 1.656-.234 2.17-.434a3.372 3.372 0 00.523-.254l.007-.002-1.616-3.222C4.868 5.57 4.498 5.25 4 5.25zM14.066 9.192L11.778 4.5h.472a.75.75 0 000-1.5h-2.233l-.656.375h.001l.002.003L11.648 8.5H11.5a.75.75 0 000 1.5h.478l-.256.506-.011.024a2.2 2.2 0 00-.178.604c-.007.072-.005.14.006.193.01.045.027.074.04.09.01.011.037.038.129.054.12.021.306.028.607.028.63 0 1.117-.14 1.423-.3a2.078 2.078 0 00.428-.33l.004-.005.002-.002v-.001h.001l-.53-.53.53.53a.75.75 0 00.157-.838z"/>
</svg>`;

const ZONE_ICON_REPO = `<svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
    <path d="M2 2.5A2.5 2.5 0 014.5 0h8.75a.75.75 0 01.75.75v12.5a.75.75 0 01-.75.75h-2.5a.75.75 0 110-1.5h1.75v-2h-8a1 1 0 00-.714 1.7.75.75 0 01-1.072 1.05A2.495 2.495 0 012 11.5v-9zm10.5-1h-8a1 1 0 00-1 1v6.708A2.486 2.486 0 014.5 9h8V1.5zm-8 11a1 1 0 100-2 1 1 0 000 2z"/>
</svg>`;

const ARROW_SVG = `<svg width="20" height="20" viewBox="0 0 20 20" fill="none">
    <path d="M4 10h12m0 0l-4-4m4 4l-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const ARROW_DOWN_SVG = `<svg width="20" height="20" viewBox="0 0 20 20" fill="none">
    <path d="M10 4v12m0 0l-4-4m4 4l4-4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const COMMITTED_TTL = 15000;
const COMMITTED_PRUNE_INTERVAL = 3000;

/**
 * Split a file path into { dir, base }.
 */
function splitPath(filePath) {
    const idx = filePath.lastIndexOf("/");
    if (idx === -1) return { dir: "", base: filePath };
    return { dir: filePath.slice(0, idx + 1), base: filePath.slice(idx + 1) };
}

/**
 * Build a file card DOM element.
 */
function createFileCard(file, animClass) {
    const card = document.createElement("div");
    card.className = "staging-file-card";
    if (animClass) card.classList.add(animClass);

    const code = document.createElement("span");
    code.className = "staging-file-code";
    code.textContent = file.statusCode || "?";

    const nameWrap = document.createElement("span");
    nameWrap.className = "staging-file-name";

    const { dir, base } = splitPath(file.path);

    const baseName = document.createElement("span");
    baseName.className = "staging-file-base";
    baseName.textContent = base;

    nameWrap.appendChild(baseName);

    if (dir) {
        const dirSpan = document.createElement("span");
        dirSpan.className = "staging-file-dir";
        dirSpan.textContent = dir;
        nameWrap.appendChild(dirSpan);
    }

    card.appendChild(code);
    card.appendChild(nameWrap);
    card.title = file.path;

    return card;
}

/**
 * Build a single zone column.
 */
function createZone(id, icon, label, colorModifier) {
    const zone = document.createElement("div");
    zone.className = `staging-zone staging-zone--${id}`;

    const header = document.createElement("div");
    header.className = "staging-zone-header";

    const iconSpan = document.createElement("span");
    iconSpan.className = "staging-zone-icon";
    iconSpan.innerHTML = icon;

    const labelSpan = document.createElement("span");
    labelSpan.className = "staging-zone-label";
    labelSpan.textContent = label;

    const badge = document.createElement("span");
    badge.className = `staging-zone-badge staging-zone-badge--${colorModifier}`;
    badge.textContent = "0";

    header.appendChild(iconSpan);
    header.appendChild(labelSpan);
    header.appendChild(badge);

    const body = document.createElement("div");
    body.className = "staging-zone-body";

    zone.appendChild(header);
    zone.appendChild(body);

    return { el: zone, body, badge };
}

/**
 * Build an arrow separator between zones.
 */
function createArrow(commandLabel) {
    const arrow = document.createElement("div");
    arrow.className = "staging-zone-arrow";

    const arrowH = document.createElement("span");
    arrowH.className = "staging-zone-arrow-icon staging-zone-arrow-icon--horizontal";
    arrowH.innerHTML = ARROW_SVG;

    const arrowV = document.createElement("span");
    arrowV.className = "staging-zone-arrow-icon staging-zone-arrow-icon--vertical";
    arrowV.innerHTML = ARROW_DOWN_SVG;

    const cmd = document.createElement("span");
    cmd.className = "staging-zone-arrow-label";
    cmd.textContent = commandLabel;

    arrow.appendChild(arrowH);
    arrow.appendChild(arrowV);
    arrow.appendChild(cmd);

    return arrow;
}

export function createStagingView() {
    const el = document.createElement("div");
    el.className = "staging-view";

    const heading = document.createElement("h3");
    heading.className = "staging-heading";
    heading.textContent = "Three Zones of Git";

    const zones = document.createElement("div");
    zones.className = "staging-zones";

    const working = createZone("working", ZONE_ICON_WORKING, "Working Directory", "orange");
    const staging = createZone("staging", ZONE_ICON_STAGING, "Staging Area", "green");
    const repo = createZone("repo", ZONE_ICON_REPO, "Repository", "blue");

    zones.appendChild(working.el);
    zones.appendChild(createArrow("git add"));
    zones.appendChild(staging.el);
    zones.appendChild(createArrow("git commit"));
    zones.appendChild(repo.el);

    el.appendChild(heading);
    el.appendChild(zones);

    // State tracking for animations
    let prevState = new Map(); // path → "working" | "staging"
    const committedFiles = new Map(); // path → { file, addedAt }

    const pruneTimer = setInterval(() => {
        const now = Date.now();
        let changed = false;
        for (const [path, entry] of committedFiles) {
            if (now - entry.addedAt > COMMITTED_TTL) {
                committedFiles.delete(path);
                changed = true;
            }
        }
        if (changed) renderRepo();
    }, COMMITTED_PRUNE_INTERVAL);

    // ResizeObserver for responsive layout
    const resizeObserver = new ResizeObserver((entries) => {
        for (const entry of entries) {
            const width = entry.contentRect.width;
            el.classList.toggle("staging-view--vertical", width < 240);
        }
    });
    resizeObserver.observe(el);

    function renderZone(zoneObj, files, animMap) {
        zoneObj.body.innerHTML = "";
        zoneObj.badge.textContent = String(files.length);

        if (files.length === 0) {
            const empty = document.createElement("div");
            empty.className = "staging-zone-empty";
            empty.textContent = "No files";
            zoneObj.body.appendChild(empty);
            return;
        }

        for (const file of files) {
            const animClass = animMap ? animMap.get(file.path) : null;
            zoneObj.body.appendChild(createFileCard(file, animClass));
        }
    }

    function renderRepo() {
        const repoFiles = Array.from(committedFiles.values());
        repo.body.innerHTML = "";
        repo.badge.textContent = String(repoFiles.length);

        if (repoFiles.length === 0) {
            const empty = document.createElement("div");
            empty.className = "staging-zone-empty";
            empty.textContent = "No recent commits";
            repo.body.appendChild(empty);
            return;
        }

        const now = Date.now();
        for (const entry of repoFiles) {
            const card = createFileCard(entry.file, null);
            const elapsed = now - entry.addedAt;
            if (elapsed < 500) {
                card.classList.add("staging-anim-fade-in");
            } else if (elapsed > COMMITTED_TTL - 3000) {
                card.classList.add("staging-anim-committed-fade");
            }
            repo.body.appendChild(card);
        }
    }

    function update(status) {
        if (!status) return;

        const workingFiles = [
            ...(status.modified || []),
            ...(status.untracked || []),
        ];
        const stagingFiles = status.staged || [];

        // Build current state map
        const currentState = new Map();
        for (const f of workingFiles) {
            currentState.set(f.path, "working");
        }
        for (const f of stagingFiles) {
            currentState.set(f.path, "staging");
        }

        // Detect transitions for animations
        const workingAnims = new Map();
        const stagingAnims = new Map();

        for (const [path, prevZone] of prevState) {
            const curZone = currentState.get(path);

            if (prevZone === "working" && curZone === "staging") {
                // File was staged: animate slide-from-left in staging zone
                stagingAnims.set(path, "staging-anim-slide-from-left");
            } else if (prevZone === "staging" && curZone === "working") {
                // File was unstaged: animate slide-from-right in working zone
                workingAnims.set(path, "staging-anim-slide-from-right");
            } else if (prevZone === "staging" && !curZone) {
                // File was committed (left staging, not in working)
                const file = findFile(stagingFiles, workingFiles, path) ||
                    { path, statusCode: "C" };
                committedFiles.set(path, { file, addedAt: Date.now() });
            }
        }

        prevState = currentState;

        renderZone(working, workingFiles, workingAnims);
        renderZone(staging, stagingFiles, stagingAnims);
        renderRepo();
    }

    function findFile(staged, working, path) {
        for (const f of staged) {
            if (f.path === path) return f;
        }
        for (const f of working) {
            if (f.path === path) return f;
        }
        return null;
    }

    return { el, update };
}

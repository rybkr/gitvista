/**
 * Git Lifecycle View
 *
 * Visualizes git's three conceptual areas as a vertical pipeline —
 * Working → Staged → Committed — with animated transitions as files
 * move between zones via git add, git reset, and git commit.
 */

// Circle with filled dot (recording / active editing)
const ICON_WORKING = `<svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.3">
    <circle cx="7" cy="7" r="5.5"/>
    <circle cx="7" cy="7" r="2" fill="currentColor" stroke="none"/>
</svg>`;

// Circle with plus (queued / ready to add)
const ICON_STAGED = `<svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.3">
    <circle cx="7" cy="7" r="5.5"/>
    <path d="M7 4.5v5M4.5 7h5"/>
</svg>`;

// Circle with checkmark (done / committed)
const ICON_COMMITTED = `<svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="1.3">
    <circle cx="7" cy="7" r="5.5"/>
    <path d="M4.5 7l2 2 3-3.5" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`;

const COMMITTED_TTL = 15000;
const COMMITTED_PRUNE_INTERVAL = 3000;

function splitPath(filePath) {
    const idx = filePath.lastIndexOf("/");
    if (idx === -1) return { dir: "", base: filePath };
    return { dir: filePath.slice(0, idx + 1), base: filePath.slice(idx + 1) };
}

function createFileCard(file, animClass) {
    const card = document.createElement("div");
    card.className = "staging-file-card";
    if (animClass) card.classList.add(animClass);

    const code = document.createElement("span");
    code.className = "staging-file-code";
    const sc = file.statusCode || "?";
    code.textContent = sc;

    if (sc === "M") code.classList.add("staging-file-code--modified");
    else if (sc === "A") code.classList.add("staging-file-code--added");
    else if (sc === "D") code.classList.add("staging-file-code--deleted");
    else if (sc === "?") code.classList.add("staging-file-code--untracked");

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

    if (file.blobHash) {
        const hash = document.createElement("span");
        hash.className = "staging-file-hash";
        hash.textContent = file.blobHash.slice(0, 7);
        hash.title = file.blobHash;
        card.appendChild(hash);
    }

    card.appendChild(nameWrap);
    card.title = file.path;

    return card;
}

function createZone(id, icon, label, colorModifier) {
    const zone = document.createElement("div");
    zone.className = `staging-zone staging-zone--${id}`;

    const header = document.createElement("div");
    header.className = "staging-zone-header";

    const iconSpan = document.createElement("span");
    iconSpan.className = `staging-zone-icon staging-zone-icon--${colorModifier}`;
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

function createConnector(fwdLabel, revLabel) {
    const connector = document.createElement("div");
    connector.className = "staging-connector";

    const trackTop = document.createElement("div");
    trackTop.className = "staging-connector-track";

    const fwd = document.createElement("span");
    fwd.className = "staging-connector-label";
    fwd.textContent = fwdLabel;

    const rev = document.createElement("span");
    rev.className = "staging-connector-label staging-connector-label--rev";
    rev.textContent = revLabel;

    const trackBottom = document.createElement("div");
    trackBottom.className = "staging-connector-track";

    connector.appendChild(trackTop);
    connector.appendChild(fwd);
    connector.appendChild(rev);
    connector.appendChild(trackBottom);

    return { el: connector, fwdLabel: fwd, revLabel: rev };
}

export function createStagingView() {
    const el = document.createElement("div");
    el.className = "staging-view";

    const zones = document.createElement("div");
    zones.className = "staging-zones";

    const working = createZone("working", ICON_WORKING, "Working", "warning");
    const staging = createZone("staging", ICON_STAGED, "Staged", "success");
    const repo = createZone("repo", ICON_COMMITTED, "Committed", "info");

    const addConnector = createConnector("git add", "git restore --staged");
    const commitConnector = createConnector("git commit", "git restore");

    zones.appendChild(working.el);
    zones.appendChild(addConnector.el);
    zones.appendChild(staging.el);
    zones.appendChild(commitConnector.el);
    zones.appendChild(repo.el);

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

    function renderZone(zoneObj, files, animMap) {
        zoneObj.body.innerHTML = "";
        zoneObj.badge.textContent = String(files.length);

        if (files.length === 0) {
            const empty = document.createElement("div");
            empty.className = "staging-zone-empty";
            empty.textContent = "Clean";
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

        let highlightRestoreStaged = false;
        let highlightRestore = false;

        for (const [path, prevZone] of prevState) {
            const curZone = currentState.get(path);

            if (prevZone === "working" && curZone === "staging") {
                stagingAnims.set(path, "staging-anim-slide-down");
            } else if (prevZone === "staging" && curZone === "working") {
                workingAnims.set(path, "staging-anim-slide-up");
                highlightRestoreStaged = true;
            } else if (prevZone === "staging" && !curZone) {
                const file = findFile(stagingFiles, workingFiles, path) ||
                    { path, statusCode: "C" };
                committedFiles.set(path, { file, addedAt: Date.now() });
            } else if (prevZone === "working" && !curZone) {
                highlightRestore = true;
            }
        }

        if (highlightRestoreStaged) {
            addConnector.revLabel.classList.add("staging-connector-label--highlight");
            setTimeout(() => {
                addConnector.revLabel.classList.remove("staging-connector-label--highlight");
            }, 600);
        }
        if (highlightRestore) {
            commitConnector.revLabel.classList.add("staging-connector-label--highlight");
            setTimeout(() => {
                commitConnector.revLabel.classList.remove("staging-connector-label--highlight");
            }, 600);
        }

        // Detect new files (not in prevState) for entrance animation
        for (const [path, zone] of currentState) {
            if (!prevState.has(path)) {
                if (zone === "working") {
                    workingAnims.set(path, "staging-anim-fade-in");
                } else if (zone === "staging") {
                    stagingAnims.set(path, "staging-anim-fade-in");
                }
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

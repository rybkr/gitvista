/**
 * Git Lifecycle View
 *
 * Visualizes git's working tree, index, local branch, and upstream tracking state
 * as a horizontal conveyor with animated transitions for local file movement.
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
const UPSTREAM_STATUS_LABELS = {
    up_to_date: "Up to date",
    ahead: "Ahead",
    behind: "Behind",
    diverged: "Diverged",
    unavailable: "Unavailable",
};
const UPSTREAM_REASON_LABELS = {
    detached_head: "Detached HEAD",
    no_current_branch: "No current branch",
    no_upstream_config: "No upstream tracking branch",
    missing_remote_ref: "Upstream ref not found",
    no_common_ancestor: "No common ancestor",
};

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

    const headerText = document.createElement("div");
    headerText.className = "staging-zone-header-text";

    const iconSpan = document.createElement("span");
    iconSpan.className = `staging-zone-icon staging-zone-icon--${colorModifier}`;
    iconSpan.innerHTML = icon;

    const labelSpan = document.createElement("span");
    labelSpan.className = "staging-zone-label";
    labelSpan.textContent = label;

    const meta = document.createElement("span");
    meta.className = "staging-zone-meta";

    const badge = document.createElement("span");
    badge.className = `staging-zone-badge staging-zone-badge--${colorModifier}`;
    badge.textContent = "0";

    header.appendChild(iconSpan);
    headerText.appendChild(labelSpan);
    headerText.appendChild(meta);
    header.appendChild(headerText);
    header.appendChild(badge);

    const body = document.createElement("div");
    body.className = "staging-zone-body";

    zone.appendChild(header);
    zone.appendChild(body);

    return { el: zone, body, badge, label: labelSpan, meta };
}

function createSummaryCard({ tone, title, body }) {
    const card = document.createElement("div");
    card.className = "staging-summary-card";
    if (tone) card.classList.add(`staging-summary-card--${tone}`);

    const titleEl = document.createElement("div");
    titleEl.className = "staging-summary-title";
    titleEl.textContent = title;

    const bodyEl = document.createElement("div");
    bodyEl.className = "staging-summary-body";
    bodyEl.textContent = body;

    card.appendChild(titleEl);
    card.appendChild(bodyEl);
    return card;
}

function summarizeUpstream(upstream) {
    if (!upstream) {
        return {
            tone: "muted",
            title: UPSTREAM_STATUS_LABELS.unavailable,
            body: "No upstream tracking data available.",
        };
    }

    if (upstream.status === "up_to_date") {
        return { tone: "stable", title: "Up to date", body: "Local branch matches the tracked upstream tip." };
    }
    if (upstream.status === "ahead") {
        return {
            tone: "ahead",
            title: `Ahead by ${upstream.aheadCount || 0}`,
            body: "Local commits have not been pushed yet.",
        };
    }
    if (upstream.status === "behind") {
        return {
            tone: "behind",
            title: `Behind by ${upstream.behindCount || 0}`,
            body: "Remote commits exist locally only as tracking refs.",
        };
    }
    if (upstream.status === "diverged") {
        return {
            tone: "diverged",
            title: `Diverged +${upstream.aheadCount || 0} / -${upstream.behindCount || 0}`,
            body: "Local and upstream both contain unique commits.",
        };
    }

    return {
        tone: "muted",
        title: UPSTREAM_REASON_LABELS[upstream.reason] || UPSTREAM_STATUS_LABELS[upstream.status] || "Unavailable",
        body: upstream.ref ? `Tracking target ${upstream.ref} is not currently comparable.` : "The current branch is not connected to an upstream tracking ref.",
    };
}

export function createStagingView() {
    const el = document.createElement("div");
    el.className = "staging-view";

    const intro = document.createElement("div");
    intro.className = "staging-intro";

    const eyebrow = document.createElement("div");
    eyebrow.className = "staging-intro-eyebrow";
    eyebrow.textContent = "Git lifecycle";

    const title = document.createElement("h2");
    title.className = "staging-intro-title";
    title.textContent = "Index conveyor";

    const subtitle = document.createElement("p");
    subtitle.className = "staging-intro-subtitle";
    subtitle.textContent = "Files travel left to right from the working tree into the index and local branch, with the tracked upstream shown as the final published state.";

    intro.appendChild(eyebrow);
    intro.appendChild(title);
    intro.appendChild(subtitle);

    const zones = document.createElement("div");
    zones.className = "staging-zones";

    const working = createZone("working", ICON_WORKING, "Working", "warning");
    const staging = createZone("staging", ICON_STAGED, "Staged", "success");
    const local = createZone("local", ICON_COMMITTED, "Local branch", "info");
    const upstream = createZone("upstream", ICON_COMMITTED, "Upstream", "upstream");

    zones.appendChild(working.el);
    zones.appendChild(staging.el);
    zones.appendChild(local.el);
    zones.appendChild(upstream.el);

    el.appendChild(intro);
    el.appendChild(zones);

    // State tracking for animations
    let prevState = new Map(); // path → "working" | "staging"
    let currentHead = null;
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
        if (changed) renderLocal();
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

    function renderLocal() {
        const localFiles = Array.from(committedFiles.values());
        local.body.innerHTML = "";
        local.badge.textContent = String(localFiles.length);

        if (localFiles.length === 0) {
            const empty = document.createElement("div");
            empty.className = "staging-zone-empty";
            empty.textContent = "No recent commits";
            local.body.appendChild(empty);
            return;
        }

        const now = Date.now();
        for (const entry of localFiles) {
            const card = createFileCard(entry.file, null);
            const elapsed = now - entry.addedAt;
            if (elapsed < 500) {
                card.classList.add("staging-anim-fade-in");
            } else if (elapsed > COMMITTED_TTL - 3000) {
                card.classList.add("staging-anim-committed-fade");
            }
            local.body.appendChild(card);
        }
    }

    function renderUpstream() {
        upstream.body.innerHTML = "";
        const upstreamInfo = currentHead?.upstream || null;
        if (!upstreamInfo || upstreamInfo.status === "unavailable" || upstreamInfo.status === "up_to_date") {
            upstream.badge.textContent = "0";
        } else if (upstreamInfo.status === "diverged") {
            upstream.badge.textContent = String((upstreamInfo.aheadCount || 0) + (upstreamInfo.behindCount || 0));
        } else {
            upstream.badge.textContent = String(upstreamInfo.aheadCount || upstreamInfo.behindCount || 0);
        }

        const summary = summarizeUpstream(upstreamInfo);
        upstream.body.appendChild(createSummaryCard(summary));
    }

    function renderHeaders() {
        working.meta.textContent = "Modified + untracked";
        staging.meta.textContent = "Git index";
        local.meta.textContent = currentHead?.isDetached
            ? "Detached HEAD"
            : (currentHead?.branchName || "Current branch");
        upstream.meta.textContent = currentHead?.upstream?.branchName || "No tracked upstream";
    }

    function updateStatus(status) {
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
                stagingAnims.set(path, "staging-anim-slide-down");
            } else if (prevZone === "staging" && curZone === "working") {
                workingAnims.set(path, "staging-anim-slide-up");
            } else if (prevZone === "staging" && !curZone) {
                const file = findFile(stagingFiles, workingFiles, path) ||
                    { path, statusCode: "C" };
                committedFiles.set(path, { file, addedAt: Date.now() });
            }
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
        renderLocal();
        renderUpstream();
        renderHeaders();
    }

    function updateHead(headInfo) {
        currentHead = headInfo || null;
        renderHeaders();
        renderUpstream();
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

    return {
        el,
        update: updateStatus,
        updateStatus,
        updateHead,
    };
}

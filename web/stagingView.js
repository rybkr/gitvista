/**
 * Git Lifecycle View
 *
 * Visualizes git's working tree, index, local branch, and upstream tracking state
 * as a horizontal conveyor with animated transitions for local file movement.
 */

import { apiUrl } from "./apiBase.js";
import { createDiffContentViewer } from "./diffContentViewer.js";

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
const ICON_DIFF = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4">
    <path d="M5 3.5v9M11 3.5v9" stroke-linecap="round"/>
    <path d="M2.5 6.5h5M8.5 9.5h5" stroke-linecap="round"/>
</svg>`;

const COMMITTED_TTL = 15000;
const COMMITTED_PRUNE_INTERVAL = 3000;
const UPSTREAM_REASON_LABELS = {
    detached_head: "Detached HEAD",
    no_current_branch: "No branch",
    no_upstream_config: "No upstream",
    missing_remote_ref: "Missing upstream ref",
    no_common_ancestor: "No merge base",
};
const LANE_HELP = {
    working: "Files changed in the working tree but not staged in the index.",
    staged: "Files currently in the git index and ready for commit.",
    local: "Files that recently left the index through a local commit.",
    upstream: "Current branch compared against its tracked remote branch.",
};

function splitPath(filePath) {
    const idx = filePath.lastIndexOf("/");
    if (idx === -1) return { dir: "", base: filePath };
    return { dir: filePath.slice(0, idx + 1), base: filePath.slice(idx + 1) };
}

function createFileCard(file, animClass, fileAction, diffAction) {
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

    if (fileAction) {
        card.classList.add("staging-file-card--interactive");
        card.addEventListener("click", () => {
            fileAction();
        });
    }

    if (diffAction) {
        const action = document.createElement("button");
        action.type = "button";
        action.className = "staging-file-action";
        action.setAttribute("aria-label", `View diff for ${file.path}`);
        action.title = `View diff for ${file.path}`;
        action.innerHTML = `${ICON_DIFF}<span>Diff</span>`;
        action.addEventListener("click", (event) => {
            event.stopPropagation();
            diffAction();
        });
        card.appendChild(action);
    }

    return card;
}

function createInfoButton(text, id) {
    const wrap = document.createElement("span");
    wrap.className = "staging-help";

    const button = document.createElement("button");
    button.type = "button";
    button.className = "staging-help-button";
    button.setAttribute("aria-label", `Explain ${id}`);
    button.setAttribute("aria-describedby", `staging-help-${id}`);
    button.textContent = "i";

    const tooltip = document.createElement("span");
    tooltip.className = "staging-help-tooltip";
    tooltip.id = `staging-help-${id}`;
    tooltip.setAttribute("role", "tooltip");
    tooltip.textContent = text;

    wrap.appendChild(button);
    wrap.appendChild(tooltip);
    return wrap;
}

function createZone(id, icon, label, colorModifier, helpText) {
    const zone = document.createElement("div");
    zone.className = `staging-zone staging-zone--${id}`;

    const header = document.createElement("div");
    header.className = "staging-zone-header";

    const titleRow = document.createElement("div");
    titleRow.className = "staging-zone-title-row";

    const headerText = document.createElement("div");
    headerText.className = "staging-zone-header-text";

    const iconSpan = document.createElement("span");
    iconSpan.className = `staging-zone-icon staging-zone-icon--${colorModifier}`;
    iconSpan.innerHTML = icon;

    const labelSpan = document.createElement("span");
    labelSpan.className = "staging-zone-label";
    labelSpan.textContent = label;

    titleRow.appendChild(labelSpan);
    if (helpText) {
        titleRow.appendChild(createInfoButton(helpText, id));
    }

    const meta = document.createElement("span");
    meta.className = "staging-zone-meta";

    const badge = document.createElement("span");
    badge.className = `staging-zone-badge staging-zone-badge--${colorModifier}`;
    badge.textContent = "0";

    header.appendChild(iconSpan);
    headerText.appendChild(titleRow);
    headerText.appendChild(meta);
    header.appendChild(headerText);
    header.appendChild(badge);

    const body = document.createElement("div");
    body.className = "staging-zone-body";

    zone.appendChild(header);
    zone.appendChild(body);

    return { el: zone, body, badge, label: labelSpan, meta };
}

function createSummaryCard({ tone, title, meta }) {
    const card = document.createElement("div");
    card.className = "staging-summary-card";
    if (tone) card.classList.add(`staging-summary-card--${tone}`);

    const titleEl = document.createElement("div");
    titleEl.className = "staging-summary-title";
    titleEl.textContent = title;

    card.appendChild(titleEl);
    if (meta) {
        const metaEl = document.createElement("div");
        metaEl.className = "staging-summary-meta";
        metaEl.textContent = meta;
        card.appendChild(metaEl);
    }
    return card;
}

function summarizeUpstream(upstream) {
    if (!upstream) {
        return { mode: "empty", text: "No upstream" };
    }

    if (upstream.status === "up_to_date") {
        return { mode: "empty", text: "Up to date" };
    }
    if (upstream.status === "ahead") {
        return {
            mode: "summary",
            tone: "ahead",
            title: `Ahead +${upstream.aheadCount || 0}`,
            meta: upstream.branchName || "",
        };
    }
    if (upstream.status === "behind") {
        return {
            mode: "summary",
            tone: "behind",
            title: `Behind -${upstream.behindCount || 0}`,
            meta: upstream.branchName || "",
        };
    }
    if (upstream.status === "diverged") {
        return {
            mode: "summary",
            tone: "diverged",
            title: `Diverged +${upstream.aheadCount || 0} / -${upstream.behindCount || 0}`,
            meta: upstream.branchName || "",
        };
    }

    return { mode: "empty", text: UPSTREAM_REASON_LABELS[upstream.reason] || "Unavailable" };
}

export function createStagingView(options = {}) {
    const el = document.createElement("div");
    el.className = "staging-view";

    const zones = document.createElement("div");
    zones.className = "staging-zones";
    const diffViewer = createDiffContentViewer();
    const diffOverlay = document.createElement("div");
    diffOverlay.className = "staging-diff-overlay";
    diffOverlay.style.display = "none";
    diffOverlay.appendChild(diffViewer.el);
    let activeDiffContext = null;

    const working = createZone("working", ICON_WORKING, "Working", "warning", LANE_HELP.working);
    const staging = createZone("staging", ICON_STAGED, "Staged", "success", LANE_HELP.staged);
    const local = createZone("local", ICON_COMMITTED, "Local branch", "info", LANE_HELP.local);
    const upstream = createZone("upstream", ICON_COMMITTED, "Upstream", "upstream", LANE_HELP.upstream);

    zones.appendChild(working.el);
    zones.appendChild(staging.el);
    zones.appendChild(local.el);
    zones.appendChild(upstream.el);

    el.appendChild(zones);
    el.appendChild(diffOverlay);

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

    diffViewer.onBack(() => {
        diffViewer.close();
        diffOverlay.style.display = "none";
        activeDiffContext = null;
    });
    diffViewer.setHeaderActionRenderer((fileDiff) => {
        if (!activeDiffContext || typeof options.onOpenInExplorer !== "function") return null;

        const action = document.createElement("button");
        action.type = "button";
        action.className = "staging-diff-open-explorer";
        action.textContent = "Open in File Explorer";
        action.addEventListener("click", () => {
            options.onOpenInExplorer({
                path: fileDiff.path,
                source: activeDiffContext.source,
                url: activeDiffContext.url,
                title: activeDiffContext.title,
            });
        });
        return action;
    });

    function showDiffForFile(file, source) {
        const basePath = source === "index" ? "/index/diff" : "/working-tree/diff";
        const url = apiUrl(`${basePath}?path=${encodeURIComponent(file.path)}`);
        activeDiffContext = {
            source,
            url,
            title: `${source === "index" ? "Staged" : "Working"} diff — ${file.path}`,
        };
        diffOverlay.style.display = "flex";
        diffViewer.showFromUrl(url);
    }

    function renderZone(zoneObj, files, animMap, diffSource = null) {
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
            const fileAction = typeof options.onSelectFile === "function"
                ? () => options.onSelectFile({ path: file.path, source: diffSource })
                : null;
            const diffAction = diffSource ? () => showDiffForFile(file, diffSource) : null;
            zoneObj.body.appendChild(createFileCard(file, animClass, fileAction, diffAction));
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
        if (summary.mode === "empty") {
            const empty = document.createElement("div");
            empty.className = "staging-zone-empty";
            empty.textContent = summary.text;
            upstream.body.appendChild(empty);
        } else {
            upstream.body.appendChild(createSummaryCard(summary));
        }
    }

    function renderHeaders() {
        working.meta.textContent = "";
        staging.meta.textContent = "";
        local.meta.textContent = currentHead?.isDetached
            ? "Detached HEAD"
            : (currentHead?.branchName || "Current branch");
        upstream.meta.textContent = currentHead?.upstream?.branchName || "";
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

        renderZone(working, workingFiles, workingAnims, "working");
        renderZone(staging, stagingFiles, stagingAnims, "index");
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

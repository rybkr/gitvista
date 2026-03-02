/**
 * Lightweight frontend telemetry store + HUD for debugging large-repo behavior.
 */

const state = {
    startMs: Date.now(),
    connectionState: "disconnected",
    reconnectAttempt: 0,
    wsMessages: 0,
    wsBytesTotal: 0,
    wsLastBytes: 0,
    wsMaxBytes: 0,
    deltas: 0,
    summaries: 0,
    summaryCommits: 0,
    bootstrapDeltas: 0,
    bootstrapComplete: false,
    bootstrapCommits: 0,
    addedCommits: 0,
    diffStatsRequests: 0,
    diffStatsFailures: 0,
    diffStatsLastLimit: 0,
};
const listeners = new Set();

function emit() {
    for (const listener of listeners) {
        listener();
    }
}

export const telemetryStore = {
    recordConnectionState(connectionState, attempt = 0) {
        state.connectionState = connectionState;
        state.reconnectAttempt = attempt;
        emit();
    },
    recordWsMessage(bytes = 0) {
        const size = Number.isFinite(bytes) ? Math.max(0, bytes) : 0;
        state.wsMessages++;
        state.wsBytesTotal += size;
        state.wsLastBytes = size;
        if (size > state.wsMaxBytes) state.wsMaxBytes = size;
        emit();
    },
    recordDelta(delta) {
        state.deltas++;
        const added = delta?.addedCommits?.length ?? 0;
        state.addedCommits += added;
        if (delta?.bootstrap) {
            state.bootstrapDeltas++;
            state.bootstrapCommits += added;
            if (delta.bootstrapComplete) state.bootstrapComplete = true;
        }
        emit();
    },
    recordSummary(summary) {
        state.summaries++;
        const total = Number.isFinite(summary?.totalCommits) ? summary.totalCommits : 0;
        state.summaryCommits = Math.max(0, Math.floor(total));
        emit();
    },
    recordDiffStatsRequest(limit = 0, ok = true) {
        state.diffStatsRequests++;
        state.diffStatsLastLimit = Number.isFinite(limit) ? Math.max(0, Math.floor(limit)) : 0;
        if (!ok) state.diffStatsFailures++;
        emit();
    },
    subscribe(listener) {
        if (typeof listener !== "function") return () => {};
        listeners.add(listener);
        return () => listeners.delete(listener);
    },
    snapshot() {
        return { ...state };
    },
};

function formatBytes(bytes) {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

export function createTelemetryHud({ getGraphTelemetry }) {
    const el = document.createElement("div");
    el.className = "telemetry-hud";

    const header = document.createElement("div");
    header.className = "telemetry-hud__header";
    header.textContent = "Telemetry";

    const toggleBtn = document.createElement("button");
    toggleBtn.className = "telemetry-hud__toggle";
    toggleBtn.type = "button";
    toggleBtn.textContent = "Hide";
    header.appendChild(toggleBtn);

    const body = document.createElement("pre");
    body.className = "telemetry-hud__body";

    el.appendChild(header);
    el.appendChild(body);

    let hidden = false;
    toggleBtn.addEventListener("click", () => {
        hidden = !hidden;
        body.style.display = hidden ? "none" : "block";
        toggleBtn.textContent = hidden ? "Show" : "Hide";
    });

    let lastRenderCount = 0;
    let lastAt = Date.now();
    let rafId = null;
    let loopId = null;
    let queued = false;

    const renderHud = () => {
        queued = false;
        const now = Date.now();
        const dt = Math.max(1, now - lastAt);

        const snap = telemetryStore.snapshot();
        const graph = getGraphTelemetry?.() ?? {};
        const renderCount = graph.renderCount ?? 0;
        const renderPerSec = ((renderCount - lastRenderCount) * 1000) / dt;
        lastRenderCount = renderCount;
        lastAt = now;

        const lines = [
            `uptime: ${Math.floor((now - snap.startMs) / 1000)}s`,
            `connection: ${snap.connectionState}${snap.reconnectAttempt ? ` (#${snap.reconnectAttempt})` : ""}`,
            `ws msgs: ${snap.wsMessages}  last: ${formatBytes(snap.wsLastBytes)}  max: ${formatBytes(snap.wsMaxBytes)}  total: ${formatBytes(snap.wsBytesTotal)}`,
            `summaries: ${snap.summaries}  summary commits: ${snap.summaryCommits}`,
            `deltas: ${snap.deltas}  added commits: ${snap.addedCommits}`,
            `bootstrap: ${snap.bootstrapDeltas} batches  commits ${snap.bootstrapCommits}  complete=${snap.bootstrapComplete}`,
            `layout: ${graph.layoutMode ?? "?"}  known commits: ${graph.commitsCount ?? 0}  index: ${graph.commitIndexSize ?? 0}`,
            `materialized: ${graph.materializedCommits ?? 0}  viewport entries: ${graph.viewportEntries ?? 0}`,
            `hydration pending: ${graph.hydrationPending ?? 0}  inflight: ${graph.hydrationInflight ?? 0}  fetched: ${graph.hydrationFetched ?? 0}  errors: ${graph.hydrationErrors ?? 0}`,
            `nodes: ${graph.nodesCount ?? 0}  links: ${graph.linksCount ?? 0}  renders/s: ${renderPerSec.toFixed(1)}`,
            `diffstats req: ${snap.diffStatsRequests}  fail: ${snap.diffStatsFailures}  last limit: ${snap.diffStatsLastLimit}`,
        ];
        body.textContent = lines.join("\n");
    };

    const scheduleRender = () => {
        if (queued) return;
        queued = true;
        rafId = requestAnimationFrame(renderHud);
    };

    // Event-driven updates for websocket/delta/summary/diffstats events.
    const unsubscribe = telemetryStore.subscribe(scheduleRender);
    // Lightweight periodic refresh for graph-side metrics (render count, hydration).
    loopId = setInterval(scheduleRender, 120);
    scheduleRender();

    return {
        el,
        destroy() {
            unsubscribe();
            if (loopId !== null) clearInterval(loopId);
            if (rafId !== null) cancelAnimationFrame(rafId);
            el.remove();
        },
    };
}

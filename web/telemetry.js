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
    repoSummaries: 0,
    deltas: 0,
    repoSummaryCommits: 0,
    bootstrapChunks: 0,
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
    recordRepoSummary(summary) {
        state.repoSummaries++;
        const total = Number.isFinite(summary?.commitCount) ? summary.commitCount : 0;
        state.repoSummaryCommits = Math.max(0, Math.floor(total));
        emit();
    },
    recordBootstrapChunk(chunk) {
        state.bootstrapChunks++;
        state.bootstrapCommits += chunk?.commits?.length ?? 0;
        if (chunk?.final) state.bootstrapComplete = true;
        emit();
    },
    recordBootstrapComplete() {
        state.bootstrapComplete = true;
        emit();
    },
    recordDelta(delta) {
        state.deltas++;
        state.addedCommits += delta?.addedCommits?.length ?? 0;
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
    const triggerEl = document.createElement("button");
    triggerEl.className = "telemetry-trigger";
    triggerEl.type = "button";
    triggerEl.title = "Metrics";
    const triggerIcon = document.createElement("span");
    triggerIcon.className = "telemetry-trigger__icon";
    triggerIcon.setAttribute("aria-hidden", "true");
    triggerIcon.innerHTML = `<svg width="12" height="12" viewBox="0 0 16 16" fill="none">
        <path d="M4.5 2.75a1.75 1.75 0 1 0 0 3.5a1.75 1.75 0 0 0 0-3.5ZM4.5 9.75a1.75 1.75 0 1 0 0 3.5a1.75 1.75 0 0 0 0-3.5ZM11.5 6.25a1.75 1.75 0 1 0 0 3.5a1.75 1.75 0 0 0 0-3.5Z" stroke="currentColor" stroke-width="1.2"/>
        <path d="M6.2 4.5h3a2.3 2.3 0 0 1 2.3 2.3v.1M6.2 11.5h2.4a2.9 2.9 0 0 0 2.9-2.9V8.1" stroke="currentColor" stroke-width="1.2" stroke-linecap="round"/>
    </svg>`;
    const triggerLabel = document.createElement("span");
    triggerLabel.className = "telemetry-trigger__label";
    triggerLabel.textContent = "";
    triggerEl.appendChild(triggerIcon);
    triggerEl.appendChild(triggerLabel);

    const panelEl = document.createElement("section");
    panelEl.className = "telemetry-overlay";

    const header = document.createElement("div");
    header.className = "telemetry-overlay__header";
    const title = document.createElement("span");
    title.textContent = "Runtime Telemetry";
    const closeBtn = document.createElement("button");
    closeBtn.className = "telemetry-overlay__close";
    closeBtn.type = "button";
    closeBtn.textContent = "Close";
    header.appendChild(title);
    header.appendChild(closeBtn);

    const body = document.createElement("div");
    body.className = "telemetry-overlay__body";
    const metrics = document.createElement("div");
    metrics.className = "telemetry-overlay__metrics";
    body.appendChild(metrics);

    panelEl.appendChild(header);
    panelEl.appendChild(body);

    let lastRenderCount = 0;
    let lastAt = Date.now();
    let rafId = null;
    let loopId = null;
    let queued = false;
    let visible = false;

    const setVisible = (nextVisible) => {
        visible = Boolean(nextVisible);
        panelEl.classList.toggle("is-visible", visible);
        triggerEl.classList.toggle("is-active", visible);
        if (visible) scheduleRender();
    };

    const toggle = () => setVisible(!visible);
    const hide = () => setVisible(false);
    const isVisible = () => visible;

    triggerEl.addEventListener("click", (event) => {
        event.stopPropagation();
        toggle();
    });
    closeBtn.addEventListener("click", (event) => {
        event.stopPropagation();
        hide();
    });

    const onKeyDown = (event) => {
        if (event.key === "Escape" && visible) {
            hide();
        }
    };
    const onDocClick = (event) => {
        if (!visible) return;
        if (panelEl.contains(event.target) || triggerEl.contains(event.target)) return;
        hide();
    };
    window.addEventListener("keydown", onKeyDown);
    document.addEventListener("click", onDocClick);

    function metricRow(label, value, tone = "normal") {
        const row = document.createElement("div");
        row.className = "telemetry-overlay__metric";
        const key = document.createElement("span");
        key.className = "telemetry-overlay__metric-key";
        key.textContent = label;
        const val = document.createElement("span");
        val.className = "telemetry-overlay__metric-value";
        if (tone !== "normal") val.classList.add(`is-${tone}`);
        val.textContent = value;
        row.appendChild(key);
        row.appendChild(val);
        return row;
    }

    const renderHud = () => {
        queued = false;
        if (!visible) return;

        const now = Date.now();
        const dt = Math.max(1, now - lastAt);

        const snap = telemetryStore.snapshot();
        const graph = getGraphTelemetry?.() ?? {};
        const renderCount = graph.renderCount ?? 0;
        const renderPerSec = ((renderCount - lastRenderCount) * 1000) / dt;
        lastRenderCount = renderCount;
        lastAt = now;

        const connectionTone = snap.connectionState === "connected"
            ? "ok"
            : snap.connectionState === "reconnecting"
                ? "warn"
                : "error";

        const rows = [
            metricRow("Uptime", `${Math.floor((now - snap.startMs) / 1000)}s`),
            metricRow(
                "Connection",
                `${snap.connectionState}${snap.reconnectAttempt ? ` #${snap.reconnectAttempt}` : ""}`,
                connectionTone,
            ),
            metricRow("WS Messages", String(snap.wsMessages)),
            metricRow("WS Total", formatBytes(snap.wsBytesTotal)),
            metricRow("WS Last / Max", `${formatBytes(snap.wsLastBytes)} / ${formatBytes(snap.wsMaxBytes)}`),
            metricRow("Repo Summary / Delta", `${snap.repoSummaries} / ${snap.deltas}`),
            metricRow("Repo Commits", String(snap.repoSummaryCommits)),
            metricRow("Added Commits", String(snap.addedCommits)),
            metricRow("Bootstrap", `${snap.bootstrapChunks} chunks · ${snap.bootstrapCommits} commits`),
            metricRow("Layout", `${graph.layoutMode ?? "?"} · ${renderPerSec.toFixed(1)} r/s`),
            metricRow("Nodes / Links", `${graph.nodesCount ?? 0} / ${graph.linksCount ?? 0}`),
            metricRow("Known / Indexed", `${graph.commitsCount ?? 0} / ${graph.commitIndexSize ?? 0}`),
            metricRow("Materialized", String(graph.materializedCommits ?? 0)),
            metricRow("Viewport Entries", String(graph.viewportEntries ?? 0)),
            metricRow(
                "Hydration",
                `pending ${graph.hydrationPending ?? 0} · inflight ${graph.hydrationInflight ?? 0} · fetched ${graph.hydrationFetched ?? 0}`,
            ),
            metricRow(
                "Diffstats",
                `req ${snap.diffStatsRequests} · fail ${snap.diffStatsFailures} · limit ${snap.diffStatsLastLimit}`,
            ),
        ];
        metrics.replaceChildren(...rows);
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
        triggerEl,
        panelEl,
        toggle,
        hide,
        isVisible,
        destroy() {
            unsubscribe();
            if (loopId !== null) clearInterval(loopId);
            if (rafId !== null) cancelAnimationFrame(rafId);
            window.removeEventListener("keydown", onKeyDown);
            document.removeEventListener("click", onDocClick);
            panelEl.remove();
            triggerEl.remove();
        },
    };
}

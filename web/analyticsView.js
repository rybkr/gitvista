/**
 * Analytics view — commit velocity trend line chart.
 *
 * Factory: createAnalyticsView({ getCommits, getTags })
 * Returns: { el, update() }
 */

const PERIODS = [
    { label: "3m", months: 3 },
    { label: "6m", months: 6 },
    { label: "1y", months: 12 },
    { label: "All", months: 0 },
];

const CHART_HEIGHT = 200;
const ROLLING_WINDOW = 4;
const MS_PER_WEEK = 7 * 24 * 60 * 60 * 1000;

/** Returns the Monday-based ISO week start for a given date. */
function weekStart(date) {
    const d = new Date(date);
    const day = d.getUTCDay();
    const diff = (day === 0 ? -6 : 1) - day;
    d.setUTCDate(d.getUTCDate() + diff);
    d.setUTCHours(0, 0, 0, 0);
    return d.getTime();
}

/** Formats a timestamp as "Mon YYYY" for axis labels. */
function formatMonthYear(ts) {
    const d = new Date(ts);
    const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun",
        "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
    return `${months[d.getUTCMonth()]} ${d.getUTCFullYear()}`;
}

/** Formats a date range as "Mon D – Mon D, YYYY". */
function formatWeekRange(ts) {
    const start = new Date(ts);
    const end = new Date(ts + 6 * 24 * 60 * 60 * 1000);
    const months = ["Jan", "Feb", "Mar", "Apr", "May", "Jun",
        "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
    const s = `${months[start.getUTCMonth()]} ${start.getUTCDate()}`;
    const e = `${months[end.getUTCMonth()]} ${end.getUTCDate()}, ${end.getUTCFullYear()}`;
    return `${s} – ${e}`;
}

/** Reads a CSS variable from the document. */
function cssVar(name) {
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

/**
 * Buckets commits into ISO weeks and computes rolling average.
 *
 * @param {Map<string, object>} commits
 * @param {number} periodMonths - 0 means all
 * @returns {{ weeks: {ts: number, count: number, avg: number}[], totalCommits: number, avgPerWeek: number, bestWeek: {ts: number, count: number} | null }}
 */
function computeVelocity(commits, periodMonths) {
    const timestamps = [];
    for (const c of commits.values()) {
        const when = c.author?.when || c.author?.When;
        if (when) timestamps.push(new Date(when).getTime());
    }

    if (timestamps.length === 0) {
        return { weeks: [], totalCommits: 0, avgPerWeek: 0, bestWeek: null };
    }

    timestamps.sort((a, b) => a - b);

    let rangeStart = timestamps[0];
    const rangeEnd = Date.now();

    if (periodMonths > 0) {
        const cutoff = new Date();
        cutoff.setUTCMonth(cutoff.getUTCMonth() - periodMonths);
        rangeStart = Math.max(rangeStart, cutoff.getTime());
    }

    const firstWeek = weekStart(rangeStart);
    const lastWeek = weekStart(rangeEnd);

    // Bucket commits into weeks
    const buckets = new Map();
    for (let w = firstWeek; w <= lastWeek; w += MS_PER_WEEK) {
        buckets.set(w, 0);
    }
    for (const ts of timestamps) {
        if (ts < rangeStart) continue;
        const w = weekStart(ts);
        if (buckets.has(w)) {
            buckets.set(w, buckets.get(w) + 1);
        }
    }

    // Build sorted array
    const weeks = [];
    for (const [ts, count] of buckets) {
        weeks.push({ ts, count, avg: 0 });
    }
    weeks.sort((a, b) => a.ts - b.ts);

    // Compute rolling average
    for (let i = 0; i < weeks.length; i++) {
        let sum = 0;
        let n = 0;
        for (let j = Math.max(0, i - ROLLING_WINDOW + 1); j <= i; j++) {
            sum += weeks[j].count;
            n++;
        }
        weeks[i].avg = sum / n;
    }

    // Stats
    const totalCommits = weeks.reduce((s, w) => s + w.count, 0);
    const avgPerWeek = weeks.length > 0 ? totalCommits / weeks.length : 0;
    let bestWeek = null;
    for (const w of weeks) {
        if (!bestWeek || w.count > bestWeek.count) {
            bestWeek = { ts: w.ts, count: w.count };
        }
    }

    return { weeks, totalCommits, avgPerWeek, bestWeek };
}

/**
 * @param {{ getCommits: () => Map<string, object>, getTags: () => Map<string, string> }} deps
 */
export function createAnalyticsView({ getCommits, getTags }) {
    let selectedPeriod = "All";

    // ── Root element ──
    const el = document.createElement("div");
    el.className = "analytics-view";

    // ── Summary stats bar ──
    const summary = document.createElement("div");
    summary.className = "analytics-summary";

    function makeStat(label) {
        const stat = document.createElement("div");
        stat.className = "analytics-stat";
        const value = document.createElement("span");
        value.className = "analytics-stat-value";
        const lbl = document.createElement("span");
        lbl.className = "analytics-stat-label";
        lbl.textContent = label;
        stat.appendChild(value);
        stat.appendChild(lbl);
        return { el: stat, value };
    }

    const totalStat = makeStat("Total commits");
    const avgStat = makeStat("Avg / week");
    const bestStat = makeStat("Best week");
    summary.appendChild(totalStat.el);
    summary.appendChild(avgStat.el);
    summary.appendChild(bestStat.el);

    // ── Period selector ──
    const periodSelector = document.createElement("div");
    periodSelector.className = "analytics-period-selector";

    const periodButtons = [];
    for (const p of PERIODS) {
        const btn = document.createElement("button");
        btn.className = "analytics-period-btn";
        btn.textContent = p.label;
        btn.addEventListener("click", () => {
            selectedPeriod = p.label;
            update();
        });
        periodSelector.appendChild(btn);
        periodButtons.push({ btn, period: p });
    }

    // ── Chart container + canvas ──
    const chartContainer = document.createElement("div");
    chartContainer.className = "analytics-chart-container";

    const canvas = document.createElement("canvas");
    canvas.className = "analytics-chart-canvas";
    chartContainer.appendChild(canvas);

    // ── Tooltip ──
    const tooltip = document.createElement("div");
    tooltip.className = "analytics-tooltip";
    chartContainer.appendChild(tooltip);

    // ── Empty state ──
    const emptyState = document.createElement("div");
    emptyState.className = "analytics-empty";
    emptyState.textContent = "No commit history available. Push some commits to see velocity data.";

    el.appendChild(summary);
    el.appendChild(periodSelector);
    el.appendChild(chartContainer);
    el.appendChild(emptyState);

    // ── Chart state ──
    let currentData = null;
    const padding = { top: 20, right: 16, bottom: 32, left: 40 };

    /** Maps canvas mouse position to data coordinates. Returns week index or -1. */
    function hitTest(mouseX) {
        if (!currentData || currentData.weeks.length < 2) return -1;
        const rect = canvas.getBoundingClientRect();
        const plotWidth = rect.width - padding.left - padding.right;
        const x = mouseX - padding.left;
        if (x < 0 || x > plotWidth) return -1;
        const idx = Math.round((x / plotWidth) * (currentData.weeks.length - 1));
        return Math.max(0, Math.min(currentData.weeks.length - 1, idx));
    }

    canvas.addEventListener("mousemove", (e) => {
        const rect = canvas.getBoundingClientRect();
        const mx = e.clientX - rect.left;
        const my = e.clientY - rect.top;
        const idx = hitTest(mx);
        if (idx >= 0 && currentData) {
            const w = currentData.weeks[idx];
            tooltip.textContent = `${formatWeekRange(w.ts)}: ${w.count} commit${w.count !== 1 ? "s" : ""}`;
            tooltip.style.display = "block";
            // Position tooltip near mouse
            const plotWidth = rect.width - padding.left - padding.right;
            const tx = padding.left + (idx / (currentData.weeks.length - 1)) * plotWidth;
            tooltip.style.left = `${Math.min(tx, rect.width - 160)}px`;
            tooltip.style.top = `${Math.max(0, my - 36)}px`;
        } else {
            tooltip.style.display = "none";
        }
    });

    canvas.addEventListener("mouseleave", () => {
        tooltip.style.display = "none";
    });

    // ── Resize observer ──
    const resizeObserver = new ResizeObserver(() => {
        if (currentData) drawChart(currentData);
    });
    resizeObserver.observe(chartContainer);

    /** Renders the chart onto the canvas. */
    function drawChart(data) {
        currentData = data;
        const { weeks } = data;

        const rect = chartContainer.getBoundingClientRect();
        const width = Math.max(rect.width, 100);
        const height = CHART_HEIGHT;
        const dpr = window.devicePixelRatio || 1;

        canvas.width = width * dpr;
        canvas.height = height * dpr;
        canvas.style.width = `${width}px`;
        canvas.style.height = `${height}px`;

        const ctx = canvas.getContext("2d");
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        // Read theme colors
        const textColor = cssVar("--text-secondary") || "#57606a";
        const nodeColor = cssVar("--node-color") || "#0ea5e9";
        const borderColor = cssVar("--border-color") || "#d8dce2";
        const successColor = cssVar("--success-color") || "#059669";
        const warningColor = cssVar("--warning-color") || "#d97706";

        ctx.clearRect(0, 0, width, height);

        if (weeks.length < 2) return;

        const plotWidth = width - padding.left - padding.right;
        const plotHeight = height - padding.top - padding.bottom;

        const maxCount = Math.max(...weeks.map((w) => w.count), 1);
        const maxY = Math.ceil(maxCount * 1.1);

        // ── X helper ──
        const xAt = (i) => padding.left + (i / (weeks.length - 1)) * plotWidth;
        const yAt = (v) => padding.top + plotHeight - (v / maxY) * plotHeight;

        // ── Axes ──
        ctx.strokeStyle = borderColor;
        ctx.lineWidth = 1;

        // Y-axis line
        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top);
        ctx.lineTo(padding.left, padding.top + plotHeight);
        ctx.stroke();

        // X-axis line
        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top + plotHeight);
        ctx.lineTo(padding.left + plotWidth, padding.top + plotHeight);
        ctx.stroke();

        // Y-axis ticks
        ctx.font = "10px 'Geist', system-ui, sans-serif";
        ctx.fillStyle = textColor;
        ctx.textAlign = "right";
        ctx.textBaseline = "middle";
        const yTicks = Math.min(5, maxY);
        for (let i = 0; i <= yTicks; i++) {
            const v = Math.round((maxY / yTicks) * i);
            const y = yAt(v);
            ctx.fillText(String(v), padding.left - 6, y);
            if (i > 0) {
                ctx.save();
                ctx.strokeStyle = borderColor;
                ctx.globalAlpha = 0.3;
                ctx.setLineDash([3, 3]);
                ctx.beginPath();
                ctx.moveTo(padding.left, y);
                ctx.lineTo(padding.left + plotWidth, y);
                ctx.stroke();
                ctx.restore();
            }
        }

        // X-axis labels (month ticks)
        ctx.textAlign = "center";
        ctx.textBaseline = "top";
        let lastLabel = "";
        const labelInterval = Math.max(1, Math.floor(weeks.length / 8));
        for (let i = 0; i < weeks.length; i += labelInterval) {
            const label = formatMonthYear(weeks[i].ts);
            if (label !== lastLabel) {
                ctx.fillText(label, xAt(i), padding.top + plotHeight + 6);
                lastLabel = label;
            }
        }

        // ── Tag / release lines ──
        try {
            const tags = getTags();
            if (tags && tags.size > 0) {
                const commits = getCommits();
                // Map tag target hashes to timestamps
                const tagTimestamps = [];
                for (const [tagName, targetHash] of tags) {
                    const commit = commits.get(targetHash);
                    if (commit) {
                        const when = commit.author?.when || commit.author?.When;
                        if (when) tagTimestamps.push(new Date(when).getTime());
                    }
                }

                if (tagTimestamps.length > 0 && weeks.length >= 2) {
                    ctx.save();
                    ctx.strokeStyle = warningColor;
                    ctx.globalAlpha = 0.4;
                    ctx.setLineDash([4, 4]);
                    ctx.lineWidth = 1;

                    const firstTs = weeks[0].ts;
                    const lastTs = weeks[weeks.length - 1].ts;
                    const range = lastTs - firstTs;

                    for (const ts of tagTimestamps) {
                        if (ts < firstTs || ts > lastTs || range === 0) continue;
                        const x = padding.left + ((ts - firstTs) / range) * plotWidth;
                        ctx.beginPath();
                        ctx.moveTo(x, padding.top);
                        ctx.lineTo(x, padding.top + plotHeight);
                        ctx.stroke();
                    }
                    ctx.restore();
                }
            }
        } catch {
            // Tags not available — skip
        }

        // ── Filled area under primary line ──
        const gradient = ctx.createLinearGradient(0, padding.top, 0, padding.top + plotHeight);
        gradient.addColorStop(0, nodeColor + "33"); // ~20% opacity
        gradient.addColorStop(1, nodeColor + "05"); // ~2% opacity

        ctx.beginPath();
        ctx.moveTo(xAt(0), yAt(0));
        for (let i = 0; i < weeks.length; i++) {
            ctx.lineTo(xAt(i), yAt(weeks[i].count));
        }
        ctx.lineTo(xAt(weeks.length - 1), yAt(0));
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();

        // ── Primary line (weekly count) ──
        ctx.beginPath();
        ctx.strokeStyle = nodeColor;
        ctx.lineWidth = 1.5;
        ctx.setLineDash([]);
        for (let i = 0; i < weeks.length; i++) {
            const x = xAt(i);
            const y = yAt(weeks[i].count);
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        }
        ctx.stroke();

        // ── Rolling average line ──
        ctx.beginPath();
        ctx.strokeStyle = successColor;
        ctx.lineWidth = 2;
        ctx.setLineDash([]);
        for (let i = 0; i < weeks.length; i++) {
            const x = xAt(i);
            const y = yAt(weeks[i].avg);
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        }
        ctx.stroke();
    }

    /** Main update — re-reads commits and redraws everything. */
    function update() {
        const commits = getCommits();
        const period = PERIODS.find((p) => p.label === selectedPeriod) || PERIODS[3];

        // Update period button states
        for (const { btn, period: p } of periodButtons) {
            btn.classList.toggle("is-active", p.label === selectedPeriod);
        }

        if (!commits || commits.size === 0) {
            emptyState.style.display = "block";
            summary.style.display = "none";
            periodSelector.style.display = "none";
            chartContainer.style.display = "none";
            return;
        }

        emptyState.style.display = "none";
        summary.style.display = "";
        periodSelector.style.display = "";
        chartContainer.style.display = "";

        const data = computeVelocity(commits, period.months);

        // Update summary stats
        totalStat.value.textContent = data.totalCommits.toLocaleString();
        avgStat.value.textContent = data.avgPerWeek.toFixed(1);
        bestStat.value.textContent = data.bestWeek
            ? `${data.bestWeek.count} (${formatMonthYear(data.bestWeek.ts)})`
            : "—";

        drawChart(data);
    }

    return { el, update };
}

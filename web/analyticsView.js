/**
 * Analytics view — commit velocity trend line chart, author contributions,
 * activity heatmap, merge statistics, change size distribution, and rework rate.
 *
 * Factory: createAnalyticsView({ getCommits, getTags, fetchDiffStats })
 * Returns: { el, update() }
 */

import { getAuthorColor } from "./utils/colors.js";

const PERIODS = [
    { label: "3m", months: 3 },
    { label: "6m", months: 6 },
    { label: "1y", months: 12 },
    { label: "All", months: 0 },
];

const CHART_HEIGHT = 200;
const ROLLING_WINDOW = 4;
const MS_PER_WEEK = 7 * 24 * 60 * 60 * 1000;
const TOP_N_AUTHORS = 10;
const DAY_NAMES = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"];
const HOUR_LABELS = ["12a", "", "", "3a", "", "", "6a", "", "", "9a", "", "", "12p", "", "", "3p", "", "", "6p", "", "", "9p", "", ""];

const CHURN_WINDOW_DAYS = 21;
const SIZE_BUCKETS = [
    { label: "XS", max: 5 },
    { label: "S", max: 20 },
    { label: "M", max: 50 },
    { label: "L", max: 100 },
    { label: "XL", max: Infinity },
];

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
 * Groups commits by author email, returns top authors sorted by count.
 *
 * @param {Map<string, object>} commits
 * @param {number} periodMonths - 0 means all
 * @returns {{ authors: {name: string, email: string, count: number}[], totalInPeriod: number }}
 */
function computeAuthorCounts(commits, periodMonths) {
    let cutoff = 0;
    if (periodMonths > 0) {
        const d = new Date();
        d.setUTCMonth(d.getUTCMonth() - periodMonths);
        cutoff = d.getTime();
    }

    const byEmail = new Map();
    let totalInPeriod = 0;

    for (const c of commits.values()) {
        const when = c.author?.when || c.author?.When;
        if (!when) continue;
        const ts = new Date(when).getTime();
        if (cutoff > 0 && ts < cutoff) continue;
        totalInPeriod++;

        const email = c.author?.email || c.author?.Email || "unknown";
        const name = c.author?.name || c.author?.Name || email;
        const entry = byEmail.get(email);
        if (entry) {
            entry.count++;
        } else {
            byEmail.set(email, { name, email, count: 1 });
        }
    }

    const authors = [...byEmail.values()]
        .sort((a, b) => b.count - a.count)
        .slice(0, TOP_N_AUTHORS);

    return { authors, totalInPeriod };
}

/**
 * Builds a 7×24 grid of commit counts (Mon=0..Sun=6, hour 0..23 UTC).
 *
 * @param {Map<string, object>} commits
 * @param {number} periodMonths - 0 means all
 * @returns {{ grid: number[][], max: number }}
 */
function computeHeatmapData(commits, periodMonths) {
    let cutoff = 0;
    if (periodMonths > 0) {
        const d = new Date();
        d.setUTCMonth(d.getUTCMonth() - periodMonths);
        cutoff = d.getTime();
    }

    const grid = Array.from({ length: 7 }, () => new Array(24).fill(0));
    let max = 0;

    for (const c of commits.values()) {
        const when = c.author?.when || c.author?.When;
        if (!when) continue;
        const ts = new Date(when).getTime();
        if (cutoff > 0 && ts < cutoff) continue;

        const d = new Date(when);
        const jsDay = d.getUTCDay(); // 0=Sun
        const day = jsDay === 0 ? 6 : jsDay - 1; // Mon=0..Sun=6
        const hour = d.getUTCHours();
        grid[day][hour]++;
        if (grid[day][hour] > max) max = grid[day][hour];
    }

    return { grid, max };
}

/**
 * Computes merge commit statistics.
 *
 * @param {Map<string, object>} commits
 * @param {number} periodMonths - 0 means all
 * @returns {{ mergeCount: number, totalCount: number, mergePercent: number, mergesPerWeek: number }}
 */
function computeMergeStats(commits, periodMonths) {
    let cutoff = 0;
    if (periodMonths > 0) {
        const d = new Date();
        d.setUTCMonth(d.getUTCMonth() - periodMonths);
        cutoff = d.getTime();
    }

    let mergeCount = 0;
    let totalCount = 0;
    let minTs = Infinity;
    let maxTs = -Infinity;

    for (const c of commits.values()) {
        const when = c.author?.when || c.author?.When;
        if (!when) continue;
        const ts = new Date(when).getTime();
        if (cutoff > 0 && ts < cutoff) continue;
        totalCount++;
        if (ts < minTs) minTs = ts;
        if (ts > maxTs) maxTs = ts;

        const parents = c.parents || c.Parents || [];
        if (parents.length > 1) mergeCount++;
    }

    const mergePercent = totalCount > 0 ? (mergeCount / totalCount) * 100 : 0;
    const spanWeeks = totalCount > 0 ? Math.max(1, (maxTs - minTs) / MS_PER_WEEK) : 1;
    const mergesPerWeek = mergeCount / spanWeeks;

    return { mergeCount, totalCount, mergePercent, mergesPerWeek };
}

/**
 * Buckets commits by number of files changed into XS/S/M/L/XL.
 *
 * @param {Map<string, object>} commits
 * @param {Map<string, object>} diffStats - hash → { filesChanged, files }
 * @param {number} periodMonths - 0 means all
 * @returns {{ buckets: {label: string, count: number}[], median: number, avgSize: number }}
 */
function computeChangeSizeDistribution(commits, diffStats, periodMonths) {
    let cutoff = 0;
    if (periodMonths > 0) {
        const d = new Date();
        d.setUTCMonth(d.getUTCMonth() - periodMonths);
        cutoff = d.getTime();
    }

    const sizes = [];
    for (const [hash, c] of commits) {
        const when = c.author?.when || c.author?.When;
        if (!when) continue;
        const ts = new Date(when).getTime();
        if (cutoff > 0 && ts < cutoff) continue;

        const stats = diffStats.get(hash);
        if (!stats) continue;
        sizes.push(stats.filesChanged);
    }

    const buckets = SIZE_BUCKETS.map((b) => ({ label: b.label, count: 0 }));
    for (const size of sizes) {
        for (let i = 0; i < SIZE_BUCKETS.length; i++) {
            if (size <= SIZE_BUCKETS[i].max) {
                buckets[i].count++;
                break;
            }
        }
    }

    sizes.sort((a, b) => a - b);
    const median = sizes.length > 0 ? sizes[Math.floor(sizes.length / 2)] : 0;
    const avgSize = sizes.length > 0 ? sizes.reduce((s, v) => s + v, 0) / sizes.length : 0;

    return { buckets, median, avgSize };
}

/**
 * Computes weekly rework rate — proportion of files modified that were also
 * modified within the prior CHURN_WINDOW_DAYS days.
 *
 * @param {Map<string, object>} commits
 * @param {Map<string, object>} diffStats - hash → { filesChanged, files }
 * @param {number} periodMonths - 0 means all
 * @returns {{ weeks: {ts: number, rate: number}[], avgRate: number }}
 */
function computeReworkRate(commits, diffStats, periodMonths) {
    let cutoff = 0;
    if (periodMonths > 0) {
        const d = new Date();
        d.setUTCMonth(d.getUTCMonth() - periodMonths);
        cutoff = d.getTime();
    }

    // Collect commits with timestamps and file lists, sorted by time
    const entries = [];
    for (const [hash, c] of commits) {
        const when = c.author?.when || c.author?.When;
        if (!when) continue;
        const ts = new Date(when).getTime();
        if (cutoff > 0 && ts < cutoff) continue;

        const stats = diffStats.get(hash);
        if (!stats || !stats.files || stats.files.length === 0) continue;
        entries.push({ ts, files: stats.files });
    }
    entries.sort((a, b) => a.ts - b.ts);

    if (entries.length === 0) {
        return { weeks: [], avgRate: 0 };
    }

    const windowMs = CHURN_WINDOW_DAYS * 24 * 60 * 60 * 1000;

    // Group by ISO week
    const weekBuckets = new Map();
    for (const entry of entries) {
        const ws = weekStart(entry.ts);
        if (!weekBuckets.has(ws)) {
            weekBuckets.set(ws, []);
        }
        weekBuckets.get(ws).push(entry);
    }

    const weeks = [];
    for (const [ws, weekEntries] of weekBuckets) {
        let totalFiles = 0;
        let reworkedFiles = 0;

        for (const entry of weekEntries) {
            for (const file of entry.files) {
                totalFiles++;
                // Check if this file was modified in a prior commit within the window
                for (const other of entries) {
                    if (other.ts >= entry.ts) break;
                    if (entry.ts - other.ts > windowMs) continue;
                    if (other.files.includes(file)) {
                        reworkedFiles++;
                        break;
                    }
                }
            }
        }

        const rate = totalFiles > 0 ? (reworkedFiles / totalFiles) * 100 : 0;
        weeks.push({ ts: ws, rate });
    }

    weeks.sort((a, b) => a.ts - b.ts);
    const avgRate = weeks.length > 0
        ? weeks.reduce((s, w) => s + w.rate, 0) / weeks.length
        : 0;

    return { weeks, avgRate };
}

/**
 * @param {{ getCommits: () => Map<string, object>, getTags: () => Map<string, string>, fetchDiffStats: () => Promise<object> }} deps
 */
export function createAnalyticsView({ getCommits, getTags, fetchDiffStats }) {
    let selectedPeriod = "All";

    // ── Diff stats async cache ──
    let diffStatsCache = null;
    let diffStatsLoading = false;
    let diffStatsError = false;

    async function loadDiffStats() {
        if (diffStatsCache || diffStatsLoading || !fetchDiffStats) return;
        diffStatsLoading = true;
        diffStatsError = false;
        updateDiffStatsUI();

        try {
            const raw = await fetchDiffStats();
            diffStatsCache = new Map(Object.entries(raw));
            diffStatsLoading = false;
            updateDiffStatsUI();
            redrawDiffStatsCharts();
        } catch (err) {
            diffStatsLoading = false;
            diffStatsError = true;
            updateDiffStatsUI();
        }
    }

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

    // ── Section helper ──
    function makeSection(title) {
        const section = document.createElement("div");
        section.className = "analytics-section";
        const h3 = document.createElement("h3");
        h3.className = "analytics-section-title";
        h3.textContent = title;
        section.appendChild(h3);
        return section;
    }

    // ── Author contributions section ──
    const authorSection = makeSection("Top Contributors");
    const authorChartContainer = document.createElement("div");
    authorChartContainer.className = "analytics-chart-container";
    const authorCanvas = document.createElement("canvas");
    authorCanvas.className = "analytics-chart-canvas";
    authorChartContainer.appendChild(authorCanvas);
    authorSection.appendChild(authorChartContainer);
    el.appendChild(authorSection);

    // ── Heatmap section ──
    const heatmapSection = makeSection("Activity Heatmap");
    const heatmapChartContainer = document.createElement("div");
    heatmapChartContainer.className = "analytics-chart-container";
    const heatmapCanvas = document.createElement("canvas");
    heatmapCanvas.className = "analytics-chart-canvas";
    heatmapChartContainer.appendChild(heatmapCanvas);
    heatmapSection.appendChild(heatmapChartContainer);
    el.appendChild(heatmapSection);

    // ── Merge stats section ──
    const mergeSection = makeSection("Merge Statistics");
    const mergeSummary = document.createElement("div");
    mergeSummary.className = "analytics-summary";
    const mergeCountStat = makeStat("Merges");
    const mergePercentStat = makeStat("Merge %");
    const mergesPerWeekStat = makeStat("Merges / week");
    mergeSummary.appendChild(mergeCountStat.el);
    mergeSummary.appendChild(mergePercentStat.el);
    mergeSummary.appendChild(mergesPerWeekStat.el);
    mergeSection.appendChild(mergeSummary);
    el.appendChild(mergeSection);

    // ── Change size distribution section ──
    const changeSizeSection = makeSection("Change Size Distribution");
    const changeSizeChartContainer = document.createElement("div");
    changeSizeChartContainer.className = "analytics-chart-container";
    const changeSizeCanvas = document.createElement("canvas");
    changeSizeCanvas.className = "analytics-chart-canvas";
    changeSizeChartContainer.appendChild(changeSizeCanvas);
    changeSizeSection.appendChild(changeSizeChartContainer);
    const changeSizeSummary = document.createElement("div");
    changeSizeSummary.className = "analytics-summary";
    const medianSizeStat = makeStat("Median size");
    const avgSizeStat = makeStat("Avg size");
    changeSizeSummary.appendChild(medianSizeStat.el);
    changeSizeSummary.appendChild(avgSizeStat.el);
    changeSizeSection.appendChild(changeSizeSummary);
    el.appendChild(changeSizeSection);

    // ── Rework rate section ──
    const reworkSection = makeSection("Rework Rate");
    const reworkChartContainer = document.createElement("div");
    reworkChartContainer.className = "analytics-chart-container";
    const reworkCanvas = document.createElement("canvas");
    reworkCanvas.className = "analytics-chart-canvas";
    reworkChartContainer.appendChild(reworkCanvas);
    reworkSection.appendChild(reworkChartContainer);
    const reworkSummary = document.createElement("div");
    reworkSummary.className = "analytics-summary";
    const avgReworkStat = makeStat("Avg rework %");
    reworkSummary.appendChild(avgReworkStat.el);
    reworkSection.appendChild(reworkSummary);
    el.appendChild(reworkSection);

    // ── Diff stats loading/error message ──
    const diffStatsMsg = document.createElement("div");
    diffStatsMsg.className = "analytics-diff-stats-msg";
    diffStatsMsg.style.display = "none";
    // Insert before changeSizeSection title
    changeSizeSection.insertBefore(diffStatsMsg, changeSizeSection.firstChild);

    function updateDiffStatsUI() {
        if (diffStatsLoading) {
            diffStatsMsg.textContent = "Loading diff stats...";
            diffStatsMsg.style.display = "block";
            changeSizeChartContainer.style.display = "none";
            changeSizeSummary.style.display = "none";
            reworkChartContainer.style.display = "none";
            reworkSummary.style.display = "none";
            reworkSection.querySelector(".analytics-section-title").style.opacity = "0.5";
        } else if (diffStatsError) {
            diffStatsMsg.textContent = "Failed to load diff stats.";
            diffStatsMsg.style.display = "block";
            changeSizeChartContainer.style.display = "none";
            changeSizeSummary.style.display = "none";
            reworkChartContainer.style.display = "none";
            reworkSummary.style.display = "none";
            reworkSection.querySelector(".analytics-section-title").style.opacity = "0.5";
        } else {
            diffStatsMsg.style.display = "none";
            changeSizeChartContainer.style.display = "";
            changeSizeSummary.style.display = "";
            reworkChartContainer.style.display = "";
            reworkSummary.style.display = "";
            reworkSection.querySelector(".analytics-section-title").style.opacity = "";
        }
    }

    // ── Chart state ──
    let currentData = null;
    let cachedAuthorData = null;
    let cachedHeatmapData = null;
    let cachedChangeSizeData = null;
    let cachedReworkData = null;
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
        if (cachedAuthorData) drawAuthorChart(cachedAuthorData);
        if (cachedHeatmapData) drawHeatmap(cachedHeatmapData);
        if (cachedChangeSizeData) drawChangeSizeChart(cachedChangeSizeData);
        if (cachedReworkData) drawReworkChart(cachedReworkData);
    });
    resizeObserver.observe(chartContainer);
    resizeObserver.observe(authorChartContainer);
    resizeObserver.observe(heatmapChartContainer);
    resizeObserver.observe(changeSizeChartContainer);
    resizeObserver.observe(reworkChartContainer);

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

    /** Renders the author bar chart onto the author canvas. */
    function drawAuthorChart(data) {
        cachedAuthorData = data;
        const { authors } = data;
        if (authors.length === 0) return;

        const barHeight = 22;
        const gap = 6;
        const labelWidth = 100;
        const countWidth = 40;
        const chartPadding = { top: 4, right: 8, bottom: 4, left: labelWidth + 8 };
        const totalHeight = authors.length * (barHeight + gap) + chartPadding.top + chartPadding.bottom;

        const rect = authorChartContainer.getBoundingClientRect();
        const width = Math.max(rect.width, 100);
        const dpr = window.devicePixelRatio || 1;

        authorCanvas.width = width * dpr;
        authorCanvas.height = totalHeight * dpr;
        authorCanvas.style.width = `${width}px`;
        authorCanvas.style.height = `${totalHeight}px`;

        const ctx = authorCanvas.getContext("2d");
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, width, totalHeight);

        const textColor = cssVar("--text-secondary") || "#57606a";
        const maxCount = authors[0].count;
        const barAreaWidth = width - chartPadding.left - chartPadding.right - countWidth;

        ctx.font = "11px 'Geist', system-ui, sans-serif";
        ctx.textBaseline = "middle";

        for (let i = 0; i < authors.length; i++) {
            const a = authors[i];
            const y = chartPadding.top + i * (barHeight + gap);
            const barWidth = Math.max(2, (a.count / maxCount) * barAreaWidth);
            const radius = 3;

            // Author name (truncated)
            ctx.fillStyle = textColor;
            ctx.textAlign = "right";
            let displayName = a.name;
            if (displayName.length > 16) displayName = displayName.slice(0, 15) + "\u2026";
            ctx.fillText(displayName, chartPadding.left - 8, y + barHeight / 2);

            // Bar
            ctx.fillStyle = getAuthorColor(a.email);
            ctx.beginPath();
            ctx.roundRect(chartPadding.left, y, barWidth, barHeight, radius);
            ctx.fill();

            // Count label
            ctx.fillStyle = textColor;
            ctx.textAlign = "left";
            ctx.fillText(String(a.count), chartPadding.left + barWidth + 6, y + barHeight / 2);
        }
    }

    /** Renders the activity heatmap onto the heatmap canvas. */
    function drawHeatmap(data) {
        cachedHeatmapData = data;
        const { grid, max } = data;

        const rect = heatmapChartContainer.getBoundingClientRect();
        const availWidth = Math.max(rect.width, 100);
        const labelLeftWidth = 32;
        const labelTopHeight = 16;
        const cellGap = 2;
        const cellSize = Math.max(8, Math.floor((availWidth - labelLeftWidth - cellGap * 23) / 24));
        const totalWidth = labelLeftWidth + 24 * (cellSize + cellGap);
        const totalHeight = labelTopHeight + 7 * (cellSize + cellGap) + 4;

        const dpr = window.devicePixelRatio || 1;
        heatmapCanvas.width = totalWidth * dpr;
        heatmapCanvas.height = totalHeight * dpr;
        heatmapCanvas.style.width = `${totalWidth}px`;
        heatmapCanvas.style.height = `${totalHeight}px`;

        const ctx = heatmapCanvas.getContext("2d");
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
        ctx.clearRect(0, 0, totalWidth, totalHeight);

        const textColor = cssVar("--text-secondary") || "#57606a";
        const nodeColor = cssVar("--node-color") || "#0ea5e9";
        const borderColor = cssVar("--border-color") || "#d8dce2";

        // Hour labels (top)
        ctx.font = "9px 'Geist', system-ui, sans-serif";
        ctx.fillStyle = textColor;
        ctx.textAlign = "center";
        ctx.textBaseline = "bottom";
        for (let h = 0; h < 24; h++) {
            if (HOUR_LABELS[h]) {
                const x = labelLeftWidth + h * (cellSize + cellGap) + cellSize / 2;
                ctx.fillText(HOUR_LABELS[h], x, labelTopHeight - 2);
            }
        }

        // Day labels (left) and cells
        ctx.textAlign = "right";
        ctx.textBaseline = "middle";
        ctx.font = "10px 'Geist', system-ui, sans-serif";

        for (let day = 0; day < 7; day++) {
            const y = labelTopHeight + day * (cellSize + cellGap);

            // Day label
            ctx.fillStyle = textColor;
            ctx.fillText(DAY_NAMES[day], labelLeftWidth - 6, y + cellSize / 2);

            for (let hour = 0; hour < 24; hour++) {
                const x = labelLeftWidth + hour * (cellSize + cellGap);
                const count = grid[day][hour];

                if (count === 0 || max === 0) {
                    // Empty cell
                    ctx.fillStyle = borderColor;
                    ctx.globalAlpha = 0.3;
                    ctx.fillRect(x, y, cellSize, cellSize);
                    ctx.globalAlpha = 1;
                } else {
                    // Filled cell — opacity scales with count
                    const alpha = 0.15 + 0.85 * (count / max);
                    ctx.fillStyle = nodeColor;
                    ctx.globalAlpha = alpha;
                    ctx.fillRect(x, y, cellSize, cellSize);
                    ctx.globalAlpha = 1;
                }
            }
        }
    }

    /** Renders the change size histogram onto its canvas. */
    function drawChangeSizeChart(data) {
        cachedChangeSizeData = data;
        const { buckets } = data;
        if (buckets.length === 0) return;

        const rect = changeSizeChartContainer.getBoundingClientRect();
        const width = Math.max(rect.width, 100);
        const height = CHART_HEIGHT;
        const dpr = window.devicePixelRatio || 1;

        changeSizeCanvas.width = width * dpr;
        changeSizeCanvas.height = height * dpr;
        changeSizeCanvas.style.width = `${width}px`;
        changeSizeCanvas.style.height = `${height}px`;

        const ctx = changeSizeCanvas.getContext("2d");
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        const textColor = cssVar("--text-secondary") || "#57606a";
        const nodeColor = cssVar("--node-color") || "#0ea5e9";
        const borderColor = cssVar("--border-color") || "#d8dce2";

        ctx.clearRect(0, 0, width, height);

        const barPadding = { top: 20, right: 16, bottom: 32, left: 40 };
        const plotWidth = width - barPadding.left - barPadding.right;
        const plotHeight = height - barPadding.top - barPadding.bottom;

        const maxCount = Math.max(...buckets.map((b) => b.count), 1);
        const barWidth = Math.floor(plotWidth / buckets.length) - 8;
        const gap = (plotWidth - barWidth * buckets.length) / (buckets.length + 1);

        // Y-axis
        ctx.strokeStyle = borderColor;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(barPadding.left, barPadding.top);
        ctx.lineTo(barPadding.left, barPadding.top + plotHeight);
        ctx.stroke();

        // X-axis
        ctx.beginPath();
        ctx.moveTo(barPadding.left, barPadding.top + plotHeight);
        ctx.lineTo(barPadding.left + plotWidth, barPadding.top + plotHeight);
        ctx.stroke();

        // Y-axis ticks
        ctx.font = "10px 'Geist', system-ui, sans-serif";
        ctx.fillStyle = textColor;
        ctx.textAlign = "right";
        ctx.textBaseline = "middle";
        const yTicks = Math.min(5, maxCount);
        for (let i = 0; i <= yTicks; i++) {
            const v = Math.round((maxCount / yTicks) * i);
            const y = barPadding.top + plotHeight - (v / maxCount) * plotHeight;
            ctx.fillText(String(v), barPadding.left - 6, y);
        }

        // Bars
        ctx.fillStyle = nodeColor;
        ctx.font = "10px 'Geist', system-ui, sans-serif";
        for (let i = 0; i < buckets.length; i++) {
            const x = barPadding.left + gap + i * (barWidth + gap);
            const barH = maxCount > 0 ? (buckets[i].count / maxCount) * plotHeight : 0;
            const y = barPadding.top + plotHeight - barH;

            ctx.globalAlpha = 0.8;
            ctx.beginPath();
            ctx.roundRect(x, y, barWidth, barH, 3);
            ctx.fill();
            ctx.globalAlpha = 1;

            // Count above bar
            ctx.fillStyle = textColor;
            ctx.textAlign = "center";
            ctx.textBaseline = "bottom";
            if (buckets[i].count > 0) {
                ctx.fillText(String(buckets[i].count), x + barWidth / 2, y - 4);
            }

            // Label below x-axis
            ctx.textBaseline = "top";
            ctx.fillText(buckets[i].label, x + barWidth / 2, barPadding.top + plotHeight + 6);

            ctx.fillStyle = nodeColor;
        }
    }

    /** Renders the rework rate line chart onto its canvas. */
    function drawReworkChart(data) {
        cachedReworkData = data;
        const { weeks } = data;
        if (weeks.length < 2) return;

        const rect = reworkChartContainer.getBoundingClientRect();
        const width = Math.max(rect.width, 100);
        const height = CHART_HEIGHT;
        const dpr = window.devicePixelRatio || 1;

        reworkCanvas.width = width * dpr;
        reworkCanvas.height = height * dpr;
        reworkCanvas.style.width = `${width}px`;
        reworkCanvas.style.height = `${height}px`;

        const ctx = reworkCanvas.getContext("2d");
        ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

        const textColor = cssVar("--text-secondary") || "#57606a";
        const warningColor = cssVar("--warning-color") || "#d97706";
        const borderColor = cssVar("--border-color") || "#d8dce2";

        ctx.clearRect(0, 0, width, height);

        const plotWidth = width - padding.left - padding.right;
        const plotHeight = height - padding.top - padding.bottom;

        const maxRate = Math.max(100, Math.ceil(Math.max(...weeks.map((w) => w.rate)) * 1.1));
        const xAt = (i) => padding.left + (i / (weeks.length - 1)) * plotWidth;
        const yAt = (v) => padding.top + plotHeight - (v / maxRate) * plotHeight;

        // Axes
        ctx.strokeStyle = borderColor;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top);
        ctx.lineTo(padding.left, padding.top + plotHeight);
        ctx.stroke();
        ctx.beginPath();
        ctx.moveTo(padding.left, padding.top + plotHeight);
        ctx.lineTo(padding.left + plotWidth, padding.top + plotHeight);
        ctx.stroke();

        // Y-axis ticks (percentage)
        ctx.font = "10px 'Geist', system-ui, sans-serif";
        ctx.fillStyle = textColor;
        ctx.textAlign = "right";
        ctx.textBaseline = "middle";
        for (let i = 0; i <= 4; i++) {
            const v = Math.round((maxRate / 4) * i);
            const y = yAt(v);
            ctx.fillText(`${v}%`, padding.left - 6, y);
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

        // X-axis labels
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

        // Filled area under line
        const gradient = ctx.createLinearGradient(0, padding.top, 0, padding.top + plotHeight);
        gradient.addColorStop(0, warningColor + "33");
        gradient.addColorStop(1, warningColor + "05");

        ctx.beginPath();
        ctx.moveTo(xAt(0), yAt(0));
        for (let i = 0; i < weeks.length; i++) {
            ctx.lineTo(xAt(i), yAt(weeks[i].rate));
        }
        ctx.lineTo(xAt(weeks.length - 1), yAt(0));
        ctx.closePath();
        ctx.fillStyle = gradient;
        ctx.fill();

        // Line
        ctx.beginPath();
        ctx.strokeStyle = warningColor;
        ctx.lineWidth = 2;
        ctx.setLineDash([]);
        for (let i = 0; i < weeks.length; i++) {
            const x = xAt(i);
            const y = yAt(weeks[i].rate);
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        }
        ctx.stroke();
    }

    /** Recomputes and redraws both diff-stats-dependent charts. */
    function redrawDiffStatsCharts() {
        if (!diffStatsCache) return;
        const commits = getCommits();
        if (!commits || commits.size === 0) return;
        const period = PERIODS.find((p) => p.label === selectedPeriod) || PERIODS[3];

        const changeSizeData = computeChangeSizeDistribution(commits, diffStatsCache, period.months);
        medianSizeStat.value.textContent = `${changeSizeData.median} files`;
        avgSizeStat.value.textContent = `${changeSizeData.avgSize.toFixed(1)} files`;
        drawChangeSizeChart(changeSizeData);

        const reworkData = computeReworkRate(commits, diffStatsCache, period.months);
        avgReworkStat.value.textContent = `${reworkData.avgRate.toFixed(1)}%`;
        drawReworkChart(reworkData);
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
            authorSection.style.display = "none";
            heatmapSection.style.display = "none";
            mergeSection.style.display = "none";
            changeSizeSection.style.display = "none";
            reworkSection.style.display = "none";
            return;
        }

        emptyState.style.display = "none";
        summary.style.display = "";
        periodSelector.style.display = "";
        chartContainer.style.display = "";
        authorSection.style.display = "";
        heatmapSection.style.display = "";
        mergeSection.style.display = "";
        changeSizeSection.style.display = "";
        reworkSection.style.display = "";

        const data = computeVelocity(commits, period.months);

        // Update summary stats
        totalStat.value.textContent = data.totalCommits.toLocaleString();
        avgStat.value.textContent = data.avgPerWeek.toFixed(1);
        bestStat.value.textContent = data.bestWeek
            ? `${data.bestWeek.count} (${formatMonthYear(data.bestWeek.ts)})`
            : "—";

        drawChart(data);

        // Author contributions
        const authorData = computeAuthorCounts(commits, period.months);
        drawAuthorChart(authorData);

        // Activity heatmap
        const heatmapData = computeHeatmapData(commits, period.months);
        drawHeatmap(heatmapData);

        // Merge statistics
        const mergeData = computeMergeStats(commits, period.months);
        mergeCountStat.value.textContent = mergeData.mergeCount.toLocaleString();
        mergePercentStat.value.textContent = `${mergeData.mergePercent.toFixed(1)}%`;
        mergesPerWeekStat.value.textContent = mergeData.mergesPerWeek.toFixed(1);

        // Diff-stats-dependent charts
        if (diffStatsCache) {
            redrawDiffStatsCharts();
        } else {
            loadDiffStats();
        }
    }

    return { el, update };
}

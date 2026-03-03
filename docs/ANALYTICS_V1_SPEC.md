# Analytics V1 Implementation Spec

## 1. Goal
Enable developers and team leads to detect delivery risk in a repository within 60 seconds, using actionable analytics (not passive charts).

## 2. Target Users
- Primary: IC developers working in-repo during day-to-day coding/review.
- Secondary: Team leads running weekly planning and risk review.

## 3. Core Decision This View Must Support
"Where is delivery risk right now, and who should look at it first?"

## 4. V1 Scope
### In scope
- Actionable Summary panel with clear recommendations.
- Hotspot ranking by path/module.
- Ownership concentration (bus-factor proxy) for hotspots.
- Period-over-period deltas for key metrics.
- Robust empty/degraded/error states.

### Out of scope
- Cross-repo or org-wide analytics.
- Individual performance scoring.
- Forecasting/predictive modeling.
- External tool joins (Jira, PR systems, incidents).

## 5. Product Requirements

### 5.1 Actionable Summary (top of Analytics tab)
Display 3 summary signals, each with status `ok|watch|risk` and one-line recommendation:
1. Rework trend delta (%): compares current window vs previous window.
2. Large change share delta (%): share of commits in buckets L+XL.
3. Ownership concentration delta: average top-author share across top hotspots.

Each signal includes:
- Current value
- Delta vs prior period
- Status badge
- Recommendation text

Recommendation templates:
- Rework rising: "Rework is up {delta}%. Investigate top hotspot paths before next merge batch."
- Large changes rising: "Large/XL changes are up {delta}%. Encourage smaller PR slices in hotspot paths."
- Ownership concentration high: "Ownership is concentrated in {path}. Add a secondary reviewer this week."

### 5.2 Hotspot Ranking
Table/list sorted by risk score descending.

Per row fields:
- `path` (module path or directory)
- `churnCount` (touches in selected window)
- `reworkRate` (% files retouched within churn window)
- `largeChangeShare` (% commits touching path with size bucket L or XL)
- `topAuthorShare` (% of touches by top contributor)
- `riskScore` (0-100)
- `recommendation` (short, row-specific)

### 5.3 Ownership Concentration
For each hotspot, compute bus-factor proxy:
- `topAuthorShare = topAuthorTouches / totalTouches * 100`
- `secondaryAuthorShare` optional (for tooltip/details)

Surface:
- In hotspot rows.
- Aggregated in summary as mean topAuthorShare of top 5 hotspots.

### 5.4 Period-over-Period Deltas
For selected window W (preset or custom):
- Current window: `[start, end]`
- Previous window: same duration immediately preceding start.

Compute deltas for:
- Rework rate
- Large change share
- Avg change size
- Merge percent
- Ownership concentration aggregate

Delta display:
- Absolute delta in percentage points where relevant.
- Positive/negative coloring and directional icon.

## 6. Data + Metric Definitions

### 6.1 Path/Module Unit
V1 path unit = directory at depth 2 from repository root (fallback to file path if shallower).
Examples:
- `web/analyticsView.js` -> `web/`
- `internal/server/handlers.go` -> `internal/server/`

### 6.2 Hotspot Candidate Set
Use commits in selected range only.
For each commit diff file path:
- Map path to module key.
- Increment module touch counters.

Top hotspots = top 15 modules by touch count, then risk score tie-break.

### 6.3 Risk Score (0-100)
For each module:
- `churnNorm` = min(1, churnCount / P90(churnCount))
- `reworkNorm` = reworkRate / 100
- `largeNorm` = largeChangeShare / 100
- `ownerNorm` = topAuthorShare / 100

Risk score formula:
- `risk = round(100 * (0.35*reworkNorm + 0.30*churnNorm + 0.20*ownerNorm + 0.15*largeNorm))`

Status thresholds:
- `ok`: < 40
- `watch`: 40-69
- `risk`: >= 70

### 6.4 Rework Definition
Reuse existing churn-window concept (21 days) at file level.
For module-level rework, count files in module with prior modification within 21 days.

### 6.5 Large Change Share
Large change commit = size bucket `L` or `XL`.
For each module:
- numerator: touches from commits bucketed L/XL
- denominator: all touches for module

## 7. API Contract Changes

## 7.1 Endpoint
`GET /api/analytics` (existing) remains source of truth.

Add new response sections:
- `summarySignals`
- `hotspots`
- `deltas`

### 7.2 Response Additions (JSON)
```json
{
  "summarySignals": [
    {
      "id": "reworkTrend",
      "label": "Rework Trend",
      "current": 18.2,
      "previous": 12.4,
      "delta": 5.8,
      "status": "watch",
      "recommendation": "Rework is up 5.8%. Investigate top hotspot paths before next merge batch."
    }
  ],
  "hotspots": [
    {
      "path": "internal/server/",
      "churnCount": 42,
      "reworkRate": 24.0,
      "largeChangeShare": 31.0,
      "topAuthor": "alice@example.com",
      "topAuthorShare": 57.0,
      "riskScore": 76,
      "status": "risk",
      "recommendation": "High churn + concentrated ownership. Add secondary reviewer."
    }
  ],
  "deltas": {
    "reworkRate": {"current": 18.2, "previous": 12.4, "delta": 5.8},
    "largeChangeShare": {"current": 27.0, "previous": 20.0, "delta": 7.0},
    "avgChangeSize": {"current": 16.3, "previous": 12.1, "delta": 4.2},
    "mergePercent": {"current": 14.0, "previous": 16.5, "delta": -2.5},
    "ownershipConcentration": {"current": 54.0, "previous": 49.0, "delta": 5.0}
  }
}
```

## 7.3 Compatibility
- Existing fields remain unchanged.
- New fields optional for graceful rollout.
- Frontend must tolerate absent new fields.

## 8. Frontend UX/IA

## 8.1 Block Order (top to bottom)
1. Existing period selector.
2. New Actionable Summary panel (3 cards).
3. New Hotspots section (table/list).
4. Existing velocity/authors/heatmap/merge/change-size/rework sections.

## 8.2 Empty + Degraded + Error States
- Empty repo: show one CTA "Analyze after first push" with brief explanation.
- Partial diff coverage: keep hotspots visible with badge "partial coverage" + analyzed count.
- Fetch failure: preserve existing sections if cached; show inline retry control.

## 8.3 Interaction Rules
- Changing period refreshes summary + hotspots + deltas atomically.
- Clicking hotspot row filters/anchors existing charts where feasible (non-blocking enhancement; if not implemented, leave as no-op with clear affordance disabled).

## 9. Performance + Constraints
- Keep existing diff cap behavior (max analyzed commits bounded).
- Compute hotspot aggregation in backend once per query; avoid recomputing in browser.
- Keep response payload reasonably bounded:
  - hotspots: max 15 rows
  - summarySignals: fixed 3

## 10. Testing Requirements

### Backend
- Unit tests for risk score calculation and status thresholds.
- Unit tests for period-over-period window construction.
- Unit tests for module path normalization.
- Handler tests validating new response keys present on success.

### Frontend
- Rendering tests (or deterministic DOM assertions) for:
  - summary signal cards
  - hotspot rows
  - delta labels/colors
- Error/degraded state assertions.

## 11. Milestones
1. Backend metrics and API extensions.
2. Frontend summary + hotspot UI and wiring.
3. Integration, degraded states, and tests.
4. Polish (copy, status semantics, visual hierarchy).

## 12. Acceptance Criteria
- User can identify top 3 risk hotspots within 60 seconds of opening Analytics.
- Summary shows clear recommendation text for each signal.
- Deltas are visible and correct for all listed key metrics.
- Partial coverage and failures are explicit, not silent.
- Existing analytics charts remain functional and non-regressed.

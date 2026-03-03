package server

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rybkr/gitvista/internal/gitcore"
)

const (
	analyticsRollingWindow      = 4
	analyticsTopAuthors         = 10
	analyticsChurnWindowDays    = 21
	analyticsDiffCommitMaxLimit = 4000
)

var analyticsSizeBuckets = []struct {
	label string
	max   int
}{
	{label: "XS", max: 5},
	{label: "S", max: 20},
	{label: "M", max: 50},
	{label: "L", max: 100},
	{label: "XL", max: int(^uint(0) >> 1)},
}

type analyticsResponse struct {
	Period       string                `json:"period"`
	Start        string                `json:"start,omitempty"`
	End          string                `json:"end,omitempty"`
	Velocity     analyticsVelocity     `json:"velocity"`
	Authors      analyticsAuthors      `json:"authors"`
	Heatmap      analyticsHeatmap      `json:"heatmap"`
	Merges       analyticsMerges       `json:"merges"`
	ChangeSize   analyticsChangeSize   `json:"changeSize"`
	Rework       analyticsRework       `json:"rework"`
	Summary      []analyticsSummary    `json:"summarySignals"`
	Hotspots     []analyticsHotspot    `json:"hotspots"`
	Deltas       analyticsDeltas       `json:"deltas"`
	DiffCoverage analyticsDiffCoverage `json:"diffCoverage"`
	GeneratedAt  string                `json:"generatedAt"`
}

type analyticsSummary struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	Current        float64 `json:"current"`
	Previous       float64 `json:"previous"`
	Delta          float64 `json:"delta"`
	Status         string  `json:"status"`
	Recommendation string  `json:"recommendation"`
}

type analyticsDeltaMetric struct {
	Current  float64 `json:"current"`
	Previous float64 `json:"previous"`
	Delta    float64 `json:"delta"`
}

type analyticsDeltas struct {
	ReworkRate             analyticsDeltaMetric `json:"reworkRate"`
	LargeChangeShare       analyticsDeltaMetric `json:"largeChangeShare"`
	AvgChangeSize          analyticsDeltaMetric `json:"avgChangeSize"`
	MergePercent           analyticsDeltaMetric `json:"mergePercent"`
	OwnershipConcentration analyticsDeltaMetric `json:"ownershipConcentration"`
}

type analyticsVelocity struct {
	Weeks        []analyticsWeekCount `json:"weeks"`
	TotalCommits int                  `json:"totalCommits"`
	AvgPerWeek   float64              `json:"avgPerWeek"`
	BestWeek     *analyticsBestWeek   `json:"bestWeek,omitempty"`
}

type analyticsWeekCount struct {
	TS    int64   `json:"ts"`
	Count int     `json:"count"`
	Avg   float64 `json:"avg"`
}

type analyticsBestWeek struct {
	TS    int64 `json:"ts"`
	Count int   `json:"count"`
}

type analyticsAuthors struct {
	Authors       []analyticsAuthor `json:"authors"`
	TotalInPeriod int               `json:"totalInPeriod"`
}

type analyticsAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Count int    `json:"count"`
}

type analyticsHeatmap struct {
	Grid [7][24]int `json:"grid"`
	Max  int        `json:"max"`
}

type analyticsMerges struct {
	MergeCount    int     `json:"mergeCount"`
	TotalCount    int     `json:"totalCount"`
	MergePercent  float64 `json:"mergePercent"`
	MergesPerWeek float64 `json:"mergesPerWeek"`
}

type analyticsBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type analyticsChangeSize struct {
	Buckets []analyticsBucket `json:"buckets"`
	Median  int               `json:"median"`
	AvgSize float64           `json:"avgSize"`
}

type analyticsReworkWeek struct {
	TS   int64   `json:"ts"`
	Rate float64 `json:"rate"`
}

type analyticsRework struct {
	Weeks   []analyticsReworkWeek `json:"weeks"`
	AvgRate float64               `json:"avgRate"`
}

type analyticsDiffCoverage struct {
	EligibleCommits int  `json:"eligibleCommits"`
	AnalyzedCommits int  `json:"analyzedCommits"`
	Partial         bool `json:"partial"`
	TooLargeErrors  int  `json:"tooLargeErrors"`
	OtherErrors     int  `json:"otherErrors"`
}

type analyticsCommitEntry struct {
	Hash    gitcore.Hash
	TS      time.Time
	Parents int
	Author  gitcore.Signature
}

type analyticsDiffEntry struct {
	TS    int64
	Files []string
}

type analyticsDiffInsights struct {
	LargeChangeShare       float64
	OwnershipConcentration float64
	Hotspots               []analyticsHotspot
}

type analyticsHotspot struct {
	Path             string  `json:"path"`
	ChurnCount       int     `json:"churnCount"`
	ReworkRate       float64 `json:"reworkRate"`
	LargeChangeShare float64 `json:"largeChangeShare"`
	TopAuthor        string  `json:"topAuthor"`
	TopAuthorShare   float64 `json:"topAuthorShare"`
	RiskScore        int     `json:"riskScore"`
	Status           string  `json:"status"`
	Recommendation   string  `json:"recommendation"`
}

type analyticsQuery struct {
	period   string
	cacheKey string
	hasRange bool
	start    time.Time
	end      time.Time // inclusive
}

func buildAnalytics(repo *gitcore.Repository, q analyticsQuery) (*analyticsResponse, error) {
	months, canonical, err := parseAnalyticsPeriod(q.period)
	if err != nil {
		return nil, err
	}

	commitsMap := repo.Commits()
	entries := make([]analyticsCommitEntry, 0, len(commitsMap))
	for h, c := range commitsMap {
		if c == nil {
			continue
		}
		entries = append(entries, analyticsCommitEntry{
			Hash:    h,
			TS:      c.Author.When,
			Parents: len(c.Parents),
			Author:  c.Author,
		})
	}
	if len(entries) == 0 {
		resp := &analyticsResponse{
			Period:      canonical,
			Summary:     []analyticsSummary{},
			Hotspots:    []analyticsHotspot{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if q.hasRange {
			resp.Start = q.start.Format(time.RFC3339)
			resp.End = q.end.Format(time.RFC3339)
		}
		return resp, nil
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].TS.Equal(entries[j].TS) {
			return strings.Compare(string(entries[i].Hash), string(entries[j].Hash)) < 0
		}
		return entries[i].TS.Before(entries[j].TS)
	})

	now := time.Now().UTC()
	windowStart, windowEnd := analyticsCurrentWindow(q, months, entries, now)
	filtered := filterEntriesForWindow(entries, windowStart, windowEnd)
	if len(filtered) == 0 {
		resp := &analyticsResponse{
			Period:      canonical,
			Summary:     []analyticsSummary{},
			Hotspots:    []analyticsHotspot{},
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if q.hasRange {
			resp.Start = q.start.Format(time.RFC3339)
			resp.End = q.end.Format(time.RFC3339)
		}
		return resp, nil
	}

	velocity := computeVelocity(filtered)
	authors := computeAuthors(filtered)
	heatmap := computeHeatmap(filtered)
	merges := computeMerges(filtered)
	changeSize, rework, coverage, insights := computeDiffAnalytics(repo, commitsMap, filtered)
	prevStart, prevEnd := analyticsPreviousWindow(windowStart, windowEnd)
	previous := filterEntriesForWindow(entries, prevStart, prevEnd)
	prevMerges := analyticsMerges{}
	prevChangeSize := analyticsChangeSize{}
	prevRework := analyticsRework{}
	prevInsights := analyticsDiffInsights{}
	if len(previous) > 0 {
		prevMerges = computeMerges(previous)
		prevChangeSize, prevRework, _, prevInsights = computeDiffAnalytics(repo, commitsMap, previous)
	}
	deltas := buildAnalyticsDeltas(
		rework.AvgRate, prevRework.AvgRate,
		insights.LargeChangeShare, prevInsights.LargeChangeShare,
		changeSize.AvgSize, prevChangeSize.AvgSize,
		merges.MergePercent, prevMerges.MergePercent,
		insights.OwnershipConcentration, prevInsights.OwnershipConcentration,
	)
	summary := buildAnalyticsSummary(deltas)

	resp := &analyticsResponse{
		Period:       canonical,
		Velocity:     velocity,
		Authors:      authors,
		Heatmap:      heatmap,
		Merges:       merges,
		ChangeSize:   changeSize,
		Rework:       rework,
		Summary:      summary,
		Hotspots:     insights.Hotspots,
		Deltas:       deltas,
		DiffCoverage: coverage,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
	}
	if q.hasRange {
		resp.Start = q.start.Format(time.RFC3339)
		resp.End = q.end.Format(time.RFC3339)
	}
	return resp, nil
}

func analyticsCacheKey(repo *gitcore.Repository, queryPart string) string {
	return "analytics:v1:" + queryPart + ":head:" + string(repo.Head()) + ":count:" + strconv.Itoa(repo.CommitCount())
}

func parseAnalyticsPeriod(raw string) (months int, canonical string, err error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "all":
		return 0, "all", nil
	case "3m":
		return 3, "3m", nil
	case "6m":
		return 6, "6m", nil
	case "1y", "12m":
		return 12, "1y", nil
	default:
		return 0, "", fmt.Errorf("invalid period: %q", raw)
	}
}

func parseAnalyticsQuery(periodRaw string, startRaw string, endRaw string) (analyticsQuery, error) {
	periodRaw = strings.TrimSpace(periodRaw)
	startRaw = strings.TrimSpace(startRaw)
	endRaw = strings.TrimSpace(endRaw)

	if startRaw == "" && endRaw == "" {
		_, canonical, err := parseAnalyticsPeriod(periodRaw)
		if err != nil {
			return analyticsQuery{}, err
		}
		return analyticsQuery{
			period:   canonical,
			cacheKey: canonical,
		}, nil
	}
	if startRaw == "" || endRaw == "" {
		return analyticsQuery{}, fmt.Errorf("both start and end are required")
	}

	start, err := parseAnalyticsDate(startRaw, false)
	if err != nil {
		return analyticsQuery{}, err
	}
	end, err := parseAnalyticsDate(endRaw, true)
	if err != nil {
		return analyticsQuery{}, err
	}
	if end.Before(start) {
		return analyticsQuery{}, fmt.Errorf("end before start")
	}

	return analyticsQuery{
		period:   "all",
		cacheKey: "range:" + start.Format("20060102") + "-" + end.Format("20060102"),
		hasRange: true,
		start:    start,
		end:      end,
	}, nil
}

func parseAnalyticsDate(raw string, endOfDay bool) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("empty date")
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC(), nil
	}
	if d, err := time.Parse("2006-01-02", raw); err == nil {
		if endOfDay {
			return time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), time.UTC), nil
		}
		return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	return time.Time{}, fmt.Errorf("invalid date format: %q", raw)
}

func weekStartUTC(ts time.Time) int64 {
	d := ts.UTC()
	weekday := int(d.Weekday())
	// Go: Sunday=0. Convert to Monday-based start.
	diff := 1 - weekday
	if weekday == 0 {
		diff = -6
	}
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, diff)
	return d.UnixMilli()
}

func analyticsCurrentWindow(
	q analyticsQuery,
	months int,
	entries []analyticsCommitEntry,
	now time.Time,
) (time.Time, time.Time) {
	if q.hasRange {
		return q.start, q.end
	}
	if months > 0 {
		return now.AddDate(0, -months, 0), now
	}
	return entries[0].TS, now
}

func analyticsPreviousWindow(start time.Time, end time.Time) (time.Time, time.Time) {
	if end.Before(start) {
		return time.Time{}, time.Time{}
	}
	duration := end.Sub(start)
	if duration <= 0 {
		return time.Time{}, time.Time{}
	}
	prevEnd := start.Add(-time.Nanosecond)
	prevStart := prevEnd.Add(-duration)
	return prevStart, prevEnd
}

func filterEntriesForWindow(entries []analyticsCommitEntry, start time.Time, end time.Time) []analyticsCommitEntry {
	if start.IsZero() || end.IsZero() {
		return nil
	}
	filtered := make([]analyticsCommitEntry, 0, len(entries))
	for _, e := range entries {
		if e.TS.Before(start) || e.TS.After(end) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

func computeVelocity(entries []analyticsCommitEntry) analyticsVelocity {
	firstWeek := weekStartUTC(entries[0].TS)
	lastWeek := weekStartUTC(time.Now().UTC())
	if lastWeek < firstWeek {
		lastWeek = firstWeek
	}

	const weekMS = int64(7 * 24 * time.Hour / time.Millisecond)
	buckets := make(map[int64]int, int((lastWeek-firstWeek)/weekMS)+1)
	for w := firstWeek; w <= lastWeek; w += weekMS {
		buckets[w] = 0
	}
	for _, e := range entries {
		w := weekStartUTC(e.TS)
		buckets[w]++
	}

	weeks := make([]analyticsWeekCount, 0, len(buckets))
	for w, count := range buckets {
		weeks = append(weeks, analyticsWeekCount{TS: w, Count: count})
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].TS < weeks[j].TS })

	total := 0
	best := &analyticsBestWeek{}
	for i := range weeks {
		total += weeks[i].Count
		if i == 0 || weeks[i].Count > best.Count {
			best.TS = weeks[i].TS
			best.Count = weeks[i].Count
		}
		sum := 0
		n := 0
		start := i - analyticsRollingWindow + 1
		if start < 0 {
			start = 0
		}
		for j := start; j <= i; j++ {
			sum += weeks[j].Count
			n++
		}
		if n > 0 {
			weeks[i].Avg = float64(sum) / float64(n)
		}
	}

	avgPerWeek := 0.0
	if len(weeks) > 0 {
		avgPerWeek = float64(total) / float64(len(weeks))
	}
	return analyticsVelocity{
		Weeks:        weeks,
		TotalCommits: total,
		AvgPerWeek:   avgPerWeek,
		BestWeek:     best,
	}
}

func computeAuthors(entries []analyticsCommitEntry) analyticsAuthors {
	type agg struct {
		name  string
		email string
		count int
	}
	byEmail := make(map[string]*agg)
	for _, e := range entries {
		email := e.Author.Email
		if email == "" {
			email = "unknown"
		}
		name := e.Author.Name
		if name == "" {
			name = email
		}
		if cur, ok := byEmail[email]; ok {
			cur.count++
		} else {
			byEmail[email] = &agg{name: name, email: email, count: 1}
		}
	}

	authors := make([]analyticsAuthor, 0, len(byEmail))
	for _, a := range byEmail {
		authors = append(authors, analyticsAuthor{Name: a.name, Email: a.email, Count: a.count})
	}
	sort.Slice(authors, func(i, j int) bool {
		if authors[i].Count == authors[j].Count {
			return authors[i].Email < authors[j].Email
		}
		return authors[i].Count > authors[j].Count
	})
	if len(authors) > analyticsTopAuthors {
		authors = authors[:analyticsTopAuthors]
	}

	return analyticsAuthors{
		Authors:       authors,
		TotalInPeriod: len(entries),
	}
}

func computeHeatmap(entries []analyticsCommitEntry) analyticsHeatmap {
	var grid [7][24]int
	max := 0
	for _, e := range entries {
		d := e.TS.UTC()
		jsDay := int(d.Weekday()) // sunday=0
		day := jsDay - 1
		if jsDay == 0 {
			day = 6
		}
		hour := d.Hour()
		grid[day][hour]++
		if grid[day][hour] > max {
			max = grid[day][hour]
		}
	}
	return analyticsHeatmap{Grid: grid, Max: max}
}

func computeMerges(entries []analyticsCommitEntry) analyticsMerges {
	mergeCount := 0
	minTS := entries[0].TS.UnixMilli()
	maxTS := entries[0].TS.UnixMilli()
	for _, e := range entries {
		if e.Parents > 1 {
			mergeCount++
		}
		ts := e.TS.UnixMilli()
		if ts < minTS {
			minTS = ts
		}
		if ts > maxTS {
			maxTS = ts
		}
	}
	total := len(entries)
	mergePercent := 0.0
	if total > 0 {
		mergePercent = float64(mergeCount) * 100.0 / float64(total)
	}
	spanWeeks := float64(maxTS-minTS) / float64(7*24*time.Hour/time.Millisecond)
	if spanWeeks < 1 {
		spanWeeks = 1
	}
	return analyticsMerges{
		MergeCount:    mergeCount,
		TotalCount:    total,
		MergePercent:  mergePercent,
		MergesPerWeek: float64(mergeCount) / spanWeeks,
	}
}

func computeDiffAnalytics(
	repo *gitcore.Repository,
	commitsMap map[gitcore.Hash]*gitcore.Commit,
	filtered []analyticsCommitEntry,
) (analyticsChangeSize, analyticsRework, analyticsDiffCoverage, analyticsDiffInsights) {
	change := analyticsChangeSize{Buckets: make([]analyticsBucket, 0, len(analyticsSizeBuckets))}
	for _, b := range analyticsSizeBuckets {
		change.Buckets = append(change.Buckets, analyticsBucket{Label: b.label})
	}

	coverage := analyticsDiffCoverage{EligibleCommits: len(filtered)}
	insights := analyticsDiffInsights{Hotspots: []analyticsHotspot{}}
	if len(filtered) == 0 {
		return change, analyticsRework{}, coverage, insights
	}

	// Newest-first cap to avoid runaway costs on very large repos.
	desc := make([]analyticsCommitEntry, len(filtered))
	copy(desc, filtered)
	sort.Slice(desc, func(i, j int) bool { return desc[i].TS.After(desc[j].TS) })
	if len(desc) > analyticsDiffCommitMaxLimit {
		desc = desc[:analyticsDiffCommitMaxLimit]
		coverage.Partial = true
	}

	sizes := make([]int, 0, len(desc))
	reworkEntries := make([]analyticsDiffEntry, 0, len(desc))
	analyzed := make([]analyticsAnalyzedCommit, 0, len(desc))
	for _, e := range desc {
		c := commitsMap[e.Hash]
		if c == nil {
			continue
		}
		var parentTreeHash gitcore.Hash
		if len(c.Parents) > 0 {
			if p, ok := commitsMap[c.Parents[0]]; ok && p != nil {
				parentTreeHash = p.Tree
			}
		}

		entries, err := gitcore.TreeDiff(repo, parentTreeHash, c.Tree, "")
		if err != nil {
			if strings.Contains(err.Error(), "diff too large") {
				coverage.TooLargeErrors++
			} else {
				coverage.OtherErrors++
			}
			continue
		}

		coverage.AnalyzedCommits++
		files := make([]string, 0, len(entries))
		for _, de := range entries {
			files = append(files, de.Path)
		}
		reworkEntries = append(reworkEntries, analyticsDiffEntry{
			TS:    e.TS.UnixMilli(),
			Files: files,
		})
		size := len(entries)
		sizes = append(sizes, size)
		author := e.Author.Email
		if author == "" {
			author = "unknown"
		}
		analyzed = append(analyzed, analyticsAnalyzedCommit{
			TS:     e.TS.UnixMilli(),
			Author: author,
			Files:  files,
			Large:  size > 50, // L and XL buckets.
		})
	}

	for _, sz := range sizes {
		for i, b := range analyticsSizeBuckets {
			if sz <= b.max {
				change.Buckets[i].Count++
				break
			}
		}
	}
	if len(sizes) > 0 {
		sort.Ints(sizes)
		change.Median = sizes[len(sizes)/2]
		sum := 0
		for _, v := range sizes {
			sum += v
		}
		change.AvgSize = float64(sum) / float64(len(sizes))
	}

	rework := computeRework(reworkEntries)
	insights = computeDiffInsights(analyzed)
	return change, rework, coverage, insights
}

type analyticsAnalyzedCommit struct {
	TS     int64
	Author string
	Files  []string
	Large  bool
}

type analyticsHotspotAgg struct {
	path          string
	churnCount    int
	totalTouches  int
	reworkTouches int
	largeTouches  int
	authorTouches map[string]int
}

func computeDiffInsights(analyzed []analyticsAnalyzedCommit) analyticsDiffInsights {
	if len(analyzed) == 0 {
		return analyticsDiffInsights{Hotspots: []analyticsHotspot{}}
	}
	sort.Slice(analyzed, func(i, j int) bool { return analyzed[i].TS < analyzed[j].TS })
	moduleAgg := make(map[string]*analyticsHotspotAgg)
	lastTouchByFile := make(map[string]int64)
	windowMS := int64(analyticsChurnWindowDays * 24 * time.Hour / time.Millisecond)
	totalTouches := 0
	largeTouches := 0
	for _, c := range analyzed {
		for _, file := range c.Files {
			module := analyticsModuleKey(file)
			agg, ok := moduleAgg[module]
			if !ok {
				agg = &analyticsHotspotAgg{
					path:          module,
					authorTouches: make(map[string]int),
				}
				moduleAgg[module] = agg
			}
			totalTouches++
			agg.totalTouches++
			agg.churnCount++
			if c.Large {
				largeTouches++
				agg.largeTouches++
			}
			agg.authorTouches[c.Author]++
			if last, ok := lastTouchByFile[file]; ok && c.TS-last <= windowMS {
				agg.reworkTouches++
			}
			lastTouchByFile[file] = c.TS
		}
	}
	counts := make([]int, 0, len(moduleAgg))
	for _, agg := range moduleAgg {
		counts = append(counts, agg.churnCount)
	}
	p90 := analyticsPercentile90(counts)
	hotspots := make([]analyticsHotspot, 0, len(moduleAgg))
	for _, agg := range moduleAgg {
		if agg.totalTouches == 0 {
			continue
		}
		topAuthor, topAuthorTouches := analyticsTopAuthor(agg.authorTouches)
		reworkRate := float64(agg.reworkTouches) * 100.0 / float64(agg.totalTouches)
		largeShare := float64(agg.largeTouches) * 100.0 / float64(agg.totalTouches)
		topAuthorShare := float64(topAuthorTouches) * 100.0 / float64(agg.totalTouches)
		risk := analyticsRiskScore(agg.churnCount, p90, reworkRate, largeShare, topAuthorShare)
		status := analyticsRiskStatus(risk)
		hotspots = append(hotspots, analyticsHotspot{
			Path:             agg.path,
			ChurnCount:       agg.churnCount,
			ReworkRate:       reworkRate,
			LargeChangeShare: largeShare,
			TopAuthor:        topAuthor,
			TopAuthorShare:   topAuthorShare,
			RiskScore:        risk,
			Status:           status,
			Recommendation:   analyticsHotspotRecommendation(status, reworkRate, topAuthorShare, largeShare),
		})
	}
	sort.Slice(hotspots, func(i, j int) bool {
		if hotspots[i].RiskScore == hotspots[j].RiskScore {
			return hotspots[i].ChurnCount > hotspots[j].ChurnCount
		}
		return hotspots[i].RiskScore > hotspots[j].RiskScore
	})
	if len(hotspots) > 15 {
		hotspots = hotspots[:15]
	}
	ownership := 0.0
	topN := min(5, len(hotspots))
	for i := 0; i < topN; i++ {
		ownership += hotspots[i].TopAuthorShare
	}
	if topN > 0 {
		ownership /= float64(topN)
	}
	largeShare := 0.0
	if totalTouches > 0 {
		largeShare = float64(largeTouches) * 100.0 / float64(totalTouches)
	}
	return analyticsDiffInsights{
		LargeChangeShare:       largeShare,
		OwnershipConcentration: ownership,
		Hotspots:               hotspots,
	}
}

func analyticsModuleKey(filePath string) string {
	clean := strings.TrimPrefix(path.Clean(filePath), "./")
	if clean == "." || clean == "" {
		return filePath
	}
	dir := path.Dir(clean)
	if dir == "." {
		return clean
	}
	parts := strings.Split(dir, "/")
	if len(parts) == 1 {
		return parts[0] + "/"
	}
	return parts[0] + "/" + parts[1] + "/"
}

func analyticsPercentile90(values []int) int {
	if len(values) == 0 {
		return 1
	}
	cp := append([]int(nil), values...)
	sort.Ints(cp)
	idx := int(0.9*float64(len(cp)-1) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(cp) {
		idx = len(cp) - 1
	}
	if cp[idx] < 1 {
		return 1
	}
	return cp[idx]
}

func analyticsTopAuthor(authorTouches map[string]int) (string, int) {
	topAuthor := "unknown"
	topTouches := 0
	for author, touches := range authorTouches {
		if touches > topTouches || (touches == topTouches && author < topAuthor) {
			topAuthor = author
			topTouches = touches
		}
	}
	return topAuthor, topTouches
}

func analyticsRiskScore(churnCount int, p90 int, reworkRate float64, largeShare float64, topAuthorShare float64) int {
	churnNorm := 1.0
	if p90 > 0 {
		churnNorm = float64(churnCount) / float64(p90)
	}
	if churnNorm > 1 {
		churnNorm = 1
	}
	reworkNorm := reworkRate / 100.0
	largeNorm := largeShare / 100.0
	ownerNorm := topAuthorShare / 100.0
	risk := int((100 * (0.35*reworkNorm + 0.30*churnNorm + 0.20*ownerNorm + 0.15*largeNorm)) + 0.5)
	if risk < 0 {
		return 0
	}
	if risk > 100 {
		return 100
	}
	return risk
}

func analyticsRiskStatus(score int) string {
	switch {
	case score >= 70:
		return "risk"
	case score >= 40:
		return "watch"
	default:
		return "ok"
	}
}

func analyticsHotspotRecommendation(status string, reworkRate float64, topAuthorShare float64, largeShare float64) string {
	if status == "risk" && topAuthorShare >= 55 {
		return "High churn with concentrated ownership. Add a secondary reviewer."
	}
	if status == "risk" && reworkRate >= 20 {
		return "Rework is elevated. Stabilize this path before bundling more changes."
	}
	if largeShare >= 30 {
		return "Large changes dominate here. Prefer smaller PR slices."
	}
	if status == "watch" {
		return "Monitor this hotspot for churn and ownership concentration."
	}
	return "No immediate action required."
}

func buildAnalyticsDeltas(
	reworkCurrent float64, reworkPrevious float64,
	largeCurrent float64, largePrevious float64,
	avgSizeCurrent float64, avgSizePrevious float64,
	mergeCurrent float64, mergePrevious float64,
	ownerCurrent float64, ownerPrevious float64,
) analyticsDeltas {
	return analyticsDeltas{
		ReworkRate: analyticsDeltaMetric{
			Current:  reworkCurrent,
			Previous: reworkPrevious,
			Delta:    reworkCurrent - reworkPrevious,
		},
		LargeChangeShare: analyticsDeltaMetric{
			Current:  largeCurrent,
			Previous: largePrevious,
			Delta:    largeCurrent - largePrevious,
		},
		AvgChangeSize: analyticsDeltaMetric{
			Current:  avgSizeCurrent,
			Previous: avgSizePrevious,
			Delta:    avgSizeCurrent - avgSizePrevious,
		},
		MergePercent: analyticsDeltaMetric{
			Current:  mergeCurrent,
			Previous: mergePrevious,
			Delta:    mergeCurrent - mergePrevious,
		},
		OwnershipConcentration: analyticsDeltaMetric{
			Current:  ownerCurrent,
			Previous: ownerPrevious,
			Delta:    ownerCurrent - ownerPrevious,
		},
	}
}

func buildAnalyticsSummary(d analyticsDeltas) []analyticsSummary {
	rework := analyticsSummary{
		ID:       "reworkTrend",
		Label:    "Rework Trend",
		Current:  d.ReworkRate.Current,
		Previous: d.ReworkRate.Previous,
		Delta:    d.ReworkRate.Delta,
	}
	if rework.Current >= 25 || rework.Delta >= 8 {
		rework.Status = "risk"
	} else if rework.Current >= 15 || rework.Delta >= 3 {
		rework.Status = "watch"
	} else {
		rework.Status = "ok"
	}
	rework.Recommendation = fmt.Sprintf("Rework is %s %.1f%%. Investigate top hotspot paths before next merge batch.", analyticsTrendWord(rework.Delta), absFloat(rework.Delta))

	large := analyticsSummary{
		ID:       "largeChangeShare",
		Label:    "Large Change Share",
		Current:  d.LargeChangeShare.Current,
		Previous: d.LargeChangeShare.Previous,
		Delta:    d.LargeChangeShare.Delta,
	}
	if large.Current >= 35 || large.Delta >= 10 {
		large.Status = "risk"
	} else if large.Current >= 20 || large.Delta >= 5 {
		large.Status = "watch"
	} else {
		large.Status = "ok"
	}
	large.Recommendation = fmt.Sprintf("Large/XL change share is %s %.1f%%. Encourage smaller PR slices in hotspot paths.", analyticsTrendWord(large.Delta), absFloat(large.Delta))

	owner := analyticsSummary{
		ID:       "ownershipConcentration",
		Label:    "Ownership Concentration",
		Current:  d.OwnershipConcentration.Current,
		Previous: d.OwnershipConcentration.Previous,
		Delta:    d.OwnershipConcentration.Delta,
	}
	if owner.Current >= 65 || owner.Delta >= 10 {
		owner.Status = "risk"
	} else if owner.Current >= 50 || owner.Delta >= 5 {
		owner.Status = "watch"
	} else {
		owner.Status = "ok"
	}
	owner.Recommendation = "Ownership is concentrated in hotspot paths. Add a secondary reviewer this week."

	return []analyticsSummary{rework, large, owner}
}

func analyticsTrendWord(delta float64) string {
	if delta >= 0 {
		return "up"
	}
	return "down"
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func computeRework(entries []analyticsDiffEntry) analyticsRework {
	if len(entries) == 0 {
		return analyticsRework{}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].TS < entries[j].TS })

	const windowMS = int64(analyticsChurnWindowDays * 24 * time.Hour / time.Millisecond)
	weekMap := make(map[int64][]analyticsDiffEntry)
	for _, e := range entries {
		w := weekStartUTC(time.UnixMilli(e.TS))
		weekMap[w] = append(weekMap[w], e)
	}

	weeks := make([]analyticsReworkWeek, 0, len(weekMap))
	for ws, group := range weekMap {
		totalFiles := 0
		reworked := 0
		for _, current := range group {
			for _, file := range current.Files {
				totalFiles++
				for _, prior := range entries {
					if prior.TS >= current.TS {
						break
					}
					if current.TS-prior.TS > windowMS {
						continue
					}
					if containsString(prior.Files, file) {
						reworked++
						break
					}
				}
			}
		}
		rate := 0.0
		if totalFiles > 0 {
			rate = float64(reworked) * 100.0 / float64(totalFiles)
		}
		weeks = append(weeks, analyticsReworkWeek{TS: ws, Rate: rate})
	}

	sort.Slice(weeks, func(i, j int) bool { return weeks[i].TS < weeks[j].TS })
	avg := 0.0
	for _, w := range weeks {
		avg += w.Rate
	}
	if len(weeks) > 0 {
		avg /= float64(len(weeks))
	}
	return analyticsRework{Weeks: weeks, AvgRate: avg}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

package server

import (
	"fmt"
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
	DiffCoverage analyticsDiffCoverage `json:"diffCoverage"`
	GeneratedAt  string                `json:"generatedAt"`
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

	filtered := make([]analyticsCommitEntry, 0, len(entries))
	for _, e := range entries {
		if q.hasRange {
			if e.TS.Before(q.start) || e.TS.After(q.end) {
				continue
			}
		} else if months > 0 {
			now := time.Now().UTC()
			cutoff := now.AddDate(0, -months, 0)
			if e.TS.Before(cutoff) {
				continue
			}
		}
		filtered = append(filtered, e)
	}
	if len(filtered) == 0 {
		resp := &analyticsResponse{
			Period:      canonical,
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
	changeSize, rework, coverage := computeDiffAnalytics(repo, commitsMap, filtered)

	resp := &analyticsResponse{
		Period:       canonical,
		Velocity:     velocity,
		Authors:      authors,
		Heatmap:      heatmap,
		Merges:       merges,
		ChangeSize:   changeSize,
		Rework:       rework,
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
) (analyticsChangeSize, analyticsRework, analyticsDiffCoverage) {
	change := analyticsChangeSize{Buckets: make([]analyticsBucket, 0, len(analyticsSizeBuckets))}
	for _, b := range analyticsSizeBuckets {
		change.Buckets = append(change.Buckets, analyticsBucket{Label: b.label})
	}

	coverage := analyticsDiffCoverage{EligibleCommits: len(filtered)}
	if len(filtered) == 0 {
		return change, analyticsRework{}, coverage
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
		sizes = append(sizes, len(entries))
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
	return change, rework, coverage
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

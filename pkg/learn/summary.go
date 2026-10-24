package learn

import "sort"

// GenerateSummary generates a learning summary from the store.
// topN limits the number of top patterns returned.
func GenerateSummary(store *Store, topN int) (Summary, error) {
	entries, err := store.Read()
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{
		TotalEntries: len(entries),
		TypeCounts:   make(map[EntryType]int),
	}

	// Count types and collect patterns
	pkgCounts := make(map[string]int)
	type patternInfo struct {
		pattern    string
		reuseCount int
	}
	var patterns []patternInfo

	for _, e := range entries {
		summary.TypeCounts[e.Type]++
		if e.Pattern != "" {
			patterns = append(patterns, patternInfo{e.Pattern, e.ReuseCount})
		}
		for _, pkg := range e.Packages {
			pkgCounts[pkg]++
		}
	}

	// Sort patterns by reuse count descending
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].reuseCount > patterns[j].reuseCount
	})
	if topN > len(patterns) {
		topN = len(patterns)
	}
	for i := 0; i < topN; i++ {
		summary.TopPatterns = append(summary.TopPatterns, PatternStat{
			Pattern:    patterns[i].pattern,
			ReuseCount: patterns[i].reuseCount,
		})
	}

	addEvidenceSummary(&summary, entries, topN)

	// Identify improvement areas (packages with 2+ entries)
	for pkg, count := range pkgCounts {
		if count >= 2 {
			summary.ImprovementAreas = append(summary.ImprovementAreas, pkg)
		}
	}

	return summary, nil
}

package taskroute

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

func cleanPath(value string) (string, error) {
	if value == "" || len(value) > 512 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.ContainsAny(value, "*?[]\\:\x00") || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("invalid relative path")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return "", fmt.Errorf("invalid path character")
		}
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", fmt.Errorf("parent traversal is not allowed")
		}
	}
	cleaned := path.Clean(value)
	if cleaned == "." {
		return "", fmt.Errorf("whole-project ownership is not allowed")
	}
	return cleaned, nil
}
func validate(f Facts) ([]string, error) {
	if f.Version != 1 || !slices.Contains([]string{"docs", "bugfix", "feature", "refactor", "investigate"}, f.Kind) {
		return nil, fmt.Errorf("invalid facts version or task kind")
	}
	if !slices.Contains([]string{"", "unknown", "low", "medium", "high", "critical"}, f.Risk) || !slices.Contains([]string{"", "auto", "inline", "guided", "planned"}, f.Requested) {
		return nil, fmt.Errorf("invalid risk or requested route")
	}
	if f.FailedAttempts < 0 || f.FailedAttempts > 10 || len(f.Paths) > 1000 || len(f.Workers) > 32 || f.EstimatedChangedLines != nil && (*f.EstimatedChangedLines < 0 || *f.EstimatedChangedLines > 10000000) {
		return nil, fmt.Errorf("facts exceed numeric bounds")
	}
	// Bound aggregate claims before any path processing or pairwise comparison.
	totalOwned := 0
	for _, worker := range f.Workers {
		totalOwned += len(worker.OwnedPaths)
		if totalOwned > 256 {
			return nil, fmt.Errorf("aggregate ownership exceeds 256 paths")
		}
	}
	paths := []string{}
	for _, p := range f.Paths {
		value, err := cleanPath(p)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(paths, value) {
			paths = append(paths, value)
		}
	}
	seen := map[string]bool{}
	for _, worker := range f.Workers {
		if worker.ID == "" || len(worker.ID) > 128 || seen[worker.ID] || len(worker.OwnedPaths) == 0 || len(worker.OwnedPaths) > 1000 {
			return nil, fmt.Errorf("invalid worker identity or ownership")
		}
		for _, r := range worker.ID {
			if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.') {
				return nil, fmt.Errorf("invalid worker ID")
			}
		}
		seen[worker.ID] = true
		for _, p := range worker.OwnedPaths {
			if _, err := cleanPath(p); err != nil {
				return nil, err
			}
		}
	}
	slices.Sort(paths)
	return paths, nil
}

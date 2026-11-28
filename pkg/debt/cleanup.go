package debt

import (
	"fmt"
	"sort"
	"strings"
)

// Largest orders items for cleanup: most lines involved first.
func Largest(items []Item, top int) []Item {
	sorted := append([]Item(nil), items...)
	sort.SliceStable(sorted, func(a, b int) bool { return sorted[a].Lines > sorted[b].Lines })
	if top > 0 && len(sorted) > top {
		sorted = sorted[:top]
	}
	return sorted
}

// CleanupBrief is the instruction for a cleanup session (a person, or a
// scheduled agent): the largest items first, what to do with each, and how
// to finish.
func CleanupBrief(items []Item, top int) string {
	if len(items) == 0 {
		return "No technical-debt items to clean up.\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Clean up technical debt, largest first (%d of %d items). Change behavior nowhere; tests must pass before and after.\n\n", min(top, len(items)), len(items))
	for i, it := range Largest(items, top) {
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, it, action(it.Kind))
	}
	b.WriteString("\nWhen done: run the tests, then `auto debt baseline` (it records the smaller set and refuses new items), " +
		"and open one pull request with the cleanup and the updated " + BaselinePath + ". A person merges it.\n")
	return b.String()
}

func action(k Kind) string {
	switch k {
	case KindDuplicate:
		return "Extract the repeated block into one shared function (or delete the copy no caller needs) and call it from every place."
	case KindNearLimit:
		return "Split the file along its responsibilities before the next change pushes it over the limit."
	case KindMarker:
		return "Do what the comment defers, or delete it if no longer true; if it must wait, open an issue and reference it in the comment."
	}
	return ""
}

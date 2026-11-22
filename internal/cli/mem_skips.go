package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// printMemSkips prints one line naming why sources were left out of the
// index, such as duplicate_learning_id after a merge, so the human output
// does not hide what only --json used to show. Nothing is printed when no
// source was skipped.
func printMemSkips(out io.Writer, counts map[string]int) {
	total := 0
	reasons := make([]string, 0, len(counts))
	for reason, n := range counts {
		if n > 0 {
			total += n
			reasons = append(reasons, fmt.Sprintf("%s=%d", reason, n))
		}
	}
	if total == 0 {
		return
	}
	sort.Strings(reasons)
	fmt.Fprintf(out, "skipped %d source(s): %s (details: --json)\n", total, strings.Join(reasons, ", "))
}

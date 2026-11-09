package mutate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DirRel is where reports land, one directory per run.
var DirRel = filepath.Join(".autopus", "qa", "mutate")

// WriteJSON saves the report under DirRel/<runID>/report.json.
func WriteJSON(projectDir, runID string, r Report) (string, error) {
	dir := filepath.Join(projectDir, DirRel, runID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "report.json")
	return path, os.WriteFile(path, append(data, '\n'), 0o644)
}

// WriteText prints the score and every survivor, which is the actionable
// part: each names a behaviour no case checks.
func WriteText(w io.Writer, r Report) {
	fmt.Fprintf(w, "mutants: %d tried of %d candidates; %d killed, %d survived, %d invalid\n",
		len(r.Outcomes), r.Total, r.Killed, r.Survived, r.Invalid)
	if r.Killed+r.Survived > 0 {
		fmt.Fprintf(w, "score: %.0f%% of judged mutants were caught by %d exec case(s)\n", r.Score*100, len(r.Cases))
	}
	if len(r.Excluded) > 0 {
		fmt.Fprintf(w, "excluded (fail on the unmodified product): %s\n", strings.Join(r.Excluded, ", "))
	}
	for _, o := range r.Outcomes {
		if o.Verdict != Survived {
			continue
		}
		fmt.Fprintf(w, "survived %s:%d %s\n  - %s\n  + %s\n", o.File, o.Line, o.Operator,
			strings.TrimSpace(o.Original), strings.TrimSpace(o.Mutated))
		if hint := Hint(o.Mutant); hint != "" {
			fmt.Fprintf(w, "  hint: %s\n", hint)
		}
	}
}

// Hint says what a survivor most likely means, so a reader can tell a missing
// case from a mutant that is the same program.
func Hint(m Mutant) string {
	lower := strings.ToLower(m.Original)
	switch {
	case strings.Contains(lower, "exit(") && m.Operator == "off-by-one":
		return "no case checks this exact exit code; add one if the code is part of the contract"
	case strings.HasPrefix(m.Operator, "boundary-"):
		return "no case puts the two sides equal here; add that input, unless they can never be equal (then this mutant is the same program)"
	case m.Operator == "off-by-one":
		return "no case depends on this exact number"
	case m.Operator == "flip-boolean":
		return "no case observes the behaviour this flag switches"
	}
	return "no case observes this condition changing"
}

package hunt

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Oracles that carry a premise of their own. A policy finding is only as good
// as the policy it cites, and an inefficiency finding only as good as the
// threshold it was measured against, so both must name that premise instead
// of leaving it in the agent's head.
const (
	OraclePolicyMismatch = "policy_mismatch"
	OracleInefficiency   = "inefficiency"
)

// PolicyRef is the written rule a policy_mismatch finding says the product
// breaks: where the rule is written and the rule's own words.
type PolicyRef struct {
	Source string `json:"source"` // file path with an optional :line, e.g. docs/billing.md:42
	Quote  string `json:"quote"`  // the rule, copied verbatim from Source
}

// Measure is the number an inefficiency finding is judged by. Basis says why
// the threshold is the right one (a stated budget, or the same work counted
// once), so a threshold picked to make the repro fail is visible as such.
type Measure struct {
	Metric    string `json:"metric"`    // what was counted or timed, e.g. "GET /api/items requests per page load"
	Threshold string `json:"threshold"` // the limit the repro enforces, e.g. "1"
	Observed  string `json:"observed"`  // what the run produced, e.g. "3"
	Basis     string `json:"basis"`     // where the threshold comes from
}

// validatePremise rejects a policy or inefficiency finding that does not
// state the rule or the threshold it is judged against.
func (f Finding) validatePremise() error {
	switch f.Oracle {
	case OraclePolicyMismatch:
		if f.Policy == nil || blank(f.Policy.Source) || blank(f.Policy.Quote) {
			return fmt.Errorf("oracle %s needs policy.source and policy.quote", OraclePolicyMismatch)
		}
	case OracleInefficiency:
		m := f.Measure
		if m == nil || blank(m.Metric) || blank(m.Threshold) || blank(m.Observed) || blank(m.Basis) {
			return fmt.Errorf("oracle %s needs measure.metric, threshold, observed and basis", OracleInefficiency)
		}
		if !hasDigit.MatchString(m.Threshold) || !hasDigit.MatchString(m.Observed) {
			return fmt.Errorf("measure.threshold and measure.observed must be numbers, got %q and %q", m.Threshold, m.Observed)
		}
	}
	return nil
}

var (
	hasDigit   = regexp.MustCompile(`[0-9]`)
	lineSuffix = regexp.MustCompile(`:[0-9]+(-[0-9]+)?$`)
	spaces     = regexp.MustCompile(`\s+`)
)

// checkQuote confirms a policy_mismatch quote is really written in its source
// file under root. Without it an agent could cite a rule that does not exist,
// and the fix would bend correct behavior toward it. Whitespace is collapsed
// on both sides, so a quote that spans a wrapped line still matches.
func (f Finding) checkQuote(root string) error {
	if f.Oracle != OraclePolicyMismatch || f.Policy == nil {
		return nil
	}
	rel := filepath.Clean(filepath.FromSlash(lineSuffix.ReplaceAllString(strings.TrimSpace(f.Policy.Source), "")))
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("policy.source %q must be a path inside the project", f.Policy.Source)
	}
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return fmt.Errorf("policy.source %q cannot be read: %w", f.Policy.Source, err)
	}
	norm := func(s string) string { return strings.TrimSpace(spaces.ReplaceAllString(s, " ")) }
	if !strings.Contains(norm(string(data)), norm(f.Policy.Quote)) {
		return fmt.Errorf("policy.quote is not written in %s; copy the rule verbatim", rel)
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }

// ObservationsFile holds inefficiencies and policy doubts that could not be
// measured or reproduced. They are notes for a human, never bugs.
const ObservationsFile = "observations.md"

// writePremise renders the cited policy or the measurement of one finding.
func writePremise(b *strings.Builder, f Finding) {
	if f.Policy != nil {
		fmt.Fprintf(b, "- Policy: %s — %q\n", f.Policy.Source, f.Policy.Quote)
	}
	if m := f.Measure; m != nil {
		fmt.Fprintf(b, "- Measure: %s — observed %s, threshold %s (%s)\n", m.Metric, m.Observed, m.Threshold, m.Basis)
	}
}

// writeObservations points at the observations file when it has content.
// It links instead of inlining: the file is agent-written prose, and the
// report should not present it with the authority of a verified finding.
func writeObservations(b *strings.Builder, root string) {
	data, err := os.ReadFile(filepath.Join(root, Dir, ObservationsFile))
	if err != nil || blank(string(data)) {
		return
	}
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") {
			n++
		}
	}
	fmt.Fprintf(b, "## Observations, not verified (%d)\n\n", n)
	fmt.Fprintf(b, "Unmeasured inefficiencies and policy doubts: `%s`. They are not bugs and are not fixed automatically.\n\n",
		filepath.ToSlash(filepath.Join(Dir, ObservationsFile)))
}

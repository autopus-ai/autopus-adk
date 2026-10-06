package harneval

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Result statuses of harness_eval_result.v1.
const (
	StatusPass          = "pass"
	StatusFail          = "fail"
	StatusNotApplicable = "not_applicable"
)

// Result is the harness_eval_result.v1 document. Notes and SentinelLog are
// human diagnostics for stderr; they are never part of the document, so the
// document stays free of temp paths and host detail.
type Result struct {
	SchemaVersion    string          `json:"schema_version"`
	Status           string          `json:"status"`
	FailureReasons   []string        `json:"failure_reasons"`
	Details          []string        `json:"details"`
	Totals           Totals          `json:"totals"`
	PassRate         *float64        `json:"pass_rate"`
	BaselinePassRate *float64        `json:"baseline_pass_rate"`
	RegressionDelta  float64         `json:"regression_delta"`
	Transitions      []Transition    `json:"transitions"`
	Categories       []CategoryTotal `json:"categories"`
	SetDigest        string          `json:"set_digest"`
	SurfaceDigest    string          `json:"surface_digest"`
	ProducedAt       string          `json:"produced_at"`

	Notes       []string `json:"-"`
	SentinelLog []string `json:"-"`
}

// Totals counts the active tasks a run declared and the surface tasks it ran.
type Totals struct {
	DeclaredSurface int `json:"declared_surface"`
	ExecutedSurface int `json:"executed_surface"`
	PassedSurface   int `json:"passed_surface"`
	DeclaredAgent   int `json:"declared_agent"`
}

// EncodeResult renders the document deterministically: fixed field order,
// every array present as [] rather than null, and a trailing newline.
func EncodeResult(result *Result) ([]byte, error) {
	doc := *result
	doc.FailureReasons = emptyIfNil(doc.FailureReasons)
	doc.Details = emptyIfNil(doc.Details)
	doc.Transitions = emptyIfNil(doc.Transitions)
	doc.Categories = emptyIfNil(doc.Categories)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func emptyIfNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}

// Emit writes the result to the `--format json` channels: the document to
// outputPath when one is set, the document alone to stdout, and every human
// line to stderr. The file is written first, so a failed write leaves stdout
// empty rather than reporting a result that was not saved.
func Emit(result *Result, outputPath string, stdout, stderr io.Writer) error {
	data, err := EncodeResult(result)
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	if outputPath != "" {
		if err := os.WriteFile(outputPath, data, 0o644); err != nil {
			return fmt.Errorf("write result: %w", err)
		}
	}
	if _, err := stdout.Write(data); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	for _, line := range guidance(result) {
		if _, err := fmt.Fprintln(stderr, "harness-eval: "+line); err != nil {
			return fmt.Errorf("write guidance: %w", err)
		}
	}
	return nil
}

// reasonHints names the next step for each failure reason.
var reasonHints = map[string]string{
	ReasonInvalid:           "fix the golden set or baseline file named above; nothing is evaluated until it loads",
	ReasonBaselineMissing:   "create the first baseline once with `auto eval harness baseline --init`",
	ReasonTemplatesStale:    "regenerate templates/ from content/ with `make generate-templates` and commit the result",
	ReasonHostProbeUnpinned: "generation reached a host CLI; pin that probe in the manifest pins (the sentinel log lists each call)",
	ReasonGenerationFailed:  "a platform adapter failed to generate its surface; see the error above",
	ReasonVacuous:           "the run proved nothing; restore the floors and let every declared active surface task execute",
	ReasonRegression: "after review, pin the results with `auto eval harness baseline --update`, naming each " +
		"regression with `--accept-regression <task-id> --reason <text>`",
	ReasonExpectationChanged: "an expectation changed; after review, pin it with `auto eval harness baseline --update`",
	ReasonSetDigestMismatch:  "the golden set changed; after review, pin it with `auto eval harness baseline --update`",
	ReasonTaskMissing:        "a task was deleted without a tombstone; keep it with status.state retired and a reason instead",
}

// guidance returns the stderr lines: diagnostics and the sentinel log first,
// then one hint per failure reason, then a note on unrecorded improvements.
func guidance(result *Result) []string {
	lines := append([]string(nil), result.Notes...)
	for _, call := range result.SentinelLog {
		lines = append(lines, "sentinel: "+call)
	}
	for _, reason := range result.FailureReasons {
		if hint, known := reasonHints[reason]; known {
			lines = append(lines, hint)
		}
	}
	improved := 0
	for _, transition := range result.Transitions {
		if transition.Kind == TransitionImproved {
			improved++
		}
	}
	if improved > 0 && len(result.FailureReasons) == 0 {
		lines = append(lines, fmt.Sprintf("%d task(s) improved; record them with `auto eval harness baseline --update`", improved))
	}
	return lines
}

// ExitCode is 0 for a pass or not_applicable result without failure reasons
// and 1 for anything else, so a malformed result never reads as a pass.
func ExitCode(result *Result) int {
	if result == nil || len(result.FailureReasons) > 0 {
		return 1
	}
	if result.Status == StatusPass || result.Status == StatusNotApplicable {
		return 0
	}
	return 1
}

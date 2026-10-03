// Package record turns recorded browser sessions into v2 recording scenarios
// (SPEC-QALOOP-001 REQ-14..16).
//
// A recording is what a person did in Playwright codegen or what an agent
// logged as qamesh.recording.v1 JSONL. Conversion never invents a step: a line
// with no faithful scenario equivalent is reported by line number instead of
// being approximated.
package record

import (
	"errors"
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// SchemaVersion is the agent recording log dialect ParseJSONL reads.
const SchemaVersion = "qamesh.recording.v1"

// Recorded actions. goto opens a screen; the others become v2 action steps.
const (
	ActionGoto   = "goto"
	ActionClick  = "click"
	ActionFill   = "fill"
	ActionPress  = "press"
	ActionCheck  = "check"
	ActionSelect = "select"
)

// Recorded expectations, each one v2 expect_* step.
const (
	ExpectText  = "text"
	ExpectTitle = "title"
	ExpectURL   = "url"
	ExpectRole  = "role"
)

// Recording formats Import reads.
const (
	FormatCodegen = "codegen"
	FormatJSONL   = "jsonl"
)

// Stable error codes. CodeCandidateExists is shared with discovery, which
// writes its candidates through WriteCandidate too.
const (
	CodeUnsupportedLines  = "qa_record_unsupported_lines"
	CodeOriginMismatch    = "qa_record_origin_mismatch"
	CodeOriginMissing     = "qa_record_origin_missing"
	CodeOriginInvalid     = "qa_record_origin_invalid"
	CodeJourneyMissing    = "qa_record_journey_missing"
	CodeGotoMissing       = "qa_record_goto_missing"
	CodeURLInvalid        = "qa_record_url_invalid"
	CodeFormatUnknown     = "qa_record_format_unknown"
	CodeSourceUnreadable  = "qa_record_source_unreadable"
	CodeEmpty             = "qa_record_empty"
	CodePlaywrightMissing = "qa_record_playwright_missing"
	CodeCodegenFailed     = "qa_record_codegen_failed"
	CodeCandidateExists   = "qa_scenario_candidate_exists"
)

// Event is one recorded action or expectation. Exactly one of Action and
// Expect is set. Value holds the fill text, the selected option, or the
// expected text or title; URL holds a goto or expected URL.
type Event struct {
	Line     int
	Action   string
	Expect   string
	Target   scenario.Target
	Value    string
	ValueEnv string
	Key      string
	URL      string
	By       string
	Ac       string
}

// Recording is the ordered list of events one session produced.
type Recording struct {
	Events []Event
}

// Unsupported is a source line with no faithful scenario equivalent.
type Unsupported struct {
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Reason string `json:"reason,omitempty"`
}

// Error is a failure with a stable code. SetupGap marks a tool missing from
// the machine, which is a setup problem rather than a bad recording.
type Error struct {
	Code     string
	Message  string
	SetupGap bool
}

func (e *Error) Error() string { return e.Message }

func failf(code, format string, args ...any) error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// CodeOf returns the stable code of err and whether it is a setup gap. A
// scenario validation failure keeps the validator's own code.
func CodeOf(err error) (string, bool) {
	var recordErr *Error
	if errors.As(err, &recordErr) {
		return recordErr.Code, recordErr.SetupGap
	}
	var invalid *scenario.ValidationError
	if errors.As(err, &invalid) {
		return invalid.Code, false
	}
	return "", false
}

func unsupportedError(lines []Unsupported) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d unsupported line(s); fix them or pass --allow-partial to drop them:", len(lines))
	for _, line := range lines {
		fmt.Fprintf(&b, "\n  line %d: %s", line.Line, line.Text)
		if line.Reason != "" {
			fmt.Fprintf(&b, " (%s)", line.Reason)
		}
	}
	return &Error{Code: CodeUnsupportedLines, Message: b.String()}
}

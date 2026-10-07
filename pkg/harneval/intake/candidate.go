package intake

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"unicode"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// Wire-contract schema identifiers of the intake records.
const (
	CandidateSchemaV1 = "harness_golden_candidate.v1"
	LinkSchemaV1      = "harness_incident_link.v1"
	RejectionSchemaV1 = "harness_candidate_rejection.v1"
)

// Candidate is the harness_golden_candidate.v1 document: one open candidate
// per fingerprint, edited only by a person, whose evidence comes from the
// representative (lowest-id) entry alone.
type Candidate struct {
	SchemaVersion      string        `json:"schema_version"`
	ID                 string        `json:"id"`
	FingerprintVersion int           `json:"fingerprint_version"`
	Fingerprint        string        `json:"fingerprint"`
	Representative     string        `json:"representative"`
	LearningRefs       []string      `json:"learning_refs"`
	Expected           string        `json:"expected"`
	Actual             string        `json:"actual"`
	Repro              string        `json:"repro"`
	Redacted           bool          `json:"redacted"`
	RedactedFields     []string      `json:"redacted_fields"`
	Task               harneval.Task `json:"task"`
}

// Link is the permanent harness_incident_link.v1 record a promotion leaves
// under candidates/promoted/, keeping every learning ref and the evidence.
type Link struct {
	SchemaVersion      string   `json:"schema_version"`
	TaskID             string   `json:"task_id"`
	CandidateID        string   `json:"candidate_id"`
	FingerprintVersion int      `json:"fingerprint_version"`
	Fingerprint        string   `json:"fingerprint"`
	Representative     string   `json:"representative"`
	LearningRefs       []string `json:"learning_refs"`
	Expected           string   `json:"expected"`
	Actual             string   `json:"actual"`
	Repro              string   `json:"repro"`
	Redacted           bool     `json:"redacted"`
	RedactedFields     []string `json:"redacted_fields"`
}

// Rejection is the harness_candidate_rejection.v1 record a reject leaves
// under candidates/rejected/, so the fingerprint is never offered again.
type Rejection struct {
	SchemaVersion      string   `json:"schema_version"`
	CandidateID        string   `json:"candidate_id"`
	FingerprintVersion int      `json:"fingerprint_version"`
	Fingerprint        string   `json:"fingerprint"`
	LearningRefs       []string `json:"learning_refs"`
	Reason             string   `json:"reason"`
}

// decodeStrict decodes exactly one JSON document into target, refusing
// unknown fields and any data after the document.
func decodeStrict(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&json.RawMessage{}); err != io.EOF {
		return errors.New("data follows the JSON document")
	}
	return nil
}

// encodeRecord is the canonical record encoding: json.MarshalIndent with a
// two-space indent in struct field order, then a final LF. Records carry no
// timestamp, so the same input always yields the same bytes.
func encodeRecord(record any) ([]byte, error) {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Byte caps of REQ-HC-01 and REQ-HC-05. A raw evidence value longer than
// rawCapFactor times its cap is refused before redaction runs.
const (
	evidenceCap  = 1024
	reproCap     = 512
	patternCap   = 4096
	rawCapFactor = 4
)

// Reasons and details of refused learning text.
const (
	ReasonLearningFieldInvalid = "learning_field_invalid"
	ReasonCandidateTextInvalid = "candidate_text_invalid"

	DetailControlChar           = "control_char"
	DetailRawOverLimit          = "raw_over_limit"
	DetailOverCapAfterRedaction = "over_cap_after_redaction"
)

// TextError reports learning text intake refuses to copy: an evidence field
// (learning_field_invalid) or the pattern (candidate_text_invalid).
type TextError struct {
	Reason string
	Field  string
	Detail string
}

func (e *TextError) Error() string { return e.Reason + ": " + e.Field + ": " + e.Detail }

// hasControl reports a C0, DEL, or C1 control character; allowLayout lets
// newline and tab through.
func hasControl(text string, allowLayout bool) bool {
	for _, r := range text {
		if unicode.IsControl(r) && !(allowLayout && (r == '\n' || r == '\t')) {
			return true
		}
	}
	return false
}

// redactField applies the fixed REQ-HC-01 order to one evidence value:
// refuse a control character, refuse a raw value over rawCapFactor times its
// cap, redact, then refuse a redacted value over its cap. The cap applies
// after redaction because a placeholder can be longer than the text it hides.
func redactField(redactor Redactor, field, value string, limit int) (string, *TextError) {
	fail := func(detail string) (string, *TextError) {
		return "", &TextError{Reason: ReasonLearningFieldInvalid, Field: field, Detail: detail}
	}
	switch {
	case hasControl(value, false):
		return fail(DetailControlChar)
	case len(value) > rawCapFactor*limit:
		return fail(DetailRawOverLimit)
	}
	masked := redactor.Redact(value)
	if len(masked) > limit {
		return fail(DetailOverCapAfterRedaction)
	}
	return masked, nil
}

// redactPattern applies REQ-HC-05 to a pattern, which may hold newlines and
// tabs, then redacts it; the redacted pattern must stay within the cap too.
func redactPattern(redactor Redactor, pattern string) (string, *TextError) {
	fail := func(detail string) (string, *TextError) {
		return "", &TextError{Reason: ReasonCandidateTextInvalid, Field: "pattern", Detail: detail}
	}
	switch {
	case hasControl(pattern, true):
		return fail(DetailControlChar)
	case len(pattern) > patternCap:
		return fail(DetailRawOverLimit)
	}
	masked := redactor.Redact(pattern)
	if len(masked) > patternCap {
		return fail(DetailOverCapAfterRedaction)
	}
	return masked, nil
}

// evidence is the checked and redacted text a candidate copies from its
// representative entry, with the names of the fields redaction changed.
type evidence struct {
	pattern, expected, actual, repro string
	redactedFields                   []string
}

// evidenceFor re-checks and re-redacts the stored text of e, whose expected
// and actual are replaced by the flag pair when one was given. Store values
// are re-checked because the store can be edited by hand.
func evidenceFor(redactor Redactor, e Entry, expected, actual string) (evidence, *TextError) {
	pattern, refused := redactPattern(redactor, e.Pattern)
	if refused != nil {
		return evidence{}, refused
	}
	ev := evidence{pattern: pattern, redactedFields: []string{}}
	if pattern != e.Pattern {
		ev.redactedFields = append(ev.redactedFields, "pattern")
	}
	fields := []struct {
		name, raw string
		limit     int
		masked    *string
	}{
		{"expected", expected, evidenceCap, &ev.expected},
		{"actual", actual, evidenceCap, &ev.actual},
		{"repro", e.Repro, reproCap, &ev.repro},
	}
	for _, field := range fields {
		if *field.masked, refused = redactField(redactor, field.name, field.raw, field.limit); refused != nil {
			return evidence{}, refused
		}
		if *field.masked != field.raw {
			ev.redactedFields = append(ev.redactedFields, field.name)
		}
	}
	sort.Strings(ev.redactedFields)
	return ev, nil
}

// newCandidate builds the candidate of a new fingerprint. The draft task has
// every harness_golden_task.v1 field; a person fills category and assertions
// before promotion.
func newCandidate(fingerprint string, representative Entry, ev evidence) Candidate {
	return Candidate{
		SchemaVersion:      CandidateSchemaV1,
		ID:                 candidateIDFor(fingerprint),
		FingerprintVersion: FingerprintVersion,
		Fingerprint:        fingerprint,
		Representative:     representative.ID,
		LearningRefs:       []string{representative.ID},
		Expected:           ev.expected,
		Actual:             ev.actual,
		Repro:              ev.repro,
		Redacted:           len(ev.redactedFields) > 0,
		RedactedFields:     ev.redactedFields,
		Task: harneval.Task{
			SchemaVersion: harneval.TaskSchemaV1,
			ID:            taskIDFor(fingerprint),
			Kind:          harneval.KindSurface,
			Intent:        ev.pattern,
			Outcome:       ev.expected,
			Variants:      []harneval.Variant{},
			Assertions:    []harneval.Assertion{},
			Provenance:    harneval.Provenance{Kind: "incident", Ref: representative.ID, Fingerprint: fingerprint},
			Status:        harneval.TaskStatus{State: harneval.StateActive},
		},
	}
}

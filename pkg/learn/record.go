package learn

import (
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/insajin/autopus-adk/pkg/secretscan"
)

// ReasonFieldInvalid is the reason code of every evidence field rejection.
const ReasonFieldInvalid = "learning_field_invalid"

// Details of a ReasonFieldInvalid rejection, one per failing validation step.
// A verbatim field is a severity outside the enum (DetailUnknownValue) or a
// files or packages item that redaction would change (DetailNeedsRedaction).
const (
	DetailControlChar           = "control_char"
	DetailRawOverLimit          = "raw_over_limit"
	DetailOverCapAfterRedaction = "over_cap_after_redaction"
	DetailUnknownValue          = "unknown_value"
	DetailNeedsRedaction        = "needs_redaction"
)

// rawLimitFactor bounds a raw value before redaction runs, so an oversized
// value is refused without scanning it.
const rawLimitFactor = 4

// EvidenceField names a capped incident evidence field.
type EvidenceField string

const (
	FieldExpected EvidenceField = "expected"
	FieldActual   EvidenceField = "actual"
	FieldRepro    EvidenceField = "repro"
)

// Fields the store writer keeps verbatim instead of redacting. They have no
// cap; a FieldError names them when the writer refuses their value.
const (
	FieldSeverity EvidenceField = "severity"
	FieldFiles    EvidenceField = "files"
	FieldPackages EvidenceField = "packages"
)

// Cap returns the byte cap of the redacted value. An unknown field has cap 0,
// which rejects every non-empty value.
func (f EvidenceField) Cap() int {
	switch f {
	case FieldExpected, FieldActual:
		return 1024
	case FieldRepro:
		return 512
	default:
		return 0
	}
}

// FieldError reports an evidence value rejected with ReasonFieldInvalid. The
// message names the field and the failing step, never the value.
type FieldError struct {
	Field  EvidenceField
	Detail string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("%s: %s: %s", ReasonFieldInvalid, e.Field, e.Detail)
}

// Reason returns ReasonFieldInvalid.
func (e *FieldError) Reason() string { return ReasonFieldInvalid }

// RedactEvidenceField validates and redacts one evidence value with
// CheckEvidence, its field's cap, and pkg/secretscan.Redact. It returns the
// value to persist and whether redaction changed it; an empty value passes
// unchanged.
func RedactEvidenceField(field EvidenceField, value string) (string, bool, error) {
	redacted, detail := CheckEvidence(value, field.Cap(), redactSecrets)
	if detail != "" {
		return "", false, &FieldError{Field: field, Detail: detail}
	}
	return redacted, redacted != value, nil
}

// CheckEvidence applies the fixed SPEC-HARNEVAL-002 REQ-HC-01 order to one
// capped free-text value, with redact as step (3): (1) reject a control
// character, (2) reject a raw value longer than four times limit, (3) redact,
// (4) reject a redacted value over limit. The cap applies after redaction
// because a placeholder can be longer than the text it replaces. It returns
// the value to persist, or "" and the detail of the step that rejected it; an
// empty value passes unchanged. pkg/harneval/intake checks stored values, flag
// values, and rejection reasons with it, so intake and the store writer
// refuse the same text.
func CheckEvidence(value string, limit int, redact func(string) string) (string, string) {
	switch {
	case value == "":
		return "", ""
	case HasControlChar(value, false):
		return "", DetailControlChar
	case len(value) > rawLimitFactor*limit:
		return "", DetailRawOverLimit
	}
	redacted := redact(value)
	if len(redacted) > limit {
		return "", DetailOverCapAfterRedaction
	}
	return redacted, ""
}

// HasControlChar reports a C0 control, DEL, or C1 control in s; layout lets
// newline and tab through, as a pattern may hold them. Invalid UTF-8 counts
// too: an undecodable byte in 0x80-0x9F is an 8-bit C1 control to a terminal,
// and JSON encoding would rewrite it as a longer U+FFFD.
func HasControlChar(s string, layout bool) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(layout && (r == '\n' || r == '\t')) {
			return true
		}
	}
	return false
}

// redactSecrets is pkg/secretscan.Redact without its changed flag.
func redactSecrets(s string) string {
	out, _ := secretscan.Redact(s)
	return out
}

// CheckRecordOpts runs the store writer's checks on opts without writing, so
// a caller can refuse invalid input before it touches the project.
func CheckRecordOpts(opts RecordOpts) error {
	_, err := redactEntry(newEntry("", "", opts))
	return err
}

func recordEntry(store *Store, entryType EntryType, opts RecordOpts) error {
	if store == nil {
		return fmt.Errorf("store is nil")
	}
	if opts.Pattern == "" {
		return fmt.Errorf("pattern is required")
	}
	return store.AppendAtomic(entryType, opts)
}

// RecordGateFail records a gate failure learning entry.
func RecordGateFail(store *Store, opts RecordOpts) error {
	return recordEntry(store, EntryTypeGateFail, opts)
}

// RecordCoverageGap records a coverage gap learning entry.
func RecordCoverageGap(store *Store, opts RecordOpts) error {
	return recordEntry(store, EntryTypeCoverageGap, opts)
}

// RecordReviewIssue records a review issue learning entry.
func RecordReviewIssue(store *Store, opts RecordOpts) error {
	return recordEntry(store, EntryTypeReviewIssue, opts)
}

// RecordExecutorError records an executor error learning entry.
func RecordExecutorError(store *Store, opts RecordOpts) error {
	return recordEntry(store, EntryTypeExecutorError, opts)
}

// RecordFixPattern records a fix pattern learning entry.
func RecordFixPattern(store *Store, opts RecordOpts) error {
	return recordEntry(store, EntryTypeFixPattern, opts)
}

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
const (
	DetailControlChar           = "control_char"
	DetailRawOverLimit          = "raw_over_limit"
	DetailOverCapAfterRedaction = "over_cap_after_redaction"
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

// RedactEvidenceField validates and redacts one evidence value in the fixed
// SPEC-HARNEVAL-002 REQ-HC-01 order: (1) reject a control character, (2)
// reject a raw value longer than four times the cap, (3) redact, (4) reject a
// redacted value over the cap. The cap applies after redaction because a
// placeholder can be longer than the text it replaces. It returns the value to
// persist and whether redaction changed it; an empty value passes unchanged.
func RedactEvidenceField(field EvidenceField, value string) (string, bool, error) {
	if value == "" {
		return "", false, nil
	}
	if hasControlChar(value) {
		return "", false, &FieldError{Field: field, Detail: DetailControlChar}
	}
	limit := field.Cap()
	if len(value) > rawLimitFactor*limit {
		return "", false, &FieldError{Field: field, Detail: DetailRawOverLimit}
	}
	redacted, changed := secretscan.Redact(value)
	if len(redacted) > limit {
		return "", false, &FieldError{Field: field, Detail: DetailOverCapAfterRedaction}
	}
	return redacted, changed, nil
}

// hasControlChar reports a C0 control (tab and newline included), DEL, or C1
// control. Invalid UTF-8 counts too: an undecodable byte in 0x80-0x9F is an
// 8-bit C1 control to a terminal, and JSON encoding would rewrite it anyway.
func hasControlChar(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
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

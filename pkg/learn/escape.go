package learn

import (
	"errors"
	"fmt"
)

// EntryTypeEscape is a defect that every existing check passed and a later
// real run exposed. It is the input of the harness checklist loop.
const EntryTypeEscape EntryType = "escape"

// Escape fields. MissedBy is redacted free text; the when lists are the diff
// conditions a future check applies under, kept verbatim like files.
const (
	FieldMissedBy    EvidenceField = "missed_by"
	FieldWhenPath    EvidenceField = "when_path"
	FieldWhenKeyword EvidenceField = "when_keyword"
	FieldCommit      EvidenceField = "commit"
	FieldProvider    EvidenceField = "provider"
)

const (
	escapeMissedByCap = 512
	escapeWhenCap     = 256
	escapeShortCap    = 64
)

// Escape is what an escape lesson adds to its entry.
type Escape struct {
	// MissedBy names the check that passed while the defect was present.
	MissedBy string `json:"missed_by"`
	// WhenPaths and WhenKeywords are the diff conditions of the lesson.
	WhenPaths    []string `json:"when_paths,omitempty"`
	WhenKeywords []string `json:"when_keywords,omitempty"`
	// Commit is HEAD when the lesson was recorded; distinct commits make a
	// repeat rather than the same incident written twice.
	Commit string `json:"commit,omitempty"`
	// Provider is the agent that wrote the lesson, so a refutation can ask a
	// different one.
	Provider string `json:"provider,omitempty"`
}

// ErrEscapeIncomplete reports an escape record without its required fields.
var ErrEscapeIncomplete = errors.New("an escape lesson needs --missed-by and at least one --when-path or --when-keyword")

// RecordEscape records an escape lesson.
func RecordEscape(store *Store, opts RecordOpts) error {
	if err := checkEscapeRequired(opts.Escape); err != nil {
		return err
	}
	return recordEntry(store, EntryTypeEscape, opts)
}

func checkEscapeRequired(e *Escape) error {
	if e == nil || e.MissedBy == "" || (len(e.WhenPaths) == 0 && len(e.WhenKeywords) == 0) {
		return ErrEscapeIncomplete
	}
	for _, list := range [][]string{e.WhenPaths, e.WhenKeywords} {
		for _, item := range list {
			if item == "" {
				return ErrEscapeIncomplete
			}
		}
	}
	return nil
}

// redactEscape validates and redacts the escape fields of entry in place.
func redactEscape(entry *LearningEntry) error {
	if entry.Escape == nil {
		return nil
	}
	e := *entry.Escape
	redacted, detail := CheckEvidence(e.MissedBy, escapeMissedByCap, redactSecrets)
	if detail != "" {
		return &FieldError{Field: FieldMissedBy, Detail: detail}
	}
	e.MissedBy = redacted
	lists := []struct {
		field EvidenceField
		items []string
	}{{FieldWhenPath, e.WhenPaths}, {FieldWhenKeyword, e.WhenKeywords}}
	for _, list := range lists {
		for _, item := range list.items {
			if err := checkVerbatimItem(list.field, item, escapeWhenCap); err != nil {
				return err
			}
		}
	}
	for field, value := range map[EvidenceField]string{FieldCommit: e.Commit, FieldProvider: e.Provider} {
		if err := checkVerbatimItem(field, value, escapeShortCap); err != nil {
			return err
		}
	}
	entry.Escape = &e
	return nil
}

func checkVerbatimItem(field EvidenceField, value string, limit int) error {
	switch {
	case HasControlChar(value, false):
		return &FieldError{Field: field, Detail: DetailControlChar}
	case len(value) > limit:
		return &FieldError{Field: field, Detail: DetailOverCapAfterRedaction}
	case redactSecrets(value) != value:
		return &FieldError{Field: field, Detail: DetailNeedsRedaction}
	}
	return nil
}

// String describes the escape for listings.
func (e Escape) String() string {
	return fmt.Sprintf("missed by %s (paths %v, keywords %v)", e.MissedBy, e.WhenPaths, e.WhenKeywords)
}

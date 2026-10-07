package learn

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// EntryType represents the category of a learning entry.
type EntryType string

const (
	EntryTypeGateFail      EntryType = "gate_fail"
	EntryTypeCoverageGap   EntryType = "coverage_gap"
	EntryTypeReviewIssue   EntryType = "review_issue"
	EntryTypeExecutorError EntryType = "executor_error"
	EntryTypeFixPattern    EntryType = "fix_pattern"
)

// Severity represents the impact level of a learning entry.
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

// LearningEntry represents one learning record (R12).
type LearningEntry struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	Type       EntryType `json:"type"`
	Phase      string    `json:"phase"`
	SpecID     string    `json:"spec_id,omitempty"`
	Files      []string  `json:"files"`
	Packages   []string  `json:"packages"`
	Pattern    string    `json:"pattern"`
	Resolution string    `json:"resolution"`
	// Expected, Actual, and Repro carry incident evidence (SPEC-HARNEVAL-002).
	// Repro is data only: nothing in the harness executes it.
	Expected   string   `json:"expected,omitempty"`
	Actual     string   `json:"actual,omitempty"`
	Repro      string   `json:"repro,omitempty"`
	Severity   Severity `json:"severity"`
	ReuseCount int      `json:"reuse_count"`
	Line       int      `json:"-"`
}

// entryIDRegex matches L-{NNN} format (one or more digits after L-).
var entryIDRegex = regexp.MustCompile(`^L-\d{3,}$`)

// IsValidEntryID checks whether id matches the L-{NNN} format.
func IsValidEntryID(id string) bool {
	return entryIDRegex.MatchString(id)
}

// RelevanceQuery holds parameters for relevance matching (R13).
type RelevanceQuery struct {
	Files    []string
	Packages []string
	Keywords []string
	SpecID   string
}

// Summary holds learning summary for sync display (R8).
type Summary struct {
	TotalEntries     int
	NewEntries       int
	TypeCounts       map[EntryType]int
	TopPatterns      []PatternStat
	ImprovementAreas []string
	Improvements     []string
}

// RecordOpts holds options for recording a learning entry.
type RecordOpts struct {
	Phase      string
	SpecID     string
	Files      []string
	Packages   []string
	Pattern    string
	Resolution string
	Expected   string
	Actual     string
	Repro      string
	Severity   Severity
}

// PatternStat tracks reuse frequency of a pattern.
type PatternStat struct {
	Pattern    string
	ReuseCount int
}

// UnmarshalJSON customizes parsing of LearningEntry including fallback timestamp parsing.
func (le *LearningEntry) UnmarshalJSON(b []byte) error {
	type Alias LearningEntry
	aux := &struct {
		Timestamp string `json:"timestamp"`
		*Alias
	}{
		Alias: (*Alias)(le),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	if aux.Timestamp == "" {
		return nil
	}

	tStr := strings.Trim(aux.Timestamp, `"`)
	var t time.Time
	var err error
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05-0700",
		"2006-01-02T15:04:05.999999999-0700",
		"2006-01-02T15:04:05.999999-0700",
		"2006-01-02T15:04:05.999-0700",
	}
	for _, f := range formats {
		t, err = time.Parse(f, tStr)
		if err == nil {
			le.Timestamp = t
			return nil
		}
	}
	return err
}

// MarshalJSON customizes serialization of LearningEntry to ensure canonical RFC3339 timestamp.
func (le LearningEntry) MarshalJSON() ([]byte, error) {
	type Alias LearningEntry
	tStr := le.Timestamp.Format(time.RFC3339Nano)
	aux := &struct {
		Timestamp string `json:"timestamp"`
		Alias
	}{
		Timestamp: tStr,
		Alias:     (Alias)(le),
	}
	return json.Marshal(aux)
}

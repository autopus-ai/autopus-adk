// Package intake turns learning entries into quarantined golden-task
// candidates (SPEC-HARNEVAL-002). It owns the learning fingerprint, the
// candidate and record wire formats, and the path-confined file access every
// intake-area operation goes through. Nothing here executes a repro value.
package intake

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/harneval"
	"github.com/insajin/autopus-adk/pkg/learn"
)

// IntakeResultSchemaV1 identifies the stdout document of one intake run.
const IntakeResultSchemaV1 = "harness_intake_result.v1"

// Row results of harness_intake_result.v1.
const (
	ResultCreated            = "created"
	ResultGrouped            = "grouped"
	ResultDuplicateCandidate = "duplicate_candidate"
	ResultAlreadyPromoted    = "already_promoted"
	ResultAlreadyRejected    = "already_rejected"
	ResultSkipped            = "skipped"
)

// Reasons of skipped rows; learning_field_invalid and candidate_text_invalid
// are declared with the text checks.
const (
	ReasonLearningNotFound      = "learning_not_found"
	ReasonMissingExpectedActual = "learning_missing_expected_actual"
	ReasonCandidateIDCollision  = "candidate_id_collision"
)

// Reasons of a run-level error, which exits 1 and writes nothing.
const (
	ReasonSelectionInvalid           = "selection_invalid"
	ReasonFlagPairRequired           = "flag_pair_required"
	ReasonFlagRequiresSingleLearning = "flag_requires_single_learning"
	ReasonKindUnsupported            = "kind_unsupported"
	// ReasonEvalLinksUnreadable reports an intake record, manifest, or active
	// task that cannot be read or decoded; prune uses the same reason.
	ReasonEvalLinksUnreadable = "eval_links_unreadable"
)

// RunError is a run-level refusal with one closed reason; Detail names the
// offending field or check when there is one.
type RunError struct {
	Reason string
	Detail string
	Err    error
}

func (e *RunError) Error() string {
	msg := e.Reason
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	if e.Err != nil {
		msg += ": " + e.Err.Error()
	}
	return msg
}

func (e *RunError) Unwrap() error { return e.Err }

// Entry is the part of a learning entry that intake reads. The caller maps a
// learn store entry onto it, so the store format stays owned by pkg/learn.
type Entry struct {
	ID       string
	Type     string
	Pattern  string
	Files    []string
	Packages []string
	Expected string
	Actual   string
	Repro    string
}

// Redactor masks secrets in free text. The production implementation wraps
// pkg/secretscan.Redact (REQ-HC-01); intake depends on this seam alone and
// never imports a detector.
type Redactor interface {
	Redact(text string) string
}

// RedactorFunc adapts a function to Redactor.
type RedactorFunc func(string) string

// Redact returns f(text).
func (f RedactorFunc) Redact(text string) string { return f(text) }

// Request is one `auto eval harness intake` invocation. Entries is every
// entry the learn store holds; exactly one of LearningIDs and AllEligible
// selects among them. Expected and Actual are the optional flag pair, empty
// when not given. Redactor is required.
type Request struct {
	Root        string
	Entries     []Entry
	LearningIDs []string
	AllEligible bool
	Expected    string
	Actual      string
	Kind        string
	Redactor    Redactor
}

// Row is one processed entry. It never carries learning text.
type Row struct {
	LearningID   string   `json:"learning_id"`
	Result       string   `json:"result"`
	CandidateID  string   `json:"candidate_id,omitempty"`
	Match        string   `json:"match,omitempty"`
	Fingerprint  string   `json:"fingerprint,omitempty"`
	LearningRefs []string `json:"learning_refs,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// Result is the harness_intake_result.v1 document; rows keep processing order.
type Result struct {
	SchemaVersion string `json:"schema_version"`
	Rows          []Row  `json:"rows"`
}

// ExitCode is 2 when any row was skipped and 0 otherwise; a run-level error
// exits 1 instead.
func (r Result) ExitCode() int {
	for _, row := range r.Rows {
		if row.Result == ResultSkipped {
			return 2
		}
	}
	return 0
}

// Run checks the request, reads what the intake area already holds, plans
// one row per selected entry in ascending numeric id order, groups equal
// fingerprints under the lowest id, and then creates one candidate per new
// fingerprint. A run-level error is returned before anything is written.
func Run(req Request) (Result, error) {
	if req.Redactor == nil {
		return Result{}, errors.New("intake: Request.Redactor is required")
	}
	ids, err := req.selection()
	if err != nil {
		return Result{}, err
	}
	if err := writeSupported(); err != nil {
		return Result{}, &RunError{Reason: ReasonPlatformUnsupported, Err: err}
	}
	if err := req.checkFlagValues(); err != nil {
		return Result{}, err
	}
	a, err := openArea(req.Root)
	if err != nil {
		return Result{}, fmt.Errorf("open project root: %w", err)
	}
	// Every write is fsynced before Run returns, so a close error loses nothing.
	defer func() { _ = a.close() }()
	p := &planner{req: req, byFingerprint: map[string]*group{}}
	if p.index, err = loadIndex(a); err != nil {
		return Result{}, areaError(err, ReasonEvalLinksUnreadable)
	}
	for _, item := range selectEntries(req, ids) {
		p.add(item)
	}
	if err := p.publish(a); err != nil {
		return Result{}, areaError(err, "")
	}
	return p.result(), nil
}

// areaError turns an unsafe path into path_unsafe and any other failure into
// fallback, or leaves it a plain error when fallback is empty.
func areaError(err error, fallback string) error {
	switch {
	case errors.Is(err, errPathUnsafe):
		return &RunError{Reason: ReasonPathUnsafe, Err: err}
	case fallback != "":
		return &RunError{Reason: fallback, Err: err}
	default:
		return err
	}
}

// selection checks the flag combination and returns the requested ids.
func (req Request) selection() (map[string]bool, error) {
	if req.Kind != "" && req.Kind != harneval.KindSurface {
		return nil, &RunError{Reason: ReasonKindUnsupported, Detail: "only surface incident tasks are supported"}
	}
	if (len(req.LearningIDs) > 0) == req.AllEligible {
		return nil, &RunError{Reason: ReasonSelectionInvalid, Detail: "give exactly one of --learning and --all-eligible"}
	}
	ids := map[string]bool{}
	for _, raw := range req.LearningIDs {
		id := strings.TrimSpace(raw)
		if !learn.IsValidEntryID(id) {
			return nil, &RunError{Reason: ReasonSelectionInvalid, Detail: "learning ids must match L-NNN"}
		}
		ids[id] = true
	}
	if (req.Expected == "") != (req.Actual == "") {
		return nil, &RunError{Reason: ReasonFlagPairRequired, Detail: "--expected and --actual go together"}
	}
	if req.Expected != "" && len(ids) != 1 {
		return nil, &RunError{Reason: ReasonFlagRequiresSingleLearning, Detail: "--expected and --actual need exactly one --learning id"}
	}
	return ids, nil
}

// checkFlagValues runs the REQ-HC-01 checks on the flag pair once, so a bad
// flag value is a run-level error rather than a skipped row.
func (req Request) checkFlagValues() error {
	if req.Expected == "" {
		return nil
	}
	for _, field := range [][2]string{{"expected", req.Expected}, {"actual", req.Actual}} {
		if _, refused := redactField(req.Redactor, field[0], field[1], evidenceCap); refused != nil {
			return &RunError{Reason: refused.reason, Detail: refused.field + ": " + refused.detail}
		}
	}
	return nil
}

// selected is one entry to process, or a requested id the store lacks.
type selected struct {
	id    string
	entry *Entry
}

// selectEntries returns the selection in ascending numeric id order; equal
// ids keep store order. Entries with a malformed id are never selected, and a
// requested id without an entry yields a nil-entry item.
func selectEntries(req Request, ids map[string]bool) []selected {
	var items []selected
	found := map[string]bool{}
	for index := range req.Entries {
		entry := &req.Entries[index]
		if learn.IsValidEntryID(entry.ID) && (req.AllEligible || ids[entry.ID]) {
			items = append(items, selected{id: entry.ID, entry: entry})
			found[entry.ID] = true
		}
	}
	for id := range ids {
		if !found[id] {
			items = append(items, selected{id: id})
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return idLess(items[i].id, items[j].id) })
	return items
}

// idLess orders two valid L-NNN ids by number, so L-999 precedes L-1000.
func idLess(a, b string) bool {
	digitsA, digitsB := strings.TrimLeft(a[2:], "0"), strings.TrimLeft(b[2:], "0")
	if len(digitsA) != len(digitsB) {
		return len(digitsA) < len(digitsB)
	}
	if digitsA != digitsB {
		return digitsA < digitsB
	}
	return a < b
}

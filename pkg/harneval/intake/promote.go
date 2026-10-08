package intake

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// PromoteResultSchemaV1 identifies the stdout document of one promotion.
const PromoteResultSchemaV1 = "harness_promote_result.v1"

// Results of a promotion that holds; a refused promotion is a *RunError.
const (
	PromoteResultPromoted      = "promoted"
	PromoteResultAlreadyActive = "already_active"
)

// Current outcomes of the promoted task.
const (
	OutcomePass         = harneval.ResultPass
	OutcomeFail         = harneval.ResultFail
	OutcomeNotEvaluated = "not_evaluated"
)

// Refusal reasons of promote (REQ-HC-06) besides the path reasons declared
// with the path helpers. A promoted fingerprint is ResultAlreadyPromoted.
const (
	ReasonCandidateMissing            = "candidate_missing"
	ReasonCandidateInvalid            = "candidate_invalid"
	ReasonCandidateProvenanceMismatch = "candidate_provenance_mismatch"
	ReasonDraftIncomplete             = "draft_incomplete"
	ReasonNoActivePathForKind         = "no_active_path_for_kind"
	ReasonActiveSetInvalid            = "active_set_invalid"
	ReasonTaskIDExists                = "task_id_exists"
	ReasonPromoteRolledBack           = "promote_rolled_back"
)

// Details of a candidate_invalid document. A draft task the SPEC-HARNEVAL-001
// strict decoder rejects carries that decoder's detail instead.
const (
	DetailDecode     = "decode"
	DetailIDMismatch = "id_mismatch"
)

// PromoteRequest is one `auto eval harness promote` invocation.
type PromoteRequest struct {
	Root        string
	CandidateID string
	// Run holds the SPEC-HARNEVAL-001 run seams current_outcome is evaluated
	// with; the zero value is production. Its Baseline and Evaluate seams are
	// replaced, because one task is evaluated and compared with nothing.
	Run harneval.RunOptions
	// interrupt, when set, is called after publication steps 10, 11, and 12.
	// An error stops the promotion there as a crash would: nothing after that
	// point runs, so a test can rerun from each interruption point.
	interrupt func(step int) error
}

// PromoteResult is the harness_promote_result.v1 document.
type PromoteResult struct {
	SchemaVersion      string `json:"schema_version"`
	Result             string `json:"result"`
	CandidateID        string `json:"candidate_id"`
	TaskID             string `json:"task_id"`
	TaskPath           string `json:"task_path"`
	LinkPath           string `json:"link_path"`
	CurrentOutcome     string `json:"current_outcome"`
	NotEvaluatedReason string `json:"not_evaluated_reason,omitempty"`
}

// promotion is the state one Promote call builds check by check. set is the
// active set check 8 loaded and taskData the canonical task file; resumed is
// set when an earlier, interrupted run already published exactly those bytes.
type promotion struct {
	req                            PromoteRequest
	area                           *area
	candidate                      Candidate
	set                            *harneval.Set
	taskData                       []byte
	candidateRel, taskRel, linkRel string
	resumed                        bool
}

// Promote applies the ordered checks 1 to 9 of REQ-HC-06, stopping at the
// first failure without writing anything. It then publishes the task file,
// rolls back that file alone when the SPEC-HARNEVAL-001 load after
// publication fails, publishes the permanent link record, removes the
// candidate last, and reports the task's current outcome. A rerun after an
// interruption at any point converges on the files of an uninterrupted run.
// No repro value is ever executed.
func Promote(ctx context.Context, req PromoteRequest) (PromoteResult, error) {
	if !ValidCandidateID(req.CandidateID) {
		return PromoteResult{}, &RunError{Reason: ReasonCandidateIDInvalid,
			Err: errors.New("candidate ids must match GTC-<12 lowercase hex>")}
	}
	if err := writeSupported(); err != nil {
		return PromoteResult{}, &RunError{Reason: ReasonPlatformUnsupported, Err: err}
	}
	a, err := openArea(req.Root)
	if err != nil {
		return PromoteResult{}, fmt.Errorf("open project root: %w", err)
	}
	// Every write is fsynced before it counts, so a close error loses nothing.
	defer func() { _ = a.close() }()
	p := &promotion{req: req, area: a, candidateRel: IntakeDir + "/" + req.CandidateID + ".json"}
	if err := p.check(); err != nil {
		return PromoteResult{}, err
	}
	if err := p.publish(); err != nil {
		return PromoteResult{}, err
	}
	return p.result(ctx), nil
}

// check runs checks 2 to 9 in order. Check 9 is skipped when check 8 found
// an interrupted publication to resume.
func (p *promotion) check() error {
	for _, step := range []func() error{p.readCandidate, p.checkDraft, p.checkActivePath, p.loadActiveSet} {
		if err := step(); err != nil {
			return err
		}
	}
	if p.resumed {
		return nil
	}
	return p.checkTarget()
}

// readCandidate is checks 2 and 3: a safe layout, then a regular candidate
// file that decodes strictly and names itself.
func (p *promotion) readCandidate() error {
	if err := p.area.checkLayout(); err != nil {
		return areaError(err, "")
	}
	data, err := p.area.readFile(p.candidateRel)
	if errors.Is(err, fs.ErrNotExist) {
		return &RunError{Reason: ReasonCandidateMissing, Err: fmt.Errorf("%s does not exist", p.candidateRel)}
	}
	if err != nil {
		return areaError(err, "")
	}
	c := &p.candidate
	if err := decodeStrict(data, c); err != nil {
		return &RunError{Reason: ReasonCandidateInvalid, Detail: DetailDecode, Err: err}
	}
	if _, err := recordKey(c.SchemaVersion, CandidateSchemaV1, c.FingerprintVersion, c.Fingerprint); err != nil {
		return &RunError{Reason: ReasonCandidateInvalid, Detail: DetailDecode, Err: err}
	}
	if c.ID != p.req.CandidateID {
		return &RunError{Reason: ReasonCandidateInvalid, Detail: DetailIDMismatch,
			Err: fmt.Errorf("%s holds candidate %q", p.candidateRel, c.ID)}
	}
	return nil
}

// checkDraft is checks 4 to 6: provenance that ties the draft task to this
// candidate, the parts a person completes, and the SPEC-HARNEVAL-001 strict
// decoder on the canonical task bytes. Only a task id that passed the
// decoder's grammar is joined into a path.
func (p *promotion) checkDraft() error {
	c, task := p.candidate, p.candidate.Task
	if detail := provenanceGap(c); detail != "" {
		return &RunError{Reason: ReasonCandidateProvenanceMismatch, Detail: detail}
	}
	if detail := draftGap(task); detail != "" {
		return &RunError{Reason: ReasonDraftIncomplete, Detail: detail}
	}
	data, err := encodeRecord(task)
	if err != nil {
		return err
	}
	if _, err := harneval.DecodeTask(data); err != nil {
		return &RunError{Reason: ReasonCandidateInvalid, Detail: invalidDetail(err), Err: err}
	}
	p.taskData = data
	p.taskRel = SurfaceTaskDir + "/" + task.ID + ".json"
	p.linkRel = PromotedDir + "/" + task.ID + ".json"
	return nil
}

// provenanceGap names the provenance field that does not tie the draft task
// to its candidate, or returns "".
func provenanceGap(c Candidate) string {
	provenance := c.Task.Provenance
	switch {
	case provenance.Kind != "incident":
		return "kind"
	case provenance.Fingerprint != c.Fingerprint:
		return "fingerprint"
	case provenance.Ref != c.Representative || !slices.Contains(c.LearningRefs, provenance.Ref):
		return "ref"
	}
	return ""
}

// draftGap names the first part of the draft that is not complete, in the
// fixed order status, category, assertions, kind, or returns "".
func draftGap(task harneval.Task) string {
	switch {
	case task.Status.State != harneval.StateActive:
		return "status"
	case strings.TrimSpace(task.Category) == "":
		return "category"
	case len(task.Assertions) == 0:
		return "assertions"
	case task.Kind != harneval.KindSurface:
		return "kind"
	}
	return ""
}

// checkActivePath is check 7: an active path must cover the surface task
// directory. A manifest this lenient read cannot use is left to check 8,
// where the SPEC-HARNEVAL-001 loader names its defect.
func (p *promotion) checkActivePath() error {
	actives, err := p.area.activePaths()
	covered := err != nil || slices.ContainsFunc(actives, func(active string) bool {
		return active == SurfaceTaskDir || strings.HasPrefix(SurfaceTaskDir, active+"/")
	})
	if !covered {
		return &RunError{Reason: ReasonNoActivePathForKind, Detail: harneval.KindSurface}
	}
	return nil
}

// loadActiveSet is check 8: the existing active set must load with the
// SPEC-HARNEVAL-001 loader. When it does not but the task file already holds
// exactly the bytes this promotion publishes, an earlier run stopped between
// steps 10 and 11, and the promotion resumes at the post-check, which rolls
// the task back when it is what broke the set.
func (p *promotion) loadActiveSet() error {
	set, err := harneval.LoadSet(p.req.Root)
	if err == nil {
		p.set = set
		return nil
	}
	if existing, readErr := p.area.readFile(p.taskRel); readErr == nil && bytes.Equal(existing, p.taskData) {
		p.resumed = true
		return nil
	}
	return &RunError{Reason: ReasonActiveSetInvalid, Detail: invalidDetail(err), Err: err}
}

// checkTarget is check 9. The target already holding these bytes is an
// interrupted earlier run: step 10 confirms it instead of publishing.
// Another file of the task id, or other bytes at the target, is
// task_id_exists; a link record or an active incident task of the same
// fingerprint is already_promoted; a link record of the task id with another
// fingerprint is task_id_exists.
func (p *promotion) checkTarget() error {
	existing, err := p.area.readFile(p.taskRel)
	switch {
	case err == nil && bytes.Equal(existing, p.taskData):
		p.resumed = true
		return nil
	case err == nil:
		return taskIDExists(p.taskRel)
	case !errors.Is(err, fs.ErrNotExist):
		return areaError(err, "")
	}
	for _, task := range p.set.Tasks {
		if task.ID == p.candidate.Task.ID {
			return taskIDExists(task.Path)
		}
	}
	match, err := p.promotedMatch()
	if err != nil {
		return err
	}
	if match != "" {
		return &RunError{Reason: ResultAlreadyPromoted, Detail: match}
	}
	_, err = p.area.lstatChain(p.linkRel, false)
	switch {
	case err == nil:
		return taskIDExists(p.linkRel)
	case errors.Is(err, fs.ErrNotExist):
		return nil
	}
	return areaError(err, "")
}

// taskIDExists refuses a task id that rel already holds.
func taskIDExists(rel string) error { return &RunError{Reason: ReasonTaskIDExists, Detail: rel} }

// promotedMatch returns the id of a link record or of an active incident task
// that carries the candidate's fingerprint, matched as intake matches them.
func (p *promotion) promotedMatch() (string, error) {
	x := &index{promoted: map[fpKey]string{}}
	if err := scanRecords(p.area, PromotedDir, ValidTaskID, x.addLink); err != nil {
		return "", areaError(err, ReasonEvalLinksUnreadable)
	}
	for _, task := range p.set.Tasks {
		var ref taskRef
		ref.ID, ref.Status.State = task.ID, task.Status.State
		ref.Provenance.Kind, ref.Provenance.Fingerprint = task.Provenance.Kind, task.Provenance.Fingerprint
		x.addTask(ref)
	}
	return x.promoted[fpKey{p.candidate.FingerprintVersion, p.candidate.Fingerprint}], nil
}

// invalidDetail returns the SPEC-HARNEVAL-001 detail code carried by err.
func invalidDetail(err error) string {
	var invalid *harneval.InvalidError
	if errors.As(err, &invalid) {
		return invalid.Detail
	}
	return ""
}

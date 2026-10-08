package intake

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// Publication steps of REQ-HC-06 after which a test can inject an
// interruption.
const (
	stepTaskPublished = 10
	stepPostChecked   = 11
	stepLinkPublished = 12
)

// errPublishedDiffers reports a record name that already holds other bytes.
var errPublishedDiffers = errors.New("another file is already published under this name")

// publishOrConfirm makes rel hold exactly data. A missing rel is published by
// createExclusive: temp file, fsync, Root.Link, directory fsync, temp
// removal, directory fsync. When rel exists, createExclusive still removes
// the stale temps of rel and fsyncs the directory before it fails with
// fs.ErrExist, so an earlier publication of the same bytes is confirmed
// durable without a rewrite; other bytes are errPublishedDiffers.
func (a *area) publishOrConfirm(rel string, data []byte) error {
	err := a.createExclusive(rel, data)
	if !errors.Is(err, fs.ErrExist) {
		return err
	}
	existing, err := a.readFile(rel)
	if err != nil {
		return err
	}
	if !bytes.Equal(existing, data) {
		return fmt.Errorf("%s: %w", rel, errPublishedDiffers)
	}
	return nil
}

// publish runs steps 10 to 13 in their only order: the task file, the load
// after publication with its rollback, the link record, and the candidate
// removal. The candidate and the link stay untouched until the task has
// passed the post-check, and the candidate goes only once both files are
// durable.
func (p *promotion) publish() error {
	if err := p.publishRecord(SurfaceTaskDir, p.taskRel, p.taskData); err != nil {
		return err
	}
	if err := p.interrupted(stepTaskPublished); err != nil {
		return err
	}
	if err := p.postCheck(); err != nil {
		return err
	}
	if err := p.interrupted(stepPostChecked); err != nil {
		return err
	}
	link, err := encodeRecord(p.link())
	if err != nil {
		return err
	}
	if err := p.publishRecord(PromotedDir, p.linkRel, link); err != nil {
		return err
	}
	if err := p.interrupted(stepLinkPublished); err != nil {
		return err
	}
	if err := p.area.removeFile(p.candidateRel); err != nil {
		return areaError(err, "")
	}
	return nil
}

// publishRecord creates dir as needed and publishes data at rel, or confirms
// an earlier publication of the same bytes. Other bytes at rel, which check 9
// rules out unless the file appeared since, are task_id_exists.
func (p *promotion) publishRecord(dir, rel string, data []byte) error {
	err := p.area.ensureDir(dir)
	if err == nil {
		err = p.area.publishOrConfirm(rel, data)
	}
	switch {
	case errors.Is(err, errPublishedDiffers):
		return &RunError{Reason: ReasonTaskIDExists, Detail: rel, Err: err}
	case err != nil:
		return areaError(err, "")
	}
	return nil
}

// postCheck is step 11: the SPEC-HARNEVAL-001 loader reads the whole active
// set again. When the set is invalid, only the task file is removed and its
// directory fsynced, and the promotion ends as promote_rolled_back with the
// loader's detail.
func (p *promotion) postCheck() error {
	_, loadErr := harneval.LoadSet(p.req.Root)
	if loadErr == nil {
		return nil
	}
	if err := p.area.removeFile(p.taskRel); err != nil {
		return fmt.Errorf("roll back %s after %v: %w", p.taskRel, loadErr, err)
	}
	return &RunError{Reason: ReasonPromoteRolledBack, Detail: invalidDetail(loadErr), Err: loadErr}
}

// interrupted calls the test seam after step; nil means carry on.
func (p *promotion) interrupted(step int) error {
	if p.req.interrupt == nil {
		return nil
	}
	return p.req.interrupt(step)
}

// link is the permanent record of this promotion. It keeps every learning
// ref of the group and the representative's evidence, so prune keeps each
// entry after the candidate is gone.
func (p *promotion) link() Link {
	c := p.candidate
	fields := c.RedactedFields
	if fields == nil {
		fields = []string{}
	}
	return Link{
		SchemaVersion: LinkSchemaV1, TaskID: c.Task.ID, CandidateID: c.ID,
		FingerprintVersion: c.FingerprintVersion, Fingerprint: c.Fingerprint,
		Representative: c.Representative, LearningRefs: c.LearningRefs,
		Expected: c.Expected, Actual: c.Actual, Repro: c.Repro,
		Redacted: c.Redacted, RedactedFields: fields,
	}
}

// result is the document of a promotion that holds.
func (p *promotion) result(ctx context.Context) PromoteResult {
	result := PromoteResult{
		SchemaVersion: PromoteResultSchemaV1, Result: PromoteResultPromoted,
		CandidateID: p.candidate.ID, TaskID: p.candidate.Task.ID,
		TaskPath: p.taskRel, LinkPath: p.linkRel,
	}
	if p.resumed {
		result.Result = PromoteResultAlreadyActive
	}
	result.CurrentOutcome, result.NotEvaluatedReason = currentOutcome(ctx, p.req.Root, result.TaskID, p.req.Run)
	return result
}

// currentOutcome evaluates the promoted task alone through the
// SPEC-HARNEVAL-001 run: its load, template check, hermetic generation under
// the sentinel, and evaluator. The baseline is empty because one outcome is
// compared with nothing. A precondition that stops the run before generation
// yields not_evaluated with its reason; a run that fails after generation
// started is generation_failed, and a generated surface that never reached
// the task is task_missing. The promotion holds in every case.
func currentOutcome(ctx context.Context, root, taskID string, opts harneval.RunOptions) (string, string) {
	evaluate, mutate := opts.Evaluate, opts.Mutate
	if evaluate == nil {
		evaluate = func(task harneval.Task, generation *harneval.Generation) (harneval.TaskOutcome, bool) {
			return harneval.EvaluateTask(task, generation), true
		}
	}
	generated, evaluated, passed := false, false, false
	opts.Mutate = func(generation *harneval.Generation) error {
		generated = true
		if mutate == nil {
			return nil
		}
		return mutate(generation)
	}
	opts.Evaluate = func(task harneval.Task, generation *harneval.Generation) (harneval.TaskOutcome, bool) {
		if task.ID != taskID {
			return harneval.TaskOutcome{ID: task.ID}, false
		}
		outcome, ran := evaluate(task, generation)
		evaluated, passed = ran, outcome.Passed
		return outcome, ran
	}
	opts.Baseline = func(string) (*harneval.Baseline, error) { return &harneval.Baseline{}, nil }
	result, err := harneval.Run(ctx, root, opts)
	switch {
	case evaluated && passed:
		return OutcomePass, ""
	case evaluated:
		return OutcomeFail, ""
	case err != nil:
		return OutcomeNotEvaluated, harneval.ReasonGenerationFailed
	case !generated && len(result.FailureReasons) > 0:
		return OutcomeNotEvaluated, result.FailureReasons[0]
	}
	return OutcomeNotEvaluated, harneval.ReasonTaskMissing
}

package intake

import (
	"bytes"
	"errors"
	"io/fs"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// loadActiveSet is check 8: the existing active set must load with the
// SPEC-HARNEVAL-001 loader. When it does not but the task file already holds
// exactly the bytes this promotion publishes and no link record exists yet,
// an earlier run stopped between steps 10 and 11, and the promotion resumes
// at the post-check, which rolls the task back when it is what broke the set.
// A link record means that run already passed the post-check, so the set
// broke for another reason: the promotion stops and changes nothing.
func (p *promotion) loadActiveSet() error {
	set, err := harneval.LoadSet(p.req.Root)
	if err == nil {
		p.set = set
		return nil
	}
	if p.publishedBeforePostCheck() {
		p.resumed = true
		return nil
	}
	return &RunError{Reason: ReasonActiveSetInvalid, Detail: invalidDetail(err), Err: err}
}

// publishedBeforePostCheck reports a task file holding exactly this
// promotion's bytes with no link record of its id.
func (p *promotion) publishedBeforePostCheck() bool {
	existing, err := p.area.readFile(p.taskRel)
	if err != nil || !bytes.Equal(existing, p.taskData) {
		return false
	}
	_, err = p.area.lstatChain(p.linkRel, false)
	return errors.Is(err, fs.ErrNotExist)
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

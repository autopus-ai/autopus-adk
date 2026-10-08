package intake

import (
	"errors"
	"io/fs"
)

// group is one candidate planned in this run and the rows that joined it.
type group struct {
	candidate Candidate
	rows      []*Row
}

// planner decides every row before anything is written. Entries arrive in
// ascending numeric id order, so the first entry of a fingerprint is its
// group's representative and learning_refs stay in numeric order.
type planner struct {
	req           Request
	index         *index
	rows          []*Row
	groups        []*group
	byFingerprint map[string]*group
}

// skip records a skipped row.
func (p *planner) skip(row *Row, reason string) {
	row.Result, row.Reason = ResultSkipped, reason
	p.rows = append(p.rows, row)
}

// add decides one selected entry. --all-eligible quietly passes over entries
// that are not eligible (no evidence, or a fingerprint already known);
// --learning reports them. Every entry's own text is checked, so a member
// with invalid text is skipped and never joins a group.
func (p *planner) add(item selected) {
	if item.entry == nil {
		p.skip(&Row{LearningID: item.id}, ReasonLearningNotFound)
		return
	}
	e := *item.entry
	row := &Row{LearningID: e.ID, Fingerprint: Fingerprint(e)}
	expected, actual := e.Expected, e.Actual
	if p.req.Expected != "" {
		expected, actual = p.req.Expected, p.req.Actual
	}
	if expected == "" || actual == "" {
		if !p.req.AllEligible {
			p.skip(row, ReasonMissingExpectedActual)
		}
		return
	}
	if result, id, known := p.index.match(row.Fingerprint); known {
		if !p.req.AllEligible {
			row.Result, row.Match = result, id
			p.rows = append(p.rows, row)
		}
		return
	}
	ev, refused := evidenceFor(p.req.Redactor, e, expected, actual)
	if refused != nil {
		p.skip(row, refused.reason)
		return
	}
	p.join(row, e, ev)
}

// join adds a checked entry to the group of its fingerprint, opening the
// group when this is the fingerprint's first entry. A candidate name already
// taken by a file with another fingerprint is a collision.
func (p *planner) join(row *Row, e Entry, ev evidence) {
	if g := p.byFingerprint[row.Fingerprint]; g != nil {
		row.Result, row.CandidateID = ResultGrouped, g.candidate.ID
		if refs := g.candidate.LearningRefs; refs[len(refs)-1] != e.ID {
			g.candidate.LearningRefs = append(refs, e.ID)
		}
		g.rows = append(g.rows, row)
		p.rows = append(p.rows, row)
		return
	}
	if row.CandidateID = candidateIDFor(row.Fingerprint); p.index.openIDs[row.CandidateID] {
		p.skip(row, ReasonCandidateIDCollision)
		return
	}
	g := &group{candidate: newCandidate(row.Fingerprint, e, ev), rows: []*Row{row}}
	row.Result = ResultCreated
	p.rows = append(p.rows, row)
	p.groups = append(p.groups, g)
	p.byFingerprint[row.Fingerprint] = g
}

// collide turns every row of a group whose candidate name is taken into a
// candidate_id_collision skip.
func (g *group) collide() {
	for _, row := range g.rows {
		row.Result, row.Reason, row.LearningRefs = ResultSkipped, ReasonCandidateIDCollision, nil
	}
}

// publish writes each planned candidate; a name that appeared since the
// index was read turns the group's rows into collisions.
func (p *planner) publish(a *area) error {
	if len(p.groups) == 0 {
		return nil
	}
	if err := a.ensureDir(IntakeDir); err != nil {
		return err
	}
	for _, g := range p.groups {
		g.rows[0].LearningRefs = g.candidate.LearningRefs
		data, err := encodeRecord(g.candidate)
		if err != nil {
			return err
		}
		err = a.createExclusive(candidatePath(g.candidate.ID), data)
		if errors.Is(err, fs.ErrExist) {
			g.collide()
			continue
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// result returns the rows in processing order; rows is never null.
func (p *planner) result() Result {
	rows := make([]Row, 0, len(p.rows))
	for _, row := range p.rows {
		rows = append(rows, *row)
	}
	return Result{SchemaVersion: IntakeResultSchemaV1, Rows: rows}
}

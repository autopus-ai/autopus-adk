package intake

import (
	"errors"
	"fmt"
	"io/fs"
)

// Reasons and details of reading one open candidate, shared by promote and
// reject. A draft task the SPEC-HARNEVAL-001 strict decoder rejects is
// candidate_invalid too, with that decoder's detail instead.
const (
	ReasonCandidateMissing = "candidate_missing"
	ReasonCandidateInvalid = "candidate_invalid"

	DetailDecode     = "decode"
	DetailIDMismatch = "id_mismatch"
)

// candidatePath is the intake-area path of the open candidate id. Only an id
// that passed ValidCandidateID is ever joined into a path.
func candidatePath(id string) string { return IntakeDir + "/" + id + ".json" }

// readCandidate checks the intake layout, then reads the open candidate id: a
// regular file that decodes strictly, carries the fields every intake record
// shares, and names itself. An unsafe layout or file is path_unsafe, a
// missing file candidate_missing, and a document that does not decode or
// names another id candidate_invalid with detail decode or id_mismatch.
func (a *area) readCandidate(id string) (Candidate, error) {
	if err := a.checkLayout(); err != nil {
		return Candidate{}, areaError(err, "")
	}
	rel := candidatePath(id)
	data, err := a.readFile(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return Candidate{}, &RunError{Reason: ReasonCandidateMissing, Err: fmt.Errorf("%s does not exist", rel)}
	}
	if err != nil {
		return Candidate{}, areaError(err, "")
	}
	var c Candidate
	if err := decodeStrict(data, &c); err != nil {
		return Candidate{}, &RunError{Reason: ReasonCandidateInvalid, Detail: DetailDecode, Err: err}
	}
	if _, err := recordKey(c.SchemaVersion, CandidateSchemaV1, c.FingerprintVersion, c.Fingerprint); err != nil {
		return Candidate{}, &RunError{Reason: ReasonCandidateInvalid, Detail: DetailDecode, Err: err}
	}
	if c.ID != id {
		return Candidate{}, &RunError{Reason: ReasonCandidateInvalid, Detail: DetailIDMismatch,
			Err: fmt.Errorf("%s holds candidate %q", rel, c.ID)}
	}
	return c, nil
}

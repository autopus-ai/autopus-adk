package checklist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
)

// RefutationsDir holds the refutations this machine actually ran. It lives
// under the gitignored runtime tree, so a change cannot ship a forged one
// the way it can edit a candidate file's own refutations field.
const RefutationsDir = ".autopus/runtime/checklist-refutations"

type refutationRecord struct {
	ID          string       `json:"id"`
	Digest      string       `json:"digest"`
	DraftedBy   string       `json:"drafted_by"`
	Refutations []Refutation `json:"refutations"`
}

// candidateDigest pins what was refuted: the check and who drafted it. Any
// later edit to the condition, verify command or body changes it.
func candidateDigest(c Candidate) string {
	data, _ := json.Marshal(struct {
		Check     Check
		DraftedBy string
	}{c.Check, c.DraftedBy})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func saveRefutationRecord(root string, c Candidate) error {
	data, err := json.MarshalIndent(refutationRecord{ID: c.ID, Digest: candidateDigest(c), DraftedBy: c.DraftedBy, Refutations: c.Refutations}, "", "  ")
	if err != nil {
		return err
	}
	return writeFile(root, path.Join(RefutationsDir, c.ID+".json"), append(data, '\n'))
}

// verifiedRefutations returns the refutations this machine ran for exactly
// this candidate content, or an error saying how to get them.
func verifiedRefutations(root string, c Candidate) (Candidate, error) {
	data, err := readFile(root, path.Join(RefutationsDir, c.ID+".json"))
	if err != nil {
		return c, fmt.Errorf("%s: no refutation run on this machine; run `auto checklist refute %s` first", c.ID, c.ID)
	}
	var rec refutationRecord
	if err := json.Unmarshal(data, &rec); err != nil || rec.ID != c.ID || rec.Digest != candidateDigest(c) {
		return c, fmt.Errorf("%s changed after it was refuted on this machine; run `auto checklist refute %s` again", c.ID, c.ID)
	}
	c.Refutations, c.DraftedBy = rec.Refutations, rec.DraftedBy
	return c, nil
}

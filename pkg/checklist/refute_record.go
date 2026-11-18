package checklist

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// refutationsGitPath is where this machine keeps the refutations it ran:
// inside the git directory, which no commit can write and no clone copies,
// so a change cannot ship a forged record the way it can a tracked file.
const refutationsGitPath = "autopus/checklist-refutations"

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

var errNoGitDir = errors.New("refutation records need a git repository")

// refutationsDir is the absolute record directory inside the git directory.
func refutationsDir(root string) (string, error) {
	out, err := git(root, "rev-parse", "--git-path", refutationsGitPath)
	if err != nil {
		return "", errNoGitDir
	}
	dir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	return dir, nil
}

func saveRefutationRecord(root string, c Candidate) error {
	dir, err := refutationsDir(root)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(refutationRecord{ID: c.ID, Digest: candidateDigest(c), DraftedBy: c.DraftedBy, Refutations: c.Refutations}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return writeFile(dir, c.ID+".json", append(data, '\n'))
}

// verifiedRefutations returns the refutations this machine ran for exactly
// this candidate content, or an error saying how to get them.
func verifiedRefutations(root string, c Candidate) (Candidate, error) {
	dir, err := refutationsDir(root)
	if err != nil {
		return c, err
	}
	data, err := readFile(dir, c.ID+".json")
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

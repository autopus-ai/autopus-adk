package generate

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
	"github.com/insajin/autopus-adk/pkg/qa/testscenario"
)

// WriteCandidates writes accepted documents into the candidate directories:
// scenarios as <id>.yaml, the test-scenarios document as <SPEC-ID>.yaml. It
// returns the project-relative, slash-separated path of every candidate that
// now holds an accepted document.
//
// An existing candidate is never overwritten. Identical content is a no-op;
// different content is skipped and reported as qa_generate_candidate_conflict
// after every other document has been written, so one stale candidate does not
// throw away the rest of an agent run.
func WriteCandidates(projectDir string, out Outcome) ([]string, error) {
	written := []string{}
	var conflicts []string
	write := func(rel string, accepted Accepted) error {
		if !safeName(accepted.ID) {
			return &Error{Code: CodeCandidateInvalid, Message: fmt.Sprintf("candidate id %q cannot be a file name", accepted.ID)}
		}
		rel = filepath.Join(rel, accepted.ID+".yaml")
		switch err := createOnce(filepath.Join(projectDir, rel), []byte(accepted.Body)); {
		case errors.Is(err, errConflict):
			conflicts = append(conflicts, filepath.ToSlash(rel))
		case err != nil:
			return err
		default:
			written = append(written, filepath.ToSlash(rel))
		}
		return nil
	}
	for _, accepted := range out.Scenarios {
		if err := write(scenario.CandidatesDirRel, accepted); err != nil {
			return written, err
		}
	}
	for _, accepted := range out.TestScenarios {
		if err := write(testscenario.CandidatesDirRel, accepted); err != nil {
			return written, err
		}
	}
	if len(conflicts) > 0 {
		return written, &Error{Code: CodeCandidateConflict, Message: fmt.Sprintf(
			"a different candidate already exists at %s; promote or delete it, then generate again",
			strings.Join(conflicts, ", "))}
	}
	return written, nil
}

var errConflict = errors.New("candidate exists with different content")

// createOnce creates path with body unless it exists. O_EXCL closes the window
// between the content check and the write, so a concurrent writer is never
// clobbered either.
func createOnce(path string, body []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		current, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Equal(current, body) {
			return nil
		}
		return errConflict
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

func safeName(id string) bool {
	id = strings.TrimSpace(id)
	return id != "" && id != "." && id != ".." && !strings.ContainsAny(id, `/\`) && !strings.Contains(id, "..")
}

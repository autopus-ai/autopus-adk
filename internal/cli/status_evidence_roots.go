package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// stateRootCandidate is one directory whose pipeline state may hold a SPEC's
// evidence, with what was found there.
type stateRootCandidate struct {
	root       string
	specID     string
	checkpoint *pipeline.Checkpoint
	loadErr    error
	latest     time.Time
	found      bool
}

func (c stateRootCandidate) stateDir() string { return filepath.Join(c.root, pipelineStateDir) }

func (c stateRootCandidate) checkpointPath() string {
	return filepath.Join(c.stateDir(), c.specID+".yaml")
}

// stateRoots picks where a SPEC's last pipeline evidence lives. Candidates are
// the root holding its .autopus/specs and, when no other scanned SPEC shares
// its ID, the status directory, where a run started from a workspace root
// keeps its state. The candidate with the newest evidence wins; the other
// roots holding evidence are named so older evidence is never silently used
// or silently hidden (REQ-SEV-EXEC-05).
func (r *statusEvidenceReader) stateRoots(entry specEntry) (stateRootCandidate, []string) {
	moduleRoot := statusAbsDir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(entry.path)))))
	roots := []string{moduleRoot}
	if moduleRoot != r.baseDir && r.idCount[entry.id] == 1 {
		roots = append(roots, r.baseDir)
	}
	var found []stateRootCandidate
	for _, root := range roots {
		if candidate := loadStateRootCandidate(root, entry.id); candidate.found {
			found = append(found, candidate)
		}
	}
	if len(found) == 0 {
		return stateRootCandidate{root: moduleRoot, specID: entry.id}, nil
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].latest.After(found[j].latest) })
	var others []string
	for _, candidate := range found[1:] {
		others = append(others, candidate.root)
	}
	return found[0], others
}

// loadStateRootCandidate reads a root's checkpoint and dates its newest
// evidence: run and baseline times recorded in the files, else the times the
// files were written.
func loadStateRootCandidate(root, specID string) stateRootCandidate {
	candidate := stateRootCandidate{root: root, specID: specID}
	note := func(at time.Time) {
		candidate.found = true
		if at.After(candidate.latest) {
			candidate.latest = at
		}
	}
	if info, err := os.Stat(candidate.checkpointPath()); err == nil {
		note(info.ModTime())
		cp, loadErr := pipeline.LoadFile(candidate.checkpointPath())
		candidate.checkpoint, candidate.loadErr = cp, loadErr
		if loadErr == nil && cp.Receipt != nil {
			note(cp.Receipt.StartedAt)
			if cp.Receipt.FinishedAt != nil {
				note(*cp.Receipt.FinishedAt)
			}
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		candidate.found, candidate.loadErr = true, err
	}
	baselinePath := pipelineVerificationPath(candidate.stateDir(), specID)
	if info, err := os.Stat(baselinePath); err == nil {
		note(info.ModTime())
		var baseline struct {
			RecordedAt time.Time `json:"recorded_at"`
		}
		if data, readErr := os.ReadFile(baselinePath); readErr == nil && json.Unmarshal(data, &baseline) == nil {
			note(baseline.RecordedAt)
		}
	}
	return candidate
}

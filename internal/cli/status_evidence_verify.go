package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// Freshness states of a piece of evidence against what it judged.
const (
	freshnessFresh   = "fresh"
	freshnessStale   = "stale"
	freshnessUnknown = "unknown"
	freshnessNone    = "none"
)

// statusVerification is the last completed run's verification measured
// against the code and SPEC as they are now (REQ-SEV-BASE-02).
type statusVerification struct {
	State      string   `json:"state"`
	Reasons    []string `json:"reasons,omitempty"`
	RunID      string   `json:"run_id,omitempty"`
	BaseHead   string   `json:"base_head,omitempty"`
	RecordedAt string   `json:"recorded_at,omitempty"`
}

// treeDigestCache shares one working-tree fingerprint per repository and base
// commit across the SPECs one status run checks.
type treeDigestCache map[string]treeDigestResult

type treeDigestResult struct {
	paths map[string]string
	err   error
}

func (c treeDigestCache) digests(dir, base string) (map[string]string, error) {
	top, err := gitText(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	key := top + "\x00" + base
	if cached, ok := c[key]; ok {
		return cached.paths, cached.err
	}
	paths, err := workingTreeDigests(top, base)
	c[key] = treeDigestResult{paths: paths, err: err}
	return paths, err
}

// checkPipelineVerification compares the recorded baseline with the code and
// SPEC now. latest is the receipt of the last run that dispatched anything; a
// baseline recorded by another run cannot vouch for a completed latest run.
func checkPipelineVerification(
	root, stateDir, specID, specDir string,
	latest *pipeline.OrchestrationRunReceipt,
	cache treeDigestCache,
) statusVerification {
	completed := latest != nil && latest.Terminal == pipeline.TerminalCompleted
	data, err := os.ReadFile(pipelineVerificationPath(stateDir, specID))
	if errors.Is(err, fs.ErrNotExist) {
		if completed {
			return statusVerification{
				State: freshnessUnknown, Reasons: []string{"no verification baseline"}, RunID: statusSingleLine(latest.RunID),
			}
		}
		return statusVerification{State: freshnessNone}
	}
	if err != nil {
		return unknownVerification(statusSingleLine("verification baseline unreadable: " + err.Error()))
	}
	var baseline pipelineVerificationBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return unknownVerification(statusSingleLine("verification baseline unreadable: " + err.Error()))
	}
	if err := baseline.validate(specID); err != nil {
		return unknownVerification(statusSingleLine("verification baseline is invalid: " + err.Error()))
	}
	verification := statusVerification{
		RunID: statusSingleLine(baseline.RunID), BaseHead: baseline.BaseHead,
		RecordedAt: baseline.RecordedAt.UTC().Format(time.RFC3339),
	}
	if completed && latest.RunID != baseline.RunID {
		return verification.unknown(statusSingleLine(fmt.Sprintf(
			"verification baseline belongs to run %s, not the last completed run %s", baseline.RunID, latest.RunID)))
	}
	specFingerprint, err := specRequirementsFingerprint(specDir)
	if err != nil {
		return verification.unknown(statusSingleLine("SPEC documents unreadable: " + err.Error()))
	}
	var reasons []string
	if specFingerprint != baseline.SpecFingerprint {
		reasons = append(reasons, "SPEC documents changed since verification")
	}
	current, treeErr := cache.digests(root, baseline.BaseHead)
	if treeErr == nil {
		if changed := changedTreePaths(baseline.TreePaths, current); len(changed) > 0 {
			reasons = append(reasons, statusSingleLine("working tree changed since verification: "+summarizeStatusPaths(changed)))
		}
	}
	switch {
	case len(reasons) > 0:
		verification.State, verification.Reasons = freshnessStale, reasons
	case treeErr != nil:
		return verification.unknown(statusSingleLine("working tree cannot be compared: " + treeErr.Error()))
	default:
		verification.State = freshnessFresh
	}
	return verification
}

func unknownVerification(reason string) statusVerification {
	return statusVerification{State: freshnessUnknown, Reasons: []string{reason}}
}

func (v statusVerification) unknown(reason string) statusVerification {
	v.State, v.Reasons = freshnessUnknown, []string{reason}
	return v
}

// changedTreePaths lists the paths whose digest differs between two tree
// snapshots, including paths only one of them has.
func changedTreePaths(recorded, current map[string]string) []string {
	var changed []string
	for rel, digest := range current {
		if recorded[rel] != digest {
			changed = append(changed, rel)
		}
	}
	for rel := range recorded {
		if _, ok := current[rel]; !ok {
			changed = append(changed, rel)
		}
	}
	sort.Strings(changed)
	return changed
}

// summarizeStatusPaths names the first three paths and counts the rest.
func summarizeStatusPaths(paths []string) string {
	const shown = 3
	if len(paths) <= shown {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s (+%d more)", strings.Join(paths[:shown], ", "), len(paths)-shown)
}

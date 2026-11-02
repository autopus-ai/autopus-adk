package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// pipelineVerificationSchema names the record a completed run leaves next to
// its checkpoint so `auto status` can tell whether what the run verified still
// matches the code (SPEC-STATUSEVIDENCE-001 REQ-SEV-BASE-01).
const pipelineVerificationSchema = "pipeline_verification_baseline.v1"

// pipelineVerificationBaseline is what a completed run verified: the SPEC
// requirements and every working-tree path that differed from base_head.
// Paths under .autopus are harness state, not code; the SPEC documents among
// them are covered by spec_fingerprint.
type pipelineVerificationBaseline struct {
	Schema          string            `json:"schema"`
	SpecID          string            `json:"spec_id"`
	RunID           string            `json:"run_id"`
	TaskID          string            `json:"task_id,omitempty"`
	BaseHead        string            `json:"base_head"`
	SpecFingerprint string            `json:"spec_fingerprint"`
	TreeFingerprint string            `json:"tree_fingerprint"`
	TreePaths       map[string]string `json:"tree_paths"`
	RecordedAt      time.Time         `json:"recorded_at"`
}

var (
	gitObjectID   = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)
	sha256Digest  = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	treePathValue = regexp.MustCompile(`^(deleted|dir|sha256:[0-9a-f]{64}(:x)?|symlink:[0-9a-f]{64}|` +
		`commit:[0-9a-f]{40}([0-9a-f]{24})?|special:[^\x00-\x1f]+)$`)
)

func pipelineVerificationPath(stateDir, specID string) string {
	return filepath.Join(stateDir, specID+".verification.json")
}

// validate reports why a parsed baseline cannot vouch for anything: a field
// is missing or malformed, it names another SPEC, or its tree paths do not
// add up to its tree fingerprint (REQ-SEV-BASE-02).
func (b pipelineVerificationBaseline) validate(specID string) error {
	switch {
	case b.Schema != pipelineVerificationSchema:
		return fmt.Errorf("unrecognized schema %q", b.Schema)
	case b.SpecID != specID:
		return fmt.Errorf("it names %q", b.SpecID)
	case b.RunID == "":
		return errors.New("run_id is missing")
	case !gitObjectID.MatchString(b.BaseHead):
		return errors.New("base_head is not a commit id")
	case !sha256Digest.MatchString(b.SpecFingerprint):
		return errors.New("spec_fingerprint is not a sha256 digest")
	case !sha256Digest.MatchString(b.TreeFingerprint):
		return errors.New("tree_fingerprint is not a sha256 digest")
	case b.TreePaths == nil:
		return errors.New("tree_paths is missing")
	case b.RecordedAt.IsZero():
		return errors.New("recorded_at is missing")
	}
	for rel, digest := range b.TreePaths {
		if rel == "" || harnessStatePath(rel) || !treePathValue.MatchString(digest) {
			return fmt.Errorf("tree_paths entry %q is malformed", rel)
		}
	}
	if treeFingerprint(b.TreePaths) != b.TreeFingerprint {
		return errors.New("tree_fingerprint does not match tree_paths")
	}
	return nil
}

// recordPipelineVerification writes the baseline of a completed run. Failing
// to write it is reported, never fatal: the run itself completed, and status
// reads a missing baseline as unknown, not as verified.
func recordPipelineVerification(w io.Writer, specID, specDir string, result *pipeline.PipelineResult) {
	if result == nil || result.Receipt.Terminal != pipeline.TerminalCompleted {
		return
	}
	baseline, err := newPipelineVerificationBaseline(".", specID, specDir, result.Receipt)
	path := pipelineVerificationPath(pipelineStateDir, specID)
	if err == nil {
		err = writePipelineVerification(path, baseline)
	}
	if err != nil {
		fmt.Fprintf(w, "Pipeline verification baseline not recorded: %v\n", err)
		return
	}
	fmt.Fprintf(w, "Pipeline verification baseline: %s (HEAD %s)\n", filepath.ToSlash(path), shortCommit(baseline.BaseHead))
}

func newPipelineVerificationBaseline(
	dir, specID, specDir string,
	receipt pipeline.OrchestrationRunReceipt,
) (pipelineVerificationBaseline, error) {
	head, err := gitHeadAt(dir)
	if err != nil {
		return pipelineVerificationBaseline{}, fmt.Errorf("read git HEAD: %w", err)
	}
	specFingerprint, err := specRequirementsFingerprint(specDir)
	if err != nil {
		return pipelineVerificationBaseline{}, fmt.Errorf("fingerprint SPEC documents: %w", err)
	}
	paths, err := workingTreeDigests(dir, head)
	if err != nil {
		return pipelineVerificationBaseline{}, fmt.Errorf("fingerprint working tree: %w", err)
	}
	return pipelineVerificationBaseline{
		Schema: pipelineVerificationSchema, SpecID: specID,
		RunID: receipt.RunID, TaskID: receipt.TaskID, BaseHead: head,
		SpecFingerprint: specFingerprint, TreeFingerprint: treeFingerprint(paths),
		TreePaths: paths, RecordedAt: time.Now().UTC(),
	}, nil
}

// writePipelineVerification replaces the baseline atomically with mode 0600:
// a failed write leaves the previous baseline as it was.
func writePipelineVerification(path string, baseline pipelineVerificationBaseline) error {
	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".verification-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

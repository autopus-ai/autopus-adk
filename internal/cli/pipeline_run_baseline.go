package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// pipelineVerificationSchema names the record a completed run leaves next to
// its checkpoint so `auto status` can tell whether what the run verified still
// matches the code (SPEC-STATUSEVIDENCE-001 REQ-SEV-BASE-01).
const pipelineVerificationSchema = "pipeline_verification_baseline.v1"

// A tree the fingerprint cannot hash in bounded time is reported as unknown,
// never as unchanged.
const (
	treeFingerprintMaxPaths = 5000
	treeFingerprintMaxBytes = int64(1) << 30
)

var errTreeTooLarge = errors.New("too many changed files to fingerprint")

// verificationSpecDocuments are the SPEC documents a run is bound to, the same
// four the pipeline snapshot hashes.
var verificationSpecDocuments = []string{"spec.md", "plan.md", "acceptance.md", "research.md"}

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

func pipelineVerificationPath(stateDir, specID string) string {
	return filepath.Join(stateDir, specID+".verification.json")
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

// writePipelineVerification replaces the baseline atomically with mode 0600.
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

// specRequirementsFingerprint hashes the SPEC documents without their status
// lines: review promotion and sync rewrite those without changing a
// requirement, and a status edit must not make verified code look stale.
func specRequirementsFingerprint(specDir string) (string, error) {
	hash := sha256.New()
	for _, name := range verificationSpecDocuments {
		body, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && name != "spec.md" {
				continue
			}
			return "", err
		}
		_, _ = hash.Write([]byte(name))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(withoutStatusLines(string(body))))
		_, _ = hash.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

// withoutStatusLines drops the status lines of a document's header, which is
// everything before its first H2 heading, frontmatter included.
func withoutStatusLines(body string) string {
	lines := strings.Split(body, "\n")
	kept := make([]string, 0, len(lines))
	header := true
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			header = false
		}
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if header && (strings.HasPrefix(trimmed, "**status**") || strings.HasPrefix(trimmed, "status:")) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// workingTreeDigests returns a digest for every path whose working-tree
// content differs from base or that is untracked and not ignored, keyed by
// its path from the repository root. Renames are listed as both sides, so a
// deleted file cannot hide behind the file that replaced it. Committing the
// tree leaves the result unchanged; any later content change alters it.
func workingTreeDigests(dir, base string) (map[string]string, error) {
	top, err := gitText(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	changed, err := gitBytes(top, "diff", "--name-only", "--no-renames", "-z", base, "--")
	if err != nil {
		return nil, fmt.Errorf("diff against %s: %w", shortCommit(base), err)
	}
	untracked, err := gitBytes(top, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	set := map[string]bool{}
	for _, raw := range bytes.Split(append(changed, untracked...), []byte{0}) {
		if rel := string(raw); rel != "" && !harnessStatePath(rel) {
			set[rel] = true
		}
	}
	if len(set) > treeFingerprintMaxPaths {
		return nil, fmt.Errorf("%w: %d paths", errTreeTooLarge, len(set))
	}
	budget := treeFingerprintMaxBytes
	digests := make(map[string]string, len(set))
	for rel := range set {
		digest, err := pathDigest(top, rel, &budget)
		if err != nil {
			return nil, err
		}
		digests[rel] = digest
	}
	return digests, nil
}

// harnessStatePath reports a path inside any .autopus directory.
func harnessStatePath(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if part == ".autopus" {
			return true
		}
	}
	return false
}

func pathDigest(top, rel string, budget *int64) (string, error) {
	full := filepath.Join(top, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return "deleted", nil
	}
	if err != nil {
		return "", err
	}
	mode := info.Mode()
	switch {
	case mode&fs.ModeSymlink != 0:
		target, err := os.Readlink(full)
		if err != nil {
			return "", err
		}
		return "symlink:" + digestBytes([]byte(target)), nil
	case mode.IsDir():
		// A submodule: its checked-out commit stands for its content.
		if head, err := gitText(full, "rev-parse", "HEAD"); err == nil {
			return "commit:" + head, nil
		}
		return "dir", nil
	case !mode.IsRegular():
		return "special:" + mode.Type().String(), nil
	}
	if *budget -= info.Size(); *budget < 0 {
		return "", fmt.Errorf("%w: changed files exceed %d bytes", errTreeTooLarge, treeFingerprintMaxBytes)
	}
	file, err := os.Open(full)
	if err != nil {
		return "", err
	}
	defer file.Close() //nolint:errcheck
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	digest := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	if mode&0o111 != 0 {
		digest += ":x"
	}
	return digest, nil
}

func treeFingerprint(paths map[string]string) string {
	keys := make([]string, 0, len(paths))
	for rel := range paths {
		keys = append(keys, rel)
	}
	sort.Strings(keys)
	hash := sha256.New()
	for _, rel := range keys {
		_, _ = hash.Write([]byte(rel + "\x00" + paths[rel] + "\x00"))
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil))
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func gitHeadAt(dir string) (string, error) {
	return gitText(dir, "rev-parse", "--verify", "HEAD^{commit}")
}

func gitText(dir string, args ...string) (string, error) {
	out, err := gitBytes(dir, args...)
	return strings.TrimSpace(string(out)), err
}

// gitBytes runs a read-only git command. GIT_OPTIONAL_LOCKS=0 keeps git from
// refreshing the index as a side effect of reading it.
func gitBytes(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("git %s: %s", args[0], strings.SplitN(msg, "\n", 2)[0])
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

func shortCommit(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

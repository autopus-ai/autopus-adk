package harneval

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/evalregression"
)

// BindingSchemaV1 identifies the harness_eval_binding.v1 document
// (SPEC-HARNEVAL-003 REQ-HR-04).
const BindingSchemaV1 = "harness_eval_binding.v1"

// Attestation environments of the harness lane (REQ-HR-03).
const (
	LaneSourceEnvironment = "adk-harness-live"
	LaneTargetEnvironment = "adk-release"
)

// RunnerTreeRoots are the repository-relative trees whose files decide a
// signed result: the trusted runner, its grader preparation, sandbox profiles
// and surface driver; the verdict, records, derivation, protocol, report and
// release-check code; and the trusted oracle harness.
var RunnerTreeRoots = []string{"scripts/benchmarks/harness", "pkg/harneval", "cmd/harneval-oracle"}

// tagNamePattern is the baseline tag names GitTagCommit resolves: no
// revision syntax (^ ~ : @ space) can reach git, and none starts with "-".
var tagNamePattern = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._/+-]{0,127}$`)

// Binding is the harness_eval_binding.v1 document. Its digest is both the
// attestation source_revision and the report attributed_version, so a change
// to any field invalidates the evidence of the old binding.
type Binding struct {
	SchemaVersion          string         `json:"schema_version"`
	CandidateSurfaceDigest string         `json:"candidate_surface_digest"`
	AgentSetDigest         string         `json:"agent_set_digest"`
	CorpusDigests          []CorpusDigest `json:"corpus_digests"`
	RunnerTreeDigest       string         `json:"runner_tree_digest"`
	Policy                 LivePolicy     `json:"policy"`
	Model                  string         `json:"model"`
	WorkspaceRevision      string         `json:"workspace_revision"`
	BaselineRef            string         `json:"baseline_ref"`
	BaselineCommit         string         `json:"baseline_commit"`
	SigningKeyID           string         `json:"signing_key_id"`
	Pins                   Pins           `json:"pins"`
}

// CorpusDigest is one corpus file an active agent task pins, in the shape of
// the protocol's corpus_digests rows.
type CorpusDigest struct {
	File       string `json:"file"`
	FileSHA256 string `json:"file_sha256"`
}

// Digest is the binding digest: the SHA-256 hex of the Go json.Marshal of
// the document.
func (b Binding) Digest() string { return sha256Hex(mustMarshal(b)) }

// BindingOptions are the seams of ComputeBinding. Env is the whole
// environment of every child process; the caller passes an allowlist.
// TagCommit resolves the baseline tag; nil selects GitTagCommit.
type BindingOptions struct {
	Adapters  AdapterFactory
	Env       []string
	TagCommit func(ctx context.Context, root, ref string, env []string) (string, error)
}

// ComputeBinding derives the binding of the trusted tree at root: the
// default candidate surface generated exactly as `auto eval harness digest`
// generates it, the agent set and corpus digests of the strictly loaded set,
// the runner tree digest, the manifest live policy and pins, the commit the
// baseline_ref tag points to, and the harness lane signing key id.
func ComputeBinding(ctx context.Context, root string, opts BindingOptions) (Binding, error) {
	set, err := LoadSet(root)
	if err != nil {
		return Binding{}, fmt.Errorf("binding: load golden set: %w", err)
	}
	tree, err := RunnerTreeDigest(root)
	if err != nil {
		return Binding{}, fmt.Errorf("binding: %w", err)
	}
	surface, err := defaultSurfaceDigest(ctx, set, opts.Adapters)
	if err != nil {
		return Binding{}, fmt.Errorf("binding: %w", err)
	}
	resolve := opts.TagCommit
	if resolve == nil {
		resolve = GitTagCommit
	}
	live := set.Manifest.Live
	commit, err := resolve(ctx, root, live.BaselineRef, opts.Env)
	if err != nil {
		return Binding{}, fmt.Errorf("binding: %w", err)
	}
	if !revisionPattern.MatchString(commit) {
		return Binding{}, fmt.Errorf("binding: baseline tag %s resolved to %q, not a 40-hex commit", live.BaselineRef, commit)
	}
	return Binding{
		SchemaVersion:          BindingSchemaV1,
		CandidateSurfaceDigest: surface,
		AgentSetDigest:         AgentSetDigest(set),
		CorpusDigests:          bindingCorpus(set),
		RunnerTreeDigest:       tree,
		Policy:                 live,
		Model:                  live.Model,
		WorkspaceRevision:      live.WorkspaceRevision,
		BaselineRef:            live.BaselineRef,
		BaselineCommit:         commit,
		SigningKeyID:           evalregression.ADKHarnessEvalKeyID,
		Pins:                   set.Manifest.Pins,
	}, nil
}

// defaultSurfaceDigest generates the set under the sentinel and digests the
// default-configuration surface.
func defaultSurfaceDigest(ctx context.Context, set *Set, factory AdapterFactory) (string, error) {
	generation, err := Generate(ctx, set, factory)
	if err != nil {
		return "", fmt.Errorf("generate surface: %w", err)
	}
	defer func() { _ = generation.Close() }()
	return SurfaceDigest(generation.Surfaces[""].Root)
}

// bindingCorpus is one row per corpus file the active agent tasks pin, by
// file, as the trusted runner writes the protocol's corpus_digests.
func bindingCorpus(set *Set) []CorpusDigest {
	byFile := map[string]string{}
	for _, task := range set.Tasks {
		if task.Kind == KindAgent && task.Status.State == StateActive && task.CorpusRef != nil {
			byFile[task.CorpusRef.File] = task.CorpusRef.FileSHA256
		}
	}
	rows := make([]CorpusDigest, 0, len(byFile))
	for file, sum := range byFile {
		rows = append(rows, CorpusDigest{File: file, FileSHA256: sum})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].File < rows[j].File })
	return rows
}

// RunnerTreeDigest hashes every file under RunnerTreeRoots below root: the
// SHA-256 hex of the sorted "relpath\x00sha256(content)\n" rows, relpath
// being repository-relative with slashes. Nothing is skipped, so an
// interpreter cache written into the tree moves the digest too; the runner
// sets PYTHONDONTWRITEBYTECODE=1 as CI does. A missing or symlinked root and
// a symlink or other non-regular file under a root are errors.
func RunnerTreeDigest(root string) (string, error) {
	var rows []string
	for _, rel := range RunnerTreeRoots {
		if err := lstatComponents(root, rel, true); err != nil {
			return "", fmt.Errorf("runner tree root %s: %w", rel, err)
		}
		base := filepath.Join(root, filepath.FromSlash(rel))
		err := filepath.WalkDir(base, func(current string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(base, current)
			if err != nil {
				return err
			}
			name := rel + "/" + filepath.ToSlash(relative)
			if !entry.Type().IsRegular() {
				return fmt.Errorf("runner tree file %s is not a regular file", name)
			}
			data, err := os.ReadFile(current)
			if err != nil {
				return err
			}
			rows = append(rows, name+"\x00"+sha256Hex(data)+"\n")
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("runner tree digest: %w", err)
		}
	}
	sort.Strings(rows)
	return sha256Hex([]byte(strings.Join(rows, ""))), nil
}

// GitTagCommit resolves tag ref in the repository at root to the 40-hex
// commit it points to, peeling an annotated tag. git runs under exactly env:
// a nil env is empty, never the parent's environment.
func GitTagCommit(ctx context.Context, root, ref string, env []string) (string, error) {
	if !tagNamePattern.MatchString(ref) || strings.Contains(ref, "..") {
		return "", fmt.Errorf("baseline tag %q is not a plain tag name", ref)
	}
	cmd := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/tags/"+ref+"^{commit}")
	cmd.Env = append([]string{}, env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("resolve baseline tag %s: %w: %s", ref, err, strings.TrimSpace(stderr.String()))
	}
	commit := strings.TrimSpace(string(out))
	if !revisionPattern.MatchString(commit) {
		return "", fmt.Errorf("baseline tag %s resolved to %q, not a 40-hex commit", ref, commit)
	}
	return commit, nil
}

// LanePolicy is the strict attestation policy of binding digest
// bindingDigest: the context the signer binds and the six expected values a
// verifier passes to `auto check --eval-regression`.
func LanePolicy(bindingDigest string) evalregression.EvalRegressionAttestationPolicyV2 {
	return evalregression.EvalRegressionAttestationPolicyV2{
		ExpectedKeyID:     evalregression.ADKHarnessEvalKeyID,
		TrustLane:         evalregression.ADKHarnessEvalTrustLane,
		SourceEnvironment: LaneSourceEnvironment,
		TargetEnvironment: LaneTargetEnvironment,
		SourceRevision:    bindingDigest,
		WorkspaceScope:    ReportWorkspaceScope,
	}
}

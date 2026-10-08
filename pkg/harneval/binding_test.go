package harneval

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/evalregression"
)

// Digests of the oracle documents below, computed outside Go with hashlib
// over hand-built rows and json.dumps(separators=(",", ":")) of the binding.
const (
	oracleRunnerTreeDigest = "3e39db11b4a4291a4f664f48bc713cef753169c590a218f85ad21cb3edecb8db"
	oracleBindingDigest    = "6e1b50c7cb559ed00eebfab05d3f93b5db1f281dbe058ed6d0848a96c30606fd"
)

// oracleRunnerTree holds one file under each runner tree root.
var oracleRunnerTree = map[string]string{
	"scripts/benchmarks/harness/golden.py": "print('golden')\n",
	"scripts/benchmarks/harness/grader.sb": "(version 1)\n",
	"pkg/harneval/verdict.go":              "package harneval\n",
	"cmd/harneval-oracle/main.go":          "package main\n",
}

func (f *fixture) runnerTree() {
	f.t.Helper()
	for rel, body := range oracleRunnerTree {
		f.write(rel, body)
	}
}

func TestRunnerTreeDigest_HashesEveryFileOfTheThreeRootsAndNothingElse(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.runnerTree()
	f.write("README.md", "outside every root\n")
	f.write("internal/cli/eval_harness_export.go", "package cli\n")

	got, err := RunnerTreeDigest(f.root)
	require.NoError(t, err)
	assert.Equal(t, oracleRunnerTreeDigest, got)
}

func TestRunnerTreeDigest_AnyFileUnderARootMovesTheDigest(t *testing.T) {
	t.Parallel()
	edits := map[string]string{
		"scripts/benchmarks/harness/golden.py":                          "print('golden!')\n",
		"scripts/benchmarks/harness/grader.sb":                          "(version 1)\n(deny network*)\n",
		"pkg/harneval/verdict.go":                                       "package harneval // edited\n",
		"cmd/harneval-oracle/main.go":                                   "package main\n\nfunc main() {}\n",
		"scripts/benchmarks/harness/artifact.sb":                        "(version 1)\n",
		"scripts/benchmarks/harness/__pycache__/golden.cpython-313.pyc": "bytecode",
	}
	for rel, body := range edits {
		f := newFixture(t)
		f.runnerTree()
		f.write(rel, body)
		got, err := RunnerTreeDigest(f.root)
		require.NoError(t, err, rel)
		assert.NotEqual(t, oracleRunnerTreeDigest, got, rel)
	}
}

func TestRunnerTreeDigest_RefusesARootOrFileItCannotHash(t *testing.T) {
	t.Parallel()
	cases := map[string]func(f *fixture){
		"missing oracle harness root": func(f *fixture) {
			require.NoError(t, os.RemoveAll(filepath.Join(f.root, "cmd")))
		},
		"symlinked root": func(f *fixture) {
			dir := filepath.Join(f.root, "pkg", "harneval")
			require.NoError(t, os.Rename(dir, filepath.Join(f.root, "elsewhere")))
			require.NoError(t, os.Symlink(filepath.Join(f.root, "elsewhere"), dir))
		},
		"symlink inside a root": func(f *fixture) {
			require.NoError(t, os.Symlink(filepath.Join(f.root, "README.md"), filepath.Join(f.root, "pkg", "harneval", "link.go")))
		},
	}
	for name, mutate := range cases {
		f := newFixture(t)
		f.runnerTree()
		f.write("README.md", "outside\n")
		mutate(f)
		_, err := RunnerTreeDigest(f.root)
		assert.Error(t, err, name)
	}
}

func TestBindingDigest_IsTheSHA256OfTheMarshaledDocument(t *testing.T) {
	t.Parallel()
	live := LivePolicy{K: 2, ThresholdBP: -1000, CompletenessFloor: 0.9, MaxAgentRuns: 96, TrialTimeoutSeconds: 180,
		WorkspaceRevision: strings.Repeat("a", 40), BaselineRef: "v0.50.122", Model: "gpt-test"}
	binding := Binding{
		SchemaVersion: BindingSchemaV1, CandidateSurfaceDigest: strings.Repeat("1", 64),
		AgentSetDigest: strings.Repeat("2", 64), RunnerTreeDigest: strings.Repeat("4", 64),
		CorpusDigests: []CorpusDigest{{File: "bench/corpus_a.json", FileSHA256: strings.Repeat("3", 64)}},
		Policy:        live, Model: "gpt-test", WorkspaceRevision: strings.Repeat("a", 40), BaselineRef: "v0.50.122",
		BaselineCommit: strings.Repeat("5", 40), SigningKeyID: "adk-harness-eval-2026-10",
		Pins: Pins{GeneratorVersion: "v0.50.123", ProjectName: "harneval-fixture", CodexCLIVersion: "codex-cli 0.160.0",
			OpencodeCLIVersion: "1.18.7"},
	}
	assert.Equal(t, oracleBindingDigest, binding.Digest())
}

// bindingTree is the standard set plus the runner tree roots.
func bindingTree(t *testing.T) *fixture {
	f := newFixture(t)
	f.standard()
	f.runnerTree()
	return f
}

func stubTagCommit(commit string) func(context.Context, string, string, []string) (string, error) {
	return func(context.Context, string, string, []string) (string, error) { return commit, nil }
}

func TestComputeBinding_TakesEveryFieldFromTheTrustedTree(t *testing.T) {
	t.Parallel()
	f := bindingTree(t)
	var gotRoot, gotRef string
	var gotEnv []string
	binding, err := ComputeBinding(context.Background(), f.root, BindingOptions{
		Adapters: fakeAdapters, Env: []string{"PATH=/usr/bin", "HOME=/nowhere"},
		TagCommit: func(_ context.Context, root, ref string, env []string) (string, error) {
			gotRoot, gotRef, gotEnv = root, ref, env
			return strings.Repeat("c", 40), nil
		},
	})
	require.NoError(t, err)

	set, err := LoadSet(f.root)
	require.NoError(t, err)
	generation, err := Generate(context.Background(), set, fakeAdapters)
	require.NoError(t, err)
	defer func() { _ = generation.Close() }()
	surface, err := SurfaceDigest(generation.Surfaces[""].Root)
	require.NoError(t, err)

	assert.Equal(t, Binding{
		SchemaVersion:          "harness_eval_binding.v1",
		CandidateSurfaceDigest: surface,
		AgentSetDigest:         AgentSetDigest(set),
		CorpusDigests:          []CorpusDigest{{File: "bench/corpus_a.json", FileSHA256: sha256Of(fixtureCorpus)}},
		RunnerTreeDigest:       oracleRunnerTreeDigest,
		Policy:                 set.Manifest.Live,
		Model:                  "gpt-test",
		WorkspaceRevision:      strings.Repeat("a", 40),
		BaselineRef:            "v0.50.122",
		BaselineCommit:         strings.Repeat("c", 40),
		SigningKeyID:           evalregression.ADKHarnessEvalKeyID,
		Pins:                   set.Manifest.Pins,
	}, binding)
	assert.Equal(t, f.root, gotRoot)
	assert.Equal(t, "v0.50.122", gotRef)
	assert.Equal(t, []string{"PATH=/usr/bin", "HOME=/nowhere"}, gotEnv, "the tag resolver runs under the caller's allowlist")
}

// TestComputeBinding_S5_EachEvaluationInputMovesTheDigest: the S5 inputs this
// layer owns each change the binding digest.
func TestComputeBinding_S5_EachEvaluationInputMovesTheDigest(t *testing.T) {
	t.Parallel()
	live := func(f *fixture, key string, value any) {
		manifest := validManifest()
		manifest["live"].(map[string]any)[key] = value
		f.writeJSON(ManifestPath, manifest)
	}
	cases := map[string]func(f *fixture, opts *BindingOptions){
		"threshold_bp":       func(f *fixture, _ *BindingOptions) { live(f, "threshold_bp", -500) },
		"workspace_revision": func(f *fixture, _ *BindingOptions) { live(f, "workspace_revision", strings.Repeat("b", 40)) },
		"baseline_ref":       func(f *fixture, _ *BindingOptions) { live(f, "baseline_ref", "v0.50.121") },
		"live.model":         func(f *fixture, _ *BindingOptions) { live(f, "model", "gpt-other") },
		"baseline tag moved": func(_ *fixture, opts *BindingOptions) { opts.TagCommit = stubTagCommit(strings.Repeat("d", 40)) },
		"corpus byte":        func(f *fixture, _ *BindingOptions) { rewriteCorpus(f, `[{"id":"a01","prompt":"fix it!"}]`+"\n") },
		"expected_tests": func(f *fixture, _ *BindingOptions) {
			task := agentTask("GT-AG-001")
			task["expected_tests"] = []any{"TestVersionMismatchWinsOverUnknown", "TestSelectExplicitEvidence"}
			f.writeJSON(agentPath("GT-AG-001"), task)
		},
		"golden.py line": func(f *fixture, _ *BindingOptions) { f.write("scripts/benchmarks/harness/golden.py", "print(1)\n") },
		"grader.sb line": func(f *fixture, _ *BindingOptions) {
			f.write("scripts/benchmarks/harness/grader.sb", "(deny default)\n")
		},
		"pkg/harneval verdict": func(f *fixture, _ *BindingOptions) { f.write("pkg/harneval/verdict.go", "package harneval // x\n") },
		"oracle harness":       func(f *fixture, _ *BindingOptions) { f.write("cmd/harneval-oracle/main.go", "package main // x\n") },
		"template line": func(_ *fixture, opts *BindingOptions) {
			opts.Adapters = func(root string, _ Pins, _ []byte) []adapter.PlatformAdapter {
				return []adapter.PlatformAdapter{fakeAdapter{name: "claude-code", root: root, files: map[string]string{routerPath: "router: plan go edited\n"}}}
			}
		},
	}
	base := BindingOptions{Adapters: fakeAdapters, TagCommit: stubTagCommit(strings.Repeat("c", 40))}
	reference, err := ComputeBinding(context.Background(), bindingTree(t).root, base)
	require.NoError(t, err)
	for name, mutate := range cases {
		f, opts := bindingTree(t), base
		mutate(f, &opts)
		binding, err := ComputeBinding(context.Background(), f.root, opts)
		require.NoError(t, err, name)
		assert.NotEqual(t, reference.Digest(), binding.Digest(), name)
	}
}

// rewriteCorpus changes the corpus file and the digest the agent task pins.
func rewriteCorpus(f *fixture, body string) {
	f.write("bench/corpus_a.json", body)
	task := agentTask("GT-AG-001")
	task["corpus_ref"].(map[string]any)["file_sha256"] = sha256Of(body)
	f.writeJSON(agentPath("GT-AG-001"), task)
}

func TestComputeBinding_RefusesAnUnresolvedBaselineOrAnUnreadableTree(t *testing.T) {
	t.Parallel()
	cases := map[string]func(f *fixture, opts *BindingOptions){
		"tag lookup fails": func(_ *fixture, opts *BindingOptions) {
			opts.TagCommit = func(context.Context, string, string, []string) (string, error) { return "", errors.New("no tag") }
		},
		"tag resolves to a non-commit":  func(_ *fixture, opts *BindingOptions) { opts.TagCommit = stubTagCommit("HEAD") },
		"golden set fails to load":      func(f *fixture, _ *BindingOptions) { f.write(ManifestPath, "{") },
		"oracle harness root is absent": func(f *fixture, _ *BindingOptions) { require.NoError(t, os.RemoveAll(filepath.Join(f.root, "cmd"))) },
	}
	for name, mutate := range cases {
		f, opts := bindingTree(t), BindingOptions{Adapters: fakeAdapters, TagCommit: stubTagCommit(strings.Repeat("c", 40))}
		mutate(f, &opts)
		_, err := ComputeBinding(context.Background(), f.root, opts)
		assert.Error(t, err, name)
	}
}

func TestLanePolicy_IsTheHarnessLaneAttestationContext(t *testing.T) {
	t.Parallel()
	binding := strings.Repeat("e", 64)
	assert.Equal(t, evalregression.EvalRegressionAttestationPolicyV2{
		ExpectedKeyID: evalregression.ADKHarnessEvalKeyID, TrustLane: "adk-harness-eval",
		SourceEnvironment: "adk-harness-live", TargetEnvironment: "adk-release",
		SourceRevision: binding, WorkspaceScope: "autopus-adk",
	}, LanePolicy(binding))
}

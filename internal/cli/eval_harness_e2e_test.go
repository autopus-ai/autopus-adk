//go:build darwin

package cli

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/evalregression"
	"github.com/insajin/autopus-adk/pkg/harneval"
)

// The S5 digest chain on the macOS CI job (SPEC-HARNEVAL-003 REQ-HR-04,
// REQ-HR-10). ci.yaml runs every TestEvalHarnessE2E_ test with a go test
// -list floor and compares the PASS set, so a skip fails the step.

// e2eGit runs git in the tree with a fixed identity and no user config.
func e2eGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=harneval", "GIT_AUTHOR_EMAIL=harneval@example.invalid",
		"GIT_COMMITTER_NAME=harneval", "GIT_COMMITTER_EMAIL=harneval@example.invalid"}
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

// binaryBinding is `auto eval harness digest --binding` of the separately
// built binary over root.
func binaryBinding(t *testing.T, auto, root string) string {
	t.Helper()
	cmd := exec.Command(auto, "eval", "harness", "digest", "--binding", "--format", "json", "--dir", root)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	var doc harnessBindingDoc
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	require.Regexp(t, `^[0-9a-f]{64}$`, doc.BindingDigest)
	return doc.BindingDigest
}

// binaryPolicy is `auto eval harness policy --binding B` of the binary.
func binaryPolicy(t *testing.T, auto, binding string) evalregression.EvalRegressionAttestationPolicyV2 {
	t.Helper()
	out, err := exec.Command(auto, "eval", "harness", "policy", "--binding", binding).Output()
	require.NoError(t, err)
	var doc harnessLanePolicyDoc
	require.NoError(t, json.Unmarshal(out, &doc))
	return evalregression.EvalRegressionAttestationPolicyV2{ExpectedKeyID: doc.ExpectedKeyID, TrustLane: doc.TrustLane,
		SourceEnvironment: doc.SourceEnvironment, TargetEnvironment: doc.TargetEnvironment,
		SourceRevision: doc.SourceRevision, WorkspaceScope: doc.WorkspaceScope}
}

// TestEvalHarnessE2E_S5_DigestChainReachesTheStrictCheck: the production
// export signs a verified session of a main checkout with its in-process
// binding; an auto built in another directory with another version computes
// the same binding B, which is the protocol's binding_digest, and its policy
// verifies the evidence as ok. Changing an evaluation input moves the
// binary's binding, and the old evidence then fails its policy.
func TestEvalHarnessE2E_S5_DigestChainReachesTheStrictCheck(t *testing.T) {
	world := newReconstructWorldWith(t, 1, evalHarnessDeps{})
	got, evidence, pub := world.export(t, world.sources)
	require.Equal(t, 0, got.code, got.stderr)
	auto := buildHarnessBinary(t, "v0.0.0-harneval-e2e")
	binding := binaryBinding(t, auto, world.root)
	require.Equal(t, world.digest, binding, "the separate binary computes the export's binding")
	protocol, err := os.ReadFile(filepath.Join(world.input, harneval.ProtocolFile))
	require.NoError(t, err)
	assert.Contains(t, string(protocol), `"binding_digest":"`+binding+`"`)
	assert.Equal(t, "eval-regression: ok (version="+binding+")\n", got.stdout)

	report := filepath.Join(evidence, harnessEvidenceReport)
	at := reconstructStart.Add(31 * time.Hour)
	trusted := map[string]ed25519.PublicKey{evalregression.ADKHarnessEvalKeyID: pub}
	assert.Equal(t, "eval-regression: ok (version="+binding+")\n", strictLine(t, report, at, trusted, binaryPolicy(t, auto, binding)))

	manifest := filepath.Join(world.root, filepath.FromSlash(harneval.ManifestPath))
	edit := func(path, old, replacement string) func() {
		return func() {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, 1, bytes.Count(data, []byte(old)), "%s holds %q once", path, old)
			require.NoError(t, os.WriteFile(path, bytes.Replace(data, []byte(old), []byte(replacement), 1), 0o644))
		}
	}
	corpus := filepath.Join(world.root, "bench", "corpus_a.json")
	agent := filepath.Join(world.root, filepath.FromSlash(harnessAgentPath))
	changes := []struct {
		name  string
		apply func()
	}{
		{"threshold_bp", edit(manifest, `"threshold_bp": -1000`, `"threshold_bp": -900`)},
		{"workspace_revision", edit(manifest, strings.Repeat("a", 40), strings.Repeat("b", 40))},
		{"live.model", edit(manifest, `"model": "gpt-test"`, `"model": "gpt-other"`)},
		{"runner tree file", edit(filepath.Join(world.root, "pkg", "harneval", "verdict.go"), "package harneval\n", "package harneval\n\n// changed\n")},
		{"expected_tests", edit(agent, `"TestVersionMismatchWinsOverUnknown"`, `"TestVersionMismatchWinsOverUnknownToo"`)},
		{"corpus byte and file_sha256", func() {
			edit(corpus, "fix it", "fix It")()
			edit(agent, sha256String([]byte(harnessCorpus)), sha256String([]byte(strings.Replace(harnessCorpus, "fix it", "fix It", 1))))()
		}},
		{"baseline tag moved", func() {
			e2eGit(t, world.root, "commit", "-q", "--allow-empty", "-m", "another")
			e2eGit(t, world.root, "tag", "-f", "v0.50.122")
		}},
		{"task oracle_mode black_box", func() { e2eBlackBox(t, world.root, "two rows\n") }},
		{"black-box expected output byte and sha256", func() { e2eBlackBox(t, world.root, "two rowS\n") }},
	}
	for _, change := range changes {
		change.apply()
		moved := binaryBinding(t, auto, world.root)

		assert.NotEqual(t, binding, moved, change.name)
		assert.Equal(t, "eval-regression: attestation_policy_mismatch\n",
			strictLine(t, report, at, trusted, binaryPolicy(t, auto, moved)), change.name)
		binding = moved
	}
}

// e2eBlackBox makes GT-AG-001 a black-box task whose stdout expectation is
// the committed fixture holding stdout, pinned by its sha256.
func e2eBlackBox(t *testing.T, root, stdout string) {
	t.Helper()
	fixture := harneval.OracleFixtureRoot + "/GT-AG-001/stdout.txt"
	writeReconstructFile(t, filepath.Join(root, filepath.FromSlash(fixture)), []byte(stdout))
	path := filepath.Join(root, filepath.FromSlash(harnessAgentPath))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var task map[string]any
	require.NoError(t, json.Unmarshal(data, &task))
	task["oracle_mode"] = harneval.OracleModeBlackBox
	task["black_box_oracle"] = map[string]any{"build": "./cmd/auto", "command": []string{"{artifact}", "version"},
		"inputs": []any{}, "assertions": []any{map[string]any{"id": "stdout", "kind": harneval.BlackBoxStdout,
			"expected": map[string]any{"path": fixture, "sha256": sha256String([]byte(stdout))}}}}
	data, err = json.MarshalIndent(task, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

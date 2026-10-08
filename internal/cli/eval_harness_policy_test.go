package cli

import (
	"bytes"
	"context"
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

// TestEvalHarnessDigest_Binding_PrintsTheTrustedBindingAndItsDigest is
// serial: it starts git and generates. The parent's GIT_DIR points nowhere,
// so the tag lookup proves it ran under the allowlisted environment.
func TestEvalHarnessDigest_Binding_PrintsTheTrustedBindingAndItsDigest(t *testing.T) {
	tree := exportGitTree(t)
	revParse := exec.Command("git", "-C", tree.root, "rev-parse", "v0.50.122^{commit}")
	commit, err := revParse.Output()
	require.NoError(t, err)
	t.Setenv("GIT_DIR", filepath.Join(tree.root, "no-such-git-dir"))
	deps := harnessDeps(harnessRouter)

	got := runHarness(t, deps, "digest", "--binding", "--dir", tree.root)
	require.Equal(t, 0, got.code, got.stderr)
	var doc struct {
		BindingDigest string           `json:"binding_digest"`
		Binding       harneval.Binding `json:"binding"`
	}
	decoder := json.NewDecoder(strings.NewReader(got.stdout))
	decoder.DisallowUnknownFields()
	require.NoError(t, decoder.Decode(&doc))

	want, err := harneval.ComputeBinding(context.Background(), tree.root, harneval.BindingOptions{Adapters: deps.run.Adapters, Env: harnessChildEnv(os.Environ())})
	require.NoError(t, err)
	assert.Equal(t, want, doc.Binding)
	assert.Equal(t, want.Digest(), doc.BindingDigest)
	assert.Regexp(t, `^[0-9a-f]{64}$`, doc.BindingDigest)
	assert.Equal(t, strings.TrimSpace(string(commit)), doc.Binding.BaselineCommit)
	assert.Equal(t, evalregression.ADKHarnessEvalKeyID, doc.Binding.SigningKeyID)

	plain := harnessDoc(t, runHarness(t, deps, "digest", "--dir", tree.root).stdout)
	assert.Contains(t, plain, "set_digest", "without --binding the set digests are unchanged")
	assert.NotContains(t, plain, "binding_digest")

	require.NoError(t, os.Rename(filepath.Join(tree.root, ".git"), filepath.Join(t.TempDir(), "moved-git")))
	untagged := runHarness(t, deps, "digest", "--binding", "--dir", tree.root)
	assert.Equal(t, 1, untagged.code, "an unresolvable baseline tag has no binding, and git's exit status does not leak")
	assert.Contains(t, untagged.stderr, "Error: harness binding: binding: resolve baseline tag v0.50.122")
	assert.Empty(t, untagged.stdout)
}

func TestEvalHarnessPolicy_PrintsTheStrictPolicyTheEvidenceVerifiesWith(t *testing.T) {
	t.Parallel()
	pub, _, key := exportKey(t)
	binding := exportBinding().Digest()
	out := filepath.Join(t.TempDir(), "evidence")
	signed := runExport(t, exportSeams(t, exportSession("pass", "pass"), pub), key, exportArgs(out)...)
	require.Equal(t, 0, signed.code, signed.stderr)

	got := runHarness(t, evalHarnessDeps{}, "policy", "--binding", binding)
	require.Equal(t, 0, got.code, got.stderr)
	assert.Equal(t, map[string]any{
		"expected_key_id": evalregression.ADKHarnessEvalKeyID, "trust_lane": "adk-harness-eval",
		"source_environment": "adk-harness-live", "target_environment": "adk-release",
		"source_revision": binding, "workspace_scope": "autopus-adk",
	}, harnessDoc(t, got.stdout))

	var printed struct {
		ExpectedKeyID     string `json:"expected_key_id"`
		TrustLane         string `json:"trust_lane"`
		SourceEnvironment string `json:"source_environment"`
		TargetEnvironment string `json:"target_environment"`
		SourceRevision    string `json:"source_revision"`
		WorkspaceScope    string `json:"workspace_scope"`
	}
	require.NoError(t, json.Unmarshal([]byte(got.stdout), &printed))
	policy := evalregression.EvalRegressionAttestationPolicyV2(printed)
	var line bytes.Buffer
	report := filepath.Join(out, harnessEvidenceReport)
	assert.True(t, checkEvalRegressionStrict("", report, deriveEvalRegressionAttestationPath(report), 72*time.Hour,
		exportStarted(t).Add(time.Hour), map[string]ed25519.PublicKey{evalregression.ADKHarnessEvalKeyID: pub}, policy, &line, false, false))
	assert.Equal(t, "eval-regression: ok (version="+binding+")\n", line.String(), "the printed policy verifies the signed evidence")
}

func TestEvalHarnessPolicy_RefusesAValueThatIsNotABindingDigest(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		{"policy"}, {"policy", "--binding", ""}, {"policy", "--binding", strings.Repeat("A", 64)},
		{"policy", "--binding", strings.Repeat("a", 40)}, {"policy", "--binding", strings.Repeat("a", 64) + "\n"},
		{"policy", "--binding", strings.Repeat("a", 64), "--format", "text"},
	} {
		got := runHarness(t, evalHarnessDeps{}, args...)
		assert.Equal(t, 1, got.code, "%q", args)
		assert.Empty(t, got.stdout, "%q", args)
		assert.Contains(t, got.stderr, "Error: ", "%q", args)
	}
}

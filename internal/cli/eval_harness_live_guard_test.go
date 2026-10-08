package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// The guards of the signed live lane before and around its session
// (SPEC-HARNEVAL-003 REQ-HR-01, REQ-HR-07, REQ-HR-10): the bound check counts
// only a bound attestation the pinned workflow signed on main, a trusted step
// fills a fresh module cache that the runner and the export read as a
// file:// proxy, and the hosted sandbox preflight fails closed until T14.
const (
	liveSignerWorkflow = "autopus-ai/autopus-adk/.github/workflows/harness-eval-live.yml"
	liveModuleProxy    = `--proxy "file://$RUNNER_TEMP/modules/cache/download"`
	liveFillModules    = `scripts/benchmarks/harness/fill_modules.sh "$RUNNER_TEMP/modules"`
)

// liveBoundVerify is what the bound check must pass to gh attestation verify.
var liveBoundVerify = []string{`gh attestation verify "$RUNNER_TEMP/run-key"`, "--repo autopus-ai/autopus-adk",
	"--signer-workflow " + liveSignerWorkflow, "--source-ref refs/heads/main",
	"--predicate-type https://autopus.ai/harness-eval/bound/v1", "--deny-self-hosted-runners"}

func liveStepIndex(job liveJob, name string) int {
	for index, step := range job.Steps {
		if step.Name == name {
			return index
		}
	}
	return -1
}

// liveGuardViolations lists every guard rule the jobs break.
func liveGuardViolations(jobs map[string]liveJob) []string {
	var bad []string
	eval, sign := jobs["live-eval"], jobs["sign"]
	golden, export := liveStepIndex(eval, "golden-session"), liveStepIndex(sign, "export")
	bound, preflight, modules := liveStepIndex(eval, "bound-check"), liveStepIndex(eval, "sandbox-preflight"), liveStepIndex(eval, "modules")
	if golden < 0 || bound < 0 || bound > golden {
		return append(bad, "live-eval needs bound-check before golden-session")
	}
	script := eval.Steps[bound].Run
	for _, flag := range liveBoundVerify {
		if !strings.Contains(script, flag) {
			bad = append(bad, "bound-check must verify the bound attestation with "+flag)
		}
	}
	if strings.Contains(script, "gh api") {
		bad = append(bad, "bound-check must not count attestations from gh api without verifying them")
	}
	if preflight < 0 || preflight > golden || !strings.Contains(eval.Steps[preflight].Run, "sandbox_preflight_failed") {
		bad = append(bad, "live-eval needs a sandbox-preflight step naming sandbox_preflight_failed before golden-session")
	}
	if modules < 0 || modules > golden || !strings.Contains(eval.Steps[modules].Run, liveFillModules) ||
		!strings.Contains(eval.Steps[golden].Run, liveModuleProxy) {
		bad = append(bad, "live-eval must fill the module cache before golden-session and pass it as --proxy")
	}
	if signModules := liveStepIndex(sign, "modules"); export < 0 || signModules < 0 || signModules > export ||
		!strings.Contains(sign.Steps[signModules].Run, liveFillModules) || !strings.Contains(sign.Steps[export].Run, liveModuleProxy) {
		bad = append(bad, "sign must fill the module cache before export and pass it as --proxy")
	}
	return bad
}

func readLiveJobs(t *testing.T, source string) map[string]liveJob {
	t.Helper()
	var wf liveWorkflow
	require.NoError(t, yaml.Unmarshal([]byte(source), &wf))
	return wf.Jobs
}

// TestEvalHarnessLiveWorkflow_GuardsHoldAndVariantsAreCaught: the committed
// workflow keeps every guard, and each variant that drops one is named.
func TestEvalHarnessLiveWorkflow_GuardsHoldAndVariantsAreCaught(t *testing.T) {
	t.Parallel()
	source := readLiveWorkflow(t)
	require.Empty(t, liveGuardViolations(readLiveJobs(t, source)))
	variants := []struct{ name, old, replacement, want string }{
		{"signer workflow from the event", "--signer-workflow " + liveSignerWorkflow,
			`--signer-workflow "$GITHUB_REPOSITORY/.github/workflows/harness-eval-live.yml"`, "--signer-workflow " + liveSignerWorkflow},
		{"any branch", "--source-ref refs/heads/main", "--source-ref refs/heads/dev", "--source-ref refs/heads/main"},
		{"no preflight", "- name: sandbox-preflight\n", "- name: sandbox-check\n", "sandbox-preflight"},
		{"runner without the module proxy", "--output \"$RUNNER_TEMP/session\" --credential-env CODEX_API_KEY " + liveModuleProxy,
			"--output \"$RUNNER_TEMP/session\" --credential-env CODEX_API_KEY", "pass it as --proxy"},
	}
	for _, variant := range variants {
		require.Equal(t, 1, strings.Count(source, variant.old), variant.name)
		got := liveGuardViolations(readLiveJobs(t, strings.Replace(source, variant.old, variant.replacement, 1)))
		assert.Contains(t, strings.Join(got, "\n"), variant.want, variant.name)
	}
}

// runLiveStep runs one committed step script the way `shell: bash` does,
// with bin first on PATH and only env beside it.
func runLiveStep(t *testing.T, script, bin string, env ...string) (int, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "step.sh")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o755))
	cmd := exec.Command("/bin/bash", "--noprofile", "--norc", "-eo", "pipefail", path)
	cmd.Dir = dir
	cmd.Env = append([]string{"PATH=" + bin + ":/usr/bin:/bin", "HOME=" + dir}, env...)
	output, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(output)
	}
	require.NoError(t, err, string(output))
	return 0, string(output)
}

// TestEvalHarnessLiveWorkflow_BoundCheckVerifiesTheRunKeyAttestation runs the
// committed bound check against a stand-in gh: it verifies the file whose
// SHA-256 is the bind job's run key, and a failed verification is
// attempt_unbound before any trial.
func TestEvalHarnessLiveWorkflow_BoundCheckVerifiesTheRunKeyAttestation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the step runs under bash on macos-15")
	}
	t.Parallel()
	eval := readLiveJobs(t, readLiveWorkflow(t))["live-eval"]
	script := eval.Steps[liveStepIndex(eval, "bound-check")].Run
	bin, runner := t.TempDir(), t.TempDir()
	log := filepath.Join(t.TempDir(), "gh.log")
	fake := "#!/bin/sh\necho \"$*\" > \"$FAKE_LOG\"\ncp \"$3\" \"$FAKE_LOG.subject\"\nexit \"$FAKE_EXIT\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "gh"), []byte(fake), 0o755))
	env := []string{"RUNNER_TEMP=" + runner, "RUN_ID=18300000007", "RUN_ATTEMPT=2", "FAKE_LOG=" + log}

	code, _ := runLiveStep(t, script, bin, append(env, "FAKE_EXIT=0")...)

	require.Equal(t, 0, code)
	args, err := os.ReadFile(log)
	require.NoError(t, err)
	for _, flag := range liveBoundVerify[1:] {
		assert.Contains(t, string(args), flag)
	}
	subject, err := os.ReadFile(log + ".subject")
	require.NoError(t, err)
	sum := sha256.Sum256(subject)
	bindKey := sha256.Sum256([]byte("harneval-run:18300000007:2"))
	assert.Equal(t, hex.EncodeToString(bindKey[:]), hex.EncodeToString(sum[:]), "the subject is the bind job's run key")

	code, output := runLiveStep(t, script, bin, append(env, "FAKE_EXIT=1")...)

	assert.Equal(t, 1, code)
	assert.Contains(t, output, "harness-eval: attempt_unbound: run 18300000007 attempt 2")
}

// TestEvalHarnessLiveWorkflow_SandboxPreflightFailsClosed: until the hosted
// preflight of T14 lands, the committed step ends the job with
// sandbox_preflight_failed, so golden-session never starts.
func TestEvalHarnessLiveWorkflow_SandboxPreflightFailsClosed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the step runs under bash on macos-15")
	}
	t.Parallel()
	eval := readLiveJobs(t, readLiveWorkflow(t))["live-eval"]
	at := liveStepIndex(eval, "sandbox-preflight")
	require.GreaterOrEqual(t, at, 0)
	require.Less(t, at, liveStepIndex(eval, "golden-session"))

	code, output := runLiveStep(t, eval.Steps[at].Run, t.TempDir(), "RUNNER_TEMP="+t.TempDir())

	assert.Equal(t, 1, code)
	assert.Contains(t, output, "harness-eval: sandbox_preflight_failed: ")
}

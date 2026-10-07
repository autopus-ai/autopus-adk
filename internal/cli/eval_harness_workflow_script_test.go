package cli

import (
	"errors"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Stand-ins for the commands the eval step calls; each records its arguments.
// go build installs the fake auto at its -o path, git prints the changed
// files, and auto prints the decision document or exits with the run's code.
const (
	harnessFakeGo = `#!/bin/sh
echo "go $*" >> "$FAKE_LOG"
while [ "$#" -gt 0 ]; do
  if [ "$1" = -o ]; then cp "$FAKE_AUTO" "$2"; fi
  shift
done
`
	harnessFakeGit = `#!/bin/sh
echo "git $*" >> "$FAKE_LOG"
printf '%s\n' "$FAKE_CHANGED"
`
	harnessFakeAuto = `#!/bin/sh
echo "auto $*" >> "$FAKE_LOG"
case "$3" in
  applicable) printf '{"status":"%s","reason":"%s","matched":[]}\n' "$FAKE_STATUS" "$FAKE_REASON" ;;
  run) exit "$FAKE_RUN_EXIT" ;;
esac
`
)

// harnessStepRun is one execution of the eval step script.
type harnessStepRun struct {
	code           int
	calls, summary string
}

// runHarnessEvalStep runs the step script the way `shell: bash` does, with
// only the stand-ins, jq, and the system directories on PATH.
func runHarnessEvalStep(t *testing.T, jq, script string, env map[string]string) harnessStepRun {
	t.Helper()
	dir := t.TempDir()
	bin, runnerTemp := filepath.Join(dir, "bin"), filepath.Join(dir, "runner")
	require.NoError(t, os.Mkdir(bin, 0o755))
	require.NoError(t, os.Mkdir(runnerTemp, 0o755))
	files := map[string]string{"bin/go": harnessFakeGo, "bin/git": harnessFakeGit, "fake-auto": harnessFakeAuto, "step.sh": script}
	for rel, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, filepath.FromSlash(rel)), []byte(body), 0o755))
	}
	log, summary := filepath.Join(dir, "calls.log"), filepath.Join(dir, "step-summary.md")
	cmd := exec.Command("/bin/bash", "--noprofile", "--norc", "-eo", "pipefail", filepath.Join(dir, "step.sh"))
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + bin + ":" + filepath.Dir(jq) + ":/usr/bin:/bin", "HOME=" + dir,
		"RUNNER_TEMP=" + runnerTemp, "GITHUB_STEP_SUMMARY=" + summary,
		"FAKE_LOG=" + log, "FAKE_AUTO=" + filepath.Join(dir, "fake-auto")}
	for key, value := range env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output, err := cmd.CombinedOutput()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else {
		require.NoError(t, err, string(output))
	}
	calls, _ := os.ReadFile(log)
	written, _ := os.ReadFile(summary)
	return harnessStepRun{code: code, calls: string(calls), summary: string(written)}
}

func harnessStepEnv(base map[string]string, status, reason, runExit string) map[string]string {
	env := maps.Clone(base)
	env["FAKE_STATUS"], env["FAKE_REASON"], env["FAKE_RUN_EXIT"] = status, reason, runExit
	return env
}

// TestEvalHarnessWorkflow_StepScript_GatesOnTheApplicableDecision runs the
// committed eval step script against the stand-ins. A pull request without
// harness input ends 0 without running the eval and says so in the summary;
// one with harness input diffs base...head with --no-renames and fails with
// the run; a push runs the eval without a diff; an unknown decision fails.
// Not parallel: other tests in this package swap PATH while they generate.
func TestEvalHarnessWorkflow_StepScript_GatesOnTheApplicableDecision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the step runs under bash on ubuntu-latest")
	}
	jq, err := exec.LookPath("jq")
	if err != nil {
		t.Skip("jq is not installed; ubuntu-latest provides it")
	}
	_, workflow := readHarnessWorkflow(t)
	script := harnessWorkflowStepNamed(t, workflow.Jobs["harness-eval"], harnessEvalStepName).Run
	pr := map[string]string{"EVENT_NAME": "pull_request", "BASE_SHA": "base1", "HEAD_SHA": "head2", "FAKE_CHANGED": "README.md"}
	push := map[string]string{"EVENT_NAME": "push", "BASE_SHA": "", "HEAD_SHA": ""}

	skipped := runHarnessEvalStep(t, jq, script, harnessStepEnv(pr, "not_applicable", "no_harness_input_changed", "0"))
	assert.Equal(t, 0, skipped.code)
	assert.Contains(t, skipped.calls, "git diff --name-only --no-renames base1...head2\n")
	assert.Regexp(t, `auto eval harness applicable --event pull_request --format json --changed-files \S+/changed-files.txt\n`, skipped.calls)
	assert.NotContains(t, skipped.calls, "auto eval harness run")
	assert.Equal(t, "### harness-eval: not_applicable\n\n- no_harness_input_changed: no changed path meets the harness input set.\n",
		skipped.summary)

	regressed := runHarnessEvalStep(t, jq, script, harnessStepEnv(pr, "applicable", "harness_input_changed", "1"))
	assert.Equal(t, 1, regressed.code, "a failing run fails the check")
	assert.Regexp(t, `auto eval harness run --format json --output \S+/harness-eval/result.json --summary \S+/step-summary.md\n`,
		regressed.calls)

	evaluated := runHarnessEvalStep(t, jq, script, harnessStepEnv(push, "applicable", "non_pull_request_event", "0"))
	assert.Equal(t, 0, evaluated.code)
	assert.NotContains(t, evaluated.calls, "git ")
	assert.Contains(t, evaluated.calls, "auto eval harness applicable --event push --format json\n")
	assert.Contains(t, evaluated.calls, "auto eval harness run --format json")

	unknown := runHarnessEvalStep(t, jq, script, harnessStepEnv(push, "maybe", "unknown", "0"))
	assert.Equal(t, 1, unknown.code)
	assert.NotContains(t, unknown.calls, "auto eval harness run")
}

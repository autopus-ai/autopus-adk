package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// harnessWorkflowStep is the part of a ci.yaml step the harness-eval contract
// reads. Fields typed any only record whether the key is present.
type harnessWorkflowStep struct {
	Name            string            `yaml:"name"`
	If              string            `yaml:"if"`
	Uses            string            `yaml:"uses"`
	Shell           string            `yaml:"shell"`
	Run             string            `yaml:"run"`
	Env             map[string]string `yaml:"env"`
	With            map[string]any    `yaml:"with"`
	ContinueOnError any               `yaml:"continue-on-error"`
}

type harnessWorkflowJob struct {
	Name            string                `yaml:"name"`
	If              any                   `yaml:"if"`
	Needs           any                   `yaml:"needs"`
	ContinueOnError any                   `yaml:"continue-on-error"`
	RunsOn          string                `yaml:"runs-on"`
	TimeoutMinutes  int                   `yaml:"timeout-minutes"`
	Steps           []harnessWorkflowStep `yaml:"steps"`
}

type harnessWorkflow struct {
	On   map[string]any                `yaml:"on"`
	Jobs map[string]harnessWorkflowJob `yaml:"jobs"`
}

// harnessEvalStepName names the step that decides and runs the eval.
const harnessEvalStepName = "Evaluate harness surfaces"

var harnessPinnedAction = regexp.MustCompile(`^[^@\s]+@[0-9a-f]{40}$`)

func readHarnessWorkflow(t *testing.T) (string, harnessWorkflow) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yaml"))
	require.NoError(t, err)
	var workflow harnessWorkflow
	require.NoError(t, yaml.Unmarshal(raw, &workflow))
	return string(raw), workflow
}

func harnessWorkflowStepNamed(t *testing.T, job harnessWorkflowJob, name string) harnessWorkflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if step.Name == name {
			return step
		}
	}
	require.Failf(t, "missing step", "harness-eval has no step %q", name)
	return harnessWorkflowStep{}
}

// TestEvalHarnessWorkflow_S6_JobAlwaysReportsAndDecidesInside is the S6
// static contract of the harness-eval job (REQ-HE-05, REQ-HE-14): no paths
// filter on any trigger and no job-level if or needs, so the check is always
// reported; a bounded job on a full-history checkout; the --no-renames diff,
// the applicable decision, and the run with its summary in one step that
// fails with the run; the result uploaded even then; and the job kept after
// static-contracts, outside the omp-native-smoke section, with no secret and
// no live-lane string anywhere in ci.yaml (S12).
func TestEvalHarnessWorkflow_S6_JobAlwaysReportsAndDecidesInside(t *testing.T) {
	t.Parallel()
	raw, workflow := readHarnessWorkflow(t)
	for _, trigger := range []string{"push", "pull_request", "workflow_call"} {
		assert.Contains(t, workflow.On, trigger)
	}
	for trigger, settings := range workflow.On {
		filters, _ := settings.(map[string]any)
		assert.NotContains(t, filters, "paths", trigger)
		assert.NotContains(t, filters, "paths-ignore", trigger)
	}
	job, ok := workflow.Jobs["harness-eval"]
	require.True(t, ok, "ci.yaml has no harness-eval job")
	assert.Equal(t, "harness-eval", job.Name, "branch protection names the check by this context")
	assert.Nil(t, job.If, "a job-level if would skip the required check")
	assert.Nil(t, job.Needs, "a failed dependency would skip the required check")
	assert.Nil(t, job.ContinueOnError)
	assert.Equal(t, "ubuntu-latest", job.RunsOn)
	assert.True(t, job.TimeoutMinutes > 0 && job.TimeoutMinutes <= 15, "timeout-minutes = %d", job.TimeoutMinutes)
	for _, step := range job.Steps {
		if step.Uses != "" {
			assert.Regexp(t, harnessPinnedAction, step.Uses)
		}
		assert.Nil(t, step.ContinueOnError, step.Name)
	}
	require.NotEmpty(t, job.Steps)
	checkout := job.Steps[0]
	assert.True(t, strings.HasPrefix(checkout.Uses, "actions/checkout@"), checkout.Uses)
	assert.Equal(t, 0, checkout.With["fetch-depth"], "the base...head diff needs full history")
	assert.Equal(t, false, checkout.With["persist-credentials"])

	eval := harnessWorkflowStepNamed(t, job, harnessEvalStepName)
	assert.Equal(t, "bash", eval.Shell)
	assert.Equal(t, map[string]string{
		"EVENT_NAME": "${{ github.event_name }}",
		"BASE_SHA":   "${{ github.event.pull_request.base.sha }}",
		"HEAD_SHA":   "${{ github.event.pull_request.head.sha }}",
	}, eval.Env)
	for _, fragment := range []string{
		"set -euo pipefail",
		`git diff --name-only --no-renames "${BASE_SHA}...${HEAD_SHA}"`,
		`eval harness applicable --event "$EVENT_NAME" --format json`,
		"--changed-files",
		"eval harness run --format json --output",
		`--summary "$GITHUB_STEP_SUMMARY"`,
	} {
		assert.Contains(t, eval.Run, fragment)
	}
	assert.NotContains(t, eval.Run, "|| true", "a failed run must fail the check")

	upload := harnessWorkflowStepNamed(t, job, "Upload harness eval result")
	assert.True(t, strings.HasPrefix(upload.Uses, "actions/upload-artifact@"), upload.Uses)
	assert.Equal(t, "${{ !cancelled() }}", upload.If, "the result is uploaded when the eval step fails")
	assert.Equal(t, "${{ runner.temp }}/harness-eval", upload.With["path"])

	static := strings.Index(raw, "\n  static-contracts:\n")
	start := strings.Index(raw, "\n  harness-eval:\n")
	native := strings.Index(raw, "\n  omp-native-smoke:\n")
	require.True(t, static >= 0 && start > static && native > start,
		"harness-eval must sit after static-contracts and before omp-native-smoke")
	assert.NotContains(t, raw[start:native], "${{ secrets.")
	for _, forbidden := range []string{"golden", "harness_live_advisory"} {
		assert.NotContains(t, raw, forbidden, "the live lane stays out of ci.yaml (S12)")
	}
}

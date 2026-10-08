package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// liveExportPrefix is how the export step must start: the shell reads the
// signing key variable and env -u removes the same name (REQ-HR-01, S3).
const liveExportPrefix = `printf '%s' "$HARNESS_EVAL_SIGNING_KEY" | env -u HARNESS_EVAL_SIGNING_KEY auto eval harness export`

var pinnedUses = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}$`)

type liveWorkflow struct {
	On   map[string]map[string]any `yaml:"on"`
	Jobs map[string]liveJob        `yaml:"jobs"`
}

type liveJob struct {
	If          string            `yaml:"if"`
	RunsOn      string            `yaml:"runs-on"`
	Needs       string            `yaml:"needs"`
	Environment string            `yaml:"environment"`
	Permissions map[string]string `yaml:"permissions"`
	Env         map[string]string `yaml:"env"`
	Steps       []liveStep        `yaml:"steps"`
}

type liveStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	With map[string]any    `yaml:"with"`
	Env  map[string]string `yaml:"env"`
	Run  string            `yaml:"run"`
}

// text is everything a step names: its name, action, inputs, env, and script.
func (s liveStep) text() string {
	return fmt.Sprint(s.Name, " ", s.Uses, " ", s.With, " ", s.Env, " ", s.Run)
}

func readLiveWorkflow(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "harness-eval-live.yml"))
	require.NoError(t, err)
	return string(data)
}

// liveWorkflowViolations lists every REQ-HR-01 rule the workflow breaks.
func liveWorkflowViolations(source string) []string {
	var wf liveWorkflow
	if err := yaml.Unmarshal([]byte(source), &wf); err != nil {
		return []string{"yaml: " + err.Error()}
	}
	var bad []string
	fail := func(format string, args ...any) { bad = append(bad, fmt.Sprintf(format, args...)) }
	if len(wf.On) != 1 || wf.On["workflow_dispatch"] != nil || !hasKey(wf.On, "workflow_dispatch") {
		fail("on must be exactly workflow_dispatch without inputs")
	}
	if names := sortedKeys(wf.Jobs); !slices.Equal(names, []string{"bind", "live-eval", "sign"}) {
		fail("jobs are %v", names)
	}
	perms := map[string]map[string]string{
		"bind":      {"contents": "read", "id-token": "write", "attestations": "write"},
		"live-eval": {"contents": "read", "id-token": "write", "attestations": "write"},
		"sign":      {"contents": "read", "actions": "read", "id-token": "write", "attestations": "write"},
	}
	secretSteps := map[string]string{"bind": "", "live-eval": "golden-session", "sign": "export"}
	for name, job := range wf.Jobs {
		if job.If != "github.ref == 'refs/heads/main'" || job.RunsOn != "macos-15" {
			fail("%s must run only on main on macos-15", name)
		}
		if fmt.Sprint(job.Permissions) != fmt.Sprint(perms[name]) {
			fail("%s permissions are %v", name, job.Permissions)
		}
		if len(job.Env) > 0 {
			fail("%s declares job env", name)
		}
		for _, step := range job.Steps {
			if strings.Contains(step.Run, "${{") {
				fail("%s step %q interpolates an expression into its script", name, step.Name)
			}
			if step.Uses != "" && !pinnedUses.MatchString(strings.Fields(step.Uses)[0]) {
				fail("%s uses %s without a 40-hex SHA", name, step.Uses)
			}
			if strings.HasPrefix(step.Uses, "actions/checkout@") && fmt.Sprint(step.With["persist-credentials"]) != "false" {
				fail("%s checkout persists credentials", name)
			}
			if strings.Contains(step.text(), "secrets.") && (step.Name != secretSteps[name] || strings.Contains(step.Run, "secrets.")) {
				fail("%s step %q reads a secret", name, step.Name)
			}
		}
	}
	return append(bad, liveJobRoles(wf.Jobs)...)
}

// liveJobRoles checks what each job is for: the bind and session-result
// attestations, the attempt_unbound check before the one golden-session
// step, and an export step that holds only the key and starts the pipeline.
func liveJobRoles(jobs map[string]liveJob) []string {
	var bad []string
	bind, eval, sign := jobs["bind"], jobs["live-eval"], jobs["sign"]
	if bind.Environment != "" || eval.Environment != "adk-harness-eval-agent" || sign.Environment != "adk-harness-eval-signing" {
		bad = append(bad, "environments must be none, adk-harness-eval-agent, adk-harness-eval-signing")
	}
	if eval.Needs != "bind" || sign.Needs != "live-eval" {
		bad = append(bad, "live-eval must need bind and sign must need live-eval")
	}
	if !attests(bind, "https://autopus.ai/harness-eval/bound/v1") || !attests(eval, "https://autopus.ai/harness-eval/session-result/v1") {
		bad = append(bad, "bind and live-eval must attest bound and session-result")
	}
	golden, unbound := -1, -1
	for index, step := range eval.Steps {
		if step.Name == "golden-session" {
			if golden >= 0 {
				bad = append(bad, "live-eval has more than one golden-session step")
			}
			golden = index
		}
		if strings.Contains(step.Run, "attempt_unbound") && unbound < 0 {
			unbound = index
		}
	}
	if golden < 0 || unbound < 0 || unbound > golden {
		bad = append(bad, "live-eval must check attempt_unbound before its golden-session step")
	}
	exports := 0
	for _, step := range sign.Steps {
		for _, banned := range []string{"golden.py", "python", "codex", "go test"} {
			if strings.Contains(step.text(), banned) {
				bad = append(bad, "sign step "+step.Name+" names "+banned)
			}
		}
		if step.Name == "export" {
			exports++
			if keys := sortedKeys(step.Env); !slices.Equal(keys, []string{"HARNESS_EVAL_SIGNING_KEY"}) ||
				!strings.HasPrefix(strings.TrimSpace(step.Run), liveExportPrefix) {
				bad = append(bad, "export must hold only HARNESS_EVAL_SIGNING_KEY and start with the stdin pipeline")
			}
		}
	}
	if exports != 1 {
		bad = append(bad, "sign must have exactly one export step")
	}
	return bad
}

func attests(job liveJob, predicate string) bool {
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, "actions/attest@") && step.With["predicate-type"] == predicate {
			return true
		}
	}
	return false
}

func hasKey[V any](m map[string]V, key string) bool { _, found := m[key]; return found }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// TestEvalHarnessLiveWorkflow_S3_SeparatesBindEvalAndSign: the committed
// workflow keeps every REQ-HR-01 rule.
func TestEvalHarnessLiveWorkflow_S3_SeparatesBindEvalAndSign(t *testing.T) {
	t.Parallel()
	assert.Empty(t, liveWorkflowViolations(readLiveWorkflow(t)))
}

// TestEvalHarnessLiveWorkflow_S3_VariantsAreCaught: each variant breaks one
// rule and the contract names it, including an export that reads one
// variable and unsets another.
func TestEvalHarnessLiveWorkflow_S3_VariantsAreCaught(t *testing.T) {
	t.Parallel()
	source := readLiveWorkflow(t)
	variants := []struct{ name, old, replacement, want string }{
		{"unset a different variable", "env -u HARNESS_EVAL_SIGNING_KEY auto", "env -u SIGNING_KEY auto", "export must hold only"},
		{"pull request trigger", "on:\n  workflow_dispatch:\n", "on:\n  workflow_dispatch:\n  pull_request:\n", "on must be exactly"},
		{"dispatch input", "on:\n  workflow_dispatch:\n", "on:\n  workflow_dispatch:\n    inputs:\n      ref:\n        type: string\n", "on must be exactly"},
		{"expression in a script", `"repos/$REPOSITORY/actions`, `"repos/${{ github.repository }}/actions`, "interpolates an expression"},
		{"tag-pinned attest", "actions/attest@1e69f48acb82d1966a394da916b4c1698aa569d6 # v4.2.2\n        with:\n          subject-checksums: ${{ runner.temp }}/bound",
			"actions/attest@v4\n        with:\n          subject-checksums: ${{ runner.temp }}/bound", "without a 40-hex SHA"},
		{"secret in another step", "- name: bound-check\n        env:\n          GH_TOKEN: ${{ github.token }}",
			"- name: bound-check\n        env:\n          GH_TOKEN: ${{ secrets.CODEX_API_KEY }}", "reads a secret"},
		{"persisted credentials", "fetch-depth: 0\n          persist-credentials: false\n\n      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6\n        with:\n          go-version-file: go.mod\n\n      - name: binding",
			"fetch-depth: 0\n\n      - uses: actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16 # v6\n        with:\n          go-version-file: go.mod\n\n      - name: binding", "persists credentials"},
		{"golden session in sign", "      - name: run-meta\n", "      - name: run-meta\n        run: python3 scripts/benchmarks/harness/golden.py\n      - name: run-meta-real\n", "names python"},
		{"no bound check", "attempt_unbound", "unbound", "attempt_unbound before"},
	}
	for _, variant := range variants {
		require.Equal(t, 1, strings.Count(source, variant.old), variant.name)
		got := liveWorkflowViolations(strings.Replace(source, variant.old, variant.replacement, 1))
		assert.NotEmpty(t, got, variant.name)
		assert.Contains(t, strings.Join(got, "\n"), variant.want, variant.name)
	}
}

// TestEvalHarnessLiveWorkflow_S5StepGatesOnListFloorAndPassSet: ci.yaml's
// macos-runtime job runs the darwin-only S5 tests with a go test -list floor
// no larger than the TestEvalHarnessE2E_ tests declared, then compares the
// PASS set with the list, so a skip or a rename fails the step (REQ-HR-10).
func TestEvalHarnessLiveWorkflow_S5StepGatesOnListFloorAndPassSet(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yaml"))
	require.NoError(t, err)
	raw := string(data)
	start, end := strings.Index(raw, "\n  macos-runtime:\n"), strings.Index(raw, "\n  windows-runtime:\n")
	require.True(t, start >= 0 && end > start, "cannot isolate macos-runtime")
	job := raw[start:end]
	at := strings.Index(job, "- name: Test signed harness lane digest chain")
	require.GreaterOrEqual(t, at, 0, "macos-runtime has no S5 step")
	step := job[at:]
	requireContainsAll(t, step, `filter='^TestEvalHarnessE2E_'`, `go test -list "$filter" ./internal/cli`,
		`-v ./internal/cli -run "$filter"`, `sed -n 's/^--- PASS: \([^ ]*\).*/\1/p' "$log"`, `[[ "$observed" != "$listed" ]]`)
	floor := regexp.MustCompile(`\n\s+minimum=(\d+)\n`).FindStringSubmatch(step)
	require.Len(t, floor, 2)
	source, err := os.ReadFile("eval_harness_e2e_test.go")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(source), "//go:build darwin\n"), "the S5 chain needs sandbox-exec hosts only")
	declared := strings.Count(string(source), "\nfunc TestEvalHarnessE2E_")
	minimum, err := strconv.Atoi(floor[1])
	require.NoError(t, err)
	assert.GreaterOrEqual(t, minimum, 1, "a zero floor lets an empty list pass")
	assert.LessOrEqual(t, minimum, declared, "the floor exceeds the declared S5 tests")
}

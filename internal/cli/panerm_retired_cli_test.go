package cli_test

// SPEC-PANERM-001 T6 (S8, S9, S10) on the built auto binary: retired
// subcommands fail with the migration message and touch nothing, and the
// retired flags are hidden no-ops (--yield-rounds adds exactly one notice).

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var panermRetiredSubcommands = []string{"collect", "inject", "cleanup", "status", "wait", "result"}

var panermRetiredFlagTokens = []string{"--no-detach", "--subprocess", "--plain", "--yield-rounds"}

const panermYieldNotice = "auto: warning: --yield-rounds was retired with the orchestra pane backend (SPEC-PANERM-001); all rounds run synchronously"

func panermRetirementError(name string) string {
	return "Error: auto orchestra " + name + " was retired with the orchestra pane backend (SPEC-PANERM-001); " +
		"orchestra commands now run synchronously and print their result directly\n"
}

func TestPanermS10_RetiredSubcommandsFailWithTheMigrationMessage(t *testing.T) {
	bin := buildPanermAuto(t)
	ws := newPanermCLIWorkspace(t, readLegacyPaneConfig(t, "c7.yaml"))
	ws.extraEnv = []string{"TMUX=/tmp/panerm-fake-tmux,1,0", "CMUX_SOCKET_PATH=/tmp/panerm-fake-cmux.sock"}
	workspaceBefore, tmpBefore := snapshotTree(t, ws.root), snapshotTree(t, ws.tmp)

	for _, name := range panermRetiredSubcommands {
		for _, args := range [][]string{
			{"orchestra", name},
			{"orchestra", name, "job-123", "--timeout", "60"},
			{"--quality", "x", "--config", "/nonexistent/autopus.yaml", "orchestra", name, "job-123"},
		} {
			got := ws.run(t, bin, args...)
			assert.Equal(t, panermRetirementError(name), got.stderr, args)
			assert.Empty(t, got.stdout, args)
			assert.Equal(t, 1, got.exit, args)
		}
	}

	assert.Equal(t, workspaceBefore, snapshotTree(t, ws.root), "autopus.yaml and the workspace keep their bytes")
	assert.Equal(t, tmpBefore, snapshotTree(t, ws.tmp), "$TMPDIR is unchanged")
	assert.NoFileExists(t, ws.termLog, "no terminal multiplexer call")
	assert.Empty(t, ws.calls(t), "no provider call")

	help := ws.run(t, bin, "orchestra", "--help")
	require.Equal(t, 0, help.exit, help.stderr)
	for _, name := range []string{"brainstorm", "plan", "review", "secure", "run"} {
		assert.Regexp(t, regexp.MustCompile(`(?m)^  `+name+` `), help.stdout)
	}
	for _, name := range panermRetiredSubcommands {
		assert.NotRegexp(t, regexp.MustCompile(`(?m)^  `+name+`\b`), help.stdout)
	}
}

// panermFlagPair is one S8 pair: the same invocation with and without a
// retired flag.
type panermFlagPair struct {
	name string
	args []string
	flag string
}

var panermS8Pairs = []panermFlagPair{
	{"brainstorm --no-detach", []string{"orchestra", "brainstorm", "panerm topic", "--strategy", "consensus",
		"--providers", "claude,codex", "--format", "json"}, "--no-detach"},
	{"brainstorm --subprocess", []string{"orchestra", "brainstorm", "panerm topic", "--strategy", "consensus",
		"--providers", "claude,codex", "--format", "json"}, "--subprocess"},
	{"plan --no-detach", []string{"orchestra", "plan", "panerm plan", "--strategy", "consensus",
		"--providers", "claude,codex", "--no-persist", "--format", "json"}, "--no-detach"},
	{"plan --subprocess", []string{"orchestra", "plan", "panerm plan", "--strategy", "consensus",
		"--providers", "claude,codex", "--no-persist", "--format", "json"}, "--subprocess"},
	// templates/codex/skills/auto-review.md.tmpl:69 at B, filled in.
	{"review --no-detach", []string{"orchestra", "review", "a.go", "--risk-tier", "high", "--strategy", "debate",
		"--providers", "claude,codex", "--format", "json"}, "--no-detach"},
	{"secure --no-detach", []string{"orchestra", "secure", "a.go", "--strategy", "consensus",
		"--providers", "claude,codex", "--format", "json"}, "--no-detach"},
	{"run --subprocess", []string{"orchestra", "run", "panerm run", "--strategy", "consensus",
		"--providers", "claude,codex", "--format", "json"}, "--subprocess"},
}

// assertPairEqual runs args without and then with flag in two fresh
// workspaces and compares argv, stdout, stderr, and exit status.
func assertPairEqual(t *testing.T, bin string, args []string, flag string,
	setup func(*testing.T, panermCLIWorkspace)) (panermRun, panermRun) {
	t.Helper()
	plain, flagged := newPanermCLIWorkspace(t, nil), newPanermCLIWorkspace(t, nil)
	if setup != nil {
		setup(t, plain)
		setup(t, flagged)
	}
	want := plain.run(t, bin, args...)
	got := flagged.run(t, bin, append(append([]string(nil), args...), flag)...)

	assert.Equal(t, plain.calls(t), flagged.calls(t), "recorded provider argv")
	assert.Equal(t, plain.normalize(t, want.stdout), flagged.normalize(t, got.stdout), "stdout")
	assert.Equal(t, sortParallelProviderLines(plain.normalize(t, want.stderr)),
		sortParallelProviderLines(flagged.normalize(t, got.stderr)), "stderr")
	assert.Equal(t, want.exit, got.exit, "exit status")
	return want, got
}

// Spec review runs providers in parallel goroutines that each print a start
// line (mode=parallel) and then a completion or failure line
// (spec_review_structured_runtime.go). Which provider launches first, which
// finishes first, and whether one finishes before the other launches is
// goroutine scheduling, not behavior.
var (
	parallelProviderStartLine = regexp.MustCompile(`^SPEC 리뷰 provider 시작: \S+ \(.*, mode=parallel\)$`)
	providerOutcomeLine       = regexp.MustCompile(`^SPEC 리뷰 provider (완료|실패): \S+ \(`)
)

// sortParallelProviderLines sorts each block of parallel provider lines: a
// parallel start line and the start, completion, and failure lines right
// after it. A stderr comparison then ignores the scheduling order inside the
// block and nothing else: every line still counts, no line crosses another
// line, and sequential providers keep their order.
func sortParallelProviderLines(text string) string {
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		if !parallelProviderStartLine.MatchString(lines[i]) {
			continue
		}
		j := i + 1
		for j < len(lines) && (parallelProviderStartLine.MatchString(lines[j]) || providerOutcomeLine.MatchString(lines[j])) {
			j++
		}
		sort.Strings(lines[i:j])
		i = j - 1
	}
	return strings.Join(lines, "\n")
}

func TestPanermS8_HiddenNoOpFlagsChangeNothing(t *testing.T) {
	bin := buildPanermAuto(t)
	for _, pair := range panermS8Pairs {
		t.Run(pair.name, func(t *testing.T) {
			want, _ := assertPairEqual(t, bin, pair.args, pair.flag, nil)
			assert.Equal(t, 0, want.exit, want.stderr)
			assert.Contains(t, want.stdout, `"schema"`)
		})
	}
	for _, flag := range []string{"--subprocess", "--plain"} {
		t.Run("spec review "+flag, func(t *testing.T) {
			want, _ := assertPairEqual(t, bin, []string{"spec", "review", "SPEC-PANERMS8-001",
				"--skip-provider-readiness", "--providers", "claude,codex"}, flag, scaffoldPanermSpec)
			assert.Equal(t, 0, want.exit, want.stderr)
		})
	}

	ws := newPanermCLIWorkspace(t, nil)
	for _, path := range [][]string{{"orchestra", "brainstorm"}, {"orchestra", "plan"}, {"orchestra", "review"},
		{"orchestra", "secure"}, {"orchestra", "run"}, {"spec", "review"}} {
		help := ws.run(t, bin, append(append([]string(nil), path...), "--help")...)
		require.Equal(t, 0, help.exit, help.stderr)
		for _, token := range panermRetiredFlagTokens {
			assert.NotContains(t, help.stdout, token, strings.Join(path, " ")+" --help")
		}
	}
}

func TestPanermS9_YieldRoundsRunsEveryRoundAndWarnsOnce(t *testing.T) {
	bin := buildPanermAuto(t)
	args := []string{"orchestra", "brainstorm", "x", "--providers", "claude,codex", "--rounds", "2", "--format", "json"}
	plain, flagged := newPanermCLIWorkspace(t, nil), newPanermCLIWorkspace(t, nil)
	want := plain.run(t, bin, args...)
	got := flagged.run(t, bin, append(append([]string(nil), args...), "--yield-rounds")...)

	require.Equal(t, 0, want.exit, want.stderr)
	assert.Equal(t, want.exit, got.exit)
	assert.Equal(t, plain.normalize(t, want.stdout), flagged.normalize(t, got.stdout), "stdout")
	calls := flagged.calls(t)
	assert.Equal(t, plain.calls(t), calls, "both runs make the same provider calls")
	// Two debate rounds for each provider; claude, the default judge, adds the verdict call.
	assert.Equal(t, 3, countPanermCalls(calls, "claude | --print | "), calls)
	assert.Equal(t, 2, countPanermCalls(calls, "codex | exec | "), calls)
	rest, notices := withoutLine(got.stderr, panermYieldNotice)
	assert.Equal(t, 1, notices, got.stderr)
	assert.Equal(t, plain.normalize(t, want.stderr), flagged.normalize(t, rest), "stderr apart from the notice")
	assert.Equal(t, 0, strings.Count(want.stderr, panermYieldNotice))
}

func countPanermCalls(calls []string, prefix string) int {
	n := 0
	for _, call := range calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

func TestSortParallelProviderLines_IgnoresOnlyParallelSchedulingOrder(t *testing.T) {
	start := func(name, mode string) string {
		return "SPEC 리뷰 provider 시작: " + name + " (backend=subprocess, timeout=<duration>, mode=" + mode + ")"
	}
	done := func(name string) string {
		return "SPEC 리뷰 provider 완료: " + name + " (backend=subprocess, elapsed=<duration>)"
	}
	failed := func(name string) string {
		return "SPEC 리뷰 provider 실패: " + name + " (backend=subprocess, class=timeout, elapsed=<duration>): x"
	}
	const judge = "SPEC 리뷰 judge 완료: claude (verdict=PASS, accepted=0, rejected=0, merged=0)"
	lines := func(order ...string) string { return strings.Join(append(order, ""), "\n") }
	normalized := func(order ...string) string { return sortParallelProviderLines(lines(order...)) }
	launched := normalized("head", start("claude", "parallel"), start("codex", "parallel"), done("codex"), done("claude"),
		judge, start("codex", "sequential"), done("codex"), start("claude", "sequential"), done("claude"))

	for name, order := range map[string][]string{
		"parallel launch order": {"head", start("codex", "parallel"), start("claude", "parallel"), done("codex"),
			done("claude"), judge, start("codex", "sequential"), done("codex"), start("claude", "sequential"), done("claude")},
		// The stderr pair of CI run 37871017866.
		"parallel completion order": {"head", start("claude", "parallel"), start("codex", "parallel"), done("claude"),
			done("codex"), judge, start("codex", "sequential"), done("codex"), start("claude", "sequential"), done("claude")},
		"completion before the other launch": {"head", start("codex", "parallel"), done("codex"), start("claude", "parallel"),
			done("claude"), judge, start("codex", "sequential"), done("codex"), start("claude", "sequential"), done("claude")},
	} {
		assert.Equal(t, launched, normalized(order...), "%s is goroutine scheduling", name)
	}
	assert.Equal(t, normalized(start("codex", "parallel"), start("claude", "parallel"), failed("claude"), done("codex")),
		normalized(start("claude", "parallel"), done("codex"), start("codex", "parallel"), failed("claude")),
		"a parallel failure line is an outcome like a completion line")

	for name, order := range map[string][]string{
		"sequential launch order": {"head", start("claude", "parallel"), start("codex", "parallel"), done("codex"),
			done("claude"), judge, start("claude", "sequential"), done("claude"), start("codex", "sequential"), done("codex")},
		"an outcome after another line": {"head", start("claude", "parallel"), start("codex", "parallel"), done("codex"),
			judge, done("claude"), start("codex", "sequential"), done("codex"), start("claude", "sequential"), done("claude")},
		"a missing outcome": {"head", start("claude", "parallel"), start("codex", "parallel"), done("codex"),
			judge, start("codex", "sequential"), done("codex"), start("claude", "sequential"), done("claude")},
	} {
		assert.NotEqual(t, launched, normalized(order...), "%s still counts", name)
	}
}

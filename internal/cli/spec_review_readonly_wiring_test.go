package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// reviewJSONRecorderScript records the NUL-separated argv and the stdin of
// every run under a per-process name and answers with a reviewer PASS, also
// into the codex --output-last-message file.
const reviewJSONRecorderScript = `#!/bin/sh
record="$AUTOPUS_TEST_ARGV_DIR/$(basename "$0").$$"
cat > "$record.stdin"
printf '%s\0' "$@" > "$record.argv"
body='{"verdict":"PASS","summary":"ok","findings":[]}'
while [ $# -gt 0 ]; do
	if [ "$1" = "--output-last-message" ]; then printf '%s' "$body" > "$2"; fi
	shift
done
printf '%s\n' "$body"
`

// installReviewJSONRecorders puts JSON-answering recorder binaries with the
// given native names first on PATH and returns the evidence directory.
func installReviewJSONRecorders(t *testing.T, names ...string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("argv recorders require a POSIX shell")
	}
	binDir, evidenceDir := t.TempDir(), t.TempDir()
	for _, name := range names {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte(reviewJSONRecorderScript), 0o755))
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AUTOPUS_TEST_ARGV_DIR", evidenceDir)
	return evidenceDir
}

// readRecordedArgvs returns the argv of every recorded run of name.
func readRecordedArgvs(t *testing.T, evidenceDir, name string) [][]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(evidenceDir, name+".*.argv"))
	require.NoError(t, err)
	sort.Strings(paths)
	argvs := make([][]string, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		argvs = append(argvs, strings.Split(string(bytes.TrimSuffix(data, []byte{0})), "\x00"))
	}
	return argvs
}

// S1 (REQ-01, REQ-19) at the CLI boundary: in a pane-capable context every
// reviewer and the judge execute through the real subprocess backend with the
// projected argv plus only the Execution Boundary runtime items, and the
// receipt reads its sandbox mode from that executed argv (REQ-07).
func TestRunSpecReview_RealSubprocessBackendExecutesProjectedArgv(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, nil)
	useHermeticReadiness(t)
	tmuxLog := usePaneCapableContext(t)

	require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{}))

	pClaude := withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max")
	assert.Equal(t, [][]string{pClaude, pClaude}, readRecordedArgvs(t, fixture.evidence, "claude"), "reviewer and judge")
	codexHead := []string{
		"exec", "--json", "--sandbox", "read-only", "-m", "gpt-5.6-sol", "-c", `model_reasoning_effort="max"`,
		"--ephemeral", "--ignore-user-config", "--ignore-rules",
	}
	codex := readRecordedArgvs(t, fixture.evidence, "codex")
	require.Len(t, codex, 1)
	require.Len(t, codex[0], len(codexHead)+4, "codex argv %q", codex[0])
	assert.Equal(t, codexHead, codex[0][:len(codexHead)])
	assert.Equal(t, "--output-schema", codex[0][len(codexHead)])
	assert.Equal(t, "--output-last-message", codex[0][len(codexHead)+2])
	gemini := readRecordedArgvs(t, fixture.evidence, "agy")
	require.Len(t, gemini, 1)
	require.Len(t, gemini[0], 6, "agy argv %q", gemini[0])
	assert.Equal(t, "--print", gemini[0][0])
	assert.Contains(t, gemini[0][1], fixture.specID, "the prompt replaces the empty --print slot")
	assert.Equal(t, []string{"--mode", "plan", "--sandbox", "--disable-slash-commands"}, gemini[0][2:])
	// The agy prompt item is review text, not a flag, so it is left out.
	withoutPrompt := append([]string{gemini[0][0]}, gemini[0][2:]...)
	for _, argv := range append(append(readRecordedArgvs(t, fixture.evidence, "claude"), codex...), withoutPrompt) {
		assertNoWideningArgv(t, argv)
	}
	assert.NoFileExists(t, tmuxLog, "spec review must not call the terminal")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, "PASS", receipt.Verdict)
	assert.Equal(t, []specReviewProviderPolicyRow{
		{Provider: "claude", Role: "reviewer", SandboxMode: "read-only", Readiness: "unknown(probe_failed)"},
		{Provider: "codex", Role: "reviewer", SandboxMode: "read-only", Readiness: "unknown(probe_failed)"},
		{Provider: "gemini", Role: "reviewer", SandboxMode: "unverified", Readiness: "unknown(no_status_command)"},
		{Provider: "claude", Role: "judge", SandboxMode: "read-only", Readiness: "unknown(probe_failed)"},
	}, receipt.ProviderPolicy)
}

// S6 (REQ-04, REQ-18): an explicit widening config fails closed with the
// Error Contract before any binary, probe, or provider runs, and
// --allow-degraded never bypasses it.
func TestRunSpecReview_ExplicitPolicyViolationFailsBeforeAnyExecution(t *testing.T) {
	for _, allowDegraded := range []bool{false, true} {
		t.Run(map[bool]string{false: "strict", true: "allow-degraded"}[allowDegraded], func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
				editProvider(cfg, "claude", func(e *config.ProviderEntry) { e.Args = []string{"--print", "--verbose"} })
			})
			backend := fixture.useFakeBackend(t)
			spy := useReadinessRunner(t, replyWith(0, `{"loggedIn":true}`, ""))

			err := runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{allowDegraded: allowDegraded})

			require.Error(t, err)
			assert.Equal(t, `spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--verbose" (config key: orchestra.providers.claude.args; remedy: remove "--verbose" from orchestra.providers.claude.args)`, err.Error())
			assert.Zero(t, backend.total())
			assert.Zero(t, fixture.catalogProbes.Load())
			assert.Empty(t, spy.argvs())
			recorded, globErr := filepath.Glob(filepath.Join(fixture.evidence, "*.argv"))
			require.NoError(t, globErr)
			assert.Empty(t, recorded)
		})
	}
}

// S3 (REQ-02): the judge is projected whether it reuses a reviewer or is
// resolved separately, and a widening separate judge fails before execution.
func TestRunSpecReview_JudgeIsProjectedOnBothResolutionPaths(t *testing.T) {
	pClaude := withClaudeReadOnlySuffix("--print", "--model", "claude-fable-5-1", "--effort", "max")
	for _, providers := range [][]string{nil, {"codex"}} {
		t.Run(strings.Join(append([]string{"reviewers"}, providers...), "-"), func(t *testing.T) {
			fixture := newReadOnlyReviewFixture(t, nil)
			writeGPTReviewDocuments(t, fixture.root)
			backend := fixture.useFakeBackend(t)
			useHermeticReadiness(t)

			require.NoError(t, runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{providers: providers}))

			judge := backend.request(t, "claude", "judge")
			assert.Equal(t, pClaude, judge.Config.Args)
			assert.Equal(t, orchestra.SandboxModeReadOnly, judge.Config.SandboxMode)
		})
	}

	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		editProvider(cfg, "claude", func(e *config.ProviderEntry) { e.Args = []string{"--print", "--verbose"} })
	})
	backend := fixture.useFakeBackend(t)
	useHermeticReadiness(t)
	err := runSpecReviewWithOptions(context.Background(), fixture.specID, "", 0, specReviewOptions{providers: []string{"codex"}})
	require.Error(t, err)
	assert.Equal(t, `spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--verbose" (config key: orchestra.providers.claude.args; remedy: remove "--verbose" from orchestra.providers.claude.args)`, err.Error())
	assert.Zero(t, backend.total())
}

// writeGPTReviewDocuments adds the documents a GPT-only review must receive.
func writeGPTReviewDocuments(t *testing.T, root string) {
	t.Helper()
	for _, ref := range []string{"AGENTS.md", ".autopus/project/workspace.md", "ARCHITECTURE.md"} {
		writeCLIReviewContextFile(t, root, ref, ref+"\n")
	}
}

// S8 (REQ-06): a rejected provider that only --multi discovery selects is
// excluded with a warning, leaves the denominator and Provider Health, and
// gets an excluded receipt row. The config loader migrates an "opencode"
// entry to codex, so the unsupported provider here is "aider".
func TestRunSpecReview_MultiDiscoveredViolationIsExcludedAndRecorded(t *testing.T) {
	fixture := newReadOnlyReviewFixture(t, func(cfg *config.HarnessConfig) {
		cfg.Spec.ReviewGate.Providers = []string{"claude", "codex"}
		delete(cfg.Orchestra.Providers, "gemini")
		cfg.Orchestra.Providers["aider"] = config.ProviderEntry{Binary: "aider"}
		cfg.Orchestra.Commands["review"] = config.CommandEntry{Strategy: "debate", Providers: []string{"claude", "codex", "aider"}}
	})
	backend := fixture.useFakeBackend(t)
	useHermeticReadiness(t)
	ctx := withGlobalFlags(context.Background(), globalFlags{MultiMode: true})

	stderr := captureSpecReviewStderr(t, func() {
		require.NoError(t, runSpecReviewWithOptions(ctx, fixture.specID, "", 0, specReviewOptions{}))
	})

	assert.Contains(t, stderr, "spec review: excluding discovered provider \"aider\": unsupported provider \"aider\"\n")
	assert.Equal(t, 1, backend.calls("claude", "reviewer"))
	assert.Equal(t, 1, backend.calls("codex", "reviewer"))
	assert.Equal(t, 3, backend.total(), "claude and codex reviewers plus the judge")
	receipt := readSpecReviewReceipt(t, fixture.specDir)
	assert.Equal(t, []string{"claude success -", "codex success -"}, providerHealthRows(receipt.Providers))
	assert.Empty(t, receipt.DegradedReasons, "quorum 2 of denominator 2")
	assert.Contains(t, receipt.ProviderPolicy, specReviewProviderPolicyRow{Provider: "aider", Role: "reviewer", Excluded: true})
}

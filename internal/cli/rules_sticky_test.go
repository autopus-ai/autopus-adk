package cli

// SPEC-STICKYRULE-001 command-boundary oracles.
//
// The generated UserPromptSubmit hook invokes `auto rules sticky`, so the
// pkg/rulecond tests alone cannot prove the feature is wired: the command could
// resolve the wrong root, drop stdin, write to the wrong stream, or return an
// error that the process turns into the exit status that erases the prompt.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// stickyRuleNames are the two rules REQ-STICKYRULE-MAP-01 designates.
var stickyRuleNames = []string{"language-policy", "objective-reasoning"}

// runRulesListIn executes `auto rules list` with dir as the working directory.
func runRulesListIn(t *testing.T, dir string) string {
	t.Helper()
	t.Chdir(dir)
	return runRulesList(t)
}

// runRulesSticky executes `auto rules sticky` in dir and returns stdout/stderr.
func runRulesSticky(t *testing.T, dir, stdin string) (string, string) {
	t.Helper()
	t.Chdir(dir)

	cmd := newRulesCmd()
	var out, errOut bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"sticky", "--event", rulecond.EventUserPromptSubmit})

	require.NoError(t, cmd.Execute(),
		"REQ-STICKYRULE-FIRE-03: a non-zero exit on UserPromptSubmit erases the user's prompt")
	return out.String(), errOut.String()
}

// stickyProjectRoot builds a project root with one sticky rule installed.
func stickyProjectRoot(t *testing.T, marker string) string {
	t.Helper()
	return stickyProjectRootIn(t, t.TempDir(), marker)
}

// stickyProjectRootIn installs the same fixture into a caller-chosen root, so a
// test can place the project at the home directory the resolver treats
// specially.
func stickyProjectRootIn(t *testing.T, dir, marker string) string {
	t.Helper()

	bodyRoot := filepath.Join(dir, filepath.FromSlash(rulecond.ClaudeRulesRelDir))
	require.NoError(t, os.MkdirAll(bodyRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bodyRoot, "language-policy.md"),
		[]byte("---\nname: language-policy\nalwaysApply: true\n---\n# Language Policy\n\n"+marker+"\n"),
		0o644))

	manifestPath := filepath.Join(dir, filepath.FromSlash(rulecond.ManifestRelPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	blob, err := json.Marshal(rulecond.Manifest{
		Version: rulecond.ManifestVersion,
		Sticky:  []rulecond.StickyRule{{Name: "language-policy", Body: "language-policy.md"}},
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(manifestPath, blob, 0o644))
	return dir
}

func cliStickyPayload(sessionID, prompt string) string {
	blob, err := json.Marshal(map[string]any{
		"hook_event_name": rulecond.EventUserPromptSubmit,
		"session_id":      sessionID,
		"prompt":          prompt,
		"transcript_path": "/Users/example/transcript.jsonl",
		"cwd":             "/Users/example/private-repo",
	})
	if err != nil {
		panic(err)
	}
	return string(blob)
}

// TestRulesCmd_StickySubcommandRegistered covers the T3 wiring.
func TestRulesCmd_StickySubcommandRegistered(t *testing.T) {
	var found []string
	for _, cmd := range NewRootCmd().Commands() {
		if cmd.Name() == "rules" {
			for _, sub := range cmd.Commands() {
				found = append(found, sub.Name())
			}
		}
	}
	assert.Subset(t, found, []string{"fire", "list", "sticky"},
		"auto rules must expose fire, list, and sticky")
}

// TestRulesSticky_InjectsThroughTheCLI is the S1 oracle at the command boundary.
func TestRulesSticky_InjectsThroughTheCLI(t *testing.T) {
	const marker = "CLI-STICKY-BODY"
	dir := stickyProjectRoot(t, marker)

	stdout, stderr := runRulesSticky(t, dir,
		cliStickyPayload("S-cli", "deploy with EXAMPLEFAKESECRET"))

	var payload struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload),
		"the CLI must print a JSON hook payload on stdout")

	assert.Equal(t, rulecond.EventUserPromptSubmit, payload.HookSpecificOutput.HookEventName)
	assert.Contains(t, payload.HookSpecificOutput.AdditionalContext, marker,
		"the sticky rule body must reach additionalContext")
	assert.Contains(t, payload.HookSpecificOutput.AdditionalContext, "language-policy",
		"the sticky rule must be named")
	assert.NotContains(t, payload.HookSpecificOutput.AdditionalContext, "alwaysApply",
		"only the injectable body enters context, never the frontmatter")
	assert.NotContains(t, stdout, "EXAMPLEFAKESECRET",
		"prompt text must never re-enter context")
	assert.Empty(t, stderr)
}

// TestRulesSticky_OffCadencePromptIsSilent pins the emit-or-empty contract at
// the command boundary: prompt 2 at the default cadence produces nothing.
func TestRulesSticky_OffCadencePromptIsSilent(t *testing.T) {
	dir := stickyProjectRoot(t, "CLI-STICKY-BODY")

	first, _ := runRulesSticky(t, dir, cliStickyPayload("S-cadence", "one"))
	require.NotEmpty(t, first, "prompt index 1 always injects")

	second, stderr := runRulesSticky(t, dir, cliStickyPayload("S-cadence", "two"))

	assert.Empty(t, second, "prompt index 2 is off-cadence at the default cadence of 8")
	assert.Empty(t, stderr)
}

// TestRulesSticky_UnresolvedProjectRootIsObservable is the S13 project-root row
// for REQ-STICKYRULE-FIRE-12: a misconfigured working directory is reported,
// not silently inert.
func TestRulesSticky_UnresolvedProjectRootIsObservable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	start := filepath.Join(home, "work", "sub")
	require.NoError(t, os.MkdirAll(start, 0o755))

	stdout, stderr := runRulesSticky(t, start, cliStickyPayload("S-noroot", "hello"))

	assert.Empty(t, stdout, "no project root means nothing to inject")
	assert.Equal(t, "sticky project_root_unresolved", strings.TrimSpace(stderr),
		"the unresolved root is the one diagnostic this case writes")
}

// TestRulesSticky_HomeConfigRootStillInjects is the issue #185 sticky row. A
// project rooted at the home directory reported `sticky project_root_unresolved`
// on every prompt: `~/.claude` is refused as evidence on purpose, and before the
// fallback nothing else was accepted there, so an installed sticky surface was
// unreachable. The regular `~/autopus.yaml` is the evidence that resolves it,
// and the refusal of `~/.claude` itself is unchanged.
//
// HOME is set to the symlink-evaluated path because os.Getwd returns the
// physical directory; without that the resolver would never recognise the
// working directory as the home directory and the test would pass through the
// `.claude` marker it is meant to exclude.
func TestRulesSticky_HomeConfigRootStillInjects(t *testing.T) {
	const marker = "CLI-HOME-ROOT-BODY"
	home, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	t.Setenv("HOME", home)
	stickyProjectRootIn(t, home, marker)

	withoutConfig, stderrWithoutConfig := runRulesSticky(t, home,
		cliStickyPayload("S-home-noconfig", "hello"))
	require.Empty(t, withoutConfig, "`~/.claude` alone must stay refused as a project root")
	require.Equal(t, unresolvedRootLine, strings.TrimSpace(stderrWithoutConfig))

	require.NoError(t, os.WriteFile(filepath.Join(home, projectConfigMarker),
		[]byte("version: 1\n"), 0o644))

	stdout, stderr := runRulesSticky(t, home, cliStickyPayload("S-home-config", "hello"))

	assert.Empty(t, stderr, "a config-rooted project is resolved, not reported")
	assert.Contains(t, stdout, marker, "the sticky body must reach the payload")
}

// TestRulesSticky_CadenceComesFromTheProjectConfig pins what a project's
// autopus.yaml does to the cadence the CLI actually applies.
//
// The malformed row is the degrade path stickyCadence exists for: sticky_cadence
// is a typed int, so a non-integer scalar fails config load outright. A hook that
// runs before every user turn must then fall back to the shipped default rather
// than suppress the rules it exists to keep present, and the configured row is
// what proves the fallback is the default and not the configured value.
func TestRulesSticky_CadenceComesFromTheProjectConfig(t *testing.T) {
	const marker = "CLI-CADENCE-BODY"
	tests := []struct {
		name             string
		session          string
		cadence          int // 0 writes a malformed document instead of a config
		wantSecondPrompt bool
	}{
		{name: "unreadable cadence degrades to the default of 8", session: "S-cad-bad"},
		{name: "cadence 1 re-attaches on every prompt", session: "S-cad-one", cadence: 1, wantSecondPrompt: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := stickyProjectRoot(t, marker)
			if tt.cadence == 0 {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"),
					[]byte("hooks:\n  sticky_cadence: every-other-prompt\n"), 0o644))
			} else {
				cfg := config.DefaultFullConfig("sticky-cadence-fixture")
				cfg.Hooks.StickyCadence = tt.cadence
				require.NoError(t, config.Save(dir, cfg))
			}

			first, firstErr := runRulesSticky(t, dir, cliStickyPayload(tt.session, "one"))
			require.Contains(t, first, marker, "prompt index 1 always injects")
			require.Empty(t, firstErr)

			second, secondErr := runRulesSticky(t, dir, cliStickyPayload(tt.session, "two"))

			assert.Empty(t, secondErr, "a cadence decision is never a diagnostic")
			if tt.wantSecondPrompt {
				assert.Contains(t, second, marker, "cadence 1 injects on prompt index 2")
				return
			}
			assert.Empty(t, second,
				"an unreadable cadence resolves to the default of 8, so prompt index 2 is off-cadence")
		})
	}
}

// TestRulesList_StickyColumnAndCadenceAreInspectable is the S11 oracle for
// REQ-STICKYRULE-OBS-01.
func TestRulesList_StickyColumnAndCadenceAreInspectable(t *testing.T) {
	output := runRulesListIn(t, t.TempDir())

	assert.Contains(t, strings.ToUpper(output), "STICKY",
		"the listing must carry a sticky column")

	for name := range shippedRuleClassification {
		row := rowFor(t, output, name)
		if name == stickyRuleNames[0] || name == stickyRuleNames[1] {
			assert.Contains(t, row, "true", "%s is sticky", name)
			continue
		}
		assert.NotContains(t, row, "true", "%s is not sticky", name)
		assert.Contains(t, row, "false", "%s must report a sticky value", name)
	}

	var cadenceLine string
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(strings.ToLower(line), "cadence") {
			cadenceLine = line
			break
		}
	}
	require.NotEmpty(t, cadenceLine, "the listing must report the effective cadence:\n%s", output)
	assert.Contains(t, cadenceLine, "8",
		"with no autopus.yaml the effective cadence resolves to the default of 8")
}

// Issue #245: the configured language values travel with the sticky
// language-policy body, so a long session keeps the response language.
func TestRulesSticky_AttachesConfiguredLanguageValues(t *testing.T) {
	dir := stickyProjectRoot(t, "LANG-BODY")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(
		"mode: full\nproject_name: lang\nplatforms: [claude-code]\nlanguage:\n  comments: en\n  commits: en\n  ai_responses: ko\n"), 0o644))

	stdout, stderr := runRulesSticky(t, dir, cliStickyPayload("S-lang", "hi"))

	var payload struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload))
	context := payload.HookSpecificOutput.AdditionalContext
	assert.Contains(t, context, "LANG-BODY")
	assert.Contains(t, context, "- code_comments: en (English)\n- commit_messages: en (English)\n- ai_responses: ko (Korean)\n")
	assert.Contains(t, context, "Write every message to the user in ko (Korean)")
	assert.Empty(t, stderr)
}

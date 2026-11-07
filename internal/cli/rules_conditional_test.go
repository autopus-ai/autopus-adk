package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/rulecond"
)

// shippedRuleClassification is the S11 expected classification per rule.
// The verification harness ships four rules and no skill-scoped one.
var shippedRuleClassification = map[string]string{
	"autonomy":        "always",
	"language-policy": "always",
	"file-size-limit": "paths-scoped",
	"lore-commit":     "hook-fired",
}

// runRulesList executes `auto rules list` and returns its stdout.
func runRulesList(t *testing.T) string {
	t.Helper()
	cmd := newRulesCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"list"})
	require.NoError(t, cmd.Execute())
	return out.String()
}

// rowFor returns the printed row whose first column is the rule name.
func rowFor(t *testing.T, output, name string) string {
	t.Helper()
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == name {
			return line
		}
	}
	t.Fatalf("no row printed for rule %q in:\n%s", name, output)
	return ""
}

// TestRulesList_ClassificationIsInspectable is the S11 oracle for
// REQ-CONDRULE-OBS-01.
func TestRulesList_ClassificationIsInspectable(t *testing.T) {
	output := runRulesList(t)

	counts := map[string]int{}
	for name, want := range shippedRuleClassification {
		row := rowFor(t, output, name)
		assert.Contains(t, row, want, "rule %s classification", name)
		counts[want]++
	}
	assert.Equal(t, 1, counts["hook-fired"])
	assert.Equal(t, 1, counts["paths-scoped"])
	assert.Zero(t, counts["skill-scoped"])
	assert.Equal(t, 2, counts["always"])

	lore := rowFor(t, output, "lore-commit")
	assert.Contains(t, lore, "tool:bash", "hook-fired rules print their trigger")
	assert.Contains(t, lore, ".claude/hooks/autopus/conditional/lore-commit.md",
		"hook-fired rules print a conditional body destination")

	fileSize := rowFor(t, output, "file-size-limit")
	assert.Contains(t, fileSize, ".claude/rules/autopus/file-size-limit.md")

	autonomy := rowFor(t, output, "autonomy")
	assert.Contains(t, autonomy, ".claude/rules/autopus/autonomy.md")
}

// TestRulesFire_MissingManifestExitsZeroWithNoOutput keeps the CLI entry point
// aligned with the REQ-CONDRULE-FIRE-03 fail-open contract.
//
// The fixture carries a marker on purpose. Without one this exercised the
// unresolved-root branch instead, so the missing-manifest silence it claims to
// pin was never reached — and once the unresolved case gained a diagnostic, the
// two cases became observably different.
func TestRulesFire_MissingManifestExitsZeroWithNoOutput(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude"), 0o755))

	stdout, stderr := runRulesFire(t, dir,
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`)

	assert.Empty(t, stdout)
	assert.Empty(t, stderr, "a resolved root with no manifest is benign absence")
}

// TestRulesFire_UnresolvedProjectRootIsObservable is the issue #185 conditional
// row: outside any project the dispatcher used to exit zero with nothing on
// either stream, which is indistinguishable from "no rule matched". One stderr
// line makes the two cases tellable apart without weakening the exit-zero
// contract or naming the working directory.
func TestRulesFire_UnresolvedProjectRootIsObservable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	start := filepath.Join(home, "work", "sub")
	require.NoError(t, os.MkdirAll(start, 0o755))

	stdout, stderr := runRulesFire(t, start,
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`)

	assert.Empty(t, stdout, "no project root means no manifest to read")
	assert.Equal(t, unresolvedConditionalRootLine, strings.TrimSpace(stderr),
		"the unresolved root is the one diagnostic this case writes")
}

// TestRulesFire_ConfigOnlyRootIsResolved is the other half of issue #185: a
// project whose only evidence is autopus.yaml is a project, so the dispatcher
// stops reporting an unresolved root there and falls back to the ordinary
// missing-manifest silence.
func TestRulesFire_ConfigOnlyRootIsResolved(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	project := filepath.Join(home, "proj")
	start := filepath.Join(project, "pkg", "sub")
	require.NoError(t, os.MkdirAll(start, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, projectConfigMarker),
		[]byte("version: 1\n"), 0o644))

	stdout, stderr := runRulesFire(t, start,
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git commit -m x"}}`)

	assert.Empty(t, stdout)
	assert.Empty(t, stderr, "an autopus.yaml root is resolved, not reported")
}

// runRulesFire executes `auto rules fire` in dir with the given stdin and
// returns stdout and stderr.
func runRulesFire(t *testing.T, dir, stdin string) (string, string) {
	t.Helper()
	t.Chdir(dir)

	cmd := newRulesCmd()
	var out, errOut bytes.Buffer
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"fire", "--event", "PreToolUse"})

	require.NoError(t, cmd.Execute(), "the dispatcher must never block a tool call")
	return out.String(), errOut.String()
}

// TestRulesFire_InjectsMatchedRuleThroughTheCLI is the S1 oracle at the command
// boundary the generated PreToolUse hook actually invokes. The pkg/rulecond
// tests call Fire directly, so without this the CLI could resolve the wrong
// root, drop stdin, or write to the wrong stream and still ship green.
func TestRulesFire_InjectsMatchedRuleThroughTheCLI(t *testing.T) {
	dir := t.TempDir()
	bodyRoot := filepath.Join(dir, filepath.FromSlash(rulecond.BodyRootRelPath))
	require.NoError(t, os.MkdirAll(bodyRoot, 0o755))

	const marker = "CLI-LORE-BODY"
	require.NoError(t, os.WriteFile(filepath.Join(bodyRoot, "lore-commit.md"),
		[]byte("# Lore Commit\n\n"+marker+"\n"), 0o644))

	manifest := rulecond.Manifest{Version: "1", Rules: []rulecond.ManifestRule{{
		Name:       "lore-commit",
		Event:      "PreToolUse",
		Matcher:    "Bash",
		Conditions: []string{`\bgit\s+commit\b`},
		Body:       "lore-commit.md",
	}}}
	blob, err := json.Marshal(manifest)
	require.NoError(t, err)
	manifestPath := filepath.Join(dir, filepath.FromSlash(rulecond.ManifestRelPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, blob, 0o644))

	stdout, stderr := runRulesFire(t, dir,
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"git commit -m \"feat(x): y\""}}`)

	var payload struct {
		HookSpecificOutput struct {
			HookEventName     string `json:"hookEventName"`
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload),
		"the CLI must print a JSON hook payload on stdout")

	assert.Equal(t, "PreToolUse", payload.HookSpecificOutput.HookEventName)
	assert.Contains(t, payload.HookSpecificOutput.AdditionalContext, marker,
		"the matched rule body must reach additionalContext")
	assert.Contains(t, payload.HookSpecificOutput.AdditionalContext, "lore-commit",
		"the matched rule must be named")
	assert.NotContains(t, payload.HookSpecificOutput.AdditionalContext, "git commit -m",
		"tool input must never re-enter context")
	assert.Empty(t, stderr)
}

// TestRulesFire_NonMatchingCommandStaysSilentThroughTheCLI is the negative
// control: the CLI must not emit a payload when nothing matches.
func TestRulesFire_NonMatchingCommandStaysSilentThroughTheCLI(t *testing.T) {
	dir := t.TempDir()
	bodyRoot := filepath.Join(dir, filepath.FromSlash(rulecond.BodyRootRelPath))
	require.NoError(t, os.MkdirAll(bodyRoot, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bodyRoot, "lore-commit.md"),
		[]byte("# Lore Commit\n\nBODY\n"), 0o644))

	blob, err := json.Marshal(rulecond.Manifest{Version: "1", Rules: []rulecond.ManifestRule{{
		Name:       "lore-commit",
		Event:      "PreToolUse",
		Matcher:    "Bash",
		Conditions: []string{`\bgit\s+commit\b`},
		Body:       "lore-commit.md",
	}}})
	require.NoError(t, err)
	manifestPath := filepath.Join(dir, filepath.FromSlash(rulecond.ManifestRelPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(manifestPath), 0o755))
	require.NoError(t, os.WriteFile(manifestPath, blob, 0o644))

	stdout, stderr := runRulesFire(t, dir,
		`{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls -la"}}`)

	assert.Empty(t, stdout, "a non-match must produce no payload")
	assert.Empty(t, stderr)
}

// TestRulesCmd_RegisteredOnRoot covers the T6 wiring.
func TestRulesCmd_RegisteredOnRoot(t *testing.T) {
	var found []string
	for _, cmd := range NewRootCmd().Commands() {
		if cmd.Name() == "rules" {
			for _, sub := range cmd.Commands() {
				found = append(found, sub.Name())
			}
		}
	}
	assert.Subset(t, found, []string{"fire", "list"},
		"auto rules must expose fire and list")
}

// TestHygieneSourceMapping_ConditionalPaths covers T9: the generated conditional
// paths map back to the rule source, so drift detection does not orphan them.
func TestHygieneSourceMapping_ConditionalPaths(t *testing.T) {
	for _, generated := range []string{
		".claude/hooks/autopus/conditional/lore-commit.md",
		".claude/hooks/autopus/conditional-rules.json",
	} {
		prefixes := sourcePrefixesForGenerated(generated)
		assert.Contains(t, prefixes, "content/rules/", "source mapping for %s", generated)
		assert.True(t, sourceMatchesGenerated(generated, "content/rules/lore-commit.md"),
			"%s must be attributable to a rule source edit", generated)
	}
}

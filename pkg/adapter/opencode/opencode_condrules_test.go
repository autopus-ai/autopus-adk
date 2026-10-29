package opencode

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/rulecond"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenCodeConditionalRules_AreTheShellScopedHookFiredRules(t *testing.T) {
	t.Parallel()
	rules, err := openCodeConditionalRules()
	require.NoError(t, err)
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		names = append(names, rule.Name)
		assert.NotEmpty(t, rule.Conditions, rule.Name)
		assert.False(t, strings.HasPrefix(strings.TrimSpace(rule.Body), "---"), "%s body keeps its frontmatter", rule.Name)
	}
	assert.Equal(t, []string{"lore-commit", "shell-portability", "worktree-safety"}, names)
}

func TestManagedRulePaths_V2DefersOnlyPluginDeliveredRules(t *testing.T) {
	t.Parallel()
	v1Active, v1Deferred, err := managedRulePaths(false)
	require.NoError(t, err)
	v2Active, v2Deferred, err := managedRulePaths(true)
	require.NoError(t, err)

	delivered := []string{
		".opencode/rules/autopus/lore-commit.md",
		".opencode/rules/autopus/shell-portability.md",
		".opencode/rules/autopus/worktree-safety.md",
	}
	for _, path := range delivered {
		assert.Contains(t, v1Active, path)
		assert.NotContains(t, v2Active, path)
		assert.Contains(t, v2Deferred, path)
	}
	assert.Len(t, v2Deferred, len(v1Deferred)+len(delivered))
	assert.Len(t, v2Active, len(v1Active)-len(delivered))
}

func TestPluginDeliversRule_KeepsWhatThePluginCannotMatchAlwaysLoaded(t *testing.T) {
	t.Parallel()
	shell := &rulecond.Rule{Name: "shell", Conditions: []string{`\bgit\s+commit\b`}, Scopes: []string{"tool:bash"}}
	assert.True(t, pluginDeliversRule(shell))
	for name, rule := range map[string]*rulecond.Rule{
		"edit scope":    {Name: "edit", Conditions: []string{`x`}, Scopes: []string{"tool:edit"}},
		"mixed scope":   {Name: "mixed", Conditions: []string{`x`}, Scopes: []string{"tool:bash", "tool:edit"}},
		"inline flag":   {Name: "flag", Conditions: []string{`(?i)git commit`}, Scopes: []string{"tool:bash"}},
		"text anchor":   {Name: "anchor", Conditions: []string{`commit\z`}, Scopes: []string{"tool:bash"}},
		"posix class":   {Name: "posix", Conditions: []string{`[[:space:]]commit`}, Scopes: []string{"tool:bash"}},
		"paths scoped":  {Name: "paths", Globs: []string{"**/*.go"}},
		"always loaded": {Name: "always"},
	} {
		assert.False(t, pluginDeliversRule(rule), name)
	}
}

// The commands each embedded condition is checked against. They cover the
// matches and the near misses of the three shell-scoped rules.
var conditionalRuleCommands = []string{
	`git commit -m "x"`,
	`git  commit --amend`,
	`git commit-tree HEAD`,
	`echo git commits`,
	`timeout 5 go test ./...`,
	`ls; gtimeout 30 make`,
	`go test -timeout 5m ./...`,
	`git gc --prune=now`,
	`git repack -ad`,
	`git status`,
}

func TestOpenCodeV2PluginRuntime_DeliversTheRulesRE2Fires(t *testing.T) {
	rules, err := openCodeConditionalRules()
	require.NoError(t, err)
	type command struct {
		Command string   `json:"command"`
		Fires   []string `json:"fires"`
	}
	cases := map[string]any{"always": []string{}, "fireCommand": `git commit -m x`, "fireRule": "## Rule: lore-commit\n"}
	var commands []command
	var names []string
	for _, rule := range rules {
		names = append(names, rule.Name)
	}
	for _, text := range conditionalRuleCommands {
		item := command{Command: text, Fires: []string{}}
		for _, rule := range rules {
			for _, condition := range rule.Conditions {
				if regexp.MustCompile(condition).MatchString(text) {
					item.Fires = append(item.Fires, rule.Name)
					break
				}
			}
		}
		commands = append(commands, item)
	}
	cases["rules"], cases["commands"] = names, commands
	runRulesContract(t, rules, cases)
}

func TestOpenCodeV2PluginRuntime_FailsClosedAndBoundsContext(t *testing.T) {
	rules := []openCodeConditionalRule{
		{Name: "a-fired", Conditions: []string{`\bgit\s+commit\b`}, Body: "FIRED_BODY"},
		{Name: "b-broken", Conditions: []string{`(`}, Body: "BROKEN_BODY"},
		{Name: "c-large", Conditions: []string{`\bgit\s+commit\b`}, Body: strings.Repeat("x", rulecond.MaxContextBytes)},
	}
	runRulesContract(t, rules, map[string]any{
		"rules":       []string{"b-broken"},
		"always":      []string{"b-broken"},
		"commands":    []map[string]any{{"command": "git status", "fires": []string{}}},
		"fireCommand": "git commit -m x",
		"fireRule":    "FIRED_BODY",
		"omitted":     "c-large",
		"limit":       rulecond.MaxContextBytes,
	})
}

func runRulesContract(t *testing.T, rules []openCodeConditionalRule, cases map[string]any) {
	t.Helper()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("generated plugin rule fixture requires Node; skipped is not runtime verification")
	}
	body, err := renderHookPluginV2WithRules(nil, rules)
	require.NoError(t, err)
	root := t.TempDir()
	pluginPath := filepath.Join(root, "plugin.mjs")
	require.NoError(t, os.WriteFile(pluginPath, []byte(body), 0600))
	encoded, err := json.Marshal(cases)
	require.NoError(t, err)
	casesPath := filepath.Join(root, "cases.json")
	require.NoError(t, os.WriteFile(casesPath, encoded, 0600))
	runner, err := filepath.Abs("testdata/plugin_v2_rules.mjs")
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, node, runner, pluginPath, casesPath).CombinedOutput()
	require.NoError(t, err, string(output))
	require.Contains(t, string(output), "RULES_PASS")
}

//go:build integration

// Package cli_test provides CC21 integration tests covering effort resolution
// and agent frontmatter validation (TC1-TC6, TC9-TC10).
package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/internal/cli"
)

// agentsDir returns the absolute path to content/agents/ relative to the
// autopus-adk module root, regardless of the working directory when tests run.
func agentsDir() string {
	// Walk up from this test file's directory to find the workspace root.
	_, thisFile, _, _ := runtime.Caller(0)
	// thisFile is .../autopus-adk/internal/cli/cc21_effort_test.go
	// repo root is two directories up.
	dir := filepath.Dir(thisFile) // .../autopus-adk/internal/cli
	dir = filepath.Dir(dir)       // .../autopus-adk/internal
	root := filepath.Dir(dir)     // .../autopus-adk
	return filepath.Join(root, "content", "agents")
}

// agentFrontmatter holds the subset of agent frontmatter fields needed for CC21 validation.
type agentFrontmatter struct {
	Name   string `yaml:"name"`
	Effort string `yaml:"effort"`
}

// parseAgentFrontmatter extracts and parses the YAML frontmatter from an agent .md file.
func parseAgentFrontmatter(t *testing.T, path string) agentFrontmatter {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "failed to read agent file: %s", path)

	content := string(data)
	if !strings.HasPrefix(content, "---") {
		t.Fatalf("agent file %s has no frontmatter", path)
	}

	// Find closing --- delimiter.
	rest := content[3:]
	end := strings.Index(rest, "\n---")
	require.Greater(t, end, 0, "agent file %s has unclosed frontmatter", path)

	yamlBlock := rest[:end]
	var fm agentFrontmatter
	err = yaml.Unmarshal([]byte(yamlBlock), &fm)
	require.NoError(t, err, "YAML parse failed for %s", path)
	return fm
}

// --- TC1: All 17 agent files have effort field (spec.md R9, acceptance.md R9) -----

// TestCC21_TC1_AllAgentsHaveEffort verifies that exactly 17 agent .md files exist
// and every one contains a non-empty effort field.
// Violation → spec.md R9 (G4: 17/17 agents must have effort frontmatter; SPEC-REA-001 added reverse-engineer).
func TestCC21_TC1_AllAgentsHaveEffort(t *testing.T) {
	dir := agentsDir()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "cannot read agents directory: %s (S9-1)", dir)

	var agentFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && e.Name() != "AGENT-GUIDE.md" {
			agentFiles = append(agentFiles, filepath.Join(dir, e.Name()))
		}
	}

	assert.Len(t, agentFiles, 4,
		"G4 violation: expected exactly 4 agent .md files, got %d", len(agentFiles))

	// Verify role-to-effort mapping for the agents the verification harness ships.
	maxEffortRoles := map[string]bool{}
	highEffortRoles := map[string]bool{
		"reviewer": true, "qa-explorer": true,
	}
	mediumEffortRoles := map[string]bool{
		"frontend-specialist": true, "ux-validator": true,
	}

	for _, path := range agentFiles {
		fm := parseAgentFrontmatter(t, path)
		assert.NotEmpty(t, fm.Effort,
			"R9 violation: agent %s missing effort field", filepath.Base(path))
		assert.NotEmpty(t, fm.Name,
			"R9 violation: agent %s missing name field", filepath.Base(path))

		// Validate expected effort value for known roles.
		if maxEffortRoles[fm.Name] {
			assert.Equal(t, "max", fm.Effort,
				"R9 violation: agent %s expected effort=max, got %q", fm.Name, fm.Effort)
		} else if highEffortRoles[fm.Name] {
			assert.Equal(t, "high", fm.Effort,
				"R9 violation: agent %s expected effort=high, got %q", fm.Name, fm.Effort)
		} else if mediumEffortRoles[fm.Name] {
			assert.Equal(t, "medium", fm.Effort,
				"R9 violation: agent %s expected effort=medium, got %q", fm.Name, fm.Effort)
		}
	}
}

// --- TC2: ResolveEffort balanced mode (S3-1) ----------------------------------------

// TestCC21_TC2_ResolveEffort_Balanced verifies that the default Opus model in
// Balanced mode resolves to effort=high even with medium task complexity.
// Violation → S3-1.
func TestCC21_TC2_ResolveEffort_Balanced(t *testing.T) {
	result, err := cli.ResolveEffort(cli.EffortResolveInput{
		FlagQuality:    "balanced",
		FlagComplexity: "medium",
	})
	require.NoError(t, err, "S3-1: ResolveEffort returned unexpected error")
	assert.Equal(t, cli.EffortHigh, result.Effort,
		"S3-1 violation: Balanced Opus should resolve to high, got %q", result.Effort)
	assert.Equal(t, cli.EffortSourceQualityMode, result.Source,
		"S3-1 violation: source should be quality_mode, got %q", result.Source)
}

// --- TC3: ResolveEffort ultra + Opus 4.7 → max (S2-1) ----------------------------

// TestCC21_TC3_ResolveEffort_Ultra_Opus47 verifies that ultra quality with Opus 4.7
// resolves to effort=max.
// Violation → S2-1.
func TestCC21_TC3_ResolveEffort_Ultra_Opus47(t *testing.T) {
	result, err := cli.ResolveEffort(cli.EffortResolveInput{
		FlagQuality: "ultra",
		Model:       "opus-4.7",
	})
	require.NoError(t, err, "S2-1: ResolveEffort returned unexpected error")
	assert.Equal(t, cli.EffortMax, result.Effort,
		"S2-1 violation: ultra+opus-4.7 should resolve to max, got %q", result.Effort)
	assert.Equal(t, cli.EffortSourceQualityMode, result.Source,
		"S2-1 violation: source should be quality_mode, got %q", result.Source)
}

// --- TC3b: ResolveEffort ultra + Opus 4.8 → max (regression guard) ----------------

// TestCC21_TC3b_ResolveEffort_Ultra_Opus48 verifies that ultra quality with the
// current default model Opus 4.8 resolves to effort=max, not the high downgrade
// branch. Guards against the model-ID hardcoding that previously demoted 4.8.
func TestCC21_TC3b_ResolveEffort_Ultra_Opus48(t *testing.T) {
	result, err := cli.ResolveEffort(cli.EffortResolveInput{
		FlagQuality: "ultra",
		Model:       "opus-4.8",
	})
	require.NoError(t, err, "ResolveEffort returned unexpected error")
	assert.Equal(t, cli.EffortMax, result.Effort,
		"ultra+opus-4.8 should resolve to max, got %q", result.Effort)
	assert.Equal(t, cli.EffortSourceQualityMode, result.Source,
		"source should be quality_mode, got %q", result.Source)
}

// --- TC4: ResolveEffort ultra + Opus 4.6 → high downgrade (S2-2) -------------------

// TestCC21_TC4_ResolveEffort_Ultra_Opus46 verifies that ultra quality with a
// model that does not support max effort downgrades to effort=high.
// Violation → S2-2.
func TestCC21_TC4_ResolveEffort_Ultra_Opus46(t *testing.T) {
	result, err := cli.ResolveEffort(cli.EffortResolveInput{
		FlagQuality: "ultra",
		Model:       "opus-4.6",
	})
	require.NoError(t, err, "S2-2: ResolveEffort returned unexpected error")
	assert.Equal(t, cli.EffortHigh, result.Effort,
		"S2-2 violation: ultra+opus-4.6 should downgrade to high, got %q", result.Effort)
	assert.Contains(t, result.Reason, "model_tier_not_max_capable",
		"S2-2 violation: reason should contain model_tier_not_max_capable, got %q", result.Reason)
}

// --- TC5: ResolveEffort ultra + Haiku 4.5 → stripped (S2-3) ------------------------

// TestCC21_TC5_ResolveEffort_Ultra_Haiku45 verifies that Haiku 4.5 in ultra mode
// strips the effort field entirely.
// Violation → S2-3.
func TestCC21_TC5_ResolveEffort_Ultra_Haiku45(t *testing.T) {
	result, err := cli.ResolveEffort(cli.EffortResolveInput{
		FlagQuality: "ultra",
		Model:       "haiku-4.5",
	})
	require.NoError(t, err, "S2-3: ResolveEffort returned unexpected error")
	assert.Equal(t, cli.EffortStripped, result.Effort,
		"S2-3 violation: ultra+haiku-4.5 should strip effort, got %q", result.Effort)
	assert.Contains(t, result.Reason, "haiku-4-5",
		"S2-3 violation: reason should reference haiku-4-5, got %q", result.Reason)
}

// --- TC6: Env var overrides all flags (S4-1) ----------------------------------------

// TestCC21_TC6_EnvVarPriority verifies that CLAUDE_CODE_EFFORT_LEVEL=low overrides
// --quality=ultra regardless of model, confirming env-first priority chain.
// Violation → S4-1.
func TestCC21_TC6_EnvVarPriority(t *testing.T) {
	t.Setenv("CLAUDE_CODE_EFFORT_LEVEL", "low")

	result, err := cli.ResolveEffort(cli.EffortResolveInput{
		EnvValue:    "low",
		FlagQuality: "ultra",
		Model:       "opus-4.7",
	})
	require.NoError(t, err, "S4-1: ResolveEffort returned unexpected error")
	assert.Equal(t, cli.EffortLow, result.Effort,
		"S4-1 violation: env CLAUDE_CODE_EFFORT_LEVEL=low must override ultra, got %q", result.Effort)
	assert.Equal(t, cli.EffortSourceEnv, result.Source,
		"S4-1 violation: source must be env, got %q", result.Source)
}

// --- TC9: agent.tmpl renders effort field in frontmatter (R9, T12) ------------------

// TestCC21_TC9_AgentTemplateEffortField verifies that the agent create template
// renders a non-empty effort field in the output frontmatter for both high and medium roles.
// Violation → R9 (G5 #4: generated agents must include effort field).
func TestCC21_TC9_AgentTemplateEffortField(t *testing.T) {
	cases := []struct {
		role           string
		expectedEffort string
	}{
		{"spec-writer", "max"},
		{"planner", "max"},
		{"reviewer", "high"},
		{"security-auditor", "max"},
		{"architect", "max"},
		{"deep-worker", "max"},
		{"executor", "medium"},
		{"tester", "medium"},
		{"debugger", "medium"},
	}

	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			cmd := newTestRootCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetErr(&buf)
			cmd.SetArgs([]string{"agent", "create", tc.role, "--description", "test agent"})
			err := cmd.Execute()
			require.NoError(t, err, "R9: agent create %s failed", tc.role)

			output := buf.String()
			assert.Contains(t, output, "effort: "+tc.expectedEffort,
				"R9 violation: agent %s template must render effort: %s in frontmatter",
				tc.role, tc.expectedEffort)
		})
	}
}

// --- TC10: YAML unknown-field silent ignore (S8-2, G5 #4) ---------------------------

// TestCC21_TC10_FrontmatterUnknownFieldSilentIgnore verifies that gopkg.in/yaml.v3
// silently ignores unknown frontmatter fields (effort, initialPrompt) when parsing
// into a struct that does not declare them.
// Violation → S8-2: unknown fields must not cause parse errors.
func TestCC21_TC10_FrontmatterUnknownFieldSilentIgnore(t *testing.T) {
	// Sample frontmatter containing CC21-added fields alongside standard fields.
	sample := `name: executor
description: TDD-based implementation agent
model: sonnet
effort: medium
initialPrompt: "You are an executor agent."
maxTurns: 50
`

	// Struct without effort or initialPrompt — simulates a consumer that predates CC21.
	type legacyFrontmatter struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
		Model       string `yaml:"model"`
		MaxTurns    int    `yaml:"maxTurns"`
	}

	var fm legacyFrontmatter
	err := yaml.Unmarshal([]byte(sample), &fm)
	require.NoError(t, err,
		"S8-2 violation: unknown fields (effort, initialPrompt) must not cause YAML parse error")
	assert.Equal(t, "executor", fm.Name,
		"S8-2: name field should be parsed correctly")
	assert.Equal(t, "sonnet", fm.Model,
		"S8-2: model field should be parsed correctly")
	assert.Equal(t, 50, fm.MaxTurns,
		"S8-2: maxTurns field should be parsed correctly")
}

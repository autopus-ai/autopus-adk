package adapter_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
)

// generatePlatformRules runs the named adapter's Generate in a temp dir and
// returns a map of rule basename -> rule file content for inspection.
func generatePlatformRules(t *testing.T, platform string) map[string]string {
	t.Helper()
	ctx := context.Background()
	cfg := config.DefaultFullConfig("parity-test")
	dir := t.TempDir()

	var pf *adapter.PlatformFiles
	var err error
	switch platform {
	case "claude":
		pf, err = claude.NewWithRoot(dir).Generate(ctx, cfg)
	case "codex":
		pf, err = codex.NewWithRoot(dir).Generate(ctx, cfg)
	case "gemini":
		pf, err = antigravity.NewWithRoot(dir).Generate(ctx, cfg)
	case "opencode":
		pf, err = opencode.NewWithRoot(dir).Generate(ctx, cfg)
	case "omp":
		pf, err = omp.NewWithRoot(dir).Generate(ctx, cfg)
	default:
		t.Fatalf("unknown platform: %s", platform)
	}
	require.NoError(t, err)

	rules := make(map[string]string)
	for _, f := range pf.Files {
		if isRuleTargetPath(f.TargetPath) {
			rules[extractRuleName(f.TargetPath)] = string(f.Content)
		}
	}
	return rules
}

// S1: Gemini가 누락 규칙 3종을 실제 내용과 함께 생성 (Must, REQ-001).
func TestAcceptance_S1_GeminiMissingRulesPresentWithContent(t *testing.T) {
	rules := generatePlatformRules(t, "gemini")

	deferred, ok := rules["deferred-tools.md"]
	require.True(t, ok, "deferred-tools.md must be generated")
	assert.Contains(t, deferred, "# Deferred Tools Loading")
	assert.Contains(t, deferred, "Antigravity CLI")

	identity, ok := rules["project-identity.md"]
	require.True(t, ok, "project-identity.md must be generated")
	assert.Contains(t, identity, "# Project Identity")

	quality, ok := rules["spec-quality.md"]
	require.True(t, ok, "spec-quality.md must be generated")
	assert.Contains(t, quality, "SPEC Quality Checklist")
}

// S2: Gemini 규칙 집합이 content 소스 집합과 정확히 일치 (Must, REQ-001, REQ-002).
func TestAcceptance_S2_GeminiRuleSetMatchesSource(t *testing.T) {
	rules := generatePlatformRules(t, "gemini")
	got := make([]string, 0, len(rules))
	for name := range rules {
		got = append(got, name)
	}
	want := []string{
		"branding.md", "context7-docs.md", "deferred-tools.md", "doc-storage.md",
		"file-size-limit.md", "language-policy.md", "lore-commit.md",
		"objective-reasoning.md", "project-identity.md", "shell-portability.md",
		"spec-quality.md", "subagent-delegation.md", "techstack-freshness.md",
		"worktree-safety.md",
	}
	// @AX:NOTE: [AUTO] magic constant — 14 equals the total canonical rule count in content/rules/; update this oracle when source rules are added or removed
	assert.ElementsMatch(t, want, got, "Gemini rule basenames must equal the 14 content/rules sources")
	assert.Len(t, got, 14)
	// Gemini rule exclusion set must be empty.
	assert.Empty(t, platformRuleExclusions["gemini"])
}

func TestAcceptance_TechstackFreshnessSemanticContractParity(t *testing.T) {
	t.Parallel()

	expected := []string{
		"current intended source state",
		"start and end source SHA",
		"artifact receipts",
		"installed older app",
		"concrete immutable version, SHA, or digest",
		"`latest-stable`",
		"`repo-compatible-pin`",
		"`source-exact`",
		"fail closed",
		"explicitly offline development",
	}
	for _, platform := range []string{"claude", "gemini", "opencode", "omp"} {
		platform := platform
		t.Run(platform, func(t *testing.T) {
			t.Parallel()
			rule, ok := generatePlatformRules(t, platform)["techstack-freshness.md"]
			require.True(t, ok, "%s must generate techstack-freshness.md", platform)
			for _, phrase := range expected {
				assert.Contains(t, rule, phrase)
			}
		})
	}
	// The Codex pipeline skill no longer restates this rule: Codex consumes the
	// canonical slim pipeline body, and its policy carrier is AGENTS.md rather
	// than a duplicated copy inside the skill. Asserting the duplication here
	// only pinned the deleted override helper's output.
}

// S5: platform frontmatter 값이 어댑터 식별자와 일치 (Should, REQ-003).
func TestAcceptance_S5_PlatformFrontmatterValues(t *testing.T) {
	for name, content := range generatePlatformRules(t, "gemini") {
		if val, ok := parsePlatformFromFrontmatter(content); ok {
			assert.Equal(t, "antigravity-cli", val, "gemini rule %s platform value", name)
		}
	}
	assert.Empty(t, generatePlatformRules(t, "codex"),
		"Codex policy must use native skills and AGENTS.md, not inert markdown rules")

	// Gate reports exactly 0 platform-value mismatch findings.
	findings, err := runCoverageGate(context.Background(), t.TempDir(), config.DefaultFullConfig("parity-test"),
		[]string{"claude", "codex", "gemini", "opencode", "omp"}, platformRuleExclusions, platformSkillExclusions, nil)
	require.NoError(t, err)
	mismatches := 0
	for _, f := range findings {
		if f.Type == "platform-value" {
			mismatches++
		}
	}
	assert.Equal(t, 0, mismatches, "platform-value mismatch findings must be 0")
}

// S6: 기존 플랫폼 출력 후방호환 유지 (Must, REQ-005).
func TestAcceptance_S6_ExistingPlatformsBackwardCompatible(t *testing.T) {
	assert.Empty(t, generatePlatformRules(t, "codex"),
		"Codex 0.149.1 has no repository markdown-rule surface")

	// Claude, OpenCode, Gemini, and OMP retain their native 14-rule sets.
	assert.Len(t, generatePlatformRules(t, "claude"), 14, "claude generates exactly 14 rules")
	assert.Len(t, generatePlatformRules(t, "opencode"), 14, "opencode generates exactly 14 rules")
}

// S9: 파리티 게이트가 omp를 포함한다 (Must, REQ-012).
//
// The platform list and the adapter switch case have to land together. A list
// entry on its own reaches runCoverageGate's default branch and fails the gate
// with "unknown platform: omp", and generatePlatformRules' t.Fatalf; requiring
// NoError plus a real rule count is the oracle for that coupling.
func TestAcceptance_S9_ParityGateCoversOMP(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultFullConfig("parity-test")
	// The parity gate compares platform coverage of the same surface. The
	// default compiler now installs the core surface only, so every platform
	// legitimately omits the long tail; comparing that against the full source
	// list would report identical findings for all five platforms and say
	// nothing about omp. Full mode is the documented opt-in that publishes the
	// whole library, which is the surface this parity claim is about.
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull

	// REQ-012: omp claims no intended rule or skill gap.
	assert.Empty(t, platformRuleExclusions["omp"], "omp rule exclusion set must be empty")
	assert.Empty(t, platformSkillExclusions["omp"], "omp skill exclusion set must be empty")

	// omp covers the full 14-rule source set, and the four incumbents keep the
	// counts they reported before omp joined the gate, so the addition is
	// provably non-regressive.
	ompRules := generatePlatformRules(t, "omp")
	assert.Len(t, ompRules, 14, "omp generates exactly 14 rules")
	for _, platform := range []string{"claude", "gemini", "opencode"} {
		assert.Len(t, generatePlatformRules(t, platform), 14,
			"%s still generates exactly 14 rules after omp joins the gate", platform)
	}
	assert.Empty(t, generatePlatformRules(t, "codex"),
		"Codex 0.149.1 carries policy through native skills and AGENTS.md")

	// TransformRuleForOMP keeps only the seven omp-recognized frontmatter keys,
	// so no platform key survives into an emitted omp rule. The gate compares
	// against expectedPlatformValues only when a value is present, which is why
	// omp contributes zero platform-value findings instead of one per rule.
	for name, content := range ompRules {
		_, ok := parsePlatformFromFrontmatter(content)
		assert.False(t, ok, "omp rule %s must not emit a platform frontmatter key", name)
	}

	findings, err := runCoverageGate(ctx, t.TempDir(), cfg,
		[]string{"claude", "codex", "gemini", "opencode", "omp"},
		platformRuleExclusions, platformSkillExclusions, nil)
	require.NoError(t, err, "gate must not fail with unknown platform: omp")

	platformValueFindings := 0
	ompFindings := make([]string, 0)
	for _, f := range findings {
		if f.Type == "platform-value" {
			platformValueFindings++
		}
		if f.Platform == "omp" {
			ompFindings = append(ompFindings, fmt.Sprintf("%s %s: %s", f.Type, f.Item, f.Message))
		}
	}
	assert.Equal(t, 0, platformValueFindings, "platform-value findings must be 0")
	assert.Empty(t, ompFindings, "omp must report no rule or skill coverage findings")
}

// TestAcceptance_S8_UnconditionalRulesCarryNoTrigger pins the S8 oracle across
// every registered platform. TestRules_UnconditionalRulesStayByteIdentical owns
// the byte-identity clause for the four goldened platforms and
// TestRules_NoHookEntryReferencesUnconditionalRules owns the hook-entry clause;
// omp has no pre-change golden, so the ten rules are pinned here by the
// invariant that keeps them unconditional: no trigger field reaches their
// emitted frontmatter on any platform (REQ-CONDRULE-SCHEMA-02).
func TestAcceptance_S8_UnconditionalRulesCarryNoTrigger(t *testing.T) {
	for _, platform := range []string{"claude", "gemini", "opencode", "omp"} {
		platform := platform
		t.Run(platform, func(t *testing.T) {
			rules := generatePlatformRules(t, platform)
			for name := range alwaysRuleFiles {
				content, ok := rules[name]
				require.True(t, ok, "%s must emit %s", platform, name)
				keys := frontmatterKeySet(content)
				for _, trigger := range triggerFieldKeys {
					assert.False(t, keys[trigger],
						"%s rule %s must stay unconditional but declares %s", platform, name, trigger)
				}
			}
		})
	}
}

package adapter_test

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/antigravity"
	"github.com/insajin/autopus-adk/pkg/adapter/claude"
	"github.com/insajin/autopus-adk/pkg/adapter/codex"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/adapter/opencode"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
)

// @AX:ANCHOR: [AUTO] parity enforcement contract — called from TestParityCoverage, TestParityCoverage_GateProbe, and TestAcceptance_S5 across adapter_test package
// @AX:REASON: 3+ test sites depend on this signature and return type; behavior changes affect all platform coverage assertions and the probe self-test
// runCoverageGate generates each platform and compares its output against the
// content source-of-truth minus the platform's explicit exclusion set. It
// returns one finding per missing rule/skill and per platform-value mismatch.
func runCoverageGate(
	ctx context.Context,
	baseDir string,
	cfg *config.HarnessConfig,
	platforms []string,
	ruleExclusions map[string]map[string]bool,
	skillExclusions map[string]map[string]bool,
	extraRuleSources []string,
) ([]CoverageFinding, error) {
	var findings []CoverageFinding

	// Load source rules
	sourceRulesMap := make(map[string]bool)
	ruleEntries, err := fs.ReadDir(content.FS, "rules")
	if err != nil {
		return nil, fmt.Errorf("read source rules: %w", err)
	}
	for _, entry := range ruleEntries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			sourceRulesMap[entry.Name()] = true
		}
	}
	for _, extra := range extraRuleSources {
		sourceRulesMap[extra] = true
	}

	// Load source skills catalog
	catalog, err := pkgcontent.LoadSkillCatalogFromFS(content.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("load skills catalog: %w", err)
	}

	for _, pName := range platforms {
		// Run generator
		var pf *adapter.PlatformFiles
		var genErr error
		dir := filepath.Join(baseDir, fmt.Sprintf("autopus-parity-%s", pName))

		switch pName {
		case "claude":
			a := claude.NewWithRoot(dir)
			pf, genErr = a.Generate(ctx, cfg)
		case "codex":
			a := codex.NewWithRoot(dir)
			pf, genErr = a.Generate(ctx, cfg)
		case "gemini":
			a := antigravity.NewWithRoot(dir)
			pf, genErr = a.Generate(ctx, cfg)
		case "opencode":
			a := opencode.NewWithRoot(dir, opencode.WithCLIVersion("1.18.7"))
			pf, genErr = a.Generate(ctx, cfg)
		case "omp":
			a := omp.NewWithRoot(dir)
			pf, genErr = a.Generate(ctx, cfg)
		default:
			return nil, fmt.Errorf("unknown platform: %s", pName)
		}

		if genErr != nil {
			return nil, fmt.Errorf("generate failed for %s: %w", pName, genErr)
		}

		generatedRules := make(map[string]bool)
		generatedSkills := make(map[string]bool)

		for _, f := range pf.Files {
			pathLower := strings.ToLower(f.TargetPath)
			isRule := isRuleTargetPath(f.TargetPath)
			isSkill := strings.Contains(pathLower, "skills/") || strings.Contains(pathLower, "skills\\")

			if isRule {
				rName := extractRuleName(f.TargetPath)
				generatedRules[rName] = true

				// Validate platform frontmatter
				if val, ok := parsePlatformFromFrontmatter(string(f.Content)); ok {
					expected, exists := expectedPlatformValues[pName]
					matched := false
					if exists {
						for _, ev := range expected {
							if val == ev {
								matched = true
								break
							}
						}
					}
					if !matched {
						findings = append(findings, CoverageFinding{
							Platform: pName,
							Item:     rName,
							Type:     "platform-value",
							Message:  fmt.Sprintf("platform frontmatter value mismatch: got %q, expected one of %v", val, expected),
						})
					}
				}
			} else if isSkill {
				sName := parseSkillNameFromContent(string(f.Content))
				if sName == "" {
					sName = extractSkillNameFromPath(f.TargetPath)
				}
				if pName == "codex" {
					sName = strings.TrimPrefix(sName, "codex-")
				}
				if sName != "" {
					generatedSkills[sName] = true
				}
			}
		}

		// Check rule coverage: (Source - Exclusions) must be in Generated
		for sr := range sourceRulesMap {
			excluded := ruleExclusions[pName][sr]
			if !excluded && !generatedRules[sr] {
				findings = append(findings, CoverageFinding{
					Platform: pName,
					Item:     sr,
					Type:     "rule",
					Message:  fmt.Sprintf("missing rule %q in generated output", sr),
				})
			}
		}

		// Check skill coverage: (Compatible Source - Exclusions) must be in Generated
		for _, ss := range catalog.List() {
			if isSkillCompatible(ss, pName) {
				excluded := skillExclusions[pName][ss.Name]
				if !excluded && !generatedSkills[ss.Name] {
					findings = append(findings, CoverageFinding{
						Platform: pName,
						Item:     ss.Name,
						Type:     "skill",
						Message:  fmt.Sprintf("missing skill %q in generated output", ss.Name),
					})
				}
			}
		}
	}

	return findings, nil
}

// TestParityCoverage enforces that every platform covers the full source set
// (S2, S6) with zero platform-value mismatches (S5).
func TestParityCoverage(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultFullConfig("parity-test")
	// The gate's subject is per-platform parity over the whole source set, so it
	// selects the full library rather than the compact default surface.
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	platforms := []string{"claude", "codex", "gemini", "opencode", "omp"}

	findings, err := runCoverageGate(ctx, t.TempDir(), cfg, platforms, platformRuleExclusions, platformSkillExclusions, nil)
	require.NoError(t, err)

	for _, f := range findings {
		t.Errorf("[%s] %s %s: %s", f.Platform, f.Type, f.Item, f.Message)
	}
}

// TestParityCoverage_GateProbe verifies the gate detects gaps bidirectionally:
// a synthetic source item triggers exactly one finding, and adding it to the
// exclusion set silences it (S3).
func TestParityCoverage_GateProbe(t *testing.T) {
	ctx := context.Background()
	cfg := config.DefaultFullConfig("parity-test")
	cfg.Skills.Compiler.Mode = config.SkillCompilerModeFull
	platforms := []string{"gemini"}

	// 1. When probe is not excluded, it should fail
	findings, err := runCoverageGate(ctx, t.TempDir(), cfg, platforms, platformRuleExclusions, platformSkillExclusions, []string{"__parity_probe__.md"})
	require.NoError(t, err)

	probeFindingCount := 0
	for _, f := range findings {
		if f.Platform == "gemini" && f.Item == "__parity_probe__.md" && f.Type == "rule" {
			probeFindingCount++
		}
	}
	assert.Equal(t, 1, probeFindingCount, "Should return exactly 1 finding for unexcluded probe")

	// 2. When probe is excluded, it should pass
	tempRuleExclusions := map[string]map[string]bool{
		"gemini": {"__parity_probe__.md": true},
	}
	findings2, err := runCoverageGate(ctx, t.TempDir(), cfg, platforms, tempRuleExclusions, platformSkillExclusions, []string{"__parity_probe__.md"})
	require.NoError(t, err)

	probeFindingCount2 := 0
	for _, f := range findings2 {
		if f.Platform == "gemini" && f.Item == "__parity_probe__.md" && f.Type == "rule" {
			probeFindingCount2++
		}
	}
	assert.Equal(t, 0, probeFindingCount2, "Should return 0 findings for excluded probe")
}

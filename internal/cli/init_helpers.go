// Package cli provides helper functions for the init command.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
)

var legacyRootGeneratedGitignorePatterns = map[string]string{
	".claude/":     "/.claude/",
	".claude.json": "/.claude.json",
	".codex/":      "/.codex/",
	".gemini/":     "/.gemini/",
	".mcp.json":    "/.mcp.json",
	".opencode/":   "/.opencode/",
	"config.toml":  "/config.toml",
}

// generatePlatformFiles는 플랫폼별 파일을 생성한다.
func generatePlatformFiles(ctx context.Context, dir string, cfg *config.HarnessConfig, cmd *cobra.Command) error {
	effectiveCfg := applyFlagCC21Overrides(cfg, globalFlagsFromContext(cmd.Context()))

	for _, p := range cfg.Platforms {
		descriptor, ok := lookupPlatformDescriptor(p)
		if !ok {
			tui.Warnf(cmd.OutOrStdout(), "알 수 없는 플랫폼 %q, 건너뜀", p)
			continue
		}
		if _, err := descriptor.Generate(ctx, dir, effectiveCfg); err != nil {
			return fmt.Errorf("플랫폼 %q 파일 생성 실패: %w", p, err)
		}
		tui.Success(cmd.OutOrStdout(), p)
	}
	return nil
}

// detectDefaultPlatforms returns installed, ADK-supported platforms in a stable order.
// Falls back to Claude Code when no supported CLI is found in PATH.
func detectDefaultPlatforms() []string {
	detected := detectInstalledPlatforms()
	platforms := make([]string, 0, len(detected))
	platforms = append(platforms, detected...)

	if len(platforms) == 0 {
		return []string{lifecyclePlatformCatalog[0].name}
	}
	return platforms
}

// detectInstalledPlatforms returns installed, ADK-supported platforms in a stable order.
// Unlike detectDefaultPlatforms, it does not add any fallback platform.
//
// Exec surface: this is not purely passive. detect.DetectInstalledPlatforms runs
// `omp --version` to satisfy the REQ-019 identity gate, so calling it executes
// one binary from PATH. Every other platform is decided by presence alone.
func detectInstalledPlatforms() []string {
	detected := detect.DetectInstalledPlatforms()
	platforms := make([]string, 0, len(detected))
	seen := make(map[string]bool, len(detected))

	for _, p := range detected {
		if _, supported := lookupPlatformDescriptor(p.Name); !supported || seen[p.Name] {
			continue
		}
		platforms = append(platforms, p.Name)
		seen[p.Name] = true
	}
	return platforms
}

// gitignorePatterns는 autopus 관련 .gitignore 패턴 목록이다.
var gitignorePatterns = []string{
	".autopus/*-manifest.json",
	".autopus/context/signatures.md",
	".autopus/plugins/",
	".autopus/orchestra/",
	".autopus/brainstorms/",
	".autopus/txns/",
	".autopus/design/imports/",
	".autopus/design/verify/",
	".autopus/canary/",
	".autopus/metrics/",
	".autopus/backup/",
	".autopus/cache/",
	".autopus/docs/",
	".autopus/qa/runs/",
	".autopus/qa/cache/",
	".autopus/qa/gui/",
	".autopus/qa/feedback/",
	".autopus/qa/evidence/",
	".autopus/qa/releases/",
	".autopus/runtime/",
	".autopus/omp-model-resolution-v1.json",
	".autopus/state.json",
	".autopus/telemetry/",
	".autopus/audit.jsonl",
	"**/.autopus/specs/**/review.md",
	"**/.autopus/specs/**/review-findings.json",
	"**/.autopus/specs/**/.self-verify.log",
	"/.claude/",
	"/.claude.json",
	"/.codex/",
	"/.gemini/",
	"/.opencode/",
	".agents/skills/",
	".agents/plugins/",
	".agents/commands/",
	".agents/hooks.json",
	// Directory form, never a file-name glob. omp discovers rules with a
	// gitignore-aware glob, and measured on omp 17.3.5 a `.omp/rules/autopus-*.md`
	// entry drops all 14 generated rules from the session (domain rules 9 -> 0,
	// TTSR 3 -> 0) while `.omp/rules/` keeps all 14 discoverable. Omitting the
	// entry is not an option either: doctor's hygiene check fails on untracked
	// runtime/generated files left visible to git.
	//
	// A user who wants their own rule in this tree tracked runs
	// `git add -f .omp/rules/mine.md` or keeps the rule outside `.omp/rules/`.
	// A `!` negation cannot re-include a file whose parent directory is excluded.
	".omp/rules/",
	".omp/agents/",
	".omp/config.yml",
	// Extensions load through the same gitignore-aware discovery: measured on
	// omp 17.3.5, `.omp/extensions/autopus-*.ts` stops autopus-pipeline.ts from
	// registering its command, while `.omp/extensions/` leaves it registered.
	".omp/extensions/",
	".symphony/artifacts/",
	"/.mcp.json",
	"/config.toml",
}

// gitignoreUpdatePlan is the .gitignore work a project still needs: the managed
// patterns it is missing, plus whether a legacy pattern has to be rewritten.
type gitignoreUpdatePlan struct {
	Missing  []string
	Migrated bool
}

func (plan gitignoreUpdatePlan) Empty() bool {
	return len(plan.Missing) == 0 && !plan.Migrated
}

// Summary is tense-free so that both the writer and the update preview can wrap
// it in their own phrasing without restating the case analysis.
func (plan gitignoreUpdatePlan) Summary() string {
	switch {
	case len(plan.Missing) > 0 && plan.Migrated:
		return fmt.Sprintf("%d pattern(s), legacy rewrite", len(plan.Missing))
	case len(plan.Missing) > 0:
		return fmt.Sprintf("%d pattern(s)", len(plan.Missing))
	case plan.Migrated:
		return "legacy rewrite"
	}
	return ""
}

// planGitignoreUpdate reports what updateGitignore would change without writing.
// The update preview and the writer must agree, so both read this one function.
func planGitignoreUpdate(dir string) (gitignoreUpdatePlan, string) {
	var existing string
	if data, err := os.ReadFile(filepath.Join(dir, ".gitignore")); err == nil {
		existing = string(data)
	}
	existing, migrated := migrateLegacyRootGeneratedGitignorePatterns(existing)
	present := make(map[string]bool)
	for _, line := range strings.Split(existing, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		present[line] = true
	}

	var missing []string
	for _, pattern := range gitignorePatterns {
		if !present[pattern] {
			missing = append(missing, pattern)
		}
	}
	return gitignoreUpdatePlan{Missing: missing, Migrated: migrated}, existing
}

// updateGitignore는 .gitignore에 autopus 패턴을 추가한다.
func updateGitignore(dir string) (gitignoreUpdatePlan, error) {
	plan, existing := planGitignoreUpdate(dir)
	if plan.Empty() {
		return plan, nil
	}

	var sb strings.Builder
	sb.WriteString(existing)
	if len(plan.Missing) > 0 {
		if existing != "" && !strings.HasSuffix(existing, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n# Autopus-ADK generated files\n")
		for _, pattern := range plan.Missing {
			sb.WriteString(pattern)
			sb.WriteString("\n")
		}
	}

	return plan, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(sb.String()), 0644)
}

// applyGitignoreUpdate keeps the ignore surface in step with the pattern list on
// every entry point, not just init. Patterns added by a release reach an existing
// project only through update, and omp's rule and extension discovery is
// gitignore-aware: an unignored generated surface also fails doctor's hygiene
// check, so update owning this write is what makes the two entry points agree.
func applyGitignoreUpdate(out io.Writer, dir string) error {
	plan, err := updateGitignore(dir)
	if err != nil {
		return fmt.Errorf(".gitignore 업데이트 실패: %w", err)
	}
	if !plan.Empty() {
		fmt.Fprintf(out, "  + .gitignore updated (%s)\n", plan.Summary())
	}
	return nil
}

// appendGitignorePreviewReason is applyGitignoreUpdate's read-only twin: the
// preview and the write derive the same decision from planGitignoreUpdate, so a
// plan the preview omits cannot appear as a surprise write.
func appendGitignorePreviewReason(reasons []string, dir string) []string {
	plan, _ := planGitignoreUpdate(dir)
	if plan.Empty() {
		return reasons
	}
	return appendConfigPreviewReason(reasons, ".gitignore would be updated ("+plan.Summary()+")")
}

func migrateLegacyRootGeneratedGitignorePatterns(existing string) (string, bool) {
	if existing == "" {
		return existing, false
	}
	lines := strings.Split(existing, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if replacement, ok := legacyRootGeneratedGitignorePatterns[trimmed]; ok {
			lines[i] = replacement
			changed = true
		}
	}
	if !changed {
		return existing, false
	}
	return strings.Join(lines, "\n"), true
}

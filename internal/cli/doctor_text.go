package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/detect"
)

// @AX:WARN [AUTO] @AX:SPEC: SPEC-CONTEXT-ENGINEERING-001: this doctor orchestrator contains more than eight conditional branches.
// @AX:REASON [AUTO]: platform validation, dependency repair, health gates, and advisory checks converge on the final verdict.
func runDoctorText(cmd *cobra.Command, opts doctorOptions) error {
	// The tally counts the ERROR lines this report prints, so the result box
	// says FAIL only when the reader can see what failed.
	tally := tui.NewTally(cmd.OutOrStdout())
	var out io.Writer = tally

	// The banner follows the config load so it can name the project actually
	// being diagnosed. It used to print the literal "autopus-adk" regardless of
	// --dir or cwd.
	cfg, err := loadHarnessConfigForDir(opts.dir, globalFlags{})
	if err != nil {
		tui.BannerWithInfo(out, harnessProjectLabel(opts.dir, nil), "doctor")
		tui.FAIL(out, fmt.Sprintf("autopus.yaml 로드 실패: %v", err))
		renderHygieneText(out, collectStatusHygiene(opts.dir))
		return nil
	}
	tui.BannerWithInfo(out, harnessProjectLabel(opts.dir, cfg), "doctor")
	tui.OK(out, fmt.Sprintf("autopus.yaml (mode: %s)", cfg.Mode))

	ctx := doctorCommandContext(cmd)
	allOK := true
	var unhealthy []string
	markUnhealthy := func(section string) {
		allOK = false
		for _, seen := range unhealthy {
			if seen == section {
				return
			}
		}
		unhealthy = append(unhealthy, section)
	}
	// `--fix` only installs missing dependencies. A missing or invalid managed
	// surface is installed by `auto update`, so the two failure classes are
	// tracked apart: a fresh clone of a repo that gitignores its harness fails
	// every platform check, and pointing that operator at `--fix` sends them to
	// a command that repairs nothing and reprints the same advice.
	platformFailed := false
	depsMissing := false
	for _, p := range cfg.Platforms {
		validationErrs, validateErr := validateDoctorPlatform(ctx, opts.dir, p)

		if validateErr != nil {
			tui.FAIL(out, fmt.Sprintf("%s 검증 실패: %v", p, validateErr))
			markUnhealthy(p)
			platformFailed = true
			continue
		}
		if p == "omp" {
			// Degraded OMP readiness is a warning: `auto update` cannot repair
			// model routing, so only a failed check names the managed surface.
			healthy, failed := renderOMPDoctorReadinessText(ctx, out, opts.dir, validationErrs)
			if !healthy {
				markUnhealthy(p)
			}
			if failed {
				platformFailed = true
			}
			continue
		}

		if len(validationErrs) == 0 {
			tui.OK(out, p)
		} else {
			for _, ve := range validationErrs {
				detail := doctorPlatformValidationDetail(p, ve.Message, doctorValidationFile(ve.File))
				switch strings.ToUpper(strings.TrimSpace(ve.Level)) {
				case "ERROR":
					tui.FAIL(out, detail)
					markUnhealthy(p)
					platformFailed = true
				case "WARN":
					tui.SKIP(out, detail)
				default:
					tui.Info(out, detail)
				}
			}
		}
	}

	tui.SectionHeader(out, "Dependencies")
	statuses := detect.CheckDependencies(detect.FullModeDeps)
	for _, s := range statuses {
		if s.Installed {
			tui.OK(out, s.Name)
		} else if s.Required {
			tui.FAIL(out, fmt.Sprintf("%s not installed (install: %s)", s.Name, s.InstallCmd))
			markUnhealthy("Dependencies")
			depsMissing = true
		} else {
			tui.SKIP(out, fmt.Sprintf("%s not installed (optional, install: %s)", s.Name, s.InstallCmd))
		}
	}

	if opts.fix {
		missingDeps := filterMissing(statuses)
		if opts.requiredOnly {
			missingDeps = filterRequired(missingDeps)
		}
		if len(missingDeps) > 0 {
			if err := runDoctorFix(out, missingDeps, opts.yes); err != nil {
				tui.FAIL(out, fmt.Sprintf("Auto-install failed: %v", err))
				markUnhealthy("Dependencies")
			}
		}
	}

	if !checkRuntimeProcessesText(out, opts) {
		markUnhealthy("Runtime Processes")
	}

	// Parent-rule conflicts are a Claude Code concern: only that runtime walks
	// ancestors for `.claude/rules/`, and `isolate_rules` — the remedy this
	// block advertises — is consumed by the claude adapter alone.
	conflicts := detect.CheckParentRuleConflicts(opts.dir)
	if configuresClaudeCode(cfg) && len(conflicts) > 0 {
		tui.SectionHeader(out, "Rule Conflicts")
		if cfg.IsolateRules {
			tui.OK(out, "isolate_rules: true (parent rules ignored)")
		}
		for _, c := range conflicts {
			if cfg.IsolateRules {
				tui.Info(out, fmt.Sprintf("%s/.claude/rules/%s/ (ignored)", c.ParentDir, c.Namespace))
			} else {
				tui.SKIP(out, fmt.Sprintf("Parent rules: %s/.claude/rules/%s/", c.ParentDir, c.Namespace))
				tui.Bullet(out, "Run 'auto init' or 'auto update' to configure rule isolation.")
				markUnhealthy("Rule Conflicts")
			}
		}
	}

	tui.SectionHeader(out, "Installed CLIs")
	detected := detect.DetectPlatforms()
	if len(detected) == 0 {
		tui.SKIP(out, "No coding CLIs detected in PATH")
	} else {
		for _, p := range detected {
			tui.OK(out, fmt.Sprintf("%s (%s)", p.Name, p.Version))
		}
	}

	// The Desktop launcher check is advisory: a managed installation is valid,
	// so it never touches allOK.
	checkDesktopShimText(out, diagnoseDesktopShim())
	if !checkHomebrewTrustText(out, diagnoseHomebrewTrust(ctx)) {
		markUnhealthy("Homebrew Tap Trust")
	}

	tui.SectionHeader(out, "Quality Gate")
	if !checkQualityGate(out, cfg) {
		markUnhealthy("Quality Gate")
	}
	if !checkCodexModelOwnershipText(out, opts.dir, cfg) {
		markUnhealthy("Codex Model Ownership")
	}
	// Codex agent concurrency is advisory: a drifted worker ceiling is worth
	// reporting but never marks the harness unhealthy.
	checkCodexAgentConcurrencyText(out, opts.dir, cfg)

	if !checkProviderTransportSmokeText(out, cfg, opts) {
		markUnhealthy("Provider Transport")
	}
	if !checkProviderReadinessText(ctx, out, cfg) {
		markUnhealthy("Provider Readiness")
	}

	if configuresClaudeCode(cfg) {
		tui.SectionHeader(out, "Hooks & Permissions")
		if !checkHooksPermissions(out, opts.dir) {
			markUnhealthy("Hooks & Permissions")
		}
	}
	// The edit-guard registration report is advisory and never touches allOK.
	checkEditGuardText(out, opts.dir, cfg)
	checkReaText(ctx, out, opts.dir, cfg)

	// Legacy orchestra keys and stale completion hooks are what `auto update`
	// removes, so the result box names it unless every remaining member is one
	// that update keeps.
	retiredHealthy, retiredUpdateFixes := checkRetiredOrchestraText(out, opts.dir, cfg)
	if !retiredHealthy {
		markUnhealthy("Retired Orchestra Surface")
	}

	// Context weight is advisory: it warns on an over-weight context catalog
	// but never fails harness health, so its result does not touch allOK.
	checkContextWeight(out, opts.dir)
	// The disk row is advisory too: free space below the floor is a reason to
	// run `auto disk status`, not a broken harness.
	checkDiskText(out, opts.dir, cfg)

	hygiene := collectStatusHygiene(opts.dir)
	renderHygieneText(out, hygiene)
	if hygiene.hasWarning() {
		markUnhealthy("Hygiene")
	}

	// Drift observation is advisory: it mirrors the JSON drift checks but never
	// touches allOK, so a project with a pending update is not reported as failed.
	renderDriftTextContext(ctx, out, opts.dir, cfg)

	renderEvidenceFreshnessText(out, opts.dir, cfg)

	fmt.Fprintln(out)
	verdict := decideDoctorVerdict(doctorVerdictInput{
		allOK: allOK, errorLines: tally.Errors(), unhealthy: unhealthy,
		updateFixes: platformFailed || retiredUpdateFixes, depsMissing: depsMissing,
		retiredUpdateFixes: retiredUpdateFixes,
	})
	switch verdict.kind {
	case doctorVerdictPass:
		tui.ResultBox(out, true, verdict.message)
	case doctorVerdictFail:
		tui.ResultBox(out, false, verdict.message)
	default:
		tui.WarnResultBox(out, verdict.message)
	}

	return nil
}

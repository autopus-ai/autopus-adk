package tui

import (
	"fmt"
	"os"
	"runtime"

	"github.com/charmbracelet/huh"
)

// Language code and label data for the language selection step.
var (
	langCodes  = []string{"en", "ko", "ja", "zh"}
	langLabels = []string{"English", "Korean (한국어)", "Japanese (日本語)", "Chinese (中文)"}
)

// InitWizardOpts holds options for the init wizard.
type InitWizardOpts struct {
	Quality         string   // pre-set from --quality flag
	NoReviewGate    bool     // pre-set from --no-review-gate flag
	Platforms       []string // pre-set from --platforms flag
	Accessible      bool     // true for non-TTY / --yes mode
	Providers       []string // detected orchestra providers
	ExistingProfile string   // pre-set profile; non-empty skips the profile step
}

// InitWizardResult holds all wizard selections.
type InitWizardResult struct {
	CommentsLang string
	CommitsLang  string
	AILang       string
	Quality      string // ultra, balanced, or QualityRoleGroups
	ReviewGate   bool
	Methodology  string
	UsageProfile string
	Cancelled    bool
}

// RunInitWizard runs the interactive init wizard using huh forms.
// When opts.Accessible is true, it returns defaults without launching a TUI.
func RunInitWizard(opts InitWizardOpts) (*InitWizardResult, error) {
	EnsureSafeEnv()
	InitStyles()

	result := defaultResult(opts)

	// Non-TTY / --yes mode: return defaults directly (R3, C2)
	if opts.Accessible {
		return result, nil
	}

	if !FormsSupported(runtime.GOOS, os.Getenv) {
		fmt.Fprintln(os.Stderr, "  TUI wizard not supported in this terminal. Using defaults.")
		fmt.Fprintln(os.Stderr, "  Tip: use Windows Terminal or run with --yes flag.")
		return result, nil
	}

	steps := buildStepList(opts)
	step := 0
	for step < len(steps) {
		form := steps[step](result)
		if err := form.Run(); err != nil {
			// User cancelled (ctrl+c or esc)
			return &InitWizardResult{Cancelled: true}, nil
		}
		step++
	}

	return result, nil
}

// FormsSupported reports whether huh forms can run in this terminal.
// huh/bubbletea may hang on cmd.exe or another legacy Windows console, so
// on Windows forms need Windows Terminal (WT_SESSION) or a terminal that
// sets TERM_PROGRAM. getenv is os.Getenv outside tests.
func FormsSupported(goos string, getenv func(string) string) bool {
	return goos != "windows" || getenv("WT_SESSION") != "" || getenv("TERM_PROGRAM") != ""
}

// stepBuilder is a function that creates a huh form for a given step.
type stepBuilder func(r *InitWizardResult) *huh.Form

// @AX:NOTE [AUTO]: buildStepList uses closure-rebinding to inject the final
// step count into each step's title. Steps are collected first, then totalRef
// is set so all closures share the correct total when invoked.
// buildStepList assembles the wizard steps, skipping pre-configured ones.
func buildStepList(opts InitWizardOpts) []stepBuilder {
	// Collect step factories, then rebind with correct indices.
	type entry struct {
		name string
		fn   func(idx, total int, r *InitWizardResult) *huh.Form
	}
	var entries []entry

	// R2: skip profile step when ExistingProfile is already set.
	if opts.ExistingProfile == "" {
		entries = append(entries, entry{"profile", func(idx, total int, r *InitWizardResult) *huh.Form {
			return buildProfileStep(idx, total, r)
		}})
	}

	entries = append(entries, entry{"lang", func(idx, total int, r *InitWizardResult) *huh.Form {
		return buildLangStep(idx, total, r)
	}})

	if opts.Quality == "" {
		entries = append(entries, entry{"quality", func(idx, total int, r *InitWizardResult) *huh.Form {
			return buildQualityStep(idx, total, r)
		}})
	}

	if !opts.NoReviewGate {
		entries = append(entries, entry{"reviewgate", func(idx, total int, r *InitWizardResult) *huh.Form {
			return buildReviewGateStep(idx, total, r, opts)
		}})
	}

	entries = append(entries, entry{"methodology", func(idx, total int, r *InitWizardResult) *huh.Form {
		return buildMethodologyStep(idx, total, r)
	}})

	total := len(entries)
	rebuilt := make([]stepBuilder, total)
	for i, e := range entries {
		idx := i + 1
		fn := e.fn
		rebuilt[i] = func(r *InitWizardResult) *huh.Form {
			return fn(idx, total, r)
		}
	}
	return rebuilt
}

// buildLangStep creates the language selection step (3 sub-selects).
func buildLangStep(num, total int, r *InitWizardResult) *huh.Form {
	title := fmt.Sprintf("[%d/%d] Language Settings", num, total)
	opts := langOptions()

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Description("Code comments language").
				Options(opts...).
				Value(&r.CommentsLang),
			huh.NewSelect[string]().
				Title("Commit messages").
				Options(opts...).
				Value(&r.CommitsLang),
			huh.NewSelect[string]().
				Title("AI responses").
				Options(opts...).
				Value(&r.AILang),
		),
	).WithTheme(AutopusTheme()).WithWidth(bannerWidth + 10)
}

// QualityRoleGroups is the quality step's third choice (SPEC-MODELUX-001
// REQ-MUX-15). It is not a preset name: init hands it to the role-group
// composition step, which starts from the balanced default and saves a
// preset the Binding Contract chooses.
const QualityRoleGroups = "role-groups"

// buildQualityStep creates the quality mode selection step.
func buildQualityStep(num, total int, r *InitWizardResult) *huh.Form {
	title := fmt.Sprintf("[%d/%d] Quality Mode", num, total)

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(qualityOptions(DetectSystemLang())...).
				Value(&r.Quality),
		),
	).WithTheme(AutopusTheme()).WithWidth(bannerWidth + 10)
}

// qualityOptions lists the quality step's choices. The role-group label is
// Korean for ko and English for every other locale, as the composition step
// that follows it is.
func qualityOptions(lang string) []huh.Option[string] {
	roleGroups := "Compose by role — choose a tier for each role group"
	if lang == "ko" {
		roleGroups = "역할별 직접 구성 — 역할 그룹마다 등급을 고릅니다"
	}
	return []huh.Option[string]{
		huh.NewOption("Ultra — strict review, high coverage", "ultra"),
		huh.NewOption("Balanced — pragmatic defaults", "balanced"),
		huh.NewOption(roleGroups, QualityRoleGroups),
	}
}

// buildReviewGateStep creates the review gate confirmation step.
func buildReviewGateStep(num, total int, r *InitWizardResult, opts InitWizardOpts) *huh.Form {
	title := fmt.Sprintf("[%d/%d] Review Gate", num, total)
	desc := "Require human review before merging"
	if len(opts.Providers) > 0 {
		desc = fmt.Sprintf("Providers detected: %d — enable review gate?", len(opts.Providers))
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(title).
				Description(desc).
				Affirmative("Yes").
				Negative("No").
				Value(&r.ReviewGate),
		),
	).WithTheme(AutopusTheme()).WithWidth(bannerWidth + 10)
}

// buildMethodologyStep creates the methodology selection step.
func buildMethodologyStep(num, total int, r *InitWizardResult) *huh.Form {
	title := fmt.Sprintf("[%d/%d] Methodology", num, total)

	return huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(title).
				Options(
					huh.NewOption("TDD — test-driven development", "tdd"),
					huh.NewOption("None — no enforced methodology", "none"),
				).
				Value(&r.Methodology),
		),
	).WithTheme(AutopusTheme()).WithWidth(bannerWidth + 10)
}

// langOptions builds huh options from the language data.
func langOptions() []huh.Option[string] {
	opts := make([]huh.Option[string], len(langCodes))
	for i, code := range langCodes {
		opts[i] = huh.NewOption(langLabels[i], code)
	}
	return opts
}

// defaultResult returns a result populated with sensible defaults,
// respecting any pre-configured values from opts.
func defaultResult(opts InitWizardOpts) *InitWizardResult {
	// Detect system locale for commit messages and AI responses.
	// Code comments always default to English for international readability.
	sysLang := DetectSystemLang()

	r := &InitWizardResult{
		CommentsLang: "en",
		CommitsLang:  sysLang,
		AILang:       sysLang,
		Quality:      "balanced",
		ReviewGate:   len(opts.Providers) >= 2,
		Methodology:  "tdd",
		UsageProfile: "developer",
	}

	// Apply pre-configured values (R9, R10, R13)
	if opts.Quality != "" {
		r.Quality = opts.Quality
	}
	if opts.NoReviewGate {
		r.ReviewGate = false
	}
	if opts.ExistingProfile != "" {
		r.UsageProfile = opts.ExistingProfile
	}

	return r
}

package tui

import (
	"fmt"
	"slices"

	"github.com/charmbracelet/huh"

	"github.com/insajin/autopus-adk/pkg/reamcp"
)

// reaStepText is the REA step's Form Text in one locale.
type reaStepText struct {
	Title       string
	Description string
}

// reaStepTextFor returns the REA step text: Korean for ko and English for
// every other locale, as the composition text is.
func reaStepTextFor(lang string) reaStepText {
	if lang == "ko" {
		return reaStepText{
			Title:       "REA 역공학 MCP",
			Description: "Claude Code·Codex·Antigravity에 rea-agents@" + reamcp.Pin + "을 프로젝트 범위로 등록합니다 (Node " + reamcp.NodeRange + ")",
		}
	}
	return reaStepText{
		Title:       "REA reverse-engineering MCP",
		Description: "Registers rea-agents@" + reamcp.Pin + " for Claude Code, Codex, and Antigravity in this project (Node " + reamcp.NodeRange + ")",
	}
}

// reaStepWanted reports whether init asks the REA question: neither a flag
// nor an existing decision preset it, and a REA target platform is selected.
// OMP and OpenCode get guidance only, so selecting just them never asks.
func reaStepWanted(opts InitWizardOpts) bool {
	return opts.Rea == nil && slices.ContainsFunc(reamcp.Platforms(), func(target reamcp.Platform) bool {
		return slices.Contains(opts.Platforms, string(target))
	})
}

// reaDefault is the REA answer before the step runs: the preset decision,
// else No. No shipped agent consumes the REA MCP since v0.51.0, so it is
// opt-in. It never aliases opts.Rea.
func reaDefault(opts InitWizardOpts) *bool {
	answer := false
	if opts.Rea != nil {
		answer = *opts.Rea
	}
	return &answer
}

// buildReaStep creates the REA confirmation step in the system locale.
func buildReaStep(num, total int, r *InitWizardResult) *huh.Form {
	return buildReaStepFor(num, total, r, DetectSystemLang())
}

// buildReaStepFor creates the REA confirmation step with the text for lang.
func buildReaStepFor(num, total int, r *InitWizardResult, lang string) *huh.Form {
	text := reaStepTextFor(lang)
	if r.Rea == nil {
		r.Rea = reaDefault(InitWizardOpts{})
	}

	return huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("[%d/%d] %s", num, total, text.Title)).
				Description(text.Description).
				Affirmative("Yes").
				Negative("No").
				Value(r.Rea),
		),
	).WithTheme(AutopusTheme()).WithWidth(bannerWidth + 10)
}

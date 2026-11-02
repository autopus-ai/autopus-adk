package cli

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// composeReviewCheckTimeout is the budget of each live check request; the
// doctor smoke keeps its own 30-second default.
const composeReviewCheckTimeout = 60 * time.Second

// composeReviewCheckDetailRunes bounds the failure detail of a result line.
const composeReviewCheckDetailRunes = 200

// reviewLiveCheck asks the live check select, whose default makes no call
// (REQ-RVM-10). Only on 확인 호출 does it run runProviderTransportSmoke, the
// doctor smoke unchanged, on the candidate: a copy of the run's config with
// the working value, never the file. It sends one request per provider in
// turn, and an OMP provider retries a transient error within the request,
// so the label names the most model calls. A provider name with terminal
// control characters stops the check before any request.
func (f *composeFormRun) reviewLiveCheck(tab *reviewTabState) error {
	text := f.reviewText()
	candidate := reviewCandidate(f.shared.cfg, tab.baseline, tab.working)
	names := resolveSpecReviewProviderNames(candidate, false)
	run, err := composeSelect(f.cmd, text.Check, "", []composeOption[bool]{
		{text.CheckSkip, false},
		{fmt.Sprintf(text.CheckRun, len(names), len(names)*ompReviewMaxAttempts), true},
	}, false)
	if err != nil || !run {
		return err
	}
	for _, name := range names {
		if terminalSafe(name) != name {
			f.reviewLine(fmt.Sprintf(text.CheckRefused, name))
			return nil
		}
	}
	failed := false
	shown := reviewCheckModels(tab.baseline, tab.working)
	missing := func(provider orchestra.ProviderConfig, _ string) {
		f.reviewLine(reviewCheckMissingLine(tab.baseline, provider, text))
	}
	for _, result := range runProviderTransportSmokeWith(commandContext(f.cmd), candidate, composeReviewCheckTimeout, missing) {
		f.reviewLine(reviewCheckLine(candidate, shown, result, text))
		failed = failed || result.Status != "pass"
	}
	if failed {
		f.reviewLine(text.CheckFailed)
	}
	return nil
}

// reviewCandidate is the live check's config: the run's config after
// config.MigrateOrchestraConfig, as the runtime runs it, carrying the
// working value the way the line writer writes it. Nothing writes it.
func reviewCandidate(cfg *config.HarnessConfig, b reviewBaseline, working reviewComposition) *config.HarnessConfig {
	candidate := reviewMigratedConfig(cfg)
	if candidate == cfg {
		clone := *cfg
		candidate = &clone
	}
	candidate.Orchestra.Providers = maps.Clone(candidate.Orchestra.Providers)
	candidate.Spec.ReviewGate.Providers = slices.Clone(working.Providers)
	candidate.Spec.ReviewGate.Judge = working.Judge
	candidate.Spec.ReviewGate.JudgeModel, candidate.Spec.ReviewGate.JudgeEffort = working.JudgeModel.Model, working.JudgeModel.Effort
	// Validation has passed every value, and a value the planner would
	// still refuse leaves its entry as it was: the check never runs it.
	_ = applyReviewEntries(candidate.Orchestra.Providers, b, working)
	return candidate
}

// reviewCheckMissingLine is the live check's line for a provider whose binary
// is not installed: the provider and its binary as written, or the provider
// alone when the binary is written as a placeholder or cannot be read as
// written; an OMP provider needs omp, which is no one's value. The runtime
// warning, which spells the expanded binary, never runs from the tab.
func reviewCheckMissingLine(b reviewBaseline, provider orchestra.ProviderConfig, text composeReviewText) string {
	shown := config.ProviderBackendOMP
	if provider.Backend != config.ProviderBackendOMP {
		shown = b.binaryText(provider.Name)
	}
	if shown == "" || strings.Contains(shown, "${") {
		return fmt.Sprintf(text.CheckMissingName, provider.Name)
	}
	return fmt.Sprintf(text.CheckMissing, provider.Name, shown)
}

// reviewCheckModels is the model a result line names for each OMP provider:
// a model the tab chose, as chosen, and an unchanged one as the raw document
// writes it. The check runs the candidate's expanded model, but an
// environment value never reaches the line (REQ-RVM-26): a provider with no
// raw model, or one written as a placeholder, names none.
func reviewCheckModels(b reviewBaseline, working reviewComposition) map[string]string {
	shown := map[string]string{}
	for _, name := range b.Visible {
		switch raw := b.Raw[name].Model; {
		case !working.OMP[name]:
		case !b.OMP[name] || working.Models[name] != b.Models[name]:
			shown[name] = working.Models[name]
		case raw != "" && !strings.Contains(raw, "${"):
			shown[name] = raw
		}
	}
	return shown
}

// reviewCheckLine is one result line (Live Check Contract): the provider,
// its shown model or CLI backend, and the verdict. The review_gate result of
// a failed assembly and an OMP provider without a shown model name no model.
func reviewCheckLine(candidate *config.HarnessConfig, shown map[string]string, result providerSmokeResult, text composeReviewText) string {
	subject := result.Provider
	if entry, ok := candidate.Orchestra.Providers[result.Provider]; ok || result.Provider != "review_gate" {
		model := text.CLIBackend
		if entry.Backend == config.ProviderBackendOMP {
			model = shown[result.Provider]
		}
		if model != "" {
			subject += " (" + model + ")"
		}
	}
	verdict := text.CheckPass
	switch result.Status {
	case "pass":
	case "warn":
		verdict = text.CheckWarn
	default:
		verdict = fmt.Sprintf(text.CheckFail, reviewCheckDetail(result.Detail))
	}
	return fmt.Sprintf(text.CheckLine, subject, verdict)
}

// reviewCheckDetail is the first line of a failure detail, terminal-safe and
// cut to composeReviewCheckDetailRunes runes.
func reviewCheckDetail(detail string) string {
	line, _, _ := strings.Cut(detail, "\n")
	runes := []rune(terminalSafe(line))
	if len(runes) > composeReviewCheckDetailRunes {
		runes = runes[:composeReviewCheckDetailRunes]
	}
	return string(runes)
}

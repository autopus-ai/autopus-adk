package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// The OMP tab's seams (SPEC-MODELUX-002 REQ-MUX2-11). Tests replace a seam
// and restore it with t.Cleanup.
var (
	// composeReloadConfig re-reads autopus.yaml after an OMP write, so the
	// loaded config takes the role_model_policy the file now holds.
	composeReloadConfig = config.LoadPreview
	// updateComposeOMPDeps is the OMP catalog runner and activation the OMP
	// tab of `auto update` uses.
	updateComposeOMPDeps = defaultOMPPlatformDependencies()
)

// ompCustomMode is the profile choice that pins models per native agent on
// top of ompCustomBaseProfile.
const ompCustomMode = "custom"

// composeOMPTab is the OMP tab of one run (OMP Tab Contract).
type composeOMPTab struct {
	deps ompPlatformDependencies
	// memory plans init's config, which no file holds yet (REQ-MUX2-22):
	// a confirmed choice goes into that config, and init's save and OMP
	// generation write and activate it.
	memory bool
}

// composeReloadError is an OMP write the run could not read back
// (REQ-MUX2-11). The file is written and activated, and nothing undoes it,
// but the run must end instead of saving or generating from the config it
// holds, which still has the previous role_model_policy.
type composeReloadError struct{ err error }

func (e composeReloadError) Error() string {
	return "OMP settings were applied but could not be re-read: " + e.err.Error()
}

func (e composeReloadError) Unwrap() error { return e.err }

// ompOffered reports whether the tool tab lists OMP: the run has an OMP tab
// and the loaded config already installs omp.
func (f *composeFormRun) ompOffered() bool {
	return f.options.OMP != nil && containsPlatform(f.shared.cfg.Platforms, "omp")
}

// noteDetectedOMP prints, once per run, why an omp this update only detected
// has no tab yet: the OMP writer needs omp in the file.
func (f *composeFormRun) noteDetectedOMP() {
	if f.options.OMP == nil || f.ompNoticed || f.ompOffered() || !containsPlatform(f.shared.detected, "omp") {
		return
	}
	f.ompNoticed = true
	fmt.Fprintln(f.out, f.form.OMPDetectedOnly)
}

// omp runs the OMP tab (REQ-MUX2-10) by the OMP editor's rules: the profile,
// the family unless an explicit profile of that name keeps its own models,
// and for per-agent pins one installed model and thinking level per native
// agent. The plan's routes are shown before the confirmation; a blocker or
// a refused source ends the tab with nothing written or activated.
func (f *composeFormRun) omp() error {
	note := fmt.Sprintf(f.form.OMPCurrent, f.ompPolicySummary())
	mode, err := composeSelect(f.cmd, f.form.OMP, note, []composeOption[string]{
		{f.form.OMPBack, ""}, {"balanced", "balanced"}, {"ultra", "ultra"}, {f.form.OMPPin, ompCustomMode},
	}, "")
	if err != nil || mode == "" {
		return err
	}
	runner := f.options.OMP.deps.newRunner()
	name := mode
	if mode == ompCustomMode {
		name = ompCustomBaseProfile
	}
	family, err := f.ompFamily(name)
	if err != nil {
		return err
	}
	if mode == ompCustomMode && f.shared.cfg.RoleModelPolicy.OMPAgentSource() == config.OMPAgentSourceAutopus {
		return f.ompAutopus(name, family, runner)
	}
	var pins []string
	if mode == ompCustomMode {
		if pins, err = f.ompPins(runner); err != nil {
			return f.ompFailed(err)
		}
		if len(pins) == 0 {
			fmt.Fprintln(f.out, "No agent was pinned; nothing changed.")
			return nil
		}
	}
	opts, err := newOMPProfileApplyOptions(name, family, pins, false, false)
	if err != nil {
		return f.ompFailed(err)
	}
	preview, candidate, err := f.ompPlan(opts, runner)
	if err != nil {
		return f.ompFailed(err)
	}
	renderOMPQualitySummary(f.out, preview)
	if len(preview.Blockers) > 0 {
		return f.ompFailed(ompProfileUnavailableError{blockers: preview.Blockers})
	}
	return f.ompConfirm(opts, runner, candidate)
}

// ompPlan plans opts through planOMPProfile for update and quality, and on
// a detached copy of init's config for init, whose candidate
// role_model_policy it also returns.
func (f *composeFormRun) ompPlan(opts ompProfileApplyOptions, runner omp.OMPModelCatalogRunner) (ompProfileApplyPreviewPayload, config.RoleModelPolicyConf, error) {
	if f.options.OMP.memory {
		return planInitOMP(commandContext(f.cmd), f.shared.cfg, opts, runner)
	}
	preview, err := planOMPProfile(commandContext(f.cmd), f.shared.dir, opts, runner)
	return preview, config.RoleModelPolicyConf{}, err
}

// ompFailed ends the tab on a refusal, a failed plan, or an unreadable
// catalog with one warning line naming the cause; init first says that OMP
// models can be set after init instead (REQ-MUX2-23). Ctrl+C still ends the
// step.
func (f *composeFormRun) ompFailed(err error) error {
	if errors.Is(err, errComposeInterrupted) {
		return err
	}
	if f.options.OMP.memory && composeOMPCatalogFailure(err) {
		fmt.Fprintln(f.out, f.form.InitOMPUnavailable)
	}
	f.warn(err)
	return nil
}

// ompFamily asks the family anchoring a built-in derivation, the stored one
// first selected. An explicit profile of that name keeps its own models and
// refuses a family, so it is not asked.
func (f *composeFormRun) ompFamily(name string) (string, error) {
	policy := f.shared.cfg.RoleModelPolicy
	if _, explicit := policy.Profiles[name]; explicit {
		fmt.Fprintln(f.out, "Keeping the models in your custom profile.")
		return "", nil
	}
	stored := "openai"
	if policy.Family == "anthropic" {
		stored = "anthropic"
	}
	return composeSelect(f.cmd, f.form.OMPFamily, "",
		[]composeOption[string]{{"GPT", "openai"}, {"Claude", "anthropic"}}, stored)
}

// ompPins lists the native agents behind 완료 until the operator is done
// and returns their pins in --agent form and registry order, so the choice
// travels the attestation, preview, and rollback path the flag does.
func (f *composeFormRun) ompPins(runner omp.OMPModelCatalogRunner) ([]string, error) {
	choices, _, err := installedOMPCustomModelChoices(commandContext(f.cmd), f.cmd, runner)
	if err == nil && len(choices) == 0 {
		err = ompCatalogUnavailableError{reason: "no_usable_model"}
	}
	if err != nil {
		return nil, composeOMPCatalogError{err: err}
	}
	pins := map[string]string{}
	for {
		options := []composeOption[string]{{f.form.ListDone, ""}}
		for _, agent := range config.OMPNativeAgentNames() {
			label, pinned := pins[agent]
			if !pinned {
				label = f.form.OMPProfileDefault
			}
			options = append(options, composeOption[string]{agent + " — " + label, agent})
		}
		agent, err := composeSelect(f.cmd, f.form.List, "", options, "")
		if err != nil {
			return nil, err
		}
		if agent == "" {
			break
		}
		pin, err := f.ompPin(agent, choices)
		if err != nil {
			return nil, err
		}
		delete(pins, agent)
		if pin != "" {
			pins[agent] = pin
		}
	}
	var ordered []string
	for _, agent := range config.OMPNativeAgentNames() {
		if pin, ok := pins[agent]; ok {
			ordered = append(ordered, agent+"="+pin)
		}
	}
	return ordered, nil
}

// ompPin asks agent's model among the installed models that can serve its
// route, the catalog's whole list when it carries no capability metadata,
// then the thinking level when the model reports more than one. The
// profile default keeps the agent unpinned.
func (f *composeFormRun) ompPin(agent string, choices []ompCustomModelChoice) (string, error) {
	capability, err := ompCustomAgentCapability(agent)
	if err != nil {
		return "", err
	}
	options := []composeOption[string]{{f.form.OMPProfileDefault, ""}}
	for _, choice := range choices {
		if requireOMPCustomCapability(choice, true, capability, choices) == nil {
			options = append(options, composeOption[string]{choice.selector(), choice.selector()})
		}
	}
	selector, err := composeSelect(f.cmd, agent+" ("+capability+")", "", options, "")
	if err != nil || selector == "" {
		return "", err
	}
	choice := choices[slices.IndexFunc(choices, func(c ompCustomModelChoice) bool { return c.selector() == selector })]
	thinking := choice.Thinking[0]
	if len(choice.Thinking) > 1 {
		levels := make([]composeOption[string], 0, len(choice.Thinking))
		for _, level := range choice.Thinking {
			levels = append(levels, composeOption[string]{level, level})
		}
		if thinking, err = composeSelect(f.cmd, fmt.Sprintf(f.form.OMPThinking, agent), "", levels, thinking); err != nil {
			return "", err
		}
	}
	return selector + ":" + thinking, nil
}

// ompConfirm asks the confirmation, whose default cancels, after the notice
// that the OMP writer re-encodes the whole file; init has no file yet.
func (f *composeFormRun) ompConfirm(opts ompProfileApplyOptions, runner omp.OMPModelCatalogRunner, candidate config.RoleModelPolicyConf) error {
	if !f.options.OMP.memory {
		fmt.Fprintln(f.out, f.form.OMPRewrite)
	}
	apply, err := composeSelect(f.cmd, f.form.Confirm, "",
		[]composeOption[bool]{{f.form.ConfirmCancel, false}, {f.form.ConfirmApply, true}}, false)
	if err != nil {
		return err
	}
	switch {
	case !apply:
		fmt.Fprintln(f.out, f.Text.Cancelled)
		return nil
	case f.options.OMP.memory:
		return f.ompApplyInit(candidate)
	}
	return f.ompApply(opts, runner)
}

// ompApply writes and activates the confirmed choice through
// applyOMPProfile, which revalidates the catalog and the source and restores
// the file when activation fails, then gives the loaded config the
// role_model_policy the file now holds: no later save or generation of the
// run writes the previous one back.
func (f *composeFormRun) ompApply(opts ompProfileApplyOptions, runner omp.OMPModelCatalogRunner) error {
	if _, err := applyOMPProfile(commandContext(f.cmd), f.shared.dir, opts, runner, f.options.OMP.deps.activate); err != nil {
		return f.ompFailed(err)
	}
	fmt.Fprintln(f.out, "OMP models applied. Start a new OMP session to use them.")
	reloaded, err := composeReloadConfig(f.shared.dir)
	if err != nil {
		fmt.Fprintf(f.out, f.form.OMPReloadFailed+"\n", terminalSafe(err.Error()))
		return composeReloadError{err: err}
	}
	f.shared.cfg.RoleModelPolicy = reloaded.RoleModelPolicy
	f.applied = true
	return nil
}

// ompPolicySummary is the current role_model_policy in one line for the
// profile select's note: profile, family, and every pin, or the platform
// default without a profile.
func (f *composeFormRun) ompPolicySummary() string {
	policy := f.shared.cfg.RoleModelPolicy
	if policy.Profile == "" {
		return composeTableLabelsFor(f.shared.lang).platformDefault
	}
	parts := []string{policy.Profile}
	if policy.Family != "" {
		parts = append(parts, policy.Family)
	}
	for _, agent := range sortedOMPProfileOverriddenAgents(ompProfileOverriddenAgents(&policy)) {
		candidate := policy.Agents[agent].Candidates[0]
		parts = append(parts, agent+"="+candidate.Selector+":"+candidate.Thinking)
	}
	return strings.Join(parts, " · ")
}

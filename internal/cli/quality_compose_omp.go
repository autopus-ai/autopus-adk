package cli

import (
	"errors"
	"fmt"

	"github.com/insajin/autopus-adk/internal/cli/tui"
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

// omp runs the OMP tab: the OMP Autopus Tab Contract of SPEC-MODELUX-003,
// which asks no profile or family. The MODELUX-002 profile tab served the
// bundled agents of agent_source: native, which v0.50.129 removed.
func (f *composeFormRun) omp() error {
	return f.ompAutopus()
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
// step, and Esc returns to the tool tab (SPEC-TUIUX-001 REQ-TUX-14).
func (f *composeFormRun) ompFailed(err error) error {
	if errors.Is(err, errComposeInterrupted) || errors.Is(err, tui.ErrNavBack) {
		return err
	}
	if f.options.OMP.memory && composeOMPCatalogFailure(err) {
		fmt.Fprintln(f.out, f.form.InitOMPUnavailable)
	}
	f.warn(err)
	return nil
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

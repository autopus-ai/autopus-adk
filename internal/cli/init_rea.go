package cli

import (
	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// initRea is one init run's REA decision (REQ-REA-02): the flag, else the
// existing configuration's decision, else the wizard answer on an
// interactive run, else on.
type initRea struct {
	*reaRun
	// preset is the decision the wizard cannot change: the flag, else the
	// existing decision, else undecided.
	preset reaState
}

// prepareInitRea runs where init replaces the existing configuration with a
// fresh DefaultFullConfig. It carries an existing features.rea into cfg and
// resolves the preset over cfg's platforms. The existing file is read without
// writing its normalization back.
func prepareInitRea(dir string, cfg *config.HarnessConfig, on, off bool) *initRea {
	held := false
	if existing, err := config.LoadPreview(dir); err == nil && existing.Features.Rea.Enabled != nil {
		cfg.Features.Rea = existing.Features.Rea
		held = true
	}
	preset := reaFlagState(on, off)
	if preset == reaUndecided {
		preset = resolveReaDecision(dir, cfg).State
	}
	run := newReaRun(dir, cfg, held)
	run.wholeSave = true // init writes autopus.yaml whole from cfg
	return &initRea{reaRun: run, preset: preset}
}

// wizardPreset is InitWizardOpts.Rea: the preset decision, or nil so the
// wizard asks.
func (r *initRea) wizardPreset() *bool {
	switch r.preset {
	case reaOn:
		return reaBool(true)
	case reaOff:
		return reaBool(false)
	}
	return nil
}

// decideInit settles the run's decision before the platform writes. answer
// is the wizard's REA answer on an interactive run and nil otherwise; with no
// preset, nil leaves the project undecided, so nothing is registered. No
// shipped agent consumes the REA MCP since v0.51.0, so it is opt-in.
func (r *initRea) decideInit(cmd *cobra.Command, cfg *config.HarnessConfig, answer *bool) {
	state := r.preset
	if state == reaUndecided && answer != nil {
		state = reaOff
		if *answer {
			state = reaOn
		}
	}
	r.decide(reaContext(cmd), cmd.OutOrStdout(), cfg, state)
	r.snapshot(cfg.Platforms)
}

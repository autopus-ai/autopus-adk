package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// initInteractive is the interactive part of one `auto init`
// (SPEC-TUIUX-001, Navigation Contract init rows): the wizard, the
// composition stage, and the review, in a loop that moves back between them.
// Nothing in it writes a file, so 취소 and Ctrl+C anywhere end init with the
// project untouched.
type initInteractive struct {
	cmd *cobra.Command
	dir string
	cfg *config.HarnessConfig
	// quality is the --quality flag, providers the detected orchestra
	// providers, opts the wizard's options.
	quality   string
	providers []string
	opts      tui.InitWizardOpts
	rea       *initRea
	// base is the config before any wizard answer: each pass of the wizard
	// starts from it, and the review shows what the run changes from it.
	base *config.HarnessConfig
}

// initPlace is where the loop goes next.
type initPlace int

const (
	initAtWizard initPlace = iota
	initAtStage
	initAtStageResume
	initAtReview
)

// run returns the Codex catalog snapshot the run generates with, or reports
// cancelled. The wizard resumes at its last step whenever the loop goes back
// to it, and every such pass starts from the base config again, so a
// dropped composition leaves nothing. The review's 뒤로 resumes the stage
// with its composition kept when the stage asked forms; the review appears
// only when the composition step takes its form path (Form Gate Contract).
func (r *initInteractive) run() (driftGenerationSnapshot, bool, error) {
	base, err := cloneHarnessConfig(r.cfg)
	if err != nil {
		return driftGenerationSnapshot{}, false, err
	}
	r.base = base
	var result *tui.InitWizardResult
	var stage initStage
	for place := initAtWizard; ; {
		switch place {
		case initAtWizard:
			opts := r.opts
			opts.Resume = result
			answered, err := runInitWizard(opts)
			if err != nil || answered.Cancelled {
				return driftGenerationSnapshot{}, err == nil, err
			}
			if result != nil {
				if err := r.restore(); err != nil {
					return driftGenerationSnapshot{}, false, err
				}
			}
			result = answered
			r.applyAnswers(result)
			place = initAtStage
		case initAtStage, initAtStageResume:
			stage = runInitComposeStage(r.cmd, r.dir, r.cfg, r.quality, result.Quality, place == initAtStageResume)
			switch stage.outcome {
			case initStageCancelled:
				return stage.snapshot, true, nil
			case initStageBack:
				place = initAtWizard
				continue
			}
			// The claude review model follows the chosen preset's reviewer
			// before the reviewer step shows it.
			config.ApplyClaudeReviewFollow(r.cfg)
			if initReviewerWanted(r.cmd, r.cfg) {
				err := runInitReviewerStep(r.cmd, r.dir, r.cfg, stage.snapshot)
				switch initReviewerOutcome(err) {
				case initStageCancelled:
					return stage.snapshot, true, nil
				case initStageBack:
					place = initAtWizard
					continue
				}
				if err != nil {
					warnUpdateCompose(r.cmd.OutOrStdout(), composeLanguage(), err)
				}
				stage.asked = true
			}
			r.applyMethodology(result)
			r.rea.decideInit(r.cmd, r.cfg, result.Rea)
			if !composeFormsProbe(r.cmd) {
				return stage.snapshot, false, nil
			}
			place = initAtReview
		default:
			choice, err := r.review(result)
			switch {
			case err != nil:
				return driftGenerationSnapshot{}, false, err
			case choice == initReviewApply:
				return stage.snapshot, false, nil
			case choice == initReviewCancel:
				return driftGenerationSnapshot{}, true, nil
			case stage.asked:
				place = initAtStageResume
			default:
				place = initAtWizard
			}
		}
	}
}

// restore puts the base config back before a later pass of the wizard.
func (r *initInteractive) restore() error {
	clone, err := cloneHarnessConfig(r.base)
	if err != nil {
		return fmt.Errorf("restore init config: %w", err)
	}
	*r.cfg = *clone
	return nil
}

// applyAnswers applies the wizard's answers that come before the stage.
func (r *initInteractive) applyAnswers(result *tui.InitWizardResult) {
	r.cfg.UsageProfile = config.UsageProfile(result.UsageProfile)
	r.cfg.Language.Comments = result.CommentsLang
	r.cfg.Language.Commits = result.CommitsLang
	r.cfg.Language.AIResponses = result.AILang
	// Before the compose step: its review tab edits these keys, and nothing
	// after it may overwrite them (SPEC-REVMODEL-001 REQ-RVM-13).
	r.cfg.Spec.ReviewGate.Enabled = result.ReviewGate
	if result.ReviewGate {
		r.cfg.Spec.ReviewGate.Providers = r.providers
	}
}

// applyMethodology applies the wizard's methodology answer after the stage.
func (r *initInteractive) applyMethodology(result *tui.InitWizardResult) {
	if result.Methodology == "tdd" {
		r.cfg.Methodology.Mode = "tdd"
		r.cfg.Methodology.Enforce = true
		return
	}
	r.cfg.Methodology.Mode = "none"
	r.cfg.Methodology.Enforce = false
}

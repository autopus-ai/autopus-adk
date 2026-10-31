package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// qualityReviewNoJudge is the judge argument of a review without a judge.
const qualityReviewNoJudge = "none"

// qualityReviewEdit is one `auto quality review` edit: the tab's baseline,
// read from autopus.yaml as written, the working value the subcommand
// changes, and the lists that attest a changed model (REQ-RVM-02, 05).
type qualityReviewEdit struct {
	cmd      *cobra.Command
	dir      string
	cfg      *config.HarnessConfig
	lang     string
	text     composeReviewText
	baseline reviewBaseline
	working  reviewComposition
	attested reviewAttestation
}

// startQualityReviewEdit loads the config like every quality command and
// takes the tab's baseline. Names it cannot read as written refuse the
// edit, as they end the tab.
func startQualityReviewEdit(cmd *cobra.Command, apply *bool) (*qualityReviewEdit, error) {
	if err := qualityReviewNoApply(apply); err != nil {
		return nil, err
	}
	dir, cfg, err := loadQualityConfig(cmd)
	if err != nil {
		return nil, err
	}
	lang := composeLanguage()
	e := &qualityReviewEdit{
		cmd: cmd, dir: dir, cfg: cfg, lang: lang, text: composeReviewTextFor(lang),
		baseline: newReviewBaseline(cfg, readReviewRaw(dir)), attested: newReviewAttestation(),
	}
	if !e.baseline.NamesOK {
		return nil, errors.New(e.text.NamesRefused)
	}
	e.working = e.baseline.reviewComposition.clone()
	return e, nil
}

// provider checks a provider argument against the orchestra.providers
// entries, the only providers the tab shows.
func (e *qualityReviewEdit) provider(name string) error {
	if slices.Contains(e.baseline.Visible, name) {
		return nil
	}
	return e.refusal(fmt.Sprintf(e.text.UnknownProvider, name), e.baseline.Visible)
}

// refusal is line followed by the values the argument could have been.
func (e *qualityReviewEdit) refusal(line string, accepted []string) error {
	values := e.text.None
	if len(accepted) > 0 {
		values = strings.Join(accepted, ", ")
	}
	return fmt.Errorf("%s (%s)", line, fmt.Sprintf(e.text.Accepted, values))
}

// finish validates the working value with the tab's planner and prints the
// change lines and notes the tab's preview prints, then writes it with the
// tab's line splice writer (REQ-RVM-09, 12). An edit equal to the baseline
// and a --dry-run write nothing. A judge that takes no part names the judge
// subcommand, which is the way out.
func (e *qualityReviewEdit) finish(dryRun bool) error {
	judge := e.working.Judge
	if e.baseline.participants(e.working) >= 2 && judge != "" && !e.working.takesPart(judge) {
		return fmt.Errorf("%s; %s", fmt.Sprintf(e.text.ErrJudge, e.baseline.judgeText(judge)), e.text.JudgeFirst)
	}
	plan, err := planReviewComposition(e.baseline, e.working, e.attested, e.text)
	if err != nil {
		return err
	}
	compose, out := composeTextFor(e.lang), e.cmd.OutOrStdout()
	if !plan.Changed {
		fmt.Fprintln(out, compose.NoChange)
		return nil
	}
	fmt.Fprintln(out, compose.ChangesHeading)
	for _, change := range plan.Changes {
		fmt.Fprintf(out, "  %s\n", terminalSafe(change))
	}
	for _, note := range plan.Notes {
		fmt.Fprintln(out, terminalSafe(note))
	}
	if dryRun {
		fmt.Fprintln(out, e.text.DryRun)
		return nil
	}
	err = saveReviewComposition(e.dir, e.cfg, e.baseline, e.working)
	var refusal reviewWriteRefusal
	var reload composeReviewReloadError
	switch {
	case errors.As(err, &refusal):
		return fmt.Errorf(e.text.WriteRefused, refusal.key)
	case errors.As(err, &reload):
		return fmt.Errorf(e.text.ReloadFailed, reload.err.Error())
	case err != nil:
		return err
	}
	fmt.Fprintln(out, compose.Applied)
	return nil
}

func newQualityReviewToggleCmd(apply *bool, on bool) *cobra.Command {
	use, short := "off <provider>...", "Stop providers from taking part in auto spec review"
	if on {
		use, short = "on <provider>...", "Let providers take part in auto spec review"
	}
	var dryRun bool
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQualityReviewToggle(cmd, apply, on, args, dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the settings that change without writing them")
	return cmd
}

// runQualityReviewToggle turns the named providers on or off with the
// tab's participation rule (REQ-RVM-03): the current list without the ones
// turned off, then the ones turned on in provider order. Several names
// change together, so a review below two providers can reach two at once.
func runQualityReviewToggle(cmd *cobra.Command, apply *bool, on bool, names []string, dryRun bool) error {
	e, err := startQualityReviewEdit(cmd, apply)
	if err != nil {
		return err
	}
	parts := map[string]bool{}
	for _, name := range e.baseline.Visible {
		parts[name] = e.working.takesPart(name)
	}
	for _, name := range names {
		if err := e.provider(name); err != nil {
			return err
		}
		parts[name] = on
	}
	e.working.Providers = e.baseline.participation(parts)
	return e.finish(dryRun)
}

func newQualityReviewJudgeCmd(apply *bool) *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "judge <provider|none>",
		Short: "Choose the judge of auto spec review, or none for a review without one",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runQualityReviewJudge(cmd, apply, args[0], dryRun)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the settings that change without writing them")
	return cmd
}

// runQualityReviewJudge sets spec.review_gate.judge to a provider that
// takes part, or to none. A judge written as a placeholder stays, with the
// tab's value refusal line (REQ-RVM-26).
func runQualityReviewJudge(cmd *cobra.Command, apply *bool, judge string, dryRun bool) error {
	e, err := startQualityReviewEdit(cmd, apply)
	if err != nil {
		return err
	}
	if raw := e.baseline.RawJudge; strings.Contains(raw, "${") {
		return fmt.Errorf(e.text.CLIValueRefused, "spec.review_gate", "judge", raw)
	}
	accepted := slices.DeleteFunc(slices.Clone(e.baseline.Visible), func(name string) bool { return !e.working.takesPart(name) })
	switch {
	case judge == qualityReviewNoJudge && !slices.Contains(e.baseline.Visible, judge):
		e.working.Judge = ""
	case slices.Contains(accepted, judge):
		e.working.Judge = judge
	default:
		return e.refusal(fmt.Sprintf(e.text.ErrJudge, judge), append(accepted, qualityReviewNoJudge))
	}
	return e.finish(dryRun)
}

package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// newQualityReviewCmd is `auto quality review`, the Multi-provider review
// tab without a terminal (SPEC-REVMODEL-001 Review Tab Contract): show
// prints what the tab shows, and model, on, off, and judge plan one edit
// with the tab's planner and attested lists and write it with its line
// splice writer.
func newQualityReviewCmd(apply *bool, deps ompPlatformDependencies) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "review",
		Short: "Show or change the providers, models, and judge of multi-provider review",
		Long:  qualityReviewCmdLong,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runQualityReviewShow(cmd, apply, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the review settings as JSON")
	cmd.AddCommand(newQualityReviewShowCmd(apply))
	cmd.AddCommand(newQualityReviewModelCmd(apply, deps))
	cmd.AddCommand(newQualityReviewToggleCmd(apply, true), newQualityReviewToggleCmd(apply, false))
	cmd.AddCommand(newQualityReviewJudgeCmd(apply))
	return cmd
}

func newQualityReviewShowCmd(apply *bool) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show each orchestra provider's part in review, backend, model, and effort, and the judge",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runQualityReviewShow(cmd, apply, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print the review settings as JSON")
	return cmd
}

// qualityReviewNoApply refuses the --apply every quality subcommand
// inherits: the review settings are read when a review runs, so there is
// no harness to regenerate.
func qualityReviewNoApply(apply *bool) error {
	if apply != nil && *apply {
		return errors.New(composeReviewTextFor(composeLanguage()).ApplyRefused)
	}
	return nil
}

// runQualityReviewShow prints the review settings as the tab reads them:
// the baseline of the file as written, never an environment value
// (REQ-RVM-26). Names the file does not hold as written refuse, as they end
// the tab.
func runQualityReviewShow(cmd *cobra.Command, apply *bool, asJSON bool) error {
	if err := qualityReviewNoApply(apply); err != nil {
		return err
	}
	dir, cfg, err := loadQualityConfig(cmd)
	if err != nil {
		return err
	}
	text := composeReviewTextFor(composeLanguage())
	b := newReviewBaseline(cfg, readReviewRaw(dir))
	if !b.NamesOK {
		return errors.New(text.NamesRefused)
	}
	path := filepath.Join(dir, "autopus.yaml")
	if asJSON {
		return writeJSONResult(cmd, jsonStatusOK, qualityReviewData(b, cfg.Spec.ReviewGate.Enabled, path), nil, nil)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "spec.review_gate.enabled = %t\n", cfg.Spec.ReviewGate.Enabled)
	fmt.Fprintf(out, "spec.review_gate.judge = %s\n", terminalSafe(reviewJudgeText(b.RawJudge, text)))
	writeReviewTable(out, qualityReviewTable(b, text))
	fmt.Fprintf(out, "config = %s\n", terminalSafe(path))
	return nil
}

// qualityReviewTable is the show table: per orchestra provider its part in
// the review and its backend and model as the tab shows them.
func qualityReviewTable(b reviewBaseline, text composeReviewText) composeTableLayout {
	layout := composeTableLayout{header: []string{text.ColProvider, text.ColReview, text.ColBackend, text.ColModel}}
	for _, name := range b.Visible {
		state := text.Off
		if b.takesPart(name) {
			state = text.On
		}
		backend, model := b.shownModel(name, text)
		layout.rows = append(layout.rows, []string{name, state, backend, model})
	}
	return escapeComposeTableLayout(layout)
}

// qualityReviewProvider is one provider of `auto quality review show
// --json`, its values as autopus.yaml writes them: backend is omp or cli,
// and effort is an OMP provider's thinking level.
type qualityReviewProvider struct {
	Name      string `json:"name"`
	TakesPart bool   `json:"takes_part"`
	Backend   string `json:"backend"`
	Binary    string `json:"binary,omitempty"`
	Model     string `json:"model"`
	Effort    string `json:"effort"`
}

type qualityReviewSettings struct {
	Config            string                  `json:"config"`
	ReviewGateEnabled bool                    `json:"review_gate_enabled"`
	ReviewProviders   []string                `json:"review_providers"`
	Judge             string                  `json:"judge"`
	Providers         []qualityReviewProvider `json:"providers"`
}

// qualityReviewData is the JSON data of show, read from the raw document
// like the table.
func qualityReviewData(b reviewBaseline, enabled bool, path string) qualityReviewSettings {
	data := qualityReviewSettings{
		Config: path, ReviewGateEnabled: enabled, ReviewProviders: b.Providers, Judge: b.RawJudge,
		Providers: []qualityReviewProvider{},
	}
	if data.ReviewProviders == nil {
		data.ReviewProviders = []string{}
	}
	for _, name := range b.Visible {
		provider := qualityReviewProvider{Name: name, TakesPart: b.takesPart(name), Backend: "cli", Binary: b.binaryText(name)}
		raw := b.Raw[name]
		if b.OMP[name] {
			provider.Backend, provider.Binary, provider.Model = "omp", "", raw.Model
			if model, thinking, ok := orchestra.SplitModelSelector(raw.Model); ok {
				provider.Model, provider.Effort = model, thinking
			}
		} else {
			value := reviewCLIValueOf(reviewArgsRowFor(b.Binaries[name]), raw.Args)
			provider.Model, provider.Effort = value.Model, value.Effort
		}
		data.Providers = append(data.Providers, provider)
	}
	return data
}

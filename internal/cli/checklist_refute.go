package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/checklist"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// checklistProviders lists the review providers usable on this machine and
// checklistAsker asks one of them; tests replace both.
var (
	checklistProviders = defaultChecklistProviders
	checklistAsker     = defaultChecklistAsker
)

func defaultChecklistProviders() ([]string, error) {
	conf, err := loadOrchestraConfig()
	if err != nil {
		return nil, err
	}
	var names []string
	for _, p := range resolveProviders(conf, "review", nil) {
		if _, err := exec.LookPath(p.Binary); err == nil {
			names = append(names, p.Name)
		}
	}
	return names, nil
}

func defaultChecklistAsker(root string) checklist.Asker {
	return func(ctx context.Context, name, prompt string) (string, error) {
		conf, err := loadOrchestraConfig()
		if err != nil {
			return "", err
		}
		providers, err := applyReadOnlyProviderPolicy(resolveProviders(conf, "review", []string{name}), readOnlyPolicyOptions{})
		if err != nil || len(providers) == 0 {
			return "", fmt.Errorf("provider %s: %v", name, err)
		}
		p := providers[0]
		cfg := orchestra.OrchestraConfig{
			Providers: []orchestra.ProviderConfig{p}, TimeoutSeconds: 600,
			WorkingDir: root, ProviderWorkDir: root, ReadOnly: true,
		}
		resp, err := orchestra.RunSingleProvider(ctx, cfg, p, prompt)
		if resp == nil {
			return "", err
		}
		return resp.Output, err
	}
}

func newChecklistRefuteCmd() *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "refute [CHK-NNN]",
		Short: "Ask a provider other than the drafter to refute pending candidates",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all == (len(args) == 1) {
				return errors.New("give one candidate id or --all")
			}
			root, err := checklistRoot()
			if err != nil {
				return err
			}
			cands, err := checklist.LoadCandidates(root)
			if err != nil {
				return err
			}
			providers, err := checklistProviders()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			ran := 0
			for _, c := range cands {
				if c.State != checklist.StatePendingRefutation || (!all && c.ID != args[0]) {
					continue
				}
				ran++
				got, err := checklist.Refute(cmd.Context(), root, c, providers, checklistAsker(root), checklistNow())
				if errors.Is(err, checklist.ErrNoRefuter) {
					fmt.Fprintf(out, "%s %s — %v\n", c.ID, c.State.Label(), err)
					continue
				}
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%s %s\n", got.ID, got.State.Label())
				for _, r := range got.Refutations {
					fmt.Fprintf(out, "  %s: %s — %s\n", r.Provider, verdictLabel(r.Verdict), intakePrintable(r.Reason))
				}
			}
			if ran == 0 {
				fmt.Fprintln(out, "반증 검토 대기 중인 후보가 없습니다")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Refute every candidate waiting for refutation")
	return cmd
}

// verdictLabel is a refutation verdict as the reports print it; JSON keeps
// the stored value.
func verdictLabel(v string) string {
	switch v {
	case "refuted":
		return "반증됨"
	case "survived":
		return "반증 실패"
	case "unparsed":
		return "판정을 읽지 못함"
	case "error":
		return "오류"
	}
	return v
}

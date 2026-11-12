package cli

import (
	"bufio"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
)

// updateModelChange is one model default `auto update` can move: the line
// that names it and the write that applies it to autopus.yaml and cfg.
type updateModelChange struct {
	line  string
	apply func() error
}

// updateModelText is the operator-facing text of update's change step.
type updateModelText struct {
	Heading, Question, Apply, Keep, Applied, Kept, Notice string
	ClaudeReview, ReviewList, ReviewListSame              string
}

var updateModelTextKO = updateModelText{
	Heading:  "모델 기본값이 바뀌었습니다:",
	Question: "이 변경을 적용할까요?", Apply: "적용", Keep: "그대로 두기",
	Applied: "적용했습니다.", Kept: "그대로 둡니다. 모델 전체 편집: auto update --configure-models",
	Notice:       "참고: 적용할 수 있는 모델 변경이 있습니다. 터미널에서 auto update로 적용합니다:",
	ClaudeReview: "claude 리뷰 model %s → %s (preset의 reviewer model)",
	ReviewList:   "orchestra review provider %s → review gate 목록 %s (spec.review_gate.providers)",
	ReviewListSame: "orchestra review provider 목록을 지우고 review gate 목록 %s를 따름 " +
		"(spec.review_gate.providers, 같은 CLI)",
}

var updateModelTextEN = updateModelText{
	Heading:  "Model defaults changed:",
	Question: "Apply these changes?", Apply: "Apply", Keep: "Keep as is",
	Applied: "Applied.", Kept: "Kept. Full model editing: auto update --configure-models",
	Notice:       "Note: model changes are available; run auto update in a terminal to apply them:",
	ClaudeReview: "claude review model %s → %s (the preset's reviewer model)",
	ReviewList:   "orchestra review providers %s → review gate list %s (spec.review_gate.providers)",
	ReviewListSame: "orchestra review drops its own provider list and follows the review gate list %s " +
		"(spec.review_gate.providers, same CLIs)",
}

func updateModelTextFor(lang string) updateModelText {
	if lang == "ko" {
		return updateModelTextKO
	}
	return updateModelTextEN
}

// planUpdateModelChanges lists the model defaults that moved since cfg was
// written and that ADK still owns: an ADK-written review command list that
// the review gate's list replaces, and a claude review model that no longer
// follows the preset's reviewer. Values an operator chose are not listed.
func planUpdateModelChanges(dir string, cfg *config.HarnessConfig, text updateModelText) []updateModelChange {
	var changes []updateModelChange
	if config.ReviewProvidersDerivable(cfg) {
		current := cfg.Orchestra.Commands["review"].Providers
		from, to := strings.Join(current, ", "), strings.Join(cfg.Spec.ReviewGate.Providers, ", ")
		line := fmt.Sprintf(text.ReviewList, terminalSafe(from), terminalSafe(to))
		if slices.Equal(slices.Sorted(slices.Values(current)), slices.Sorted(slices.Values(cfg.Spec.ReviewGate.Providers))) {
			// Same CLIs either way: only the duplicate list goes.
			line = fmt.Sprintf(text.ReviewListSame, terminalSafe(to))
		}
		changes = append(changes, updateModelChange{
			line: line,
			apply: func() error {
				if _, err := pruneConfigFileKeys(dir, []string{config.ReviewDerivedProvidersKey}); err != nil {
					return err
				}
				config.DeriveReviewProviders(cfg)
				return nil
			},
		})
	}
	if follow, ok := config.ClaudeReviewFollowFor(cfg); ok {
		if _, _, readable := claudeReviewProvider(dir, cfg); readable {
			changes = append(changes, updateModelChange{
				line: fmt.Sprintf(text.ClaudeReview, follow.FromModel+" · "+follow.FromEffort,
					follow.ToModel+" · "+follow.ToEffort),
				apply: func() error { return applyClaudeReviewFollowFile(dir, cfg, follow) },
			})
		}
	}
	return changes
}

// applyClaudeReviewFollowFile writes the claude review model move through
// the review tab's line writer, so only the two values change in the file.
func applyClaudeReviewFollowFile(dir string, cfg *config.HarnessConfig, follow config.ClaudeReviewFollow) error {
	baseline := newReviewBaseline(cfg, readReviewRaw(dir))
	if !baseline.NamesOK || !slices.Contains(baseline.Visible, "claude") || baseline.OMP["claude"] {
		return reviewWriteRefusal{key: "orchestra.providers.claude.args"}
	}
	working := baseline.reviewComposition.clone()
	working.CLI["claude"] = reviewCLIValue{Model: follow.ToModel, Effort: follow.ToEffort}
	return saveReviewComposition(dir, cfg, baseline, working)
}

// runUpdateModelChanges is the default interactive model step of `auto
// update`: it names each change and asks one confirm. Nothing is asked
// without a change. It writes only on an explicit yes.
func runUpdateModelChanges(cmd *cobra.Command, dir string, cfg *config.HarnessConfig, lang string) error {
	text := updateModelTextFor(lang)
	changes := planUpdateModelChanges(dir, cfg, text)
	if len(changes) == 0 {
		return nil
	}
	out := cmd.OutOrStdout()
	writeUpdateModelLines(out, text.Heading, changes)
	apply, err := askUpdateModelConfirm(cmd, text)
	if err != nil || !apply {
		fmt.Fprintln(out, "  "+text.Kept)
		return err
	}
	for _, change := range changes {
		if err := change.apply(); err != nil {
			return err
		}
	}
	fmt.Fprintln(out, "  "+text.Applied)
	return nil
}

// writeUpdateModelNotice is the non-interactive form: the same lines, no read.
func writeUpdateModelNotice(out io.Writer, dir string, cfg *config.HarnessConfig, lang string) bool {
	text := updateModelTextFor(lang)
	changes := planUpdateModelChanges(dir, cfg, text)
	if len(changes) > 0 {
		writeUpdateModelLines(out, text.Notice, changes)
	}
	return len(changes) > 0
}

func writeUpdateModelLines(out io.Writer, heading string, changes []updateModelChange) {
	fmt.Fprintln(out, "  "+heading)
	for _, change := range changes {
		fmt.Fprintln(out, "    - "+change.line)
	}
}

// askUpdateModelConfirm asks through a form when forms run, otherwise one
// line where only y or yes applies.
func askUpdateModelConfirm(cmd *cobra.Command, text updateModelText) (bool, error) {
	if composeFormsProbe(cmd) {
		return composeSelect(cmd, text.Question, "", []composeOption[bool]{{text.Apply, true}, {text.Keep, false}}, true)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "  %s [y/N] ", text.Question)
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}

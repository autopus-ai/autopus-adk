package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harnessfb"
)

// newHarnessEncodeCmd turns an approved lesson into a check the harness has
// run itself, or records why no check fits.
func newHarnessEncodeCmd() *cobra.Command {
	var layer, check, repro, reason string
	cmd := &cobra.Command{
		Use:   "encode HL-NNN",
		Short: "Prove a check that enforces an approved lesson, so its text rule leaves " + harnessfb.RuleFile,
		Long: "Without flags, prints how to turn the lesson into a check. With --layer, --check and --repro, the harness " +
			"runs the check on the current tree (must pass), on a snapshot copy of it (must pass) and on the copy with the " +
			"reproduction patch applied (must fail); the current tree is not changed. Only then is the lesson proven and " +
			"taken out of " + harnessfb.RuleFile + ". --layer text --reason records that no check fits; the text rule stays.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A signal must reach the check's process tree and let the
			// copy be removed; without this the process dies mid-proof.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			root, err := harnessRoot(ctx)
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if layer == "" {
				if check != "" || repro != "" || reason != "" {
					return fmt.Errorf("--layer가 필요합니다")
				}
				c, err := harnessfb.ApprovedCandidate(root, args[0])
				if err != nil {
					return err
				}
				fmt.Fprint(out, harnessfb.Brief(c))
				return nil
			}
			l, err := harnessfb.ParseLayer(layer)
			if err != nil {
				return err
			}
			var e harnessfb.Encoding
			var written []string
			if l == harnessfb.LayerText {
				if check != "" || repro != "" {
					return fmt.Errorf("--layer text에는 --check와 --repro를 쓰지 않습니다: 이유만 --reason으로 남깁니다")
				}
				e, written, err = harnessfb.KeepAsText(root, args[0], reason, harnessNow())
			} else {
				if reason != "" {
					return fmt.Errorf("--reason은 --layer text에만 씁니다")
				}
				e, written, err = harnessfb.Prove(ctx, root, harnessfb.Proof{
					ID: args[0], Tree: worktreeTop(ctx, cwd), Layer: l, Check: check, ReproPath: repro,
				}, harnessNow())
			}
			var perr *harnessfb.ProofError
			if errors.As(err, &perr) {
				if perr.Output != "" {
					fmt.Fprintf(out, "검사 출력 끝부분:\n%s\n", perr.Output)
				}
				return fmt.Errorf("%s: %w (교훈은 임시 글 규칙으로 남고, 고친 뒤 다시 제출할 수 있습니다)", args[0], err)
			}
			reportRuleFiles(out, written)
			if err != nil {
				return err
			}
			if e.Structural() {
				fmt.Fprintf(out, "%s 증명됨: %s 층, 검사 `%s`, patch가 바꾼 파일 %s. %s에서 뺐습니다\n",
					e.ID, e.Layer, e.Check, strings.Join(e.ReproPaths, ", "), harnessfb.RuleFile)
			} else {
				fmt.Fprintf(out, "%s는 글 규칙으로 남습니다: %s\n", e.ID, e.Reason)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&layer, "layer", "", "Where the check enforces the lesson: api, lint, test, hook, or text")
	cmd.Flags().StringVar(&check, "check", "", "Shell command that passes on the current tree and fails with the mistake in place")
	cmd.Flags().StringVar(&repro, "repro", "", "Patch file that reproduces the lesson's mistake on the current tree")
	cmd.Flags().StringVar(&reason, "reason", "", "Why no check fits (with --layer text)")
	return cmd
}

// printHarnessEncoding shows where an approved lesson is enforced.
func printHarnessEncoding(w io.Writer, root string, c harnessfb.Candidate) {
	e, ok := harnessfb.LoadEncoding(root, c)
	switch {
	case !ok:
		fmt.Fprintf(w, "  구조화: 대기 (임시 글 규칙) — auto harness encode %s\n", c.ID)
	case e.Structural():
		fmt.Fprintf(w, "  구조화: %s 층, 검사 `%s` (증명 %s, 재현 patch sha256 %.12s)\n",
			e.Layer, intakePrintable(e.Check), e.Proved.Format("2006-01-02"), e.ReproSHA256)
		if e.Branch != "" {
			fmt.Fprintf(w, "  증명한 브랜치: %s (검사가 이 브랜치에만 있으면 다른 브랜치에서는 막지 못합니다)\n", intakePrintable(e.Branch))
		}
		fmt.Fprintf(w, "  재현 patch가 바꾼 파일: %s\n", intakePrintable(strings.Join(e.ReproPaths, ", ")))
		if e.FailOutput != "" {
			fmt.Fprintf(w, "  재현 patch에서 검사가 낸 출력: %s\n", intakePrintable(e.FailOutput))
		}
	default:
		fmt.Fprintf(w, "  구조화: 글 규칙 유지 — %s\n", intakePrintable(e.Reason))
	}
}

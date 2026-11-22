package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sort"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/checklist"
)

// errChecklistFailed is the exit of a run in which an enforce check flagged.
var errChecklistFailed = errors.New("강제 모드 점검이 실패했습니다")

func newChecklistRunCmd() *cobra.Command {
	var base, format string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Apply the checks whose diff condition matches the change",
		Long: "Reads the checks from the merge-base of --base and HEAD, so a change cannot alter the checks that judge it, " +
			"matches each against committed and uncommitted changes, and runs the verify command of every matching check. " +
			"An observe check that flags is recorded and does not fail the run; an enforce check that flags exits non-zero.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := checklistRoot()
			if err != nil {
				return err
			}
			// Ctrl-C or a TERM from a tool timeout cancels the context, so the
			// verify command's process group is stopped instead of orphaned.
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			rep, err := checklist.Run(ctx, root, base, checklist.ShellRunner, checklistNow())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if format == "json" {
				if err := printJSON(out, rep); err != nil {
					return err
				}
			} else {
				printRunReport(out, rep)
			}
			if rep.Failed() {
				cmd.SilenceUsage = true
				return errChecklistFailed
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "Ref whose merge-base with HEAD holds the checks (default: origin/HEAD when known, else HEAD)")
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text|json)")
	return cmd
}

func printRunReport(out io.Writer, rep checklist.Report) {
	applied := 0
	for _, o := range rep.Outcomes {
		if o.Result != checklist.ResultSkipped {
			applied++
		}
	}
	fmt.Fprintf(out, "점검 %d개 중 %d개 적용\n", len(rep.Outcomes), applied)
	for _, o := range rep.Outcomes {
		fmt.Fprintf(out, "  %s [%s] %s\n", o.ID, modeLabel(o.Mode), outcomeLabel(o))
		if o.Result == checklist.ResultReview {
			fmt.Fprintf(out, "    리뷰 기준: %s\n", firstLineOf(o.Body))
		}
		if (o.Result == checklist.ResultFlagged || o.Result == checklist.ResultError) && o.Output != "" {
			fmt.Fprintf(out, "    출력: %s\n", intakePrintable(lastLineOf(o.Output)))
		}
	}
	for _, p := range sortedStringKeys(rep.ChangedChecks) {
		fmt.Fprintf(out, "  이번 변경이 바꾼 점검 %s (%s): 커밋 전에는 적용하지 않습니다\n", p, rep.ChangedChecks[p])
	}
	for _, p := range sortedStringKeys(rep.Invalid) {
		fmt.Fprintf(out, "  형식 오류로 제외 %s: %s\n", p, rep.Invalid[p])
	}
	if rep.Warning != "" {
		fmt.Fprintf(out, "  경고: %s\n", rep.Warning)
	}
}

func outcomeLabel(o checklist.Outcome) string {
	switch o.Result {
	case checklist.ResultSkipped:
		return "조건 불일치로 건너뜀"
	case checklist.ResultClean:
		return "통과"
	case checklist.ResultReview:
		return "적용됨: 리뷰어가 아래 기준으로 판단합니다"
	case checklist.ResultError:
		return "실행 오류"
	case checklist.ResultFlagged:
		if o.Mode == checklist.ModeObserve {
			return "관찰: 문제를 잡았습니다 (검증은 실패시키지 않음)"
		}
		return "실패"
	}
	return string(o.Result)
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func firstLineOf(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			return s[:i]
		}
	}
	return s
}

func lastLineOf(s string) string {
	end := len(s)
	for end > 0 && (s[end-1] == '\n' || s[end-1] == ' ') {
		end--
	}
	for i := end - 1; i >= 0; i-- {
		if s[i] == '\n' {
			return s[i+1 : end]
		}
	}
	return s[:end]
}

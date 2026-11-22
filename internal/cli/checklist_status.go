package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/checklist"
)

func newChecklistStatusCmd() *cobra.Command {
	var format string
	var retireAfter int
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report lessons, candidates, the approval queue, and check health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := checklistRoot()
			if err != nil {
				return err
			}
			entries, err := readLessons(root)
			if err != nil {
				return err
			}
			st, err := checklist.BuildStatus(root, entries, retireAfter)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if format == "json" {
				return printJSON(out, map[string]any{"status": st, "approval_queue": st.ApprovalQueue()})
			}
			printStatus(out, st, retireAfter)
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text|json)")
	cmd.Flags().IntVar(&retireAfter, "retire-after", checklist.DefaultRetireAfter,
		"Report a check for retirement after this many applications without a flag")
	return cmd
}

func printStatus(out io.Writer, st checklist.Status, retireAfter int) {
	printLessons(out, st.Lessons)
	fmt.Fprintln(out, "후보:")
	for _, c := range st.Candidates {
		fmt.Fprintf(out, "  %s %s — 반복 %d회%s\n", c.ID, c.State.Label(), c.Repeats, reproSuffix(c))
		for _, r := range c.Refutations {
			fmt.Fprintf(out, "    반증 %s: %s — %s\n", r.Provider, verdictLabel(r.Verdict), intakePrintable(r.Reason))
		}
	}
	queue := st.ApprovalQueue()
	fmt.Fprintf(out, "승인 대기열: %d개\n", len(queue))
	for _, c := range queue {
		fmt.Fprintf(out, "  %s: %s (auto checklist approve %s | reject %s)\n", c.ID, intakePrintable(firstLineOf(c.Check.Body)), c.ID, c.ID)
		if c.Check.Verify != "" {
			fmt.Fprintf(out, "    승인하면 조건에 맞는 diff에서 실행할 명령: %s\n", intakePrintable(c.Check.Verify))
		}
	}
	fmt.Fprintln(out, "점검:")
	for _, c := range st.Checks {
		last := "없음"
		if c.LastFlagged != nil {
			last = c.LastFlagged.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(out, "  %s [%s] 적용 %d회, 잡음 %d회, 마지막으로 잡은 시점 %s\n", c.ID, modeLabel(c.Mode), c.Applied, c.Flagged, last)
		if c.Retire {
			fmt.Fprintf(out, "    퇴역 후보: 최근 %d번 적용되는 동안 잡은 것이 없습니다 (auto checklist retire %s)\n", retireAfter, c.ID)
		}
		if c.Promote {
			fmt.Fprintf(out, "    승격 제안: 관찰 모드에서 %d번 잡았습니다 (auto checklist promote %s)\n", c.Flagged, c.ID)
		}
	}
	for _, p := range sortedStringKeys(st.Invalid) {
		fmt.Fprintf(out, "  형식 오류 %s: %s\n", p, st.Invalid[p])
	}
	printMergeWarnings(out, st)
}

// printMergeWarnings reports what a merge of two branches can leave behind:
// one defect drafted as two candidates, and one id held by two lessons.
func printMergeWarnings(out io.Writer, st checklist.Status) {
	for _, fp := range sortedStringKeys(st.DuplicateFingerprints) {
		ids := st.DuplicateFingerprints[fp]
		fmt.Fprintf(out, "경고: 후보 %s는 같은 결함(지문 %s)입니다. 하나만 남기고 나머지는 거절하세요 (auto checklist reject %s)\n",
			strings.Join(ids, ", "), fp, ids[len(ids)-1])
	}
	for _, c := range st.LessonIDConflicts {
		fmt.Fprintf(out, "경고: 교훈 id %s를 서로 다른 교훈 %d개가 쓰고 있습니다 (pipeline.jsonl %v번째 줄). 이 id를 지정하는 명령은 모두에 적용됩니다\n",
			c.ID, len(c.Lines), c.Lines)
	}
}

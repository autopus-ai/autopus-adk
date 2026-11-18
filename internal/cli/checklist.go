package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/checklist"
	"github.com/insajin/autopus-adk/pkg/learn"
)

// checklistNow is the clock of the checklist commands; tests pin it.
var checklistNow = time.Now

func newChecklistCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checklist",
		Short: "Turn escape lessons into conditional checks a person approves",
		Long: "The harness lesson loop. `auto learn record --type escape` records a defect every check passed; " +
			"propose turns repeated or reproduced lessons into observe-mode candidates; refute asks a different provider " +
			"to refute each one; approve, reject, promote and retire are a person's decisions; run applies the checks " +
			"whose diff condition matches; status reports lessons, the approval queue, and retirement candidates.",
	}
	cmd.AddCommand(newChecklistProposeCmd(), newChecklistRefuteCmd(), newChecklistDecideCmd("approve"),
		newChecklistDecideCmd("reject"), newChecklistDecideCmd("promote"), newChecklistDecideCmd("retire"),
		newChecklistRunCmd(), newChecklistStatusCmd())
	return cmd
}

func checklistRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("get working directory: %w", err)
	}
	return cwd, nil
}

// readLessons returns the project's learning entries; no store is no lessons.
func readLessons(root string) ([]learn.LearningEntry, error) {
	store, err := learn.NewStore(root)
	if err != nil {
		return nil, err
	}
	entries, _, err := store.ReadTolerant()
	return entries, err
}

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func newChecklistProposeCmd() *cobra.Command {
	var format string
	cmd := &cobra.Command{
		Use:   "propose",
		Short: "Write observe-mode candidates for repeated or reproduced escape lessons",
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := checklistRoot()
			if err != nil {
				return err
			}
			entries, err := readLessons(root)
			if err != nil {
				return err
			}
			created, lessons, err := checklist.Propose(root, entries, checklistNow())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if format == "json" {
				return printJSON(out, map[string]any{"created": created, "lessons": lessons})
			}
			fmt.Fprintf(out, "새 후보 %d개\n", len(created))
			for _, c := range created {
				fmt.Fprintf(out, "  %s %s — 반복 %d회%s\n", c.ID, c.State.Label(), c.Repeats, reproSuffix(c))
			}
			printLessons(out, lessons)
			if len(created) > 0 {
				fmt.Fprintln(out, "next: auto checklist refute --all")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&format, "format", "text", "Output format (text|json)")
	return cmd
}

func reproSuffix(c checklist.Candidate) string {
	if c.Repro == nil {
		return ""
	}
	return fmt.Sprintf(", 재현 명령 %q (수정 전 exit %d, 수정 후 exit %d)", c.Repro.Command, c.Repro.BeforeExit, c.Repro.AfterExit)
}

func printLessons(out io.Writer, lessons []checklist.LessonStatus) {
	if len(lessons) == 0 {
		return
	}
	fmt.Fprintln(out, "교훈:")
	for _, l := range lessons {
		line := fmt.Sprintf("  %s %s", l.ID, l.Status)
		if l.Candidate != "" {
			line += " " + l.Candidate
		}
		if l.Detail != "" {
			line += " — " + l.Detail
		}
		fmt.Fprintln(out, line+": "+intakePrintable(l.Pattern))
	}
}

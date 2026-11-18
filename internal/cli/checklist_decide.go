package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/checklist"
)

var checklistDecideShort = map[string]string{
	"approve": "Approve a candidate: add it to the checks in observe mode",
	"reject":  "Reject a candidate without changing any check",
	"promote": "Promote an observe check to enforce, so its flag fails verification",
	"retire":  "Retire a check: keep the file but never select it",
}

func newChecklistDecideCmd(action string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " CHK-NNN",
		Short: checklistDecideShort[action],
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := checklistRoot()
			if err != nil {
				return err
			}
			id, out := args[0], cmd.OutOrStdout()
			switch action {
			case "approve":
				check, err := checklist.Approve(root, id, checklistNow())
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%s 승인됨: %s/%s.md에 관찰 모드로 추가했습니다\n", check.ID, checklist.ChecksDir, check.ID)
				if check.Verify != "" {
					fmt.Fprintf(out, "  이 점검은 조건에 맞는 diff에서 %q를 실행합니다\n", check.Verify)
				}
				fmt.Fprintln(out, "  커밋한 뒤부터 적용됩니다: 점검은 기준 커밋의 파일에서 읽습니다")
			case "reject":
				if err := checklist.Reject(root, id, checklistNow()); err != nil {
					return err
				}
				fmt.Fprintf(out, "%s 거절됨: 점검은 바뀌지 않았습니다\n", id)
			case "promote", "retire":
				mode := checklist.ModeEnforce
				if action == "retire" {
					mode = checklist.ModeRetired
				}
				check, err := checklist.SetMode(root, id, mode)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%s → %s. 커밋한 뒤부터 적용됩니다\n", check.ID, modeLabel(check.Mode))
			}
			return nil
		},
	}
}

func modeLabel(m checklist.Mode) string {
	switch m {
	case checklist.ModeObserve:
		return "관찰 모드"
	case checklist.ModeEnforce:
		return "강제 모드"
	case checklist.ModeRetired:
		return "퇴역"
	}
	return string(m)
}

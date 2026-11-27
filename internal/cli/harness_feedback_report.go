package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harnessfb"
)

func newHarnessReportCmd() *cobra.Command {
	var since time.Duration
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Show where recorded sessions spent their time and which waste repeats",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := harnessRoot(cmd.Context())
			if err != nil {
				return err
			}
			sessions, err := harnessfb.LoadSessions(root, harnessNow().Add(-since))
			if err != nil {
				return err
			}
			patterns := harnessfb.Detect(sessions)
			effects, err := harnessEffects(root)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), map[string]any{"sessions": sessions, "patterns": patterns, "effects": effects})
			}
			printHarnessReport(cmd.OutOrStdout(), sessions, patterns)
			printHarnessEffects(cmd.OutOrStdout(), effects)
			return nil
		},
	}
	cmd.Flags().DurationVar(&since, "since", harnessWindow, "Only sessions active within this long")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	return cmd
}

func printHarnessReport(w io.Writer, sessions []harnessfb.Metrics, patterns []harnessfb.Pattern) {
	if len(sessions) == 0 {
		fmt.Fprintln(w, "분석할 세션 기록이 없습니다. Claude Code 세션이 끝날 때마다 Stop hook이 기록을 남깁니다.")
		return
	}
	// Spans can overlap (reviews side by side, a background test during a
	// review), so each session's sums are capped at its active time.
	var active, test, review, repeat float64
	blocks, timeouts := 0, 0
	for _, s := range sessions {
		active += s.ActiveSeconds
		test += min(s.TestWaitSeconds, s.ActiveSeconds)
		review += min(s.ReviewSeconds, s.ActiveSeconds)
		repeat += min(s.RepeatReviewSeconds, s.ActiveSeconds)
		blocks += s.StopBlocks
		timeouts += s.Timeouts
	}
	pct := func(x float64) float64 {
		if active <= 0 {
			return 0
		}
		return 100 * x / active
	}
	fmt.Fprintf(w, "세션 %d개 · 활동 시간 %.1f시간\n", len(sessions), active/3600)
	fmt.Fprintf(w, "  테스트 대기   %4.0f%%\n  리뷰          %4.0f%%  (그중 같은 변경의 반복 리뷰 %.0f%%)\n", pct(test), pct(review), pct(repeat))
	fmt.Fprintf(w, "  Stop hook 차단 %d회 · timeout %d회\n\n", blocks, timeouts)
	printed := false
	for _, p := range patterns {
		if !p.Repeated {
			continue
		}
		if !printed {
			fmt.Fprintf(w, "반복 패턴 (세션 %d개 이상):\n", harnessfb.MinSessions)
			printed = true
		}
		fmt.Fprintf(w, "- %s: %s — 세션 %d개\n", p.ID, p.Title, len(p.Hits))
		for _, h := range p.Hits {
			fmt.Fprintf(w, "    %s  %d회", shortID(h.SessionID), h.Count)
			if h.Seconds > 0 {
				fmt.Fprintf(w, ", %.1f분, 세션 시간의 %.1f%%", h.Seconds/60, 100*h.Share)
			}
			fmt.Fprintln(w)
		}
	}
	if !printed {
		fmt.Fprintf(w, "반복 패턴이 없습니다 (세션 %d개 이상에서 나온 낭비 신호가 없음).\n", harnessfb.MinSessions)
	}
	var below []string
	for _, p := range patterns {
		if !p.Repeated {
			below = append(below, fmt.Sprintf("%s(세션 %d개)", p.ID, len(p.Hits)))
		}
	}
	if len(below) > 0 {
		fmt.Fprintf(w, "아직 기준 미달: %v\n", below)
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func newHarnessProposeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "propose",
		Short: "Draft a rule for each repeated pattern without a candidate, then ask other providers to refute it",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := harnessRoot(cmd.Context())
			if err != nil {
				return err
			}
			before, err := harnessfb.LoadCandidates(root)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if err := proposeAndRefute(cmd.Context(), root, func(s string) { fmt.Fprintln(out, s) }, false); err != nil {
				return err
			}
			after, err := harnessfb.LoadCandidates(root)
			if err == nil && len(after) == len(before) {
				fmt.Fprintln(out, "새로 후보로 만들 반복 패턴이 없습니다")
			}
			return err
		},
	}
}

func newHarnessRefuteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "refute HL-NNN",
		Short: "Ask providers other than the drafter to refute a candidate again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := harnessRoot(cmd.Context())
			if err != nil {
				return err
			}
			c, err := harnessfb.LoadCandidate(root, args[0])
			if err != nil {
				return err
			}
			providers, err := harnessProviders()
			if err != nil {
				return err
			}
			got, err := harnessfb.Refute(cmd.Context(), root, c, providers, harnessAsker(root), harnessNow())
			if err != nil && !errors.Is(err, harnessfb.ErrNoRefuter) {
				return err
			}
			printHarnessCandidate(cmd.OutOrStdout(), got)
			if err != nil {
				fmt.Fprintln(cmd.OutOrStdout(), "  "+err.Error())
			}
			return nil
		},
	}
}

func newHarnessStatusCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "List candidates: the approval queue first, then the rest by state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := harnessRoot(cmd.Context())
			if err != nil {
				return err
			}
			cands, err := harnessfb.LoadCandidates(root)
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd.OutOrStdout(), cands)
			}
			out := cmd.OutOrStdout()
			if len(cands) == 0 {
				fmt.Fprintln(out, "후보가 없습니다")
				return nil
			}
			for _, group := range []harnessfb.State{harnessfb.StateAwaitingApproval, harnessfb.StatePendingRefutation,
				harnessfb.StateApproved, harnessfb.StateRefuted, harnessfb.StateRejected, harnessfb.StateRetired} {
				first := true
				for _, c := range cands {
					if c.State != group {
						continue
					}
					if first {
						fmt.Fprintf(out, "[%s]\n", group.Label())
						first = false
					}
					printHarnessCandidate(out, c)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print JSON")
	return cmd
}

func printHarnessCandidate(w io.Writer, c harnessfb.Candidate) {
	fmt.Fprintf(w, "%s %s — %s (세션 %d개)\n  규칙: %s\n", c.ID, c.State.Label(), c.Pattern, len(c.Evidence), intakePrintable(c.Lesson))
	if c.Rationale != "" {
		fmt.Fprintf(w, "  근거: %s\n", intakePrintable(c.Rationale))
	}
	for _, r := range c.Refutations {
		fmt.Fprintf(w, "  %s: %s — %s\n", r.Provider, verdictLabel(r.Verdict), intakePrintable(r.Reason))
	}
	calls, secs, tokens, known := c.Cost()
	tok := "토큰 알 수 없음"
	if known {
		tok = fmt.Sprintf("토큰 %d", tokens)
	}
	fmt.Fprintf(w, "  모델 호출 %d회, %.0f초, %s (작성 %s)\n", calls, secs, tok, c.DraftedBy)
}

func newHarnessDecideCmd(action string) *cobra.Command {
	short := map[string]string{
		"approve": "Approve a candidate that survived refutation and write it into " + harnessfb.RuleFile,
		"reject":  "Reject a candidate; nothing is written",
		"retire":  "Take an approved lesson back out of " + harnessfb.RuleFile,
	}[action]
	return &cobra.Command{
		Use:   action + " HL-NNN",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := harnessRoot(cmd.Context())
			if err != nil {
				return err
			}
			var c harnessfb.Candidate
			var written []string
			switch action {
			case "approve":
				c, written, err = harnessfb.Approve(root, args[0], harnessNow())
			case "retire":
				c, written, err = harnessfb.Retire(root, args[0], harnessNow())
			default:
				c, err = harnessfb.Reject(root, args[0], harnessNow())
			}
			for _, w := range written {
				verb := "다시 썼습니다"
				if _, statErr := os.Stat(w); statErr != nil {
					verb = "지웠습니다 (승인된 규칙 없음)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s를 %s\n", w, verb)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", c.ID, c.State.Label())
			return nil
		},
	}
}

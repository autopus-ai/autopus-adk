package cli

import (
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/insajin/autopus-adk/pkg/harnessfb"
)

// effectBaseline is how far before an approval its "before" sessions
// reach.
const effectBaseline = 30 * 24 * time.Hour

func harnessEffects(root string) ([]harnessfb.Effect, error) {
	cands, err := harnessfb.LoadCandidates(root)
	if err != nil {
		return nil, err
	}
	approved := harnessfb.Approved(root, cands)
	if len(approved) == 0 {
		return nil, nil
	}
	// Read from before the earliest approval, so an old rule keeps the
	// sessions it is compared against.
	since := harnessNow()
	for _, c := range approved {
		if c.Decided != nil && c.Decided.Before(since) {
			since = *c.Decided
		}
	}
	sessions, err := harnessfb.LoadSessions(root, since.Add(-effectBaseline))
	if err != nil {
		return nil, err
	}
	return harnessfb.Effects(sessions, approved), nil
}

func printHarnessEffects(w io.Writer, effects []harnessfb.Effect) {
	if len(effects) == 0 {
		return
	}
	fmt.Fprintln(w, "\n승인한 규칙의 효과 (승인 전 → 후):")
	for _, e := range effects {
		if e.Verdict == harnessfb.EffectNoBaseline {
			fmt.Fprintf(w, "- %s (%s): %s (전 %d개, 후 %d개 세션)\n", e.ID, e.Pattern, e.Verdict, e.Before.Sessions, e.After.Sessions)
			continue
		}
		minutes := ""
		if e.Before.Seconds > 0 || e.After.Seconds > 0 {
			minutes = fmt.Sprintf(", 세션당 %.1f분 → %.1f분", e.Before.PerSession()/60, e.After.PerSession()/60)
		}
		fmt.Fprintf(w, "- %s (%s): 나온 세션 %.0f%% → %.0f%%%s — %s (전 %d개, 후 %d개 세션)\n",
			e.ID, e.Pattern, 100*e.Before.Rate(), 100*e.After.Rate(), minutes, e.Verdict, e.Before.Sessions, e.After.Sessions)
	}
}

// controlFree drops control characters, so recorded text cannot move the
// terminal's cursor or change its colors.
func controlFree(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

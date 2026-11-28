package harnessfb

import "strings"

// MinEffectSessions is how many sessions after an approval it takes before
// the approved rule's effect is judged.
const MinEffectSessions = 5

// Window is how often a pattern showed up in a set of sessions and what it
// cost them.
type Window struct {
	Sessions int     `json:"sessions"`
	Hits     int     `json:"hits"`    // sessions that showed the pattern
	Seconds  float64 `json:"seconds"` // the pattern's time across them
}

// Rate is the share of sessions that showed the pattern.
func (w Window) Rate() float64 { return share(float64(w.Hits), float64(w.Sessions)) }

// PerSession is the pattern's average time per session.
func (w Window) PerSession() float64 {
	if w.Sessions == 0 {
		return 0
	}
	return w.Seconds / float64(w.Sessions)
}

// Effect labels.
const (
	EffectMeasuring  = "측정 중"
	EffectReduced    = "줄었음"
	EffectNone       = "효과 없음 · retire 후보"
	EffectNoBaseline = "판정 불가 · 승인 전 기준 없음"
	// EffectAreaStill is a review-area lesson whose area still draws
	// findings: the area keeps being reviewed, so this alone does not show
	// the lesson failed, and it is not offered for retirement.
	EffectAreaStill = "영역 지적 계속 · 교훈의 실수가 남았는지는 사람이 판단"
)

// Effect compares an approved rule's pattern before and after approval.
type Effect struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
	Before  Window `json:"before"`
	After   Window `json:"after"`
	Verdict string `json:"verdict"`
}

// Effects measures each approved rule: the sessions that ended before its
// approval against those that started after it. Sessions running across
// the approval count in neither. A rule is judged once MinEffectSessions
// sessions ran after it: it reduced the waste when the share of sessions
// showing the pattern, or its time per session, fell by a fifth or more.
// The sessions before include the ones that made the pattern repeat, so a
// pattern that was at a peak can fall on its own: read "reduced" as a sign,
// and "no effect" as the stronger result.
func Effects(sessions []Metrics, approved []Candidate) []Effect {
	var out []Effect
	for _, c := range approved {
		if c.Decided == nil {
			continue
		}
		e := Effect{ID: c.ID, Pattern: c.Pattern}
		for _, m := range sessions {
			var w *Window
			switch {
			case m.End.Before(*c.Decided):
				w = &e.Before
			case m.Start.After(*c.Decided):
				w = &e.After
			default:
				continue
			}
			w.Sessions++
			if _, ok := hitOf(m, c.Pattern); ok {
				w.Hits++
			}
			// Time is the pattern's own measure in every session, not only
			// in those past the detection threshold, so a small change is
			// not read as the waste disappearing.
			w.Seconds += patternSeconds(m, c.Pattern)
		}
		e.Verdict = verdict(e.Before, e.After)
		if isReviewArea(c.Pattern) && e.Verdict == EffectNone {
			e.Verdict = EffectAreaStill
		}
		out = append(out, e)
	}
	return out
}

func hitOf(m Metrics, pattern string) (Hit, bool) {
	for _, h := range sessionHits(m) {
		if h.id == pattern {
			return h.Hit, true
		}
	}
	return Hit{}, false
}

// patternSeconds is the time pattern measures in session m.
func patternSeconds(m Metrics, pattern string) float64 {
	switch pattern {
	case "test-wait":
		return m.TestWaitSeconds
	case "review-heavy":
		return m.ReviewSeconds
	case "repeat-review":
		return m.RepeatNoFixSeconds
	case "timeout":
		return m.TimeoutSeconds
	}
	if kind, ok := strings.CutPrefix(pattern, "stop-block:"); ok {
		return m.StopBlockSeconds[kind]
	}
	return 0
}

// verdict judges a rule once enough sessions ran after it. Comparing in
// integers keeps an exact fifth from being lost to rounding.
func verdict(before, after Window) string {
	if before.Sessions == 0 || before.Hits == 0 {
		return EffectNoBaseline
	}
	if after.Sessions < MinEffectSessions {
		return EffectMeasuring
	}
	rateFell := 5*after.Hits*before.Sessions <= 4*before.Hits*after.Sessions
	timeFell := before.Seconds > 0 && 5*after.Seconds*float64(before.Sessions) <= 4*before.Seconds*float64(after.Sessions)
	if rateFell || timeFell {
		return EffectReduced
	}
	return EffectNone
}

package harnessfb

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func repeatSession(id string, at time.Time, wasted bool) Metrics {
	m := Metrics{SessionID: id, Start: at, End: at.Add(time.Hour), ActiveSeconds: 3600}
	if wasted {
		m.RepeatReviews, m.RepeatReviewsNoFix, m.RepeatNoFixSeconds = 1, 1, 600
	}
	return m
}

func TestEffects(t *testing.T) {
	approvedAt := t0.Add(10 * 24 * time.Hour)
	rule := Candidate{ID: "HL-001", Pattern: "repeat-review", State: StateApproved, Decided: &approvedAt}
	var sessions []Metrics
	for i := 0; i < 4; i++ {
		sessions = append(sessions, repeatSession(fmt.Sprint("b", i), t0.Add(time.Duration(i)*24*time.Hour), i < 3))
	}
	// One session runs across the approval and counts in neither window.
	sessions = append(sessions, repeatSession("x", approvedAt.Add(-30*time.Minute), true))

	after := func(n, wasted int) []Metrics {
		out := append([]Metrics(nil), sessions...)
		for i := 0; i < n; i++ {
			out = append(out, repeatSession(fmt.Sprint("a", i), approvedAt.Add(time.Duration(i+1)*time.Hour*2), i < wasted))
		}
		return out
	}

	e := Effects(after(2, 0), []Candidate{rule})[0]
	assert.Equal(t, EffectMeasuring, e.Verdict, "too few sessions after the approval")
	assert.Equal(t, Window{Sessions: 4, Hits: 3, Seconds: 1800}, e.Before)

	e = Effects(after(5, 0), []Candidate{rule})[0]
	assert.Equal(t, EffectReduced, e.Verdict)
	assert.InDelta(t, 0.75, e.Before.Rate(), 0.001)
	assert.Zero(t, e.After.Rate())

	e = Effects(after(5, 4), []Candidate{rule})[0]
	assert.Equal(t, EffectNone, e.Verdict, "80%% of sessions still show it")

	assert.Empty(t, Effects(sessions, []Candidate{{ID: "HL-002", Pattern: "x"}}), "a rule with no approval time")
}

// Time is counted from the pattern's own measure, not only in sessions
// past the detection threshold: a review time that fell 9% reads as 9%. A
// fall across the threshold still counts as reduced, through the share of
// sessions showing the pattern.
func TestEffects_TimeIsTheRawMeasure(t *testing.T) {
	approvedAt := t0.Add(10 * 24 * time.Hour)
	rule := Candidate{ID: "HL-001", Pattern: "review-heavy", State: StateApproved, Decided: &approvedAt}
	var sessions []Metrics
	mk := func(id string, at time.Time, review float64) Metrics {
		return Metrics{SessionID: id, Start: at, End: at.Add(time.Hour), ActiveSeconds: 3600, Reviews: 1, ReviewSeconds: review}
	}
	for i := 0; i < 3; i++ {
		sessions = append(sessions, mk(fmt.Sprint("b", i), t0.Add(time.Duration(i)*24*time.Hour), 1150))
	}
	for i := 0; i < 5; i++ {
		sessions = append(sessions, mk(fmt.Sprint("a", i), approvedAt.Add(time.Duration(i+1)*2*time.Hour), 1050))
	}
	e := Effects(sessions, []Candidate{rule})[0]
	assert.InDelta(t, 1150, e.Before.PerSession(), 0.01)
	assert.InDelta(t, 1050, e.After.PerSession(), 0.01)
	assert.Equal(t, EffectReduced, e.Verdict, "the share of sessions over the threshold fell from 100% to 0%")

	// With the threshold not crossed, a 9% fall is no effect.
	for i := range sessions[3:] {
		sessions[3+i].ReviewSeconds = 1100
	}
	e = Effects(sessions, []Candidate{rule})[0]
	assert.Equal(t, EffectNone, e.Verdict)
}

func TestVerdict_Boundaries(t *testing.T) {
	assert.Equal(t, EffectNoBaseline, verdict(Window{Sessions: 4}, Window{Sessions: 9}), "the pattern never showed before")
	assert.Equal(t, EffectNoBaseline, verdict(Window{}, Window{Sessions: 9, Hits: 9}))
	assert.Equal(t, EffectReduced, verdict(Window{Sessions: 10, Hits: 7}, Window{Sessions: 25, Hits: 14}), "exactly a fifth less")
	assert.Equal(t, EffectNone, verdict(Window{Sessions: 10, Hits: 7}, Window{Sessions: 25, Hits: 15}))
}

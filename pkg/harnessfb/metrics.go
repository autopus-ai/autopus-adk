// Package harnessfb measures where a coding agent's session time goes
// (test waits, review rounds, hook blocks, timeouts), finds waste that
// repeats across sessions, and carries a proposed harness change through
// refutation by another provider to a person's approval.
package harnessfb

import "time"

// Metrics is what one session spent, as read from its transcript so far.
type Metrics struct {
	SessionID string    `json:"session_id"`
	Platform  string    `json:"platform"`
	Start     time.Time `json:"start"`
	End       time.Time `json:"end"`
	// ActiveSeconds is the session's wall time without idle gaps longer
	// than idleGap (a user away, a session left open overnight).
	ActiveSeconds float64 `json:"active_seconds"`
	Turns         int     `json:"turns"`

	TestRuns        int     `json:"test_runs"`
	TestWaitSeconds float64 `json:"test_wait_seconds"`

	Reviews       int     `json:"reviews"`
	ReviewSeconds float64 `json:"review_seconds"`
	// RepeatReviews counts reviews of a change that had already had one (a
	// change ends at a user prompt or a git commit); RepeatReviewsNoFix
	// those after which nothing was edited before the change ended, so the
	// second pass changed nothing.
	RepeatReviews       int     `json:"repeat_reviews"`
	RepeatReviewsNoFix  int     `json:"repeat_reviews_no_fix"`
	RepeatReviewSeconds float64 `json:"repeat_review_seconds"`
	// RepeatNoFixSeconds is the time of the repeat reviews that fixed nothing.
	RepeatNoFixSeconds float64 `json:"repeat_no_fix_seconds"`

	StopBlocks     int            `json:"stop_blocks"`
	StopBlockKinds map[string]int `json:"stop_block_kinds,omitempty"`
	// StopBlockSeconds is, per kind, the time from each block to the next
	// Stop hook event: the work the block sent the agent back to do.
	StopBlockSeconds map[string]float64 `json:"stop_block_seconds,omitempty"`
	// StopBlockSample is the first block reason of each kind, cut short.
	StopBlockSample map[string]string `json:"stop_block_sample,omitempty"`
	Timeouts        int               `json:"timeouts"`
	// TimeoutSeconds is how long the commands that timed out had run.
	TimeoutSeconds float64 `json:"timeout_seconds"`
}

// share is part/whole clamped to [0, 1]: spans can overlap (a background
// test while a review runs), so their sum can exceed the wall time.
func share(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	if s := part / whole; s < 1 {
		return s
	}
	return 1
}

// TestWaitShare is the part of active time spent waiting on tests.
func (m Metrics) TestWaitShare() float64 { return share(m.TestWaitSeconds, m.ActiveSeconds) }

// ReviewShare is the part of active time spent in reviews.
func (m Metrics) ReviewShare() float64 { return share(m.ReviewSeconds, m.ActiveSeconds) }

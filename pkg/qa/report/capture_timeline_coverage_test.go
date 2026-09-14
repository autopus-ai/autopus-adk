package report

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/capture"
)

// Timestamps are authoritative: a producer that reported both must be laid out
// against the journey window, not against the duration fallback.
func TestApplyCaptureTimelinePrefersTimestamps(t *testing.T) {
	t.Parallel()
	views := []CaptureStepView{{DurationMS: 1000}, {DurationMS: 1000}}
	index := capture.Index{
		StartedAt: "2026-01-02T03:00:00Z",
		EndedAt:   "2026-01-02T03:00:10Z",
		Steps: []capture.Step{
			{StartedAt: "2026-01-02T03:00:00Z", EndedAt: "2026-01-02T03:00:01Z"},
			{StartedAt: "2026-01-02T03:00:05Z", EndedAt: "2026-01-02T03:00:10Z"},
		},
	}
	applyCaptureTimeline(views, index)
	assert.InDelta(t, 0.0, views[0].Bar.OffsetPercent, 0.001)
	assert.InDelta(t, 10.0, views[0].Bar.WidthPercent, 0.001)
	// Equal durations must not produce equal bars when the timestamps differ.
	assert.InDelta(t, 50.0, views[1].Bar.OffsetPercent, 0.001)
	assert.InDelta(t, 50.0, views[1].Bar.WidthPercent, 0.001)
}

// A producer that timestamped steps but not the index still gets a filmstrip:
// the span widens to the extremes of the steps themselves.
func TestApplyCaptureTimelineDerivesSpanFromSteps(t *testing.T) {
	t.Parallel()
	views := []CaptureStepView{{DurationMS: 1}, {DurationMS: 1}}
	applyCaptureTimeline(views, capture.Index{Steps: []capture.Step{
		{StartedAt: "2026-01-02T03:00:00Z", EndedAt: "2026-01-02T03:00:02Z"},
		{StartedAt: "2026-01-02T03:00:02Z", EndedAt: "2026-01-02T03:00:04Z"},
	}})
	assert.InDelta(t, 0.0, views[0].Bar.OffsetPercent, 0.001)
	assert.InDelta(t, 50.0, views[1].Bar.OffsetPercent, 0.001)
}

// Unparseable timestamps must fall through to the duration layout rather than
// leaving every bar at zero width, which renders an empty strip.
func TestApplyCaptureTimelineFallsBackToDurations(t *testing.T) {
	t.Parallel()
	views := []CaptureStepView{{DurationMS: 300}, {DurationMS: 100}}
	applyCaptureTimeline(views, capture.Index{Steps: []capture.Step{
		{StartedAt: "not-a-time", EndedAt: "not-a-time"},
		{StartedAt: "not-a-time", EndedAt: "not-a-time"},
	}})
	require.InDelta(t, 0.0, views[0].Bar.OffsetPercent, 0.001)
	assert.InDelta(t, 75.0, views[0].Bar.WidthPercent, 0.001)
	// Back-to-back: the second bar starts where the first ended.
	assert.InDelta(t, 75.0, views[1].Bar.OffsetPercent, 0.001)
	assert.InDelta(t, 25.0, views[1].Bar.WidthPercent, 0.001)
}

// A sub-percent step must stay clickable instead of collapsing to an invisible
// sliver, so the width floors at the minimum bar.
func TestPositionByDurationFloorsTinySteps(t *testing.T) {
	t.Parallel()
	views := []CaptureStepView{{DurationMS: 1}, {DurationMS: 9999}}
	positionByDuration(views)
	assert.InDelta(t, minBarPercent, views[0].Bar.WidthPercent, 0.001)
}

// Zero total duration carries no ordering information, so no bar is invented.
func TestPositionByDurationLeavesZeroTotalUnpositioned(t *testing.T) {
	t.Parallel()
	views := []CaptureStepView{{DurationMS: 0}, {DurationMS: 0}}
	positionByDuration(views)
	assert.Equal(t, TimelineBar{}, views[0].Bar)
	assert.Equal(t, TimelineBar{}, views[1].Bar)
}

// A span that does not advance (or cannot be parsed at all) is not a timeline;
// reporting it as one would divide by zero-width.
func TestCaptureSpanRejectsNonAdvancingWindows(t *testing.T) {
	t.Parallel()
	_, _, ok := captureSpan(capture.Index{
		StartedAt: "2026-01-02T03:00:05Z",
		EndedAt:   "2026-01-02T03:00:05Z",
	})
	assert.False(t, ok, "an instantaneous window is not a span")

	_, _, ok = captureSpan(capture.Index{})
	assert.False(t, ok, "no timestamps at all is not a span")
}

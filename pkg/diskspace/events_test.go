package diskspace

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SPEC-DISK-002 S7: the log rotates before passing 1 MiB, and the tail reads
// across both files, skipping a line that does not parse.
func TestEvents_RotateAndTail(t *testing.T) {
	t.Parallel()
	state := testState(t)
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	require.NoError(t, AppendEvent(state, Event{Time: t0, Kind: EventTrimApply, Count: 1}))
	f, err := state.OpenAppend(eventsFile)
	require.NoError(t, err)
	_, err = f.WriteString("not json\n")
	require.NoError(t, err)
	info, err := f.Stat()
	require.NoError(t, err)
	_, err = f.Write(bytes.Repeat([]byte("x"), eventsMaxBytes-10-int(info.Size())))
	require.NoError(t, err)
	_, err = f.WriteString("\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.NoError(t, AppendEvent(state, Event{Time: t0.Add(time.Minute), Kind: EventReclaimApply, Count: 2}))
	rotated, err := os.Stat(state.Join(eventsRotated))
	require.NoError(t, err)
	assert.Greater(t, rotated.Size(), int64(eventsMaxBytes-20))
	require.NoError(t, AppendEvent(state, Event{Time: t0.Add(2 * time.Minute), Kind: EventLeaseRefused, Count: 3}))

	events, err := TailEvents(state, 3)
	require.NoError(t, err)
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	assert.Equal(t, []string{EventTrimApply, EventReclaimApply, EventLeaseRefused}, kinds)

	events, err = TailEvents(state, 1)
	require.NoError(t, err)
	assert.Equal(t, EventLeaseRefused, events[0].Kind)

	last, ok := LastEvent(state, EventReclaimApply)
	require.True(t, ok)
	assert.Equal(t, 2, last.Count)
	_, ok = LastEvent(state, EventLowSpaceAlert)
	assert.False(t, ok)
}

// SPEC-DISK-002 S7: one sample per five minutes per mount, retention on
// rewrite, and the trend window.
func TestSamples_IntervalRetentionTrend(t *testing.T) {
	t.Parallel()
	state := testState(t)
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	require.NoError(t, RecordSample(state, Sample{Time: t0, FreeBytes: 60 * GiB, Mount: "/"}))
	require.NoError(t, RecordSample(state, Sample{Time: t0.Add(time.Minute), FreeBytes: 59 * GiB, Mount: "/"}))
	require.NoError(t, RecordSample(state, Sample{Time: t0.Add(time.Minute), FreeBytes: 9 * GiB, Mount: "/data"}))
	assert.Len(t, LoadSamples(state), 2, "the same mount within five minutes is skipped")

	trend := ComputeTrend(LoadSamples(state), "/", t0.Add(2*time.Hour), 40*GiB)
	require.NotNil(t, trend)
	assert.Equal(t, int64(120), trend.WindowMinutes)
	assert.Equal(t, int64(-10*GiB), trend.BytesPerHour)
	assert.Nil(t, ComputeTrend(LoadSamples(state), "/", t0.Add(5*time.Minute), 40*GiB), "younger than ten minutes")
	assert.Nil(t, ComputeTrend(LoadSamples(state), "/", t0.Add(7*time.Hour), 40*GiB), "older than six hours")
	assert.Nil(t, ComputeTrend(LoadSamples(state), "/other", t0.Add(time.Hour), 40*GiB))

	var old bytes.Buffer
	line, _ := json.Marshal(Sample{Time: t0.Add(-8 * 24 * time.Hour), FreeBytes: 1, Mount: "/"})
	for old.Len() < samplesMaxBytes {
		old.Write(append(line, '\n'))
	}
	require.NoError(t, state.WriteFileAtomic(samplesFile, old.Bytes()))
	require.NoError(t, RecordSample(state, Sample{Time: t0, FreeBytes: 5, Mount: "/"}))
	kept := LoadSamples(state)
	require.Len(t, kept, 1)
	assert.Equal(t, int64(5), kept[0].FreeBytes)
	data, err := state.ReadFile(samplesFile)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), "\n"))
}

// SPEC-DISK-002 review F-07: concurrent alerts inside one window log once.
func TestEvents_UnlessRecentIsSerialized(t *testing.T) {
	t.Parallel()
	state := testState(t)
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup
	var appended atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, err := AppendEventUnlessRecent(state, Event{Time: now, Kind: EventLowSpaceAlert}, 30*time.Minute)
			assert.NoError(t, err)
			if ok {
				appended.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), appended.Load())
	ok, err := AppendEventUnlessRecent(state, Event{Time: now.Add(31 * time.Minute), Kind: EventLowSpaceAlert}, 30*time.Minute)
	require.NoError(t, err)
	assert.True(t, ok)
}

// SPEC-DISK-002 review F-20: alternating mounts keep each mount's interval.
func TestSamples_IntervalIsPerMount(t *testing.T) {
	t.Parallel()
	state := testState(t)
	t0 := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	for i, mount := range []string{"/a", "/b", "/a", "/b"} {
		require.NoError(t, RecordSample(state, Sample{Time: t0.Add(time.Duration(i) * time.Minute), FreeBytes: 1, Mount: mount}))
	}
	assert.Len(t, LoadSamples(state), 2)
}

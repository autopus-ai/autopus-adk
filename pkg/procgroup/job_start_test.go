package procgroup

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeJobStart records the calls jobStart.run makes.
type fakeJobStart struct {
	startErr, joinErr, resumeErr error
	joined, resumed              bool
	abortedJob                   int
	aborted                      bool
}

func (f *fakeJobStart) seq() jobStart[int] {
	return jobStart[int]{
		start: func() error { return f.startErr },
		join: func() (int, error) {
			f.joined = true
			if f.joinErr != nil {
				return 0, f.joinErr
			}
			return 7, nil
		},
		resume: func() error { f.resumed = true; return f.resumeErr },
		abort: func(job int, cause error) error {
			f.aborted, f.abortedJob = true, job
			return &abortedStart{err: cause}
		},
	}
}

func TestJobStart_JoinFailureRunsWithoutJob(t *testing.T) {
	for _, joinErr := range []error{
		errors.New("create job object: Access is denied."),
		errors.New("assign process to job object: Access is denied."),
	} {
		f := &fakeJobStart{joinErr: joinErr}
		job, err := f.seq().run()
		// The job is a cleanup aid: losing it must not fail the step.
		require.NoError(t, err)
		assert.Zero(t, job, "no job to terminate: Kill falls back to the process")
		assert.True(t, f.resumed, "the process must still be resumed, or it never runs")
		assert.False(t, f.aborted)
	}
}

func TestJobStart_JoinedRunsInJob(t *testing.T) {
	f := &fakeJobStart{}
	job, err := f.seq().run()
	require.NoError(t, err)
	assert.Equal(t, 7, job)
	assert.True(t, f.resumed)
}

func TestJobStart_StartFailureIsReturned(t *testing.T) {
	f := &fakeJobStart{startErr: errors.New("exec: not found")}
	_, err := f.seq().run()
	assert.ErrorIs(t, err, f.startErr)
	assert.False(t, f.joined, "nothing to join when the process never started")
	assert.False(t, f.resumed)
}

func TestJobStart_ResumeFailureAbortsThroughTheJob(t *testing.T) {
	for _, tc := range []struct {
		joinErr error
		wantJob int
	}{{nil, 7}, {errors.New("assign failed"), 0}} {
		f := &fakeJobStart{joinErr: tc.joinErr, resumeErr: errors.New("resume thread: denied")}
		_, err := f.seq().run()
		var aborted *abortedStart
		require.ErrorAs(t, err, &aborted)
		assert.ErrorIs(t, err, f.resumeErr)
		assert.True(t, f.aborted)
		assert.Equal(t, tc.wantJob, f.abortedJob, "abort uses the job only when the process joined one")
	}
}

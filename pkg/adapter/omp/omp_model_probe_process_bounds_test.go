package omp

// Run-bound tests for the model probe process: the output ceiling and the
// caller's deadline each end a run that would otherwise keep going.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/insajin/autopus-adk/pkg/processprobe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOMPModelProbeProcess_RunEnforcesOutputLimit(t *testing.T) {
	t.Parallel()

	executable := writeOMPModelProbeExecutable(t, "#!/bin/sh\nwhile :; do printf '0123456789'; done\n")
	process, err := NewOMPModelProbeProcess(executable, 64)
	require.NoError(t, err)

	// The ceiling only bounds a broken limiter. The contract is that the overflow,
	// not the deadline, ends the run — so ctx must still be live on return. An
	// absolute wall-clock bound here instead measures machine load.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := process.Run(ctx, "models", "--json")
	assert.ErrorIs(t, err, processprobe.ErrOutputLimit)
	assert.NoError(t, ctx.Err(), "overflow must terminate the run before the deadline")
	assert.LessOrEqual(t, len(output), 64)
}

func TestOMPModelProbeProcess_RunHonorsContextTimeout(t *testing.T) {
	root := t.TempDir()
	startedFile := filepath.Join(root, "started")
	executable := writeOMPModelProbeExecutable(t, "#!/bin/sh\nprintf 'started\\n'\nprintf ready > '"+
		strings.ReplaceAll(startedFile, "'", "'\"'\"'")+"'\nsleep 10\n")
	process, err := NewOMPModelProbeProcess(executable, 1024)
	require.NoError(t, err)

	ctx := newOMPModelProbeDeadlineContext()
	defer func() {
		if ctx.Err() == nil {
			ctx.expire()
		}
	}()
	type result struct {
		output []byte
		err    error
	}
	done := make(chan result, 1)
	go func() {
		output, runErr := process.Run(ctx, "--version")
		done <- result{output: output, err: runErr}
	}()
	require.Eventually(t, func() bool {
		_, statErr := os.Stat(startedFile)
		return statErr == nil
	}, 10*time.Second, 10*time.Millisecond)

	started := time.Now()
	ctx.expire()
	run := <-done
	err = run.err
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded))
	assert.Less(t, time.Since(started), 5*time.Second)
	assert.Equal(t, []byte("started\n"), run.output)
}

type ompModelProbeDeadlineContext struct {
	context.Context
	done chan struct{}
}

func newOMPModelProbeDeadlineContext() *ompModelProbeDeadlineContext {
	return &ompModelProbeDeadlineContext{Context: context.Background(), done: make(chan struct{})}
}

func (ctx *ompModelProbeDeadlineContext) Done() <-chan struct{} {
	return ctx.done
}

func (ctx *ompModelProbeDeadlineContext) Err() error {
	select {
	case <-ctx.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func (ctx *ompModelProbeDeadlineContext) expire() {
	close(ctx.done)
}

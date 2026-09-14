//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package omp

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/processprobe"
)

// Guards against a stderr flood being absorbed silently: the write must report
// the limit error and wake the shared signal so supervision can terminate.
func TestOMPReadinessCountWriter_ReportsAndSignalsOverflow(t *testing.T) {
	signal := newOMPReadinessLimitSignal()
	writer := &ompReadinessCountWriter{limit: 8, signal: signal}

	written, err := writer.Write([]byte("12345678"))
	require.NoError(t, err)
	assert.Equal(t, 8, written)
	assert.False(t, writer.Exceeded())
	select {
	case <-signal.Done():
		t.Fatal("a write inside the budget must not signal the limit")
	default:
	}

	written, err = writer.Write([]byte("9"))
	assert.Equal(t, 1, written, "the writer must consume the data it rejects")
	assert.True(t, errors.Is(err, processprobe.ErrOutputLimit))
	assert.True(t, writer.Exceeded())
	<-signal.Done()

	// Notify is idempotent: a second overflow must not panic on a closed channel.
	_, err = writer.Write([]byte("0"))
	assert.True(t, errors.Is(err, processprobe.ErrOutputLimit))
}

// Guards against the capture growing past its budget and against losing the
// prefix that still fits, which diagnostics need to explain the truncation.
func TestOMPReadinessRPCStreamCapture_TruncatesAtBudgetAndSignals(t *testing.T) {
	signal := newOMPReadinessLimitSignal()
	capture := newOMPReadinessRPCStreamCapture(6, signal)

	_, err := capture.Write([]byte("abc"))
	require.NoError(t, err)
	_, err = capture.Write([]byte("defgh"))
	assert.True(t, errors.Is(err, processprobe.ErrOutputLimit))
	assert.True(t, capture.Exceeded())
	assert.Equal(t, "abcdef", string(capture.Bytes()), "the in-budget prefix must survive")
	<-signal.Done()
}

// Guards against the terminal detector firing on an unpaired tool call or a
// non-assistant message: only a completed assistant turn ends the probe.
func TestOMPReadinessRPCStreamCapture_TerminalRequiresPairedAssistantTurn(t *testing.T) {
	terminated := func(t *testing.T, frames ...string) bool {
		t.Helper()
		capture := newOMPReadinessRPCStreamCapture(4096, newOMPReadinessLimitSignal())
		for _, frame := range frames {
			_, err := capture.Write([]byte(frame + "\n"))
			require.NoError(t, err)
		}
		select {
		case <-capture.Terminal():
			return true
		default:
			return false
		}
	}

	assert.False(t, terminated(t,
		`{"type":"message_end","message":{"role":"assistant"}}`),
		"an assistant turn without a paired tool call is not terminal")
	assert.False(t, terminated(t,
		`{"type":"tool_execution_start","toolCallId":""}`,
		`{"type":"tool_execution_end","toolCallId":""}`,
		`{"type":"message_end","message":{"role":"assistant"}}`),
		"an empty tool call id must not count as a pairing")
	assert.False(t, terminated(t,
		`{"type":"tool_execution_start","toolCallId":"a"}`,
		`{"type":"tool_execution_end","toolCallId":"b"}`,
		`{"type":"message_end","message":{"role":"assistant"}}`),
		"a mismatched tool call id must not count as a pairing")
	assert.False(t, terminated(t,
		`{"type":"tool_execution_start","toolCallId":"a"}`,
		`{"type":"tool_execution_end","toolCallId":"a"}`,
		`{"type":"message_end","message":{"role":"user"}}`),
		"only an assistant message ends the turn")
	assert.True(t, terminated(t,
		`{"type":"tool_execution_start","toolCallId":"a"}`,
		`{"type":"tool_execution_end","toolCallId":"a"}`,
		`{"type":"message_end","message":{"role":"assistant"}}`))
}

// Guards against hanging when the provider refuses the turn instead of
// producing one: agent_end and a refused prompt response are both terminal.
func TestOMPReadinessRPCStreamCapture_TerminalOnRefusalAndAgentEnd(t *testing.T) {
	for _, test := range []struct {
		name, frame string
		want        bool
	}{
		{name: "agent end", frame: `{"type":"agent_end"}`, want: true},
		{name: "failed prompt", frame: `{"type":"response","command":"prompt","success":false}`, want: true},
		{
			name:  "prompt without agent invocation",
			frame: `{"type":"response","command":"prompt","success":true,"data":{"agentInvoked":false}}`,
			want:  true,
		},
		{
			name:  "prompt with agent invocation",
			frame: `{"type":"response","command":"prompt","success":true,"data":{"agentInvoked":true}}`,
		},
		{name: "unrelated response", frame: `{"type":"response","command":"get_state","success":true}`},
		{name: "malformed line", frame: `not json at all`},
		{name: "unknown frame", frame: `{"type":"turn_start"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := newOMPReadinessRPCStreamCapture(4096, newOMPReadinessLimitSignal())
			_, err := capture.Write([]byte(test.frame + "\n"))
			require.NoError(t, err)
			select {
			case <-capture.Terminal():
				assert.True(t, test.want, "frame must not be terminal")
			default:
				assert.False(t, test.want, "frame must be terminal")
			}
		})
	}
}

// Guards against a frame split across pipe reads being dropped or rescanned:
// scanning must resume from the buffered partial line exactly once.
func TestOMPReadinessRPCStreamCapture_ScansFramesSplitAcrossWrites(t *testing.T) {
	capture := newOMPReadinessRPCStreamCapture(4096, newOMPReadinessLimitSignal())
	frame := `{"type":"agent_end"}` + "\n"

	_, err := capture.Write([]byte(frame[:8]))
	require.NoError(t, err)
	select {
	case <-capture.Terminal():
		t.Fatal("a partial line must not be interpreted")
	default:
	}

	_, err = capture.Write([]byte(frame[8:]))
	require.NoError(t, err)
	<-capture.Terminal()
	assert.Equal(t, frame, string(capture.Bytes()))
}

// Guards against silently unbounded capture or a double-owned output stream:
// both misconfigurations must be refused before any process starts.
func TestRunOMPReadinessRPCCommand_RefusesInvalidSupervision(t *testing.T) {
	skipWithoutPOSIXShellOMP(t)

	cmd := exec.Command("/bin/sh", "-c", "true")
	_, err := runOMPReadinessRPCCommand(context.Background(), cmd, nil, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output limit")
	assert.Nil(t, cmd.Process, "an invalid limit must not start the process")

	preconfigured := exec.Command("/bin/sh", "-c", "true")
	preconfigured.Stdout = &bytes.Buffer{}
	_, err = runOMPReadinessRPCCommand(context.Background(), preconfigured, nil, 1024)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already configured")
	assert.Nil(t, preconfigured.Process)
}

// Guards against a flooding provider being trusted: supervision must stop the
// process and join the output-limit error onto the partial transcript.
func TestRunOMPReadinessRPCCommand_TerminatesOnOutputFlood(t *testing.T) {
	skipWithoutPOSIXShellOMP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
		`cat >/dev/null; while :; do printf '0123456789abcdef0123456789abcdef'; done`)
	output, err := runOMPReadinessRPCCommand(ctx, cmd, ompProviderFreeRPCInput(), 64)

	require.Error(t, err)
	assert.True(t, errors.Is(err, processprobe.ErrOutputLimit))
	assert.LessOrEqual(t, len(output), 64, "the transcript must stay inside its budget")
	assert.NoError(t, ctx.Err(), "the flood must be stopped by the limit, not the deadline")
}

// Guards against a hung provider outliving its probe: cancellation must kill
// the process group and surface the context error to the caller.
func TestRunOMPReadinessRPCCommand_TerminatesOnContextDeadline(t *testing.T) {
	skipWithoutPOSIXShellOMP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `cat >/dev/null; sleep 120`)
	start := time.Now()
	_, err := runOMPReadinessRPCCommand(ctx, cmd, ompProviderFreeRPCInput(), 4096)

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assert.Less(t, time.Since(start), 30*time.Second, "the probe must not wait for the hung child")
}

// Guards against a nonzero provider exit being reported as success while the
// partial transcript is still returned for diagnostics.
func TestRunOMPReadinessRPCCommand_ReturnsPartialTranscriptOnExitFailure(t *testing.T) {
	skipWithoutPOSIXShellOMP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
		`cat >/dev/null; printf '{"type":"ready"}\n'; exit 7`)
	output, err := runOMPReadinessRPCCommand(ctx, cmd, ompProviderFreeRPCInput(), 4096)

	require.Error(t, err)
	assert.False(t, errors.Is(err, processprobe.ErrOutputLimit))
	assert.Equal(t, "{\"type\":\"ready\"}\n", string(output))
}

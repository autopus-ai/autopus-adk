package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeResponses splits the writer output into one response per line.
func decodeResponses(t *testing.T, buf *bytes.Buffer) []jsonRPCResponse {
	t.Helper()
	var out []jsonRPCResponse
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var resp jsonRPCResponse
		require.NoError(t, json.Unmarshal([]byte(line), &resp), line)
		out = append(out, resp)
	}
	return out
}

// Start must answer every framed request in order, reply -32700 to malformed
// JSON without aborting the loop, and stay silent on blank lines and
// notifications. Guards against a parse error killing the session.
func TestStart_DispatchesLinesAndSurvivesParseErrors(t *testing.T) {
	t.Parallel()

	s, buf := newTestServer("http://127.0.0.1:1")
	input := strings.Join([]string{
		``,
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`,
		`{not json`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"some/unknown/notification"}`,
		`{"jsonrpc":"2.0","id":2,"method":"nope"}`,
	}, "\n") + "\n"
	s.SetIO(strings.NewReader(input), nil)

	require.NoError(t, s.Start(context.TODO()))

	responses := decodeResponses(t, buf)
	require.Len(t, responses, 3)

	assert.Equal(t, float64(1), responses[0].ID)
	assert.Nil(t, responses[0].Error)

	require.NotNil(t, responses[1].Error)
	assert.Equal(t, -32700, responses[1].Error.Code)
	assert.Nil(t, responses[1].ID)

	require.NotNil(t, responses[2].Error)
	assert.Equal(t, -32601, responses[2].Error.Code)
	assert.Contains(t, responses[2].Error.Message, "nope")
	assert.Equal(t, float64(2), responses[2].ID)
}

// Start must stop with the context error instead of draining buffered input
// once the caller cancels. Guards against a cancelled worker shutdown still
// executing queued tool calls.
func TestStart_CancelledContextStopsDispatch(t *testing.T) {
	t.Parallel()

	s, buf := newTestServer("http://127.0.0.1:1")
	s.SetIO(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`+"\n"), nil)

	ctx, cancel := context.WithCancel(context.TODO())
	cancel()

	err := s.Start(ctx)

	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, buf.String())
}

// SetIO must ignore nil streams rather than clearing previously configured
// ones. Guards against a nil writer panicking the response path.
func TestSetIO_NilArgumentsKeepExistingStreams(t *testing.T) {
	t.Parallel()

	s, buf := newTestServer("http://127.0.0.1:1")
	reader := strings.NewReader(`{"jsonrpc":"2.0","id":7,"method":"ping"}` + "\n")
	s.SetIO(reader, nil)

	replacement := &bytes.Buffer{}
	s.SetIO(nil, replacement)
	s.SetIO(nil, nil)

	require.NoError(t, s.Start(context.TODO()))

	assert.Empty(t, buf.String())
	responses := decodeResponses(t, replacement)
	require.Len(t, responses, 1)
	assert.Equal(t, float64(7), responses[0].ID)
}

// StartSSE must propagate the listener failure to its caller instead of
// blocking forever on an unusable address.
func TestStartSSE_InvalidAddrReturnsError(t *testing.T) {
	t.Parallel()

	s, _ := newTestServer("http://127.0.0.1:1")
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	err := s.StartSSE(ctx, "127.0.0.1:not-a-port")

	require.Error(t, err)
}

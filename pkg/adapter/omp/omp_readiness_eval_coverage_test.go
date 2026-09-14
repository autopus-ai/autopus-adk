package omp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Guards against a readiness verdict that reports support while hiding which
// evidence was missing: every unsupported outcome must carry a distinct reason.
func TestEvaluateOMPVersion_DistinguishesProbeFailureFromInvalidOutput(t *testing.T) {
	for _, test := range []struct {
		name            string
		probe           ompProbeResult
		wantSupported   bool
		wantReason      string
		wantVersion     string
		wantNotObserved bool
	}{
		{
			name:       "probe reason wins over output",
			probe:      ompProbeResult{output: []byte("omp/18.0.5\n"), reason: "timeout"},
			wantReason: "timeout",
		},
		{
			name:       "foreign identity is invalid output",
			probe:      ompProbeResult{output: []byte("codex-cli 0.1.0\n")},
			wantReason: "output_invalid",
		},
		{
			name:       "empty output is invalid",
			probe:      ompProbeResult{output: nil},
			wantReason: "output_invalid",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capability, version := evaluateOMPVersion(test.probe)
			assert.False(t, capability.Supported)
			assert.Equal(t, test.wantReason, capability.Reason)
			assert.Empty(t, version, "an unverified identity must not report a version")
			assert.Equal(t, "identity.version", capability.ID)
		})
	}
}

// Guards against a help-probe verdict that accepts a binary missing one of the
// required launch flags, and against losing the oversized-output distinction.
func TestEvaluateOMPHelpCapability_RequiresEveryNeedle(t *testing.T) {
	help := ompProbeResult{output: []byte("--mode rpc --no-session\n")}

	present := evaluateOMPHelpCapability("launch.rpc", help, "--mode", "rpc")
	assert.True(t, present.Supported)
	assert.Equal(t, "flag_present", present.Reason)

	missing := evaluateOMPHelpCapability("launch.cwd", help, "--mode", "--cwd")
	assert.False(t, missing.Supported, "one missing needle must fail the whole capability")
	assert.Equal(t, "flag_missing", missing.Reason)

	oversized := evaluateOMPHelpCapability("launch.cwd", ompProbeResult{reason: "output_oversized"}, "--cwd")
	assert.False(t, oversized.Supported)
	assert.Equal(t, "output_invalid", oversized.Reason,
		"an oversized help transcript is unusable evidence, not a missing flag")

	timeout := evaluateOMPHelpCapability("launch.cwd", ompProbeResult{reason: "timeout"}, "--cwd")
	assert.Equal(t, "timeout", timeout.Reason)
}

// Guards against reporting intent tracing as enabled from partial or
// mis-shaped `config get` evidence instead of failing closed.
func TestEvaluateOMPIntentTracing_FailsClosedOnAmbiguousEvidence(t *testing.T) {
	for _, test := range []struct {
		name          string
		probe         ompProbeResult
		wantSupported bool
		wantReason    string
	}{
		{
			name:          "bare boolean true",
			probe:         ompProbeResult{output: []byte("true\n")},
			wantSupported: true,
			wantReason:    "effective_intent_tracing_enabled",
		},
		{
			name:       "bare boolean false",
			probe:      ompProbeResult{output: []byte("false")},
			wantReason: "effective_intent_tracing_disabled",
		},
		{
			name: "descriptor wrapper enabled",
			probe: ompProbeResult{output: []byte(
				`{"key":"tools.intentTracing","value":true,"type":"boolean","description":"trace"}`)},
			wantSupported: true,
			wantReason:    "effective_intent_tracing_enabled",
		},
		{
			name: "descriptor wrapper disabled",
			probe: ompProbeResult{output: []byte(
				`{"key":"tools.intentTracing","value":false,"type":"boolean"}`)},
			wantReason: "effective_intent_tracing_disabled",
		},
		{
			name: "descriptor for another key",
			probe: ompProbeResult{output: []byte(
				`{"key":"tools.other","value":true,"type":"boolean"}`)},
			wantReason: "output_invalid",
		},
		{
			name: "descriptor without a value",
			probe: ompProbeResult{output: []byte(
				`{"key":"tools.intentTracing","type":"boolean"}`)},
			wantReason: "output_invalid",
		},
		{
			name: "descriptor of the wrong type",
			probe: ompProbeResult{output: []byte(
				`{"key":"tools.intentTracing","value":true,"type":"string"}`)},
			wantReason: "output_invalid",
		},
		{
			name:       "trailing garbage after a valid boolean",
			probe:      ompProbeResult{output: []byte("true\nfalse\n")},
			wantReason: "output_invalid",
		},
		{
			name:       "oversized transcript",
			probe:      ompProbeResult{reason: "output_oversized"},
			wantReason: "output_invalid",
		},
		{
			name:       "probe failure is preserved",
			probe:      ompProbeResult{reason: "exit_nonzero"},
			wantReason: "exit_nonzero",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capability := evaluateOMPIntentTracing(test.probe)
			assert.Equal(t, "config.intent_tracing", capability.ID)
			assert.Equal(t, test.wantSupported, capability.Supported)
			assert.Equal(t, test.wantReason, capability.Reason)
		})
	}
}

// Guards against accepting a pre-intent tool dump that omits a required tool
// or already carries intent-tracing parameters.
func TestParseOMPPreIntentTools_RejectsIntentShapedAndIncompleteDumps(t *testing.T) {
	tool := func(name, params string) string {
		return fmt.Sprintf(`{"name":%q,"parameters":%s}`, name, params)
	}
	object := `{"type":"object"}`
	batchTask := `{"properties":{"context":{},"tasks":{}}}`
	flatTask := `{"properties":{"task":{}}}`
	intentTask := `{"properties":{"task":{},"i":{}}}`

	for _, test := range []struct {
		name  string
		tools []string
		want  bool
	}{
		{name: "batch task shape", tools: []string{tool("task", batchTask), tool("hub", object), tool("todo", object)}, want: true},
		{name: "flat task shape", tools: []string{tool("task", flatTask), tool("hub", object), tool("todo", object)}, want: true},
		{name: "intent parameter present", tools: []string{tool("task", intentTask), tool("hub", object), tool("todo", object)}},
		{name: "task without a usable shape", tools: []string{tool("task", `{"properties":{"context":{}}}`), tool("hub", object), tool("todo", object)}},
		{name: "task without properties", tools: []string{tool("task", object), tool("hub", object), tool("todo", object)}},
		{name: "todo missing", tools: []string{tool("task", batchTask), tool("hub", object)}},
		{name: "unnamed tool", tools: []string{tool("", object)}},
		{name: "tool without parameters", tools: []string{`{"name":"hub"}`}},
		{name: "no tools at all"},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := fmt.Sprintf(`{"sessionId":"s","isStreaming":false,"isCompacting":false,`+
				`"messageCount":0,"queuedMessageCount":0,"dumpTools":[%s]}`, strings.Join(test.tools, ","))
			idle, dump := parseOMPReadinessState([]byte(state))
			assert.True(t, idle, "the session is idle regardless of the tool dump verdict")
			assert.Equal(t, test.want, dump)
		})
	}
}

// Guards against accepting a command catalog whose entries lack a name or an
// ownership source, which would let an unattributed command pass readiness.
func TestParseOMPReadinessCommands_RequiresNamedAndSourcedEntries(t *testing.T) {
	assert.True(t, parseOMPReadinessCommands([]byte(`{"commands":[{"name":"auto","source":"project"}]}`)))
	assert.False(t, parseOMPReadinessCommands([]byte(`{"commands":[]}`)),
		"an empty catalog is not evidence of command support")
	assert.False(t, parseOMPReadinessCommands([]byte(`{"commands":[{"name":" ","source":"project"}]}`)))
	assert.False(t, parseOMPReadinessCommands([]byte(`{"commands":[{"name":"auto","source":""}]}`)))
	assert.False(t, parseOMPReadinessCommands([]byte(`not json`)))
}

// Guards against negotiating readiness against a binary that never advertises
// protocol 2 in its ready frame.
func TestParseOMPProviderFreeRPC_RequiresAdvertisedProtocolTwo(t *testing.T) {
	fixture := string(providerFreeRPCFixture(t))
	withoutTwo := strings.Replace(fixture, `"supportedProtocolVersions":[1,2]`,
		`"supportedProtocolVersions":[1,3]`, 1)
	require.NotEqual(t, fixture, withoutTwo)

	_, ok := parseOMPProviderFreeRPC([]byte(withoutTwo))
	assert.False(t, ok, "a ready frame without protocol 2 must invalidate the transcript")

	capabilities := evaluateOMPProviderFreeRPC(ompProbeResult{output: []byte(withoutTwo)})
	for _, capability := range capabilities {
		assert.Equal(t, "output_invalid", capability.Reason, capability.ID)
	}
}

// Guards against a partial RPC probe silently degrading to "response_missing"
// per capability when the whole transcript failed to arrive.
func TestEvaluateOMPProviderFreeRPC_MapsProbeFailureToEveryCapability(t *testing.T) {
	for reason, want := range map[string]string{
		"timeout":          "timeout",
		"exit_nonzero":     "exit_nonzero",
		"output_oversized": "output_invalid",
	} {
		capabilities := evaluateOMPProviderFreeRPC(ompProbeResult{reason: reason})
		require.Len(t, capabilities, 5)
		for _, capability := range capabilities {
			assert.False(t, capability.Supported)
			assert.Equal(t, want, capability.Reason, capability.ID)
		}
	}

	partial := evaluateOMPProviderFreeRPC(ompProbeResult{output: []byte(
		`{"type":"ready","protocolVersion":1,"supportedProtocolVersions":[1,2]}` + "\n")})
	for _, capability := range partial {
		assert.False(t, capability.Supported)
		assert.Equal(t, "response_missing", capability.Reason, capability.ID)
	}
}

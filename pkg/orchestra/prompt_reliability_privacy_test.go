package orchestra

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const privacyPromptSentinel = "PROMPT-SENTINEL-DO-NOT-PERSIST"

type privacyPromptReceipt struct {
	TransportMode string            `json:"transport_mode"`
	Status        string            `json:"status"`
	Mismatch      string            `json:"mismatch,omitempty"`
	FailureCode   string            `json:"failure_code,omitempty"`
	Prompt        SanitizedArtifact `json:"prompt"`
}

func TestSanitizePromptArtifactUsesUTF8ByteLength(t *testing.T) {
	const prompt = "한🐙"
	artifact := sanitizePromptArtifact(prompt)

	assert.Equal(t, len(prompt), artifact.ByteLength)
	assert.NotEmpty(t, artifact.Hash)
	assert.Empty(t, artifact.Preview)
}

func TestPromptReceiptCommonBoundaryAllowsOnlyStableFailureCodes(t *testing.T) {
	for _, test := range []struct {
		mode      string
		candidate string
		wantCode  string
	}{
		{mode: "file_ipc", candidate: "file_ipc_ready_failed", wantCode: "file_ipc_ready_failed"},
		{mode: "file_ipc", candidate: "RAW arbitrary failure", wantCode: "file_ipc_failed"},
		{mode: "prompt_ready", candidate: "RAW pane-1", wantCode: "prompt_ready_failed"},
		{mode: "send_long_text", candidate: "RAW pane-1", wantCode: "prompt_send_failed"},
		{mode: "submit_enter", candidate: "RAW pane-1", wantCode: "prompt_enter_failed"},
		{mode: "sendkeys", candidate: "RAW pane-1", wantCode: "prompt_sendkeys_failed"},
		{mode: "unknown", candidate: "RAW pane-1", wantCode: "prompt_transport_failed"},
		{mode: "sendkeys", candidate: "file_ipc_abort_failed", wantCode: "prompt_sendkeys_failed"},
		{mode: "file_ipc", candidate: "prompt_send_failed", wantCode: "file_ipc_failed"},
		{mode: "unknown", candidate: "file_ipc_ready_failed", wantCode: "prompt_transport_failed"},
	} {
		t.Run(test.mode+"_"+test.wantCode, func(t *testing.T) {
			receipt := promptReceipt(
				"run-privacy", "claude", test.mode,
				privacyPromptSentinel, 2, "failed", test.candidate,
			)
			data, err := json.Marshal(receipt)
			require.NoError(t, err)

			var persisted privacyPromptReceipt
			require.NoError(t, json.Unmarshal(data, &persisted))
			assert.Equal(t, test.wantCode, persisted.FailureCode)
			assert.Empty(t, persisted.Mismatch)
			assert.Empty(t, persisted.Prompt.Preview)
			assert.Equal(t, len(privacyPromptSentinel), persisted.Prompt.ByteLength)
			assert.NotEmpty(t, persisted.Prompt.Hash)
			assertPromptRawExcludes(t, string(data), privacyPromptSentinel)
			if test.candidate != test.wantCode {
				assertPromptRawExcludes(t, string(data), test.candidate)
			}
		})
	}
}

func TestPromptFailurePoliciesAllowEveryFallbackAndKnownCode(t *testing.T) {
	tests := []struct {
		mode  string
		codes []string
	}{
		{mode: promptTransportFileIPC, codes: []string{
			promptFailureFileIPCReady,
			promptFailureFileIPCInput,
			promptFailureFileIPCAbort,
			promptFailureFileIPCReleaseAck,
			promptFailureFileIPCFallback,
		}},
		{mode: promptTransportReady, codes: []string{promptFailureReady}},
		{mode: promptTransportSendLongText, codes: []string{promptFailureSend}},
		{mode: promptTransportSubmitEnter, codes: []string{promptFailureEnter}},
		{mode: promptTransportSendKeys, codes: []string{promptFailureSendKeys}},
		{mode: "unknown", codes: []string{promptFailureTransport}},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			policy := promptFailurePolicyForTransport(test.mode)
			assert.True(t, policy.allows(policy.fallback))
			for _, code := range test.codes {
				assert.True(t, policy.allows(code))
				assert.Equal(t, code, normalizePromptFailureCode(promptFailureStatusFailed, test.mode, code))
			}
		})
	}
}

func assertPromptRawExcludes(t *testing.T, raw string, forbidden ...string) {
	t.Helper()
	for _, value := range forbidden {
		assert.NotContains(t, raw, value)
	}
}

package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Stream lines of the S15 fixtures (claude 2.1.289 stream-json shapes).
const (
	lpInit55       = `{"type":"system","subtype":"init","model":"claude-opus-5-5","apiKeySource":"none","tools":["Glob","Grep","Read"]}`
	lpInit48       = `{"type":"system","subtype":"init","model":"claude-opus-4-8"}`
	lpAssistant55  = `{"type":"assistant","message":{"model":"claude-opus-5-5","content":[{"type":"text","text":"x"}]}}`
	lpAssistant48  = `{"type":"assistant","message":{"model":"claude-opus-4-8"}}`
	lpAssistantNil = `{"type":"assistant","message":{"content":[]}}`
	lpFallback48   = `{"type":"system","subtype":"model_refusal_fallback","original_model":"claude-opus-5-5","fallback_model":"claude-opus-4-8","api_refusal_category":"cyber"}`
)

func lpResult(text string) string {
	return `{"type":"result","subtype":"success","is_error":false,"result":` + lpJSONString(text) + `}`
}

func lpJSONString(s string) string {
	data, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func lpStream(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

func TestParseBandStream_ModelRecordAndChecks(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                 string
		stream, requested    string
		want                 localPatchModel
		substituted, refused bool
		unverified, success  bool
	}{
		{"init and assistants on the requested model",
			lpStream(lpInit55, lpAssistant55, lpResult("ok")), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}, false, false, false, true},
		{"refusal fallback names the fallback model and category",
			lpStream(lpInit55, lpFallback48, lpAssistant48, lpResult("ok")), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-4-8", RefusalCategory: "cyber"}, true, true, true, true},
		{"no init event",
			lpStream(lpAssistant55, lpResult("ok")), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}, true, false, true, true},
		{"assistant on another model without fallback",
			lpStream(lpInit55, lpAssistant55, lpAssistant48, lpResult("ok")), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-4-8"}, true, false, true, true},
		{"init other than requested",
			lpStream(lpInit48, lpAssistant48, lpResult("ok")), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-4-8"}, true, false, true, true},
		{"assistant without message.model",
			lpStream(lpInit55, lpAssistantNil, lpResult("ok")), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}, false, false, true, true},
		{"no --model is unverifiable",
			lpStream(lpInit55, lpAssistant55, lpResult("ok")), "",
			localPatchModel{Request: "patch", Actual: "claude-opus-5-5"}, true, false, true, true},
		{"result with is_error",
			lpStream(lpInit55, `{"type":"result","subtype":"success","is_error":true,"result":"x"}`), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}, false, false, false, false},
		{"no result event; junk lines skipped",
			lpStream("not json", `[1,2]`, `null`, lpInit55, `{"type":`), "claude-opus-5-5",
			localPatchModel{Request: "patch", Requested: "claude-opus-5-5", Actual: "claude-opus-5-5"}, false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reply := parseBandStream(tc.stream, "patch", tc.requested)
			assert.Equal(t, tc.want, reply.model)
			assert.Equal(t, tc.substituted, reply.substituted, "substituted")
			assert.Equal(t, tc.refused, reply.refused, "refused")
			assert.Equal(t, tc.unverified, reply.unverified, "unverified")
			assert.Equal(t, tc.success, reply.success, "success")
			assert.True(t, reply.streamed)
		})
	}
}

func TestParseBandStream_LastResultTextIsTheReply(t *testing.T) {
	t.Parallel()
	reply := parseBandStream(lpStream(lpInit55, lpResult("first"), lpResult("```diff\n+x\n```")), "diagnosis", "claude-opus-5-5")
	require.True(t, reply.success)
	assert.Equal(t, "```diff\n+x\n```", reply.text)
	assert.Equal(t, "diagnosis", reply.model.Request)
}

func TestBandConfinedReply_PatchCode_Order(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		reply bandConfinedReply
		want  string
	}{
		{"refused wins over unverified", bandConfinedReply{refused: true, unverified: true, success: true}, lpCodePatchModelRefused},
		{"unverified", bandConfinedReply{unverified: true, success: true}, lpCodePatchModelUnverified},
		{"stream past the bound goes to the Patch Policy", bandConfinedReply{overBound: true}, ""},
		{"no success result is empty output", bandConfinedReply{}, bandProviderEmptyOutput},
		{"verified success", bandConfinedReply{success: true, text: "x"}, ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.reply.patchCode(), tc.name)
	}
}

func TestBandConfinedReply_PatchReply_Bounds(t *testing.T) {
	t.Parallel()
	big := strings.Repeat("a", healthband.ProviderCaptureBytes+1)
	reply := bandConfinedReply{success: true, text: big}
	assert.True(t, reply.patchReply().Dropped, "a result text past 1 MiB is dropped")
	assert.Len(t, reply.patchReply().Text, healthband.ProviderCaptureBytes)
	reply = bandConfinedReply{overBound: true, success: true, text: "x"}
	assert.True(t, reply.patchReply().Dropped, "a stream past 8 MiB is too large")
	reply = bandConfinedReply{success: true, text: "x"}
	assert.Equal(t, healthband.PatchReply{Text: "x"}, reply.patchReply())
}

func TestBandConfinedReply_DiagnosisText(t *testing.T) {
	t.Parallel()
	text, dropped, reason := bandConfinedReply{success: true, text: "### Summary"}.diagnosisText()
	assert.Equal(t, "### Summary", text)
	assert.False(t, dropped)
	assert.Empty(t, reason)
	for _, reply := range []bandConfinedReply{{overBound: true, success: true, text: "x"}, {text: "x"}, {success: true, text: " \n"}} {
		_, _, reason = reply.diagnosisText()
		assert.Equal(t, bandProviderEmptyOutput, reason)
	}
}

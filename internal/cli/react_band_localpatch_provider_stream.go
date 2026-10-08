package cli

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Stream and model record of the Local Patch Provider Contract item 7
// (SPEC-SIGMABAND-002 REQ-03, REQ-07). claude can retry a refused request on
// another model and say so only in its event stream (probe A1 run 2), so a
// confined request runs with --output-format stream-json --verbose and band
// reads the captured stream as JSON lines: the system init event's model,
// every model_refusal_fallback event, the message.model of every assistant
// event, and the last result event, whose text is the reply.

// bandProviderStreamBytes bounds a confined request's stream: room for the
// JSON escaping of a 1 MiB result text and for tool events.
const bandProviderStreamBytes = 8 << 20

// bandStreamModel is a model name of the stream as band records it: a name
// outside the models[] alphabet (healthband.ValidLocalPatchModel) is treated
// as absent, so a patch request ends patch_model_unverified and no record or
// BS line is refused for it.
func bandStreamModel(model string) string {
	if !healthband.ValidLocalPatchModel(model) {
		return ""
	}
	return model
}

// bandStreamLine is the part of one stream-json event that band reads.
type bandStreamLine struct {
	Type            string  `json:"type"`
	Subtype         string  `json:"subtype"`
	Model           string  `json:"model"`
	FallbackModel   string  `json:"fallback_model"`
	RefusalCategory string  `json:"api_refusal_category"`
	IsError         bool    `json:"is_error"`
	Result          *string `json:"result"`
	Message         *struct {
		Model *string `json:"model"`
	} `json:"message"`
}

// bandConfinedReply is one confined request's stream as band read it.
type bandConfinedReply struct {
	model       localPatchModel // the models[] entry of the request
	streamed    bool            // the request returned a stream, so model holds its entry
	substituted bool            // model_substituted for this request
	refused     bool            // a model_refusal_fallback event occurred
	unverified  bool            // band cannot tell which model wrote the reply
	overBound   bool            // the stream passed bandProviderStreamBytes
	success     bool            // the last result event is a success without is_error
	text        string          // the result text of that event
	initModel   string          // the system init event's model
}

// parseBandStream reads a captured stream; a line that is not a JSON object
// is skipped. requested is the --model value of the argv, "" without one.
func parseBandStream(stream, request, requested string) bandConfinedReply {
	requested = bandStreamModel(requested)
	reply := bandConfinedReply{streamed: true, overBound: len(stream) > bandProviderStreamBytes}
	var (
		hasInit, missingModel   bool
		assistants              []string
		lastFallback, lastClass string
		result                  *bandStreamLine
	)
	for _, line := range strings.Split(stream, "\n") {
		line = strings.TrimSpace(line)
		var event bandStreamLine
		if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		switch {
		case event.Type == "system" && event.Subtype == "init" && !hasInit:
			hasInit, reply.initModel = true, bandStreamModel(event.Model)
		case event.Type == "system" && event.Subtype == "model_refusal_fallback":
			reply.refused = true
			lastFallback, lastClass = bandStreamModel(event.FallbackModel), bandStreamModel(event.RefusalCategory)
		case event.Type == "assistant":
			if event.Message == nil || event.Message.Model == nil || bandStreamModel(*event.Message.Model) == "" {
				missingModel = true
				continue
			}
			assistants = append(assistants, *event.Message.Model)
		case event.Type == "result":
			result = &event
		}
	}
	actual := reply.initModel
	differs := false
	for _, model := range assistants {
		if model != reply.initModel {
			actual, differs = model, true
		}
	}
	if reply.refused {
		actual = lastFallback
	}
	reply.model = localPatchModel{Request: request, Requested: requested, Actual: actual, RefusalCategory: lastClass}
	reply.substituted = reply.refused || differs || reply.initModel != requested
	reply.unverified = !hasInit || reply.initModel == "" || requested == "" || reply.initModel != requested ||
		missingModel || differs
	if result != nil && result.Subtype == "success" && !result.IsError && result.Result != nil {
		reply.success, reply.text = true, *result.Result
	}
	return reply
}

// capturedText takes the result text as 001's diagnosis capture does: the
// 1 MiB head, and whether bytes were dropped.
func (r bandConfinedReply) capturedText() (string, bool) {
	capture := healthband.NewHeadBuffer(healthband.ProviderCaptureBytes)
	_, _ = io.WriteString(capture, r.text)
	return capture.Captured()
}

// diagnosisText is the reply of a diagnosis: a stream past its bound or
// without a success result is 001's empty output, and a diagnosis proceeds
// whatever model answered (the substitution is only recorded).
func (r bandConfinedReply) diagnosisText() (string, bool, string) {
	if r.overBound || !r.success || strings.TrimSpace(r.text) == "" {
		return "", false, bandProviderEmptyOutput
	}
	text, dropped := r.capturedText()
	return text, dropped, ""
}

// patchCode is the end of a patch request at Local Patch Flow step 6: a
// refusal fallback (patch_model_refused) before an unverifiable model
// (patch_model_unverified), then empty output; a stream past its bound goes
// on to the Patch Policy, whose item 1 refuses it as patch_too_large.
func (r bandConfinedReply) patchCode() string {
	switch {
	case r.refused:
		return lpCodePatchModelRefused
	case r.unverified:
		return lpCodePatchModelUnverified
	case r.overBound:
		return ""
	case !r.success || strings.TrimSpace(r.text) == "":
		return bandProviderEmptyOutput
	}
	return ""
}

// patchReply is Patch Policy item 1's input: the captured result text, and
// whether the stream or the text capture passed its bound.
func (r bandConfinedReply) patchReply() healthband.PatchReply {
	text, dropped := r.capturedText()
	return healthband.PatchReply{Text: text, Dropped: dropped || r.overBound}
}

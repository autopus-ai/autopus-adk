package orchestra

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// providerEnvelope names the machine-readable stdout envelope a provider CLI
// was asked for. It is read from the executed argv, not from OutputFormat:
// OutputFormat describes the response text the envelope carries, which the
// rest of the pipeline validates after the envelope is removed.
type providerEnvelope int

const (
	envelopeNone       providerEnvelope = iota
	envelopeAgyStream                   // agy --output-format stream-json: NDJSON ending in a result event
	envelopeAgyJSON                     // agy --output-format json: one object at the end
	envelopeClaudeJSON                  // claude --output-format json: one type=result object
)

const outputFormatFlag = "--output-format"

// providerBinaryBase is the file name of a provider binary without .exe, so a
// configured absolute path and a bare name select the same envelope.
func providerBinaryBase(binary string) string {
	return strings.TrimSuffix(filepath.Base(strings.TrimSpace(binary)), ".exe")
}

// providerEnvelopeFor reports the envelope the argv requests. Claude
// stream-json stays raw: its confined consumer reads every event itself.
func providerEnvelopeFor(binary string, args []string) providerEnvelope {
	format := lastOutputFormat(args)
	switch providerBinaryBase(binary) {
	case "agy":
		switch format {
		case "stream-json":
			return envelopeAgyStream
		case "json":
			return envelopeAgyJSON
		}
	case "claude":
		if format == "json" {
			return envelopeClaudeJSON
		}
	}
	return envelopeNone
}

// lastOutputFormat is the last --output-format value before a `--` separator,
// in separated or inline form, or "" when the argv names none.
func lastOutputFormat(args []string) string {
	format := ""
	for i := 0; i < len(args) && args[i] != "--"; i++ {
		switch {
		case args[i] == outputFormatFlag && i+1 < len(args):
			format = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(args[i], outputFormatFlag+"="):
			format = strings.TrimSpace(strings.TrimPrefix(args[i], outputFormatFlag+"="))
		}
	}
	return format
}

// agyResult is the terminal payload of agy json and stream-json output.
type agyResult struct {
	Status   *string         `json:"status"`
	Response *string         `json:"response"`
	Usage    *agyUsageCounts `json:"usage"`
}

type agyUsageCounts struct {
	InputTokens     *int64 `json:"input_tokens"`
	OutputTokens    *int64 `json:"output_tokens"`
	ThinkingTokens  *int64 `json:"thinking_tokens"`
	CacheReadTokens *int64 `json:"cache_read_tokens"`
	TotalTokens     *int64 `json:"total_tokens"`
}

type agyStreamEvent struct {
	Event  string     `json:"event"`
	Result *agyResult `json:"result"`
}

type claudeResultEvent struct {
	Type    string  `json:"type"`
	Subtype string  `json:"subtype"`
	IsError bool    `json:"is_error"`
	Result  *string `json:"result"`
}

// terminalEnvelopeLine parses one stdout line as the terminal envelope of
// kind. Lines that are not JSON, or JSON that is not terminal, report false.
func terminalEnvelopeLine(kind providerEnvelope, line []byte) (agyResult, *claudeResultEvent, bool) {
	switch kind {
	case envelopeAgyStream:
		var event agyStreamEvent
		if json.Unmarshal(line, &event) == nil && event.Event == "result" && event.Result != nil {
			return *event.Result, nil, true
		}
	case envelopeAgyJSON:
		var result agyResult
		if json.Unmarshal(line, &result) == nil && (result.Status != nil || result.Response != nil) {
			return result, nil, true
		}
	case envelopeClaudeJSON:
		var event claudeResultEvent
		if json.Unmarshal(line, &event) == nil && event.Type == "result" {
			return agyResult{}, &event, true
		}
	}
	return agyResult{}, nil, false
}

// decodeProviderEnvelope returns the response text inside raw stdout. A
// stream with no terminal envelope, or one that reports failure, is an error:
// a process that died before its result has no answer to hand on.
func decodeProviderEnvelope(kind providerEnvelope, raw string) (string, error) {
	var (
		agy    agyResult
		claude *claudeResultEvent
		found  bool
	)
	visit := func(line []byte) {
		if a, c, ok := terminalEnvelopeLine(kind, line); ok {
			agy, claude, found = a, c, true
		}
	}
	forEachJSONLine(raw, visit)
	if !found && kind != envelopeAgyStream {
		// A pretty-printed single object spans lines.
		if start, end := strings.IndexByte(raw, '{'), strings.LastIndexByte(raw, '}'); start >= 0 && end > start {
			visit([]byte(raw[start : end+1]))
		}
	}
	if !found {
		return "", fmt.Errorf("no result envelope in %s output", envelopeName(kind))
	}
	if kind == envelopeClaudeJSON {
		if claude.IsError || claude.Result == nil {
			return "", fmt.Errorf("claude result reported failure (subtype %q)", claude.Subtype)
		}
		return *claude.Result, nil
	}
	if agy.Status != nil && *agy.Status != "SUCCESS" {
		return "", fmt.Errorf("agy result reported status %s", *agy.Status)
	}
	if agy.Response == nil {
		return "", fmt.Errorf("agy result has no response")
	}
	return *agy.Response, nil
}

func envelopeName(kind providerEnvelope) string {
	switch kind {
	case envelopeAgyStream:
		return "agy stream-json"
	case envelopeAgyJSON:
		return "agy json"
	case envelopeClaudeJSON:
		return "claude json"
	}
	return "provider"
}

// unwrapProviderEnvelope returns the response text of envelope output, or raw
// stdout unchanged for providers without one. When no terminal envelope
// decodes, raw stdout stays for diagnostics beside the error.
func unwrapProviderEnvelope(kind providerEnvelope, raw string) (string, error) {
	if kind == envelopeNone || strings.TrimSpace(raw) == "" {
		// Blank stdout stays the empty-output failure callers classify.
		return raw, nil
	}
	text, err := decodeProviderEnvelope(kind, raw)
	if err != nil {
		return raw, err
	}
	return text, nil
}

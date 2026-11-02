package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// decodeCompletionContract reads a present completion_contract value
// (REQ-WPOST-INPUT-02). Every problem is collected rather than stopping at the
// first, so the sender sees them all: a value that is not an object (null
// included), an unknown key, and a field of the wrong type (null included).
// Unknown keys are refused because a misspelt key would otherwise drop a
// requirement silently.
func decodeCompletionContract(raw json.RawMessage) (completionContractMessage, []string) {
	var contract completionContractMessage
	trimmed := bytes.TrimSpace(raw)
	if bytes.Equal(trimmed, []byte("null")) {
		return contract, []string{"completion_contract must be an object, got null"}
	}
	var fields map[string]json.RawMessage
	if len(trimmed) == 0 || trimmed[0] != '{' || json.Unmarshal(trimmed, &fields) != nil {
		return contract, []string{"completion_contract must be an object"}
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var problems []string
	for _, key := range keys {
		value := fields[key]
		var problem string
		switch key {
		case "version":
			contract.Version, problem = decodeContractInt(key, value)
		case "required_actions":
			contract.RequiredActions, problem = decodeContractStrings(key, value)
		case "forbidden_actions":
			contract.ForbiddenActions, problem = decodeContractStrings(key, value)
		case "remote":
			contract.Remote, problem = decodeContractString(key, value)
		case "ref":
			contract.Ref, problem = decodeContractString(key, value)
		case "expected_commit":
			contract.ExpectedCommit, problem = decodeContractString(key, value)
		case "push_match":
			contract.PushMatch, problem = decodeContractString(key, value)
		default:
			problem = "unknown key " + quoteClip(key)
		}
		if problem != "" {
			problems = append(problems, problem)
		}
	}
	return contract, problems
}

func decodeContractInt(key string, value json.RawMessage) (int, string) {
	var n int
	if isJSONNull(value) || json.Unmarshal(value, &n) != nil {
		return 0, key + " must be an integer"
	}
	return n, ""
}

func decodeContractString(key string, value json.RawMessage) (string, string) {
	var s string
	if isJSONNull(value) || json.Unmarshal(value, &s) != nil {
		return "", key + " must be a string"
	}
	return s, ""
}

func decodeContractStrings(key string, value json.RawMessage) ([]string, string) {
	var items []json.RawMessage
	if isJSONNull(value) || json.Unmarshal(value, &items) != nil {
		return nil, key + " must be an array of strings"
	}
	values := make([]string, 0, len(items))
	for i, item := range items {
		var s string
		if isJSONNull(item) || json.Unmarshal(item, &s) != nil {
			return nil, fmt.Sprintf("%s[%d] must be a string", key, i)
		}
		values = append(values, s)
	}
	return values, ""
}

// isJSONNull reports an explicit null, which encoding/json would otherwise
// accept silently as the zero value.
func isJSONNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

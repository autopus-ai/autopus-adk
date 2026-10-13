package omp

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompDisabledAgentsKey is the one project config key autopus mode claims on
// its own: OMP hides the bundled agents it lists from the task tool.
const ompDisabledAgentsKey = "task.disabledAgents"

var ompDisabledAgentsPath = strings.Split(ompDisabledAgentsKey, ".")

// ompDisabledAgentsIn reads task.disabledAgents through the managed-claim
// parser, which refuses duplicate keys, aliases, and flow-style parents.
func ompDisabledAgentsIn(data []byte) (any, bool, error) {
	document, err := parseOMPManagedDocument(data)
	if err != nil {
		return nil, false, err
	}
	if err := rejectDuplicateOMPManagedKeys(document); err != nil {
		return nil, false, err
	}
	location, err := locateOMPManagedPath(document, ompDisabledAgentsPath)
	if err != nil || !location.exists {
		return nil, false, err
	}
	value, err := decodeOMPManagedNode(location.value)
	return value, true, err
}

// ompDisabledAgentsFingerprint is the managed-value fingerprint of the key's
// state, the missing fingerprint when it is absent.
func ompDisabledAgentsFingerprint(value any, present bool) string {
	if !present {
		return OMPMissingManagedValueFingerprint()
	}
	fingerprint, err := FingerprintOMPManagedValue(value)
	if err != nil {
		return ""
	}
	return fingerprint
}

func ompDefaultDisabledAgentsFingerprint() string {
	return ompDisabledAgentsFingerprint(config.OMPAutopusDisabledAgents(), true)
}

// ompDisabledAgentsAreDefault reports the value ADK writes: exactly scout,
// security-reviewer, and sonic, in that order.
func ompDisabledAgentsAreDefault(value any, present bool) bool {
	return present && ompDisabledAgentsFingerprint(value, true) == ompDefaultDisabledAgentsFingerprint()
}

// setOMPDisabledAgents renders the key canonically, replacing or inserting it
// where the managed merge would, or removes it together with any parent the
// removal leaves empty. Rendering the same way every emission does is what
// lets a later reading tell an edit of this key alone from any other edit.
func setOMPDisabledAgents(data []byte, value any, present bool) ([]byte, error) {
	if !present {
		return removeOMPManagedPath(data, ompDisabledAgentsPath)
	}
	current, exists, err := ompDisabledAgentsIn(data)
	if err != nil {
		return nil, err
	}
	result, err := MergeOMPProjectManagedConfig(OMPProjectManagedInput{
		Existing: data, Mode: ompConfigFileMode,
		Claims: []OMPManagedKeyClaim{{
			Path: ompDisabledAgentsKey, Value: value, Complete: true, FullArrayOwnership: true,
			PriorFingerprint: ompDisabledAgentsFingerprint(current, exists),
		}},
	})
	if err != nil {
		return nil, err
	}
	return result.Bytes, nil
}

// applyOMPAgentSurfacePrior returns data with task.disabledAgents back in its
// preimage state: absent, or the default set. A file already in that state
// keeps its bytes.
func applyOMPAgentSurfacePrior(data []byte, prior string) ([]byte, error) {
	value, present, err := ompDisabledAgentsIn(data)
	if err != nil {
		return nil, err
	}
	switch {
	case ompDisabledAgentsFingerprint(value, present) == prior:
		return data, nil
	case prior == OMPMissingManagedValueFingerprint():
		return setOMPDisabledAgents(data, nil, false)
	case prior == ompDefaultDisabledAgentsFingerprint():
		return setOMPDisabledAgents(data, config.OMPAutopusDisabledAgents(), true)
	}
	return nil, fmt.Errorf("project ownership agent surface prior invalid")
}

// removeOMPManagedPath cuts one key's entry, then any parent mapping the cut
// left without entries, so a file ADK created for the key alone becomes empty
// again instead of keeping a bare parent line.
func removeOMPManagedPath(data []byte, segments []string) ([]byte, error) {
	for depth := len(segments); depth > 0; depth-- {
		document, err := parseOMPManagedDocument(data)
		if err != nil {
			return nil, err
		}
		location, err := locateOMPManagedPath(document, segments[:depth])
		if err != nil {
			return nil, err
		}
		if !location.exists || (depth < len(segments) && !ompEmptyManagedValue(location.value)) {
			return data, nil
		}
		start, end, err := ompManagedEntrySpan(data, document, location.key)
		if err != nil {
			return nil, err
		}
		data = append(append([]byte(nil), data[:start]...), data[end:]...)
	}
	return data, nil
}

func ompEmptyManagedValue(node *yaml.Node) bool {
	switch node.Kind {
	case yaml.MappingNode:
		return len(node.Content) == 0
	case yaml.ScalarNode:
		return node.Tag == "!!null" && strings.TrimSpace(node.Value) == ""
	}
	return false
}

func ompBlankConfig(data []byte) bool {
	return len(bytes.TrimSpace(data)) == 0
}

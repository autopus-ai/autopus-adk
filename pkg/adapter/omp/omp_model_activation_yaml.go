package omp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func canonicalOMPManagedValueJSON(value any) ([]byte, error) {
	normalized, err := normalizeOMPManagedValue(value)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("managed value is not canonicalizable: %w", err)
	}
	return data, nil
}

func normalizeOMPManagedValue(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(map[string]any, len(typed))
		for _, key := range keys {
			normalized, err := normalizeOMPManagedValue(typed[key])
			if err != nil {
				return nil, err
			}
			result[key] = normalized
		}
		return result, nil
	case map[string]string:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = item
		}
		return normalizeOMPManagedValue(result)
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			normalized, err := normalizeOMPManagedValue(item)
			if err != nil {
				return nil, err
			}
			result[i] = normalized
		}
		return result, nil
	case []string:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = item
		}
		return result, nil
	case string, bool, nil, int, int64, float64:
		return typed, nil
	default:
		return nil, fmt.Errorf("managed value type %T is unsupported", value)
	}
}

func ompManagedValueNode(value any) (*yaml.Node, error) {
	data, err := canonicalOMPManagedValueJSON(value)
	if err != nil {
		return nil, err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil || len(document.Content) != 1 {
		return nil, fmt.Errorf("decode canonical managed value: %w", err)
	}
	clearOMPManagedFlowStyle(document.Content[0])
	return document.Content[0], nil
}

func clearOMPManagedFlowStyle(node *yaml.Node) {
	node.Style = 0
	for _, child := range node.Content {
		clearOMPManagedFlowStyle(child)
	}
}

func parseOMPManagedDocument(data []byte) (*yaml.Node, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err == io.EOF {
		// Only comments: an empty mapping a claim can be appended to.
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	} else if err != nil {
		return nil, fmt.Errorf("managed config YAML is invalid: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err == nil {
		if len(extra.Content) > 0 {
			return nil, fmt.Errorf("managed_key_conflict: multiple YAML documents are unsupported")
		}
	} else if err != io.EOF {
		return nil, fmt.Errorf("managed config YAML is invalid: %w", err)
	}
	if len(document.Content) == 0 {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode || root.Style&yaml.FlowStyle != 0 {
		return nil, fmt.Errorf("managed_key_conflict: project config root must be a block mapping")
	}
	return root, nil
}

func rejectDuplicateOMPManagedKeys(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode {
		return fmt.Errorf("managed_key_conflict: YAML aliases are unsupported")
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return fmt.Errorf("managed_key_conflict: mapping keys must be strings")
			}
			if _, exists := seen[key.Value]; exists {
				return fmt.Errorf("duplicate_key: %s", key.Value)
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := rejectDuplicateOMPManagedKeys(child); err != nil {
			return err
		}
	}
	return nil
}

func decodeOMPManagedNode(node *yaml.Node) (any, error) {
	var value any
	if err := node.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode managed value: %w", err)
	}
	return normalizeOMPManagedValue(value)
}

func ompYAMLNodeContainsSequence(node *yaml.Node) bool {
	if node.Kind == yaml.SequenceNode {
		return true
	}
	for _, child := range node.Content {
		if ompYAMLNodeContainsSequence(child) {
			return true
		}
	}
	return false
}

func replaceOMPManagedEntry(data []byte, key, value *yaml.Node) ([]byte, error) {
	start, end, err := ompManagedEntrySpan(data, key)
	if err != nil {
		return nil, err
	}
	entry, err := renderOMPManagedEntry(key.Column-1, []string{key.Value}, value)
	if err != nil {
		return nil, err
	}
	result := make([]byte, 0, len(data)-(end-start)+len(entry))
	result = append(result, data[:start]...)
	result = append(result, entry...)
	result = append(result, data[end:]...)
	return result, nil
}

func insertOMPManagedEntry(data []byte, mapping *yaml.Node, missing []string, value *yaml.Node) ([]byte, error) {
	if len(missing) == 0 {
		return nil, fmt.Errorf("managed_key_conflict: missing insertion path")
	}
	indent := 0
	offset := len(data)
	if len(mapping.Content) > 0 {
		lastKey := mapping.Content[len(mapping.Content)-2]
		indent = lastKey.Column - 1
		_, blockEnd, err := ompManagedEntrySpan(data, lastKey)
		if err != nil {
			return nil, err
		}
		offset = blockEnd
	} else if mapping.Line > 0 {
		return nil, fmt.Errorf("managed_key_conflict: cannot extend an inline or empty parent mapping")
	}
	entry, err := renderOMPManagedEntry(indent, missing, value)
	if err != nil {
		return nil, err
	}
	prefix := append([]byte(nil), data[:offset]...)
	if len(prefix) > 0 && prefix[len(prefix)-1] != '\n' {
		prefix = append(prefix, '\n')
	}
	result := make([]byte, 0, len(prefix)+len(entry)+len(data[offset:]))
	result = append(result, prefix...)
	result = append(result, entry...)
	result = append(result, data[offset:]...)
	return result, nil
}

func renderOMPManagedEntry(indent int, path []string, value *yaml.Node) ([]byte, error) {
	current := cloneOMPManagedNode(value)
	for i := len(path) - 1; i >= 0; i-- {
		mapping := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mapping.Content = append(mapping.Content, scalarOMPModelYAML(path[i]), current)
		current = mapping
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(current); err != nil {
		return nil, fmt.Errorf("encode managed key: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close managed key encoder: %w", err)
	}
	prefix := strings.Repeat(" ", indent)
	lines := strings.SplitAfter(output.String(), "\n")
	var indented strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		indented.WriteString(prefix)
		indented.WriteString(line)
	}
	return []byte(indented.String()), nil
}

func cloneOMPManagedNode(node *yaml.Node) *yaml.Node {
	clone := *node
	clone.Content = make([]*yaml.Node, len(node.Content))
	for i, child := range node.Content {
		clone.Content[i] = cloneOMPManagedNode(child)
	}
	return &clone
}

func ompManagedEntrySpan(data []byte, key *yaml.Node) (int, int, error) {
	lines := ompManagedLineOffsets(data)
	if key.Line < 1 || key.Line > len(lines) {
		return 0, 0, fmt.Errorf("managed_key_conflict: key position is invalid")
	}
	start := lines[key.Line-1]
	end := len(data)
	keyIndent := key.Column - 1
	for lineIndex := key.Line; lineIndex < len(lines); lineIndex++ {
		lineStart := lines[lineIndex]
		lineEnd := len(data)
		if lineIndex+1 < len(lines) {
			lineEnd = lines[lineIndex+1]
		}
		line := strings.TrimSuffix(string(data[lineStart:lineEnd]), "\n")
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent <= keyIndent {
			end = lineStart
			break
		}
	}
	return start, end, nil
}

func ompManagedLineOffsets(data []byte) []int {
	offsets := []int{0}
	for i, value := range data {
		if value == '\n' && i+1 < len(data) {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

package omp

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ompManagedEntrySpan returns the byte span of key's entry in its block
// mapping. The node tree bounds the entry: it ends before the node that
// follows it in document order, so an indentless sequence (`key:\n- item`)
// or a four-space layout stays inside its entry whatever its columns. Only
// the lines just before that bound are read by indentation: after the
// entry's last content line, the first comment or document marker at or left
// of the key's column belongs to what follows, and the blank lines before it
// stay with the entry, as they always have.
func ompManagedEntrySpan(data []byte, root, key *yaml.Node) (int, int, error) {
	lines := ompManagedLineOffsets(data)
	if key.Line < 1 || key.Line > len(lines) {
		return 0, 0, fmt.Errorf("managed_key_conflict: key position is invalid")
	}
	following, found := ompManagedFollowingLine(root, key)
	if !found || following != 0 && (following <= key.Line || following > len(lines)) {
		return 0, 0, fmt.Errorf("managed_key_conflict: key position is invalid")
	}
	bound := len(lines)
	if following != 0 {
		bound = following - 1
	}
	keyIndent := key.Column - 1
	last := key.Line - 1
	for index := key.Line; index < bound; index++ {
		if ompManagedContentLine(ompManagedLine(data, lines, index), keyIndent) {
			last = index
		}
	}
	end := bound
	for index := last + 1; index < bound; index++ {
		if strings.TrimSpace(ompManagedLine(data, lines, index)) != "" {
			end = index
			break
		}
	}
	start := lines[key.Line-1]
	if end >= len(lines) {
		return start, len(data), nil
	}
	return start, lines[end], nil
}

// ompManagedFollowingLine returns the line of the node that follows key's
// entry in document order: key's next sibling key, or else the next sibling
// of the nearest enclosing node that has one. 0 means the entry runs to the
// end of the document; found reports that key is in the tree.
func ompManagedFollowingLine(node, key *yaml.Node) (line int, found bool) {
	step := 1
	if node.Kind == yaml.MappingNode {
		step = 2
	}
	for i := 0; i+step-1 < len(node.Content); i += step {
		switch {
		case node.Kind == yaml.MappingNode && node.Content[i] == key:
			line, found = 0, true
		case node.Kind == yaml.MappingNode:
			line, found = ompManagedFollowingLine(node.Content[i+1], key)
		default:
			line, found = ompManagedFollowingLine(node.Content[i], key)
		}
		if !found {
			continue
		}
		if line == 0 && i+step < len(node.Content) {
			line = node.Content[i+step].Line
		}
		return line, true
	}
	return 0, false
}

// ompManagedContentLine reports a line that belongs to an entry whose key
// sits at keyIndent: anything indented past the key, or a line at or left of
// it that is neither a comment nor a document marker. Before the next key,
// only an indentless sequence item is such a line.
func ompManagedContentLine(line string, keyIndent int) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if len(line)-len(strings.TrimLeft(line, " ")) > keyIndent {
		return true
	}
	marker := trimmed == "---" || trimmed == "..." ||
		strings.HasPrefix(trimmed, "--- ") || strings.HasPrefix(trimmed, "... ")
	return !strings.HasPrefix(trimmed, "#") && !marker
}

func ompManagedLine(data []byte, lines []int, index int) string {
	end := len(data)
	if index+1 < len(lines) {
		end = lines[index+1]
	}
	return strings.TrimSuffix(string(data[lines[index]:end]), "\n")
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

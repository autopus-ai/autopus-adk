package harneval

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// pathSegment is one json_path segment: an object key and an optional index.
type pathSegment struct {
	key   string
	index int
}

const (
	noIndex  = -1
	anyIndex = -2
)

var segmentPattern = regexp.MustCompile(`^([A-Za-z0-9_-]+)(?:\[([0-9]+|\*)\])?$`)

// parseJSONPath parses `segment('.'segment)*` where a segment is a key of
// [A-Za-z0-9_-]+ optionally followed by [N] or [*].
func parseJSONPath(expr string) ([]pathSegment, error) {
	if expr == "" {
		return nil, fmt.Errorf("json_path is empty")
	}
	parts := strings.Split(expr, ".")
	segments := make([]pathSegment, 0, len(parts))
	for _, part := range parts {
		match := segmentPattern.FindStringSubmatch(part)
		if match == nil {
			return nil, fmt.Errorf("json_path %q has an invalid segment %q", expr, part)
		}
		segment := pathSegment{key: match[1], index: noIndex}
		switch match[2] {
		case "":
		case "*":
			segment.index = anyIndex
		default:
			index, err := strconv.Atoi(match[2])
			if err != nil {
				return nil, fmt.Errorf("json_path %q index %q: %w", expr, match[2], err)
			}
			segment.index = index
		}
		segments = append(segments, segment)
	}
	return segments, nil
}

// resolveJSONPath returns every value the segments select from doc, in
// document order. A missing key, a non-object, or an out-of-range index
// selects nothing.
func resolveJSONPath(doc any, segments []pathSegment) []any {
	current := []any{doc}
	for _, segment := range segments {
		var next []any
		for _, value := range current {
			object, ok := value.(map[string]any)
			if !ok {
				continue
			}
			child, ok := object[segment.key]
			if !ok {
				continue
			}
			next = append(next, selectIndex(child, segment.index)...)
		}
		current = next
	}
	return current
}

func selectIndex(value any, index int) []any {
	if index == noIndex {
		return []any{value}
	}
	array, ok := value.([]any)
	if !ok {
		return nil
	}
	if index == anyIndex {
		return array
	}
	if index < len(array) {
		return []any{array[index]}
	}
	return nil
}

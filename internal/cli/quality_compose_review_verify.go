package cli

import (
	"fmt"
	"maps"
	"reflect"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// verifyReviewLineEdit is the whole-document check of the review line
// splice (Line Splice Writer Contract), the way verifyQualityLineEdit checks
// the quality writers. Every target must hold its planned value; with every
// target entry set aside, edited must parse to the same data as original,
// each alias compared by the data it resolves to; and edited must load,
// strictly and validated, to original's load with only the planned values
// changed. Any mismatch refuses the write with a target's path.
func verifyReviewLineEdit(original, edited []byte, targets []reviewTarget) error {
	var before, after yaml.Node
	if err := yaml.Unmarshal(original, &before); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if yaml.Unmarshal(edited, &after) != nil {
		return reviewWriteRefusal{key: targets[0].key()}
	}
	for _, target := range targets {
		if !target.heldBy(yamlPathValue(&after, target.path)) {
			return reviewWriteRefusal{key: target.key()}
		}
	}
	for _, target := range targets {
		dropYAMLPathEntry(&before, target.path)
		dropYAMLPathEntry(&after, target.path)
	}
	if !sameYAMLData(&before, &after) {
		return reviewWriteRefusal{key: targets[0].key()}
	}
	want, err := config.ParseConfig(original)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	got, err := config.ParseConfig(edited)
	if err != nil {
		return reviewWriteRefusal{key: targets[0].key()}
	}
	want.Orchestra.Providers = maps.Clone(want.Orchestra.Providers)
	for _, target := range targets {
		switch {
		case target.isList:
			want.Spec.ReviewGate.Providers = slices.Clone(target.list)
		case target.path[0] == "spec":
			setReviewGateTarget(&want.Spec.ReviewGate, target)
		default:
			name := target.path[2]
			entry := want.Orchestra.Providers[name]
			switch target.path[3] {
			case "model":
				entry.Model = target.value
			case "backend":
				entry.Backend = target.value
			case "model_policy":
				entry.ModelPolicy = target.value
			case "args":
				// The expanded list has the raw list's shape, so the same edit
				// planned on it is what the edited file loads to.
				planned, ok := slices.Clone(target.list), true
				if len(entry.Args) > 0 {
					planned, ok = target.args.planned(entry.Args)
				}
				if !ok {
					return reviewWriteRefusal{key: target.key()}
				}
				entry.Args = planned
			}
			want.Orchestra.Providers[name] = entry
		}
	}
	if !reflect.DeepEqual(want, got) {
		return reviewWriteRefusal{key: targets[0].key()}
	}
	return nil
}

// heldBy reports a node that holds the planned value: every value the splice
// writes is a plain string, and every args element it leaves is still a
// scalar with its raw value, whatever its tag, so a number or a bool next to
// the target values never refuses the edit (REQ-RVM-25). A removed key is
// held by its absence (SPEC-JUDGEMODEL-001).
func (t reviewTarget) heldBy(node *yaml.Node) bool {
	if t.remove || node == nil {
		return t.remove && node == nil
	}
	if !t.isList && t.args == nil {
		return node.Kind == yaml.ScalarNode && node.ShortTag() == "!!str" && node.Value == t.value
	}
	if node.Kind != yaml.SequenceNode || len(node.Content) != len(t.list) {
		return false
	}
	for i, item := range node.Content {
		written := t.written == nil || t.written[i]
		if item.Kind != yaml.ScalarNode || item.Value != t.list[i] || written && item.ShortTag() != "!!str" {
			return false
		}
	}
	return true
}

// yamlPathValue returns the value node at path below the document's root
// mapping, or nil.
func yamlPathValue(doc *yaml.Node, path []string) *yaml.Node {
	if len(doc.Content) == 0 {
		return nil
	}
	node := doc.Content[0]
	for _, step := range path {
		if _, node = yamlMappingPair(node, step); node == nil {
			return nil
		}
	}
	return node
}

// dropYAMLPathEntry removes the entry at path from the document it owns, as
// dropRootMappingKey does for a root key.
func dropYAMLPathEntry(doc *yaml.Node, path []string) {
	if len(doc.Content) == 0 {
		return
	}
	parent := doc.Content[0]
	for _, step := range path[:len(path)-1] {
		if _, parent = yamlMappingPair(parent, step); parent == nil || parent.Kind != yaml.MappingNode {
			return
		}
	}
	last := path[len(path)-1]
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == last {
			parent.Content = slices.Delete(slices.Clone(parent.Content), i, i+2)
			return
		}
	}
}

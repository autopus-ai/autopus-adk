package cli

import (
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/content"
)

// reviewInsert is a missing key the splice adds at the end of its mapping.
type reviewInsert struct {
	mapping *yaml.Node
	text    string // `<key>: <encoded value>`
	key     string // the target path, for a refusal
}

// spliceReviewTargets replaces the value span of every target in source by
// the Line Splice Writer Contract, locating each through the raw document
// (no environment expansion) and encoding new values with OMPYAMLScalar.
// Missing keys of one mapping are added together at its end, in target
// order. It returns the targets with every args edit's planned raw list. A
// target it cannot edit safely refuses the whole edit with its path.
func spliceReviewTargets(source []byte, targets []reviewTarget) ([]byte, []reviewTarget, error) {
	// A line break yaml.v3 sees but a split at \n does not moves node lines
	// away from the lines the splice cuts, as for the quality writers.
	if hasYAMLOnlyLineBreak(source) {
		return nil, nil, reviewWriteRefusal{key: targets[0].key()}
	}
	document, _, err := parseAutopusConfigDocument(source)
	if err != nil {
		return nil, nil, reviewWriteRefusal{key: targets[0].key()}
	}
	aliased := yamlAliasTargets(document)
	lines := strings.SplitAfter(string(source), "\n")
	newline := detectConfigNewline(source)
	resolved := slices.Clone(targets)
	var edits []reviewLineEdit
	var inserts []reviewInsert
	for i := range resolved {
		spans, insert, ok := planReviewSpan(document.Content[0], aliased, lines, newline, &resolved[i])
		if !ok {
			return nil, nil, reviewWriteRefusal{key: resolved[i].key()}
		}
		edits = append(edits, spans...)
		if insert != nil {
			inserts = append(inserts, *insert)
		}
	}
	for i, insert := range inserts {
		if slices.ContainsFunc(inserts[:i], func(earlier reviewInsert) bool { return earlier.mapping == insert.mapping }) {
			continue
		}
		var texts []string
		for _, same := range inserts[i:] {
			if same.mapping == insert.mapping {
				texts = append(texts, same.text)
			}
		}
		edit, ok := insertReviewKeys(insert.mapping, lines, newline, texts)
		if !ok {
			return nil, nil, reviewWriteRefusal{key: insert.key}
		}
		edits = append(edits, edit)
	}
	return applyReviewLineEdits(lines, edits, newline), resolved, nil
}

// planReviewSpan finds one target's spans. Every mapping on the way must be
// a block mapping no alias names, so nothing else in the file shares what the
// edit changes; the parent may hold no merge key; the key and value carry
// no anchor or alias. A missing scalar or args key is returned as an insert;
// a missing providers list refuses.
func planReviewSpan(root *yaml.Node, aliased map[*yaml.Node]bool, lines []string, newline string, target *reviewTarget) ([]reviewLineEdit, *reviewInsert, bool) {
	node := root
	for depth, step := range target.path {
		if node.Kind != yaml.MappingNode || node.Style&yaml.FlowStyle != 0 || aliased[node] {
			return nil, nil, false
		}
		key, value := yamlMappingPair(node, step)
		if depth < len(target.path)-1 {
			if key == nil || value == nil {
				return nil, nil, false
			}
			node = value
			continue
		}
		switch {
		case yamlMappingHasMerge(node):
			return nil, nil, false
		case key == nil && target.remove:
			return nil, nil, true
		case key == nil && target.isList:
			return nil, nil, false
		case key == nil && target.args != nil:
			list, ok := target.args.planned(target.args.migrated)
			target.list = list
			return nil, &reviewInsert{mapping: node, text: step + ": " + reviewFlowListText(list), key: target.key()}, ok
		case key == nil:
			return nil, &reviewInsert{mapping: node, text: step + ": " + content.OMPYAMLScalar(target.value), key: target.key()}, true
		case key.Anchor != "" || value.Anchor != "" || value.Kind == yaml.AliasNode || aliased[value]:
			return nil, nil, false
		case target.isList:
			edit, ok := spliceReviewList(value, lines, newline, target.list)
			return []reviewLineEdit{edit}, nil, ok
		case target.args != nil:
			edits, ok := spliceReviewArgsTarget(key, value, lines, newline, target)
			return edits, nil, ok
		case target.remove:
			edit, ok := removeReviewScalarLine(key, value, lines)
			return []reviewLineEdit{edit}, nil, ok
		}
		edit, ok := spliceReviewScalar(key, value, lines, target.value)
		return []reviewLineEdit{edit}, nil, ok
	}
	return nil, nil, false
}

// planned is the args list the edit leaves from args, or false when the
// planner refuses the wanted values.
func (t *reviewArgsTarget) planned(args []string) ([]string, bool) {
	edit, ok := planReviewArgsEdit(args, t.row, t.want)
	return edit.apply(args), ok
}

// spliceReviewArgsTarget edits the raw args node (REQ-RVM-25): a non-empty
// list gets only its target values or appended pairs, an empty list or a
// null value takes the migrated args with the target values changed, and any
// other node refuses.
func spliceReviewArgsTarget(key, value *yaml.Node, lines []string, newline string, target *reviewTarget) ([]reviewLineEdit, bool) {
	switch {
	case value.Kind == yaml.SequenceNode && len(value.Content) == 0:
		list, planned := target.args.planned(target.args.migrated)
		target.list = list
		edit, ok := spliceReviewList(value, lines, newline, list)
		return []reviewLineEdit{edit}, ok && planned
	case value.Kind == yaml.SequenceNode:
		raw := reviewArgsValues(value)
		edit, ok := planReviewArgsEdit(raw, target.args.row, target.args.want)
		if !ok {
			return nil, false
		}
		target.written = edit.written(raw)
		if target.list = edit.apply(raw); edit.empty() {
			return nil, true
		}
		return spliceReviewArgs(value, lines, newline, edit)
	case value.Kind == yaml.ScalarNode && value.ShortTag() == "!!null":
		list, planned := target.args.planned(target.args.migrated)
		target.list = list
		edit, ok := spliceReviewScalarText(key, value, lines, reviewFlowListText(list))
		return []reviewLineEdit{edit}, ok && planned
	}
	return nil, false
}

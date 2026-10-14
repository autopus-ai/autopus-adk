package config

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// placeholderSentinel stands in for every ${NAME} the loader would expand
// while the raw file is checked. A plain alphanumeric token keeps a flow
// mapping, a key, a tag, or a block scalar around it intact, so the checked
// tree has the shape the expanded file will have.
const placeholderSentinel = "AutopusPlaceholderSentinel"

// rejectRoleModelCandidatePlaceholders refuses a ${NAME} placeholder in any
// model candidate of role_model_policy before the loader expands the file
// (security review P4): a candidate's selector, thinking, family, or one of
// its keys would otherwise come from the environment. Keys and values under
// every `candidates` are checked, aliases are followed wherever they are
// anchored, and the refusal names the path only, never a value. The root
// profile and family keep the SPEC-MODELUX-002 behaviour: they load, and the
// OMP writer refuses to rewrite them.
func rejectRoleModelCandidatePlaceholders(raw []byte) error {
	if !bytes.Contains(raw, []byte("${")) {
		return nil
	}
	substituted := envVarPattern.ReplaceAll(raw, []byte(placeholderSentinel))
	var document yaml.Node
	if yaml.Unmarshal(substituted, &document) != nil || len(document.Content) == 0 {
		// Even with every placeholder replaced the file does not parse, so its
		// expanded shape is unknown: a placeholder anywhere in role_model_policy
		// is refused rather than left to the decoder.
		if roleModelPolicyBlockHasPlaceholder(raw) {
			return fmt.Errorf("role_model_policy.candidate_placeholder_unsupported: role_model_policy does not parse with its placeholders")
		}
		return nil
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "role_model_policy" {
			walk := roleModelPlaceholderWalk{visited: map[roleModelPlaceholderVisit]bool{}}
			return walk.find(root.Content[i+1], "role_model_policy", false)
		}
	}
	return nil
}

type roleModelPlaceholderVisit struct {
	node      *yaml.Node
	candidate bool
}

type roleModelPlaceholderWalk struct {
	visited map[roleModelPlaceholderVisit]bool
}

func (w roleModelPlaceholderWalk) find(node *yaml.Node, path string, candidate bool) error {
	visit := roleModelPlaceholderVisit{node: node, candidate: candidate}
	if node == nil || w.visited[visit] {
		return nil
	}
	w.visited[visit] = true
	switch node.Kind {
	case yaml.AliasNode:
		return w.find(node.Alias, path, candidate)
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			if candidate && roleModelPlaceholderIn(key) {
				return roleModelPlaceholderError(path + ".<key>")
			}
			inner := candidate || key.Value == "candidates"
			if err := w.find(node.Content[i+1], path+"."+roleModelPlaceholderLabel(key.Value), inner); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for index, item := range node.Content {
			if err := w.find(item, fmt.Sprintf("%s[%d]", path, index), candidate); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if candidate && roleModelPlaceholderIn(node) {
			return roleModelPlaceholderError(path)
		}
	}
	return nil
}

// roleModelPlaceholderIn reports a scalar holding a placeholder the loader
// would expand, or a literal "${" a quoted escape produced.
func roleModelPlaceholderIn(node *yaml.Node) bool {
	return node.Kind == yaml.ScalarNode &&
		(strings.Contains(node.Value, placeholderSentinel) || strings.Contains(node.Value, "${"))
}

func roleModelPlaceholderLabel(value string) string {
	if strings.Contains(value, placeholderSentinel) || strings.Contains(value, "${") {
		return "<key>"
	}
	return value
}

func roleModelPlaceholderError(path string) error {
	return fmt.Errorf("role_model_policy.candidate_placeholder_unsupported: %s", path)
}

var roleModelPolicyLine = regexp.MustCompile(`(?m)^role_model_policy\s*:`)

// roleModelPolicyBlockHasPlaceholder reports ${ in the role_model_policy
// block of a raw file: the top-level key's line through the line before the
// next top-level key. A file that names role_model_policy only inside a flow
// document counts as a match, since its block cannot be told apart.
func roleModelPolicyBlockHasPlaceholder(raw []byte) bool {
	location := roleModelPolicyLine.FindIndex(raw)
	if location == nil {
		return bytes.Contains(raw, []byte("role_model_policy"))
	}
	block := raw[location[0]:]
	lines := bytes.Split(block, []byte("\n"))
	for index, line := range lines {
		if index > 0 && len(line) > 0 && line[0] != ' ' && line[0] != '\t' && line[0] != '#' && line[0] != '-' {
			break
		}
		if bytes.Contains(line, []byte("${")) {
			return true
		}
	}
	return false
}

package config

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// rejectRoleModelCandidatePlaceholders refuses a ${NAME} placeholder in any
// model candidate of role_model_policy before the loader expands the file
// (security review P4): a candidate's selector, thinking, or family would
// otherwise come from the environment. The root profile and family keep the
// SPEC-MODELUX-002 behaviour: they load, and the OMP writer refuses to
// rewrite them. A file that does not parse is left to the decoder.
func rejectRoleModelCandidatePlaceholders(raw []byte) error {
	if !bytes.Contains(raw, []byte("${")) {
		return nil
	}
	var document yaml.Node
	if yaml.Unmarshal(raw, &document) != nil || len(document.Content) == 0 {
		return nil
	}
	root := document.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "role_model_policy" {
			return findRoleModelCandidatePlaceholder(root.Content[i+1], "role_model_policy", false)
		}
	}
	return nil
}

func findRoleModelCandidatePlaceholder(node *yaml.Node, path string, candidate bool) error {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i].Value
			if err := findRoleModelCandidatePlaceholder(node.Content[i+1], path+"."+key, candidate || key == "candidates"); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for index, item := range node.Content {
			if err := findRoleModelCandidatePlaceholder(item, fmt.Sprintf("%s[%d]", path, index), candidate); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if candidate && strings.Contains(node.Value, "${") {
			return fmt.Errorf("role_model_policy.candidate_placeholder_unsupported: %s", path)
		}
	}
	return nil
}

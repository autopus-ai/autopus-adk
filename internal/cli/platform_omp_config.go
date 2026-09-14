package cli

import (
	"errors"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

var ompProfilePlaceholderPattern = regexp.MustCompile(`\$\{[^}]*\}`)

func validateOMPProfileSource(original []byte) error {
	_, policy, err := parseAutopusConfigDocument(original)
	if err != nil {
		return err
	}
	if policy != nil && yamlNodeContainsOMPProfilePlaceholder(policy, make(map[*yaml.Node]bool)) {
		return errors.New("role_model_policy_placeholder_unsupported")
	}
	return nil
}

func yamlNodeContainsOMPProfilePlaceholder(node *yaml.Node, visited map[*yaml.Node]bool) bool {
	if node == nil || visited[node] {
		return false
	}
	visited[node] = true
	if node.Kind == yaml.ScalarNode && ompProfilePlaceholderPattern.MatchString(node.Value) {
		return true
	}
	if yamlNodeContainsOMPProfilePlaceholder(node.Alias, visited) {
		return true
	}
	for _, child := range node.Content {
		if yamlNodeContainsOMPProfilePlaceholder(child, visited) {
			return true
		}
	}
	return false
}

func parseAutopusConfigDocument(original []byte) (*yaml.Node, *yaml.Node, error) {
	var document yaml.Node
	if yaml.Unmarshal(original, &document) != nil || len(document.Content) != 1 ||
		document.Content[0].Kind != yaml.MappingNode ||
		len(document.Content[0].Content)%2 != 0 {
		return nil, nil, errors.New("autopus_config_invalid")
	}
	mapping := document.Content[0]
	seen := make(map[string]struct{}, len(mapping.Content)/2)
	var policy *yaml.Node
	for index := 0; index < len(mapping.Content); index += 2 {
		key := mapping.Content[index]
		if key.Kind != yaml.ScalarNode || key.Value == "" {
			return nil, nil, errors.New("autopus_config_invalid")
		}
		if _, duplicate := seen[key.Value]; duplicate {
			return nil, nil, errors.New("autopus_config_duplicate_key")
		}
		seen[key.Value] = struct{}{}
		if key.Value == "role_model_policy" {
			policy = mapping.Content[index+1]
		}
	}
	return &document, policy, nil
}

func marshalAutopusConfig(original []byte, cfg *config.HarnessConfig) ([]byte, error) {
	if cfg == nil || cfg.Validate() != nil {
		return nil, errors.New("autopus_config_invalid")
	}
	return replaceAutopusConfigSection(original, "role_model_policy", cfg.RoleModelPolicy)
}

// replaceAutopusConfigSection rewrites exactly one top-level key of an existing
// autopus.yaml and leaves every sibling node untouched, so a caller that owns
// one section never reformats the rest of a user's file. Comments inside the
// replaced section do not survive: the section is re-encoded from the typed
// value, which is the only representation the harness validates against.
func replaceAutopusConfigSection(original []byte, key string, section any) ([]byte, error) {
	document, _, err := parseAutopusConfigDocument(original)
	if err != nil {
		return nil, err
	}
	sectionData, err := yaml.Marshal(section)
	if err != nil {
		return nil, errors.New("autopus_config_marshal_failed")
	}
	var sectionDocument yaml.Node
	if yaml.Unmarshal(sectionData, &sectionDocument) != nil || len(sectionDocument.Content) != 1 {
		return nil, errors.New("autopus_config_marshal_failed")
	}
	mapping := document.Content[0]
	sectionIndex := -1
	for index := 0; index < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			sectionIndex = index + 1
			break
		}
	}
	if sectionIndex >= 0 {
		mapping.Content[sectionIndex] = sectionDocument.Content[0]
	} else {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			sectionDocument.Content[0],
		)
	}
	encoded, err := yaml.Marshal(document)
	if err != nil {
		return nil, errors.New("autopus_config_marshal_failed")
	}
	return encoded, nil
}

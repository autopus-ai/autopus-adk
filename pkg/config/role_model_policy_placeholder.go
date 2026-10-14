package config

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// rejectRoleModelCandidatePlaceholders refuses a ${NAME} placeholder that
// fills a model candidate of role_model_policy, or a key anywhere in it
// (security review P4): a candidate's selector, thinking, or family, or an
// agent, profile, or capability name, would otherwise come from the
// environment. tokens is the file expandConfigPlaceholders checked, so the
// tokenized tree has the shape of the expanded one. It is pruned as
// decodeStrict prunes and decoded with merge keys and aliases resolved, so a
// candidate is read where the decoder puts it: a root merge from a reserved
// block, a merge inside the policy or a candidate, and an anchored list all
// land in the policy. The root profile and family keep the SPEC-MODELUX-002
// behaviour: they load, and the OMP writer refuses to rewrite them. The
// refusal names the path, a key spelled as its placeholder, never a value.
func rejectRoleModelCandidatePlaceholders(tokens *placeholderTokens, expanded []byte) error {
	if tokens == nil {
		return nil
	}
	var document yaml.Node
	if err := yaml.Unmarshal(tokens.text, &document); err != nil || document.Kind == 0 {
		return nil
	}
	pruneToleratedKeys(&document)
	var decoded struct {
		Policy any `yaml:"role_model_policy"`
	}
	if document.Decode(&decoded) != nil {
		// A token carries no tag but !!str (noteSecrets), so the tokenized
		// file fails only on its structure, which the expanded file shares,
		// and its strict decode then stops the load. Should the expanded
		// file still hold a policy, it is refused.
		var policy struct {
			Policy any `yaml:"role_model_policy"`
		}
		if yaml.Unmarshal(expanded, &policy) == nil && policy.Policy != nil {
			return fmt.Errorf("role_model_policy.candidate_placeholder_unsupported: " +
				"role_model_policy does not decode with its placeholders unexpanded")
		}
		return nil
	}
	return tokens.candidatePlaceholder(decoded.Policy, "role_model_policy", false)
}

// candidatePlaceholder returns the refusal for the first token under value,
// in key order, that lands in a key or in a candidate.
func (p *placeholderTokens) candidatePlaceholder(value any, path string, candidate bool) error {
	switch typed := value.(type) {
	case map[string]any:
		general := make(map[any]any, len(typed))
		for key, item := range typed {
			general[key] = item
		}
		return p.candidatePlaceholder(general, path, candidate)
	case map[any]any:
		keys := make([]string, 0, len(typed))
		items := make(map[string]any, len(typed))
		for key, item := range typed {
			spelled := fmt.Sprint(key)
			keys = append(keys, spelled)
			items[spelled] = item
		}
		sort.Strings(keys)
		for _, key := range keys {
			child := path + "." + p.restore(key)
			if p.holds(key) {
				return roleModelPlaceholderError(child)
			}
			if path == "role_model_policy" && (key == "profile" || key == "family") {
				continue
			}
			if err := p.candidatePlaceholder(items[key], child, candidate || key == "candidates"); err != nil {
				return err
			}
		}
	case []any:
		for index, item := range typed {
			if err := p.candidatePlaceholder(item, fmt.Sprintf("%s[%d]", path, index), candidate); err != nil {
				return err
			}
		}
	case string:
		if candidate && p.holds(typed) {
			return roleModelPlaceholderError(path)
		}
	}
	return nil
}

func roleModelPlaceholderError(path string) error {
	return fmt.Errorf("role_model_policy.candidate_placeholder_unsupported: %s", path)
}

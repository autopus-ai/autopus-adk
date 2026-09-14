package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// ompRoleModelFamilyAliases maps operator-facing CLI spellings onto the
// canonical family values stored in role_model_policy.family. Only the
// canonical value is ever persisted, so config readers never learn aliases.
var ompRoleModelFamilyAliases = map[string]string{
	"claude": "anthropic",
	"gpt":    "openai",
}

// ompProfileAgentInheritToken clears an explicit root-level agent override.
const ompProfileAgentInheritToken = "inherit"

// ompProfileApplyOptions is the complete operator intent for one run. The same
// value builds the effective proposal for both --plan and apply, so a preview
// can never describe a different selection than the one apply persists.
type ompProfileApplyOptions struct {
	name     string
	family   string
	agents   []ompProfileAgentAssignment
	plan     bool
	jsonMode bool
}

// ompProfileAgentAssignment pins or clears one agent's root-level override.
type ompProfileAgentAssignment struct {
	agent    string
	inherit  bool
	selector string
	thinking string
}

// newOMPProfileApplyOptions validates every flag before the config snapshot is
// opened, so a malformed family or agent assignment never reaches a write.
func newOMPProfileApplyOptions(
	name string,
	family string,
	agents []string,
	plan bool,
	jsonMode bool,
) (ompProfileApplyOptions, error) {
	if !config.IsValidQualityPresetName(name) {
		return ompProfileApplyOptions{}, ompProfilePlanError{reason: "profile_name_invalid"}
	}
	canonicalFamily, err := normalizeOMPRoleModelFamily(family)
	if err != nil {
		return ompProfileApplyOptions{}, err
	}
	assignments, err := parseOMPProfileAgentAssignments(agents)
	if err != nil {
		return ompProfileApplyOptions{}, err
	}
	return ompProfileApplyOptions{
		name: name, family: canonicalFamily, agents: assignments, plan: plan, jsonMode: jsonMode,
	}, nil
}

// normalizeOMPRoleModelFamily folds a CLI family spelling onto its canonical
// stored value. An empty flag keeps the config's existing family.
func normalizeOMPRoleModelFamily(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return "", nil
	}
	if canonical, ok := ompRoleModelFamilyAliases[normalized]; ok {
		normalized = canonical
	}
	if !config.IsValidRoleModelFamily(normalized) {
		return "", fmt.Errorf(
			"family_invalid: %s (available: %s)",
			safeOMPOperatorToken(value), strings.Join(ompRoleModelFamilyChoices(), ", "),
		)
	}
	return normalized, nil
}

// ompRoleModelFamilyChoices lists canonical families plus their CLI aliases.
func ompRoleModelFamilyChoices() []string {
	canonical := config.RoleModelFamilies()
	choices := make([]string, 0, len(canonical)+len(ompRoleModelFamilyAliases))
	choices = append(choices, canonical...)
	for alias := range ompRoleModelFamilyAliases {
		choices = append(choices, alias)
	}
	sort.Strings(choices)
	return choices
}

// parseOMPProfileAgentAssignments parses every repeatable --agent value.
// Duplicate or malformed assignments fail before any config is prepared, so a
// bad flag can never reach the snapshot.
func parseOMPProfileAgentAssignments(values []string) ([]ompProfileAgentAssignment, error) {
	assignments := make([]ompProfileAgentAssignment, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		assignment, err := parseOMPProfileAgentAssignment(raw)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[assignment.agent]; duplicate {
			return nil, fmt.Errorf("agent_override_duplicate: %s", assignment.agent)
		}
		seen[assignment.agent] = struct{}{}
		assignments = append(assignments, assignment)
	}
	return assignments, nil
}

func parseOMPProfileAgentAssignment(raw string) (ompProfileAgentAssignment, error) {
	agent, value, split := strings.Cut(strings.TrimSpace(raw), "=")
	agent, value = strings.TrimSpace(agent), strings.TrimSpace(value)
	if !split || agent == "" || value == "" {
		return ompProfileAgentAssignment{}, fmt.Errorf(
			"agent_override_malformed: %s (expected <agent>=<provider/model>:<thinking> or <agent>=%s)",
			safeOMPOperatorToken(raw), ompProfileAgentInheritToken,
		)
	}
	// A native OMP agent name and any logical role that collapses onto it are
	// both valid keys: the operator sees native names in the preview and may
	// still pin a single logical role.
	if _, err := config.ResolveOMPPolicyAgent(agent); err != nil {
		return ompProfileAgentAssignment{}, fmt.Errorf(
			"agent_override_unknown_agent: %s", safeOMPOperatorToken(agent),
		)
	}
	if value == ompProfileAgentInheritToken {
		return ompProfileAgentAssignment{agent: agent, inherit: true}, nil
	}
	selector, thinking, hasThinking := strings.Cut(value, ":")
	if !hasThinking || thinking == "" || !safeOMPProfileSelector(selector) {
		return ompProfileAgentAssignment{}, fmt.Errorf(
			"agent_override_malformed: %s (expected <provider/model>:<thinking>)",
			safeOMPOperatorToken(value),
		)
	}
	if !config.IsOMPNativeThinkingLevel(thinking) {
		return ompProfileAgentAssignment{}, fmt.Errorf(
			"agent_override_thinking_invalid: %s", safeOMPOperatorToken(thinking),
		)
	}
	return ompProfileAgentAssignment{agent: agent, selector: selector, thinking: thinking}, nil
}

// safeOMPProfileSelector accepts exactly provider/model with both halves set.
func safeOMPProfileSelector(selector string) bool {
	provider, model, split := strings.Cut(selector, "/")
	return split && provider != "" && model != "" && !strings.Contains(model, "/")
}

package omp

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// OMPModelProjectionInput is a provider-neutral bridge from the routing
// resolver into OMP-native config.
type OMPModelProjectionInput struct {
	Agents []OMPProjectionAgent
	// AgentSource is the source the agents are served by. Empty infers it
	// from the agent names, as the routing compile does.
	AgentSource string
}

// OMPProjectionAgent is one native OMP agent whose route already resolved.
// Agents without a resolved route are absent and inherit runtime defaults.
// Role and Capability carry the semantic ADK identity the route was resolved
// under; they are audit provenance and are never emitted to OMP config.
type OMPProjectionAgent struct {
	Agent      string
	Role       string
	Capability string
	Selector   string
	Thinking   string
	Fallbacks  []OMPProjectionCandidate
}

// OMPProjectionCandidate is one ordered, exact availability fallback.
type OMPProjectionCandidate struct {
	Selector string
	Thinking string
}

// OMPModelProjection is the canonical OMP-native representation consumed by
// config activation. It binds concrete selectors to bundled agent names; no
// role alias and no generated agent definition stands between the two.
type OMPModelProjection struct {
	FallbackChains []OMPFallbackChainProjection
	Agents         []OMPAgentModelProjection
	// AgentSource is the source the agents were compiled for.
	AgentSource string `json:"-"`
}

type OMPFallbackChainProjection struct {
	Selector   string
	Candidates []string
}

type OMPAgentModelProjection struct {
	Agent             string
	Role              string
	Capability        string
	Thinking          string
	EffectiveSelector string
}

// CompileOMPModelProjection orders resolved agents in the served set's order.
// At most one entry per agent survives, because OMP resolves
// task.agentModelOverrides by exact agent name.
func CompileOMPModelProjection(input OMPModelProjectionInput) (OMPModelProjection, error) {
	names := make([]string, 0, len(input.Agents))
	for _, agent := range input.Agents {
		names = append(names, agent.Agent)
	}
	source := ompServedAgentSource(input.AgentSource, names)
	agents, err := validateOMPProjectionAgents(input.Agents, source)
	if err != nil {
		return OMPModelProjection{}, err
	}

	projection := OMPModelProjection{
		Agents:      make([]OMPAgentModelProjection, 0, len(agents)),
		AgentSource: source,
	}
	fallbacksBySelector := make(map[string][]string)
	for _, served := range config.OMPAgentNames(source) {
		resolved, selected := agents[served]
		if !selected {
			continue
		}
		selector := formatOMPProjectedSelector(resolved.Selector, resolved.Thinking)
		projection.Agents = append(projection.Agents, OMPAgentModelProjection{
			Agent: served, Role: resolved.Role, Capability: resolved.Capability,
			Thinking: resolved.Thinking, EffectiveSelector: selector,
		})
		if len(resolved.Fallbacks) == 0 {
			continue
		}
		// Fallback chains stay keyed by effective selector: agents sharing a
		// selector must agree on the chain or the projection fails closed.
		chain := OMPFallbackChainProjection{Selector: selector}
		for _, fallback := range resolved.Fallbacks {
			chain.Candidates = append(chain.Candidates,
				formatOMPProjectedSelector(fallback.Selector, fallback.Thinking))
		}
		if previous, exists := fallbacksBySelector[selector]; exists {
			if !equalOMPProjectedStrings(previous, chain.Candidates) {
				return OMPModelProjection{}, fmt.Errorf("fallback_chain_conflict: %s", selector)
			}
			continue
		}
		fallbacksBySelector[selector] = append([]string(nil), chain.Candidates...)
		projection.FallbackChains = append(projection.FallbackChains, chain)
	}
	return projection, nil
}

func validateOMPProjectionAgents(
	inputs []OMPProjectionAgent,
	servedSource string,
) (map[string]OMPProjectionAgent, error) {
	agents := make(map[string]OMPProjectionAgent, len(inputs))
	for _, input := range inputs {
		source, roleErr := config.OMPRoleAgent(input.Role)
		if roleErr != nil {
			return nil, fmt.Errorf("agent_role_unmapped: %q", input.Agent)
		}
		resolved, err := config.ResolveOMPPolicyAgent(source)
		if err != nil || !ompProjectionAgentGovernedBy(servedSource, source, input.Agent) {
			return nil, fmt.Errorf("agent_role_unmapped: %q", input.Agent)
		}
		if input.Capability != resolved.Capability {
			return nil, fmt.Errorf("role_capability_mismatch: agent=%s role=%s capability=%s",
				input.Agent, input.Role, input.Capability)
		}
		if _, exists := agents[input.Agent]; exists {
			return nil, fmt.Errorf("agent_duplicate: %q", input.Agent)
		}
		if err := validateOMPProjectedSelector(input.Selector, input.Thinking); err != nil {
			return nil, fmt.Errorf("agent %s: %w", input.Agent, err)
		}
		for index, fallback := range input.Fallbacks {
			if err := validateOMPProjectedSelector(fallback.Selector, fallback.Thinking); err != nil {
				return nil, fmt.Errorf("agent %s fallback[%d]: %w", input.Agent, index, err)
			}
		}
		agents[input.Agent] = input
	}
	return agents, nil
}

// ompProjectionAgentGovernedBy reports whether the role row source may carry
// the model of agent. Native agents follow the collapse; an Autopus agent is
// carried by its own row, and task by the planner row it borrows.
func ompProjectionAgentGovernedBy(servedSource, source, agent string) bool {
	if servedSource == config.OMPAgentSourceNative {
		return config.OMPNativeAgentGovernedBy(source, agent)
	}
	if agent == config.OMPAutopusDefaultAgent {
		representative, err := config.OMPNativeAgentRepresentative(agent)
		return err == nil && representative == source
	}
	for _, canonical := range config.CanonicalAgentNames() {
		if canonical == agent {
			return source == agent
		}
	}
	return false
}

func validateOMPProjectedSelector(selector, thinking string) error {
	parts := strings.Split(selector, "/")
	if len(parts) != 2 || !isOMPProjectionIdentifier(parts[0]) || !isOMPProjectionIdentifier(parts[1]) {
		return fmt.Errorf("selector_invalid: %q", selector)
	}
	if !config.IsOMPNativeThinkingLevel(thinking) {
		return fmt.Errorf("thinking_unsupported: %q", thinking)
	}
	return nil
}

func isOMPProjectionIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || (index > 0 && strings.ContainsRune("._-", char)) {
			continue
		}
		return false
	}
	return true
}

func formatOMPProjectedSelector(selector, thinking string) string {
	return selector + ":" + thinking
}

func equalOMPProjectedStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func splitOMPProjectedSelector(value string) (string, string, error) {
	index := strings.LastIndexByte(value, ':')
	if index <= 0 || index == len(value)-1 {
		return "", "", fmt.Errorf("selector_invalid: %q", value)
	}
	return value[:index], value[index+1:], nil
}

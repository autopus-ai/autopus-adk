package omp

import (
	"fmt"

	"github.com/insajin/autopus-adk/pkg/config"
)

// OMPModelOverlayFromProjection is the explicit bridge into config activation.
// It returns detached maps so activation cannot mutate the compiled projection.
func OMPModelOverlayFromProjection(
	projection OMPModelProjection,
) (OMPModelOverlayProjection, error) {
	overlay := OMPModelOverlayProjection{
		AgentModelOverrides: make(map[string]string, len(projection.Agents)),
		FallbackChains:      make(map[string][]string, len(projection.FallbackChains)),
	}
	for _, agent := range projection.Agents {
		if _, err := config.OMPNativeAgentRepresentative(agent.Agent); err != nil {
			return OMPModelOverlayProjection{}, err
		}
		if _, duplicate := overlay.AgentModelOverrides[agent.Agent]; duplicate {
			return OMPModelOverlayProjection{}, fmt.Errorf("agent_duplicate: %q", agent.Agent)
		}
		selector, thinking, splitErr := splitOMPProjectedSelector(agent.EffectiveSelector)
		if splitErr != nil {
			return OMPModelOverlayProjection{}, splitErr
		}
		if validateErr := validateOMPProjectedSelector(selector, thinking); validateErr != nil {
			return OMPModelOverlayProjection{}, validateErr
		}
		if thinking != agent.Thinking {
			return OMPModelOverlayProjection{}, fmt.Errorf("agent_projection_mismatch: agent=%s", agent.Agent)
		}
		overlay.AgentModelOverrides[agent.Agent] = agent.EffectiveSelector
	}
	for _, chain := range projection.FallbackChains {
		selector, thinking, splitErr := splitOMPProjectedSelector(chain.Selector)
		if splitErr != nil {
			return OMPModelOverlayProjection{}, splitErr
		}
		if validateErr := validateOMPProjectedSelector(selector, thinking); validateErr != nil {
			return OMPModelOverlayProjection{}, validateErr
		}
		if _, duplicate := overlay.FallbackChains[chain.Selector]; duplicate {
			return OMPModelOverlayProjection{}, fmt.Errorf("fallback_chain_duplicate: %s", chain.Selector)
		}
		for _, candidate := range chain.Candidates {
			candidateSelector, candidateThinking, candidateErr := splitOMPProjectedSelector(candidate)
			if candidateErr != nil {
				return OMPModelOverlayProjection{}, candidateErr
			}
			if validateErr := validateOMPProjectedSelector(candidateSelector, candidateThinking); validateErr != nil {
				return OMPModelOverlayProjection{}, validateErr
			}
		}
		overlay.FallbackChains[chain.Selector] = append([]string(nil), chain.Candidates...)
	}
	return overlay, nil
}

package config

import "fmt"

// OMP agent sources. Autopus is the only one: OMP is served the 17 canonical
// Autopus agents as generated .omp/agents files, and every bundled agent but
// task is disabled. agent_source: native restored the v0.50.125 surface, where
// every role collapsed onto the five bundled agents, and was removed in
// v0.50.129. A config that still declares it fails to load with
// agent_source_removed instead of being served the autopus surface without a
// word; an explicit agent_source: autopus stays valid.
const (
	OMPAgentSourceAutopus = "autopus"

	// ompAgentSourceRemovedNative is the retired value, recognized only to
	// refuse it with the way back.
	ompAgentSourceRemovedNative = "native"

	// OMPAutopusDefaultAgent is the bundled agent OMP runs a dispatch that
	// names no agent on. It stays enabled in autopus mode, otherwise every
	// unnamed dispatch would fail OMP's preflight.
	OMPAutopusDefaultAgent = "task"
)

// OMPAutopusAgentNames returns the 17 canonical agents followed by task, the
// agents autopus mode serves in rank order.
func OMPAutopusAgentNames() []string {
	return append(CanonicalAgentNames(), OMPAutopusDefaultAgent)
}

// OMPAutopusDisabledAgents is the task.disabledAgents value autopus mode
// claims: every bundled agent but task, across the omp releases ADK supports,
// so OMP offers the 17 Autopus agents and task only. 18.12.3 bundles scout,
// security-reviewer, sonic, and seance; 17.2.7 also designer and librarian.
// OMP filters the task tool's agents by name, so a name the installed
// release does not bundle is an inert entry. reviewer is not in it because
// the generated reviewer takes the bundled one's name. New names go at the
// end: the order is the emitted list and the default every claim compares
// with, and the list a new name retires moves to
// OMPAutopusRetiredDisabledAgents so a project claimed under it migrates.
func OMPAutopusDisabledAgents() []string {
	return []string{"scout", "security-reviewer", "sonic", "designer", "librarian", "seance"}
}

// OMPAutopusRetiredDisabledAgents lists every earlier value of
// OMPAutopusDisabledAgents, oldest first. A project claimed under one of them
// still holds ADK's value, not the user's: the next update re-claims the
// current list, and a release restores whichever generation the preimage
// held. Only the claim logic reads this; nothing emits a retired list.
func OMPAutopusRetiredDisabledAgents() [][]string {
	return [][]string{
		// v0.50.127: the bundles of omp 18.6.1 and 17.2.7, before 18.12.3
		// added seance.
		{"scout", "security-reviewer", "sonic", "designer", "librarian"},
	}
}

// validateOMPAgentSource accepts an absent or explicit autopus source. The
// removed native value gets its own error, which says what to do, so it is
// never mistaken for a typo.
func validateOMPAgentSource(declared string) error {
	switch declared {
	case "", OMPAgentSourceAutopus:
		return nil
	case ompAgentSourceRemovedNative:
		return fmt.Errorf("role_model_policy.agent_source_removed: %q was removed in v0.50.129; "+
			"delete role_model_policy.agent_source from autopus.yaml, then run auto update "+
			"to restore the Autopus agent surface", declared)
	}
	return fmt.Errorf("role_model_policy.agent_source_invalid: %q", declared)
}

// OMPAgentNames lists the agents this resolved profile routes: every agent
// autopus mode serves, or only the pinned agents and task for a pins-only
// profile.
func (c RoleModelProfileConf) OMPAgentNames() []string {
	if c.PinsOnly {
		return c.ompPinsOnlyAgentNames()
	}
	return OMPAutopusAgentNames()
}

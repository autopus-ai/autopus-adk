package config

// CodexResolutionSessionModel marks reverse-engineer's analysis-rung
// placement left on the Codex session model: the resolution carries no model
// and no effort, so the agent TOML writes neither line.
const CodexResolutionSessionModel CodexResolutionReason = "session_model"

// CodexReaAnalysisFallback is the Codex counterpart of OMPReaAnalysisFallback
// (SPEC-REA-001 scope expansion of the Gate 1 decision): an analysis-rung
// agent's standard native balanced placement, the analysis rung, falls back to the
// Codex session model instead of refusing generation when a read catalog
// lacks the rung's model or effort. resolution is the catalog's answer for
// that placement. An unreadable catalog keeps today's unverified placement,
// and every other rung, custom tier, explicit effort, or Ultra keeps its path.
func (q QualityConf) CodexReaAnalysisFallback(agent string, resolution CodexProfileResolution) bool {
	if !IsAnalysisRungAgent(agent) {
		return false
	}
	candidate, placed := q.NativeBalancedAgentCandidate(QualityProviderCodex, agent)
	if !placed || candidate != balancedRungCandidates[builtinRoleModelFamilyOpenAI][balancedRungAnalysis] {
		return false
	}
	return resolution.Fallback && resolution.Reason != CodexResolutionCatalogUnknown
}

// CodexSessionModelResolution is the answer of a placement that runs on the
// Codex session model.
func CodexSessionModelResolution(requested CodexProfile) CodexProfileResolution {
	return CodexProfileResolution{Requested: requested, Fallback: true, Reason: CodexResolutionSessionModel}
}

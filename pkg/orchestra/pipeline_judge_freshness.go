package orchestra

// @AX:ANCHOR: [AUTO] backend capability boundary for proving fresh per-request judge execution
// @AX:REASON: [AUTO] custom backends must opt in explicitly or the required judge freshness gate remains unverified
type freshPipelineExecutionBackend interface {
	freshExecutionPerRequest() bool
}

func pipelineBackendHasFreshExecutionSemantics(backend ExecutionBackend) bool {
	switch backend.(type) {
	case *subprocessBackend:
		return true
	}
	declared, ok := backend.(freshPipelineExecutionBackend)
	return ok && declared.freshExecutionPerRequest()
}

func verifyFreshPipelineJudgeSession(
	evidence *FreshJudgeSessionEvidence,
	response *ProviderResponse,
	backend ExecutionBackend,
) {
	if declared, ok := backend.(freshPipelineExecutionBackend); ok && declared.freshExecutionPerRequest() {
		if response == nil {
			evidence.Reason = "modeled fresh backend execution returned no response"
			return
		}
		evidence.Isolated = true
		evidence.Verified = true
		evidence.Reason = "fresh per-request backend execution verified"
		return
	}
	verifyFreshSubprocessJudgeSession(evidence, response)
}

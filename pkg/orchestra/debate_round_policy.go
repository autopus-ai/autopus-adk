package orchestra

func mayStopDebateEarly(round, planned int, responses []ProviderResponse, cfg OrchestraConfig) bool {
	return round >= 2 && round < planned && len(responses) >= 2 && consensusReached(responses, cfg)
}

// @AX:NOTE [AUTO] REQ-7 magic constant 0.66 — default consensus threshold; configurable via ConsensusThreshold field
func consensusReached(responses []ProviderResponse, cfg OrchestraConfig) bool {
	if len(responses) < 2 {
		return false
	}
	threshold := cfg.ConsensusThreshold
	if threshold <= 0 {
		threshold = 0.66 // Default consensus threshold
	}
	metrics := deriveConsensusMetrics(responses, threshold)
	return metrics != nil && metrics.TotalClaims > 0 && metrics.DissentClaims == 0
}

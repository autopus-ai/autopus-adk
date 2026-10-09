package orchestra

import "fmt"

// @AX:ANCHOR: [AUTO] fan_in>=3 fail-closed debate gate for judge verdict and fresh-session evidence
// @AX:REASON: [AUTO] interactive, subprocess, pipeline, and runner paths depend on the same blocked outcome contract
func finalizeDebateOutcome(result *OrchestraResult, cfg OrchestraConfig) (*OrchestraResult, error) {
	if result != nil && result.FreshJudgeSession == nil {
		result.FreshJudgeSession = freshJudgeSessionFromResponses(result.Responses)
	}
	result = finalizeOrchestraResultForConfig(result, cfg)
	if result == nil || cfg.Strategy != StrategyDebate || cfg.JudgeProvider == "" || cfg.NoJudge {
		return result, nil
	}
	evidenceErr := freshJudgeSessionError(result.FreshJudgeSession)
	if configErr := freshJudgeConfigError(findOrBuildJudgeConfig(cfg)); configErr != nil {
		evidenceErr = configErr
	}
	if evidenceErr != nil {
		appendDegradedReason(result, "fresh_judge_session")
	}
	if result.JudgeStatus == JudgePassed && evidenceErr == nil {
		return result, nil
	}
	if result.JudgeStatus == JudgePassed && evidenceErr != nil {
		result.FailedProviders = append(result.FailedProviders, FailedProvider{
			Name:            cfg.JudgeProvider,
			Role:            "judge",
			Error:           fmt.Sprintf("required fresh judge session evidence failed: %v", evidenceErr),
			FailureClass:    "execution_error",
			NextRemediation: "retry the judge in a fresh execution session or use --no-judge explicitly",
		})
	}
	if !hasJudgeFailure(result.FailedProviders) {
		result.FailedProviders = append(result.FailedProviders, FailedProvider{
			Name:            cfg.JudgeProvider,
			Role:            "judge",
			Error:           "required judge did not produce a usable verdict",
			FailureClass:    "execution_error",
			NextRemediation: "retry the judge or use --no-judge explicitly",
		})
	}
	result.JudgeStatus = JudgeFailed
	result.Degraded = true
	result.TerminalState = TerminalBlocked
	result.GateStatus = "blocked"
	result.AnalysisVerdict = "incomplete"
	appendDegradedReason(result, "judge_failure")
	result = finalizeOrchestraResultForConfig(result, cfg)
	return result, requiredJudgeError(result, cfg.JudgeProvider)
}

func hasJudgeFailure(failed []FailedProvider) bool {
	for _, entry := range failed {
		if entry.Role == "judge" {
			return true
		}
	}
	return false
}

func requiredJudgeError(result *OrchestraResult, judge string) error {
	detail := "required judge did not produce a usable verdict"
	for _, entry := range result.FailedProviders {
		if entry.Role == "judge" && entry.Error != "" {
			detail = entry.Error
			break
		}
	}
	return fmt.Errorf("orchestra: required judge %q failed: %s", judge, detail)
}

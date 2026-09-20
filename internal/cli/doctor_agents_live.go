package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/insajin/autopus-adk/pkg/agentprobe"
	"github.com/insajin/autopus-adk/pkg/telemetry"
)

var doctorOpenCodeLifecycle = agentprobe.RunOpenCode
var doctorCodexLifecycle = agentprobe.RunCodex

func runDoctorAgentLive(ctx context.Context, opts doctorAgentsOptions) (doctorAgentObservation, error) {
	observation := doctorAgentObservation{Platform: opts.platform, RuntimeVersion: "unknown", Status: "probe_not_implemented"}
	for _, p := range agentProbePlatforms {
		if p.name == opts.platform {
			_, err := exec.LookPath(p.executable)
			installed := err == nil
			observation.Installed = &installed
		}
	}
	if opts.platform == "codex" {
		evidence, err := doctorCodexLifecycle(ctx, agentprobe.CodexOptions{Model: opts.model, WorkingDir: opts.directory, Timeout: opts.timeout})
		observation.ExecutionSurface = "supervisor_native_collaboration_tools"
		observation.UsageReason = "native_call_attribution_not_collected"
		if err != nil {
			observation.FailureReason = agentprobe.CodexFailureCode(err)
		}
		observation.Transport = map[string]string{"cleanup_semantics": "archive_and_unload_owned_threads; archived_history_retained"}
		return finishDoctorAgentLive(observation, evidence, err)
	}
	if opts.platform != "opencode" {
		return observation, fmt.Errorf("native lifecycle collector unavailable for this platform; support remains unverified")
	}
	evidence, transport, runErr := doctorOpenCodeLifecycle(ctx, agentprobe.OpenCodeOptions{
		Endpoint: opts.endpoint, Username: opts.username, Password: os.Getenv(opts.passwordEnv),
		RuntimeVersion: opts.runtimeVersion, ProviderID: opts.provider, ModelID: opts.model,
		Directory: opts.directory, Timeout: opts.timeout,
	})
	observation.Transport = transport
	for _, message := range transport.Messages {
		if message.ErrorStatusCode != nil {
			observation.FailureReason = fmt.Sprintf("provider_http_%d", *message.ErrorStatusCode)
		}
	}
	observation.ExecutionSurface = "controller_created_native_linked_sessions"
	if evidence.SupervisorID != "unobserved" && evidence.SupervisorID != "" {
		teamEvidence, usageErr := agentprobe.OpenCodeTeamUsage(evidence, transport)
		if usageErr == nil {
			teamReport, aggregateErr := telemetry.SummarizeTeamUsage(teamEvidence)
			if aggregateErr == nil {
				observation.TeamEvidence = &teamEvidence
				observation.TeamUsage = &teamReport
			}
		}
		if observation.TeamUsage == nil {
			observation.UsageReason = "native_usage_incomplete_or_invalid"
		}
	}
	return finishDoctorAgentLive(observation, evidence, runErr)
}

func finishDoctorAgentLive(observation doctorAgentObservation, evidence agentprobe.Evidence, runErr error) (doctorAgentObservation, error) {
	if runErr != nil && observation.FailureReason == "" {
		observation.FailureReason = "native_probe_failed"
		if errors.Is(runErr, context.DeadlineExceeded) {
			observation.FailureReason = "probe_deadline_exceeded"
		}
		if errors.Is(runErr, context.Canceled) {
			observation.FailureReason = "probe_cancelled"
		}
	}
	observation.RuntimeVersion = evidence.RuntimeVersion
	decision, err := agentprobe.Evaluate(evidence)
	if err != nil {
		observation.Status = "native_capture_incomplete"
		return observation, fmt.Errorf("native lifecycle capture incomplete; inspect transport reason")
	}
	observation.Evidence = &evidence
	observation.Lifecycle = &decision
	observation.Status = decision.Overall
	if runErr != nil && decision.Overall == "unknown" {
		observation.Status = "blocked"
	}
	observation.RuntimeVerified = runErr == nil && decision.Overall == "pass"
	if !observation.RuntimeVerified {
		return observation, fmt.Errorf("native lifecycle probe incomplete; inspect gate and transport reasons")
	}
	return observation, nil
}

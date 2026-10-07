package orchestra

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/telemetry"
)

const usageSourceSubprocess = "subprocess_stdout"

type usageBinding struct {
	RunID    string
	CallID   string
	TaskID   string
	Provider string
	Model    string
	Effort   string
	Phase    string
	Role     string
	Attempt  int
}

func newUsageBinding(provider, role string, round int) usageBinding {
	runID := NewSessionID()
	providerID := sanitizeProviderName(provider)
	roleID := sanitizeProviderName(role)
	if roleID == "" {
		roleID = "call"
	}
	return usageBinding{
		RunID: runID, CallID: fmt.Sprintf("%s:%s:r%d", providerID, roleID, round),
		Provider: provider, Phase: role, Role: role,
	}
}

func decorateProviderUsage(provider ProviderConfig, role string, round int, raw string) ([]telemetry.UsageEnvelope, UsageCapability) {
	binding := newUsageBinding(provider.Name, role, round)
	name := strings.ToLower(strings.TrimSpace(provider.Name))
	binary := strings.ToLower(strings.TrimSpace(provider.Binary))
	var receipts []telemetry.UsageEnvelope
	supported := true
	switch {
	case name == "codex" || binary == "codex":
		receipts = parseCodexUsage(raw, binding)
	case name == "claude" || binary == "claude":
		receipts = parseClaudeUsage(raw, binding)
	default:
		supported = false
	}
	if supported {
		if len(receipts) > 0 {
			return receipts, UsageCapability{Supported: true, Observed: true, Source: usageSourceSubprocess}
		}
		return []telemetry.UsageEnvelope{unavailableUsage(binding)}, UsageCapability{
			Supported: true, Source: usageSourceSubprocess, Reason: telemetry.UsageReasonProviderAbsent,
		}
	}
	return []telemetry.UsageEnvelope{unavailableUsage(binding)}, UsageCapability{
		Source: usageSourceSubprocess, Reason: telemetry.UsageReasonProviderAbsent,
	}
}

func unavailableUsage(binding usageBinding) telemetry.UsageEnvelope {
	return telemetry.NormalizeUsage(telemetry.UsageInput{
		RunID: binding.RunID, CallID: binding.CallID, TaskID: binding.TaskID,
		Attempt: binding.Attempt, Provider: binding.Provider, Model: binding.Model,
		Effort: binding.Effort, Phase: binding.Phase, Role: binding.Role,
		Source: telemetry.UsageSourceProvider,
	})
}

func finishProviderResponse(startArgs providerResponseArgs, rawStdout, rawStderr, lastMessagePath string) (*ProviderResponse, error) {
	usage, capability := decorateProviderUsage(startArgs.provider, "", 0, rawStdout)
	stdout, stderr := applyCodexLastMessageOutput(rawStdout, rawStderr, lastMessagePath)
	response, err := buildProviderResponse(startArgs.start, startArgs.provider, stdout, stderr,
		startArgs.fastFailReason, startArgs.waitErr, startArgs.ctx, startArgs.exitCode)
	if response != nil {
		response.Usage = usage
		response.UsageCapability = capability
		startArgs.execution.finish(response.Duration)
		response.Execution = startArgs.execution
	}
	return response, err
}

type providerResponseArgs struct {
	start          time.Time
	provider       ProviderConfig
	fastFailReason string
	waitErr        error
	ctx            context.Context
	exitCode       int
	execution      *ProviderExecution
}

func aggregateOrchestraUsage(result *OrchestraResult) {
	if result == nil {
		return
	}
	var receipts []telemetry.UsageEnvelope
	var capabilities []UsageCapability
	appendResponse := func(response ProviderResponse) {
		receipts = append(receipts, response.Usage...)
		capabilities = append(capabilities, response.UsageCapability)
	}
	for _, response := range result.Responses {
		appendResponse(response)
	}
	for _, round := range result.RoundHistory {
		for _, response := range round {
			appendResponse(response)
		}
	}
	for _, failed := range result.FailedProviders {
		receipts = append(receipts, failed.Usage...)
		capabilities = append(capabilities, failed.UsageCapability)
	}
	result.Usage = dedupeUsage(receipts)
	result.UsageAggregate = telemetry.AggregateUsage(receipts)
	result.UsageCapability = aggregateUsageCapability(capabilities)
}

func dedupeUsage(receipts []telemetry.UsageEnvelope) []telemetry.UsageEnvelope {
	type identity struct{ runID, callID string }
	seen := make(map[identity]telemetry.UsageEnvelope, len(receipts))
	result := make([]telemetry.UsageEnvelope, 0, len(receipts))
	for _, receipt := range receipts {
		key := identity{receipt.RunID, receipt.CallID}
		if prior, exists := seen[key]; exists {
			if telemetry.AggregateUsage([]telemetry.UsageEnvelope{prior, receipt}).PromotionBlocked {
				result = append(result, receipt)
			}
			continue
		}
		seen[key] = receipt
		result = append(result, receipt)
	}
	return result
}

func aggregateUsageCapability(capabilities []UsageCapability) UsageCapability {
	if len(capabilities) == 0 {
		return UsageCapability{Reason: telemetry.UsageReasonProviderAbsent}
	}
	result := UsageCapability{Supported: true, Observed: true, Source: "mixed"}
	for _, capability := range capabilities {
		result.Supported = result.Supported && capability.Supported
		result.Observed = result.Observed && capability.Observed
		if result.Reason == "" && capability.Reason != "" {
			result.Reason = capability.Reason
		}
	}
	return result
}

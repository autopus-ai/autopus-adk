package cli

import (
	"context"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

func buildOMPPlatformProjection(
	ctx context.Context,
	root string,
	runner omp.OMPModelCatalogRunner,
	now time.Time,
) ompPlatformProjection {
	projection := defaultOMPPlatformProjection()
	rows, agentCatalog := buildOMPAgentCatalog(root)
	projection.Models = newOMPModelOperatorProjection(rows, agentCatalog)
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	if !config.Exists(root) {
		projection.Reason = "autopus_config_missing"
		projection.Blockers = []string{"config:missing"}
		return projection
	}
	cfg, err := config.LoadPreview(root)
	if err != nil {
		projection.Reason = "autopus_config_invalid"
		projection.Blockers = []string{"config:invalid"}
		return projection
	}
	if !containsOMPString(cfg.Platforms, "omp") {
		projection.Reason = "omp_platform_not_configured"
		projection.Blockers = []string{"platform:not_configured"}
		return projection
	}
	projection.Configured = true
	projection.Models = buildOMPModelOperatorProjection(ctx, root, cfg, runner, projection.Models)
	projection.Context = buildOMPContextOperatorProjection(ctx, root, cfg, runner, now)
	projection.ChildRuntime.EvidenceSource = projection.Context.EvidenceSource
	projection.ReceiptVerification = ompReceiptVerificationProjection{
		ModelStatus: projection.Models.ReceiptStatus, ModelVerified: projection.Models.ReceiptVerified,
		ModelReason:   projection.Models.Reason,
		ContextStatus: projection.Context.ReceiptStatus, ContextVerified: projection.Context.ReceiptVerified,
		ContextReason: projection.Context.Reason,
	}
	projection.Blockers = collectOMPOperatorBlockers(projection)
	projection.Status, projection.Reason = summarizeOMPOperatorStatus(projection)
	return projection
}

func buildOMPModelOperatorProjection(
	ctx context.Context,
	root string,
	cfg *config.HarnessConfig,
	runner omp.OMPModelCatalogRunner,
	result ompModelOperatorProjection,
) ompModelOperatorProjection {
	profileName, profile, selected := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	if !selected {
		return result
	}
	result.Enabled = true
	result.Profile = safeOMPOperatorToken(profileName)
	input := buildOMPModelDoctorInput(ctx, root, cfg, runner)
	report := omp.CheckOMPModelRoutingDoctor(input)
	result.Status = safeOMPOperatorReason(report.Status)
	if result.Status == "not_available" {
		result.Status = "blocked"
	}
	result.Reason = safeOMPOperatorReason(report.Reason)
	result.CatalogStatus = safeOMPOperatorReason(input.Probe.Status)
	result.CatalogReason = safeOMPOperatorReason(input.Probe.Reason)
	result.CatalogTrust = safeOMPOperatorToken(input.Probe.CatalogTrust)
	result.CatalogVersion = safeOMPOperatorVersion(input.Probe.Version)
	result.CatalogFingerprint = input.Probe.Catalog.Fingerprint
	result.ReceiptStatus = safeOMPOperatorReason(report.ReceiptStatus)
	result.ReceiptVerified = result.ReceiptStatus == "valid" && result.Status != "blocked"
	result.Models = overlayOMPAgentCatalog(result.Models, input.Compilation.Resolutions, profile, result.ReceiptVerified)
	return result
}

func buildOMPContextOperatorProjection(
	ctx context.Context,
	root string,
	cfg *config.HarnessConfig,
	runner ompContextDoctorRunner,
	now time.Time,
) ompContextOperatorProjection {
	result := ompContextOperatorProjection{
		Status: "disabled", Reason: "profile_not_selected", PromotionFreshness: "not_applicable",
		ReceiptStatus: "not_applicable", ReceiptFreshness: "not_applicable", EvidenceSource: "not_available",
	}
	policy, selected, err := workflowContextPolicyFromConfig(cfg)
	if err != nil || !selected {
		if err != nil {
			result.Status, result.Reason = "blocked", "context_profile_invalid"
		}
		return result
	}
	input := ompContextDoctorInput{
		Enabled: true, Profile: policy.Profile, RequestedHistoryMode: policy.HistoryMode,
		RequestedMemoryMode: policy.MemoryMode, FallbackMode: policy.Fallback,
		RuntimeRootPolicy: policy.RuntimeRootPolicy,
		Current:           probeOMPContextCurrentRuntime(ctx, runner), Receipt: readOMPContextDoctorReceipt(root, now),
	}
	report := checkOMPContextDoctor(input)
	result.Enabled = true
	result.Profile = safeOMPOperatorToken(report.Profile)
	result.Status = safeOMPOperatorReason(report.Status)
	result.Reason = safeOMPOperatorReason(report.Reason)
	result.RequestedHistoryMode = safeOMPOperatorToken(report.RequestedHistoryMode)
	result.EffectiveHistoryMode = safeOMPOperatorToken(report.EffectiveHistoryMode)
	result.RequestedMemoryMode = safeOMPOperatorToken(report.RequestedMemoryMode)
	result.EffectiveMemoryMode = safeOMPOperatorToken(report.EffectiveMemoryMode)
	result.FallbackMode = safeOMPOperatorToken(report.FallbackMode)
	result.FallbackReason = safeOMPOperatorReason(report.FallbackReason)
	result.ReceiptStatus = safeOMPOperatorReason(report.ReceiptStatus)
	result.ReceiptFreshness = safeOMPOperatorReason(report.ReceiptFreshness)
	result.ReceiptVerified = result.ReceiptStatus == "valid" && result.ReceiptFreshness == "fresh"
	if input.Receipt.Status == "valid" {
		result.EvidenceSource = safeOMPOperatorToken(input.Receipt.Receipt.Capabilities.ProbeSource)
		result.PromotionFreshness = ompPromotionFreshness(
			input.Receipt.Receipt.PromotionCheckedAt, input.Receipt.Freshness, now,
		)
	} else {
		result.PromotionFreshness = result.ReceiptFreshness
	}
	return result
}

func collectOMPOperatorBlockers(projection ompPlatformProjection) []string {
	var blockers []string
	// OMP owns the bundled agent registry, so a missing local definition is the
	// expected state and never blocks. A project file that shadows a bundled
	// agent is a real degradation the operator has to see.
	if projection.Models.AgentCatalogStatus == "degraded" {
		blockers = appendUniqueOMPBlocker(blockers, "agents:"+projection.Models.AgentCatalogReason)
	}
	if projection.Models.Enabled {
		if projection.Models.CatalogStatus != "ready" || projection.Models.CatalogReason != "catalog_ready" {
			blockers = appendUniqueOMPBlocker(blockers, "models:"+projection.Models.CatalogReason)
		}
		if projection.Models.Status == "blocked" {
			blockers = appendUniqueOMPBlocker(blockers, "models:"+projection.Models.Reason)
		}
		for _, row := range projection.Models.Models {
			if row.Status == "blocked" {
				blockers = appendUniqueOMPBlocker(blockers, "models:"+row.Capability+":"+row.Reason)
			}
		}
	}
	if projection.Context.Enabled && projection.Context.Status != "supported" {
		blockers = appendUniqueOMPBlocker(blockers, "context:"+projection.Context.Reason)
	}
	return sortOMPBlockers(blockers)
}

func summarizeOMPOperatorStatus(projection ompPlatformProjection) (string, string) {
	if projection.Models.Enabled && projection.Models.Status == "blocked" ||
		projection.Context.Enabled && projection.Context.Status == "blocked" {
		if len(projection.Blockers) != 0 {
			return "blocked", projection.Blockers[0]
		}
		return "blocked", "operator_check_blocked"
	}
	if projection.Models.AgentCatalogStatus == "degraded" ||
		projection.Models.Enabled && projection.Models.Status == "degraded" ||
		projection.Context.Enabled && projection.Context.Status == "degraded" {
		if len(projection.Blockers) != 0 {
			return "degraded", projection.Blockers[0]
		}
		return "degraded", "operator_check_degraded"
	}
	return "ready", "ready"
}

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/execplane"
)

// pipelineTierIntegrityReceiptSchema versions the gate's receipt. It follows the
// existing pipeline_execution_owner_receipt.v1 convention — one schema-tagged
// JSON file per concern under pipelineStateDir — instead of inventing a second
// observation mechanism.
const pipelineTierIntegrityReceiptSchema = "pipeline_tier_integrity_receipt.v1"

// pipelineTierIntegrityProviders are the providers whose accounts the process
// plane owns. Both are answered by the single `orca account list --json`
// response the probe already reads, so covering both costs no extra lookup.
var pipelineTierIntegrityProviders = []string{execplane.ProviderClaude, execplane.ProviderCodex}

// execplaneTierProbeFunc gathers the read-only evidence one provider's verdict
// needs. Keeping it a package-level seam lets tests drive the gate without an
// orca binary, matching runtimeCodexCatalogProbe.
type execplaneTierProbeFunc func(context.Context, string) (execplane.Evidence, error)

var runtimeExecplaneTierProbe execplaneTierProbeFunc = execplane.NewProber().Inspect

// pipelineTierIntegrityReceipt records the gate outcome: one folded verdict plus
// the per-provider receipts it was folded from, so a later reader can tell an
// unresolved execution account apart from an unverifiable catalog.
type pipelineTierIntegrityReceipt struct {
	Schema             string                       `json:"schema"`
	SpecID             string                       `json:"spec_id"`
	VerificationStatus string                       `json:"verification_status"`
	Reason             string                       `json:"reason"`
	CheckedAt          time.Time                    `json:"checked_at"`
	Providers          []execplane.IntegrityReceipt `json:"providers"`
}

// pipelineTierIntegrityResult is what the handoff boundary needs from the gate.
// The reason is never empty: a status a reader cannot act on is not evidence.
type pipelineTierIntegrityResult struct {
	Status      string
	Reason      string
	ReceiptPath string
}

// runPipelineTierIntegrityGate checks that the tier this run requests was
// decided under the entitlement that will actually serve it, and persists the
// receipt.
//
// It only reads: one `orca account list --json` roster and credentials already
// on disk. No checkpoint, worktree, Run, worker, or provider session exists yet
// when it returns, so an unverified verdict leaves nothing behind (REQ-007).
//
// An unverified verdict is recorded, not fatal. REQ-006 admits an explicit
// record beside fail-closed, and the requested tier still reaches the process
// plane exactly as it did before this gate existed — so failing the run would
// break working setups over a check that is new. What it must never do is pass
// quietly: the status and reason travel with the handoff result.
func runPipelineTierIntegrityGate(ctx context.Context, projectDir, specID string) pipelineTierIntegrityResult {
	cfg, err := config.LoadPreview(projectDir)
	if err != nil {
		return pipelineTierIntegrityResult{
			Status: execplane.StatusUnverified,
			Reason: "quality configuration is unreadable, so the requested tier is unknown",
		}
	}
	checkedAt := time.Now().UTC()
	receipts := make([]execplane.IntegrityReceipt, 0, len(pipelineTierIntegrityProviders))
	for _, provider := range pipelineTierIntegrityProviders {
		// The probe's error adds nothing the receipt does not already carry: a
		// failed probe returns an indeterminate resolution whose non-empty
		// reason lands in the receipt, which is REQ-009's unverified branch and
		// not a pipeline abort. That covers orca missing from PATH.
		evidence, _ := runtimeExecplaneTierProbe(ctx, provider)
		receipts = append(receipts, evaluateProviderTierIntegrity(
			ctx, cfg, tierRequestForProvider(cfg.Quality, provider), evidence, checkedAt))
	}
	status, reason := foldPipelineTierIntegrity(receipts)
	path, err := persistPipelineTierIntegrityReceipt(specID, pipelineTierIntegrityReceipt{
		Schema: pipelineTierIntegrityReceiptSchema, SpecID: specID,
		VerificationStatus: status, Reason: reason, CheckedAt: checkedAt, Providers: receipts,
	})
	if err != nil {
		// A verdict nobody can reconstruct is not evidence, so an unwritable
		// receipt degrades the verdict even when every provider verified.
		return pipelineTierIntegrityResult{
			Status: execplane.StatusUnverified,
			Reason: "tier integrity receipt could not be persisted: " + err.Error(),
		}
	}
	return pipelineTierIntegrityResult{Status: status, Reason: reason, ReceiptPath: path}
}

// evaluateProviderTierIntegrity turns one provider's evidence into a receipt,
// taking REQ-004's re-probe branch before the verdict is formed. Matching
// grades never reach it, so the ordinary path stays free of subprocesses.
func evaluateProviderTierIntegrity(
	ctx context.Context,
	cfg *config.HarnessConfig,
	request execplane.TierRequest,
	evidence execplane.Evidence,
	checkedAt time.Time,
) execplane.IntegrityReceipt {
	catalog, note := reprobeCatalogUnderExecutionAccount(ctx, cfg, request.Provider, evidence)
	receipt := execplane.Evaluate(
		request, evidence.Resolution, evidence.ExecEntitlement, catalog, checkedAt)
	if note != "" {
		receipt.ResolutionReason += "; " + note
	}
	return receipt
}

// pipelineTierIntegritySkipped is the outcome recorded when no gate ran. REQ-008
// places the check at the process-plane handoff, so the OMP-owned path never
// probes orca — and claiming "verified" without a check is exactly the silent
// downgrade this SPEC exists to prevent.
func pipelineTierIntegritySkipped() pipelineTierIntegrityResult {
	return pipelineTierIntegrityResult{
		Status: execplane.StatusUnverified,
		Reason: "tier integrity is checked only on the orca-owned path",
	}
}

// foldPipelineTierIntegrity reduces the per-provider receipts to one verdict.
// Verified requires every provider to be verified and every receipt to be
// complete (REQ-005); anything else is unverified with each provider's reason
// named, so a reader can tell which provider degraded the run and why.
func foldPipelineTierIntegrity(receipts []execplane.IntegrityReceipt) (string, string) {
	if len(receipts) == 0 {
		return execplane.StatusUnverified, "no provider was checked"
	}
	status := execplane.StatusVerified
	reasons := make([]string, 0, len(receipts))
	for _, receipt := range receipts {
		reason := receipt.ResolutionReason
		if reason == "" {
			reason = "no reason recorded"
		}
		switch {
		case !receipt.Complete():
			status = execplane.StatusUnverified
			reason = "receipt is incomplete: " + reason
		case receipt.VerificationStatus != execplane.StatusVerified:
			status = execplane.StatusUnverified
		}
		reasons = append(reasons, receipt.Provider+": "+reason)
	}
	return status, strings.Join(reasons, "; ")
}

// persistPipelineTierIntegrityReceipt writes the gate receipt beside the
// execution owner receipt, with the same 0600 and schema-tagged treatment. It is
// a separate file because the owner receipt's field set is itself a rendered
// contract (templates/shared/omp-agent-pipeline.md.tmpl), so the integrity
// receipt cannot ride inside it without breaking that contract.
func persistPipelineTierIntegrityReceipt(specID string, receipt pipelineTierIntegrityReceipt) (string, error) {
	data, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode tier integrity receipt: %w", err)
	}
	data = append(data, '\n')
	if err := os.MkdirAll(pipelineStateDir, 0o700); err != nil {
		return "", fmt.Errorf("create tier integrity receipt directory: %w", err)
	}
	path := filepath.Join(pipelineStateDir, specID+".tier-integrity.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write tier integrity receipt: %w", err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", fmt.Errorf("secure tier integrity receipt: %w", err)
	}
	return filepath.ToSlash(path), nil
}

// tierRequestForProvider restates the tier contract J1 already decided for one
// provider. Both values come from the quality plane with no I/O, and neither is
// derived from an entitlement grade — the gate compares grades for equality and
// never maps one onto a model.
//
// The resolved model is the run's ceiling for that provider: the strongest model
// the requested mode grants. Verifying the ceiling covers every tier under it,
// and a pipeline run dispatches phases across the whole ladder.
func tierRequestForProvider(quality config.QualityConf, provider string) execplane.TierRequest {
	request := execplane.TierRequest{
		Provider:      provider,
		RequestedTier: quality.EffectiveMode(provider),
		// A provider this gate does not own holds nothing, and Evidence says so
		// rather than leaving a reader to infer it from a missing field.
		Evidence: execplane.EvidenceNone,
	}
	switch provider {
	case execplane.ProviderCodex:
		// The managed subprocess profile is what this mode pins codex to.
		// model/effort is the notation reportCodexRuntimeFallback already uses.
		profile := quality.CodexOrchestraProfile()
		request.ResolvedModel = profile.Model
		if profile.Effort != "" {
			request.ResolvedModel += "/" + profile.Effort
		}
		// The pipeline holds a codex catalog the provider itself served, and a
		// mismatched entitlement is answered by re-probing rather than by
		// weakening the claim.
		request.Evidence = execplane.EvidenceProbedCatalog
	case execplane.ProviderClaude:
		request.ResolvedModel = topClaudeModelForRun(quality)
		// No account-scoped catalog probe exists for claude, so parity carries
		// only the weaker claim that the executor runs on the same plan as the
		// reference session. The receipt names that weakness instead of letting
		// it read like a probed catalog.
		request.Evidence = execplane.EvidenceEntitlementParity
	}
	// An unknown provider also leaves the resolved model empty, which fails the
	// receipt's completeness check instead of guessing a model on its behalf.
	return request
}

// topClaudeModelForRun returns the strongest Claude slug the requested mode
// grants any role the pipeline dispatches. The descending order is expressed with the slugs
// config exports rather than restating the tier names it keys them by, so no
// second tier ladder is introduced here.
func topClaudeModelForRun(quality config.QualityConf) string {
	agents := config.RoleNames()
	for _, model := range []string{
		config.ClaudeFableModel, config.ClaudeOpusModel,
		config.ClaudeSonnetModel, config.ClaudeHaikuModel,
	} {
		for _, agent := range agents {
			if quality.ClaudeAgentModel(agent, "") == model {
				return model
			}
		}
	}
	// AgentTier's own fallback, for a preset that names no tier at all.
	return config.ClaudeModelForTier("")
}

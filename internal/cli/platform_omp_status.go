package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func runOMPPlatformStatus(
	cmd *cobra.Command,
	root string,
	jsonMode bool,
	deps ompPlatformDependencies,
) error {
	deps = normalizeOMPPlatformDependencies(deps)
	projection := buildOMPPlatformProjection(cmd.Context(), root, deps.newRunner(), deps.now())
	return renderOMPPlatformStatus(cmd, projection, jsonMode)
}

func renderOMPPlatformStatus(
	cmd *cobra.Command,
	projection ompPlatformProjection,
	jsonMode bool,
) error {
	if jsonMode {
		return writeJSONResult(
			cmd,
			ompProjectionEnvelopeStatus(projection.Status),
			projection,
			ompProjectionWarnings(projection.Blockers),
			ompProjectionChecks(projection),
		)
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "OMP operator status")
	_, _ = fmt.Fprintf(out, "Platform: configured=%t status=%s reason=%s\n", projection.Configured, projection.Status, projection.Reason)
	_, _ = fmt.Fprintf(
		out, "DAG: contract_owner=%s effective_owner=%s receipt=%s source=%s\n",
		projection.DAG.ContractOwner, projection.DAG.EffectiveOwner,
		projection.DAG.ReceiptStatus, projection.DAG.Source,
	)
	_, _ = fmt.Fprintf(
		out, "Models: enabled=%t profile=%s status=%s reason=%s catalog=%s trust=%s receipt=%s verified=%t\n",
		projection.Models.Enabled, valueOrDash(projection.Models.Profile), projection.Models.Status,
		projection.Models.Reason, projection.Models.CatalogReason, projection.Models.CatalogTrust,
		projection.Models.ReceiptStatus, projection.Models.ReceiptVerified,
	)
	_, _ = fmt.Fprintf(
		out, "Agent registry: status=%s reason=%s source=%s expected=%d shadowed=%d\n",
		projection.Models.AgentCatalogStatus, projection.Models.AgentCatalogReason,
		projection.Models.AgentCatalogSource, projection.Models.ExpectedAgents,
		projection.Models.ShadowedAgents,
	)
	for _, row := range projection.Models.Models {
		_, _ = fmt.Fprintf(
			out, "  %s role=%s capability=%s model_source=%s selector=%s model=%s/%s thinking=%s source=%s:%s status=%s reason=%s shadowed=%t fallback=%t verified=%t\n",
			row.Agent, row.Role, row.Capability, row.ModelSource, valueOrDash(row.EffectiveSelector),
			valueOrDash(row.Provider), valueOrDash(row.Model), valueOrDash(row.Thinking),
			row.Source, row.ConfigSource, row.Status, row.Reason, row.Shadowed,
			row.FallbackUsed, row.Verified,
		)
	}
	_, _ = fmt.Fprintf(
		out, "Context: enabled=%t profile=%s status=%s reason=%s history=%s->%s memory=%s->%s fallback=%s:%s promotion=%s\n",
		projection.Context.Enabled, valueOrDash(projection.Context.Profile), projection.Context.Status,
		projection.Context.Reason, valueOrDash(projection.Context.RequestedHistoryMode),
		valueOrDash(projection.Context.EffectiveHistoryMode), valueOrDash(projection.Context.RequestedMemoryMode),
		valueOrDash(projection.Context.EffectiveMemoryMode), valueOrDash(projection.Context.FallbackMode),
		valueOrDash(projection.Context.FallbackReason), projection.Context.PromotionFreshness,
	)
	_, _ = fmt.Fprintf(
		out, "Receipts: model=%s verified=%t context=%s/%s verified=%t\n",
		projection.ReceiptVerification.ModelStatus, projection.ReceiptVerification.ModelVerified,
		projection.ReceiptVerification.ContextStatus, projection.Context.ReceiptFreshness,
		projection.ReceiptVerification.ContextVerified,
	)
	_, _ = fmt.Fprintf(out, "Child runtime: %s source=%s reason=%s\n", projection.ChildRuntime.Status, projection.ChildRuntime.Source, projection.ChildRuntime.Reason)
	_, _ = fmt.Fprintln(out, projection.ChildRuntime.Limitation)
	_, _ = fmt.Fprintln(out, "Next: "+projection.ChildRuntime.NextCommand)
	renderOMPOperatorBlockers(out, projection.Blockers)
	return nil
}

func renderOMPExplain(
	cmd *cobra.Command,
	projection ompPlatformProjection,
	jsonMode bool,
) error {
	data := struct {
		Platform string                     `json:"platform"`
		Status   string                     `json:"status"`
		Reason   string                     `json:"reason"`
		Models   ompModelOperatorProjection `json:"models"`
		Blockers []string                   `json:"blockers"`
	}{
		Platform: projection.Platform, Status: projection.Status, Reason: projection.Reason,
		Models: projection.Models, Blockers: projection.Blockers,
	}
	if jsonMode {
		return writeJSONResult(
			cmd, ompProjectionEnvelopeStatus(projection.Status), data,
			ompProjectionWarnings(projection.Blockers), ompProjectionChecks(projection),
		)
	}
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "OMP model routing explanation")
	_, _ = fmt.Fprintf(
		out, "Profile: %s enabled=%t status=%s reason=%s\nCatalog: %s %s trust=%s\nReceipt: %s verified=%t\n",
		valueOrDash(projection.Models.Profile), projection.Models.Enabled,
		projection.Models.Status, projection.Models.Reason,
		valueOrDash(projection.Models.CatalogVersion), valueOrDash(projection.Models.CatalogFingerprint),
		projection.Models.CatalogTrust, projection.Models.ReceiptStatus, projection.Models.ReceiptVerified,
	)
	_, _ = fmt.Fprintf(
		out, "Agent registry: status=%s reason=%s source=%s expected=%d shadowed=%d\n",
		projection.Models.AgentCatalogStatus, projection.Models.AgentCatalogReason,
		projection.Models.AgentCatalogSource, projection.Models.ExpectedAgents,
		projection.Models.ShadowedAgents,
	)
	for _, row := range projection.Models.Models {
		_, _ = fmt.Fprintf(
			out, "- agent=%s capability=%s role=%s model_source=%s selector=%s provider=%s model=%s thinking=%s source=%s:%s status=%s reason=%s shadowed=%t verified=%t\n",
			row.Agent, row.Capability, row.Role, row.ModelSource, valueOrDash(row.EffectiveSelector),
			valueOrDash(row.Provider), valueOrDash(row.Model), valueOrDash(row.Thinking),
			row.Source, row.ConfigSource, row.Status, row.Reason, row.Shadowed, row.Verified,
		)
		for _, attempt := range row.FallbackAttempts {
			_, _ = fmt.Fprintf(
				out, "    fallback[%d]=%s status=%s reason=%s\n",
				attempt.Index, attempt.Selector, attempt.Status, attempt.Reason,
			)
		}
	}
	renderOMPOperatorBlockers(out, projection.Blockers)
	return nil
}

func renderOMPOperatorBlockers(out interface{ Write([]byte) (int, error) }, blockers []string) {
	if len(blockers) == 0 {
		_, _ = fmt.Fprintln(out, "Blockers: none")
		return
	}
	_, _ = fmt.Fprintln(out, "Blockers:")
	for _, blocker := range blockers {
		_, _ = fmt.Fprintln(out, "- "+blocker)
	}
}

func ompProjectionEnvelopeStatus(status string) jsonEnvelopeStatus {
	if status == "ready" {
		return jsonStatusOK
	}
	return jsonStatusWarn
}

func ompProjectionWarnings(blockers []string) []jsonMessage {
	warnings := make([]jsonMessage, 0, len(blockers))
	for _, blocker := range blockers {
		warnings = append(warnings, jsonMessage{Code: "omp_operator_blocker", Message: blocker})
	}
	return warnings
}

func ompProjectionChecks(projection ompPlatformProjection) []jsonCheck {
	checks := []jsonCheck{
		{ID: "omp.platform", Severity: projectionCheckSeverity(projection.Configured), Status: projectionCheckStatus(projection.Configured), Detail: projection.Reason},
		{ID: "omp.models", Severity: projectionStatusSeverity(projection.Models.Status), Status: projectionStatusCheck(projection.Models.Status), Detail: projection.Models.Reason},
		{
			ID: "omp.agent_registry", Severity: projectionStatusSeverity(projection.Models.AgentCatalogStatus),
			Status: projectionStatusCheck(projection.Models.AgentCatalogStatus), Detail: projection.Models.AgentCatalogReason,
		},
		{ID: "omp.context", Severity: projectionStatusSeverity(projection.Context.Status), Status: projectionStatusCheck(projection.Context.Status), Detail: projection.Context.Reason},
		receiptProjectionCheck("omp.receipt.models", projection.Models.Enabled, projection.Models.ReceiptVerified, projection.Models.ReceiptStatus),
		receiptProjectionCheck("omp.receipt.context", projection.Context.Enabled, projection.Context.ReceiptVerified, projection.Context.ReceiptStatus),
		{ID: "omp.child_runtime", Severity: "info", Status: "skip", Detail: "live_child_state_hub_only"},
	}
	return checks
}

func receiptProjectionCheck(id string, enabled, verified bool, detail string) jsonCheck {
	if !enabled {
		return jsonCheck{ID: id, Severity: "info", Status: "skip", Detail: detail}
	}
	return jsonCheck{
		ID: id, Severity: projectionCheckSeverity(verified),
		Status: projectionCheckStatus(verified), Detail: detail,
	}
}

func projectionCheckSeverity(pass bool) string {
	if pass {
		return "info"
	}
	return "warning"
}

func projectionCheckStatus(pass bool) string {
	if pass {
		return "pass"
	}
	return "fail"
}

func projectionStatusSeverity(status string) string {
	if status == "blocked" {
		return "error"
	}
	if status == "degraded" {
		return "warning"
	}
	return "info"
}

func projectionStatusCheck(status string) string {
	switch status {
	case "supported", "ready":
		return "pass"
	case "degraded":
		return "warn"
	case "disabled":
		return "skip"
	default:
		return "fail"
	}
}

func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" || value == "not_available" {
		return "-"
	}
	return value
}

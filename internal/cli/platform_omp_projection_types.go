package cli

import (
	"sort"
	"strings"
	"time"
)

const ompLiveStateLimitation = "Live child state is only available through the OMP hub; Autopus does not read user-owned OMP session roots."
const ompLiveStateNextCommand = "In the active OMP session, call hub with {\"op\":\"jobs\"}."

type ompFallbackProjection struct {
	Index    int    `json:"index"`
	Selector string `json:"selector"`
	Status   string `json:"status"`
	Reason   string `json:"reason"`
}

// ompEffectiveModelProjection is one bundled native OMP agent's model state.
// Agent is a native OMP name; Role and Capability record the logical role that
// supplied the route. ModelSource names the native config key that carries the
// selection, or "inherit" when OMP's own configuration governs it.
type ompEffectiveModelProjection struct {
	Agent             string                  `json:"agent"`
	Role              string                  `json:"role"`
	Capability        string                  `json:"capability"`
	ModelSource       string                  `json:"model_source"`
	EffectiveSelector string                  `json:"effective_selector"`
	Provider          string                  `json:"provider,omitempty"`
	Model             string                  `json:"model,omitempty"`
	Thinking          string                  `json:"thinking,omitempty"`
	Source            string                  `json:"source"`
	ConfigSource      string                  `json:"config_source"`
	Status            string                  `json:"status"`
	Reason            string                  `json:"reason"`
	Shadowed          bool                    `json:"shadowed"`
	Verified          bool                    `json:"verified"`
	FallbackUsed      bool                    `json:"fallback_used"`
	FallbackAttempts  []ompFallbackProjection `json:"fallback_attempts"`
}

type ompModelOperatorProjection struct {
	Enabled            bool                          `json:"enabled"`
	Profile            string                        `json:"profile,omitempty"`
	Status             string                        `json:"status"`
	Reason             string                        `json:"reason"`
	CatalogStatus      string                        `json:"catalog_status"`
	CatalogReason      string                        `json:"catalog_reason"`
	CatalogTrust       string                        `json:"catalog_trust"`
	CatalogVersion     string                        `json:"catalog_version,omitempty"`
	CatalogFingerprint string                        `json:"catalog_fingerprint,omitempty"`
	AgentCatalogStatus string                        `json:"agent_catalog_status"`
	AgentCatalogReason string                        `json:"agent_catalog_reason"`
	AgentCatalogSource string                        `json:"agent_catalog_source"`
	ExpectedAgents     int                           `json:"expected_agents"`
	ShadowedAgents     int                           `json:"shadowed_agents"`
	ReceiptStatus      string                        `json:"receipt_status"`
	ReceiptVerified    bool                          `json:"receipt_verified"`
	Models             []ompEffectiveModelProjection `json:"models"`
}

type ompContextOperatorProjection struct {
	Enabled              bool   `json:"enabled"`
	Profile              string `json:"profile,omitempty"`
	Status               string `json:"status"`
	Reason               string `json:"reason"`
	RequestedHistoryMode string `json:"requested_history_mode,omitempty"`
	EffectiveHistoryMode string `json:"effective_history_mode,omitempty"`
	RequestedMemoryMode  string `json:"requested_memory_mode,omitempty"`
	EffectiveMemoryMode  string `json:"effective_memory_mode,omitempty"`
	FallbackMode         string `json:"fallback_mode,omitempty"`
	FallbackReason       string `json:"fallback_reason,omitempty"`
	PromotionFreshness   string `json:"promotion_freshness"`
	ReceiptStatus        string `json:"receipt_status"`
	ReceiptFreshness     string `json:"receipt_freshness"`
	ReceiptVerified      bool   `json:"receipt_verified"`
	EvidenceSource       string `json:"evidence_source"`
}

type ompDAGOperatorProjection struct {
	ContractOwner  string `json:"contract_owner"`
	EffectiveOwner string `json:"effective_owner"`
	ReceiptStatus  string `json:"receipt_status"`
	Source         string `json:"source"`
	Reason         string `json:"reason"`
}

type ompChildRuntimeProjection struct {
	Status         string `json:"status"`
	Source         string `json:"source"`
	EvidenceSource string `json:"evidence_source"`
	Reason         string `json:"reason"`
	Limitation     string `json:"limitation"`
	NextCommand    string `json:"next_command"`
}

type ompReceiptVerificationProjection struct {
	ModelStatus     string `json:"model_status"`
	ModelVerified   bool   `json:"model_verified"`
	ModelReason     string `json:"model_reason"`
	ContextStatus   string `json:"context_status"`
	ContextVerified bool   `json:"context_verified"`
	ContextReason   string `json:"context_reason"`
}

type ompPlatformProjection struct {
	Platform            string                           `json:"platform"`
	Configured          bool                             `json:"configured"`
	Status              string                           `json:"status"`
	Reason              string                           `json:"reason"`
	DAG                 ompDAGOperatorProjection         `json:"dag"`
	Models              ompModelOperatorProjection       `json:"models"`
	Context             ompContextOperatorProjection     `json:"context"`
	ChildRuntime        ompChildRuntimeProjection        `json:"child_runtime"`
	ReceiptVerification ompReceiptVerificationProjection `json:"receipt_verification"`
	Blockers            []string                         `json:"blockers"`
}

func defaultOMPPlatformProjection() ompPlatformProjection {
	return ompPlatformProjection{
		Platform: "omp", Status: "blocked", Reason: "not_evaluated",
		DAG: ompDAGOperatorProjection{
			ContractOwner: "omp-local", EffectiveOwner: "unverified",
			ReceiptStatus: "hub_only", Source: "generated_omp_workflow_contract",
			Reason: "live_child_state_hub_only",
		},
		Models: ompModelOperatorProjection{
			Status: "disabled", Reason: "profile_not_selected", CatalogStatus: "not_probed",
			CatalogReason: "profile_not_selected", ReceiptStatus: "not_applicable",
			AgentCatalogStatus: "not_evaluated", AgentCatalogReason: "platform_not_evaluated",
			AgentCatalogSource: ompNativeAgentRegistrySource,
			Models:             []ompEffectiveModelProjection{},
		},
		Context: ompContextOperatorProjection{
			Status: "disabled", Reason: "profile_not_selected", PromotionFreshness: "not_applicable",
			ReceiptStatus: "not_applicable", ReceiptFreshness: "not_applicable", EvidenceSource: "not_available",
		},
		ChildRuntime: ompChildRuntimeProjection{
			Status: "unavailable", Source: "omp_hub", EvidenceSource: "not_available",
			Reason: "live_child_state_hub_only", Limitation: ompLiveStateLimitation,
			NextCommand: ompLiveStateNextCommand,
		},
		ReceiptVerification: ompReceiptVerificationProjection{
			ModelStatus: "not_applicable", ModelReason: "profile_not_selected",
			ContextStatus: "not_applicable", ContextReason: "profile_not_selected",
		},
		Blockers: []string{},
	}
}

func safeOMPOperatorToken(value string) string {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return "not_available"
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"authorization", "bearer ", "api_key", "api-key", "password", "secret", "access_token", "refresh_token"} {
		if strings.Contains(lower, marker) {
			return "redacted"
		}
	}
	for _, char := range value {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' ||
			strings.ContainsRune("._-/:=", char) {
			continue
		}
		return "redacted"
	}
	return value
}

func safeOMPOperatorReason(value string) string {
	return safeOMPOperatorToken(value)
}

func safeOMPOperatorVersion(value string) string {
	if !ompDoctorVersion.MatchString(value) {
		return "not_available"
	}
	return value
}

func appendUniqueOMPBlocker(blockers []string, value string) []string {
	value = safeOMPOperatorToken(value)
	if value == "not_available" || value == "redacted" {
		return blockers
	}
	for _, existing := range blockers {
		if existing == value {
			return blockers
		}
	}
	return append(blockers, value)
}

func sortOMPBlockers(blockers []string) []string {
	sort.Strings(blockers)
	if blockers == nil {
		return []string{}
	}
	return blockers
}

func ompPromotionFreshness(raw string, receiptFreshness string, now time.Time) string {
	if receiptFreshness != "fresh" {
		return receiptFreshness
	}
	if raw == "" {
		return "missing"
	}
	checkedAt, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || checkedAt.After(now.Add(5*time.Minute)) {
		return "invalid"
	}
	if now.Sub(checkedAt) > 24*time.Hour {
		return "stale"
	}
	return "fresh"
}

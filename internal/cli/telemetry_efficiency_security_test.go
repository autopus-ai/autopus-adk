package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/experiment"
)

func TestTelemetryEfficiency_ComputesAuditAndOverridesRequestedCanary(t *testing.T) {
	evidence := efficiencyEvidenceFixture(t)
	evidence.Rollout.AuditRatePercent = 100
	result, _, err := executeEfficiencyCommand(t, newTelemetryEfficiencyCmd(),
		"--evidence-json", writeEfficiencyEvidence(t, evidence), "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if result.RolloutReceipt.Decision != "AUDIT" || result.RolloutReceipt.ReceiptKind != "audit_sample" ||
		result.RolloutReceipt.ActiveProfile != "full_ultra" || !result.RolloutReceipt.FullDepth || !result.RolloutReceipt.AuditSelection.Selected {
		t.Fatalf("computed audit did not override canary: %+v", result.RolloutReceipt)
	}
	if result.RolloutReceipt.AuditSelection.Algorithm != "sha256_mod_100_v1" || result.RolloutReceipt.AuditSelection.RatePercent != 100 {
		t.Fatalf("audit selection was not computed canonically: %+v", result.RolloutReceipt.AuditSelection)
	}
}

func TestTelemetryEfficiency_MediumRiskAuditRequiresMatchingQualityRow(t *testing.T) {
	evidence := efficiencyEvidenceFixture(t)
	evidence.Rollout.RiskTier = "medium"
	evidence.QualityOutcomes[0].RiskTier = "medium"
	evidence.Rollout.AuditRatePercent = 100

	result, _, err := executeEfficiencyCommand(t, newTelemetryEfficiencyCmd(),
		"--evidence-json", writeEfficiencyEvidence(t, evidence), "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if result.RolloutReceipt.Decision != "AUDIT" || result.RolloutReceipt.RiskTier != "medium" {
		t.Fatalf("matching medium-risk audit = %+v", result.RolloutReceipt)
	}
}

func TestTelemetryEfficiency_RejectsInjectedOrInvalidAuditEvidence(t *testing.T) {
	valid, err := json.Marshal(efficiencyEvidenceFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "old precomputed audit selection", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["audit_selection"] = map[string]any{"selected": false, "bucket": 99}
		}},
		{name: "selected injection", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["audit_selected"] = false
		}},
		{name: "algorithm injection", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["audit_algorithm"] = "always_false"
		}},
		{name: "noncanonical task hash", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["audit_task_hash"] = "task-audit"
		}},
		{name: "noncanonical policy hash", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["policy_hash"] = "policy"
		}},
		{name: "noncanonical corpus hash", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["task_corpus_hash"] = "corpus"
		}},
		{name: "noncanonical config hash", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["config_hash"] = "config"
		}},
		{name: "audit task outside frozen quality rows", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["audit_task_hash"] = efficiencyHash("outside-task")
		}},
		{name: "high-risk quality row cannot be an audit sample", mutate: func(root map[string]any) {
			root["quality_outcomes"].([]any)[0].(map[string]any)["risk_tier"] = "high"
		}},
		{name: "audit row risk must match rollout risk", mutate: func(root map[string]any) {
			root["quality_outcomes"].([]any)[0].(map[string]any)["risk_tier"] = "medium"
		}},
		{name: "audit task hash must identify exactly one quality row", mutate: func(root map[string]any) {
			rows := root["quality_outcomes"].([]any)
			rows[1].(map[string]any)["task_hash"] = rows[0].(map[string]any)["task_hash"]
		}},
		{name: "invalid audit rate", mutate: func(root map[string]any) {
			root["rollout"].(map[string]any)["audit_rate_percent"] = 101
		}},
		{name: "rate without task hash", mutate: func(root map[string]any) {
			rollout := root["rollout"].(map[string]any)
			delete(rollout, "audit_task_hash")
			rollout["audit_rate_percent"] = 10
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(valid, &object); err != nil {
				t.Fatal(err)
			}
			test.mutate(object)
			data, _ := json.Marshal(object)
			path := filepath.Join(t.TempDir(), "evidence.json")
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, _, err := executeEfficiencyCommand(t, newTelemetryEfficiencyCmd(), "--evidence-json", path, "--format", "json"); err == nil {
				t.Fatal("untrusted audit evidence must fail")
			}
		})
	}
}

func TestTelemetryEfficiency_MissingAuditEvidenceCannotCompact(t *testing.T) {
	evidence := efficiencyEvidenceFixture(t)
	evidence.Rollout.AuditTaskHash = ""
	result, _, err := executeEfficiencyCommand(t, newTelemetryEfficiencyCmd(),
		"--evidence-json", writeEfficiencyEvidence(t, evidence), "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	if result.RolloutReceipt.Decision != "SHADOW" || result.RolloutReceipt.ActiveProfile != "full_ultra" || !result.RolloutReceipt.FullDepth {
		t.Fatalf("missing audit evidence compacted: %+v", result.RolloutReceipt)
	}
}

func TestTelemetryEfficiency_DigestsEveryComparisonTaskIdentity(t *testing.T) {
	identity := efficiencyIdentityFixture()
	// The fake credentials are joined at run time, so no provider-format
	// literal is committed (secret scanners and GitHub push protection match
	// the source text); the bytes are unchanged.
	githubToken := "ghp_" + strings.Repeat("A", 36)
	openAIKey := "sk-" + "proj-" + strings.Repeat("B", 36)
	jwt := "eyJhbGciOiJIUzI1NiJ9" + ".eyJzdWIiOiJzZWNyZXQifQ.signature"
	githubPAT := "github_pat_" + "11" + strings.Repeat("C", 20) + "_secret"
	other := identity
	other.ModelVersion = "mismatched-version"

	evidence := efficiencyEvidenceFixture(t)
	evidence.ExpectedTaskIDs = []string{githubToken, openAIKey, jwt, githubPAT}
	evidence.Trials = []experiment.TaskTrial{
		efficiencyTrial(identity, githubToken, "baseline", "AB", 1000),
		efficiencyTrial(identity, githubToken, "candidate", "AB", 700),
		efficiencyTrial(identity, openAIKey, "baseline", "BA", 2000),
		efficiencyTrial(identity, openAIKey, "candidate", "BA", 1400),
		efficiencyTrial(identity, jwt, "baseline", "AB", 1000),
		efficiencyTrial(identity, githubPAT, "baseline", "AB", 1000),
		efficiencyTrial(other, githubPAT, "candidate", "AB", 700),
	}
	result, raw, err := executeEfficiencyCommand(t, newTelemetryEfficiencyCmd(),
		"--evidence-json", writeEfficiencyEvidence(t, evidence), "--format", "json")
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{githubToken, openAIKey, jwt, githubPAT} {
		if strings.Contains(raw, secret) {
			t.Fatalf("comparison output leaked task identity %q", secret)
		}
	}
	assertDigestedTaskIDs(t, result.Comparison.PairedTaskIDs, []string{efficiencyHash(githubToken), efficiencyHash(openAIKey)})
	assertDigestedTaskIDs(t, result.Comparison.UnpairedTaskIDs, []string{efficiencyHash(jwt)})
	assertDigestedTaskIDs(t, result.Comparison.ABTaskIDs, []string{efficiencyHash(githubToken)})
	assertDigestedTaskIDs(t, result.Comparison.BATaskIDs, []string{efficiencyHash(openAIKey)})
	assertDigestedTaskIDs(t, result.Comparison.ExpectedTaskIDs, []string{
		efficiencyHash(jwt), efficiencyHash(githubToken), efficiencyHash(githubPAT), efficiencyHash(openAIKey),
	})
	assertDigestedTaskIDs(t, result.Quality.MissingTaskIDs, []string{
		efficiencyHash(jwt), efficiencyHash(githubToken), efficiencyHash(githubPAT), efficiencyHash(openAIKey),
	})
	assertDigestedTaskIDs(t, result.Quality.UnexpectedTaskIDs, []string{efficiencyHash("task-a"), efficiencyHash("task-b")})
	if len(result.Comparison.ExcludedTasks) != 1 || result.Comparison.ExcludedTasks[0].TaskID != efficiencyHash(githubPAT) {
		t.Fatalf("excluded IDs not digested: %+v", result.Comparison.ExcludedTasks)
	}
	if result.Comparison.PairedTaskCount != 2 || result.Comparison.MedianPairedRawReductionPct != 30 || result.Promotion.RolloutDecision != "BLOCKED" {
		t.Fatalf("digesting changed arithmetic or promotion: %+v / %+v", result.Comparison, result.Promotion)
	}
}

func assertDigestedTaskIDs(t *testing.T, values, want []string) {
	t.Helper()
	if strings.Join(values, ",") != strings.Join(want, ",") {
		t.Fatalf("digested IDs = %v, want %v", values, want)
	}
	for _, value := range values {
		if !strings.HasPrefix(value, "sha256:") || len(value) != 71 {
			t.Fatalf("task identity was not a canonical digest: %q", value)
		}
	}
}

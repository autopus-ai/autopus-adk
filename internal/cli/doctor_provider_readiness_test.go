package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// doctorReadinessConfig is an OMP-backed claude, a CLI codex, and a CLI
// gemini (agy) as review gate, with the given judge.
func doctorReadinessConfig(judge string, reviewers ...string) *config.HarnessConfig {
	cfg := config.DefaultFullConfig("doctor-readiness")
	cfg.Spec.ReviewGate.Providers = reviewers
	cfg.Spec.ReviewGate.Judge = judge
	cfg.Orchestra.Providers["claude"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: ompAnthropicModel}
	return cfg
}

// countSmokeFactoryCalls proves no model-calling backend is built.
func countSmokeFactoryCalls(t *testing.T) *int {
	t.Helper()
	calls := 0
	original := providerSmokeBackendFactory
	providerSmokeBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		calls++
		return fakeProviderSmokeBackend{}
	}
	t.Cleanup(func() { providerSmokeBackendFactory = original })
	return &calls
}

// S15 (REQ-14): doctor reports one readiness check per distinct review-gate
// provider and the judge, in JSON and text, without a model call.
func TestDoctorProviderReadiness_ReportsJSONAndTextChecks(t *testing.T) {
	installFakeOMP(t)
	clearProviderCredentialEnv(t)
	smokeCalls := countSmokeFactoryCalls(t)
	useReadinessRunner(t, replyByExecutable(map[string]readinessReply{
		"omp":   replyWith(0, ompUsageJSON("[]", "[]", `[{"provider":"anthropic","account":"c3d4","reason":"Refresh token expired"}]`), ""),
		"codex": replyWith(0, "", "Logged in using ChatGPT\n"),
	}))
	cfg := doctorReadinessConfig("claude", "claude", "codex", "gemini")

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectProviderReadinessChecks(context.Background(), cfg)
	report.collectProviderTransportSmokeChecks(cfg, doctorOptions{})
	var text bytes.Buffer
	textOK := checkProviderReadinessText(context.Background(), &text, cfg)

	assert.Equal(t, []jsonCheck{
		{ID: "doctor.provider_readiness.claude", Severity: "error", Status: "fail", Detail: `not_ready(auth_expired): run "omp login anthropic"`},
		{ID: "doctor.provider_readiness.codex", Severity: "info", Status: "pass", Detail: "ready"},
		{ID: "doctor.provider_readiness.gemini", Severity: "info", Status: "skip", Detail: "unknown(no_status_command)"},
		{ID: "doctor.provider_transport.smoke", Severity: "info", Status: "skip", Detail: "provider transport smoke skipped; run 'auto doctor --provider-smoke'"},
	}, report.checks)
	assert.Equal(t, jsonStatusWarn, report.status)
	assert.Equal(t, []jsonMessage{{Code: "provider_unready",
		Message: `claude provider is not ready: not_ready(auth_expired); run "omp login anthropic"`}}, report.warnings)
	assert.False(t, textOK)
	lines := strings.Split(text.String(), "\n")
	assert.Contains(t, text.String(), "Provider Readiness")
	assert.Contains(t, lines, `  [ERROR] claude readiness: not_ready(auth_expired) - run "omp login anthropic"`)
	assert.Contains(t, lines, "  [OK] codex readiness: ready")
	assert.Contains(t, lines, "  [WARN] gemini readiness: unknown(no_status_command)")
	assert.Contains(t, text.String(), providerReadinessAdvice)
	assert.Zero(t, *smokeCalls)
}

// S15: judge codex with reviewer claude yields exactly those two checks.
func TestDoctorProviderReadiness_ChecksDistinctReviewersAndJudge(t *testing.T) {
	installFakeOMP(t)
	clearProviderCredentialEnv(t)
	spy := useReadinessRunner(t, replyByExecutable(map[string]readinessReply{
		"omp":   replyWith(0, ompUsageJSON(`[{"provider":"anthropic","account":"a1b2"}]`, "[]", "[]"), ""),
		"codex": replyWith(0, "", "Logged in using ChatGPT\n"),
	}))

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectProviderReadinessChecks(context.Background(), doctorReadinessConfig("codex", "claude"))

	ids := make([]string, 0, len(report.checks))
	for _, check := range report.checks {
		ids = append(ids, check.ID+"="+check.Status)
	}
	assert.Equal(t, []string{"doctor.provider_readiness.claude=pass", "doctor.provider_readiness.codex=pass"}, ids)
	assert.Equal(t, jsonStatusOK, report.status)
	assert.Empty(t, report.warnings)
	assert.Len(t, spy.argvs(), 2)

	var text bytes.Buffer
	assert.True(t, checkProviderReadinessText(context.Background(), &text, nil), "no config means nothing to probe")
	empty := doctorJSONReport{status: jsonStatusOK}
	empty.collectProviderReadinessChecks(context.Background(), nil)
	assert.Empty(t, empty.checks)
}

// S16 (REQ-15): doctor output never carries account identifiers or tokens
// from probe output; an OMP account warning is reported redacted.
func TestDoctorProviderReadiness_NeverPrintsAccountIdentifiers(t *testing.T) {
	installFakeOMP(t)
	clearProviderCredentialEnv(t)
	useReadinessRunner(t, replyByExecutable(map[string]readinessReply{
		"claude": replyWith(0, claudeLoggedInR1, ""),
		"omp": replyWith(0, ompUsageJSON(`[{"provider":"anthropic","account":"dev@example.com"}]`, "[]",
			`[{"provider":"anthropic","account":"sk-ant-oat01-abc123","reason":"Refresh token expired for dev@example.com"}]`), ""),
	}))
	cfg := config.DefaultFullConfig("doctor-readiness")
	cfg.Spec.ReviewGate.Providers = []string{"claude", "gemini"}
	cfg.Spec.ReviewGate.Judge = ""
	cfg.Orchestra.Providers["gemini"] = config.ProviderEntry{Backend: config.ProviderBackendOMP, Model: ompAnthropicModel}

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectProviderReadinessChecks(context.Background(), cfg)
	var text bytes.Buffer
	checkProviderReadinessText(context.Background(), &text, cfg)

	encoded, err := json.Marshal(struct {
		Checks   []jsonCheck   `json:"checks"`
		Warnings []jsonMessage `json:"warnings"`
	}{report.checks, report.warnings})
	require.NoError(t, err)
	require.Len(t, report.checks, 2)
	warning := `omp anthropic: 1 of 2 accounts unusable (auth_expired); run "omp usage --redact" for details`
	assert.Equal(t, map[string]string{"warning": warning}, report.checks[1].Fields)
	assert.Contains(t, text.String(), warning)
	for _, secret := range []string{"dev@example.com", "org-123", "sk-ant-"} {
		assert.NotContains(t, string(encoded), secret)
		assert.NotContains(t, text.String(), secret)
	}
}

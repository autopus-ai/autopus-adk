package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// probeOMPFamily classifies one OMP-backed provider of family against usage.
func probeOMPFamily(t *testing.T, usage, family string) providerReadinessResult {
	t.Helper()
	installFakeOMP(t)
	installReadinessRunner(t, replyWith(0, usage, ""))
	provider := orchestra.ProviderConfig{Name: "claude", Backend: "omp", Model: family + "/model"}
	return probeProviderReadiness(context.Background(), []orchestra.ProviderConfig{provider},
		providerReadinessOptions{Env: []string{"HOME=/a"}})[0]
}

// assertOMPDisabledAccountsDetected is the CD-5 oracle: each family that owns a
// disabled credential surfaces as not_ready, or as ready with an unusable-account warning.
func assertOMPDisabledAccountsDetected(t *testing.T, usage string) {
	t.Helper()
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(usage), &top), "CD-5: the capture is a JSON object")
	var disabled []map[string]any
	require.NoError(t, json.Unmarshal(top["disabledCredentials"], &disabled), "CD-5: disabledCredentials holds objects")
	families := map[string]bool{}
	for _, element := range disabled {
		provider, providerIsString := element["provider"].(string)
		_, causeIsString := element["cause"].(string)
		_, reasonIsString := element["reason"].(string)
		require.True(t, providerIsString && (causeIsString || reasonIsString),
			"CD-5: disabled credentials carry a string provider and a string cause or reason")
		families[provider] = true
	}
	require.NotEmpty(t, families, "CD-5: the capture holds a disabled or expired account")
	for family := range families {
		result := probeOMPFamily(t, usage, family)
		switch result.Status {
		case providerReadinessNotReady:
			assert.Contains(t, []string{"auth_expired", "account_disabled"}, result.Reason)
			assert.Equal(t, "omp login "+family, result.Remedy)
		case providerReadinessReady:
			require.Len(t, result.Warnings, 1)
			assert.True(t, strings.HasPrefix(result.Warnings[0], "omp "+family+": "), result.Warnings[0])
		default:
			t.Errorf("CD-5: disabled %s account is not detected: %s", family, result.Token())
		}
	}
}

func TestProbeProviderReadiness_OMPAssumedDisabledShape_SatisfiesCaptureOracle(t *testing.T) {
	assertOMPDisabledAccountsDetected(t, readReadinessFixture(t, "omp_usage_disabled_assumed.json"))
}

// withoutOMPReports drops every report of family from a usage document.
func withoutOMPReports(t *testing.T, usage, family string) string {
	t.Helper()
	var top map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(usage), &top))
	var reports []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(top["reports"], &reports))
	kept := []map[string]json.RawMessage{}
	for _, report := range reports {
		if provider, _ := readinessJSONString(report["provider"]); provider != family {
			kept = append(kept, report)
		}
	}
	encoded, err := json.Marshal(kept)
	require.NoError(t, err)
	top["reports"] = encoded
	data, err := json.Marshal(top)
	require.NoError(t, err)
	return string(data)
}

// CD-5: the redacted real `omp usage --json --redact` capture holds one usable
// and one expired anthropic credential, plus usable codex and antigravity
// reports. omp names a disabled credential's failure "cause", not "reason".
func TestProbeProviderReadiness_OMPCapturedDisabledAccount_IsDetected(t *testing.T) {
	capture := readReadinessFixture(t, "omp_usage_disabled_capture.json")
	assertOMPDisabledAccountsDetected(t, capture)

	// A usable account wins, so anthropic is ready and warns about the expired one.
	anthropic := probeOMPFamily(t, capture, "anthropic")
	assert.Equal(t, "ready", anthropic.Token())
	assert.Equal(t, []string{`omp anthropic: 1 of 2 accounts unusable (auth_expired); run "omp usage --redact" for details`},
		anthropic.Warnings)
	for _, family := range []string{"openai-codex", "google-antigravity"} {
		result := probeOMPFamily(t, capture, family)
		assert.Equal(t, "ready", result.Token(), family)
		assert.Empty(t, result.Warnings, family)
	}

	// Without the usable account (the incident), the same real credential is expired.
	expiredOnly := probeOMPFamily(t, withoutOMPReports(t, capture, "anthropic"), "anthropic")
	assert.Equal(t, "not_ready(auth_expired)", expiredOnly.Token())
	assert.Equal(t, `run "omp login anthropic"`, expiredOnly.RunRemedy())
}

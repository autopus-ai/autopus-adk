package omp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestProbeOMPModelCatalogForProfile_MetadataInsufficiencyUsesOperatorAttestation(t *testing.T) {
	t.Parallel()

	profile := integrationHarnessConfig(config.RoleModelConfigModeOverlay).RoleModelPolicy.Profiles["p1"]
	profile.CatalogTrust = config.RoleModelCatalogTrustOperatorAttested
	runner := &modelCatalogFakeRunner{outputs: map[string][]byte{
		"--version":                     []byte("omp/17.2.6\n"),
		"models --json --no-extensions": operatorAttestedAvailableCatalogJSON(),
	}, errors: map[string]error{}}

	got := ProbeOMPModelCatalogForProfile(context.Background(), OMPModelCatalogProbeOptions{
		Runner: runner, Timeout: time.Second, MaxOutput: 4096,
	}, profile)

	require.Equal(t, "ready", got.Status)
	require.Equal(t, "catalog_ready", got.Reason)
	require.Equal(t, config.RoleModelCatalogTrustOperatorAttested, got.CatalogTrust)
	require.Len(t, got.Catalog.Models, 3)
	for _, model := range got.Catalog.Models {
		require.True(t, model.OperatorAttested, model.Provider+"/"+model.Model)
		require.False(t, model.AuthEnabled, model.Provider+"/"+model.Model)
		require.False(t, model.Keyless, model.Provider+"/"+model.Model)
	}

	resolution := ResolveOMPModelRoute(got.Catalog, got.Reason, OMPModelRouteRequest{
		Agent: "executor", Role: config.OMPAgentRoleName("executor"), Capability: config.CapabilityCodingToolUse,
		Candidates: []OMPRoutingCandidate{{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"}},
		Required:   true,
	})
	require.Equal(t, "selected", resolution.Status)
	require.Equal(t, "openai/beta-coder:high", resolution.EffectiveSelector)
	require.Equal(t, "openai", resolution.EffectiveFamily)
	require.Equal(t, "operator_attested", resolution.EvidenceClass)
	require.Equal(t, []string{"--version", "models --json --no-extensions"}, runner.calls)
	assertNoSensitiveOMPProbeArguments(t, runner.calls)
}

// An operator may pin a bundled agent by its native name. Reading the
// capability from the ADK role matrix alone rejected that key, so a metadata
// light installation refused every native-keyed pin with catalog_invalid.
func TestProbeOMPModelCatalogForProfile_AttestsNativeKeyedAgentOverride(t *testing.T) {
	t.Parallel()

	profile := integrationHarnessConfig(config.RoleModelConfigModeOverlay).RoleModelPolicy.Profiles["p1"]
	profile.CatalogTrust = config.RoleModelCatalogTrustOperatorAttested
	profile.Agents = map[string]config.RoleAgentOverrideConf{
		"task": {Candidates: []config.RoleModelCandidateConf{
			{Selector: "openai/beta-coder", Thinking: "high", Family: "openai"},
		}},
	}
	runner := &modelCatalogFakeRunner{outputs: map[string][]byte{
		"--version":                     []byte("omp/17.2.6\n"),
		"models --json --no-extensions": operatorAttestedAvailableCatalogJSON(),
	}, errors: map[string]error{}}

	got := ProbeOMPModelCatalogForProfile(context.Background(), OMPModelCatalogProbeOptions{
		Runner: runner, Timeout: time.Second, MaxOutput: 4096,
	}, profile)

	require.Equal(t, "catalog_ready", got.Reason)
	require.Equal(t, config.RoleModelCatalogTrustOperatorAttested, got.CatalogTrust)
	require.NotEmpty(t, got.Catalog.Models)
}

func TestProbeOMPModelCatalogForProfile_OperatorAttestationDoesNotOverrideOtherFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		version []byte
		catalog []byte
		err     error
		max     int
		want    string
	}{
		{name: "identity", version: []byte("other/17.2.6"), catalog: operatorAttestedAvailableCatalogJSON(), max: 4096, want: "identity_unverified"},
		{name: "invalid", version: []byte("omp/17.2.6"), catalog: []byte(`{"models":[`), max: 4096, want: "catalog_invalid"},
		{name: "timeout", version: []byte("omp/17.2.6"), err: context.DeadlineExceeded, max: 4096, want: "catalog_timeout"},
		{name: "oversized", version: []byte("omp/17.2.6"), catalog: []byte(strings.Repeat("x", 65)), max: 64, want: "catalog_oversized"},
		{name: "empty", version: []byte("omp/17.2.6"), catalog: []byte(`{"models":[]}`), max: 4096, want: "catalog_empty"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			profile := integrationHarnessConfig(config.RoleModelConfigModeOverlay).RoleModelPolicy.Profiles["p1"]
			profile.CatalogTrust = config.RoleModelCatalogTrustOperatorAttested
			runner := &modelCatalogFakeRunner{outputs: map[string][]byte{
				"--version": test.version, "models --json --no-extensions": test.catalog,
			}, errors: map[string]error{"models --json --no-extensions": test.err}}

			got := ProbeOMPModelCatalogForProfile(context.Background(), OMPModelCatalogProbeOptions{
				Runner: runner, Timeout: time.Second, MaxOutput: test.max,
			}, profile)

			require.Equal(t, "blocked", got.Status)
			require.Equal(t, test.want, got.Reason)
			require.Empty(t, got.Catalog.Models)
			assertNoSensitiveOMPProbeArguments(t, runner.calls)
		})
	}
}

func TestOMPModelIntegration_OperatorAttestationIsBoundIntoReceipt(t *testing.T) {
	t.Parallel()

	cfg := integrationHarnessConfig(config.RoleModelConfigModeOverlay)
	profile := cfg.RoleModelPolicy.Profiles["p1"]
	profile.CatalogTrust = config.RoleModelCatalogTrustOperatorAttested
	cfg.RoleModelPolicy.Profiles["p1"] = profile
	runner := &modelIntegrationFakeRunner{catalog: operatorAttestedAvailableCatalogJSON()}
	files, err := NewWithRoot(t.TempDir()).WithModelIntegrationRunner(runner).
		WithModelIntegrationClock(func() time.Time { return time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC) }).
		prepareFiles(context.Background(), cfg)
	require.NoError(t, err)
	require.Zero(t, runner.modelRequests)

	mapping, ok := integrationMappingsByPath(files)[OMPModelReceiptRelativePath]
	require.True(t, ok, "model receipt mapping")
	var receipt OMPModelResolutionReceipt
	require.NoError(t, json.Unmarshal(mapping.Content, &receipt))
	require.Equal(t, config.RoleModelCatalogTrustOperatorAttested, receipt.CatalogTrust)
	require.Len(t, receipt.Roles, len(config.OMPNativeAgentNames()))
	for _, role := range receipt.Roles {
		require.Equal(t, "operator_attested", role.EvidenceClass, role.Agent)
		require.NotEmpty(t, role.EffectiveFamily, role.Agent)
	}

	calls := make([]string, 0, len(runner.calls))
	for _, call := range runner.calls {
		calls = append(calls, strings.Join(call, " "))
	}
	assertNoSensitiveOMPProbeArguments(t, calls)
}

func TestCheckOMPModelRoutingDoctor_OperatorAttestedBindsTrustEvidenceAndFamily(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	receipt := modelReceiptFixture(time.Date(2026, 8, 30, 4, 5, 6, 0, time.UTC))
	receipt.CatalogTrust = config.RoleModelCatalogTrustOperatorAttested
	for index := range receipt.Roles {
		receipt.Roles[index].EvidenceClass = "operator_attested"
		receipt.Roles[index].EffectiveFamily = receipt.Roles[index].Provider
	}
	_, err := WriteOMPModelResolutionReceipt(OMPModelReceiptWriteInput{
		WorkspaceRoot: root,
		Receipt:       receipt,
	})
	require.NoError(t, err)

	fresh := operatorAttestedDoctorInput(root)
	report := CheckOMPModelRoutingDoctor(fresh)
	require.Equal(t, "supported", report.Status)
	require.Equal(t, "fresh", report.Reason)
	for _, role := range report.Roles {
		require.Equal(t, "operator_attested", role.EvidenceClass)
	}

	tests := []struct {
		name   string
		mutate func(*OMPModelDoctorInput)
	}{
		{"catalog trust", func(input *OMPModelDoctorInput) {
			input.Probe.CatalogTrust = config.RoleModelCatalogTrustStrict
		}},
		{"role evidence", func(input *OMPModelDoctorInput) {
			input.Compilation.Resolutions[0].EvidenceClass = "availability"
		}},
		{"effective family", func(input *OMPModelDoctorInput) {
			input.Compilation.Resolutions[0].EffectiveFamily = "other"
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			input := operatorAttestedDoctorInput(root)
			test.mutate(&input)
			got := CheckOMPModelRoutingDoctor(input)
			require.Equal(t, "blocked", got.Status)
			require.Equal(t, "projection_mismatch", got.Reason)
		})
	}
}

func operatorAttestedDoctorInput(root string) OMPModelDoctorInput {
	input := modelDoctorInput(root)
	input.Probe.CatalogTrust = config.RoleModelCatalogTrustOperatorAttested
	for index := range input.Compilation.Resolutions {
		resolution := &input.Compilation.Resolutions[index]
		resolution.EvidenceClass = "operator_attested"
		resolution.EffectiveFamily = resolution.EffectiveProvider
	}
	return input
}

// TestOMPOperatorAttestedDeclarations_IncludeAgentOverrideCandidates proves a
// selector declared only by an agent override still receives attested
// metadata for that agent's capability, and keeps the closed-route rules.
func TestOMPOperatorAttestedDeclarations_IncludeAgentOverrideCandidates(t *testing.T) {
	t.Parallel()

	profile := integrationHarnessConfig(config.RoleModelConfigModeOverlay).RoleModelPolicy.Profiles["p1"]
	profile.Agents = map[string]config.RoleAgentOverrideConf{
		"executor": {Candidates: []config.RoleModelCandidateConf{
			{Selector: "openai/delta-coder", Thinking: "medium", Family: "openai"},
		}},
	}
	declarations, ok := ompOperatorAttestedDeclarations(profile)
	require.True(t, ok)
	delta, declared := declarations["openai/delta-coder"]
	require.True(t, declared)
	require.Equal(t, "openai", delta.family)
	require.Equal(t, []string{config.CapabilityCodingToolUse}, sortedOMPDeclarationValues(delta.capabilities))
	require.Equal(t, []string{"medium"}, sortedOMPDeclarationValues(delta.thinking))

	profile.Agents["executor"] = config.RoleAgentOverrideConf{Candidates: []config.RoleModelCandidateConf{
		{Selector: "openai/delta-coder", Thinking: "medium"},
	}}
	_, ok = ompOperatorAttestedDeclarations(profile)
	require.False(t, ok, "override candidates without a family must not be attested")

	profile.Agents = map[string]config.RoleAgentOverrideConf{
		"future-agent": {Candidates: []config.RoleModelCandidateConf{
			{Selector: "openai/delta-coder", Thinking: "medium", Family: "openai"},
		}},
	}
	_, ok = ompOperatorAttestedDeclarations(profile)
	require.False(t, ok, "unmapped agent overrides must not be attested")
}

func operatorAttestedAvailableCatalogJSON() []byte {
	return []byte(`{"models":[
		{"provider":"anthropic","id":"alpha-reasoner","available":true},
		{"provider":"openai","id":"beta-coder","available":true},
		{"provider":"google","id":"gamma-vision","available":true}
	]}`)
}

func assertNoSensitiveOMPProbeArguments(t *testing.T, calls []string) {
	t.Helper()
	for _, call := range calls {
		lower := strings.ToLower(call)
		for _, forbidden := range []string{" prompt", " agent", "credential", "api-key", "api_key", "token", "auth"} {
			require.NotContains(t, lower, forbidden, call)
		}
	}
}

package cli

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// bandSecretEnv are inherited credentials no read-only diagnosis needs.
var bandSecretEnv = []string{
	"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN",
	"GOOGLE_APPLICATION_CREDENTIALS", "AZURE_CLIENT_SECRET",
}

// Security M2: the diagnosis provider starts without GitHub tokens and cloud
// credentials, so a provider steered by injected evidence cannot reach them,
// while the rest of the environment stays.
func TestReactBandDiagnose_ProviderStartsWithoutTokensOrCloudCredentials(t *testing.T) {
	fakes := installBandFakeProviders(t, "codex")
	for _, key := range bandSecretEnv {
		t.Setenv(key, "synthetic-"+strings.ToLower(key))
	}
	t.Setenv("BAND_KEEP", "kept")
	fixture := newBandDiagnoseFixture(t)
	codex := bandHarness("codex", "", map[string]config.ProviderEntry{"codex": config.CodexProviderEntryForQuality(config.QualityConf{})})

	require.Equal(t, "ok", fixture.diagnoser(codex, nil).Run(context.Background(), fixture.claim).DiagnosisStatus)

	env := strings.Split(fakes.record(t, "codex", "env"), "\n")
	assert.Contains(t, env, "BAND_KEEP=kept")
	for _, key := range bandSecretEnv {
		for _, entry := range env {
			assert.False(t, strings.HasPrefix(entry, key+"="), "%s reached the provider", key)
		}
	}
}

// The OMP route gets the same list: the request carries it, and the OMP
// review session builds its environment without those variables.
func TestReactBandDiagnose_OMPRouteDropsTheSameVariables(t *testing.T) {
	fixture := newBandDiagnoseFixture(t)
	backend := &bandFakeBackend{response: orchestra.ProviderResponse{Output: "routed diagnosis"}}
	d := fixture.diagnoser(bandHarness("claude", "", map[string]config.ProviderEntry{"claude": bandOMPClaude()}), nil)
	d.backends = func(orchestra.OrchestraConfig) map[string]orchestra.ExecutionBackend {
		return map[string]orchestra.ExecutionBackend{config.ProviderBackendOMP: backend}
	}
	require.Equal(t, "ok", d.Run(context.Background(), fixture.claim).DiagnosisStatus)
	require.Len(t, backend.calls(), 1)
	assert.Subset(t, backend.calls()[0].Config.UnsetEnv, bandSecretEnv)

	t.Setenv("GH_ENTERPRISE_TOKEN", "synthetic")
	t.Setenv("BAND_KEEP", "kept")
	env, err := ompReviewEnvironment(backend.calls()[0].Config.UnsetEnv)
	require.NoError(t, err)
	assert.Contains(t, env, "BAND_KEEP=kept")
	assert.NotContains(t, strings.Join(env, "\n"), "GH_ENTERPRISE_TOKEN=")
	all, err := ompReviewEnvironment(nil)
	require.NoError(t, err)
	assert.Contains(t, all, "GH_ENTERPRISE_TOKEN=synthetic", "other OMP reviews keep their environment")
	assert.NotEmpty(t, os.Getenv("GH_ENTERPRISE_TOKEN"), "the band process itself is unchanged")
}

// Security L2: band bounds the provider capture while the provider runs, one
// byte past its 1 MiB head so the head capture still sees a drop.
func TestReactBandDiagnose_ProviderCaptureIsBoundedWhileItRuns(t *testing.T) {
	t.Parallel()
	fixture := newBandDiagnoseFixture(t)
	d := fixture.diagnoser(bandHarness("codex", "", map[string]config.ProviderEntry{"codex": config.CodexProviderEntryForQuality(config.QualityConf{})}), nil)
	var limit int
	d.run = func(_ context.Context, _ orchestra.OrchestraConfig, provider orchestra.ProviderConfig, _ string) (*orchestra.ProviderResponse, error) {
		limit = provider.MaxOutputBytes
		return &orchestra.ProviderResponse{Output: strings.Repeat("x", healthband.ProviderCaptureBytes+1)}, nil
	}
	outcome := d.Run(context.Background(), fixture.claim)
	assert.Equal(t, "ok", outcome.DiagnosisStatus)
	assert.Equal(t, healthband.ProviderCaptureBytes+1, limit)
}

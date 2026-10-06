package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

type fakeProviderSmokeBackend struct {
	responses map[string]*orchestra.ProviderResponse
	errors    map[string]error
}

func (f fakeProviderSmokeBackend) Execute(_ context.Context, req orchestra.ProviderRequest) (*orchestra.ProviderResponse, error) {
	if err := f.errors[req.Provider]; err != nil {
		return f.responses[req.Provider], err
	}
	return f.responses[req.Provider], nil
}

func (f fakeProviderSmokeBackend) Name() string {
	return "fake-provider-smoke"
}

func TestRunProviderTransportSmoke_ClassifiesProviderResults(t *testing.T) {
	dir := t.TempDir()
	setFakeProviderOnPath(t, dir, "claude")
	setFakeProviderOnPath(t, dir, "codex")
	setFakeProviderOnPath(t, dir, "agy")

	cfg := config.DefaultFullConfig("provider-smoke")
	cfg.Spec.ReviewGate.Providers = []string{"claude", "codex", "gemini"}
	cfg.Orchestra.Providers["claude"] = config.ProviderEntry{Binary: "claude"}
	cfg.Orchestra.Providers["codex"] = config.ProviderEntry{Binary: "codex"}
	cfg.Orchestra.Providers["gemini"] = config.ProviderEntry{Binary: "agy"}

	origFactory := providerSmokeBackendFactory
	providerSmokeBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		return fakeProviderSmokeBackend{
			responses: map[string]*orchestra.ProviderResponse{
				"claude": {Provider: "claude", Output: providerSmokeMarker},
				"codex":  {Provider: "codex", Output: "", EmptyOutput: true},
				"gemini": {Provider: "gemini", Output: "different output"},
			},
		}
	}
	defer func() { providerSmokeBackendFactory = origFactory }()

	results := runProviderTransportSmoke(context.Background(), cfg, time.Second)

	require.Len(t, results, 3)
	assert.Equal(t, "pass", results[0].Status)
	assert.Equal(t, "fail", results[1].Status)
	assert.Equal(t, "warn", results[2].Status)
}

func TestCollectProviderTransportSmokeChecksSkippedByDefault(t *testing.T) {
	report := doctorJSONReport{status: jsonStatusOK}
	report.collectProviderTransportSmokeChecks(config.DefaultFullConfig("provider-smoke"), doctorOptions{})

	assert.Equal(t, jsonStatusOK, report.status)
	require.Len(t, report.checks, 1)
	assert.Equal(t, "skip", report.checks[0].Status)
}

func TestCollectProviderTransportSmokeChecksWarnsOnFailure(t *testing.T) {
	dir := t.TempDir()
	setFakeProviderOnPath(t, dir, "claude")

	cfg := config.DefaultFullConfig("provider-smoke")
	cfg.Spec.ReviewGate.Providers = []string{"claude"}
	cfg.Orchestra.Providers["claude"] = config.ProviderEntry{Binary: "claude"}

	origFactory := providerSmokeBackendFactory
	providerSmokeBackendFactory = func(orchestra.OrchestraConfig) orchestra.ExecutionBackend {
		return fakeProviderSmokeBackend{
			responses: map[string]*orchestra.ProviderResponse{"claude": {Provider: "claude"}},
			errors:    map[string]error{"claude": errors.New("transport failed")},
		}
	}
	defer func() { providerSmokeBackendFactory = origFactory }()

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectProviderTransportSmokeChecks(cfg, doctorOptions{providerSmoke: true, providerSmokeTimeout: time.Second})

	assert.Equal(t, jsonStatusWarn, report.status)
	require.Len(t, report.checks, 1)
	assert.Equal(t, "fail", report.checks[0].Status)
	require.Len(t, report.warnings, 1)
	assert.Equal(t, "provider_transport_failed", report.warnings[0].Code)
}

func TestSetFakeProviderOnPathSupportsRepeatedCalls(t *testing.T) {
	dir := t.TempDir()
	setFakeProviderOnPath(t, dir, "provider-a")
	setFakeProviderOnPath(t, dir, "provider-b")

	_, err := os.Stat(filepath.Join(dir, "bin", "provider-a"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "bin", "provider-b"))
	require.NoError(t, err)
}

package omp

import (
	"context"
	"testing"
)

func TestOMPModelIntegrationCatalog_RetryModelFallbackIsExactAllowlistedSetting(t *testing.T) {
	t.Parallel()

	runner := newModelIntegrationRunner()
	result := ProbeOMPModelCatalog(context.Background(), OMPModelCatalogProbeOptions{
		Runner: runner, Settings: []string{"retry.modelFallback"},
	})
	if result.Status != "ready" || len(result.Settings) != 1 ||
		result.Settings[0].Key != "retry.modelFallback" || !result.Settings[0].Supported {
		t.Fatalf("retry.modelFallback probe = %#v", result)
	}
}

func TestOMPModelIntegrationCatalog_MetadataRunnerRejectsUnownedReads(t *testing.T) {
	t.Parallel()

	accepted := [][]string{
		{"models", "--json", "--no-extensions"},
		{"config", "get", "retry.modelFallback", "--json"},
		{"--config", "/tmp/owned.yml", "config", "get", "task.agentModelOverrides", "--json"},
	}
	for _, args := range accepted {
		if !safeOMPModelIntegrationArgs(args) {
			t.Fatalf("safe args rejected: %v", args)
		}
	}
	rejected := [][]string{
		{"models", "--json"},
		{"models", "--json", "--extensions"},
		{"config", "get", "provider.apiKey", "--json"},
		{"--other", "value", "config", "get", "task.agentModelOverrides", "--json"},
		{"--config", "bad\npath", "config", "get", "task.agentModelOverrides", "--json"},
	}
	for _, args := range rejected {
		if safeOMPModelIntegrationArgs(args) {
			t.Fatalf("unsafe args accepted: %v", args)
		}
	}
}

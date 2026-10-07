package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/codexruntime"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

const codexRuntimeCatalogTimeout = 5 * time.Second

type codexCatalogProbeFunc func(context.Context, string) ([]byte, error)

var (
	runtimeCodexCatalogProbe   codexCatalogProbeFunc = probeCodexModelCatalog
	runtimeCodexFallbackWriter io.Writer             = os.Stderr
)

func probeCodexModelCatalog(ctx context.Context, binary string) ([]byte, error) {
	return codexruntime.ProbeModelCatalog(ctx, binary, codexRuntimeCatalogTimeout)
}

func resolveCodexProviderCapabilities(ctx context.Context, providers []orchestra.ProviderConfig) []orchestra.ProviderConfig {
	return resolveCodexProviderCapabilitiesWith(ctx, providers, runtimeCodexCatalogProbe, runtimeCodexFallbackWriter)
}

func resolveCodexProviderCapabilitiesWith(
	ctx context.Context,
	providers []orchestra.ProviderConfig,
	probe codexCatalogProbeFunc,
	writer io.Writer,
) []orchestra.ProviderConfig {
	resolved := append([]orchestra.ProviderConfig(nil), providers...)
	for i := range resolved {
		provider := &resolved[i]
		provider.Args = append([]string(nil), provider.Args...)
		if provider.Backend == config.ProviderBackendOMP || provider.Name != "codex" || provider.ModelPolicy != config.ProviderModelPolicyQuality {
			continue
		}

		catalogJSON, err := probe(ctx, provider.Binary)
		if err != nil {
			catalogJSON = nil
		}
		entry := config.ProviderEntry{
			Binary:      provider.Binary,
			Args:        provider.Args,
			ModelPolicy: provider.ModelPolicy,
		}
		entry, resolution := config.ResolveCodexProviderProfile(entry, catalogJSON)
		provider.Args = entry.Args
		reportCodexRuntimeFallback(writer, resolution)
	}
	return resolved
}

func reportCodexRuntimeFallback(writer io.Writer, resolution config.CodexProfileResolution) {
	if writer == nil || !resolution.Fallback {
		return
	}
	selected := "runtime-default"
	if resolution.Effective.Model != "" {
		selected = resolution.Effective.Model
		if resolution.Effective.Effort != "" {
			selected += "/" + resolution.Effective.Effort
		}
	}
	fmt.Fprintf(writer,
		"Codex model fallback: requested=%s/%s selected=%s reason=%s\n",
		strings.TrimSpace(resolution.Requested.Model),
		strings.TrimSpace(resolution.Requested.Effort),
		selected,
		resolution.Reason,
	)
}

package cli

import (
	"fmt"
	"os"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

func filterInstalledProviders(all []orchestra.ProviderConfig) []orchestra.ProviderConfig {
	var available []orchestra.ProviderConfig
	for _, provider := range all {
		binary := provider.Binary
		if provider.Backend == config.ProviderBackendOMP {
			binary = config.ProviderBackendOMP
		}
		if detect.IsInstalled(binary) {
			available = append(available, provider)
			continue
		}
		fmt.Fprintf(os.Stderr, "경고: %s 바이너리를 찾을 수 없습니다 (건너뜀)\n", binary)
	}
	return available
}

func configureSpecReviewProviders(providers []orchestra.ProviderConfig) []orchestra.ProviderConfig {
	configured := append([]orchestra.ProviderConfig(nil), providers...)
	for i := range configured {
		configured[i].ResultReadyPatterns = mergeStringValues(configured[i].ResultReadyPatterns, []string{"VERDICT:"})
		if configured[i].ResultReadyGrace <= 0 {
			configured[i].ResultReadyGrace = specReviewResultReadyGrace
		}
	}
	return configured
}

func resolveSpecReviewProviderNames(cfg *config.HarnessConfig, multi bool) []string {
	if cfg == nil {
		return nil
	}
	names := mergeProviderNames(cfg.Spec.ReviewGate.Providers)
	if !multi {
		return names
	}
	if command, ok := cfg.Orchestra.Commands["review"]; ok {
		names = mergeProviderNames(names, command.Providers)
	}
	return mergeProviderNames(names, sortedProviderKeys(cfg.Orchestra.Providers), defaultProviders())
}

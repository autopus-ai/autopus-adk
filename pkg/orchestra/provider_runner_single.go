package orchestra

import (
	"context"
	"errors"
	"fmt"
)

// ErrBackendUnavailable reports a provider whose configured backend has no
// route in the execution config, so it was not run at all.
var ErrBackendUnavailable = errors.New("orchestra: provider backend is not available in this execution path")

// singleProviderRole is the role a single-provider run records on its
// request and response.
const singleProviderRole = "single"

// RunSingleProvider runs exactly one provider once through
// runConfiguredProvider, the path every orchestra strategy uses. A provider
// with a backend (OMP-backed providers) always runs on the backend
// registered in cfg.ProviderBackends; a missing route fails with
// ErrBackendUnavailable before anything starts, so such a provider never
// falls back to the raw subprocess runner. A provider without a backend runs
// as a subprocess of its configured argv, unchanged. The call is bounded by
// the provider's execution timeout (ExecutionTimeout, else
// cfg.TimeoutSeconds, else 120 s); a timed-out run returns a response with
// TimedOut set. The working directory comes from provider.WorkDir, else
// cfg.ProviderWorkDir.
func RunSingleProvider(ctx context.Context, cfg OrchestraConfig, provider ProviderConfig, prompt string) (*ProviderResponse, error) {
	if provider.Backend != "" && cfg.ProviderBackends[provider.Backend] == nil {
		return nil, fmt.Errorf("%w: provider %s backend %q", ErrBackendUnavailable, provider.Name, provider.Backend)
	}
	ctx, cancel := context.WithTimeout(ctx, providerExecutionTimeout(provider, cfg.TimeoutSeconds))
	defer cancel()
	return runConfiguredProvider(ctx, cfg, provider, prompt, singleProviderRole, 1, nil)
}

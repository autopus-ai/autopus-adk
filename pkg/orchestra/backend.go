package orchestra

import (
	"context"
	"time"
)

// ExecutionBackend abstracts how a provider is executed. subprocessBackend
// spawns a child process with schema-enforced JSON I/O; routed backends such
// as OMP are injected through OrchestraConfig.ProviderBackends.
type ExecutionBackend interface {
	// Execute runs a single provider and returns its response.
	Execute(ctx context.Context, req ProviderRequest) (*ProviderResponse, error)
	// Name returns the backend identifier (e.g., "subprocess", "omp").
	Name() string
}

// ProviderRequest is the input for ExecutionBackend.Execute.
type ProviderRequest struct {
	Provider   string         // provider name
	Prompt     string         // prompt text to send
	SchemaPath string         // path to JSON schema file (subprocess mode)
	Role       string         // role descriptor for the provider
	Round      int            // current round number (debate/multi-round)
	Timeout    time.Duration  // per-provider timeout
	Config     ProviderConfig // full provider configuration
}

// SelectBackend returns the default ExecutionBackend: the subprocess backend
// (SPEC-PANERM-001 retired the interactive pane backend).
//
// Callers that exchange free text rather than schema-guided JSON must not use
// this: the subprocess backend validates JSON output.
func SelectBackend(OrchestraConfig) ExecutionBackend {
	return NewSubprocessBackendImpl()
}

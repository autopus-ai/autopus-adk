package orchestra

import (
	"os"
	"strings"
	"time"
)

// Sandbox mode vocabulary recorded on provider execution evidence.
const (
	SandboxModeReadOnly = "read-only"
	// SandboxModeUnverified marks a read-only projection whose enforcement has
	// no live evidence yet, so the receipt does not claim read-only.
	SandboxModeUnverified     = "unverified"
	SandboxModeWorkspaceWrite = "workspace-write"
	SandboxModeUnrestricted   = "unrestricted"
)

// ProviderExecution records how one provider process was launched. It is the
// provenance carrier behind the command/cwd/pid/sandbox fields of
// ProviderRunReceipt so a receipt can prove where and how a provider ran.
type ProviderExecution struct {
	Command     []string  `json:"command"`                // argv including the binary
	Cwd         string    `json:"cwd,omitempty"`          // effective process working directory
	PID         int       `json:"pid,omitempty"`          // started process ID; 0 when the process never started
	SandboxMode string    `json:"sandbox_mode,omitempty"` // read-only, unverified, workspace-write, or unrestricted
	StartedAt   time.Time `json:"started_at"`
	EndedAt     time.Time `json:"ended_at"`
}

// newProviderExecution captures launch provenance before the process starts.
// The cwd is the explicit provider WorkDir or, when empty, the inherited
// orchestrator cwd so the receipt always names the directory that ran.
func newProviderExecution(provider ProviderConfig, args []string, start time.Time) *ProviderExecution {
	command := make([]string, 0, len(args)+1)
	command = append(command, provider.Binary)
	command = append(command, args...)
	cwd := strings.TrimSpace(provider.WorkDir)
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return &ProviderExecution{
		Command:     command,
		Cwd:         cwd,
		SandboxMode: ProviderSandboxMode(provider, args),
		StartedAt:   start,
	}
}

// finish stamps the process end time derived from the measured duration so the
// receipt window matches the response Duration exactly.
func (e *ProviderExecution) finish(duration time.Duration) {
	if e == nil {
		return
	}
	e.EndedAt = e.StartedAt.Add(duration)
}

// resolveProviderWorkDir applies the run-level provider working directory to
// a provider that does not pin its own, so every subprocess dispatch site
// shares one cwd policy.
func resolveProviderWorkDir(cfg OrchestraConfig, provider ProviderConfig) ProviderConfig {
	if provider.WorkDir == "" {
		provider.WorkDir = cfg.ProviderWorkDir
	}
	return provider
}

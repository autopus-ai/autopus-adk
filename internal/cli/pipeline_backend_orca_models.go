package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"

	ompadapter "github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orcarun"
	"github.com/insajin/autopus-adk/pkg/pipeline"
)

// pipelineOrcaProviderAgents maps receipt provider identifiers onto orca agent
// names. Unknown providers fail closed: running a phase under the wrong agent
// is worse than refusing to run it.
var pipelineOrcaProviderAgents = map[string]string{
	"openai-codex": "codex",
	"anthropic":    "claude",
	"google":       "gemini",
	"gemini":       "gemini",
}

// pipelineOrcaPhaseRole returns the receipt role a phase is routed from, used
// as the PhaseResponse role so orca receipts read like OMP receipts.
func pipelineOrcaPhaseRole(phase pipeline.PhaseID) string {
	for role, routed := range ompPipelinePhaseRoles {
		if routed == phase {
			return role
		}
	}
	return string(phase)
}

// loadPipelineOrcaPhaseLaunch derives one orca launch per canonical phase from
// the OMP model resolution receipt.
//
// The receipt keeps provider, model, selector, and thinking effort apart. Only
// the opaque provider model id and the effort level cross the process-plane
// boundary (REQ-107); the joined selector stays inside the policy plane.
func loadPipelineOrcaPhaseLaunch(projectDir string) (map[pipeline.PhaseID]orcarun.Launch, error) {
	receipt, err := ompadapter.LoadOMPModelResolutionReceipt(projectDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// The omp owner tolerates an absent receipt because the OMP process
			// picks its own default model. Orca cannot: worker-start requires an
			// explicit --agent, and no agent name exists for the "omp" platform.
			// Guessing one would invent a routing decision in the process plane,
			// which is exactly what INV-101 forbids, so this fails closed.
			return nil, errors.New(
				"no model routing receipt at .autopus/omp-model-resolution-v1.json: " +
					"set role_model_policy.profile in autopus.yaml and rerun auto update, " +
					"because the orca path needs an explicit agent for every phase")
		}
		return nil, err
	}
	rows := make(map[string]ompadapter.OMPModelRoleReceipt, len(receipt.Roles))
	for _, row := range receipt.Roles {
		if previous, duplicate := rows[row.Agent]; duplicate && previous.Selector != row.Selector {
			return nil, fmt.Errorf("conflicting orca model routes for native agent %s", row.Agent)
		}
		rows[row.Agent] = row
	}
	launches := make(map[pipeline.PhaseID]orcarun.Launch, len(ompPipelinePhaseRoles))
	for role, phase := range ompPipelinePhaseRoles {
		native, err := config.OMPNativeAgentForRole(role)
		if err != nil {
			return nil, err
		}
		row, resolved := rows[native]
		if !resolved {
			return nil, fmt.Errorf(
				"OMP model receipt has no route for native agent %s required by phase %s", native, phase,
			)
		}
		agent, known := pipelineOrcaProviderAgents[row.Provider]
		if !known {
			return nil, fmt.Errorf("no orca agent is defined for provider %q (agent %s)", row.Provider, native)
		}
		if strings.TrimSpace(row.Model) == "" {
			return nil, fmt.Errorf("orca model route for phase %s has no provider model id", phase)
		}
		launches[phase] = orcarun.Launch{Agent: agent, Model: row.Model, Effort: row.Thinking}
	}
	return launches, nil
}

// newPipelineOrcaBackendForRun builds the run-scoped orca backend.
//
// Availability of the orca binary is checked before anything else: when the
// process plane is missing, no receipt is read, no subprocess is spawned, and
// no run state is written. The returned error wraps orcarun.ErrOrcaUnavailable
// so the caller can tell an absent process plane from a misconfigured one.
func newPipelineOrcaBackendForRun(
	projectDir string,
	specID string,
	resolvedSpec resolvedPipelineSpec,
	gitHash string,
) (*pipelineOrcaBackend, error) {
	if _, err := exec.LookPath(pipelineOrcaBinary); err != nil {
		return nil, fmt.Errorf("pipeline: %w: %v", orcarun.ErrOrcaUnavailable, err)
	}
	if strings.TrimSpace(resolvedSpec.SnapshotHash) == "" || strings.TrimSpace(gitHash) == "" {
		return nil, errors.New("pipeline: orca backend requires a resolved SPEC snapshot and git commit hash")
	}
	launches, err := loadPipelineOrcaPhaseLaunch(projectDir)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load orca model routes: %w", err)
	}
	return newPipelineOrcaBackend(pipelineOrcaBackendConfig{
		SpecID:      specID,
		ProjectDir:  projectDir,
		PhaseLaunch: launches,
		Client:      orcarun.New(),
	})
}

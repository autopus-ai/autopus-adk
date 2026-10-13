package omp

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/insajin/autopus-adk/pkg/config"
)

// OMPAgentSurfaceReport is what an operator surface can say about one
// project's agent surface, read without writing: the doctor's agent catalog
// and the update's migration lines both come from it, so they agree with
// what generation decides.
type OMPAgentSurfaceReport struct {
	// Source is the effective agent source of the config.
	Source string
	// RecordedAgents are canonical agents whose definition the manifest records.
	RecordedAgents []string
	// UserAgentFiles are canonical agents with a definition on disk the
	// manifest does not record: generation leaves those files alone.
	UserAgentFiles []string
	// MissingAgents are canonical agents with no definition on disk.
	MissingAgents []string
	// DisabledAgentsUserOwned reports task.disabledAgents as the user's, so
	// scout, security-reviewer, and sonic stay enabled in autopus mode.
	DisabledAgentsUserOwned bool
	// DisabledAgentsClaimed reports ADK's own claim of task.disabledAgents.
	DisabledAgentsClaimed bool
}

// InspectOMPAgentSurface reads the agent surface of root for cfg.
func InspectOMPAgentSurface(root string, cfg *config.HarnessConfig) (report OMPAgentSurfaceReport, returnErr error) {
	if cfg == nil {
		return OMPAgentSurfaceReport{}, fmt.Errorf("하네스 설정이 필요합니다")
	}
	report.Source = cfg.RoleModelPolicy.OMPAgentSource()
	workspace, err := openOMPRootedWorkspace(root)
	if err != nil {
		return report, err
	}
	defer func() { joinOMPRootedCloseError(&returnErr, workspace.Close()) }()
	manifest, err := loadOMPManifestAt(workspace, adapterName)
	if err != nil {
		return report, err
	}
	for _, agent := range config.CanonicalAgentNames() {
		path := OMPAgentDefinitionPath(agent)
		_, recorded := managedOMPFile(manifest, path)
		_, statErr := workspace.lstat(path)
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return report, fmt.Errorf("inspect OMP agent definition %s: %w", path, statErr)
		}
		present := statErr == nil
		switch {
		case !present:
			report.MissingAgents = append(report.MissingAgents, agent)
		case !recorded:
			report.UserAgentFiles = append(report.UserAgentFiles, agent)
		}
		if recorded {
			report.RecordedAgents = append(report.RecordedAgents, agent)
		}
	}
	state, err := evaluateOMPClaimSetAt(workspace, report.Source == config.OMPAgentSourceAutopus)
	if err != nil {
		return report, err
	}
	_, claimed := state.ownership.agentSurfaceKey()
	report.DisabledAgentsUserOwned = state.userOwned
	report.DisabledAgentsClaimed = state.ledger && claimed
	return report, nil
}

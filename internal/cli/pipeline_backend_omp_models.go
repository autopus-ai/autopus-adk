package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ompadapter "github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/insajin/autopus-adk/pkg/processprobe"
)

// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-OMP-004: model-route loading is the installed OMP-to-phase authority boundary.
// @AX:REASON [AUTO]: Executable version, persisted receipt binding, and all five canonical roles determine process model selection.
func loadPipelineOMPPhaseModels(
	projectDir, executable string,
) (map[pipeline.PhaseID]string, error) {
	_, identity, err := canonicalPipelineOMPExecutable(executable)
	if err != nil {
		return nil, err
	}
	environment, err := normalizePipelineOMPEnvironment(os.Environ())
	if err != nil {
		return nil, err
	}
	return loadPipelineOMPPhaseModelsWithAuthority(projectDir, executable, identity, environment)
}

// @AX:WARN [AUTO]: Authority-bound model-route loading has cyclomatic complexity 16 across identity, freshness, duplicate, and completeness checks.
// @AX:REASON [AUTO]: A stale or partial receipt must never select a phase model for the long-lived OMP process.
func loadPipelineOMPPhaseModelsWithAuthority(
	projectDir, executable string,
	identity pipelineOMPExecutableIdentity,
	environment []string,
) (map[pipeline.PhaseID]string, error) {
	if err := verifyPipelineOMPExecutable(executable, identity); err != nil {
		return nil, err
	}
	normalizedEnvironment, err := normalizePipelineOMPEnvironment(environment)
	if err != nil {
		return nil, err
	}
	observedVersion, err := observePipelineOMPVersion(projectDir, executable, identity, normalizedEnvironment)
	if err != nil {
		return nil, err
	}
	metadataDir := filepath.Join(projectDir, ".autopus")
	if info, err := os.Lstat(metadataDir); err != nil {
		if os.IsNotExist(err) {
			return map[pipeline.PhaseID]string{}, nil
		}
		return nil, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("OMP model receipt parent is unsafe")
	}
	receiptPath := filepath.Join(projectDir, filepath.FromSlash(ompadapter.OMPModelReceiptRelativePath))
	if _, err := os.Lstat(receiptPath); err != nil {
		if os.IsNotExist(err) {
			return map[pipeline.PhaseID]string{}, nil
		}
		return nil, err
	}
	receipt, err := ompadapter.LoadOMPModelResolutionReceipt(projectDir)
	if err != nil {
		return nil, err
	}
	if receipt.OMPVersion != observedVersion {
		return nil, fmt.Errorf("OMP model receipt version does not match installed executable")
	}
	pinsOnly, err := ompPipelinePinsOnly(projectDir)
	if err != nil {
		return nil, err
	}
	models := make(map[pipeline.PhaseID]string, len(ompPipelinePhaseRoles))
	selectors := make(map[string]string, len(receipt.Roles))
	for _, role := range receipt.Roles {
		if previous, duplicate := selectors[role.Agent]; duplicate && previous != role.Selector {
			return nil, fmt.Errorf("conflicting OMP model routes for agent %s", role.Agent)
		}
		selectors[role.Agent] = role.Selector
	}
	// Each phase runs on the agent of its own role. A phase whose agent the
	// receipt omits fails closed instead of inheriting an unrelated selector,
	// which is also what a receipt written for the bundled agents before
	// v0.50.131 gets until auto update rewrites it.
	for role, phase := range ompPipelinePhaseRoles {
		selector, resolved := selectors[role]
		if !resolved && pinsOnly {
			// A pins-only policy routes only what it pins; this phase's
			// agent keeps the OMP session model.
			continue
		}
		if !resolved || strings.TrimSpace(selector) == "" {
			return nil, ompPipelineMissingRouteError(role, phase)
		}
		models[phase] = selector
	}
	return models, nil
}

// ompPipelinePinsOnly reports whether the project's policy pins agents
// without a profile, which routes only the pinned phases. Loading the config
// also refuses a project that still names the removed agent_source: native.
func ompPipelinePinsOnly(projectDir string) (bool, error) {
	cfg, err := config.LoadPreview(projectDir)
	if err != nil {
		return false, err
	}
	policy := cfg.RoleModelPolicy
	return policy.Profile == "" && len(policy.Agents) > 0, nil
}

func ompPipelineMissingRouteError(agent string, phase pipeline.PhaseID) error {
	return fmt.Errorf("OMP model receipt has no route for agent %s required by phase %s", agent, phase)
}

// ompPipelinePhaseRoles maps each canonical pipeline phase onto the logical
// role that owns it, which is also the OMP agent that runs it.
var ompPipelinePhaseRoles = map[string]pipeline.PhaseID{
	"planner": pipeline.PhasePlan, "tester": pipeline.PhaseTestScaffold,
	"executor": pipeline.PhaseImplement, "validator": pipeline.PhaseValidate,
	"reviewer": pipeline.PhaseReview,
}

func observePipelineOMPVersion(
	projectDir, executable string,
	identity pipelineOMPExecutableIdentity,
	environment []string,
) (string, error) {
	// @AX:NOTE [AUTO] @AX:SPEC: SPEC-OMP-004: executable identity probes are bounded to 5 seconds and 4 KiB.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "--version")
	cmd.Dir = projectDir
	cmd.Env = append([]string(nil), environment...)
	if err := verifyPipelineOMPExecutable(executable, identity); err != nil {
		return "", err
	}
	output, err := processprobe.OutputLimited(cmd, 4<<10)
	if err != nil {
		return "", fmt.Errorf("OMP pipeline executable identity probe failed")
	}
	if err := verifyPipelineOMPExecutable(executable, identity); err != nil {
		return "", err
	}
	observed := strings.TrimSpace(string(output))
	if !installedOMPVersionPattern.MatchString(observed) {
		return "", fmt.Errorf("OMP pipeline executable identity is invalid")
	}
	return observed, nil
}

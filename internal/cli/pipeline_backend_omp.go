package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/pipeline"
)

type pipelineOMPBackendConfig struct {
	Executable         string
	ProjectDir         string
	SpecID             string
	SpecDir            string
	SnapshotHash       string
	GitCommitHash      string
	RuntimeBase        string
	Environment        []string
	canonicalEnv       []string
	PhaseModels        map[pipeline.PhaseID]string
	ModelContextWindow int
	MaxTime            time.Duration
	ManagedActive      pipelineOMPManagedActiveRunner
	managedInner       bool
	ownRuntimeBase     bool
	executableID       pipelineOMPExecutableIdentity
}

type pipelineOMPRunMode uint8

const (
	pipelineOMPRunModeUnknown pipelineOMPRunMode = iota
	pipelineOMPRunModeCanonical
	pipelineOMPRunModeActive
)

type pipelineOMPBackend struct {
	mu                   sync.Mutex
	config               pipelineOMPBackendConfig
	process              *pipelineOMPProcess
	protocol             *pipelineOMPRPCProtocol
	activeExecutableRoot string
	mode                 pipelineOMPRunMode
	closed               bool
}

var _ pipeline.PhaseBackend = (*pipelineOMPBackend)(nil)
var _ pipeline.PhaseBackendCloser = (*pipelineOMPBackend)(nil)

func newPipelineOMPBackend(config pipelineOMPBackendConfig) (*pipelineOMPBackend, error) {
	normalized, err := normalizePipelineOMPBackendConfig(config)
	if err != nil {
		return nil, err
	}
	backend := &pipelineOMPBackend{config: normalized}
	if coordinator, ok := normalized.ManagedActive.(*pipelineOMPManagedActiveCoordinator); ok {
		if coordinator.current == nil {
			normalized, backend.activeExecutableRoot, err = materializePipelineOMPActiveRuntimeConfig(normalized)
			if err != nil {
				if normalized.ownRuntimeBase {
					_ = os.Remove(normalized.RuntimeBase)
				}
				return nil, err
			}
			backend.config = normalized
		}
		if coordinator.start == nil {
			coordinator.start = newPipelineOMPActiveSessionStart(normalized)
		}
		if coordinator.current == nil {
			coordinator.current = newPipelineOMPActiveCurrentRuntimeProvider(normalized)
		}
	}
	return backend, nil
}

// @AX:WARN [AUTO]: Backend normalization has cyclomatic complexity 19 across authority and runtime checks.
// @AX:REASON [AUTO]: The RPC process must not start until every path, identity, permission, timeout, and copied input is canonical.
func normalizePipelineOMPBackendConfig(config pipelineOMPBackendConfig) (pipelineOMPBackendConfig, error) {
	for _, item := range []struct{ name, value string }{
		{"executable", config.Executable}, {"project directory", config.ProjectDir},
		{"SPEC ID", config.SpecID}, {"SPEC directory", config.SpecDir},
		{"snapshot hash", config.SnapshotHash}, {"git commit hash", config.GitCommitHash},
	} {
		if strings.TrimSpace(item.value) == "" || strings.ContainsRune(item.value, 0) {
			return config, fmt.Errorf("OMP pipeline %s is required", item.name)
		}
	}
	executable, executableID := config.Executable, config.executableID
	if executableID.info == nil {
		var err error
		executable, executableID, err = canonicalPipelineOMPExecutable(config.Executable)
		if err != nil {
			return config, err
		}
	} else if err := verifyPipelineOMPExecutable(executable, executableID); err != nil {
		return config, err
	}
	config.managedInner = pipelineOMPManagedInner(config.Environment)
	environment, err := normalizePipelineOMPEnvironment(config.Environment)
	if err != nil {
		return config, err
	}
	if info, err := os.Lstat(config.ProjectDir); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return config, errors.New("OMP pipeline project directory is unsafe")
	}
	if config.RuntimeBase == "" {
		base, err := os.MkdirTemp("", "autopus-omp-pipeline-")
		if err != nil {
			return config, err
		}
		config.RuntimeBase, config.ownRuntimeBase = base, true
	}
	baseInfo, err := os.Lstat(config.RuntimeBase)
	if err != nil || !baseInfo.IsDir() || baseInfo.Mode()&os.ModeSymlink != 0 || baseInfo.Mode().Perm()&0o077 != 0 {
		if config.ownRuntimeBase {
			_ = os.Remove(config.RuntimeBase)
		}
		return config, errors.New("OMP pipeline runtime base is unsafe")
	}
	if config.ModelContextWindow == 0 {
		config.ModelContextWindow = pipelineOMPActiveDefaultContextWindow
	}
	if config.ModelContextWindow < 8192 || config.ModelContextWindow > 1<<30 {
		return config, errors.New("OMP pipeline model context window is invalid")
	}
	if config.MaxTime <= 0 {
		config.MaxTime = 15 * time.Minute
	}
	config.ProjectDir = filepath.Clean(config.ProjectDir)
	config.SpecDir = filepath.Clean(config.SpecDir)
	config.Executable, config.executableID = executable, executableID
	config.Environment = environment
	config.canonicalEnv = pipelineOMPCanonicalEnvironment(environment)
	config.PhaseModels = clonePipelineOMPPhaseModels(config.PhaseModels)
	return config, nil
}

// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-OMP-004: this is the sole canonical phase-to-OMP RPC dispatch boundary.
// @AX:REASON [AUTO]: Sealed authority validation, one-process reuse, model selection, and fail-closed cleanup converge here.
// @AX:WARN [AUTO]: OMP phase dispatch has more than eight authority, lifecycle, initialization, and failure branches.
// @AX:REASON [AUTO]: Any mismatch or RPC failure must close the run-scoped process before another phase can reuse it.
func (backend *pipelineOMPBackend) Execute(
	ctx context.Context,
	request pipeline.PhaseRequest,
) (*pipeline.PhaseResponse, error) {
	response := &pipeline.PhaseResponse{
		Provider: "omp", Backend: "rpc-canonical-full", Role: string(request.PhaseID),
	}
	binding, err := request.OMPExecutionView.Binding()
	if err != nil {
		response.FailureClass = "execution_error"
		return response, err
	}
	expected := pipeline.OMPExecutionViewBinding{
		SpecID: backend.config.SpecID, SnapshotHash: backend.config.SnapshotHash,
		GitCommitHash: backend.config.GitCommitHash, PhaseID: request.PhaseID, Attempt: request.Attempt,
	}
	if binding != expected {
		response.FailureClass = "execution_error"
		return response, errors.New("pipeline: sealed OMP execution view binding mismatch")
	}
	snapshot, err := request.OMPExecutionView.Open(expected)
	if err != nil || snapshot.ProjectDir != backend.config.ProjectDir || snapshot.SpecDir != backend.config.SpecDir {
		response.FailureClass = "execution_error"
		return response, errors.New("pipeline: sealed OMP execution view authority mismatch")
	}
	if strings.HasPrefix(strings.TrimSpace(snapshot.Prompt), "/auto") {
		response.FailureClass = "execution_error"
		return response, errors.New("pipeline: managed OMP phase prompt cannot reissue /auto")
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.closed {
		return response, errors.New("pipeline: OMP backend is closed")
	}
	if backend.mode == pipelineOMPRunModeUnknown && !pipelineOMPPhaseModelsComplete(backend.config.PhaseModels) {
		backend.mode = pipelineOMPRunModeCanonical
	}
	if backend.mode != pipelineOMPRunModeCanonical && backend.config.ManagedActive != nil && !backend.config.managedInner {
		candidate, candidateErr := newPipelineOMPManagedActiveCandidate(
			snapshot, backend.config.PhaseModels[request.PhaseID], backend.config.PhaseModels,
		)
		var prepared pipelineOMPManagedActivePrepared
		prepareErr := candidateErr
		if prepareErr == nil {
			prepared, prepareErr = backend.config.ManagedActive.Prepare(ctx, candidate)
			if prepareErr == nil {
				prepareErr = validatePipelineOMPManagedActivePrepared(candidate, prepared)
			}
		}
		if prepareErr == nil && backend.mode == pipelineOMPRunModeUnknown {
			if activator, ok := backend.config.ManagedActive.(interface {
				Activate(context.Context, pipelineOMPManagedActiveCandidate, pipelineOMPManagedActivePrepared) error
			}); ok {
				prepareErr = activator.Activate(ctx, candidate, prepared)
			}
			if prepareErr == nil {
				backend.mode = pipelineOMPRunModeActive
			}
		}
		if prepareErr != nil {
			if backend.mode == pipelineOMPRunModeActive {
				response.FailureClass = "execution_error"
				_ = backend.closeLocked()
				return response, fmt.Errorf("pipeline: managed active run preflight drifted: %w", prepareErr)
			}
			backend.mode = pipelineOMPRunModeCanonical
		} else {
			response.Backend = "rpc-managed-active-history"
			response.Output, err = backend.config.ManagedActive.Execute(ctx, candidate, prepared)
			if err != nil || strings.TrimSpace(response.Output) == "" {
				if err == nil {
					err = errors.New("pipeline: managed active OMP returned empty assistant output")
				}
				response.FailureClass = "execution_error"
				_ = backend.closeLocked()
				return response, err
			}
			return response, nil
		}
	}
	if backend.mode == pipelineOMPRunModeUnknown {
		backend.mode = pipelineOMPRunModeCanonical
	}
	if backend.process == nil {
		backend.process, err = startPipelineOMPProcess(ctx, backend.config)
		if err == nil {
			backend.protocol = newPipelineOMPRPCProtocol(backend.process)
			backend.protocol.declaredContextWindow = backend.config.ModelContextWindow
			initCtx, cancel := context.WithTimeout(ctx, backend.config.MaxTime)
			err = backend.protocol.initialize(initCtx)
			cancel()
		}
		if err != nil {
			_ = backend.closeLocked()
			response.FailureClass = "execution_error"
			return response, err
		}
	}
	executeCtx, cancel := context.WithTimeout(ctx, backend.config.MaxTime)
	defer cancel()
	response.Output, err = backend.protocol.execute(executeCtx, backend.config.PhaseModels[request.PhaseID], snapshot.Prompt)
	if err != nil {
		response.FailureClass = "execution_error"
		_ = backend.closeLocked()
		return response, err
	}
	return response, nil
}

func (backend *pipelineOMPBackend) Close() error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	return backend.closeLocked()
}

func (backend *pipelineOMPBackend) closeLocked() error {
	if backend.closed {
		return nil
	}
	backend.closed = true
	var err error
	if backend.process != nil {
		err = backend.process.Close()
	}
	if backend.config.ManagedActive != nil {
		err = errors.Join(err, backend.config.ManagedActive.Close())
	}
	if backend.activeExecutableRoot != "" {
		err = errors.Join(err, os.RemoveAll(backend.activeExecutableRoot))
		backend.activeExecutableRoot = ""
	}
	if backend.config.ownRuntimeBase {
		err = errors.Join(err, os.Remove(backend.config.RuntimeBase))
	}
	return err
}

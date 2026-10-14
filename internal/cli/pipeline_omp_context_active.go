package cli

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/insajin/autopus-adk/pkg/version"
)

type pipelineOMPManagedActiveCandidate struct {
	Snapshot         pipeline.OMPExecutionSnapshot
	OriginalTask     string
	DeliveryCommand  string
	Provider         string
	Model            string
	ScopeProvider    string
	ModelScopeDigest string
	AutoSourceCommit string
	AutoSourceTree   string
}

type pipelineOMPManagedActivePrepared struct {
	Lease   *pipelineOMPActiveLease
	Binding pipelineOMPActiveLeaseBinding
	grant   pipelineOMPVerifiedGrant
}

type pipelineOMPManagedActiveRunner interface {
	Prepare(context.Context, pipelineOMPManagedActiveCandidate) (pipelineOMPManagedActivePrepared, error)
	Execute(context.Context, pipelineOMPManagedActiveCandidate, pipelineOMPManagedActivePrepared) (string, error)
	Close() error
}

func newPipelineOMPManagedActiveCandidate(
	snapshot pipeline.OMPExecutionSnapshot,
	modelSelector string,
	phaseModels map[pipeline.PhaseID]string,
) (pipelineOMPManagedActiveCandidate, error) {
	provider, model, ok := strings.Cut(modelSelector, "/")
	if !ok || !safePipelineOMPToken(provider) || !safePipelineOMPToken(model) {
		return pipelineOMPManagedActiveCandidate{}, errors.New("pipeline: managed active model authority is unavailable")
	}
	scopeProvider, scopeDigest, err := pipelineOMPActiveModelScope(phaseModels)
	if err != nil || scopeProvider != provider {
		return pipelineOMPManagedActiveCandidate{}, errors.New("pipeline: managed active model scope is unavailable")
	}
	return pipelineOMPManagedActiveCandidate{
		Snapshot: snapshot, OriginalTask: "/auto go " + snapshot.SpecID,
		DeliveryCommand: pipelineOMPContextCommand(snapshot.PhaseID), Provider: provider, Model: model,
		ScopeProvider: scopeProvider, ModelScopeDigest: scopeDigest,
		AutoSourceCommit: version.SourceCommit(), AutoSourceTree: version.SourceTree(),
	}, nil
}

// pipelineOMPPhaseModelsComplete reports a model for every canonical phase. A
// pins-only policy leaves unpinned phases on the OMP session model, which one
// model-scoped managed active lease cannot express, so such a run keeps the
// canonical path for every phase (SPEC-MODELUX-003 review A2).
func pipelineOMPPhaseModelsComplete(phaseModels map[pipeline.PhaseID]string) bool {
	for _, phase := range ompPipelinePhaseRoles {
		if strings.TrimSpace(phaseModels[phase]) == "" {
			return false
		}
	}
	return true
}

func pipelineOMPActiveModelScope(phaseModels map[pipeline.PhaseID]string) (string, string, error) {
	if len(phaseModels) == 0 {
		return "", "", errors.New("empty model scope")
	}
	parts := make([]string, 0, len(phaseModels))
	providerScope := ""
	for phase, selector := range phaseModels {
		provider, model, ok := strings.Cut(selector, "/")
		if !ok || !safePipelineOMPToken(provider) || !safePipelineOMPToken(model) ||
			providerScope != "" && providerScope != provider {
			return "", "", errors.New("heterogeneous model scope")
		}
		providerScope = provider
		parts = append(parts, string(phase)+"="+selector)
	}
	sort.Strings(parts)
	return providerScope, pipelineOMPActiveHash([]byte(strings.Join(parts, "\x00"))), nil
}

func validatePipelineOMPManagedActivePrepared(
	candidate pipelineOMPManagedActiveCandidate,
	prepared pipelineOMPManagedActivePrepared,
) error {
	if prepared.Lease == nil {
		return errors.New("pipeline: managed active lease is unavailable")
	}
	binding := prepared.Binding
	identity := pipelineOMPContextIdentity(candidate.Snapshot)
	if binding.SpecID != candidate.Snapshot.SpecID || binding.TaskID != identity[0] ||
		binding.Phase != string(candidate.Snapshot.PhaseID) || binding.SessionID != identity[1] ||
		binding.SnapshotHash != candidate.Snapshot.SnapshotHash ||
		binding.GitCommitHash != candidate.Snapshot.GitCommitHash ||
		binding.OriginalTaskHash != workflowContextRuntimeHash(candidate.OriginalTask) ||
		binding.DecisionDeltaHash != workflowContextRuntimeHash(candidate.Snapshot.ActivePrompt) ||
		binding.ModelScopeDigest != candidate.ModelScopeDigest ||
		binding.AutoSourceCommit != candidate.AutoSourceCommit || binding.AutoSourceTree != candidate.AutoSourceTree {
		return errors.New("pipeline: managed active lease candidate mismatch")
	}
	if err := validatePipelineOMPActiveLeaseBinding(binding); err != nil {
		return errors.New("pipeline: managed active lease binding is invalid")
	}
	return nil
}

func pipelineOMPManagedInner(environment []string) bool {
	for _, entry := range environment {
		if entry == "AUTOPUS_OMP_MANAGED_INNER=1" {
			return true
		}
	}
	return false
}

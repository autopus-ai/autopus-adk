package omp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
	"gopkg.in/yaml.v3"
)

const DefaultOMPModelOverlayPath = ".autopus/runtime/omp-model-routing-v1.yml"

// OMPModelOverlayProjection is the flat, YAML-shaped view of a compiled
// projection. AgentModelOverrides maps a bundled OMP agent name to a concrete
// model selector, which is the only binding OMP consults for a task spawn.
type OMPModelOverlayProjection struct {
	AgentModelOverrides map[string]string
	FallbackChains      map[string][]string
}

type OMPModelOverlayWriteInput struct {
	WorkspaceRoot string
	RelativePath  string
	Projection    OMPModelOverlayProjection
}

type OMPModelOverlayEvidence struct {
	RelativePath string
	ConfigHash   string
	Bytes        []byte
}

type OMPModelCommandRunner interface {
	Run(context.Context, []string) ([]byte, error)
}

type OMPModelActivationRequest struct {
	WorkspaceRoot        string
	OverlayRelativePath  string
	InvocationArgv       []string
	ReadbackArgv         []string
	ExpectedConfigHash   string
	ExpectedReadbackHash string
}

type OMPModelActivationEvidence struct {
	InvocationArgv []string
	ReadbackArgv   []string
	ConfigHash     string
	ReadbackHash   string
}

func CompileOMPModelOverlay(projection OMPModelOverlayProjection) ([]byte, error) {
	if len(projection.AgentModelOverrides) == 0 {
		return nil, fmt.Errorf("%s must not be empty", config.OMPNativeAgentModelOverridesKey)
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	overrides := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, agent := range sortedOMPModelKeys(projection.AgentModelOverrides) {
		selector := projection.AgentModelOverrides[agent]
		if err := validateOMPModelOverlayToken("agent", agent); err != nil {
			return nil, err
		}
		if err := validateOMPModelOverlayToken("selector", selector); err != nil {
			return nil, err
		}
		appendOMPModelYAMLPair(overrides, agent, scalarOMPModelYAML(selector))
	}
	task := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendOMPModelYAMLPair(task, "agentModelOverrides", overrides)
	appendOMPModelYAMLPair(root, "task", task)

	fallbacks := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	fallbackKeys := sortedOMPModelSliceKeys(projection.FallbackChains)
	for _, selector := range fallbackKeys {
		if err := validateOMPModelOverlayToken("fallback selector", selector); err != nil {
			return nil, err
		}
		chain := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, fallback := range projection.FallbackChains[selector] {
			if err := validateOMPModelOverlayToken("fallback candidate", fallback); err != nil {
				return nil, err
			}
			chain.Content = append(chain.Content, scalarOMPModelYAML(fallback))
		}
		appendOMPModelYAMLPair(fallbacks, selector, chain)
	}
	retry := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendOMPModelYAMLPair(retry, "fallbackChains", fallbacks)
	appendOMPModelYAMLPair(root, "retry", retry)

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return nil, fmt.Errorf("encode OMP model overlay: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("close OMP model overlay encoder: %w", err)
	}
	return output.Bytes(), nil
}

func WriteOMPModelOverlay(input OMPModelOverlayWriteInput) (evidence OMPModelOverlayEvidence, returnErr error) {
	workspace, err := openOMPRootedWorkspace(input.WorkspaceRoot)
	if err != nil {
		return OMPModelOverlayEvidence{}, err
	}
	defer func() { joinOMPRootedCloseError(&returnErr, workspace.Close()) }()
	return writeOMPModelOverlayAt(workspace, input)
}

func writeOMPModelOverlayAt(
	workspace *ompRootedWorkspace,
	input OMPModelOverlayWriteInput,
) (OMPModelOverlayEvidence, error) {
	data, err := CompileOMPModelOverlay(input.Projection)
	if err != nil {
		return OMPModelOverlayEvidence{}, err
	}
	relative := input.RelativePath
	if relative == "" {
		relative = DefaultOMPModelOverlayPath
	}
	if err := workspace.atomicWrite(relative, data, 0o600); err != nil {
		return OMPModelOverlayEvidence{}, err
	}
	return OMPModelOverlayEvidence{RelativePath: relative, ConfigHash: OMPModelSHA256(data), Bytes: append([]byte(nil), data...)}, nil
}

// @AX:WARN [AUTO]: model activation verification contains 9 if branches.
// @AX:REASON [AUTO]: runner, selector, overlay, subprocess, bounded output, status, and resolved-model evidence are fail-closed.
func VerifyOMPModelActivation(
	ctx context.Context,
	runner OMPModelCommandRunner,
	request OMPModelActivationRequest,
) (evidence OMPModelActivationEvidence, returnErr error) {
	if runner == nil {
		return OMPModelActivationEvidence{}, fmt.Errorf("activation runner is required")
	}
	workspace, err := openOMPRootedWorkspace(request.WorkspaceRoot)
	if err != nil {
		return OMPModelActivationEvidence{}, err
	}
	defer func() { joinOMPRootedCloseError(&returnErr, workspace.Close()) }()
	return verifyOMPModelActivationAt(ctx, runner, request, workspace)
}

func verifyOMPModelActivationAt(
	ctx context.Context,
	runner OMPModelCommandRunner,
	request OMPModelActivationRequest,
	workspace *ompRootedWorkspace,
) (OMPModelActivationEvidence, error) {
	if request.OverlayRelativePath != DefaultOMPModelOverlayPath {
		return OMPModelActivationEvidence{}, fmt.Errorf("activation requires the generated owner-only overlay")
	}
	path, err := workspace.absolute(request.OverlayRelativePath)
	if err != nil {
		return OMPModelActivationEvidence{}, err
	}
	if err := requireExactOMPModelConfigArg(request.InvocationArgv, path); err != nil {
		return OMPModelActivationEvidence{}, fmt.Errorf("activation argv mismatch: %w", err)
	}
	if err := requireExactOMPModelConfigArg(request.ReadbackArgv, path); err != nil {
		return OMPModelActivationEvidence{}, fmt.Errorf("readback argv mismatch: %w", err)
	}
	if err := validateOMPModelWorkspaceBinding(workspace); err != nil {
		return OMPModelActivationEvidence{}, err
	}
	config, err := readOMPModelOwnedFileAt(workspace, request.OverlayRelativePath)
	if err != nil {
		return OMPModelActivationEvidence{}, err
	}
	configHash := OMPModelSHA256(config)
	if !validOMPModelHash(request.ExpectedConfigHash) || configHash != request.ExpectedConfigHash {
		return OMPModelActivationEvidence{}, fmt.Errorf("activation config hash mismatch")
	}
	if !validOMPModelHash(request.ExpectedReadbackHash) {
		return OMPModelActivationEvidence{}, fmt.Errorf("expected readback hash is invalid")
	}
	if err := validateOMPModelWorkspaceBinding(workspace); err != nil {
		return OMPModelActivationEvidence{}, err
	}
	stagedPath, cleanup, err := stageOMPModelActivationConfig(config)
	if err != nil {
		return OMPModelActivationEvidence{}, err
	}
	readbackArgv := replaceOMPModelConfigArg(request.ReadbackArgv, stagedPath)
	readback, runErr := runner.Run(ctx, readbackArgv)
	cleanupErr := cleanup()
	if runErr != nil || cleanupErr != nil {
		return OMPModelActivationEvidence{}, fmt.Errorf("OMP config readback: %w", errors.Join(runErr, cleanupErr))
	}
	readbackHash := OMPModelSHA256(readback)
	if readbackHash != request.ExpectedReadbackHash {
		return OMPModelActivationEvidence{}, fmt.Errorf("activation readback hash mismatch")
	}
	if err := validateOMPModelWorkspaceBinding(workspace); err != nil {
		return OMPModelActivationEvidence{}, err
	}
	return OMPModelActivationEvidence{
		InvocationArgv: append([]string(nil), request.InvocationArgv...),
		ReadbackArgv:   readbackArgv,
		ConfigHash:     configHash,
		ReadbackHash:   readbackHash,
	}, nil
}

func sortedOMPModelKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedOMPModelSliceKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func appendOMPModelYAMLPair(mapping *yaml.Node, key string, value *yaml.Node) {
	mapping.Content = append(mapping.Content, scalarOMPModelYAML(key), value)
}

func scalarOMPModelYAML(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func validateOMPModelOverlayToken(label, value string) error {
	if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("%s is invalid", label)
	}
	return nil
}

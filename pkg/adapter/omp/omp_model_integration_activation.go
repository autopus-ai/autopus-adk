package omp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func (i *ompModelIntegration) finalize(
	ctx context.Context,
	a *Adapter,
	workspace *ompRootedWorkspace,
	configMapping adapter.FileMapping,
) (adapter.FileMapping, []adapter.FileMapping, error) {
	overlay, err := OMPModelOverlayFromProjection(i.projection)
	if err != nil {
		return configMapping, nil, err
	}
	expected := ompIntegratedExpectedValues(overlay, i.profile.Safety)
	configSource := i.profile.ConfigMode
	configRelative := DefaultOMPModelOverlayPath
	var runtime []adapter.FileMapping
	activationConfig, err := compileOMPIntegratedOverlay(overlay, i.profile.Safety)
	if err != nil {
		return configMapping, nil, err
	}
	projectOwnershipDigest := ""

	switch i.profile.ConfigMode {
	case "overlay":
		runtime = append(runtime, ompIntegratedMapping(configRelative, activationConfig))
	case "project-managed":
		configRelative = configFile
		merged, ledger, ownershipDigest, mergeErr := i.mergeOMPIntegratedProjectConfig(
			workspace, configMapping, expected,
		)
		if mergeErr != nil {
			return configMapping, nil, mergeErr
		}
		configMapping = merged
		projectOwnershipDigest = ownershipDigest
		runtime = append(runtime, ledger)
	default:
		return configMapping, nil, fmt.Errorf("config_mode_invalid: %s", i.profile.ConfigMode)
	}

	readback, err := verifyOMPIntegratedReadback(
		ctx, a.modelIntegrationRunner, activationConfig, expected,
	)
	if err != nil {
		return configMapping, nil, err
	}
	receipt, err := i.receiptMapping(
		a, configSource, configRelative, activationConfig, readback, projectOwnershipDigest,
	)
	if err != nil {
		return configMapping, nil, err
	}
	runtime = append(runtime, receipt)
	return configMapping, runtime, nil
}

func compileOMPIntegratedOverlay(
	projection OMPModelOverlayProjection,
	safety config.RoleSafetyPolicyConf,
) ([]byte, error) {
	data, err := CompileOMPModelOverlay(projection)
	if err != nil {
		return nil, err
	}
	claims := append([]OMPManagedKeyClaim{{
		Path: "retry.modelFallback", Value: ompIntegratedModelFallback(projection),
		Complete: true, PriorFingerprint: OMPMissingManagedValueFingerprint(),
	}}, ompIntegratedSafetyClaims(safety)...)
	merged, err := MergeOMPProjectManagedConfig(OMPProjectManagedInput{
		Existing: data, Mode: 0o600, Claims: claims,
	})
	if err != nil {
		return nil, err
	}
	return merged.Bytes, nil
}

// ompIntegratedModelFallback reports whether OMP may retry a role on another
// model. It is enabled only when the projection actually emitted a nonempty
// fallback chain: a profile whose agents each declare one exact candidate must
// block on an unavailable model instead of silently landing on a weaker one,
// so the key is written as false rather than left to the runtime default.
func ompIntegratedModelFallback(projection OMPModelOverlayProjection) bool {
	for _, chain := range projection.FallbackChains {
		if len(chain) > 0 {
			return true
		}
	}
	return false
}

func ompIntegratedExpectedValues(
	projection OMPModelOverlayProjection,
	safety config.RoleSafetyPolicyConf,
) map[string]any {
	fallbacks := make(map[string]any, len(projection.FallbackChains))
	for selector, candidates := range projection.FallbackChains {
		fallbacks[selector] = append([]string(nil), candidates...)
	}
	values := map[string]any{
		config.OMPNativeAgentModelOverridesKey: projection.AgentModelOverrides,
		"retry.fallbackChains":                 fallbacks,
		"retry.modelFallback":                  ompIntegratedModelFallback(projection),
	}
	if safety.ApprovalMode != "" {
		values["tools.approvalMode"] = safety.ApprovalMode
	}
	if safety.IsolationMode != "" {
		values["task.isolation.mode"] = safety.IsolationMode
	}
	return values
}

func ompIntegratedSafetyClaims(safety config.RoleSafetyPolicyConf) []OMPManagedKeyClaim {
	claims := make([]OMPManagedKeyClaim, 0, 2)
	missing := OMPMissingManagedValueFingerprint()
	if safety.ApprovalMode != "" {
		claims = append(claims, OMPManagedKeyClaim{
			Path: "tools.approvalMode", Value: safety.ApprovalMode,
			Complete: true, PriorFingerprint: missing,
		})
	}
	if safety.IsolationMode != "" {
		claims = append(claims, OMPManagedKeyClaim{
			Path: "task.isolation.mode", Value: safety.IsolationMode,
			Complete: true, PriorFingerprint: missing,
		})
	}
	return claims
}

func ompIntegratedMapping(path string, data []byte) adapter.FileMapping {
	return adapter.FileMapping{
		TargetPath: path, OverwritePolicy: adapter.OverwriteAlways,
		Checksum: adapter.Checksum(string(data)), Content: append([]byte(nil), data...),
	}
}

func verifyOMPIntegratedReadback(
	ctx context.Context,
	runner OMPModelCatalogRunner,
	configData []byte,
	expected map[string]any,
) ([]byte, error) {
	if runner == nil {
		return nil, fmt.Errorf("activation runner is required")
	}
	tempRoot, err := os.MkdirTemp("", "autopus-omp-model-readback-*")
	if err != nil {
		return nil, fmt.Errorf("create activation temp root: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempRoot) }()
	configPath := filepath.Join(tempRoot, "config.yml")
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		return nil, fmt.Errorf("write activation temp config: %w", err)
	}

	return ReadOMPModelExpectedValues(ctx, runner, configPath, expected)
}

// ReadOMPModelExpectedValues reads every expected owned key and returns one
// canonical JSON map. The caller owns configPath selection and containment.
func ReadOMPModelExpectedValues(
	ctx context.Context,
	runner OMPModelCatalogRunner,
	configPath string,
	expected map[string]any,
) ([]byte, error) {
	if runner == nil || configPath == "" || len(expected) == 0 {
		return nil, fmt.Errorf("activation readback inputs are required")
	}
	keys := make([]string, 0, len(expected))
	for key := range expected {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	readback := make(map[string]any, len(keys))
	selectorRunner, useSelectorRPC := runner.(ompModelSelectorRPCStateRunner)
	if useSelectorRPC {
		overrides, ok := expected[config.OMPNativeAgentModelOverridesKey].(map[string]string)
		if !ok || len(overrides) == 0 {
			return nil, fmt.Errorf("activation readback %s is invalid",
				config.OMPNativeAgentModelOverridesKey)
		}
		resolved, err := readOMPModelAgentSelectorsViaRPC(ctx, selectorRunner, configPath, overrides)
		if err != nil {
			return nil, err
		}
		readback[config.OMPNativeAgentModelOverridesKey] = resolved
	}
	for _, key := range keys {
		if useSelectorRPC && key == config.OMPNativeAgentModelOverridesKey {
			continue
		}
		output, runErr := runner.Run(ctx, cliBinary,
			"--config", configPath, "config", "get", key, "--json")
		if runErr != nil {
			return nil, fmt.Errorf("activation readback %s: %w", key, runErr)
		}
		value, parseErr := parseOMPIntegratedReadback(output, key)
		if parseErr != nil {
			return nil, parseErr
		}
		want, wantErr := canonicalOMPManagedValueJSON(expected[key])
		got, gotErr := canonicalOMPManagedValueJSON(value)
		if wantErr != nil || gotErr != nil || !bytes.Equal(want, got) {
			return nil, fmt.Errorf("activation readback mismatch: %s", key)
		}
		readback[key] = value
	}
	return canonicalOMPManagedValueJSON(readback)
}

func parseOMPIntegratedReadback(output []byte, key string) (any, error) {
	var wrapper struct {
		Key   string          `json:"key"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(output, &wrapper); err != nil || wrapper.Key != key || len(wrapper.Value) == 0 {
		return nil, fmt.Errorf("activation readback invalid: %s", key)
	}
	var value any
	if err := json.Unmarshal(wrapper.Value, &value); err != nil {
		return nil, fmt.Errorf("activation readback invalid: %s", key)
	}
	return value, nil
}

// OMPManagedModelInvocationArgv returns the mandatory config argv without executing it.
func OMPManagedModelInvocationArgv(root, relative string, command []string) ([]string, error) {
	path, err := resolveOMPModelOwnedPath(root, relative, false)
	if err != nil {
		return nil, err
	}
	for _, arg := range command {
		if arg == "--config" || arg == "" || strings.ContainsAny(arg, "\x00\r\n") {
			return nil, fmt.Errorf("managed invocation arguments are invalid")
		}
	}
	result := []string{"--config", path}
	return append(result, command...), nil
}

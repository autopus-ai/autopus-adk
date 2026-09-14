package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

const ompModelDoctorProbeOutput = 128 * 1024

type ompModelDoctorExecRunner struct {
	process *omp.OMPModelProbeProcess
	pinErr  error
}

func newOMPModelDoctorExecRunner() *ompModelDoctorExecRunner {
	process, err := omp.NewOMPInstalledModelProbeProcess("omp", ompModelDoctorProbeOutput)
	return &ompModelDoctorExecRunner{process: process, pinErr: err}
}

func (runner ompModelDoctorExecRunner) Run(
	ctx context.Context,
	executable string,
	args ...string,
) ([]byte, error) {
	if executable != "omp" || !safeOMPModelDoctorProbeArgs(args) {
		return nil, errors.New("unsafe OMP model doctor command")
	}
	if runner.pinErr != nil {
		return nil, runner.pinErr
	}
	return runner.process.Run(ctx, args...)
}

func (runner ompModelDoctorExecRunner) RunWithInput(
	ctx context.Context,
	executable string,
	input []byte,
	args ...string,
) ([]byte, error) {
	if executable != "omp" || !bytes.Equal(input, []byte(`{"id":"autopus-model-state","type":"get_state"}`+"\n")) ||
		!omp.SafeOMPModelSelectorRPCArgs(args) {
		return nil, errors.New("unsafe OMP model doctor selector command")
	}
	if runner.pinErr != nil {
		return nil, runner.pinErr
	}
	return runner.process.RunInput(ctx, input, args...)
}

func safeOMPModelDoctorProbeArgs(args []string) bool {
	joined := strings.Join(args, " ")
	if joined == "--version" || joined == "models --json --no-extensions" ||
		joined == "config list --json" {
		return true
	}
	keyIndex := 2
	if len(args) == 6 && args[0] == "--config" && filepath.IsAbs(args[1]) &&
		!strings.ContainsAny(args[1], "\x00\r\n") {
		keyIndex = 4
	}
	if len(args) != keyIndex+2 || args[keyIndex-2] != "config" ||
		args[keyIndex-1] != "get" || args[keyIndex+1] != "--json" {
		return false
	}
	allowed := map[string]bool{
		config.OMPNativeAgentModelOverridesKey: true,
		"retry.fallbackChains":                 true,
		"retry.modelFallback":                  true,
		"tools.approvalMode":                   true,
		"task.isolation.mode":                  true,
	}
	return allowed[args[keyIndex]]
}

func buildOMPModelDoctorInput(
	ctx context.Context,
	root string,
	cfg *config.HarnessConfig,
	runner omp.OMPModelCatalogRunner,
) omp.OMPModelDoctorInput {
	if cfg == nil {
		return omp.OMPModelDoctorInput{}
	}
	profileName, profile, selected := cfg.RoleModelPolicy.SelectedRoleModelProfileForQuality(cfg.Quality)
	if !selected {
		return omp.OMPModelDoctorInput{}
	}
	input := omp.OMPModelDoctorInput{
		Enabled: true, WorkspaceRoot: root, Profile: profileName,
		ConfigSource: profile.ConfigMode, ConfiguredSource: profile.ConfigMode,
	}
	if receiptSource, ownershipDigest, _ := omp.OMPModelDoctorReceiptConfigSource(root); receiptSource != "" {
		input.ConfigSource = receiptSource
		input.ProjectOwnershipDigest = ownershipDigest
	}
	if runner == nil {
		input.Probe = omp.OMPModelCatalogProbeResult{Status: "blocked", Reason: "identity_unverified"}
		return input
	}
	input.Probe = omp.ProbeOMPModelCatalogForProfile(ctx, omp.OMPModelCatalogProbeOptions{
		Executable: "omp", Runner: runner, Timeout: ompDoctorProbeTimeout,
		MaxOutput: ompModelDoctorProbeOutput,
	}, profile)
	if input.Probe.Status != "ready" || input.Probe.Reason != "catalog_ready" {
		return input
	}

	input.Compilation = compileOMPModelDoctorRouting(profile, input.Probe.Catalog)
	expectation, err := omp.CompileOMPModelDoctorActivationExpectation(profile, input.Probe.Catalog)
	if err != nil {
		return input
	}
	input.Activation = readOMPModelDoctorActivationExact(
		ctx, root, input.ConfigSource, runner, expectation,
	)
	return input
}

// compileOMPModelDoctorRouting mirrors the generation bridge for doctor rows:
// one route per bundled native OMP agent, keyed by the native name, carrying
// the semantic role and capability of the logical role that supplied it. A
// native agent whose governing role the profile does not declare is skipped so
// a partial profile still reports.
func compileOMPModelDoctorRouting(
	profile config.RoleModelProfileConf,
	catalog omp.OMPModelCatalog,
) omp.OMPModelRoutingCompilation {
	routes := make(map[string]omp.OMPModelRouteRequest)
	diverseRoles := make(map[string]struct{}, len(profile.FamilyDiversity.Roles))
	if profile.FamilyDiversity.Enabled {
		for _, role := range profile.FamilyDiversity.Roles {
			diverseRoles[role] = struct{}{}
		}
	}
	for _, native := range config.OMPNativeAgentNames() {
		policy, route, err := profile.OMPNativeAgentRoute(native)
		if err != nil {
			continue
		}
		_, preferDistinctFamily := diverseRoles[policy.Role]
		request := omp.OMPModelRouteRequest{
			Agent: native, Role: policy.Role, Capability: policy.Capability,
			Required: route.Required, DegradedAction: route.DegradedAction,
			PreferDistinctExecutorFamily: preferDistinctFamily,
		}
		for _, candidate := range route.Candidates {
			request.Candidates = append(request.Candidates, omp.OMPRoutingCandidate{
				Selector: candidate.Selector, Thinking: candidate.Thinking, Family: candidate.Family,
			})
		}
		routes[native] = request
	}
	return omp.CompileOMPModelRouting(omp.OMPModelRoutingInput{
		Catalog: catalog, CatalogReason: "catalog_ready", Routes: routes,
	})
}

func readOMPModelDoctorActivation(
	ctx context.Context,
	root string,
	runner omp.OMPModelCatalogRunner,
) omp.OMPModelActivationEvidence {
	path, ok := safeOMPModelDoctorOverlayPath(root)
	if !ok {
		return omp.OMPModelActivationEvidence{}
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > ompModelDoctorProbeOutput {
		return omp.OMPModelActivationEvidence{}
	}
	readback, err := runner.Run(ctx, "omp", "--config", omp.DefaultOMPModelOverlayPath,
		"config", "get", config.OMPNativeAgentModelOverridesKey)
	if err != nil || len(readback) > ompModelDoctorProbeOutput {
		return omp.OMPModelActivationEvidence{ConfigHash: omp.OMPModelSHA256(data)}
	}
	return omp.OMPModelActivationEvidence{
		ConfigHash: omp.OMPModelSHA256(data), ReadbackHash: omp.OMPModelSHA256(readback),
	}
}

func readOMPModelDoctorActivationExact(
	ctx context.Context,
	root string,
	configSource string,
	runner omp.OMPModelCatalogRunner,
	expectation omp.OMPModelDoctorActivationExpectation,
) omp.OMPModelActivationEvidence {
	relative := omp.DefaultOMPModelOverlayPath
	requireOwnerOnly := true
	if configSource == config.RoleModelConfigModeProjectManaged {
		relative = ".omp/config.yml"
		requireOwnerOnly = false
	} else if configSource != config.RoleModelConfigModeOverlay {
		return omp.OMPModelActivationEvidence{}
	}
	path, ok := safeOMPModelDoctorConfigPath(root, relative, requireOwnerOnly)
	if !ok {
		return omp.OMPModelActivationEvidence{}
	}
	configHash := expectation.ConfigHash
	if requireOwnerOnly {
		data, err := os.ReadFile(path)
		if err != nil || len(data) > ompModelDoctorProbeOutput {
			return omp.OMPModelActivationEvidence{}
		}
		configHash = omp.OMPModelSHA256(data)
	}
	readback, err := omp.ReadOMPModelExpectedValues(ctx, runner, path, expectation.ExpectedValues)
	if err != nil || len(readback) > ompModelDoctorProbeOutput {
		return omp.OMPModelActivationEvidence{ConfigHash: configHash}
	}
	return omp.OMPModelActivationEvidence{
		ConfigHash: configHash, ReadbackHash: omp.OMPModelSHA256(readback),
	}
}

func safeOMPModelDoctorOverlayPath(root string) (string, bool) {
	return safeOMPModelDoctorConfigPath(root, omp.DefaultOMPModelOverlayPath, true)
}

// @AX:WARN [AUTO]: doctor config path validation has cyclomatic complexity 15.
// @AX:REASON [AUTO]: gocyclo reports 15 across canonical-root, symlink, regular-file, ownership, and permission checks.
func safeOMPModelDoctorConfigPath(root, relative string, ownerOnly bool) (string, bool) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	rootInfo, err := os.Lstat(absRoot)
	if err != nil || rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return "", false
	}
	canonicalRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", false
	}
	target := filepath.Join(absRoot, filepath.FromSlash(relative))
	canonicalTarget, err := filepath.EvalSymlinks(target)
	if err != nil {
		return "", false
	}
	contained, err := filepath.Rel(canonicalRoot, canonicalTarget)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := os.Lstat(canonicalTarget)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		ownerOnly && info.Mode().Perm() != 0o600 {
		return "", false
	}
	return canonicalTarget, true
}

func probeOMPModelRoutingDoctor(
	ctx context.Context,
	root string,
) omp.OMPModelDoctorReport {
	cfg, err := config.LoadPreview(root)
	if err != nil {
		return omp.OMPModelDoctorReport{}
	}
	input := buildOMPModelDoctorInput(ctx, root, cfg, newOMPModelDoctorExecRunner())
	return omp.CheckOMPModelRoutingDoctor(input)
}

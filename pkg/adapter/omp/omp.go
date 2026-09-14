package omp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

const (
	adapterName = "omp"
	cliBinary   = "omp"
	adapterVer  = "1.0.0"

	// ompRetiredAgentDir held one generated definition per ADK role until the
	// native cutover. OMP now serves every Autopus workflow from its bundled
	// registry, so nothing is written here and every manifest-recorded file
	// left over is retired.
	ompRetiredAgentDir = ".omp/agents"
)

// Adapter is the oh-my-pi (omp) platform adapter.
type Adapter struct {
	root                   string
	engine                 *tmpl.Engine
	modelIntegrationRunner OMPModelCatalogRunner
	modelIntegrationClock  func() time.Time
	rootedWorkspaceHook    func()
	rootedRootCreatedHook  func()
}

// New creates an adapter rooted at the current directory.
func New() *Adapter {
	return NewWithRoot(".")
}

// NewWithRoot creates an adapter rooted at the specified path.
func NewWithRoot(root string) *Adapter {
	return &Adapter{
		root: root, engine: tmpl.New(),
		modelIntegrationRunner: newOMPModelIntegrationExecRunner(),
		modelIntegrationClock:  time.Now,
	}
}

func (a *Adapter) Name() string        { return adapterName }
func (a *Adapter) Version() string     { return adapterVer }
func (a *Adapter) CLIBinary() string   { return cliBinary }
func (a *Adapter) SupportsHooks() bool { return false }

// Detect accepts only a bounded, sanitized oh-my-pi identity probe.
func (a *Adapter) Detect(ctx context.Context) (bool, error) {
	_, ok := detect.ProbeOMPIdentity(ctx, cliBinary)
	return ok, nil
}

// InstallHooks is a no-op for omp.
func (a *Adapter) InstallHooks(_ context.Context, _ []adapter.HookConfig, _ *adapter.PermissionSet) error {
	return nil
}

// Generate creates omp platform files based on the harness config.
// @AX:ANCHOR [AUTO]: preserve OMP generation as the manifest-owning platform entry point.
// @AX:REASON [AUTO]: init, add, update, preview, doctor, and drift dispatch depend on generation recording every emitted file for later update and cleanup.
func (a *Adapter) Generate(ctx context.Context, cfg *config.HarnessConfig) (pf *adapter.PlatformFiles, returnErr error) {
	workspace, err := openOrCreateOMPRootedWorkspace(a.root, a.rootedRootCreatedHook)
	if err != nil {
		return nil, err
	}
	defer func() { joinOMPRootedCloseError(&returnErr, workspace.Close()) }()
	if a.rootedWorkspaceHook != nil {
		a.rootedWorkspaceHook()
	}
	files, err := a.prepareFilesAt(ctx, cfg, workspace)
	if err != nil {
		return nil, err
	}

	pf = &adapter.PlatformFiles{Files: files, Checksum: adapter.Checksum(fmt.Sprintf("%d", len(files)))}
	resolveMode := a.fileModeResolverAt(workspace)
	writes := make([]adapter.TransactionWrite, 0, len(files))
	for _, file := range files {
		perm := resolveMode(file.TargetPath)
		unchanged, err := ompRootedMappingUnchanged(workspace, file, perm)
		if err != nil {
			return nil, err
		}
		if !unchanged {
			writes = append(writes, adapter.TransactionWrite{
				Path: file.TargetPath, Content: file.Content, Perm: perm,
			})
		}
	}
	manifest := adapter.ManifestFromFiles(adapterName, pf)
	oldManifest, err := loadOMPManifestAt(workspace, adapterName)
	if err != nil {
		return nil, fmt.Errorf("매니페스트 로드 실패: %w", err)
	}
	// A re-init over an install that still records .omp/agents would orphan
	// the retired ADK agent definitions: OMP resolves task agents by exact
	// name, so a stale project file keeps shadowing the bundled agent of the
	// same name forever. Only manifest-recorded paths are eligible, so a user
	// agent OMP never wrote is never touched, and the rooted transaction
	// snapshots each removal before deleting it.
	removes := adapter.TransactionRemovesFromManifestDiff(
		adapter.BuildManifestDiff(oldManifest, files, []string{ompRetiredAgentDir}), false,
	)
	if len(writes) == 0 && len(removes) == 0 &&
		ompRootedManifestUnchanged(workspace, oldManifest, manifest) {
		manifest = nil
	}
	plan := adapter.TransactionPlan{Writes: writes, Removes: removes, Manifest: manifest}
	if _, err := applyOMPTransactionAt(workspace, adapterName, plan); err != nil {
		return nil, err
	}
	return pf, nil
}

// Update updates omp files based on the manifest diff.
func (a *Adapter) Update(ctx context.Context, cfg *config.HarnessConfig) (pf *adapter.PlatformFiles, returnErr error) {
	workspace, err := openOMPRootedWorkspace(a.root)
	if err != nil {
		return nil, err
	}
	defer func() { joinOMPRootedCloseError(&returnErr, workspace.Close()) }()
	if a.rootedWorkspaceHook != nil {
		a.rootedWorkspaceHook()
	}
	if err := hardenLegacyOMPFilesForUpdate(workspace, adapterName); err != nil {
		return nil, fmt.Errorf("매니페스트 로드 실패: %w", err)
	}
	oldManifest, err := loadOMPManifestAt(workspace, adapterName)
	if err != nil {
		return nil, fmt.Errorf("매니페스트 로드 실패: %w", err)
	}

	files, err := a.prepareFilesAt(ctx, cfg, workspace)
	if err != nil {
		return nil, err
	}
	plan, pf, err := a.buildUpdateTransactionPlanAt(workspace, oldManifest, files, cfg)
	if err != nil {
		return nil, err
	}
	if _, err := applyOMPTransactionAt(workspace, adapterName, plan); err != nil {
		return nil, err
	}
	return pf, nil
}

func (a *Adapter) prepareFiles(ctx context.Context, cfg *config.HarnessConfig) (files []adapter.FileMapping, returnErr error) {
	workspace, err := openOMPRootedWorkspace(a.root)
	if err != nil {
		return nil, err
	}
	defer func() { joinOMPRootedCloseError(&returnErr, workspace.Close()) }()
	return a.prepareFilesAt(ctx, cfg, workspace)
}

// @AX:WARN [AUTO]: OMP surface preparation contains more than eight conditional branches.
// @AX:REASON [AUTO]: model integration, generated mappings, context bridge, optional project config, and runtime finalization converge here.
func (a *Adapter) prepareFilesAt(
	ctx context.Context,
	cfg *config.HarnessConfig,
	workspace *ompRootedWorkspace,
) ([]adapter.FileMapping, error) {
	if cfg == nil {
		return nil, fmt.Errorf("하네스 설정이 필요합니다")
	}

	modelIntegration, err := a.prepareModelIntegration(ctx, cfg)
	if err != nil {
		return nil, err
	}

	var files []adapter.FileMapping
	appendFiles := func(items []adapter.FileMapping, err error) error {
		if err != nil {
			return err
		}
		files = append(files, items...)
		return nil
	}

	// 1. Rules
	if err := appendFiles(a.prepareRuleMappings()); err != nil {
		return nil, err
	}
	// 2. Skills
	if err := appendFiles(a.prepareSkillMappings(cfg)); err != nil {
		return nil, err
	}
	// 3. Commands
	if err := appendFiles(a.prepareCommandMappings(cfg)); err != nil {
		return nil, err
	}
	// 4. Native context bridge (explicit OMP context-policy opt-in only)
	if err := appendFiles(prepareOMPContextBridgeMappings(cfg)); err != nil {
		return nil, err
	}
	// 5. Runtime/model claims. A plain OMP install needs no base config.
	if modelIntegration != nil {
		var configMapping adapter.FileMapping
		if modelIntegration.profile.ConfigMode == config.RoleModelConfigModeProjectManaged {
			configMapping, err = a.prepareConfigMappingAt(workspace, cfg)
			if err != nil {
				return nil, err
			}
		}
		configMapping, runtimeMappings, finalizeErr := modelIntegration.finalize(ctx, a, workspace, configMapping)
		if finalizeErr != nil {
			return nil, finalizeErr
		}
		if configMapping.TargetPath != "" {
			files = append(files, configMapping)
		}
		files = append(files, runtimeMappings...)
	}

	return files, nil
}

// ompConfigFileMode is the permission a freshly created .omp/config.yml gets.
// The file carries provider settings and can hold credentials, so it starts
// owner-only instead of world readable.
const ompConfigFileMode = os.FileMode(0600)

// Package opencode implements the OpenCode platform adapter.
package opencode

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

const (
	markerBegin = "<!-- AUTOPUS:BEGIN -->"
	markerEnd   = "<!-- AUTOPUS:END -->"
	adapterName = "opencode"
	cliBinary   = "opencode"
	adapterVer  = "1.0.0"
	configFile  = "opencode.json"
)

// Adapter is the OpenCode platform adapter.
type Adapter struct {
	root          string
	engine        *tmpl.Engine
	pinnedVersion *string
	versionOnce   sync.Once
	major         int
	versionKnown  bool
}

// New creates an adapter rooted at the current directory.
func New() *Adapter {
	return NewWithRoot(".")
}

// NewWithRoot creates an adapter rooted at the specified path.
func NewWithRoot(root string, options ...Option) *Adapter {
	a := &Adapter{root: root, engine: tmpl.New()}
	for _, option := range options {
		if option != nil {
			option(a)
		}
	}
	return a
}

func (a *Adapter) Name() string        { return adapterName }
func (a *Adapter) Version() string     { return adapterVer }
func (a *Adapter) CLIBinary() string   { return cliBinary }
func (a *Adapter) SupportsHooks() bool { return true }

// Detect checks whether the opencode binary is installed in PATH.
func (a *Adapter) Detect(_ context.Context) (bool, error) {
	_, err := exec.LookPath(cliBinary)
	return err == nil, nil
}

// Generate creates OpenCode platform files based on the harness config.
func (a *Adapter) Generate(ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	files, err := a.prepareFiles(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := writeMappings(a.root, files); err != nil {
		return nil, err
	}

	pf := &adapter.PlatformFiles{Files: files, Checksum: adapter.Checksum(fmt.Sprintf("%d", len(files)))}
	m := adapter.ManifestFromFiles(adapterName, pf)
	if err := m.Save(a.root); err != nil {
		return nil, fmt.Errorf("매니페스트 저장 실패: %w", err)
	}
	return pf, nil
}

// Update updates OpenCode files based on the manifest diff.
func (a *Adapter) Update(ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	oldManifest, err := adapter.LoadManifest(a.root, adapterName)
	if err != nil {
		return nil, fmt.Errorf("매니페스트 로드 실패: %w", err)
	}

	files, err := a.prepareFiles(ctx, cfg)
	if err != nil {
		return nil, err
	}
	plan, pf := a.buildUpdateTransactionPlan(oldManifest, files)
	if _, err := adapter.ApplyTransaction(a.root, adapterName, plan); err != nil {
		return nil, err
	}
	return pf, nil
}

// InstallHooks updates OpenCode plugin wiring for the provided hooks.
func (a *Adapter) InstallHooks(_ context.Context, hooks []adapter.HookConfig, _ *adapter.PermissionSet) error {
	if err := a.validateRuntime(); err != nil {
		return err
	}
	mapping, err := a.prepareHookPluginMapping(hooks)
	if err != nil {
		return err
	}
	configMapping, err := a.prepareConfigMapping()
	if err != nil {
		return err
	}
	if err := writeMapping(a.root, mapping); err != nil {
		return err
	}
	return writeMapping(a.root, configMapping)
}

func writeMappings(root string, files []adapter.FileMapping) error {
	for _, file := range files {
		if err := writeMapping(root, file); err != nil {
			return err
		}
	}
	return nil
}

func writeMapping(root string, file adapter.FileMapping) error {
	targetPath := filepath.Join(root, file.TargetPath)
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("디렉터리 생성 실패 %s: %w", filepath.Dir(targetPath), err)
	}
	perm := os.FileMode(0644)
	if isExecutablePath(file.TargetPath) {
		perm = 0755
	}
	if file.OverwritePolicy == adapter.OverwriteMerge {
		if err := adapter.WriteFileIfChanged(targetPath, file.Content, perm); err != nil {
			return fmt.Errorf("파일 쓰기 실패 %s: %w", file.TargetPath, err)
		}
		return nil
	}
	if err := os.WriteFile(targetPath, file.Content, perm); err != nil {
		return fmt.Errorf("파일 쓰기 실패 %s: %w", file.TargetPath, err)
	}
	return nil
}

func isExecutablePath(path string) bool {
	return filepath.Dir(path) == filepath.Join(".git", "hooks")
}

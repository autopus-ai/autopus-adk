// Package codex implements the Codex platform adapter.
package codex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

const (
	adapterName = "codex"
	cliBinary   = "codex"
	adapterVer  = "1.0.0"
)

// Adapter is the Codex platform adapter.
type Adapter struct {
	root                string
	engine              *tmpl.Engine
	codexCatalogProbed  bool
	codexCatalogJSON    []byte
	codexVersionProbed  bool
	codexVersion        string
	codexFallbackWriter io.Writer
	codexFallbackSeen   map[string]struct{}
	pluginBaseVersion   *string
}

// Option customizes a Codex adapter.
type Option func(*Adapter)

// WithModelCatalog uses a fixed model catalog instead of probing the Codex CLI.
func WithModelCatalog(catalogJSON []byte) Option {
	return func(a *Adapter) {
		a.codexCatalogProbed = true
		a.codexCatalogJSON = append([]byte(nil), catalogJSON...)
	}
}

// WithCLIVersion pins the `codex --version` response instead of probing the
// CLI. An empty version marks the release as probed but unreadable, which is
// how the generation path expresses "namespace chosen from documentation".
func WithCLIVersion(version string) Option {
	return func(a *Adapter) {
		a.codexVersionProbed = true
		a.codexVersion = version
	}
}

// WithPluginBaseVersion pins the generator version the plugin manifest takes
// its semver base from, instead of the version this binary was built with. A
// hermetic evaluation uses it so every build of one source tree produces the
// same surface; the value is normalized exactly like a build version.
func WithPluginBaseVersion(generatorVersion string) Option {
	return func(a *Adapter) {
		a.pluginBaseVersion = &generatorVersion
	}
}

// New creates an adapter rooted at the current directory.
func New() *Adapter {
	return &Adapter{root: ".", engine: tmpl.New(), codexFallbackWriter: os.Stderr}
}

// NewWithRoot creates an adapter rooted at the specified path.
func NewWithRoot(root string, options ...Option) *Adapter {
	a := &Adapter{root: root, engine: tmpl.New(), codexFallbackWriter: os.Stderr}
	for _, option := range options {
		if option != nil {
			option(a)
		}
	}
	return a
}

func (a *Adapter) Name() string      { return adapterName }
func (a *Adapter) Version() string   { return adapterVer }
func (a *Adapter) CLIBinary() string { return cliBinary }

// SupportsHooks returns true. Codex supports hooks via .codex/hooks.json.
func (a *Adapter) SupportsHooks() bool { return true }

// Detect checks whether the codex binary is installed in PATH.
func (a *Adapter) Detect(_ context.Context) (bool, error) {
	_, err := exec.LookPath(cliBinary)
	return err == nil, nil
}

// Generate prepares all Codex mappings before applying one transaction.
func (a *Adapter) Generate(ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	return a.applyPreparedFiles(ctx, cfg)
}

// Update applies the same prepared transaction path as Generate.
func (a *Adapter) Update(ctx context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	return a.applyPreparedFiles(ctx, cfg)
}

func (a *Adapter) applyPreparedFiles(
	ctx context.Context,
	cfg *config.HarnessConfig,
) (*adapter.PlatformFiles, error) {
	a.prepareCodexCatalog(ctx)
	a.prepareCodexVersion(ctx)
	oldManifest, err := adapter.LoadManifest(a.root, adapterName)
	if err != nil {
		return nil, fmt.Errorf("매니페스트 로드 실패: %w", err)
	}
	newFiles, err := a.prepareFilesWithManifest(cfg, oldManifest)
	if err != nil {
		return nil, err
	}
	plan, files, err := a.buildUpdateTransactionPlan(cfg, oldManifest, newFiles)
	if err != nil {
		return nil, err
	}
	if _, err := adapter.ApplyTransaction(a.root, adapterName, plan); err != nil {
		return nil, err
	}
	return files, nil
}

func checksum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func codexOwnsRootDoc(cfg *config.HarnessConfig) bool {
	return cfg == nil || !containsPlatform(cfg.Platforms, "opencode")
}

func filterCodexManifestFiles(cfg *config.HarnessConfig, pf *adapter.PlatformFiles) *adapter.PlatformFiles {
	if pf == nil || codexOwnsRootDoc(cfg) {
		return pf
	}
	filtered := make([]adapter.FileMapping, 0, len(pf.Files))
	for _, file := range pf.Files {
		if file.TargetPath != "AGENTS.md" {
			filtered = append(filtered, file)
		}
	}
	return &adapter.PlatformFiles{Files: filtered, Checksum: pf.Checksum}
}

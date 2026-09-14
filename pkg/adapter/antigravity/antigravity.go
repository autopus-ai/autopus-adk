// Package antigravity implements the Antigravity CLI platform adapter.
package antigravity

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	tmpl "github.com/insajin/autopus-adk/pkg/template"
)

const (
	adapterName       = "antigravity-cli"
	legacyAdapterName = "gemini-cli"
	cliBinary         = "agy"
	adapterVer        = "1.0.0"
)

// Adapter is the Antigravity CLI platform adapter.
//
// Generation is project-local. A plugin under `.agents/plugins/` is a
// workspace customization `agy` discovers on its own, so the adapter never
// stages a second copy into the user's global registry: that copy would carry
// this project's rendered content into every other project and shadow-collide
// with the workspace bundle on the shared `autopus` name.
type Adapter struct {
	root   string
	engine *tmpl.Engine
}

// New creates an adapter rooted at the current directory.
func New() *Adapter {
	return &Adapter{root: ".", engine: tmpl.New()}
}

// NewWithRoot creates an adapter rooted at the specified path.
func NewWithRoot(root string) *Adapter {
	return &Adapter{root: root, engine: tmpl.New()}
}

func (a *Adapter) Name() string      { return adapterName }
func (a *Adapter) Version() string   { return adapterVer }
func (a *Adapter) CLIBinary() string { return cliBinary }

// SupportsHooks returns true. Antigravity CLI supports hooks via settings.json.
func (a *Adapter) SupportsHooks() bool { return true }

// Detect checks whether the agy binary is installed on PATH.
func (a *Adapter) Detect(_ context.Context) (bool, error) {
	_, err := exec.LookPath(cliBinary)
	return err == nil, nil
}

// Generate creates Antigravity CLI files based on the harness config.
func (a *Adapter) Generate(_ context.Context, cfg *config.HarnessConfig) (*adapter.PlatformFiles, error) {
	antigravitySkillDir := filepath.Join(a.root, ".gemini", "skills", "autopus")
	if err := os.MkdirAll(antigravitySkillDir, 0755); err != nil {
		return nil, fmt.Errorf(".gemini/skills/autopus 디렉터리 생성 실패: %w", err)
	}
	var files []adapter.FileMapping

	geminiMD, err := a.injectMarkerSection(cfg)
	if err != nil {
		return nil, fmt.Errorf("GEMINI.md 마커 주입 실패: %w", err)
	}

	geminiMDPath := filepath.Join(a.root, "GEMINI.md")
	if err := os.WriteFile(geminiMDPath, []byte(geminiMD), 0644); err != nil {
		return nil, fmt.Errorf("GEMINI.md 쓰기 실패: %w", err)
	}
	files = append(files, adapter.FileMapping{
		TargetPath:      "GEMINI.md",
		OverwritePolicy: adapter.OverwriteMarker,
		Checksum:        checksum(geminiMD),
		Content:         []byte(geminiMD),
	})

	pluginFiles, err := prepareAntigravityPluginJSON()
	if err != nil {
		return nil, fmt.Errorf("antigravity plugin manifest 생성 실패: %w", err)
	}
	for _, pf := range pluginFiles {
		destPath := filepath.Join(a.root, pf.TargetPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("antigravity plugin 디렉터리 생성 실패: %w", err)
		}
		if err := os.WriteFile(destPath, pf.Content, 0644); err != nil {
			return nil, fmt.Errorf("antigravity plugin 파일 쓰기 실패 %s: %w", destPath, err)
		}
	}
	files = append(files, pluginFiles...)

	skillFiles, err := a.renderSkillTemplates(cfg, antigravitySkillDir)
	if err != nil {
		return nil, fmt.Errorf("제미니 스킬 템플릿 렌더링 실패: %w", err)
	}
	files = append(files, skillFiles...)

	cmdFiles, err := a.renderCommandTemplates(cfg)
	if err != nil {
		return nil, fmt.Errorf("제미니 커맨드 템플릿 렌더링 실패: %w", err)
	}
	files = append(files, cmdFiles...)

	ruleFiles, err := a.renderRuleTemplates(cfg)
	if err != nil {
		return nil, fmt.Errorf("제미니 룰 템플릿 렌더링 실패: %w", err)
	}
	files = append(files, ruleFiles...)

	// Copy agent content files (full mode)
	if cfg.IsFullMode() {
		agentFiles, err := a.renderAgentFiles()
		if err != nil {
			return nil, fmt.Errorf("제미니 에이전트 파일 복사 실패: %w", err)
		}
		files = append(files, agentFiles...)
	}

	// Generate settings.json with hooks and permissions already merged.
	settingsFiles, err := a.generateSettingsWithHooks(cfg)
	if err != nil {
		return nil, fmt.Errorf("제미니 설정 생성 실패: %w", err)
	}
	for _, sf := range settingsFiles {
		destPath := filepath.Join(a.root, sf.TargetPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return nil, fmt.Errorf("설정 디렉터리 생성 실패: %w", err)
		}
		if err := os.WriteFile(destPath, sf.Content, 0644); err != nil {
			return nil, fmt.Errorf("설정 파일 쓰기 실패: %w", err)
		}
	}
	files = append(files, settingsFiles...)

	routerFiles, err := a.renderRouterCommand(cfg)
	if err != nil {
		return nil, fmt.Errorf("제미니 라우터 템플릿 렌더링 실패: %w", err)
	}
	files = append(files, routerFiles...)

	statusFiles, err := a.copyStatusline()
	if err != nil {
		return nil, fmt.Errorf("제미니 statusline 복사 실패: %w", err)
	}
	files = append(files, statusFiles...)

	hookConfigs := a.configuredAntigravityHooks(cfg)
	hookFiles, err := a.writeAntigravityHooksJSON(hookConfigs)
	if err != nil {
		return nil, err
	}
	files = append(files, hookFiles...)

	resourceFiles, err := preparePluginSkillResources(files, cfg)
	if err != nil {
		return nil, err
	}
	files = append(files, resourceFiles...)

	files, err = rewriteSanitizedAntigravityMappings(a.root, files)
	if err != nil {
		return nil, err
	}
	completionHookAssets, err := prepareAntigravityCompletionHookAssets()
	if err != nil {
		return nil, err
	}
	rollbackHooks, err := applyAntigravityManagedHookAssets(a.root, completionHookAssets)
	if err != nil {
		return nil, err
	}
	files = append(files, completionHookAssets...)

	pf := &adapter.PlatformFiles{
		Files:    files,
		Checksum: checksum(geminiMD),
	}

	m := adapter.ManifestFromFiles(adapterName, pf)
	if err := m.Save(a.root); err != nil {
		return nil, errors.Join(fmt.Errorf("매니페스트 저장 실패: %w", err), rollbackHooks())
	}

	return pf, nil
}

// Update updates files based on the manifest.
// Update and prepareFiles moved to antigravity_update.go to keep antigravity.go under
// the 300-line file size limit.

// prepareStatusline reads statusline.sh from embedded FS and returns a FileMapping.
func (a *Adapter) prepareStatusline() ([]adapter.FileMapping, error) {
	data, err := contentfs.FS.ReadFile("statusline.sh")
	if err != nil {
		return nil, fmt.Errorf("statusline.sh 읽기 실패: %w", err)
	}
	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(".gemini", "statusline.sh"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        checksum(string(data)),
		Content:         data,
	}}, nil
}

// copyStatusline copies statusline.sh to the target project.
func (a *Adapter) copyStatusline() ([]adapter.FileMapping, error) {
	data, err := contentfs.FS.ReadFile("statusline.sh")
	if err != nil {
		return nil, fmt.Errorf("statusline.sh 읽기 실패: %w", err)
	}
	destPath := filepath.Join(a.root, ".gemini", "statusline.sh")
	if err := os.WriteFile(destPath, data, 0755); err != nil {
		return nil, fmt.Errorf("statusline.sh 쓰기 실패: %w", err)
	}
	return []adapter.FileMapping{{
		TargetPath:      filepath.Join(".gemini", "statusline.sh"),
		OverwritePolicy: adapter.OverwriteAlways,
		Checksum:        checksum(string(data)),
		Content:         data,
	}}, nil
}

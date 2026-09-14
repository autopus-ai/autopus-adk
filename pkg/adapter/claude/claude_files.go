package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	contentfs "github.com/insajin/autopus-adk/content"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

var claudeRootHookFiles = []string{
	"task-created-validate.sh",
	"README.md",
}

// prepareMCPConfig는 .mcp.json 내용을 준비한다 (디스크 쓰기 없음).
// @AX:WARN [AUTO]: MCP configuration merge contains more than eight conditional branches.
// @AX:REASON [AUTO]: template admission, existing JSON preservation, managed-server replacement, malformed user structure rejection, and serialization converge here.
func (a *Adapter) prepareMCPConfig(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	tmplContent, err := templates.FS.ReadFile("claude/mcp.json.tmpl")
	if err != nil {
		return nil, fmt.Errorf("MCP 템플릿 읽기 실패: %w", err)
	}

	rendered, err := a.engine.RenderString(string(tmplContent), cfg)
	if err != nil {
		return nil, fmt.Errorf("MCP 템플릿 렌더링 실패: %w", err)
	}

	// Parse rendered JSON
	var newMCP map[string]interface{}
	if err := json.Unmarshal([]byte(rendered), &newMCP); err != nil {
		return nil, fmt.Errorf("MCP JSON 파싱 실패: %w", err)
	}

	// Merge with existing .mcp.json to preserve user servers and root keys.
	targetPath := filepath.Join(a.root, ".mcp.json")
	if data, readErr := os.ReadFile(targetPath); readErr == nil {
		var existing map[string]interface{}
		if err := json.Unmarshal(data, &existing); err != nil {
			return nil, fmt.Errorf("기존 MCP JSON 파싱 실패: %w", err)
		}
		if existing == nil {
			existing = make(map[string]interface{})
		}
		existingServers := make(map[string]interface{})
		if raw, exists := existing["mcpServers"]; exists {
			typed, ok := raw.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("기존 MCP mcpServers 필드는 객체여야 함")
			}
			existingServers = typed
		}
		for name, server := range existingServers {
			if isManagedClaudeMCPServer(name, server) {
				delete(existingServers, name)
			}
		}
		newServers, _ := newMCP["mcpServers"].(map[string]interface{})
		for name, server := range newServers {
			existingServers[name] = server
		}
		existing["mcpServers"] = existingServers
		newMCP = existing
	} else if !os.IsNotExist(readErr) {
		return nil, fmt.Errorf("기존 MCP JSON 읽기 실패: %w", readErr)
	}

	out, err := json.MarshalIndent(newMCP, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("MCP JSON 직렬화 실패: %w", err)
	}
	outStr := string(out) + "\n"

	return []adapter.FileMapping{{
		TargetPath:      ".mcp.json",
		OverwritePolicy: adapter.OverwriteMerge,
		Checksum:        checksum(outStr),
		Content:         []byte(outStr),
	}}, nil
}

func claudeSkillCatalog(subDir string, cfg *config.HarnessConfig) (*pkgcontent.SkillCatalog, error) {
	if subDir != "skills" || cfg == nil {
		return nil, nil
	}
	catalog, err := pkgcontent.LoadSkillCatalogFromFS(contentfs.FS, "skills")
	if err != nil {
		return nil, fmt.Errorf("Claude skill catalog init: %w", err)
	}
	return catalog, nil
}

// claudeSkillResourceMappings returns the reference bodies installed beside a
// generated skill. A compacted SKILL.md links to them with the relative
// references/<file>.md form, so they have to land in the skill's own directory
// or the link points at nothing.
func claudeSkillResourceMappings(name, skillDir string, cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	resources, err := pkgcontent.RenderSkillResources(name, "claude", cfg)
	if err != nil {
		return nil, fmt.Errorf("Claude skill resource render %s: %w", name, err)
	}

	names := pkgcontent.SkillResourceNames(resources)
	files := make([]adapter.FileMapping, 0, len(names))
	for _, rel := range names {
		data := resources[rel]
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(skillDir, filepath.FromSlash(rel)),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(string(data)),
			Content:         data,
		})
	}
	return files, nil
}

// @AX:NOTE [AUTO]: Unknown embedded skills remain compiled for backward compatibility; catalog entries obey bundle state.
func claudeSkillCompiled(catalog *pkgcontent.SkillCatalog, filename string, cfg *config.HarnessConfig) bool {
	if catalog == nil {
		return true
	}
	name := strings.TrimSuffix(filename, filepath.Ext(filename))
	skill, ok := catalog.Get(name)
	return !ok || pkgcontent.ResolveCatalogSkillState(skill, "claude", cfg).Compiled
}

func isClaudeRootHookFile(name string) bool {
	for _, candidate := range claudeRootHookFiles {
		if candidate == name {
			return true
		}
	}
	return false
}

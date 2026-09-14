package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/templates"
)

const agentsTemplateDir = "codex/agents"

// generateAgents renders TOML agent templates and writes to .codex/agents/.
func (a *Adapter) generateAgents(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	var files []adapter.FileMapping

	entries, err := templates.FS.ReadDir(agentsTemplateDir)
	if err != nil {
		return nil, fmt.Errorf("codex agent 템플릿 디렉터리 읽기 실패: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		name := entry.Name()
		agentFile := strings.TrimSuffix(name, ".tmpl")

		tmplContent, err := templates.FS.ReadFile(agentsTemplateDir + "/" + name)
		if err != nil {
			return nil, fmt.Errorf("codex agent 템플릿 읽기 실패 %s: %w", name, err)
		}

		rendered, err := a.engine.RenderString(string(tmplContent), a.codexRenderData(cfg))
		if err != nil {
			return nil, fmt.Errorf("codex agent 템플릿 렌더링 실패 %s: %w", name, err)
		}
		rendered = normalizeCodexHelperPaths(rendered)
		rendered = normalizeCodexToolingBody(rendered)
		rendered = normalizeCodexAgentContracts(rendered)

		targetPath := filepath.Join(a.root, ".codex", "agents", agentFile)
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return nil, fmt.Errorf(".codex/agents 디렉터리 생성 실패: %w", err)
		}
		if err := os.WriteFile(targetPath, []byte(rendered), 0644); err != nil {
			return nil, fmt.Errorf("codex agent 파일 쓰기 실패 %s: %w", targetPath, err)
		}

		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(".codex", "agents", agentFile),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(rendered),
			Content:         []byte(rendered),
		})
	}

	return files, nil
}

// prepareAgentFiles returns agent file mappings without writing to disk.
func (a *Adapter) prepareAgentFiles(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	var files []adapter.FileMapping

	entries, err := templates.FS.ReadDir(agentsTemplateDir)
	if err != nil {
		return nil, fmt.Errorf("codex agent 템플릿 디렉터리 읽기 실패: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tmpl") {
			continue
		}

		name := entry.Name()
		agentFile := strings.TrimSuffix(name, ".tmpl")

		tmplContent, err := templates.FS.ReadFile(agentsTemplateDir + "/" + name)
		if err != nil {
			return nil, fmt.Errorf("codex agent 템플릿 읽기 실패 %s: %w", name, err)
		}

		rendered, err := a.engine.RenderString(string(tmplContent), a.codexRenderData(cfg))
		if err != nil {
			return nil, fmt.Errorf("codex agent 템플릿 렌더링 실패 %s: %w", name, err)
		}
		rendered = normalizeCodexHelperPaths(rendered)
		rendered = normalizeCodexToolingBody(rendered)
		rendered = normalizeCodexAgentContracts(rendered)

		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(".codex", "agents", agentFile),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(rendered),
			Content:         []byte(rendered),
		})
	}

	return files, nil
}

func normalizeCodexAgentContracts(rendered string) string {
	rendered = normalizeCodexNativeSkillReferences(rendered)
	hasReceipt := strings.Contains(rendered, "`owned_paths`") &&
		strings.Contains(rendered, "`changed_files`") &&
		strings.Contains(rendered, "`verification`") &&
		strings.Contains(rendered, "`blockers`") &&
		strings.Contains(rendered, "`next_required_step`")
	hasV2 := strings.Contains(rendered, codexV2ContractHeading)
	if hasReceipt && hasV2 {
		return rendered
	}

	var contract strings.Builder
	if !hasV2 {
		contract.WriteString(`
## Codex Multi-Agent V2 Contract

When coordinating other workers, use exactly ` + "`spawn_agent(task_name, message, ...)`" + `,
` + "`send_message(...)`" + `, ` + "`followup_task(...)`" + `, target-less
` + "`wait_agent()`" + `, ` + "`interrupt_agent(...)`" + `, and ` + "`list_agents()`" + `.
All workers use the same shared cwd and filesystem. Parallel writers require
disjoint write ownership; otherwise schedule them sequentially. Use
` + "`@auto`" + ` or ` + "`$codex-auto`" + ` for routing and load detailed
` + "`$codex-auto-<route>`" + ` or ` + "`$codex-<skill>`" + ` contracts as needed.
`)
	}
	if !hasReceipt {
		contract.WriteString(`
## Supervisor Return Contract

When spawned by a supervisor, the final response MUST include exactly:

- ` + "`owned_paths`" + `: exact files, directories, or modules the worker owned
- ` + "`changed_files`" + `: files actually changed, or ` + "`none`" + `
- ` + "`verification`" + `: commands, checks, or inspections run, including failures
- ` + "`blockers`" + `: unresolved blockers, or ` + "`none`" + `
- ` + "`next_required_step`" + `: the next gate, retry, handoff, or ` + "`none`" + `
`)
	}
	anchor := "\n'''"
	if idx := strings.LastIndex(rendered, anchor); idx >= 0 {
		return rendered[:idx] + contract.String() + rendered[idx:]
	}
	return strings.TrimRight(rendered, "\n") + "\n" + strings.TrimLeft(contract.String(), "\n")
}

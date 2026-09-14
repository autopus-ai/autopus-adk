package claude

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

type claudeWorkflowRoute struct {
	name       string
	start      string
	end        string
	context    string
	customBody string
}

var claudeWorkflowRoutes = []claudeWorkflowRoute{
	{name: "setup", start: "## setup —", end: "## idea —", context: "core + architecture"},
	{name: "status", start: "## status —", end: "## dev —", context: "core + SPEC index"},
	{name: "goal", context: "core", customBody: claudeGoalContract},
	{name: "update", context: "core + workspace", customBody: claudeUpdateContract},
	{name: "plan", start: "## plan —", end: "## go —", context: "core + architecture + relevant SPEC evidence"},
	{name: "go", start: "## go —", end: "## verify —", context: "core + selected SPEC + acceptance"},
	{name: "fix", start: "## fix —", end: "## map —", context: "core + changed scope"},
	{name: "review", start: "## review —", end: "## secure —", context: "core + changed scope + acceptance"},
	{name: "sync", start: "## sync —", end: "## why —", context: "core + selected SPEC + workspace"},
	{name: "idea", start: "## idea —", end: "## plan —", context: "core + relevant product evidence"},
	{name: "map", start: "## map —", end: "## review —", context: "core + architecture"},
	{name: "why", start: "## why —", end: "## status —", context: "core + decision references"},
	{name: "verify", start: "## verify —", end: "## browse —", context: "core + changed UI scope"},
	{name: "secure", start: "## secure —", end: "## stale —", context: "core + changed scope + security constraints"},
	{name: "test", start: "## test —", end: "## qa —", context: "core + scenarios"},
	{name: "qa", start: "## qa —", end: "## canary —", context: "core + declared QA journey evidence"},
	{name: "dev", start: "## dev —", end: "## ADK Management —", context: "core + selected SPEC lifecycle"},
	{name: "canary", start: "## canary —", end: "## fix —", context: "core + canary"},
	{name: "doctor", context: "core + workspace", customBody: claudeDoctorContract},
}

func isClaudeWorkflowDetailSource(filename string) bool {
	for _, route := range claudeWorkflowRoutes {
		if filename == "auto-"+route.name+".md" {
			return true
		}
	}
	return false
}

// @AX:ANCHOR [AUTO]: preserve the Claude workflow-skill mapping boundary.
// @AX:REASON [AUTO]: two generation paths and a regression test consume the same rendered file mappings.
func (a *Adapter) prepareWorkflowSkillMappings(cfg *config.HarnessConfig) ([]adapter.FileMapping, error) {
	source, err := templates.FS.ReadFile("claude/commands/auto-workflows.md.tmpl")
	if err != nil {
		return nil, fmt.Errorf("상세 workflow source 읽기 실패: %w", err)
	}
	rendered, err := a.engine.RenderString(string(source), cfg)
	if err != nil {
		return nil, fmt.Errorf("상세 workflow source 렌더링 실패: %w", err)
	}

	files := make([]adapter.FileMapping, 0, len(claudeWorkflowRoutes))
	for _, route := range claudeWorkflowRoutes {
		body := route.customBody
		if body == "" {
			body, err = extractClaudeWorkflowSection(rendered, route.start, route.end)
			if err != nil {
				return nil, fmt.Errorf("auto-%s 상세 contract 생성 실패: %w", route.name, err)
			}
		}
		content := renderClaudeWorkflowDetail(route, body)
		outputName := pkgcontent.ResolveCatalogSkillOutputName("auto-"+route.name, "claude")
		files = append(files, adapter.FileMapping{
			TargetPath:      filepath.Join(".claude", "skills", outputName, "SKILL.md"),
			OverwritePolicy: adapter.OverwriteAlways,
			Checksum:        checksum(content),
			Content:         []byte(content),
		})
	}
	return files, nil
}

// @AX:ANCHOR [AUTO]: keep Claude workflow section extraction semantics stable.
// @AX:REASON [AUTO]: the mapping generator and two marker-failure regression call sites depend on its boundary and errors.
func extractClaudeWorkflowSection(source, startMarker, endMarker string) (string, error) {
	start := strings.Index(source, startMarker)
	if start < 0 {
		return "", fmt.Errorf("시작 marker %q 없음", startMarker)
	}
	end := len(source)
	if endMarker != "" {
		relEnd := strings.Index(source[start+len(startMarker):], endMarker)
		if relEnd < 0 {
			return "", fmt.Errorf("종료 marker %q 없음", endMarker)
		}
		end = start + len(startMarker) + relEnd
	}
	return strings.TrimSpace(source[start:end]), nil
}

func renderClaudeWorkflowDetail(route claudeWorkflowRoute, body string) string {
	return fmt.Sprintf(`---
name: auto-%s
description: Lazy-loaded Autopus %s workflow contract
---

# auto-%s

## Context Profile

%s

## Contract

%s
`, route.name, route.name, route.name, renderClaudeContextProfile(route), body)
}

func renderClaudeContextProfile(route claudeWorkflowRoute) string {
	switch route.name {
	case "plan":
		return `- Required: core,architecture,relevant_spec
- Optional: signature,learning
- Excluded: test,canary

### Context Mapping

- Required: core workspace policy, architecture, and relevant SPEC evidence.
- Conditional: signatures and learnings only when explicitly declared by the route or task.
- Excluded by default: scenarios and canary.`
	case "test":
		return `- Required: core,test
- Optional: signature,learning
- Excluded: canary

### Context Mapping

- Required: core workspace policy and scenarios.
- Scenarios map to the test context.`
	case "canary":
		return `- Required: core,canary
- Optional: learning
- Excluded: test,signature

### Context Mapping

- Required: core workspace policy, canary, and the declared canary command.
- Excluded: scenarios, signatures, and unrelated learnings.`
	case "go":
		return `- Supervisor Required: core,resolved_spec,plan,acceptance
- Worker Optional: signature,learning,task_declared_extra
- Excluded: test,canary`
	default:
		return fmt.Sprintf("- Required: %s.\n- Load only route-relevant evidence; unrelated optional project documents remain excluded.", route.context)
	}
}

const claudeGoalContract = `- This is a thin wrapper over the Codex thread goal feature; never persist goal state in project files.
- status: use get_goal and report objective, status, budget, and next required step.
- create: check for an active goal, then call create_goal with the explicit objective and optional token budget only.
- complete: call update_goal(status="complete") only when the objective is achieved and no required work remains.
- blocked: call update_goal(status="blocked") only after the same blocking condition recurs for at least three consecutive goal turns.
- clear, pause, or resume: use the matching /goal slash command when supported; do not invent local behavior.
- If goal tools are unavailable, state the runtime limitation and provide the matching /goal fallback.`

const claudeUpdateContract = `1. Detect whether the current directory is the ADK source, a product repo, or a meta workspace.
2. Preserve user-owned provider/model configuration and forward all update flags unchanged.
3. Run the canonical update workflow. Never patch installed generated, plugin-cache, or runtime surfaces directly.
4. Verify Generate/Update parity, manifest health, and tracked-but-ignored drift.
5. Report canonical source changes separately from local generated reflections.`

const claudeDoctorContract = `1. Run the harness doctor command with every user argument unchanged.
2. Inspect canonical source availability, platform wiring, rules, plugins, hooks, dependencies, and manifests.
3. Classify source defects separately from generated/runtime drift and user-owned configuration.
4. Report PASS, WARN, or FAIL with exact evidence and a safe repair command.
5. Do not mutate files unless the user explicitly requested repair.`

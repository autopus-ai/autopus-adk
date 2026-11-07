package codex

import (
	"fmt"
	"strings"
)

type customWorkflowBody struct {
	skill string
}

func renderCustomWorkflowSkill(spec workflowSpec) (string, bool) {
	body, ok := customWorkflowBodies(spec)
	if !ok {
		return "", false
	}
	frontmatter := fmt.Sprintf("---\nname: %s\ndescription: >\n  %s\n---", spec.Name, spec.Description)
	return frontmatter + "\n\n" + strings.TrimSpace(injectCodexBrandingBlock(body.skill, false)) + "\n", true
}

// @AX:NOTE: [AUTO] hardcoded workflow dispatch table — adding a new auto-* workflow requires a case here; this switch is the single source of truth for custom workflow rendering
func customWorkflowBodies(spec workflowSpec) (customWorkflowBody, bool) {
	switch spec.Name {
	case "auto-update":
		return updateWorkflowBody(spec.Name, spec.Description), true
	case "auto-verify":
		return cliWorkflowBody(spec.Name, "Frontend UX Verification", spec.Description, "auto verify", "Playwright 기반 검증 결과와 자동 수정 가능 여부를 함께 보고합니다."), true
	case "auto-test":
		return cliWorkflowBody(spec.Name, "E2E Scenario Runner", spec.Description, "auto test run", "scenario별 PASS / FAIL 결과를 정리하고 실패 시 다음 복구 액션을 제안합니다."), true
	case "auto-doctor":
		return cliWorkflowBody(spec.Name, "Harness Diagnostics", spec.Description, "auto doctor", "platform wiring, rules, plugins, dependencies 상태를 요약하고 fix 필요 시 명시합니다."), true
	default:
		return customWorkflowBody{}, false
	}
}

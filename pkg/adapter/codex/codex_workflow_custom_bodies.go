package codex

import "strings"

func cliWorkflowBody(name, title, summary, command, result string) customWorkflowBody {

	skill := compose(
		"# "+name+" — "+title,
		"",
		"## 설명",
		"",
		summary,
		"",
		"## Codex Invocation",
		"",
		"- `@auto "+strings.TrimPrefix(name, "auto-")+" ...`",
		"- `$"+name+" ...`",
		"- `$auto "+strings.TrimPrefix(name, "auto-")+" ...`",
		"",
		"## 실행 순서",
		"",
		"1. 대상 디렉터리와 전달된 플래그를 확인합니다.",
		"2. Bash tool로 `"+command+"`를 실행합니다.",
		"3. "+result,
	)

	return customWorkflowBody{skill: skill}
}

func compose(lines ...string) string {
	return strings.Join(lines, "\n")
}

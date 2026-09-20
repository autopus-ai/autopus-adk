package opencode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const openCodeDefaultModel = "openai/gpt-5.4"

func splitFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[4:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content
	}
	frontmatter := rest[:idx]
	body := rest[idx+4:]
	body = strings.TrimPrefix(body, "\n")
	return frontmatter, body
}

func buildMarkdown(frontmatter, body string) string {
	if strings.TrimSpace(frontmatter) == "" {
		return strings.TrimSpace(body) + "\n"
	}
	return fmt.Sprintf("---\n%s\n---\n\n%s\n", strings.TrimSpace(frontmatter), strings.TrimSpace(body))
}

func normalizeOpenCodeMarkdown(content string) string {
	replacer := strings.NewReplacer(
		"@auto ", "/auto ",
		"@auto-", "/auto-",
		"## Codex Notes", "## OpenCode Notes",
		"## Codex 기본 실행 모델", "## OpenCode 기본 실행 모델",
		"Codex에서는", "OpenCode에서는",
		"Codex의", "OpenCode의",
		"Codex는", "OpenCode는",
		"Codex에서", "OpenCode에서",
		"Codex 런타임 정책", "OpenCode 런타임 정책",
		"Codex 런타임이", "OpenCode 런타임이",
		"- `--team`: `.codex/skills/codex-agent-teams/SKILL.md`의 Codex team profile 적용",
		"- `--team`: verify the requested native team lifecycle in the current runtime; if unavailable, stop with unsupported-mode and do not substitute the OpenCode task-based pipeline.",
		"spawn_agent(...)", "task(...)",
		"`spawn_agent`", "`task`",
		".codex/skills/codex-agent-pipeline/SKILL.md", ".agents/skills/agent-pipeline/SKILL.md",
		".codex/skills/agent-pipeline.md", ".agents/skills/agent-pipeline/SKILL.md",
	)
	return replacer.Replace(content)
}

func normalizeOpenCodeSkillBody(body, subcommand string) string {
	body = normalizeOpenCodeMarkdown(body)
	body = normalizeOpenCodeToolingBody(body)
	body = rewriteOpenCodeWorkflowExamples(body, subcommand)
	body = augmentOpenCodeFlagDocs(body)
	if subcommand == "" {
		return body
	}

	replacer := strings.NewReplacer(
		"/auto-"+subcommand, "/auto "+subcommand,
		"@auto-"+subcommand, "/auto "+subcommand,
		"$auto-"+subcommand, "/auto "+subcommand,
	)
	return replacer.Replace(body)
}

func normalizeOpenCodeToolingBody(body string) string {
	replacer := strings.NewReplacer(
		"Agent(", "task(",
		"spawn_agent(", "task(",
		"spawn_agent ", "task ",
		"AskUserQuestion(", "question(",
		"request_user_input", "question",
		"TaskCreate(", "todowrite(",
		"TaskUpdate(", "todowrite(",
		"TaskList(", "todowrite(",
		"TaskGet(", "todowrite(",
		"TeamCreate(", "task(",
		"using `spawn_agent`", "using `task`",
		"using spawn_agent", "using task",
		"`spawn_agent(...)`", "`task(...)`",
		"`spawn_agent`", "`task`",
	)
	body = replacer.Replace(body)
	body = strings.ReplaceAll(body, "task = ", "prompt = ")
	body = strings.ReplaceAll(body, "task=", "prompt=")
	body = strings.ReplaceAll(body, "Task(\n", "task(\n")
	return body
}

func rewriteOpenCodeWorkflowExamples(body, subcommand string) string {
	if subcommand != "go" {
		return body
	}

	replacer := strings.NewReplacer(
		"```\ntask executor \\\n  --task \"Implement {task description}\" \\\n  --spec \"{SPEC_PATH}\" \\\n  --plan \"{SPEC_DIR}/plan.md\" \\\n  --constraint \"File size limit applies only to source code files; SPEC Markdown under .autopus/specs/** is exempt\"\n```",
		"```text\ntask(\n  subagent_type = \"executor\",\n  prompt = \"Implement {task description}. Use {SPEC_PATH} and {SPEC_DIR}/plan.md as context. Respect the 300-line source-code file limit; SPEC Markdown under .autopus/specs/** is exempt.\"\n)\n```",
		"```\ntask tester \\\n  --task \"Write tests for {scope}\" \\\n  --spec \".autopus/specs/{SPEC-ID}/acceptance.md\" \\\n  --coverage-target 85\n```",
		"```text\ntask(\n  subagent_type = \"tester\",\n  prompt = \"Write tests for {scope}. Use .autopus/specs/{SPEC-ID}/acceptance.md as context and target 85%% coverage.\"\n)\n```",
		"```\ntask reviewer \\\n  --task \"Review implementation for {SPEC-ID}\" \\\n  --criteria \"TRUST-5\"\n```",
		"```text\ntask(\n  subagent_type = \"reviewer\",\n  prompt = \"Review implementation for {SPEC-ID} using TRUST-5 criteria.\"\n)\n```",
	)
	return replacer.Replace(body)
}

func thinRouterCommandBody() string {
	return strings.TrimSpace("`$ARGUMENTS`\n\nTreat the text above as the full payload passed after `/auto`.\nImmediately load skill `auto` and use it as the canonical router.\nStrip global flags first, resolve the first remaining token as the subcommand, then load the matching `auto-*` skill.\nPreserve `--model <provider/model>` and `--variant <value>` when present.\nDo not restate or expand the arguments unless needed for execution.")
}

func thinWorkflowCommandBody(name string) string {
	subcommand := strings.TrimPrefix(name, "auto-")
	return strings.TrimSpace(fmt.Sprintf("`$ARGUMENTS`\n\nTreat the text above as the full argument payload for `/%s`.\nReinterpret it as `/auto %s ...` payload로 다시 해석하고, `skill` 도구로 `auto`를 로드한 뒤 canonical router 규칙을 따르세요.\nPreserve `--model <provider/model>` and `--variant <value>` when present.\nDo not restate or expand the arguments unless needed for execution.", name, subcommand))
}

func skillInvocationNote(name string) string {
	if name == "auto" {
		return "## OpenCode Invocation\n\n이 스킬은 다음 두 경로로 사용할 수 있습니다.\n\n- `/auto <subcommand> ...`\n- `skill` 도구로 직접 `auto` 로드\n\n직접 로드되면 사용자의 최신 요청을 `/auto` 뒤 인자로 간주하고 해석하세요.\n\n## OpenCode Model Override\n\n- 기본 세션 모델은 `" + openCodeDefaultModel + "` 입니다.\n- 사용자 오버라이드: `--model <provider/model>`\n- reasoning 오버라이드: `--variant <value>`\n- `--model` / `--variant`가 주어지면 이후 단계로 그대로 전달하고 자동으로 덮어쓰지 않습니다.\n"
	}
	subcommand := strings.TrimPrefix(name, "auto-")
	return fmt.Sprintf("## OpenCode Invocation\n\n이 스킬은 다음 두 경로로 사용할 수 있습니다.\n\n- `/auto %s ...`\n- `/%s ...`\n- `skill` 도구로 직접 `%s` 로드\n\n직접 로드되면 사용자의 최신 요청을 해당 명령의 인자로 간주하세요.\n\n## OpenCode Model Override\n\n- 기본 세션 모델은 `"+openCodeDefaultModel+"` 입니다.\n- 사용자 오버라이드: `--model <provider/model>`\n- reasoning 오버라이드: `--variant <value>`\n- `--model` / `--variant`가 주어지면 이후 단계로 그대로 전달하고 자동으로 덮어쓰지 않습니다.\n", subcommand, name, name)
}

func injectOpenCodeBrandingBlock(body string) string {
	if strings.Contains(body, "## Autopus Branding") {
		return body
	}

	block := strings.TrimSpace("## Autopus Branding\n\n" +
		"When handling this workflow, start the response with the canonical banner " +
		"(see `.opencode/rules/autopus/branding.md`):\n\n" +
		"```text\n" +
		"🐙 Autopus ─────────────────────────\n" +
		"```\n\n" +
		"End the completed response with `🐙`.")
	return injectAfterFirstHeading(body, block)
}

func augmentOpenCodeFlagDocs(body string) string {
	replacer := strings.NewReplacer(
		"- `--quality <mode>`: 하위 에이전트 품질 모드 지정", "- `--quality <mode>`: 하위 에이전트 품질 모드 지정\n- `--model <provider/model>`: OpenCode 세션/서브에이전트 모델 오버라이드\n- `--variant <value>`: provider-specific reasoning variant 오버라이드",
		"- `--quality <mode>`", "- `--quality <mode>`\n- `--model <provider/model>`\n- `--variant <value>`",
		"`--auto`, `--loop`, `--multi`, `--quality`, `--team`, `--solo`", "`--auto`, `--loop`, `--multi`, `--quality`, `--model`, `--variant`, `--team`, `--solo`",
		"`--auto`, `--loop`, `--team`, `--multi`, `--quality`", "`--auto`, `--loop`, `--team`, `--multi`, `--quality`, `--model`, `--variant`",
		"`--auto` / `--loop` / `--multi` / `--quality`", "`--auto` / `--loop` / `--multi` / `--quality` / `--model` / `--variant`",
	)
	body = replacer.Replace(body)
	body = strings.ReplaceAll(body, "OpenCode currently assumes `gpt-5.4` as the default runtime model; LOW/MEDIUM/HIGH act as reasoning-profile hints until explicit model overrides are surfaced", "OpenCode defaults to `gpt-5.4` when no explicit model override is provided; LOW/MEDIUM/HIGH act as reasoning-profile hints, and users can override with `--model`")
	body = strings.ReplaceAll(body, "OpenCode currently assumes `gpt-5.4` as the default runtime model and should vary reasoning effort rather than the model ID", "OpenCode defaults to `gpt-5.4` when no explicit model override is provided and should vary reasoning effort rather than the model ID")
	body = strings.ReplaceAll(body, "OpenCode: keep the platform default `gpt-5.4`; treat Layer 2 / Check 7 as higher-reasoning validation passes", "OpenCode: keep the platform default `gpt-5.4` unless the user passed `--model`; treat Layer 2 / Check 7 as higher-reasoning validation passes")
	return body
}

func injectAfterFirstHeading(body, block string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			out := make([]string, 0, len(lines)+4)
			out = append(out, lines[:i+1]...)
			out = append(out, "")
			out = append(out, block)
			out = append(out, "")
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return block + "\n\n" + body
}

func uniqueStrings(values ...[]string) []string {
	seen := map[string]bool{}
	var result []string
	for _, list := range values {
		for _, item := range list {
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	if result == nil {
		return map[string]any{}, nil
	}
	return result, nil
}

func jsonStringSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

func jsonPluginSlice(value any) []string {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			result = append(result, typed)
		case []any:
			if len(typed) == 0 {
				continue
			}
			if str, ok := typed[0].(string); ok {
				result = append(result, str)
			}
		}
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func toSlash(path string) string {
	return filepath.ToSlash(path)
}

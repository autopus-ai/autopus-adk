package omp

import (
	"fmt"
	"strings"

	pkgcontent "github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/templates"
)

func ompRouterBody(prefix string) string {
	return prefix + strings.Join([]string{
		"`$ARGUMENTS`",
		"",
		templates.TaskTriagePolicy(),
		"",
		"## Router Contract",
		"",
		"Treat the payload above as the complete text supplied after `/auto`.",
		"Route to exactly one matching detail skill from this exact map: " + ompWorkflowTargets() + ".",
		"Preserve `--model <provider/model>` and `--variant <value>` exactly as supplied.",
		"For `go`, preserve at most one `--execution-owner omp|orca` pair exactly; omission defaults to `omp`, and aliases or fuzzy correction are forbidden.",
		"Do not fuzzy-correct an unknown subcommand; report it as unsupported and show the exact map.",
		"This is an emitted routing contract, not an OMP runtime parser or model-quality claim.",
	}, "\n")
}

func ompWorkflowTargets() string {
	targets := make([]string, 0, len(workflowSpecs)-1)
	for _, spec := range workflowSpecs {
		if spec.Name != "auto" {
			targets = append(targets, "`"+spec.Name+"`")
		}
	}
	return strings.Join(targets, ", ")
}

func splitOMPFrontmatter(content string) (string, string) {
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[4:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return "", content
	}
	body := strings.TrimPrefix(rest[idx+4:], "\n")
	return rest[:idx], body
}

func normalizeOMPWorkflowBody(body string) string {
	body = strings.NewReplacer(
		"@auto ", "/auto ",
		"@auto-", "/auto-",
	).Replace(body)
	body = pkgcontent.ReplacePlatformReferences(body, "omp")
	body = normalizeOMPTeamMultiContract(body)
	body = normalizeOMPExecutionOwnerContract(body)
	body = dedupeOMPReference(body, ".omp/skills/agent-pipeline/SKILL.md")
	return strings.TrimSpace(body)
}

func normalizeOMPTeamMultiContract(body string) string {
	body = strings.ReplaceAll(
		body,
		"- `--team`은 OMP Multi-Agent V2 팀 프로파일입니다. 모든 worker는 같은 shared cwd/filesystem을 사용하며 병렬 writer는 disjoint write ownership을 가져야 합니다. 메인 세션은 `task` batch, `send_message`, `followup_task`, target-less `hub with {\"i\":\"Waiting for blocked work\",\"op\":\"wait\",\"ids\":[\"<job id>\"]}`, `interrupt_agent`, `list_agents`만 사용합니다.",
		"- `--team` uses the OMP Team and Provider Axes contract plus the pipeline reference above; Lead/Builder/Guardian share cwd/filesystem, write only disjoint paths, dispatch through `task`, coordinate through `hub`, and leave parent progress to `todo`.",
	)
	body = strings.ReplaceAll(
		body,
		"- `/goal` active state가 있으면 `get_goal`로 목표를 확인하고 worker 프롬프트와 최종 `goal_status`에 반영합니다. 새 goal이 필요하면 `/auto goal \"<objective>\" [--budget N]`을 사용하고, `create_goal`/`update_goal`은 사용자가 명시했거나 goal tool contract가 충족될 때만 사용합니다.",
		"- Use only a runtime-exposed OMP goal surface. If none is available, do not claim or create persisted goal state; `/auto goal` remains the explicit route.",
	)
	return strings.NewReplacer(
		"OMP Multi-Agent V2", "OMP native task-batch",
		".omp/skills/agent-teams/SKILL.md", ".omp/skills/agent-pipeline/SKILL.md",
		"`--team` → OMP team profile", "`--team` → owner `omp` native task-batch team profile",
		"spawn_agent", "task",
		"send_message", "hub send",
		"followup_task", "hub send",
		"wait_agent()", "hub wait",
		"interrupt_agent", "hub cancel",
		"list_agents", "hub list",
		"get_goal", "runtime-exposed goal state",
		"create_goal", "/auto goal",
		"update_goal", "/auto goal",
	).Replace(body)
}

func dedupeOMPReference(body, ref string) string {
	first := strings.Index(body, ref)
	if first < 0 {
		return body
	}
	head := body[:first+len(ref)]
	tail := strings.ReplaceAll(body[first+len(ref):], ref, "the pipeline reference above")
	return head + tail
}

func normalizeOMPExecutionOwnerContract(body string) string {
	body = strings.NewReplacer(
		"- Choose exactly one topology before dispatch: `OMP-local` or `Orca-supervised`.",
		"- Choose exactly one DAG owner with `--execution-owner omp|orca` before dispatch; omission selects owner `omp`.",
		"- `OMP-local` is the default. The current OMP session is the sole DAG owner and uses its native `task`, `hub`, and `todo` tools.",
		"- Owner `omp` is the default. The current OMP session is the sole DAG owner and uses its native `task`, `hub`, and `todo` tools.",
		"- `Orca-supervised` is allowed only when the user explicitly selects a supervised or durable topology. Before any Orca orchestration, run and read `orca skills get orchestration --full`.",
		"- Owner `orca` is allowed only when `--execution-owner orca` is explicit. Before any Orca orchestration, run and read `orca skills get orchestration --full`.",
		"- The single DAG owner invariant is mandatory: when Orca owns the DAG, the OMP session does not dispatch a competing DAG; when OMP owns it, Orca does not dispatch one.",
		"- The single DAG owner invariant is mandatory: owner `orca` creates no OMP task DAG, and owner `omp` creates no Orca Run.",
	).Replace(body)
	return strings.NewReplacer(
		"`OMP-local`", "owner `omp`",
		"`Orca-supervised`", "owner `orca`",
		"OMP-local", "owner `omp`",
		"Orca-supervised", "owner `orca`",
	).Replace(body)
}

func injectOMPExecutionOwnerControl(body, name string) string {
	if name != "auto-go" {
		return body
	}
	block := strings.Join([]string{
		"## Execution Owner Control Plane",
		"",
		"- Invocation: `/auto go SPEC-ID [--execution-owner omp|orca]`. Omission selects `omp` with receipt source `default`; a supplied exact value has source `explicit`.",
		"- Parse the flag exactly once before any `task`, DAG-owning `todo`, OMP subprocess, or Orca Run effect. Reject repeated, mixed, case-shifted, whitespace-padded, or aliased values without fallback.",
		"- Persist the body-free receipt `.autopus/pipeline-state/<SPEC-ID>.execution-owner.json` with schema `pipeline_execution_owner_receipt.v1` and only owner, source, reason, SPEC/run identity, `checked_at`, and `verification_status` evidence.",
		"- Owner `omp`: the current OMP session is the sole DAG owner, uses native `task`, `hub`, and `todo`, and must not create an Orca Run.",
		"- Owner `orca`: do not initialize an OMP task/todo DAG or call `task`. Run and read `orca skills get orchestration --full`, then use its supervised durable cross-worktree Run contract.",
		"- At `auto pipeline run SPEC-ID --platform omp --execution-owner orca`, the tier integrity gate runs and the receipt is persisted before any Run, worker, or provider session exists, and the pipeline then executes its phases on supervised Orca workers. Never retry a failure as owner `omp`.",
	}, "\n")
	return injectOMPAfterHeading(body, block)
}

func injectOMPTeamMultiControl(body, name string) string {
	if name != "auto-go" {
		return body
	}
	block := strings.Join([]string{
		"## OMP Team and Provider Axes",
		"",
		"- `--team` selects owner `omp` native `task` batch with explicit Lead/Builder/Guardian responsibilities.",
		"- `--multi` selects provider-diverse planning and review, not team execution topology.",
		"- `--team --multi` composes team execution with provider-diverse planning and review.",
		"- without `--team` or `--solo`, owner `omp` uses the default native `task` batch pipeline.",
		"- `--team` conflicts with `--solo` and owner `orca`.",
		"- Lead, Builder, and Guardian coordinate only through native `task`, `hub`, and `todo`; the main OMP session owns the checklist and DAG.",
	}, "\n")
	return injectOMPAfterHeading(body, block)
}

func injectOMPInvocation(body, name string) string {
	subcommand := strings.TrimPrefix(name, "auto-")
	note := fmt.Sprintf("## OMP Invocation\n\n- `/auto %s ...`\n- `/%s ...`\n- Load detail skill `%s` for either entrypoint.", subcommand, name, name)
	return injectOMPAfterHeading(body, note)
}

func injectOMPAfterHeading(body, block string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "# ") {
			out := make([]string, 0, len(lines)+4)
			out = append(out, lines[:i+1]...)
			out = append(out, "", block, "")
			out = append(out, lines[i+1:]...)
			return strings.Join(out, "\n")
		}
	}
	return block + "\n\n" + body
}

func renderOMPCompactWorkflowSkill(spec workflowSpec) string {
	command, result := ompCompactWorkflowContract(spec.Name)
	sections := []string{
		"# " + spec.Name,
		"",
		"## OMP Invocation",
		"",
		"- `/auto " + strings.TrimPrefix(spec.Name, "auto-") + " ...`",
		"- `/" + spec.Name + " ...`",
		"- Load detail skill `" + spec.Name + "` for either entrypoint.",
	}
	if spec.Name == "auto-test" {
		sections = append(sections, "", "## Context Profile: test", "", "- Required: core,test", "- Optional: signature,learning", "- Excluded: canary")
	}
	description := normalizeOMPDescription(spec.Description)
	sections = append(sections,
		"", "## 실행 계약", "", description,
		"", "1. 대상 디렉터리와 전달된 인자를 확인합니다.",
		"2. "+command,
		"3. "+result,
	)
	content := strings.Join(sections, "\n")
	return buildMarkdown(ompSkillFrontmatter(spec.Name, description), normalizeOMPWorkflowBody(content))
}

func ompCompactWorkflowContract(name string) (string, string) {
	switch name {
	case "auto-status":
		return "Bash tool로 `auto status`를 실행해 SPEC lifecycle과 module 상태를 수집한 뒤 현재 workspace의 `.autopus/pipeline-state/<SPEC-ID>.execution-owner.json` receipt와 현재 OMP 세션의 native `hub` jobs 상태를 읽습니다. 외부 CLI가 OMP user-session roots를 검사할 수 있다고 가정하지 않습니다.", "receipt의 owner/source/verification과 OMP-native `hub` 상태를 결합해 단일 DAG owner 위반 여부, SPEC별 status, 다음 실행 가능한 명령을 보고합니다."
	case "auto-update":
		return "Bash tool로 `auto update`를 실행해 현재 repo 또는 meta workspace의 harness surface를 갱신합니다.", "변경된 generated surface와 source-of-truth 경계를 보고합니다."
	case "auto-map":
		return "Workspace tools로 구조, entrypoint, 의존성을 분석합니다.", "핵심 파일과 다음 액션을 요약합니다."
	case "auto-why":
		return "Workspace tools로 Lore, SPEC, ARCHITECTURE를 검색해 요청된 결정의 근거를 추적합니다.", "근거 파일과 결정 이유를 인용합니다."
	case "auto-verify":
		return "Playwright로 대상 frontend의 핵심 flow와 visual state를 검증합니다.", "실행 evidence와 UX regression을 보고합니다."
	case "auto-secure":
		return "요청 범위를 OWASP 관점으로 감사합니다.", "실행 가능한 finding과 검증 공백을 보고합니다."
	case "auto-test":
		return "scenarios.md를 로드하고 선언된 E2E 시나리오를 실행합니다.", "시나리오별 PASS / WARN / FAIL과 evidence를 보고합니다."
	case "auto-dev":
		return "`auto plan` → `auto go` → `auto sync`를 순차 실행합니다.", "첫 실패 단계에서 멈추고 재개 방법을 보고합니다."
	case "auto-doctor":
		return "Bash tool로 `auto doctor`를 실행해 harness 설치와 platform wiring을 값으로 진단합니다.", "finding별 expected/got과 repair action을 보고합니다."
	case "auto-goal":
		return "현재 OMP 세션의 목표 의도를 확인하고 지원되는 goal surface만 사용합니다.", "미지원 persisted state를 만들지 않고 제약을 명시합니다."
	default:
		command := "Bash tool로 `" + strings.ReplaceAll(strings.TrimPrefix(name, "auto-"), "-", " ") + "` 대신 `auto " + strings.TrimPrefix(name, "auto-") + "`를 실행합니다."
		return command, "PASS / WARN / FAIL 결과와 후속 액션을 요약합니다."
	}
}

package opencode

import (
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/templates"
)

func routerDescription() string {
	return "Autopus 명령 라우터 — OpenCode helper 및 update workflow 서브커맨드를 해석합니다"
}

func routerSubcommandCount() int {
	return len(workflowSpecs) - 1
}

func routerDetailSkills() string {
	names := make([]string, 0, routerSubcommandCount())
	for _, spec := range workflowSpecs {
		if spec.Name == "auto" {
			continue
		}
		names = append(names, fmt.Sprintf("`%s`", spec.Name))
	}
	return strings.Join(names, ", ")
}

func routerSupportedSubcommandsInline() string {
	names := make([]string, 0, routerSubcommandCount())
	for _, spec := range workflowSpecs {
		if spec.Name == "auto" {
			continue
		}
		names = append(names, fmt.Sprintf("`%s`", strings.TrimPrefix(spec.Name, "auto-")))
	}
	return strings.Join(names, ", ")
}

func thinRouterSkillBody() string {
	sections := []string{
		strings.TrimSpace(skillInvocationNote("auto")),
		"",
		templates.TaskTriagePolicy(),
		"",
		"## Router Contract",
		"",
		"- 먼저 global flags `--auto`, `--loop`, `--multi`, `--quality`, `--team`, `--solo`, `--model`, `--variant` 를 분리합니다.",
		"- 첫 non-flag 토큰을 subcommand로 사용합니다. 토큰이 없으면 사용자 의도를 분류합니다.",
		"- 지원 서브커맨드: " + routerSupportedSubcommandsInline(),
		"- `--model` / `--variant` 값은 이후 단계로 그대로 전달하고 자동으로 덮어쓰지 않습니다.",
		"- `--auto` authorizes the selected execution depth; ordinary work stays inline and delegation requires independent work or specialist/context isolation.",
		"- 실제로 위임이 필요하지만 `--auto`가 없고 현재 OpenCode 런타임이 암묵적 `task(...)` 호출을 허용하지 않으면, 조용히 단일 세션으로 폴백하지 말고 서브에이전트 진행 여부 또는 `--solo` 선택을 사용자에게 확인합니다.",
		"- 서브커맨드를 해석한 뒤에는 반드시 대응하는 상세 스킬(" + routerDetailSkills() + ") 중 하나를 로드합니다.",
		"- 지원하지 않는 서브커맨드면 목록을 짧게 안내하고 가장 가까운 워크플로우를 제안합니다.",
		"- 이 스킬은 얇은 라우터입니다. 고정된 서브커맨드는 바로 상세 스킬로 넘깁니다.",
		"",
		"## Shell Portability",
		"",
		"- GNU `timeout`으로 `auto` 명령을 감싸지 않습니다. macOS에는 기본 `timeout` 명령이 없어 `timeout 540 auto ...`가 exit 127로 실패합니다.",
		"- 실행 제한은 런타임/도구의 native timeout 또는 background/cancel 기능을 사용합니다.",
		"- provider 실행 제한은 shell wrapper가 아니라 `auto spec review ... --timeout <seconds>` 또는 `auto orchestra ... --timeout <seconds>`로 전달합니다.",
		"- `command not found: timeout`은 Autopus/provider 실패가 아니라 shell wrapper 실패로 분류하고 wrapper 없이 재실행합니다.",
		"",
		"## Context Load",
		"",
		"- 먼저 `core`(`AGENTS.md`, `.autopus/project/workspace.md`)를 읽고 선택된 command profile만 추가로 로드합니다.",
		"- `architecture`는 `ARCHITECTURE.md`, product, structure, tech 문서입니다. `test`는 scenarios, `canary`는 canary, `signature`는 signatures, `learning`은 sanitized learnings입니다.",
		"- `plan = core + architecture + relevant SPEC`; signature와 learning은 independently relevant할 때만 조건부로 로드합니다.",
		"- `test = core + test`; signature와 learning은 조건부입니다.",
		"- `canary = core + canary`; learning만 조건부입니다.",
		"- 선택된 profile 밖의 scenarios, canary, signatures, learnings, unrelated product documents는 로드하지 않습니다.",
		"- core 문서가 없으면 컨텍스트 부재를 명시하고 `/auto setup`을 권장합니다.",
		"",
		"## SPEC Path Resolution",
		"",
		"- SPEC-ID를 받으면 실제 `SPEC_PATH`, `SPEC_DIR`, `TARGET_MODULE`, `WORKING_DIR`를 먼저 해석한 뒤 상세 스킬로 넘깁니다.",
		"- 해석 순서: `.autopus/specs/{SPEC-ID}/spec.md` → `**/.autopus/specs/{SPEC-ID}/spec.md` 재귀 탐색 (`.git`, `node_modules`, `vendor`, `.cache`, `dist` 제외)",
		"- 0개면 사용 가능한 SPEC 목록과 함께 중단하고, 2개 이상이면 duplicate 경로를 보고하고 중단합니다.",
		"- `auto-go`, `auto-review`, `auto-sync` 같은 상세 스킬은 루트 상대경로를 고정 가정하지 말고 해석된 값을 사용해야 합니다.",
		"",
		"## OpenCode Notes",
		"",
		"- `status`, `verify`, `test`, `qa`, `doctor`는 thin wrapper 성격입니다.",
		"- `goal`은 Codex `/goal` compatibility wrapper입니다. OpenCode에서는 goal tool을 직접 실행하지 않고 Codex fallback command를 안내합니다.",
		"- `qa`는 QAMESH guidance로 `auto qa init`, `auto qa plan`, `auto qa run`, `auto qa release`, `auto qa evidence`, `auto qa feedback` 중 목적에 맞는 CLI를 실제 실행합니다.",
		"- `map`, `secure`, `why`는 OpenCode native analysis workflow로 처리합니다.",
		"- `dev`는 `plan -> go -> sync` 순서를 유지하며 `--auto`, `--loop`, `--team`, `--multi`, `--quality`, `--model`, `--variant` 를 하위 단계로 전달합니다.",
		"- 고정된 서브커맨드만 필요하면 `/auto-canary`, `/auto-plan`, `/auto-go` 같은 direct alias를 우선 사용하는 편이 더 짧고 저렴합니다.",
	}
	return strings.Join(sections, "\n")
}

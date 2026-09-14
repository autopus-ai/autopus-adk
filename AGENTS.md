<!-- AUTOPUS:BEGIN -->
# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: autopus-adk
- **모드**: full
- **플랫폼**: claude-code, codex, antigravity-cli, opencode, omp

## Installed Components

- Claude Skills: .claude/skills/<name>/SKILL.md
- Claude Agents: .claude/agents/autopus/
- Claude Rules: .claude/rules/autopus/
- Claude Hooks: .claude/hooks/autopus/
- Claude Settings: .claude/settings.json
- Claude Statusline: .claude/statusline.sh
- Claude Root Doc: CLAUDE.md
- Codex Native Skills: .codex/skills/codex-<name>/SKILL.md
- Codex Agents: .codex/agents/
- Codex Hook Scripts: .codex/hooks/
- Codex Hook Registry: .codex/hooks.json
- Codex Config: .codex/config.toml
- Codex Plugin Router: @auto ... / $codex-auto-<route>
- Gemini Skills: .gemini/skills/autopus/<name>/SKILL.md
- Gemini Agents: .gemini/agents/autopus/
- Gemini Rules: .gemini/rules/autopus/
- Gemini Commands: .gemini/commands/
- Gemini Hooks: .gemini/hooks/
- Gemini Settings: .gemini/settings.json
- Gemini Statusline: .gemini/statusline.sh
- Gemini Hook Registry: .agents/hooks.json
- Gemini Root Doc: GEMINI.md
- Gemini Plugin Bundle: .agents/plugins/autopus/
- Shared Commands: .agents/commands/
- OpenCode Rules: .opencode/rules/autopus/
- OpenCode Commands: .opencode/commands/
- OpenCode Agents: .opencode/agents/
- OpenCode Plugins: .opencode/plugins/
- OpenCode-owned Shared Skills: .agents/skills/
- OMP Skills: .omp/skills/
- OMP Agents: .omp/agents/
- OMP Rules: .omp/rules/
- OMP Commands: .omp/commands/
- Plugin Marketplace: .agents/plugins/marketplace.json


## Language Policy

These settings are prompt instructions for every agent in this project. They are
not mechanically enforced: no hook, linter, or CI gate inspects the language of
comments, commit messages, or responses. The pre-commit Lore check validates the
commit type prefix and sign-off trailers only, never the language. Treat a
violation as a review finding, not as something a gate will catch.

- **Code comments**: en
- **Commit messages**: ko
- **AI responses**: ko

## Autopus Branding

The canonical banner header is the first line below. Start a `/auto` or `@auto`
response with it and end the completed response with `🐙`.

```text
🐙 Autopus ─────────────────────────
  프로젝트: {project-name} | 모드: {mode}
  SPEC: {draft}개 draft · {approved}개 approved · {implemented}개 구현중 · {completed}개 완료
  다음: {next-step recommendation}
```

Subagent completion summaries use the A3 Agent Result shape:

```text
🐙 {agent-name} ─────────────────────
  {key metric 1} | {key metric 2} | {key metric 3}
  다음: {next step guidance}
```

General responses that applied no harness rule carry no banner, footer, or emoji.

## Document Storage

IMPORTANT: A document stored in the wrong place causes sync failures and version
control gaps.

| Document Type | Location | Git Repo |
|---------------|----------|----------|
| Project context | Root | meta repo |
| Harness bootstrap config | Root | meta repo |
| Generated harness/runtime surface | Local working copy only | Do not commit |
| Cross-module SPEC | Root `.autopus/specs/` | meta repo |
| Module-specific SPEC | `{module}/.autopus/specs/` | module repo |
| Brainstorm/runtime output | Local working copy only | Do not commit |
| Module CHANGELOG | `{module}/CHANGELOG.md` | module repo |

Module detection: match the referenced `pkg/`, `cmd/`, `internal/`, `src/`, or
`app/` paths to their owning submodule. Paths spanning 2+ modules are
cross-module and belong at the root.

SPEC and BS IDs MUST be globally unique across the workspace. Scan both
`.autopus/specs/SPEC-*` and `*/.autopus/specs/SPEC-*` (and the matching
`brainstorms/BS-*` pair) before allocating an ID; a collision is a hard error.

Run `auto sync verify` before committing. It is read-only and prints the
topology it detected. In a single repository holding `autopus.yaml` it
partitions every dirty path into a commit candidate, a blocked
generated/runtime path, or an unclassified path. In a multi-repo workspace the
same partition splits candidates into Phase A (module) and Phase B (meta)
commits. Where neither layout applies it stops with a dedicated
`unsupported topology:` diagnostic instead of a classification result.
`auto check --hygiene --staged` is not an equivalent substitute: it checks
generated/runtime hygiene only and never partitions the full dirty path set.

## Execution Model

- **Codex V2**: multi_agent_v2 uses spawn_agent, send_message, followup_task, wait_agent, interrupt_agent, list_agents.
- **Codex Concurrency**: spawn 되는 worker 상한은 `autopus.yaml`의 `codex.agents.max_concurrent_threads`이며(기본 4), coordinator thread는 세지 않습니다. 요청한 값은 요청일 뿐입니다. provider/account/host 한도가 우선하고, 변경은 새 세션부터 적용되며, config 파일 쓰기 성공은 확보된 capacity의 근거가 아닙니다. `auto doctor`의 `doctor.codex.agents.concurrency`에서 requested/loaded/effective를 확인하세요. 무거운 로컬 test/build 병렬도는 이 값이 관리하지 않습니다.
- **Codex Workspace**: 모든 agent는 shared cwd/filesystem을 사용합니다. fork_turns는 대화 context만 분기하며 filesystem isolation이나 merge를 제공하지 않습니다.
- **Codex Router**: @auto <route> ...는 auto plugin을 호출하고 상세 workflow는 $codex-auto-<route> skill로 라우팅합니다.
- **Codex /goal**: Codex goals feature를 사용합니다. @auto goal은 이 기능의 thin wrapper이며, active goal이 있으면 get_goal로 목표를 반영하고 create_goal/update_goal은 Codex goal tool contract를 만족할 때만 사용하세요.
- **OpenCode**: 기본 실행 모델은 task(...) 기반 subagent-first 입니다.
- **OpenCode Invocation**: /auto <subcommand> ... 또는 /auto-<subcommand> ... alias를 사용합니다.


## OpenCode Notes

- The generated rules are loaded through opencode.json instructions.
- Runtime plugins are registered through the plugin field in opencode.json.
- Use /auto <subcommand> ... or direct aliases like /auto-plan ... .
- Default OpenCode session model is assumed to be openai/gpt-5.4; users can override with opencode -m <provider/model> or opencode run -m <provider/model>.
- Use --variant <value> for provider-specific reasoning effort overrides when needed.
- Project skills are published under .agents/skills/ so OpenCode can load them through the native skill tool. If you want a smaller mixed Codex + OpenCode surface, set skills.shared_surface to "auto" or "core" to reduce Codex workspace noise.
- A dedicated OpenCode statusline surface is not generated by default; use CLI wrappers like auto status and auto doctor for observability.

## Core Guidelines

### Supervisor Contract

IMPORTANT: 메인 세션은 얇은 라우터가 아니라 phase/gate를 관리하는 supervisor입니다. 각 단계마다 필수 단계, skip 조건, retry 한도, 다음 필수 단계를 명확히 유지하세요.

### Subagent Delegation

IMPORTANT: 3개 이상 파일 수정, 다중 도메인 변경, 또는 신규 코드 200줄 초과가 예상되면 기본적으로 서브에이전트를 사용하세요. 단, 읽기 위주 탐색/리서치/테스트 분석은 병렬 fan-out을 우선하고, 쓰기 위주 구현은 파일 소유권이 겹치면 순차 실행으로 전환하세요.

### Worker Contracts

IMPORTANT: 각 worker 프롬프트에는 반드시 소유 파일/모듈, 수정 금지 범위, 완료 기준, 반환 형식을 포함하세요. 최소 반환 필드는 `owned_paths`, `changed_files`, `verification`, `blockers`, `next_required_step` 입니다.

### Review Convergence

IMPORTANT: 리뷰는 discovery와 verification을 분리하세요. 첫 리뷰는 finding discovery에 집중하고, 재시도는 열린 finding 해결 여부만 diff 기준으로 확인하세요. 같은 범위를 무한 재탐색하지 마세요.

### File Size Limit

IMPORTANT: 300줄 제한은 소스 코드 파일에만 적용합니다. SPEC Markdown files under .autopus/specs/** are documentation and exempt from the 300-line source code limit. prd.md, spec.md, plan.md, acceptance.md, research.md, review.md는 300줄 초과만으로 분할하거나 거절하지 마세요.

### Mandatory Compact Policy

IMPORTANT: Write tests before implementation when behavior changes. New code comments are English. Source and test files MUST stay at or below 300 lines. Do not edit outside assigned ownership. Every spawned worker returns exactly `owned_paths`, `changed_files`, `verification`, `blockers`, `next_required_step`.

### Prompting Notes

IMPORTANT: 사용자가 계획만 요구한 경우를 제외하면, 긴 선행 계획만 출력하고 멈추지 마세요. 먼저 코드베이스를 확인하고, 필요한 경우 서브에이전트를 스폰한 뒤, 검증까지 이어서 진행하세요.

## Agents

The following specialized agents are available.

### Annotator Agent

Phase 2.5 @AX tag scanning and application specialist.

### Architect Agent

시스템 아키텍처를 설계하고 기술 결정을 내리는 에이전트입니다.

### Debugger Agent

버그의 근본 원인을 분석하고 최소한의 수정으로 해결하는 에이전트입니다.

### Deep Worker Agent

장시간 실행이 필요한 복잡한 태스크를 체크포인트와 검증 루프를 통해 안전하게 완료하는 에이전트입니다.

### DevOps Agent

CI/CD, 컨테이너화, 인프라 설정을 전담하는 에이전트입니다.

### Executor Agent

TDD 또는 DDD 방법론에 따라 코드를 구현하는 에이전트입니다.

### Explorer Agent

코드베이스를 빠르게 탐색하고 구조를 분석하는 에이전트입니다.

### Frontend-Specialist Agent

Phase 3.5 Playwright E2E testing, screenshot analysis, and UX verification specialist.

### Perf-Engineer Agent

Benchmark execution, profiling, and performance regression detection specialist.

### Planner Agent

기능 기획과 요구사항 분석을 전담하는 에이전트입니다.

### Reviewer Agent

TRUST 5 기준으로 코드를 체계적으로 검토하는 에이전트입니다.

### Security Auditor Agent

OWASP Top 10 기준으로 보안 취약점을 탐지하고 수정하는 에이전트입니다.

### Spec Writer Agent

SPEC 문서를 생성하는 전문 에이전트입니다.

### Tester Agent

테스트를 설계하고 구현하는 전담 에이전트입니다.

### UX Validator Agent

Claude Vision(멀티모달)으로 프론트엔드 스크린샷을 분석하여 레이아웃 및 접근성 문제를 탐지하는 에이전트입니다.

### Validator Agent

코드 품질을 빠르게 검증하는 경량 에이전트입니다.


## Rules

See .codex/skills/codex-agent-pipeline/SKILL.md for Codex phase and gate contracts.
See .codex/skills/codex-<name>/SKILL.md for Codex-native workflow skills.
See .codex/agents/ for Codex agent definitions.
See .opencode/rules/autopus/ for OpenCode guidance.

<!-- AUTOPUS:END -->

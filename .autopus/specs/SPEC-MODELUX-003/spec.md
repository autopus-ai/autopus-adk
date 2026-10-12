# SPEC-MODELUX-003: 에이전트별 effort 편집과 OMP 기본값을 Autopus 에이전트로

**Status**: draft
**Created**: 2026-10-12
**Domain**: MODELUX
**Module**: autopus-adk
**Release**: v0.50.125 (SPEC-MODELUX-001·002와 같은 branch `feat/agent-model-ux`, PR #196)
**Predecessor**: SPEC-MODELUX-002(이 branch에 구현됨, S20 smoke 완료). 폼 흐름, Label Contract, Per-Tool Binding Contract, OMP Tab Contract, Write Safety Contract는 그 SPEC 정의를 쓴다. 두 SPEC이 다르면 이 SPEC이 우선하고 차이는 `## Related SPECs`에 적는다.
**Source PRD**: `prd.md`. 코드 근거, probe, 결정은 `research.md`와 `evidence/probe-omp-18.6.1.md`에 있다.

## 목적

SPEC-MODELUX-002 S20 smoke 뒤 사용자가 두 가지를 요청했다. "effort도 수정할 수 있게 해줘야지." "omp도 … 기존 에이전트를 없애버리고, 우리 autopus 식으로 에이전트를 생성해서 이걸 디폴트로 해야 할 것 같아."

이 SPEC은 두 가지를 한다. (1) Claude Code·Codex 탭에서 등급과 함께 effort를 그 도구가 받는 값으로 고르고 `quality.presets.<name>.efforts`에 저장한다. 값이 없으면 지금의 자동 effort다. OMP 탭은 thinking을 고른다. (2) OMP의 기본 agent source를 Autopus로 바꾼다. 정규 16개 에이전트를 `.omp/agents/<agent>.md`로 만들고, scout·security-reviewer·sonic은 `task.disabledAgents`로 끄고, reviewer는 같은 이름의 Autopus reviewer가 대신하며, task는 이름 없는 dispatch의 기본값이라 켜 둔다. 프로필이 있으면 route는 16개 이름(+task)을 key로 한 `task.agentModelOverrides`로 기존 활성화·증명·되읽기 경로를 탄다. 이전 방식은 한 릴리스 동안 `role_model_policy.agent_source: native`로 고를 수 있다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md` `## Outcome Lock`이다.
- Mandatory requirements: REQ-MUX3-01 ~ REQ-MUX3-17 (Must). REQ-MUX3-18, 19는 Should.
- Explicit non-goals:
  - 프로필(`role_model_policy.profile`)이 없는 프로젝트에 quality 등급으로 OMP route를 자동으로 만들기(non-goal, (사용자 결정 2026-10-13: 세션 모델 상속. 프로필이 없으면 부모 세션 모델을 상속하고 frontmatter에 thinking만 쓴다))
  - native 경로와 `agent_source` key 제거(다음 릴리스, Evolution Idea)
  - `.omp/agents` frontmatter의 concrete model이나 OMP role alias(`@slow` 등)
  - `모든 도구 같게` 탭의 effort 편집(도구마다 어휘가 다르다), 줄 driver(SPEC-MODELUX-001)의 키·문구 변경
  - OMP writer(`replaceAutopusConfigSection`)의 문서 전체 재encode 변경, OpenCode·Antigravity effort, Gemini 표 변경
  - OMP `/review` 출력 schema를 OMP 버전마다 따라가기(18.6.1 모양을 고정한다)
- Completion evidence: Must S1–S17 PASS, `auto eval harness run`, 신규·변경 package coverage 85% 이상, 신규·변경 소스 파일 300줄 이하, `GOOS=windows GOARCH=amd64 go build ./...`, `evidence/manual-smoke-003.txt`(S17), `auto spec validate --strict` 통과.

## Trust Model

- SPEC-MODELUX-001·002 Trust Model을 그대로 따른다. `autopus.yaml`은 반쯤 신뢰하고 writer는 loader가 펼친 값을 쓰기 원천으로 쓰지 않는다.
- 새 비신뢰 입력은 둘이다. 이미 있는 `.omp/agents/*.md`는 `Lstat`과 manifest checksum 비교에만 쓰고 내용을 생성 판단에 쓰지 않는다. 이미 있는 `.omp/config.yml`은 기존 managed-claim parser(`omp_model_activation_project.go:143-188`, 중복 key·merge key 거부)로만 읽는다.
- 사용자 파일과 사용자 소유 설정은 쓰지 않는다(REQ-MUX3-09, 10). 생성 agent 파일의 내용은 embed된 `content/agents/*.md`에서 오고 scalar는 `OMPYAMLScalar`로 encode한다.
- 폼 라벨의 catalog·YAML 글자는 SPEC-MODELUX-002처럼 `terminalSafe`를 거친다. 새 로그나 artifact는 없다. 증거 파일은 커밋 전에 `$HOME`, scratch 경로, 사용자 이름을 placeholder로 바꾼다.

## Requirements

### REQ-MUX3-01 effort key
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL accept an optional `quality.presets.<name>.efforts` mapping from canonical agent names to one of low, medium, high, xhigh, or max, SHALL reject any other agent name or value at load with an error naming the key path and the value, and SHALL load and generate files without the key exactly as HEAD 93dbb564 does.

- 필드는 `[NEW] config.QualityPreset.Efforts`(`yaml:"efforts,omitempty"`), 검증은 `schema_model_selection.go:7-35`의 `validateModelSelectionConfig`에 더한다. 오류 형식 `quality.presets.<name>.efforts.<agent>: effort_invalid "<value>"`.
- 값 집합은 Claude(문서)와 Codex(`codex_profile.go:46`, 에이전트는 `ultra`를 `max`로 낮춘다 `:189-195`)가 함께 받는 단계다. OMP thinking은 이 key에 저장하지 않는다(Effort Storage Contract).

### REQ-MUX3-02 effort 해석
Priority: Must · EARS: StateDriven

WHERE the provider's effective preset holds an effort for an agent, THE SYSTEM SHALL take that agent off the standard balanced placement, project the effort as its Claude effort unless its tier is haiku, and request it as its Codex effort resolved against the run's catalog snapshot, and SHALL otherwise keep the automatic effort.

- `[NEW] QualityConf.AgentEffort(provider, agent) (string, bool)`는 `AgentTier`(`quality_tier.go:76-91`)와 같은 프리셋을 본다. `NativeBalancedAgentCandidate`(`native_balanced.go:20-24`)는 명시 등급처럼 명시 effort도 표준 배치를 떠나는 조건으로 본다.
- Claude: `ClaudeAgentProfile`이 명시 effort를 돌려준다. haiku 등급은 effort 줄이 없다(Haiku 4.5는 Claude Code effort 표에 없다). Codex: `CodexAgentProfile`의 사다리 effort 자리에 명시 effort를 넣고, 해석은 지금의 `ResolveCodexProfile`이다(지원하지 않으면 `(대체)`).

### REQ-MUX3-03 effort 선택 (Claude Code·Codex 탭)
Priority: Must · EARS: EventDriven

WHEN the operator answers a tier select of a group or an agent on the Claude Code or Codex tab, THEN THE SYSTEM SHALL ask that tool's effort in a select whose default is 그대로 and whose other options are exactly the levels the Effort Option Contract allows for every affected agent, labelled with the cells each option produces, and SHALL skip the select when no level is allowed.

- 라벨은 SPEC-MODELUX-002 Label Contract의 `<cells>`에서 effort만 그 단계로 바꾼 것이다. 자동과 같은 단계에는 `(자동)`을 붙인다.
- 자동과 같은 단계를 고르면 저장하지 않는다(자동으로 둔다). 에이전트마다 판정한다.

### REQ-MUX3-04 effort 저장
Priority: Must · EARS: EventDriven

WHEN the operator confirms a Claude Code or Codex tab edit whose tiers or efforts differ from its start, THEN THE SYSTEM SHALL write `quality.presets.<p>-custom` with every canonical agent's tier and the explicit efforts of the working composition in the one saveQualityBlock edit of the Per-Tool Binding Contract, and SHALL print the old-binary line only when the write introduces the first `efforts` mapping of the file.

- `setQualityToolPreset`(`quality_tool_persist.go:49-61`)은 effort도 쓴다. 지금은 프리셋을 `{Description, Agents}`로 바꿔 effort를 지운다.
- 바뀌는 설정 줄에 `quality.presets.<p>-custom.efforts 새로 씀 (N개 에이전트)`(또는 `고쳐 씀`, `지움`)이 더해진다. init은 SPEC-MODELUX-002처럼 메모리 cfg에 같은 값을 넣는다.

### REQ-MUX3-05 생성 = 미리보기
Priority: Must · EARS: EventDriven

WHEN a confirmed effort change is applied in the same run by update generation, auto quality --apply, or init generation, THEN THE SYSTEM SHALL produce Claude Code `effort` and Codex `model_reasoning_effort` values equal to the after-table cells computed with the run's snapshot.

### REQ-MUX3-06 agent source key
Priority: Must · EARS: StateDriven

WHERE `role_model_policy.agent_source` is absent or `autopus`, THE SYSTEM SHALL serve OMP with Autopus agents, WHERE it is `native`, THE SYSTEM SHALL keep the SPEC-MODELUX-002 behavior of five native agents, and THE SYSTEM SHALL reject any other value at load with `role_model_policy.agent_source_invalid`, while `agent_source` alone never makes a role model policy configured.

- `[NEW] RoleModelPolicyConf.AgentSource`(`yaml:"agent_source,omitempty"`). 정책 존재 판정 `role_model_policy_validate.go:22-25`에 넣지 않는다. 모드 helper는 `[NEW] pkg/config/omp_agent_source.go`.
- 이 key를 쓴 파일은 v0.50.113–v0.50.124가 읽지 못한다(strict). help·README·CHANGELOG에 쓰고, 다음 릴리스 제거 계획을 적는다.

### REQ-MUX3-07 OMP agent 파일 생성
Priority: Must · EARS: EventDriven

WHEN OMP files are generated in autopus mode, THEN THE SYSTEM SHALL write `.omp/agents/<agent>.md` for the 16 canonical agents by the OMP Agent File Contract, write no `model` field, and record each written file in the OMP manifest.

- 생성기는 `61f72bb0^`의 `prepareAgentMappings`와 `renderAgentForOMP`를 되살려 model·thinking 선택 인자를 뺀 것이다(`[NEW] pkg/adapter/omp/omp_agents.go`, `[NEW] pkg/content/agent_transformer_omp_render.go`). `omp.go:106-114`의 은퇴 정리는 native 모드에서만 돈다.

### REQ-MUX3-08 reviewer와 OMP `/review`
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL give the generated reviewer an `output` schema with `overall_correctness` (correct, incorrect), `explanation`, `confidence`, and optional `findings` in the omp 18.6.1 bundled reviewer shape, so that OMP's review command keeps its verdict, while Autopus dispatches keep their own per-call schema.

- 근거: `/review`는 `agent: "reviewer"`로 보내고 verdict 필드를 읽는다(S:780806-780816, S:338744-339163). schema 우선순위는 caller > agent(S:526717)이고 Autopus dispatch는 호출마다 `outputSchema`를 준다(`skill_transformer_replace_omp_render.go:90-91`).

### REQ-MUX3-09 사용자 agent 파일
Priority: Must · EARS: Unwanted

IF `.omp/agents/<agent>.md` exists and the OMP manifest does not record it, THEN THE SYSTEM SHALL leave it byte-identical, skip only that agent, and print the user-file line, and IF the manifest records it but its checksum differs, THEN THE SYSTEM SHALL copy it to `.autopus/backup/<timestamp>/.omp/agents/<agent>.md` before overwriting it.

- 지금 OMP update는 `ActionBackup`도 백업 없이 쓰고(`omp_rooted_lifecycle.go:136-175`), Generate는 manifest가 없으면 덮는다(`:111-114`). 이 요구사항이 그 두 자리를 agent 파일에 대해 막는다. 건너뛴 이름은 OMP가 사용자 정의를 쓴다(이름 선착, S:524490).

### REQ-MUX3-10 native agent 끄기
Priority: Must · EARS: EventDriven

WHEN OMP files are generated in autopus mode, THEN THE SYSTEM SHALL claim `task.disabledAgents` as [scout, security-reviewer, sonic] in `.omp/config.yml` through MergeOMPProjectManagedConfig and the project ownership ledger regardless of the model profile, SHALL keep task enabled, and SHALL leave a user-owned value unchanged and report `agents:disabled_agents_user_owned` as degraded instead of failing.

- Disabled Agents Contract가 경우별 동작이다. plain `omp` 세션은 overlay를 읽지 않고 project `.omp/config.yml`을 읽으므로(S:466140-466160) 이 key는 config_mode와 무관하게 `.omp/config.yml`에 간다.
- task를 끄면 이름 없는 dispatch가 preflight에서 실패한다(S:526773, probe P4). 새 파일은 0600이다(`omp.go:230-233`).

### REQ-MUX3-11 Autopus 이름 route
Priority: Must · EARS: StateDriven

WHERE a role model policy profile is selected in autopus mode, THE SYSTEM SHALL resolve one route per canonical agent and one for task by the Route Contract and SHALL write `task.agentModelOverrides` keyed by those names only through the existing activation, attestation, RPC readback, and rollback path.

- 바뀌는 자리: bridge(`omp_model_integration_bridge.go:31-113`), projection(`omp_model_projection.go:59-132`), 순위·family 기준(`omp_model_routing_compile.go:33,105-112`). 모두 `OMPNativeAgentNames()` 대신 모드의 agent 집합을 쓴다. P3에서 사용자 이름 key가 record로 되읽혔다.

### REQ-MUX3-12 소비처
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL derive every OMP agent consumer from the mode's agent set by the Agent Set Contract, so that in autopus mode a generated file under its own name is never reported as a shadow and a missing generated file is reported as degraded.

### REQ-MUX3-13 OMP 탭 (autopus)
Priority: Must · EARS: EventDriven

WHEN the OMP tab runs in autopus mode, THEN THE SYSTEM SHALL offer 역할 그룹 and 에이전트별 over the 16 canonical agents, list per selection the profile default and the installed models whose capability covers every affected agent, ask a thinking select over the levels the chosen model reports, and pin the plan's effective selector when only the thinking changes, persisting through the SPEC-MODELUX-002 OMP paths.

- update·quality는 `applyOMPProfile`과 cfg 동기화, init은 메모리 경로(SPEC-MODELUX-002 REQ-MUX2-11, 22, 23)다. native 모드는 SPEC-MODELUX-002 OMP 탭 그대로다. 줄 OMP 편집기(native pin)는 바꾸지 않는다. 그 pin은 Route Contract ②로 펼쳐진다.

### REQ-MUX3-14 update migration
Priority: Must · EARS: EventDriven

WHEN auto update runs in autopus mode on a project whose OMP manifest records no `.omp/agents` file, THEN THE SYSTEM SHALL show the migration lines of the Migration Contract in the preview and perform the agent files, the disabled-agents claim, and any profile activation in the OMP adapter's one transaction, without writing autopus.yaml for the migration.

### REQ-MUX3-15 native로 되돌리기
Priority: Must · EARS: EventDriven

WHEN OMP files are generated with `agent_source: native`, THEN THE SYSTEM SHALL prune the manifest-recorded generated agent files, restore `task.disabledAgents` to its preimage through the ownership ledger, and route, render, and report the five native agents as HEAD 93dbb564 does.

### REQ-MUX3-16 기존 안전 성질
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL keep every SPEC-MODELUX-001 and SPEC-MODELUX-002 safety property: raw quality writer, alias and merge refusal, placeholder guards, terminalSafe, no stdin read on non-interactive runs, one catalog snapshot per run, OMP attestation, readback, and rollback, and manifest ownership.

### REQ-MUX3-17 test seam과 수동 smoke
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL run every new form and OMP path through the existing seams (composeRunForm, composeFormsProbe, ompModelDoctorE2ERunner, platformOMPModelRunner, ompCLIFakeRunner) and SHALL record one manual real-terminal smoke in `evidence/manual-smoke-003.txt`.

### REQ-MUX3-18 locale
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL render every new title, option, note, and change line in Korean when tui.DetectSystemLang returns ko and in English otherwise, while written values stay locale-independent.

### REQ-MUX3-19 문서
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL document effort editing, the `efforts` key and its old-binary limit, the Autopus OMP default, `agent_source: native` with its removal in the next release, and the migration in init, update, and quality help, README, and the v0.50.125 CHANGELOG, without removing sentences existing tests check.

## Contracts

### Effort Option Contract

| 도구 | 허용 단계 | 원천 | 묻지 않는 경우 |
|---|---|---|---|
| Claude Code | `low`, `medium`, `high`, `xhigh`, `max` | Claude Code 문서: Fable 5.1·Opus 5.5·Sonnet 5.5는 다섯 단계 | 영향받는 에이전트 중 haiku 등급이 있음 |
| Codex | 영향받는 모든 에이전트의 후보 model(run snapshot으로 해석한 effective model)의 `supported_reasoning_levels` 교집합 ∩ 위 다섯 단계 | `codex_profile.go:66-75`, `codex_profile_resolve.go:30-33` | snapshot을 못 읽음(안내 1줄) 또는 교집합이 빔 |
| OMP | 고른 model이 catalog에 보고한 thinking 단계. 프로필 기본값이면 plan의 effective model. 그룹은 교집합 | SPEC-MODELUX-002 OMP Tab Contract의 thinking 규칙 | 단계가 하나 이하 |

### Effort Storage Contract

| 도구 | 저장 | 생성 |
|---|---|---|
| Claude Code, Codex | `quality.presets.<p>-custom.efforts.<agent>`(자동과 다른 에이전트만) | Claude `effort:`(haiku 제외), Codex `model_reasoning_effort`(catalog 해석) |
| OMP | 기존 pin `role_model_policy.agents.<agent>.candidates[0].thinking` | override 값 `<selector>:<thinking>` |
| 자동 | key 없음 | 지금의 자동 값(`ClaudeAgentEffort`, Codex 사다리, 선언 effort) |

### Agent Set Contract

| 사용처 | autopus (기본) | native |
|---|---|---|
| `.omp/agents` | 정규 16개 생성 | 생성하지 않고 manifest의 옛 파일 정리(`omp.go:106-114`) |
| `task.disabledAgents` | [scout, security-reviewer, sonic] claim | claim 없음, ADK claim은 preimage로 복원 |
| route·override key | 정규 16개 + task | native 5개 |
| 표 OMP 칸 | 에이전트 자신의 route `<model>:<thinking>` | `task · …`(지금) |
| OMP 탭 단위 | 16개(역할 그룹·에이전트별) | native 5개 |
| skill dispatch | `"agent": "<role>"`, 이름 없는 dispatch는 task(생략) | native 접힘(`skill_transformer_replace_omp_render.go:103-124`) |
| receipt·pipeline phase | phase 역할 이름 그대로 | native 접힘(`pipeline_backend_omp_models.go:75-110`) |
| doctor agent catalog | 17행(16 + task), 생성 파일이 없으면 `agents:autopus_agent_missing` degraded, 생성 파일 이름은 shadow로 세지 않음 | 지금(shadow면 `agents:native_agent_shadowed`) |
| routing family 기준 | executor | task(`omp_model_routing_compile.go:33`) |

### OMP Agent File Contract

| frontmatter | 값 |
|---|---|
| `name` | 정규 에이전트 이름 |
| `description` | `content/agents/<agent>.md`의 description(`OMPYAMLScalar`) |
| `tools` | `61f72bb0^` 규칙: portable 도구 map(`Read`→read 등, TodoWrite 제외), MCP 이름 유지, 역할별 native 도구(debugger·executor·reviewer·validator `lsp`, frontend-specialist `browser`·`inspect_image`·`lsp`, ux-validator `browser`·`inspect_image`), 정렬 |
| `thinkingLevel` | 선언 effort(`max`, `high`, `medium`). 없으면 줄 없음 |
| `output` | reviewer만: REQ-MUX3-08 모양 |
| `model`, `spawns` | 쓰지 않는다 |

- 본문은 `NormalizeOMPResourcePaths(NormalizeAgentReferences(body, "omp"))`이고 dispatch 이름은 autopus 모드 규칙이다.

### Disabled Agents Contract

| `.omp/config.yml` 상태 | 동작 |
|---|---|
| 없음 | 0600으로 만들고 claim을 쓰고 ledger에 지문을 남긴다 |
| 있음, `task.disabledAgents` 없음 | 기존 bytes를 두고 claim을 넣는다(prior = missing) |
| 있음, ADK 소유(ledger 지문 일치) | 다시 쓴다 |
| 있음, 사용자 소유(지문 불일치) | 쓰지 않는다. 안내 1줄, doctor degraded, 명령은 성공 |
| native 모드 | ADK claim이면 preimage로 복원(ADK가 만든 파일이고 남는 것이 없으면 파일 삭제). 사용자 소유면 그대로 |

### Route Contract

| 대상 | route |
|---|---|
| 정규 에이전트 A | ① A의 operator pin ② `OMPNativeAgentForRole(A)` = N이 A와 다르고 N에 operator pin이 있으면 그 pin(옛 native key: task → 13개 역할, scout → explorer, security-reviewer → security-auditor) ③ 프로필이 A에 만든 route |
| task | 지금의 `OMPNativeAgentRoute("task")`(대표 planner) |
| scout, security-reviewer, sonic | route 없음. `sonic` pin은 받을 에이전트가 없어 미리보기에 경고 1줄 |
| 충돌 | autopus 모드는 `omp_native_agent_conflict`를 내지 않는다 |

### Migration Contract

| 경우 | 미리보기 | 쓰기 | autopus.yaml |
|---|---|---|---|
| autopus, manifest에 `.omp/agents` 없음 | `+ .omp/agents/<agent>.md` 16줄(사용자 파일은 사용자 파일 줄), `task.disabledAgents: scout, security-reviewer, sonic`, 안내 1줄 | OMP adapter Update 한 transaction: 파일, claim, ledger, 프로필이 있으면 overlay 또는 config와 활성화·되읽기 | migration으로 쓰지 않는다 |
| `agent_source: native` | `- .omp/agents/<agent>.md` 줄, `task.disabledAgents 복원` | prune, claim 복원, native 5 route 활성화 | 사용자가 쓴 값 그대로 |
| 실패 | - | 기존 transaction snapshot 복원과 활성화 실패 복원 | - |

### Form Text (ko / en)

| 자리 | ko | en |
|---|---|---|
| effort Select | `<그룹 또는 에이전트> effort` · `그대로 — <현재>` · 단계 라벨 `(자동)` | `<group or agent> effort` · `keep — <current>` · `(automatic)` |
| Codex 안내 | `Codex catalog를 읽지 못해 effort는 자동입니다` | `Codex catalog unreadable; effort stays automatic` |
| 바뀌는 설정 | `quality.presets.<p>-custom.efforts 새로 씀 (N개 에이전트)` / `고쳐 씀` / `지움`, `quality.presets.*.efforts는 v0.50.125보다 오래된 auto가 읽지 못합니다` | `… efforts written (N agents)` / `rewritten` / `removed`, `auto older than v0.50.125 cannot read quality.presets.*.efforts` |
| OMP thinking | `프로필 기본값 (<thinking>)` | `profile default (<thinking>)` |
| 사용자 파일 | `.omp/agents/<agent>.md는 사용자 파일이라 그대로 둡니다` | `.omp/agents/<agent>.md is a user file; left unchanged` |
| disabledAgents | `.omp/config.yml의 task.disabledAgents를 사용자가 정해 두어 scout, security-reviewer, sonic을 끄지 않았습니다` | `task.disabledAgents in .omp/config.yml is user-owned; scout, security-reviewer, and sonic stay enabled` |
| migration | `OMP는 이제 Autopus 에이전트 16개를 씁니다. 한 릴리스 동안 role_model_policy.agent_source: native로 되돌릴 수 있습니다.` | `OMP now uses the 16 Autopus agents; set role_model_policy.agent_source: native to go back for one release.` |

## Existing Test Changes

SPEC-MODELUX-002와 달리 0건이 아니다. OMP 기본값이 바뀌기 때문이다.

1. T1이 먼저 `rg 'OMPNativeAgentNames|OMPNativeAgentForRole|shadowsOMPNativeAgent|native_agent_shadowed|\.omp/agents' --glob '*_test.go'`로 목록을 만들어 `research.md`에 붙인다(grep 15개, explorer 추정 약 32개, 미확인).
2. native 계약을 고정한 test는 fixture에 `role_model_policy.agent_source: native` 한 줄만 더하고 단언은 바꾸지 않는다.
3. 기본값 전환 때문에 단언이 바뀌어야 하는 test(예: 기본 생성 파일 목록)는 그 단언만 바꾸고 파일별 이유를 review에 적는다. 새 동작은 새 test 파일에 둔다.
4. SPEC-MODELUX-001·002의 줄 driver와 폼 test는 바꾸지 않는다. effort가 없는 fixture는 출력이 같다(INV-MUX3-01).

## 생성 파일 상세

- `pkg/config`: `schema.go`(+1줄 `Efforts`), `[NEW] quality_effort.go`(+test: `AgentEffort`, 값 집합, 검증), `schema_model_selection.go`(검증 호출), `quality_effective.go`, `native_balanced.go`, `codex_profile.go`(명시 effort), `role_model_policy.go`(+1줄 `AgentSource`), `[NEW] omp_agent_source.go`(+test: 모드, agent 집합), `role_model_policy_validate.go`, `[NEW] role_model_policy_autopus_agents.go`(+test: Route Contract).
- `pkg/content`: `[NEW] agent_transformer_omp_render.go`(+test: OMP Agent File Contract, reviewer output), `skill_transformer_replace_omp.go`·`skill_transformer_replace_omp_render.go`(`[NEW] NormalizeOMPSemanticReferencesFor(body, source)`; 옛 이름은 native 동작을 유지한다).
- `pkg/adapter/omp`: `[NEW] omp_agents.go`(+test: 생성, 사용자 파일, 백업), `[NEW] omp_agent_surface_claim.go`(+test: Disabled Agents Contract), `omp.go`, `omp_rooted_lifecycle.go`, `omp_skills.go`, `omp_model_integration_bridge.go`, `omp_model_projection.go`, `omp_model_routing_compile.go`.
- `internal/cli`: `[NEW] quality_compose_effort.go`(+test: Effort Option Contract, 라벨), `[NEW] quality_compose_form_effort.go`(+test: effort Select), `quality_compose_form_edit.go`, `quality_compose_tool.go`, `quality_tool_persist.go`, `quality_compose_form_text.go`, `[NEW] quality_compose_omp_autopus.go`(+test: REQ-MUX3-13), `quality_compose_omp.go`(모드 분기 몇 줄), `platform_omp_agent_catalog.go`, `platform_omp_projection_build.go`, `platform_omp_profile_preview.go`, `pipeline_backend_omp_models.go`, `pipeline_backend_orca_models.go`, `doctor_omp_model_routing_probe.go`, `quality_compose_table.go`, `[NEW] platform_omp_agent_migration.go`(+test: Migration Contract 줄).
- 문서: README, CHANGELOG(v0.50.125), help 상수 파일. 증거: `evidence/probe-omp-18.6.1.md`, `[NEW] evidence/manual-smoke-003.txt`.
- 모든 신규·변경 소스 파일은 300줄 이하다. `quality_compose_omp.go`(292), `quality_compose_form_driver.go`(290), `schema.go`(290)에는 호출 몇 줄만 더한다.

## Related SPECs

- SPEC-MODELUX-002(구현됨): 바뀌는 점은 셋이다. (1) REQ-MUX2-10의 "지정 단위 native 5개"는 autopus 모드에서 16개다. (2) non-goal "새 YAML key"를 사용자 결정으로 뒤집어 `efforts`, `agent_source`를 더한다. (3) Per-Tool Binding Contract의 프리셋 쓰기가 effort를 담는다. 줄 driver(REQ-MUX2-15)와 OMP 쓰기 경로(REQ-MUX2-11, 22, 23)는 그대로다.
- SPEC-MODELUX-001(구현됨): 등급, Binding Contract, raw writer를 그대로 쓴다.
- SPEC-ADK-MINIMALITY-DISCIPLINE-001(commit 61f72bb0): OMP native 재사용 결정을 기본값에서 대체한다. native 경로는 `agent_source: native`로 한 릴리스 남는다.
- SPEC-OMP-001(16개 생성), SPEC-OMP-005(`@autopus_<agent>` alias, 은퇴): 생성은 되살리고 alias는 되살리지 않는다. SPEC-OMP-006(review backend)의 `reviewer` 의존은 미확인이며 S15가 receipt를 확인한다.
- Sibling: 없다(`research.md` `## Sibling SPEC Decision`).

## Open Issues

- Q1 | resolved 2026-10-13 | 사용자가 "세션 모델 상속"을 골랐다. 프로필 없는 프로젝트의 자동 route는 non-goal이다

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-MUX3-01 | T2 | S1 | INV-MUX3-01 |
| REQ-MUX3-02 | T2 | S2 | INV-MUX3-01, INV-MUX3-03 |
| REQ-MUX3-03 | T9 | S3, S4 | INV-MUX3-02, INV-MUX3-03 |
| REQ-MUX3-04 | T9 | S3, S4, S5 | INV-MUX3-03 |
| REQ-MUX3-05 | T9 | S3, S4 | INV-MUX3-03 |
| REQ-MUX3-06 | T3 | S6 | INV-MUX3-07 |
| REQ-MUX3-07 | T4 | S7 | INV-MUX3-04 |
| REQ-MUX3-08 | T4 | S7, S17 | INV-MUX3-04 |
| REQ-MUX3-09 | T4 | S8 | INV-MUX3-06 |
| REQ-MUX3-10 | T5 | S7, S8 | INV-MUX3-06 |
| REQ-MUX3-11 | T6 | S9 | INV-MUX3-04, INV-MUX3-05 |
| REQ-MUX3-12 | T7, T8 | S14, S15 | INV-MUX3-05 |
| REQ-MUX3-13 | T10 | S10, S11 | INV-MUX3-02 |
| REQ-MUX3-14 | T11 | S12 | INV-MUX3-06 |
| REQ-MUX3-15 | T5, T11 | S13 | INV-MUX3-07 |
| REQ-MUX3-16 | T1, T13 | S16 | INV-MUX3-01 |
| REQ-MUX3-17 | T13 | S16, S17 | INV-MUX3-05 |
| REQ-MUX3-18 | T12 | S18 | - |
| REQ-MUX3-19 | T12 | S19 | - |

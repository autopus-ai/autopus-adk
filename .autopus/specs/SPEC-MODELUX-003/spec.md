# SPEC-MODELUX-003: 에이전트별 effort 편집과 OMP 기본값을 Autopus 에이전트로

**Status**: approved
**Created**: 2026-10-12
**Revised**: 2026-10-13 (review round 1 F-001–F-020, round 2 F-002·F-008·F-013·F-021·F-022, round 3 F-002·F-008·F-023, round 4 F-002; 구현 리뷰 뒤 S21 NO catalog 판정을 provider 단위 degrade로 정정; 2026-10-14 S17 smoke 뒤 OMP 탭 멀티프로바이더·프로필 없는 pin: REQ-MUX3-13 개정, REQ-MUX3-22, 23 추가)
**Domain**: MODELUX
**Module**: autopus-adk
**Release**: v0.50.126. 작업 branch는 `feat/modelux-003`이고 기준은 `v0.50.125`(dd6d6ea2, origin/main)다. v0.50.125는 SPEC-MODELUX-001·002로 이미 발행됐다.
**Predecessor**: SPEC-MODELUX-002(v0.50.125에 포함). 폼 흐름, Label Contract, Per-Tool Binding Contract, OMP Tab Contract, Write Safety Contract는 그 SPEC 정의를 쓴다. 다르면 이 SPEC이 우선하고 차이는 `## Related SPECs`에 적는다.
**Source PRD**: `prd.md`. 근거, probe, 결정, review 반영은 `research.md`와 `evidence/probe-omp-18.6.1.md`에 있다.

## 목적

SPEC-MODELUX-002 smoke 뒤 사용자가 두 가지를 요청했다. "effort도 수정할 수 있게 해줘야지." "omp도 … 기존 에이전트를 없애버리고, 우리 autopus 식으로 에이전트를 생성해서 이걸 디폴트로 해야 할 것 같아."

이 SPEC은 두 가지를 한다. (1) Claude Code·Codex 탭에서 등급과 함께 effort를 그 도구가 받는 값으로 고르고 `quality.presets.<name>.efforts`에 저장한다. 값이 없으면 지금의 자동 effort다. OMP 탭은 thinking을 고른다. (2) OMP의 기본 agent source를 Autopus로 바꾼다. 정규 16개 에이전트를 `.omp/agents/<agent>.md`로 만들고, scout·security-reviewer·sonic은 `task.disabledAgents`로 끄고, reviewer는 같은 이름의 Autopus reviewer가 대신하며, task는 이름 없는 dispatch의 기본값이라 켜 둔다. 프로필이 있으면 route는 16개 이름과 task를 key로 한 `task.agentModelOverrides`로 기존 활성화·증명·되읽기 경로를 탄다. 이전 방식은 v0.50.126 한 릴리스 동안 `role_model_policy.agent_source: native`로 고를 수 있다.

S17 smoke 뒤(2026-10-14) 사용자가 OMP 탭을 다시 정했다: "omp의 경우 claude, gpt 계열 이렇게 나누기보다는 멀티프로바이더를 자유롭게 선택할 수 있어야 맞는 것 같은데". autopus 모드 OMP 탭은 프로필과 family를 묻지 않고, 역할 그룹(또는 에이전트별)마다 provider가 섞인 증명된 설치 model이나 `OMP 세션 모델 그대로`를 고른다. 결과는 프로필 없는 pin으로 저장되고 pin된 에이전트만 override key를 받는다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md` `## Outcome Lock`이다.
- Mandatory requirements: REQ-MUX3-01 ~ REQ-MUX3-17, REQ-MUX3-20 ~ REQ-MUX3-23 (Must). REQ-MUX3-18, 19는 Should.
- Explicit non-goals:
  - 프로필(`role_model_policy.profile`)이 없는 프로젝트에 quality 등급으로 OMP route를 자동으로 만들기(non-goal, (사용자 결정 2026-10-13: 세션 모델 상속. 프로필이 없으면 부모 세션 모델을 상속하고 frontmatter에 thinking만 쓴다))
  - native 경로와 `agent_source` key 제거(v0.50.127, Evolution Idea)
  - `.omp/agents` frontmatter의 concrete model이나 OMP role alias(`@slow` 등)
  - `모든 도구 같게` 탭의 effort 편집, 줄 driver(SPEC-MODELUX-001)의 키·문구 변경
  - 줄 OMP 편집기, `auto platform omp profile apply`, native 모드 OMP 탭의 프로필 경로 변경(그대로 프로필을 쓴다)
  - OMP writer(`replaceAutopusConfigSection`)의 문서 전체 재encode 변경, OpenCode·Antigravity effort
  - OMP 버전마다 `/review` schema 따라가기(18.6.1 golden을 고정한다), 프로필 claim key와 claim 밖 사용자 편집에 대한 `managed_key_conflict` 규칙 변경(`task.disabledAgents`만 바뀐 경우는 이 SPEC의 Claim Set Contract가 새로 정한다)
- Completion evidence: Must S1–S17, S20–S25 PASS, `auto eval harness run`, 신규·변경 package coverage 85% 이상, 신규·변경 소스 파일 300줄 이하, `GOOS=windows GOARCH=amd64 go build ./...`, `evidence/manual-smoke-003.txt`(S17, S23), `auto spec validate --strict`. `research.md` `## Completion Debt`의 CD-1·CD-2가 남아 있으면 완료가 아니다.

## Trust Model

- SPEC-MODELUX-001·002 Trust Model을 그대로 따른다. `autopus.yaml`은 반쯤 신뢰하고 writer는 loader가 펼친 값을 쓰기 원천으로 쓰지 않는다.
- 새 비신뢰 입력은 둘이다. 이미 있는 `.omp/agents/*.md`는 `Lstat`과 manifest checksum 비교에만 쓴다. 이미 있는 `.omp/config.yml`은 기존 managed-claim parser(`omp_model_activation_project.go:143-188`, 중복 key·merge key 거부)로만 읽는다.
- 사용자 파일과 사용자 소유 key는 쓰지 않는다(REQ-MUX3-09, 10). 생성 agent 파일의 내용은 embed된 `content/agents/*.md`와 golden에서 오고 scalar는 `OMPYAMLScalar`로 encode한다. 폼 라벨의 catalog·YAML 글자는 `terminalSafe`를 거친다.
- 새 로그나 artifact는 없다. 증거 파일은 커밋 전에 `$HOME`, scratch 경로, 사용자 이름을 placeholder로 바꾼다.

## Requirements

### REQ-MUX3-01 effort key
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL accept an optional `quality.presets.<name>.efforts` mapping from canonical agent names to one of low, medium, high, xhigh, or max, SHALL reject any other agent name or value at load with an error naming the key path and the value, and SHALL load and generate files without the key exactly as v0.50.125 does.

- 필드 `[NEW] config.QualityPreset.Efforts`(`yaml:"efforts,omitempty"`), 검증은 `schema_model_selection.go:7-35`. 오류 형식 `quality.presets.<name>.efforts.<agent>: effort_invalid "<value>"`.
- 값 집합은 Claude(문서)와 Codex(`codex_profile.go:46`, 에이전트는 `ultra`를 `max`로 낮춘다 `:189-195`)가 함께 받는 단계다. OMP thinking은 이 key에 저장하지 않는다.

### REQ-MUX3-02 effort 해석
Priority: Must · EARS: StateDriven

WHERE the provider's effective preset holds an effort for an agent, THEN THE SYSTEM SHALL take that agent off the standard balanced placement, project the effort as its Claude effort unless its tier is haiku, and request it as its Codex effort resolved against the run's catalog snapshot, and SHALL otherwise keep the automatic effort.

- `[NEW] QualityConf.AgentEffort(provider, agent) (string, bool)`는 `AgentTier`(`quality_tier.go:76-91`)와 같은 프리셋을 본다. `NativeBalancedAgentCandidate`(`native_balanced.go:20-24`)는 명시 등급처럼 명시 effort도 표준 배치를 떠나는 조건으로 본다.

### REQ-MUX3-03 effort 선택 (Claude Code·Codex 탭)
Priority: Must · EARS: EventDriven

WHEN the operator answers a tier select of a group or an agent on the Claude Code or Codex tab, THEN THE SYSTEM SHALL ask that tool's effort for the affected agents that can take one, in a select whose default is 그대로 and whose other options are exactly the levels the Effort Option Contract allows for every such agent, labelled with the cells each option produces, and SHALL skip the select when no affected agent can take an effort or no level is allowed.

- 라벨은 SPEC-MODELUX-002 Label Contract의 `<cells>`에서 effort만 그 단계로 바꾼 것이고, 자동과 같은 단계에는 `(자동)`을 붙인다. 자동과 같은 단계를 고른 에이전트는 저장하지 않는다(에이전트마다 판정).

### REQ-MUX3-04 effort 저장
Priority: Must · EARS: EventDriven

WHEN the operator confirms a Claude Code or Codex tab edit whose tiers or efforts differ from its start, THEN THE SYSTEM SHALL write `quality.presets.<p>-custom` with every canonical agent's tier and the explicit efforts of the working composition in the one saveQualityBlock edit of the Per-Tool Binding Contract, and SHALL print the old-binary line only when the write introduces the first `efforts` mapping of the file.

- 바뀌는 설정 줄에 `quality.presets.<p>-custom.efforts 새로 씀 (N개 에이전트)`(또는 `고쳐 씀`, `지움`)이 더해진다. init은 SPEC-MODELUX-002처럼 메모리 cfg에 같은 값을 넣는다.

### REQ-MUX3-05 생성 = 미리보기
Priority: Must · EARS: EventDriven

WHEN a confirmed effort change is applied in the same run by update generation, auto quality --apply, or init generation, THEN THE SYSTEM SHALL produce Claude Code `effort` and Codex `model_reasoning_effort` values equal to the after-table cells computed with the run's snapshot.

### REQ-MUX3-06 agent source key
Priority: Must · EARS: StateDriven

WHERE `role_model_policy.agent_source` is `native`, THEN THE SYSTEM SHALL serve OMP with the five native agents as v0.50.125 does, and SHALL otherwise serve OMP with Autopus agents, reject any value other than absent, `autopus`, or `native` at load with `role_model_policy.agent_source_invalid`, and never treat `agent_source` alone as a configured role model policy.

- `[NEW] RoleModelPolicyConf.AgentSource`(`yaml:"agent_source,omitempty"`). 정책 존재 판정 `role_model_policy_validate.go:22-25`에 넣지 않는다. 모드 helper는 `[NEW] pkg/config/omp_agent_source.go`. 이 key를 쓴 파일은 v0.50.113–v0.50.125가 읽지 못한다.

### REQ-MUX3-07 OMP agent 파일 생성
Priority: Must · EARS: EventDriven

WHEN OMP files are generated in autopus mode, THEN THE SYSTEM SHALL write `.omp/agents/<agent>.md` for the 16 canonical agents by the OMP Agent File Contract, write no `model` field, write `thinkingLevel` only for the agents that the generation's `task.agentModelOverrides` does not name, and record each written file in the OMP manifest.

- 생성기는 `61f72bb0^`의 `prepareAgentMappings`·`renderAgentForOMP`를 model 인자 없이 되살린 것이다(`[NEW] pkg/adapter/omp/omp_agents.go`, `[NEW] pkg/content/agent_transformer_omp_render.go`). route가 있는 에이전트에 `thinkingLevel`을 쓰지 않으므로 override `:<thinking>`과의 우선순위가 결과를 바꾸지 않는다(F-018). `omp.go:106-114`의 은퇴 정리는 native 모드에서만 돈다.

### REQ-MUX3-08 reviewer와 OMP `/review`
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL give the generated reviewer an `output` schema equal to the `output` of the omp 18.6.1 bundled reviewer captured in `[NEW] testdata/omp-18.6.1-reviewer-output.yaml`, so that OMP's review command keeps its verdict, while Autopus dispatches keep their own per-call schema.

- 근거: `/review`는 `agent: "reviewer"`로 보내고 verdict 필드를 읽는다(S:780806-780816, S:338744-339163). schema 우선순위는 caller > agent(S:526717). golden은 `omp agents unpack`(P0)의 `output` 블록이다. 런타임 verdict는 S23(CD-2)이 닫는다.

### REQ-MUX3-09 사용자 agent 파일
Priority: Must · EARS: Unwanted

IF `.omp/agents/<agent>.md` exists and the OMP manifest does not record it, THEN THE SYSTEM SHALL leave it byte-identical in both Generate and Update, skip only that agent, and print the user-file line, and when the manifest records it and its current on-disk checksum differs from the manifest-recorded checksum, THE SYSTEM SHALL overwrite it only through the OMP transaction, whose existing backup keeps the user's bytes.

- 넣는 자리는 둘이다. Generate의 쓰기 loop(`omp.go:89-100`, manifest는 은퇴 제거에만 쓴다)와 Update의 action resolver(`omp_rooted_lifecycle.go:98-134`). transaction은 덮기 전 기존 bytes를 `.autopus/backup/<id>/transaction/omp/<path>`(0600)에 남긴다(`omp_rooted_transaction.go:114`). 새 백업 경로는 만들지 않는다.

### REQ-MUX3-10 project claim 집합
Priority: Must · EARS: EventDriven

WHEN OMP files are generated, THEN THE SYSTEM SHALL compute the project claim set of the Claim Set Contract, apply it with one MergeOMPProjectManagedConfig call over the ledger preimage and one ledger update inside the OMP transaction, keep task enabled, leave a user-owned `task.disabledAgents` unchanged while reporting `agents:disabled_agents_user_owned` as degraded, and restore every byte of `.omp/config.yml`, the ledger, the manifest, and the agent files when the transaction fails.

- plain `omp`는 overlay를 읽지 않고 project `.omp/config.yml`을 읽으므로(S:466140-466160) 이 claim은 config_mode와 무관하게 `.omp/config.yml`에 간다. task를 끄면 이름 없는 dispatch가 preflight에서 실패한다(S:526773, P4). 새 파일은 0600이다(`omp.go:230-233`).
- 사용자가 `task.disabledAgents`만 고치면 프로필 claim이 함께 있어도 사용자 소유로 넘기고 값을 되돌리지 않는다(Claim Set Contract의 사용자 편집 행, S24). 그 표시는 C가 비어 release해도 표시만 남은 ledger로 남는다.

### REQ-MUX3-11 Autopus 이름 route
Priority: Must · EARS: StateDriven

WHERE a role model policy profile is selected in autopus mode, THEN THE SYSTEM SHALL resolve one route per canonical agent and one for task by the Route Contract, validate the policy at load by the autopus rule instead of the native collapse, and write `task.agentModelOverrides` keyed by those names only through the existing activation, attestation, readback, and rollback path.

### REQ-MUX3-12 소비처
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL derive every OMP agent consumer listed in the Agent Set Contract from the mode's agent set, so that in autopus mode a generated file under its own name is never reported as a shadow, a missing generated file is reported as degraded, and only routes the profile derived may degrade to the runtime default.

### REQ-MUX3-13 OMP 탭 (autopus)
Priority: Must · EARS: EventDriven

WHEN the OMP tab runs in autopus mode, THEN THE SYSTEM SHALL ask no profile or family, offer 역할 그룹 and 에이전트별 over the 16 canonical agents, list for each selection 그대로, OMP 세션 모델 그대로, and the attested installed models of every provider by the OMP Autopus Tab Contract, ask a thinking select over only the levels the chosen model reports, start from the current pins or the current profile's effective routes, and persist the result as a pins-only policy through the existing OMP plan, write, and activation paths.

- 증명 규칙: strict catalog면 family·capability를 보고하고 영향받는 모든 에이전트의 capability를 덮는 model만, metadata 없는 catalog면 설치 model 중 built-in 선언에 있는 것만 보이고 안내 1줄을 쓴다. 이 목록은 autopus 탭 전용 `[NEW] installedOMPAttestedModelChoices`(`quality_omp_attested_choices.go`)이고 adapter·CLI plan과 같은 증명 규칙(Pins-only Policy Contract 증명 행)을 쓴다. 줄 OMP 편집기와 native 탭의 `installedOMPCustomModelChoices`(`quality_omp_custom.go`)는 좁히지 않는다: 설치 model 전부와 그 출력·번호가 v0.50.125와 같다(S16).
- 이 탭이 더하는 catalog 읽기는 시작 값이 프로필일 때의 pin 없는 pre-plan 1회다. 모델 목록, 확인 전 plan, `applyOMPProfile`의 재해석(`platform_omp_profile_apply.go:190` → `platform_omp_profile_effective.go:60`), 활성화 Update는 기존 경로라 탭 진입당 읽기 횟수는 단언하지 않는다. REQ-MUX3-16의 run당 1회 규칙은 Codex snapshot에 관한 것이다.
- update·quality는 기존 OMP 쓰기(`applyOMPProfile`과 cfg 동기화), init은 메모리 경로(SPEC-MODELUX-002 REQ-MUX2-11, 22, 23)다. native 모드 탭, 줄 OMP 편집기(`runOMPQualityInteractive`), `auto platform omp profile apply`는 지금처럼 프로필을 쓴다.

### REQ-MUX3-14 update migration
Priority: Must · EARS: EventDriven

WHEN auto update runs in autopus mode on a project whose OMP manifest records no `.omp/agents` file, THEN THE SYSTEM SHALL show the migration lines of the Migration Contract in the preview and perform the agent files, the claim set, and any profile activation in the OMP adapter's one transaction, without writing autopus.yaml for the migration.

### REQ-MUX3-15 native로 되돌리기
Priority: Must · EARS: EventDriven

WHEN OMP files are generated with `agent_source: native`, THEN THE SYSTEM SHALL prune the manifest-recorded generated agent files, drop `task.disabledAgents` from the claim set so that the Claim Set Contract restores its preimage, and route, render, and report the five native agents as v0.50.125 does.

### REQ-MUX3-16 기존 안전 성질
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL keep every SPEC-MODELUX-001 and SPEC-MODELUX-002 safety property (raw quality writer, alias and merge refusal, placeholder guards, terminalSafe, no stdin read on non-interactive runs, one Codex catalog snapshot per run, OMP attestation, readback, and rollback, manifest ownership), keep the line driver's output and the generation of effort-free files byte-identical to v0.50.125, and change existing tests only by the rules of Existing Test Changes.

### REQ-MUX3-17 test seam과 수동 smoke
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL run every new path through the existing seams (composeRunForm, composeFormsProbe, ompModelDoctorE2ERunner, platformOMPModelRunner, ompCLIFakeRunner, the RunWithInput fake of `omp_model_activation_rpc_test.go`, the test-only `[NEW] ompRootedWriteFailureHook`) and SHALL record the real-terminal smoke S17 and the consented live check S23 in `evidence/manual-smoke-003.txt`.

### REQ-MUX3-18 locale
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL render every new title, option, note, and change line in Korean when tui.DetectSystemLang returns ko and in English otherwise, while written values stay locale-independent.

### REQ-MUX3-19 문서
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL document effort editing, the `efforts` key and its old-binary limit, the Autopus OMP default, `agent_source: native` with its removal in v0.50.127, and the migration in init, update, and quality help, README, and the v0.50.126 CHANGELOG, without removing sentences existing tests check.

### REQ-MUX3-20 efforts 보존
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL carry every preset's `efforts` through each path that copies, rebuilds, previews, or refreshes a quality preset by the Efforts Preservation Contract, so that an edit or refresh that does not change an agent's effort leaves its stored effort, preview cell, and generated value unchanged.

### REQ-MUX3-21 OMP 렌더 mode
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL render every OMP surface that names an agent through the OMP Render Mode Contract, so that in autopus mode generated skills, rules, commands, workflows, agent bodies, and the pipeline template dispatch canonical agent names and never select scout, security-reviewer, or sonic, while native mode renders byte-identical to v0.50.125.

### REQ-MUX3-22 프로필 없는 pin
Priority: Must · EARS: StateDriven

WHERE role_model_policy holds agent pins without a profile in autopus mode, THEN THE SYSTEM SHALL load the policy, route only the pinned agents and task by the Route Contract through the Pins-only Policy Contract, emit no override key for any other agent, attest every pin against the installed catalog or the built-in declarations, and fail closed when a pinned model is unavailable, while native mode keeps rejecting agents without a profile.

- `agents_require_profile`(`role_model_policy_validate_agents.go:21-23`)는 native 모드에만 남는다. integration gate(`omp_model_integration.go:39-41`)는 프로필과 agents가 모두 없을 때만 건너뛴다.

### REQ-MUX3-23 프로필 설정의 변환
Priority: Must · EARS: EventDriven

WHEN the operator applies an OMP tab edit over a profile-based policy in autopus mode, THEN THE SYSTEM SHALL write a pins-only policy in which every agent the operator did not change keeps all candidates of its previous effective route, every agent set to the OMP session model has no pin, the legacy native keys scout, security-reviewer, and sonic are expanded into per-agent pins and removed, and `agents.task` keeps the task route as a pin only when that route differs from the planner route after the conversion, SHALL print the conversion line before the confirmation, and SHALL leave the file unchanged when the operator cancels.

- degraded route: pin 없는 pre-plan은 built-in route가 runtime default로 떨어지는 경우(`explicit_degraded`)를 blocker로 낸다. 그래서 그런 프로필은 변환하지 않고 오류 줄 뒤 메뉴로 돌아가며, 변환은 route를 말없이 세션 모델로 바꾸지 않는다("바꾸지 않은 에이전트의 route는 그대로"가 늘 참이다).
- task: 변환 전 task route(Route Contract task 행)와 변환 뒤 planner route가 다르면 `agents.task`를 그 route의 candidate 전부로 남기고, 같으면 지운다(task는 planner를 따른다). 그래서 변환은 task의 override 값도 바꾸지 않는다. 남은 `agents.task`는 task 자신만 정하고(pins-only에는 Route Contract ②가 없다) 에이전트별 목록에 `task`로 보인다.

## Contracts

### Effort Option Contract

| 도구 | 허용 단계 | 원천 | 묻지 않는 경우 |
|---|---|---|---|
| Claude Code | `low`, `medium`, `high`, `xhigh`, `max`. haiku 등급 에이전트는 대상에서 빠진다 | Claude Code 문서(Fable 5.1·Opus 5.5·Sonnet 5.5) | 영향받는 에이전트가 모두 haiku |
| Codex | 영향받는 모든 에이전트의 후보 model(run snapshot으로 해석한 effective model)의 `supported_reasoning_levels` 교집합 ∩ 위 다섯 단계 | `codex_profile.go:66-75`, `codex_profile_resolve.go:30-33` | snapshot을 못 읽음(안내 1줄) 또는 교집합이 빔 |
| OMP | 고른 model이 보고한 thinking 단계만(OMP Autopus Tab Contract) | installed catalog | 단계가 하나 이하, `OMP 세션 모델 그대로` |

### Effort Storage Contract

| 도구 | 저장 | 생성 |
|---|---|---|
| Claude Code, Codex | `quality.presets.<p>-custom.efforts.<agent>`(자동과 다른 에이전트만) | Claude `effort:`(haiku 제외), Codex `model_reasoning_effort`(catalog 해석) |
| OMP | 기존 pin `role_model_policy.agents.<agent>.candidates[0].thinking` | override 값 `<selector>:<thinking>` |
| 자동 | key 없음 | 지금 자동 값(`ClaudeAgentEffort`, Codex 사다리, 선언 effort) |

### Efforts Preservation Contract

| 자리 | 규칙 |
|---|---|
| `composeBindingPlan.set`, `composeCandidate`(`quality_compose_bind_write.go:29-52`) | 복사하는 프리셋의 efforts를 그대로 옮긴다 |
| 모든 도구 같게의 binding(`composeBindTarget`, `quality_compose_bind.go:143-174`) | 편집 전 전역 binding 프리셋 P(`quality.EffectiveMode("")`)에 efforts가 없으면 지금 동작 그대로다. P에 efforts가 있을 때: 결과가 P 자신인 built-in의 최신 기본 등급이고 P가 Historical이면 그 갱신으로 등급·description만 바꾸고 efforts를 남긴다(`u` 갱신 행과 같다). 결과가 다른 built-in X의 기본 등급과 같으면 Current(쓰기 없음)·Historical(갱신)·Absent(새로 씀) 분기를 타지 않고 `custom`에 묶는다. built-in 항목은 사용자가 넣지 않은 efforts를 받지 않고, 분류(`ClassifyBuiltinQualityPreset`, Agents만 비교)는 바뀌지 않는다. `custom`에는 P의 efforts를 모든 에이전트에 옮긴다(등급이 바뀐 에이전트 포함, 도구별 탭 effort Select의 기본 `그대로`와 같다). P와 다른 이름에 옮기면 `quality.presets.<name>.efforts 옮겨 씀 (N개 에이전트)`를 쓴다 |
| 도구별 미리보기(`withQualityToolPreset`, `quality_tool_interactive.go:207-222`)와 저장(`setQualityToolPreset`, `quality_tool_persist.go:49-61`) | 작업 구성의 efforts(기존 + 이번 편집) |
| 최신 기본값 갱신(`u`, `ClassifyBuiltinQualityPreset` 분류는 그대로) | 등급과 description만 최신으로 바꾸고 efforts는 옮긴다 |
| `config.Save`, raw quality writer | typed 필드라 그대로 왕복한다 |

### Agent Set Contract

| 소비처 | autopus (기본) | native |
|---|---|---|
| route key·load 검증: `role_model_policy_native_agents.go`, `role_model_policy_validate.go:52-54,72-80`, `[NEW] role_model_policy_autopus_agents.go` | Route Contract, `[NEW] validateOMPAutopusAgentRoutes` | 접힘, `validateOMPNativeAgentCollapse` |
| bridge·projection·overlay·compile: `omp_model_integration_bridge.go`, `omp_model_projection.go:59-132`, `omp_model_projection_overlay.go:19`, `omp_model_routing_compile.go:33,105-112` | 정규 16 + task 이름 그대로, 순위는 정규 순서 뒤 task, family 기준 executor | v0.50.125 |
| degraded: `omp_model_integration_degraded.go:21-37` | Route Contract ①②면 operator pin, ③만 degrade 가능 | `OMPNativeAgentOperatorPinned` |
| activation·doctor: `omp_model_integration_activation.go`, `omp_model_doctor_projection.go`, `doctor_omp_model_routing_probe.go` | key 17개 | key 5개 |
| CLI: `platform_omp_agent_catalog.go`, `platform_omp_projection_build.go`, `platform_omp_profile_preview.go`, `pipeline_backend_omp_models.go`, `pipeline_backend_orca_models.go`, `quality_compose_table.go`, `quality_compose_omp.go` | catalog 17행, `agents:autopus_agent_missing`, 생성 이름은 shadow 아님, phase 역할 이름 그대로, 표 칸은 자기 route, 탭 16개 | v0.50.125 |
| 줄 OMP 편집기 `quality_omp_custom.go:57-58` | 그대로(native pin, Route Contract ②로 펼침) | 그대로 |
| 렌더, `.omp/agents`, claim | OMP Render Mode Contract, 16개 생성, claim 집합에 `task.disabledAgents` | native 접힘, 생성 안 함·prune, claim 없음 |

### OMP Agent File Contract

| frontmatter | 값 |
|---|---|
| `name`, `description` | 정규 에이전트 이름, `content/agents/<agent>.md`의 description(`OMPYAMLScalar`) |
| `tools` | `61f72bb0^` 규칙: portable 도구 map(TodoWrite 제외), MCP 이름 유지, 역할별 native 도구(debugger·executor·reviewer·validator `lsp`, frontend-specialist `browser`·`inspect_image`·`lsp`, ux-validator `browser`·`inspect_image`), 정렬 |
| `thinkingLevel` | 이번 생성의 `task.agentModelOverrides`가 그 이름을 담지 않을 때만 선언 effort(`max`, `high`, `medium`) |
| `output` | reviewer만: golden과 같다 |
| `model`, `spawns` | 쓰지 않는다 |

- 본문은 `NormalizeOMPResourcePaths`와 OMP Render Mode Contract의 autopus 렌더를 거친다.

### Claim Set Contract

| 항목 | 규칙 |
|---|---|
| ledger | 기존 `.autopus/omp-model-project-ownership-v1.json` 하나. bytes는 담지 않고 commitment와 지문만 담는다: `original_missing`·`original_commitment`, `last_emitted_commitment`, key마다 `fingerprint`·`prior_fingerprint`·`owner`(`profile` 또는 `agent_surface`), `agent_surface_emitted`(마지막으로 쓴 값), `agent_surface_user_owned`(`omp_model_integration_ownership.go:25-57`). 이 계약의 preimage는 이 기록으로 확인해 다시 얻는 원본 내용(claim key를 prior 값으로 되돌린 파일)이다 |
| claim 집합 C | project-managed 프로필이면 그 `ManagedKeys` 전부 ∪ autopus 모드이고 사용자 소유(표시가 있거나 아래 첫 claim 판정이 사용자 값으로 본 경우)가 아니면 `task.disabledAgents = [scout, security-reviewer, sonic]`(FullArrayOwnership) |
| 첫 claim에서 그 key가 이미 있음 | ledger에 그 key의 claim도 표시도 없는데 `.omp/config.yml`에 `task.disabledAgents`가 이미 있으면 값으로 판정한다. `[scout, security-reviewer, sonic]`과 다르면 처음부터 사용자 소유다: C에 넣지 않고 표시를 쓰며(C가 그 밖에 비면 표시만 남은 ledger를 만들고, 프로필 claim이 있으면 그 ledger에 함께 둔다), 파일 bytes는 그대로 두고 `사용자가 정해 두어` 안내 1줄과 degraded를 낸다. 그 key는 C에 없으므로 claim 목록에 오르지 않고, 그래서 소유 key의 prior 재계산(`omp_model_integration_project.go:60-70`)과 `applyOMPManagedClaim`의 prior 지문 비교(`omp_model_activation_project.go:167`, 불일치면 `managed_key_conflict`)에 닿지 않는다. 값이 그 집합과 같으면 현재 값을 preimage 값으로 보고 그대로 claim한다(prior = 현재 지문이라 충돌하지 않고 값은 그대로다. release는 그 값이 든 preimage를 복원하므로 key가 남는다) |
| 적용 | C가 비지 않으면 생성마다 preimage 위에 C 전부로 `MergeOMPProjectManagedConfig` 한 번, config mapping 하나, ledger 갱신 하나. model integration이 없어도 mapping을 만든다(`omp.go:208-216` gate 변경, `omp_model_integration_project.go:14-99`의 수 검사는 프로필 claim에만) |
| C가 빔 | ledger가 있으면 release: preimage를 복원하고(ADK가 만든 파일이면 삭제) claim을 모두 지운다. 사용자 소유 표시가 없으면 ledger를 지우고, 있으면 표시만 담은 ledger(claim과 commitment 없음)를 남긴다(`omp_rooted_lifecycle_config.go:64-83`을 C 기준으로) |
| 표시만 남은 ledger | 관리하는 key가 없다. autopus 모드에서는 표시를 판단하려고 `.omp/config.yml`을 읽기만 하고, 아래 재claim이 아니면 쓰지 않는다. C에 프로필 claim이 다시 생기면 현재 파일을 새 preimage로 잡고 표시를 그대로 둔다. native 모드에서도 표시는 남는다 |
| 사용자가 C 밖을 고침 | C에 `profile` claim이 있으면 지금처럼 `managed_key_conflict`이고 아무것도 쓰지 않는다(`task.disabledAgents`를 같이 고쳤어도 같다). C가 `agent_surface`뿐이면 아래 `task.disabledAgents` 판정을 먼저 하고, 새 preimage = 현재 파일에서 ADK claim key만 preimage 값으로 되돌린 것으로 다시 잡고 적용한다 |
| 사용자가 `task.disabledAgents`만 고침(지움 포함) | 판정: 현재 파일의 그 key를 ADK가 마지막으로 쓴 값으로 바꾼(없으면 같은 자리에 넣은) bytes의 commitment가 `LastEmittedCommitment`(`omp_model_integration_ownership_rooted.go:49-62`)와 같으면 그 key만 바뀐 것이다. 프로필 claim이 있든 없든 사용자 소유로 넘긴다: 새 preimage = 원본 preimage에 사용자 값(지웠으면 삭제), C에서 그 key를 빼고, ledger에 `[NEW] agent_surface_user_owned`를 남기며, 파일 bytes는 그대로 두고 안내 1줄과 degraded를 낸다. 그 뒤의 merge·전환·release는 새 preimage를 쓰므로 사용자 값을 되돌리지 않는다 |
| 사용자 소유 표시가 있음 | 그 key를 claim하지 않는다. 표시는 release·전환·native 뒤에도 남고 `auto clean`만 지운다. 생성마다 현재 값을 읽어(표시만 남은 ledger도 같다) 사용자 값이 `[scout, security-reviewer, sonic]`과 같아지면 현재 파일을 preimage로 잡고 표시를 지운 뒤 다시 claim한다 |
| clean | `agent_surface`만 있는 ledger는 receipt 없이 release와 같게 복원한다. 표시만 남은 ledger는 파일을 바꾸지 않고 지운다(`omp_model_integration_clean.go:14-32` 조건 변경) |
| 실패 | OMP transaction 실패는 `.omp/config.yml`, ledger, manifest, agent 파일을 실행 전 bytes로 되돌린다(기존 rooted transaction). 활성화 실패는 기존 복원이다 |
| allowlist | `task.disabledAgents`는 프로필 `managed_keys`에 넣지 않는다(`role_model_policy_validate.go:174-190` 그대로). `agent_surface` claim은 adapter가 만든다 |

| 전환 | `.omp/config.yml` 결과 |
|---|---|
| 없음 → autopus(프로필 없음 또는 overlay) | preimage + `task.disabledAgents`. 새 파일은 0600. 그 key가 이미 다른 값이면 바꾸지 않고 표시만 남은 ledger를 만든다(첫 claim 행) |
| autopus + project-managed 프로필 | preimage + 프로필 key + `task.disabledAgents`(merge 한 번) |
| project-managed → overlay 또는 프로필 해제 | preimage + `task.disabledAgents` |
| → native | C가 비면 preimage 복원(없던 파일이면 삭제), 아니면 preimage + 프로필 key |

### OMP Autopus Tab Contract

| 단계 | 형식 | 선택지(순서) | 기본값 | 다음 |
|---|---|---|---|---|
| 시작 값 | 폼 밖 | 지금 policy가 pins-only면 그 pin, 프로필이 있으면 pin 없는 pre-plan의 effective route, policy가 없으면 모두 `OMP 세션 모델` | - | 방식 |
| 방식 | Select `어떻게 고칠까요?` | 역할 그룹 · 에이전트별 | 역할 그룹 | 편집 |
| 역할 그룹 | Role Group Contract 순서(설계·판단, 구현, 검증·리뷰, 탐색·보조)로 그룹마다 model Select, 제목 `<그룹> — <멤버>` | `그대로 — <현재>` · `OMP 세션 모델 그대로` · 증명된 설치 model(`provider/model`, provider와 model 이름순, 모든 provider가 섞인다) | 그대로 | thinking |
| 에이전트별 | 목록 Select → model Select 반복 | 목록: 완료 + 16개(`<agent> — <현재>`) + `agents.task`가 있으면 `task — <현재>` / model: 위와 같고, `task`는 `OMP 세션 모델 그대로` 대신 `planner를 따름`(pin을 지운다) | 완료 / 그대로 | 완료: 미리보기 |
| thinking | Select(고른 model이 보고한 단계가 둘 이상일 때) | 그 model이 보고한 단계만 | 현재 thinking이 목록에 있으면 그것, 아니면 첫 단계 | 다음 그룹 |
| 미리보기 | 폼 밖: 기존 요약(`renderOMPQualitySummary`), 변환 줄(REQ-MUX3-23), 옛 바이너리 줄 | - | - | 확인 |
| 확인 | Select `적용할까요?` | 취소 · 적용 | 취소 | 적용: 기존 OMP 쓰기 경로 → 메뉴 |

- `<현재>`는 pin이나 route가 있으면 `provider/model · thinking`, 없으면 `OMP 세션 모델`이다. `OMP 세션 모델 그대로`는 그 에이전트의 pin을 지우고 thinking을 묻지 않는다.
- 설계·판단 그룹 편집은 planner를 바꾸고 기존 `agents.task`는 바꾸지 않는다. task는 `agents.task`가 없을 때만 planner를 따른다.
- 증명된 model: REQ-MUX3-13의 규칙. 그룹은 영향받는 모든 에이전트가 받는 model만 보인다. 결과가 시작과 같으면 `변경 없음`이고 쓰지 않는다. pin이 하나도 남지 않으면 `role_model_policy`의 `profile`·`family`·`agents`를 지우고, 남는 field가 없으면 key 전체를 지운다.
- 프로필·family 단계는 autopus 모드에서 묻지 않는다. 시작 값을 얻는 pre-plan이 blocker를 내면 오류 줄 뒤 방식 Select 없이 메뉴로 돌아가고 쓰거나 활성화하지 않는다. native 모드는 SPEC-MODELUX-002 OMP Tab Contract 그대로다.

### Pins-only Policy Contract

| 항목 | 규칙 |
|---|---|
| 저장 YAML | `role_model_policy: {version: v1, agents: {<agent>: {candidates: [{selector, thinking, family}]}}}`. 기존 `config_mode`, `agent_source`는 둔다. `profile`·`family`는 쓰지 않는다. 새 key는 없다 |
| load 검증 | candidate(`agents.*.candidates`, `profiles.*` route와 agents의 candidates)의 scalar에 `${`가 있으면 loader가 펼치기 전에 `role_model_policy.candidate_placeholder_unsupported: <path>`로 거부한다(root `profile`·`family` placeholder는 SPEC-MODELUX-002처럼 읽고 OMP writer가 다시 쓰기를 거부한다). autopus 모드는 프로필 없는 `agents`를 받는다. key는 정규 16과 task뿐이고(scout·security-reviewer·sonic은 `[NEW] role_model_policy.agents_native_key_requires_profile`) candidates가 필요하다(`role_model_policy_validate_agents.go:32-34`). 남은 `agents.task`는 task 자신만 정한다(Route Contract ②는 프로필이 있을 때만). native 모드는 `agents_require_profile` 그대로 |
| 활성화 | `[NEW] config.PinsOnlyRoleModelProfile(configMode)`(capability route 없음, `ManagedKeys` = `builtinRoleModelManagedKeys(mode)`. catalog trust는 catalog가 정한다: strict catalog면 strict라 receipt가 availability 증거를 담고, metadata 없는 catalog면 operator-attested다) 위에 root pin을 얹고 지금 경로(catalog probe, bridge, compile, projection, overlay 또는 claim 집합, 활성화, 되읽기, rollback)를 탄다 |
| override | pin된 에이전트와 Route Contract의 task 행만 key를 갖는다. 나머지는 key가 없어 세션 모델을 상속하고 frontmatter `thinkingLevel`(선언 effort)을 갖는다(REQ-MUX3-07) |
| 증명 | pin의 model·family는 strict catalog의 보고(family·capability), 또는 metadata 없는 catalog에서는 built-in 선언 합집합(`[NEW] config.BuiltinRoleModelSelectorFamilies`: built-in family anthropic·openai의 모든 tier model)으로만 증명한다. pin은 이미 autopus.yaml에 있어도 자기 model·family를 증명하지 못하고, 선언의 family가 pin이 적은 family보다 우선한다. TUI 목록(`ompBuiltinDeclaredFamilies`), CLI plan, adapter 활성화가 같은 규칙이다. 아니면 CLI는 `agent_override_model_undeclared:<selector>` plan blocker, adapter는 `pin_model_unattested`(선언에 없는 model) 또는 `pin_family_mismatch`(다른 family)로 실패하고 아무것도 쓰지 않는다 |
| degrade | 모든 route가 operator pin이라 provider가 없으면 `required_route_unresolved`로 실패하고 아무것도 쓰지 않는다. degrade할 파생 route가 없어 override가 비는 일도 없다(빈 override는 기존 활성화가 거부한다, S21) |
| receipt·doctor | profile 칸은 예약 표기 `[NEW] pins-only`(autopus.yaml에 쓰지 않는다). 이 이름은 예약이라 `profiles.pins-only`나 `profile: pins-only`는 load에서 `role_model_policy.profile_name_reserved`, `auto platform omp profile apply`에서 `profile_name_reserved`로 거부한다. doctor는 `disabled/profile_not_selected`(`platform_omp_agent_catalog.go:24-36`) 대신 pin된 에이전트 행을 보인다 |
| 옛 바이너리 | 프로필 없는 `agents`는 v0.50.125 이하가 `agents_require_profile`로 거부한다. 쓰기 전 미리보기에 한 줄 |
| 변환(REQ-MUX3-23) | 프로필 기반 policy를 이 탭에서 고치면 바꾸지 않은 에이전트에 그 effective route의 candidate 전부를 pin으로 옮기고, 세션 모델로 고른 에이전트는 key를 두지 않으며, `scout`·`security-reviewer`·`sonic` key와 13개 역할에 접히던 `agents.task`는 에이전트별 pin으로 펼친다. `scout`·`security-reviewer`·`sonic` key는 지우고, `agents.task`는 변환 전 task route가 변환 뒤 planner route와 다르면 그 route로 남기고 같으면 지운다. `profile`·`family`를 지운다. 확인 전에 Form Text `OMP 변환` 줄(프로필의 route를 에이전트별 지정으로 옮기고 바꾸지 않은 에이전트의 route는 그대로)을 보인다. ultra의 quality 연동이 끝나고 family diversity 힌트가 고정되는 점은 변환 줄이 아니라 README·CHANGELOG가 알린다 |
| pipeline | pin 없는 phase는 세션 모델을 상속한다(phase model 없음). managed active run은 한 model scope를 모든 phase에 묶으므로 phase model이 하나라도 없으면 그 run은 처음부터 canonical 경로를 쓴다. Orca(phase마다 명시 agent)와 context observe-session(implement phase model 필요)은 그 phase에 pin이 없으면 fail closed다 |
| 다른 경로 | 줄 OMP 편집기, `auto platform omp profile apply`, native 모드 탭은 지금처럼 프로필을 쓰고, 그 결과(프로필 + pin)도 그대로 읽힌다 |

### Route Contract

| 대상 | route |
|---|---|
| 정규 에이전트 A | ① `agents.A` pin ② 프로필이 있을 때만: N = `OMPNativeAgentForRole(A)`가 A와 다르고 `agents.N` pin이 있으면 그 pin(task → 13개 역할, scout → explorer, security-reviewer → security-auditor) ③ 프로필이 A에 만든 route. pins-only면 ③은 없다: route와 override key가 없고 세션 모델을 상속한다 |
| task | ① `agents.task` pin ② `agents.planner` pin ③ 프로필의 planner route(pins-only면 없음). 다른 접히는 역할의 pin은 task에 쓰지 않는다 |
| scout, security-reviewer, sonic | route 없음. `agents.sonic` pin은 받을 에이전트가 없어 미리보기에 경고 1줄 |
| load 검증 | autopus: `validateOMPAutopusAgentRoutes`(key는 정규 16, task, scout, security-reviewer, sonic 중 하나, route마다 해석 가능, pins-only면 route가 없는 에이전트를 허용). native: `validateOMPNativeAgentCollapse`와 `agents_require_profile` |
| degraded | ①② route는 operator pin이라 provider가 없으면 실패한다. ③만 지금 규칙으로 runtime default가 될 수 있다 |
| 충돌 | autopus 모드는 `omp_native_agent_conflict`를 내지 않는다 |

### OMP Render Mode Contract

| 항목 | 규칙 |
|---|---|
| mode 원천 | 생성 시점 cfg의 agent source. OMP adapter가 OMP 렌더 호출마다 넘긴다 |
| 진입점 | `[NEW] ReplacePlatformReferencesWith(body, platform, opts)`. 기존 `ReplacePlatformReferences(body, platform)`는 기본 option(autopus)이다. 호출 자리: `skill_transformer_replace.go:89-131`(`NormalizeAgentReferences` 포함), `skill_resources.go:33-66`, `skill_transformer.go:121-149`(`SkillTransformOptions`), `agent_transformer_mapping.go:75`, `generate.go:105`, `pkg/adapter/omp/omp_rules.go:49`, `omp_workflow_render.go:56`, `omp_skills.go:136`, OMP agent 본문 |
| dispatch | autopus는 `"agent": "<role>"`, 이름 없는 dispatch는 생략(task). native는 `skill_transformer_replace_omp_render.go:103-124`의 접힘 |
| 템플릿 | `templates/shared/omp-agent-pipeline.md.tmpl:56-65`의 native 문장을 mode 조건으로 나누고 autopus 문장은 16개 Autopus 에이전트와 task를 고르는 법을 쓴다 |
| cfg 없는 렌더 | `generate.go:105`처럼 cfg 없이 만든 OMP 텍스트는 autopus다. native 생성은 OMP adapter가 원천에서 다시 렌더한다(T8이 경로를 확인한다) |
| oracle | 실제 `Adapter.Generate` 산출물. autopus에서 `.omp/` 아래 어느 파일에도 `"agent": "scout"`, `"agent": "security-reviewer"`, `"agent": "sonic"`, `There is no Autopus-specific agent`가 없다. native에서는 v0.50.125와 bytes가 같다 |

### Migration Contract

| 경우 | 미리보기 | 쓰기 | autopus.yaml |
|---|---|---|---|
| autopus, manifest에 `.omp/agents` 없음 | `+ .omp/agents/<agent>.md` 16줄(사용자 파일은 사용자 파일 줄), `task.disabledAgents: scout, security-reviewer, sonic`, 안내 1줄 | OMP adapter Update 한 transaction: 파일, claim 집합, ledger, 프로필이 있으면 overlay 또는 config와 활성화·되읽기 | 쓰지 않는다 |
| `agent_source: native` | `- .omp/agents/<agent>.md` 줄, `task.disabledAgents 복원` | prune, claim 집합 갱신, native 5 route 활성화 | 사용자가 쓴 값 그대로 |
| 실패 | - | 기존 transaction 복원과 활성화 실패 복원 | - |

### Form Text (ko / en)

| 자리 | ko | en |
|---|---|---|
| effort Select | `<그룹 또는 에이전트> effort` · `그대로 — <현재>` · 단계 라벨 `(자동)` | `<group or agent> effort` · `keep — <current>` · `(automatic)` |
| Codex 안내 | `Codex catalog를 읽지 못해 effort는 자동입니다` | `Codex catalog unreadable; effort stays automatic` |
| 바뀌는 설정 | `quality.presets.<p>-custom.efforts 새로 씀 (N개 에이전트)` / `고쳐 씀` / `지움`, `quality.presets.*.efforts는 v0.50.126보다 오래된 auto가 읽지 못합니다`, `quality.presets.<name>.efforts 옮겨 씀 (N개 에이전트)` | `… efforts written (N agents)` / `rewritten` / `removed`, `auto older than v0.50.126 cannot read quality.presets.*.efforts`, `quality.presets.<name>.efforts carried (N agents)` |
| OMP model | `OMP 세션 모델 그대로` · 현재 `OMP 세션 모델` · `<provider>/<model> · <thinking>` · `planner를 따름` | `keep the OMP session model` · `OMP session model` · `<provider>/<model> · <thinking>` · `follow planner` |
| OMP metadata 안내 | `설치된 OMP catalog에 family·capability 정보가 없어 Autopus가 증명할 수 있는 model만 보입니다` | `The installed OMP catalog has no family or capability metadata; only models Autopus can attest are listed` |
| OMP 변환 | `OMP 프로필 <name>의 route를 에이전트별 지정으로 옮깁니다. 바꾸지 않은 에이전트의 route는 그대로입니다.` | `Moving the routes of OMP profile <name> to per-agent pins; unchanged agents keep their routes.` |
| OMP 옛 바이너리 | `프로필 없는 role_model_policy.agents는 v0.50.126보다 오래된 auto가 읽지 못합니다` | `auto older than v0.50.126 cannot read role_model_policy.agents without a profile` |
| 사용자 파일 | `.omp/agents/<agent>.md는 사용자 파일이라 그대로 둡니다` | `.omp/agents/<agent>.md is a user file; left unchanged` |
| disabledAgents | `.omp/config.yml의 task.disabledAgents를 사용자가 정해 두어 scout, security-reviewer, sonic을 끄지 않았습니다` | `task.disabledAgents in .omp/config.yml is user-owned; scout, security-reviewer, and sonic stay enabled` |
| migration | `OMP는 이제 Autopus 에이전트 16개를 씁니다. v0.50.126 동안 role_model_policy.agent_source: native로 되돌릴 수 있습니다.` | `OMP now uses the 16 Autopus agents; set role_model_policy.agent_source: native to go back during v0.50.126.` |

## Existing Test Changes

OMP 기본값이 바뀌므로 0건이 아니다. 목록은 `research.md` `### 기존 test`(27개 파일, 49건)다.

1. 기본값 전환 단언 4개 파일(`latest_cli_omp_contract_test.go:24,29`, `omp_acceptance_surface_test.go:52,151`, `omp_latest_contract_test.go:30`, `omp_native_registry_test.go:27`)은 그 단언만 autopus 기대값으로 바꾸고, 같은 파일에 `agent_source: native` fixture를 더해 옛 단언을 native에서 지킨다.
2. native 계약 test는 fixture에 `agent_source: native` 한 줄을 더하거나 렌더 함수에 native option을 준다. 단언은 바꾸지 않는다.
3. SPEC-MODELUX-002 폼 test는 새 effort Select마다 입력에 Enter(`\n`) 하나를 더한다. 기존 줄 단언은 그대로다. 폼 출력 전체를 bytes로 비교하는 test가 있으면 새 Select 블록이 더해진 golden으로 바꾸고 T1 목록에 적는다.
4. 줄 driver test는 바꾸지 않는다. 줄 driver에는 effort 질문이 없다. 나머지는 T1이 파일마다 분류하고 review에 이유를 적는다.

## 생성 파일 상세

- `pkg/config`: `schema.go`(+1줄), `[NEW] quality_effort.go`(+test), `schema_model_selection.go`, `quality_effective.go`, `native_balanced.go`, `codex_profile.go`, `role_model_policy.go`(+1줄), `[NEW] omp_agent_source.go`(+test), `role_model_policy_validate.go`, `[NEW] role_model_policy_autopus_agents.go`(+test), `quality_preset_history.go`(갱신 경로가 이 package면), `role_model_policy_validate_agents.go`, `role_model_policy.go`(선택 프로필), `[NEW] role_model_policy_pins.go`(+test).
- `pkg/content`: `[NEW] agent_transformer_omp_render.go`(+test), `skill_transformer_replace.go`, `skill_transformer_replace_omp.go`, `skill_transformer_replace_omp_render.go`, `skill_resources.go`, `skill_transformer.go`, `agent_transformer_mapping.go`, `generate.go`. `templates/shared/omp-agent-pipeline.md.tmpl`.
- `pkg/adapter/omp`: `[NEW] omp_agents.go`(+test), `[NEW] omp_agent_surface_claim.go`(+test), `omp.go`, `omp_rooted_lifecycle.go`, `omp_rooted_lifecycle_config.go`, `[NEW] omp_rooted_transaction_hook.go`(test 전용 실패 seam), `omp_model_integration_project.go`, `omp_model_integration_ownership_rooted.go`, `omp_model_integration_clean.go`, `omp_model_integration.go`(gate), `omp_model_integration_bridge.go`, `omp_model_projection.go`, `omp_model_projection_overlay.go`, `omp_model_integration_degraded.go`, `omp_model_routing_compile.go`, `omp_model_doctor_projection.go`, `omp_skills.go`, `omp_rules.go`, `omp_workflow_render.go`, `[NEW] testdata/omp-18.6.1-reviewer-output.yaml`.
- `internal/cli`: `[NEW] quality_compose_effort.go`(+test), `[NEW] quality_compose_form_effort.go`(+test), `quality_compose_form_edit.go`, `quality_compose_tool.go`, `quality_tool_persist.go`, `quality_tool_interactive.go`, `quality_compose_bind.go`, `quality_compose_bind_write.go`, `quality_compose_form_text.go`, `[NEW] quality_compose_omp_autopus.go`(+test), `quality_compose_omp.go`(분기 몇 줄), `quality_omp_custom.go`, `quality_compose_omp_init.go`, `platform_omp_profile_declarations.go`, `platform_omp_profile_options.go`, `platform_omp_profile_effective.go`, `platform_omp_profile_plan.go`, `platform_omp_profile_apply.go`, `platform_omp_agent_catalog.go`, `platform_omp_projection_build.go`, `platform_omp_profile_preview.go`, `pipeline_backend_omp_models.go`, `pipeline_backend_orca_models.go`, `doctor_omp_model_routing_probe.go`, `quality_compose_table.go`, `[NEW] platform_omp_agent_migration.go`(+test).
- 문서: README, CHANGELOG(v0.50.126), help 상수. 증거: `evidence/probe-omp-18.6.1.md`, `[NEW] evidence/manual-smoke-003.txt`. 신규·변경 소스는 300줄 이하이고, 292·290줄 파일에는 호출 몇 줄만 더한다.

## Related SPECs

- SPEC-MODELUX-002(v0.50.125): 바뀌는 점은 넷이다. (1) REQ-MUX2-10 "지정 단위 native 5개"는 autopus 모드에서 16개다. (2) non-goal "새 YAML key"를 사용자 결정으로 뒤집는다(`efforts`, `agent_source`). (3) Per-Tool Binding Contract의 프리셋 쓰기와 복사가 efforts를 담는다. (4) 폼 test 입력에 effort Select의 Enter가 더해진다. (5) autopus 모드 OMP 탭은 OMP Tab Contract의 profile·계열 단계를 쓰지 않고(OMP Autopus Tab Contract), 프로필 없는 pin을 저장한다(REQ-MUX3-22). 줄 driver(REQ-MUX2-15)와 OMP 쓰기 경로(REQ-MUX2-11, 22, 23)는 그대로다.
- SPEC-MODELUX-001(v0.50.125): 등급, Binding Contract, raw writer를 그대로 쓴다.
- SPEC-ADK-MINIMALITY-DISCIPLINE-001(61f72bb0): OMP native 재사용 결정을 기본값에서 대체한다. native는 v0.50.126 동안 `agent_source: native`로 남는다.
- SPEC-OMP-001(16개 생성), SPEC-OMP-005(`@autopus_<agent>` alias, 은퇴): 생성은 되살리고 alias는 되살리지 않는다. SPEC-OMP-006 review backend의 `reviewer` 의존은 S15 receipt가 확인한다.
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
| REQ-MUX3-08 | T4 | S7, S23 | INV-MUX3-04 |
| REQ-MUX3-09 | T4 | S8 | INV-MUX3-06 |
| REQ-MUX3-10 | T5 | S7, S8, S22, S24 | INV-MUX3-06, INV-MUX3-09 |
| REQ-MUX3-11 | T3, T6 | S9, S21 | INV-MUX3-04, INV-MUX3-05 |
| REQ-MUX3-12 | T6, T7 | S15, S21 | INV-MUX3-05 |
| REQ-MUX3-13 | T11, T16 | S10, S11, S17, S25 | INV-MUX3-02, INV-MUX3-10 |
| REQ-MUX3-14 | T12 | S12 | INV-MUX3-06 |
| REQ-MUX3-15 | T5, T12 | S13, S22 | INV-MUX3-07, INV-MUX3-09 |
| REQ-MUX3-16 | T1, T14 | S16 | INV-MUX3-01 |
| REQ-MUX3-17 | T14 | S16, S17, S23 | INV-MUX3-05 |
| REQ-MUX3-18 | T13 | S18 | - |
| REQ-MUX3-19 | T13 | S19 | - |
| REQ-MUX3-20 | T10 | S5, S20 | INV-MUX3-03, INV-MUX3-08 |
| REQ-MUX3-21 | T8 | S14 | INV-MUX3-04 |
| REQ-MUX3-22 | T15, T16 | S10, S11 | INV-MUX3-05, INV-MUX3-10 |
| REQ-MUX3-23 | T11, T16 | S25 | INV-MUX3-11 |

# SPEC-MODELUX-003 리서치

worktree `feat/agent-model-ux` HEAD `93dbb564`에서 읽었다(2026-10-12). 줄 번호는 이 HEAD 기준이다. OMP 사실은 설치된 omp 18.6.1 실행 파일의 `strings -n 8` 덤프(`S:<줄>`)와 scratch probe로 확인했고 원문은 `evidence/probe-omp-18.6.1.md`에 있다. 확인하지 못한 것은 `미확인`이다.
## 기존 코드 분석
### effort (Claude·Codex)
- `pkg/config/schema.go:24-27` `QualityPreset{Description, Agents}`. effort key가 없다(파일 290줄). strict decode `loader_strict.go:26-50`(`KnownFields(true)`, 모르는 key는 오류)는 v0.50.113부터다(SPEC-MODELUX-002 research "도구별 저장 값" 행). 그래서 새 key를 쓴 파일은 v0.50.113–v0.50.124가 읽지 못한다. v0.50.113 이전 바이너리가 무시하고 저장 때 지우는지는 미확인.
- 검증 자리 `schema_model_selection.go:7-35`(프리셋 이름, `quality.providers`는 claude·codex만). 등급 `quality_tier.go:76-91` `AgentTier`, Claude ID `:7-12`(haiku = `claude-haiku-4-5`).
- Claude: `quality_effective.go:28-34` `ClaudeAgentProfile`(표준 배치면 matrix thinking), `:48-65` `ClaudeAgentEffort`(haiku 없음, fable max, opus high·ultra max, 그 밖 medium·ultra high). 쓰기 `pkg/adapter/claude/claude_agent_content.go:67-112`(값이 비면 effort 줄 삭제). 표준 배치 `native_balanced.go:9-31`은 표준과 다른 등급을 명시한 에이전트를 custom 경로로 보낸다(L20-24).
- Codex: 사다리 `codex_profile.go:105-124`, effort 순서 `:46`, catalog `supported_reasoning_levels` `:66-75`, `codex_profile_resolve.go:30-33` `Supports`, `:52-111` 해석(없는 effort는 아래 단계, `effort_unavailable`). 표와 템플릿은 `pkg/adapter/codex/codex_resolve.go:21-31`을 같이 쓴다.
- 폼: 등급 Select뿐이다(`internal/cli/quality_compose_form_edit.go:12-57`). 도구별 planner `quality_compose_tool.go:56-73`은 등급 map만 비교한다. `quality_tool_persist.go:49-61` `setQualityToolPreset`은 프리셋을 `{Description, Agents}`로 통째로 바꾼다. effort를 더하면 이 함수가 effort도 써야 기존 값이 사라지지 않는다.
- 300줄 근처: `quality_compose_form_driver.go` 290, `quality_compose_omp.go` 292, `schema.go` 290. 새 로직은 새 파일에 둔다.
- Claude Code 문서(2026-10-12): sub-agents `effort`는 `low`, `medium`, `high`, `xhigh`, `max`이고 "available levels depend on the model", 없으면 세션 effort를 따른다. model-config: Fable 5.1·Opus 5.5·Sonnet 5.5는 다섯 단계 모두, 지원하지 않는 단계는 그 아래 가장 높은 단계로 돈다. Haiku 4.5는 표에 없다.
### OMP native 구조 (SPEC-ADK-MINIMALITY-DISCIPLINE-001 이후)
- 16→5 접힘: `pkg/config/omp_native_agents.go:6-8`(이름 5개), `:12-27`(13개 역할 → task, explorer → scout, security-auditor → security-reviewer), `:32-47` 대표(task=planner, sonic=validator), `:57-63` GovernedBy.
- route key: `role_model_policy_native_agents.go:13`(`task.agentModelOverrides`), `:77-101`, `:118-162`(대표 우선, 대표가 없으면 `omp_native_agent_conflict`), `:166-173` 검증. `role_model_policy_validate.go:22-25`는 version·profile·family·config_mode·profiles·agents가 모두 비면 정책이 없다고 본다. `:52-54` 접힘 검증.
- built-in 프로필은 정규 16개 에이전트마다 자기 route를 이미 만든다(`role_model_policy_builtin.go:3-11`, `:214-240`), thinking은 등급별(`:42-47` fable max, opus xhigh, sonnet medium, haiku low). 접힘은 bridge에서 일어난다.
- adapter: `pkg/adapter/omp/omp.go:20-24`·`:106-114`(manifest에 남은 `.omp/agents`를 지운다), `:169-228` 생성(규칙, 스킬, 명령, context bridge, 프로필이 있을 때만 model integration `omp_model_integration.go:35-41`). bridge `omp_model_integration_bridge.go:31-62`·projection `:66-113`, `omp_model_projection.go:59-99`(L69), `:101-132`(L111), compile 순위 `omp_model_routing_compile.go:105-112`, family 기준 `:33`(task).
- 활성화: overlay `.autopus/runtime/omp-model-routing-v1.yml`(`omp_model_activation.go:15`)은 Autopus가 띄운 프로세스만 `PI_CONFIG_FILES`로 읽는다(`internal/cli/pipeline_omp_context_active_process.go:237`). project-managed는 `.omp/config.yml`에 claim으로 쓴다(`omp_model_activation_project.go:13-19`, `:53`, `:143-188` 지문 불일치는 `managed_key_conflict`, 배열은 `FullArrayOwnership` 필요). 되읽기 `omp_model_integration_activation.go:185-226`, RPC는 override마다 1프로세스(`omp_model_activation_rpc.go:55-95`, worker pool).
- update: `omp_rooted_lifecycle.go:98-134`는 manifest에 없는 기존 파일을 `ActionBackup`으로 판정하지만 `:136-175`는 Skip만 거르고 그대로 쓴다. 영구 백업은 clean에만 있다(`omp_clean_rollback.go:44` `.autopus/backup/<ts>`). Generate는 manifest가 없으면 기존 파일을 덮는다(`:111-114`). 그래서 사용자 `.omp/agents/<name>.md` 보호는 새로 해야 한다.
- 옛 생성기: `git show 61f72bb0^:pkg/adapter/omp/omp_agents.go` L13-29(`content/agents`마다 `.omp/agents/<name>.md`), `61f72bb0^:pkg/content/agent_transformer_omp.go`(도구 map, 역할별 native 도구, frontmatter). 지금 `pkg/content/agent_transformer_omp.go`는 31줄(helper만).
- native 이름 소비처: `internal/cli/platform_omp_agent_catalog.go:42-79`(shadow면 degraded), `platform_omp_projection_build.go`(`agents:native_agent_shadowed` blocker, explorer 조사 L139-143), `platform_omp_profile_preview.go:90-94`, `pipeline_backend_omp_models.go:75-110`, `pipeline_backend_orca_models.go`, `doctor_omp_model_routing_probe.go`, `quality_compose_table.go`, `quality_omp_custom.go:57-58`, `quality_compose_omp.go:169,193`. skill dispatch `pkg/content/skill_transformer_replace_omp_render.go:103-124`(task는 생략), 진입 `skill_transformer_replace_omp.go:57`(cfg 없음, 호출 `skill_transformer_replace.go:96`, `pkg/adapter/omp/omp_skills.go:136`). `doctor_omp_selector_validation.go:25-50`는 `.omp/agents` frontmatter model을 이미 검사한다.
- test: native helper를 부르는 `_test.go`는 grep으로 15개다. native 이름 문자열까지 세면 explorer 추정 약 32개(미확인).
## OMP probe 결과 (`evidence/probe-omp-18.6.1.md`)
| ID | 질문 | 방법 | 결과 |
|---|---|---|---|
| P0 | bundled 정의 | `omp agents unpack --dir <scratch>` | PASS. 5개. reviewer는 `model: ["@slow"]`, `output`(overall_correctness·explanation·confidence·findings). task는 `@task`, `thinkingLevel: auto` |
| P1 | project 파일 발견과 reviewer 대체 | 정적 S:524490(project → … → bundled, 이름 선착), S:359871(name·description 필수) | live 미확인: 18.6.1 `omp agents`는 `unpack`뿐이고 읽기 전용 목록이 없다. 과거 live 근거: 61f72bb0 Tested "실제 OMP 세션 발견 20개→기본 5개"(16 + 5 − reviewer 1) |
| P2 | disabledAgents가 목록에서 뺌 | 정적 S:564994, S:526773 | 설정 되읽기 PASS(P3b), 세션 목록 live 미확인 |
| P3 | 사용자 이름 key override | `PI_CONFIG_FILES` overlay + `omp config get … --json` | PASS. `planner` key가 record로 되읽힌다. dispatch 적용은 정적 S:526773(`overrides[<agent>]`), live 미확인 |
| P4 | task를 꺼도 되는가 | 정적 S:526773 | 안전하지 않다. 이름 없는 dispatch는 `defaultAgent`(task)로 가고 꺼진 agent는 preflight 오류다. 사용자 결정대로 task는 켠다 |
- 추가 발견: OMP `/review`(S:780806-780816)는 `reviewer`에 1개 task를 보내고 verdict 필드를 읽는다(S:338744-339163). 출력 schema는 caller > agent > session(S:526717)이라, Autopus pipeline dispatch(호출마다 `outputSchema`, `skill_transformer_replace_omp_render.go:90-91`)는 영향이 없고 `/review`만 agent `output`에 기댄다.
- 그 밖: plain `omp`는 project `.omp/config.yml`을 설정 level로 읽는다(S:466140-466160). override의 `:<thinking>`이 frontmatter `thinkingLevel`보다 먼저인지는 정적 근거 일부(S:543407)뿐이라 미확인.
## 설계 결정
| 결정 | 선택 | 기각한 대안 | 근거와 trade-off |
|---|---|---|---|
| effort 저장 | `quality.presets.<name>.efforts.<agent>`, 값 `low`·`medium`·`high`·`xhigh`·`max`, load 때 검증 | 에이전트별 ID 저장, `quality.providers.omp` 신설 | 사용자 결정. 두 도구가 같이 받는 값의 교집합이다. 대가: key를 쓴 파일은 ≤ v0.50.124가 못 읽는다. 쓰기가 key를 처음 만들 때만 미리보기에 한 줄 |
| 명시 effort와 표준 배치 | 명시 effort가 있으면 그 에이전트는 표준 배치를 떠난다(등급 이동과 같다) | 배치의 model은 두고 effort만 교체 | `native_balanced.go:20-24`의 기존 규칙과 같다. Codex 배치 해석은 대체를 거부하므로(`codex_resolve.go:13-20`) effort만 바꾸면 생성 거부가 난다 |
| Claude haiku | effort를 쓰지 않고 선택지도 내지 않는다 | 값을 그대로 쓰기 | Haiku 4.5는 effort 표에 없다. 지금 생성기도 쓰지 않는다 |
| Codex 선택지 | run snapshot에서 후보 model이 보고한 단계만, 그룹은 교집합, snapshot이 없으면 묻지 않음 | 고정 목록 + `(미확인)` | 사용자 결정 "Invalid/unsupported combos are not offered" |
| OMP effort | 기존 pin의 thinking. thinking만 바꾸면 plan의 effective selector로 pin | quality effort를 OMP로 번역 | OMP는 quality 프리셋과 별개다(`quality_omp_policy.go:16-17`). attestation 경로를 그대로 탄다 |
| OMP 에이전트 | autopus가 기본: 정규 16개 `.omp/agents/<agent>.md`, model 없음, `thinkingLevel` = 선언 effort | frontmatter에 concrete model, OMP role alias(`@slow`) | 사용자 결정(model은 attestation 우회). alias는 은퇴한 SPEC-OMP-005 방식이다 |
| reviewer | OMP `/review`가 읽는 `output`(P0 모양)을 Autopus reviewer에 넣는다 | schema 없이 대체 | 없으면 `/review` verdict가 사라진다. pipeline은 caller schema라 영향 없다 |
| native 끄기 | project `.omp/config.yml`의 `task.disabledAgents` claim(기존 merge·ledger), 프로필과 무관. 사용자 소유면 쓰지 않고 degraded | overlay에만 쓰기, 파일 통째 쓰기 | plain 세션은 overlay를 읽지 않는다. 지문 불일치 거부는 기존 claim 규칙이다 |
| route 단위 | 정규 16개 + task, 옛 native key pin은 접히던 에이전트로 펼친다 | native pin 거부, autopus.yaml 재작성 | MODELUX-002 OMP 탭·줄 편집기·`--agent` pin을 그대로 살린다. autopus.yaml을 쓰지 않는다 |
| 되돌리기 key | `role_model_policy.agent_source: native`(없음·`autopus` = 기본) | `future_extension` 아래, `features.omp`, CLI flag | OMP route key의 owner가 `role_model_policy`다. typed 필드라 OMP writer가 그대로 왕복한다. 정책 존재 판정(L22-25)에 넣지 않는다 |
| 사용자 agent 파일 | manifest에 없는 같은 이름 파일은 그대로 두고 그 에이전트만 건너뜀. manifest 파일을 사용자가 고쳤으면 `.autopus/backup/<ts>/`에 복사 후 덮음 | 무조건 덮기 | 지금 OMP update는 백업 없이 덮는다(`omp_rooted_lifecycle.go:136-175`) |
| 프로필 없는 프로젝트 | 지금처럼 route 없음(부모 model 상속) + frontmatter thinking | quality 등급으로 route 자동 생성 | 매 생성마다 catalog 증명이 필요하고 "plain install은 base config가 없다"(`omp.go:208`)를 바꾼다. 사용자 확인 질문으로 남김 |
## Superseded Decisions
- 61f72bb0(SPEC-ADK-MINIMALITY-DISCIPLINE-001) "OMP는 native agent를 재사용하고 16개 생성을 멈춘다"를 이 SPEC이 기본값에서 뒤집는다. native는 한 릴리스 동안 opt-in이다.
- SPEC-MODELUX-002 REQ-MUX2-10 "OMP 지정 단위는 native 5개"와 non-goal "새 YAML key"는 autopus 모드에서 바뀐다(단위 16개, key 둘). native 모드에서는 그대로다.
## Plan Intent Ledger
Clarification Ledger unavailable. 입력은 planning context의 사용자 결정 표(answered)다. 다시 묻지 않았다.

| Field | Status | SPEC 반영 |
|---|---|---|
| effort storage (`efforts` key, 자동 기본, 옛 바이너리 안내) | answered | REQ-MUX3-01, 02, 04 |
| effort UI (도구 탭별, 그룹·에이전트, 유효 값만) | answered | REQ-MUX3-03, 13 |
| OMP agents (16개 생성, override key, model 없음, thinking) | answered | REQ-MUX3-07, 08, 11, 12 |
| native built-ins (disabledAgents, reviewer 대체, task 유지) | answered | REQ-MUX3-09, 10, P4 |
| rollback (`agent_source: native` 한 릴리스) | answered | REQ-MUX3-06, 15 |
| release (v0.50.125, PR #196, 새 secret 없음) | answered | header |
## Question Audit
- question_transport: none · question_count: 0 · unresolved_fields: none. "프로필 없는 프로젝트의 자동 route"(Q1)는 2026-10-13 사용자가 세션 모델 상속으로 정했다.
## Outcome Lock
- User-visible outcome: 세 진입점의 Claude Code·Codex 탭에서 등급과 함께 effort를 그 도구의 유효 값으로 고르고, 같은 run의 생성 파일이 미리보기와 같다. OMP 탭은 thinking을 고른다. OMP 프로젝트는 기본으로 Autopus 에이전트 16개를 쓴다: `.omp/agents`가 생기고 scout·security-reviewer·sonic은 꺼지며 reviewer는 Autopus reviewer다. 프로필이 있으면 route가 16개 이름(+task)으로 활성화·되읽기된다. `auto update`가 기존 프로젝트를 옮기고 `agent_source: native`가 되돌린다.
- Mandatory requirements: REQ-MUX3-01 ~ REQ-MUX3-17 (Must). REQ-MUX3-18, 19는 Should.
- Explicit non-goals: `spec.md` `## Outcome Boundary`와 같다.
- Completion evidence: Must S1–S17 PASS, `auto eval harness run`, 신규·변경 package coverage 85% 이상, 신규·변경 소스 300줄 이하, `GOOS=windows GOARCH=amd64 go build ./...`, `evidence/manual-smoke-003.txt`(S17), `auto spec validate --strict`.
## Visual Planning Brief
흐름도와 wireframe은 `plan.md`에 있고 `wireframe intent: assumed`다. 요구사항으로 묶는 것은 Effort Option Contract와 Form Flow의 순서·기본값뿐이다.
## Technology Stack Decision
| Mode | Selected stack | Resolved versions | Source refs | Checked at | Rejected alternatives |
|---|---|---|---|---|---|
| brownfield | 기존 Go 모듈, huh 폼, 기존 OMP adapter·claim·activation | huh v1.0.0, bubbletea v1.3.6, omp 18.6.1(외부 CLI). 새 의존성 없음 | `go.mod`, `omp --version`, Claude Code 문서 | 2026-10-12 | OMP plugin 배포, role alias |
## Minimality Decision Matrix
| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | 사용자 피드백(effort 편집, OMP를 Autopus 에이전트로) | proceed | effort key·폼, OMP 16개 생성, disabledAgents, route key, migration, rollback |
| existing code/helper/pattern | 61f72bb0^ 생성기, `MergeOMPProjectManagedConfig`·ownership ledger, activation·RPC 되읽기, MODELUX-002 폼·planner·writer, `ResolveCodexProfile` | reuse | 판단·쓰기·활성화는 기존 함수다 |
| stdlib/native | `maps`, `slices`, `os` | use | 교집합, 사본 |
| existing dependency | yaml.v3, huh | reuse | frontmatter, 폼 |
| new dependency or new abstraction | 새 의존성 0. 새 abstraction: effort 해석·선택지, agent source 모드, agent 파일 writer, disabledAgents claim, autopus route 규칙 | accepted | 각각 REQ-MUX3-01–03, 06, 07, 10, 11의 실제 요구다 |
| minimum sufficient verification | 실제 adapter `Generate`, 가짜 OMP runner, catalog fixture, 수동 smoke 1회 | required checks | security·data-loss(사용자 파일, claim 복원)·deterministic-oracle gate를 줄이지 않는다 |
## Semantic Invariant Inventory
| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-MUX3-01 | "Absent → today's automatic effort" | `efforts`가 없는 파일의 생성 결과는 HEAD 93dbb564와 bytes가 같다 | Claude·Codex 생성 파일 | S1, S2 |
| INV-MUX3-02 | "values valid for that tool … not offered" | 선택지 = 도구·model이 지원하는 단계(그룹은 교집합) | 선택지 목록 | S3, S4, S10 |
| INV-MUX3-03 | "Labels show real values" | 라벨·미리보기·생성의 effort가 같은 snapshot의 같은 값 | 라벨, 표, frontmatter, TOML | S3, S4 |
| INV-MUX3-04 | "do NOT put concrete models into frontmatter" | `.omp/agents/*.md`에 `model` key가 없고 route는 override로만 | agent 파일, override | S7, S9 |
| INV-MUX3-05 | "keyed by the 16 Autopus names" | override key 집합 = 정규 16개 + task, 옛 pin은 접히던 에이전트로 펼쳐짐 | `task.agentModelOverrides` | S9 |
| INV-MUX3-06 | "keep user-authored .omp/agents files" | manifest 밖 사용자 파일·사용자 소유 disabledAgents는 bytes 불변 | 파일 SHA | S8 |
| INV-MUX3-07 | "agent_source: native restores" | native 왕복 뒤 OMP 표면이 HEAD 93dbb564 생성과 같다 | 파일 목록, `.omp/config.yml` | S13 |
## Feature Coverage Map
| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| effort 저장·해석·생성 | REQ-MUX3-01, 02, 05 / S1, S2 | covered |
| effort 폼(Claude·Codex·OMP) | REQ-MUX3-03, 04, 13 / S3–S5, S10, S11 | covered |
| OMP 16개 생성, reviewer, 사용자 파일 | REQ-MUX3-07–09 / S7, S8 | covered |
| disabledAgents, route key, 소비처 | REQ-MUX3-10–12 / S7, S9, S14, S15 | covered |
| migration, rollback, 안전 | REQ-MUX3-06, 14–17 / S6, S12, S13, S16, S17 | covered |
## Completion Debt
| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

P1·P2·P3의 dispatch 적용은 live 미확인이며 S17 수동 smoke가 닫는다. smoke가 빠지면 debt가 되어 sync 완료를 막는다.
## Evolution Ideas
These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| 다음 릴리스에 `agent_source: native`와 native 경로 제거 | 한 릴리스 유예가 사용자 결정이다 | v0.50.126 계획 |
| RPC 되읽기를 selector별 1회로 묶기 | 17회 실행 시간이 실측 전이다 | 활성화 지연 보고 |
| 프로필 없는 프로젝트의 등급 기반 OMP route | 사용자가 세션 모델 상속을 골랐다(Q1 해결) | 사용자 결정 |
## Sibling SPEC Decision
| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | effort와 OMP 에이전트는 같은 구성 단계·같은 PR #196이다. task 13개, 소스 파일 약 38개(test 포함)로 기준(25개 초과 task이면서 40개 초과 파일)에 못 미친다 | None |
## Reference Discipline
| Reference | Type | Verification |
|-----------|------|--------------|
| 위 `pkg/config`, `pkg/adapter/{omp,codex,claude}`, `pkg/content`, `internal/cli` 경로 | existing | Read, grep, `wc -l`(2026-10-12) |
| `61f72bb0^` 생성기, omp 18.6.1 덤프, Claude Code 문서 | existing (history, binary, docs) | `git show`, `strings`, WebFetch |
| `QualityPreset.Efforts`, `AgentEffort`, `RoleModelPolicyConf.AgentSource`, `TransformAgentForOMP`(복원), `NormalizeOMPSemanticReferencesFor`, `quality_effort*.go`, `omp_agent_source.go`, `omp_agents.go`, `omp_agent_surface_claim.go`, `role_model_policy_autopus_agents.go`, `quality_compose_form_effort.go`, `quality_compose_omp_autopus.go`, `evidence/manual-smoke-003.txt` | [NEW] planned addition | 부재 확인(grep, 2026-10-12) |
## Reviewer Brief
- Intended scope: effort key·해석·폼, OMP 기본 agent source 전환(16개 생성, disabledAgents, route key, 소비처), migration과 rollback.
- Explicit non-goals: 프로필 없는 자동 route, native 경로 제거, OMP writer 재encode 변경, OpenCode·Antigravity effort, 줄 driver 변경.
- Self-verified: Traceability, invariant 7개, P0·P3 실행, 정적 확인, 사용자 파일 덮어쓰기 위험 위치.
- Reviewer should focus on: 사용자 `.omp/agents`·`.omp/config.yml` 보존, claim 복원, 옛 native pin 펼침의 정확성, override 17개 되읽기, `/review` 호환, effort와 표준 배치 규칙, strict loader 안내, 그리고 Completion Debt only.
## Open Risks
- R1: P1·P2·P3 dispatch 적용은 live 미확인(읽기 전용 목록 없음). S17이 OMP TUI `/agents`(메시지 없이)로 확인한다. R2: override `:thinking`과 frontmatter `thinkingLevel`의 우선순위 미확인. R3: RPC 되읽기 5회 → 17회.
- R4: 기존 test 변경을 피할 수 없다(기본값이 바뀐다). 규칙은 `spec.md` `## Existing Test Changes`. R5: user-level `disabledAgents`와 project 값의 병합 방식 미확인. R6: OMP가 claude-plugins agent도 발견한다(S:524490). Autopus `.claude/agents`가 그 경로에 드는지는 미확인.
## Self-Verify Summary
- Q-CORR-01 | status: PASS | attempt: 1 | files: research.md, spec.md | reason: 코드 사실은 file:line, OMP 사실은 S:줄과 probe 기록이다. 확인하지 못한 것은 미확인이다
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md, research.md | reason: 새 함수·필드·파일·evidence는 [NEW]다
- Q-COMP-01 | status: PASS | attempt: 1 | files: spec.md, plan.md, acceptance.md, research.md, prd.md | reason: 계약, 계획, oracle, 근거, 요약이 나뉜다
- Q-COMP-02 | status: PASS | attempt: 1 | files: acceptance.md | reason: Must 시나리오는 실제 명령·adapter·가짜 runner와 수동 smoke로 닫는다
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md | reason: Traceability Matrix가 REQ 19개를 task, 시나리오, invariant에 잇는다
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md | reason: A1은 P3 증거가 있는 PASS, A2·A3는 not-run 이유가 있다
- Q-SEC-01 | status: PASS | attempt: 1 | files: spec.md | reason: 사용자 파일·사용자 소유 설정을 쓰지 않는 규칙과 terminalSafe를 Trust Model에 적었다
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 구성 단계와 OMP 표면 하나로 모이고 sibling이 없다
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: EARS 문장과 bare Given/When/Then/And 형식이다
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline이 existing, history·binary·docs, [NEW]를 나눈다
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 결정 표 6행을 REQ와 Plan Intent Ledger로 옮겼고 결정 밖 해석(Q1)은 Open Issues로 뺐다
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, acceptance.md | reason: INV-MUX3-01~07이 S1–S13의 bytes·key 집합·선택지 목록 oracle에 이어진다
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt는 None이고, S17 수동 smoke가 빠지면 debt가 된다고 적었다
- Q-FEAS-01 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: OMP 쓰기를 adapter 한 transaction으로 모으고 Disabled Agents·Migration Contract가 경우별 쓰기와 복원을 정한다. 300줄 근처 파일을 표시했다
- Q-SEC-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: 사용자 `.omp/agents`·사용자 소유 `task.disabledAgents`는 쓰지 않고 S8이 SHA로 판정한다
- Q-COH-02 | status: PASS | attempt: 1 | files: research.md | reason: Outcome Lock 작업이 모두 Primary SPEC에 있고 P1·P2 live 확인은 S17로 닫는다

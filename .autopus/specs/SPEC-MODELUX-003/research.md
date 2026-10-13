# SPEC-MODELUX-003 리서치

branch `feat/modelux-003`, 기준 `v0.50.125`(dd6d6ea2, origin/main, SPEC-MODELUX-001·002 포함)에서 읽었다. `git diff --stat 93dbb564 dd6d6ea2 -- '*.go'`가 비어 있어 초안의 줄 번호가 main에서도 같다. OMP 사실은 omp 18.6.1 실행 파일의 `strings -n 8` 덤프(`S:<줄>`)와 scratch probe다(`evidence/probe-omp-18.6.1.md`). Revision 1은 review round 1(F-001–F-020)을, revision 2–4는 round 2–4의 미해결 건(F-002, F-008, F-013, F-021, F-022, F-023)을 반영했다(`## Review Round 1`–`## Review Round 4`). 확인하지 못한 것은 `미확인`이다.
## 기존 코드 분석
### effort (Claude·Codex)
- `pkg/config/schema.go:24-27` `QualityPreset{Description, Agents}`(290줄). strict decode `loader_strict.go:26-50`은 v0.50.113부터라(SPEC-MODELUX-002 research "도구별 저장 값") 새 key를 쓴 파일은 v0.50.113–v0.50.125가 읽지 못한다. 그 이전 바이너리의 동작은 미확인.
- 검증 `schema_model_selection.go:7-35`, 등급 `quality_tier.go:76-91`, Claude ID `:7-12`(haiku = `claude-haiku-4-5`), `quality_effective.go:28-34`·`:48-65`, Claude 쓰기 `pkg/adapter/claude/claude_agent_content.go:67-112`, 표준 배치 `native_balanced.go:9-31`(명시 등급은 L20-24에서 custom으로). Codex: `codex_profile.go:46`, `:66-75`, `:105-124`, `:189-195`, `codex_profile_resolve.go:30-33`·`:52-111`, `pkg/adapter/codex/codex_resolve.go:13-31`.
- 프리셋을 다시 만드는 자리(F-013): `internal/cli/quality_compose_bind.go:173`(편집 프리셋), `quality_compose_bind_write.go:29-34`(`composeBindingPlan.set`)·`:47-52`(`composeCandidate`), `quality_tool_interactive.go:207-222`(`withQualityToolPreset`, 미리보기), `quality_tool_persist.go:49-61`(`setQualityToolPreset`). 모두 `{Description, Agents}`만 쓴다. 분류 `pkg/config/quality_preset_history.go:116` `ClassifyBuiltinQualityPreset`은 Agents만 비교한다.
- 폼: 등급 Select뿐이다(`quality_compose_form_edit.go:12-57`). 300줄 근처: `quality_compose_form_driver.go` 290, `quality_compose_omp.go` 292, `schema.go` 290.
- Claude Code 문서(2026-10-12): `effort`는 `low`·`medium`·`high`·`xhigh`·`max`, model마다 다르고 없으면 세션 effort. Fable 5.1·Opus 5.5·Sonnet 5.5는 다섯 단계, 지원하지 않는 단계는 그 아래로 돈다. Haiku 4.5는 표에 없다.
### T1 분류 (effort 폼, T2·T9·T10)
측정: effort Select에 임시 계측(`[DEBUG-e7c1]`, 커밋 전 제거)을 넣고 폼을 쓰는 기존 test 19개 파일의 test마다 따로 돌려 effort Select가 열린 수를 셌다(2026-10-13, 5f51ba23 기준 test). 15개 test(7개 파일)가 Claude Code·Codex 탭을 지나 effort Select를 연다. T2와 T10은 기존 test를 바꾸지 않는다(efforts 없는 fixture의 동작이 같다). 분류는 셋이다. **Enter**: 새 effort Select마다 입력에 Enter 하나(규칙 3). **abort 위치**: n번째 폼에서 Ctrl+C를 주는 test가 effort Select 4개만큼 위치를 옮겨 전과 같은 폼을 가리킨다(규칙 3의 같은 조정, 단언 무변경). **무변경**.

| 파일 | 분류 | 이유 |
|---|---|---|
| `internal/cli/quality_compose_form_review_test.go` | Enter | 공유 입력 `composeFormS5`에 4개(이 변수를 쓰는 5개 파일 11개 test가 함께 맞는다). `UnchangedEditWritesNothing` 4개, `CancelAfterAWriteKeepsThatWrite`의 Claude 탭 4개 |
| `internal/cli/quality_compose_form_edit_test.go` | Enter | `ClaudeAgentEditBindsClaudeCustom`의 executor 값 뒤 1개 |
| `internal/cli/quality_compose_form_entry_test.go` | Enter, abort 위치 | `ClaudeAgentApplyGeneratesThePreview` 1개. `InterruptEndsWithoutError`는 확인 Select가 8번째에서 12번째가 되어 8→12 |
| `internal/cli/quality_compose_form_driver_test.go` | Enter | `AppliedEditRedrawsTheTableAndAsksAgain`의 Codex 탭 4개 |
| `internal/cli/update_compose_form_test.go` | Enter, abort 위치 | `KeepPathsWriteNothing`의 "Codex groups unchanged" 4개. `InterruptAfterAWriteKeepsTheWrite`는 쓰기 뒤 메뉴가 9번째에서 13번째가 되어 9→13 |
| `internal/cli/init_compose_form_test.go`, `quality_compose_form_locale_test.go` | 무변경 | `composeFormS5`로 함께 맞는다 |
| `internal/cli/quality_compose_form_paths_test.go` | 무변경 | 도구 탭 경우의 abort가 첫 effort Select 전(4·5번째)이다 |
| `internal/cli/quality_compose_line_golden_test.go`, `quality_compose_driver*_test.go` | 무변경 | 줄 driver에는 effort 질문이 없다(규칙 4). golden bytes 동일 |
| `internal/cli/quality_compose_form_labels_test.go`, `quality_compose_tool*_test.go`, `quality_compose_form_refusal_test.go` | 무변경 | labeler·planner를 직접 부르고 폼 Select가 없다. 함수 signature를 유지했다 |
| `internal/cli/quality_tool_interactive_test.go`, `quality_compose_bind*_test.go`, `quality_config_block*_test.go` | 무변경 | efforts 없는 fixture라 쓰기 결과가 같다 |
### OMP native 구조
- 접힘 `pkg/config/omp_native_agents.go:6-63`, route key `role_model_policy_native_agents.go:13`·`:77-101`·`:118-162`(대표가 pin되지 않은 채 접히는 pin이 다르면 `omp_native_agent_conflict`, F-001), load 검증 `role_model_policy_validate.go:52-54`와 `:72-80`(모드와 무관하게 접힘 검증), managed key allowlist `:174-190`(`task.disabledAgents` 없음). built-in 프로필은 16개 에이전트마다 route를 만든다(`role_model_policy_builtin.go:3-11`, `:42-47`, `:214-240`).
- 접힘 소비처(production, grep 16개 파일): `pkg/config` 두 파일, `pkg/content/skill_transformer_replace_omp_render.go:103-124`, `pkg/adapter/omp/`의 `omp_model_integration_bridge.go:31-113`, `omp_model_projection.go:59-132`(L69, L111), `omp_model_routing_compile.go:33,105-112`, `omp_model_projection_overlay.go:19`(대표를 모르는 이름은 `unknown native OMP agent`), `omp_model_integration_degraded.go:21-37`(`OMPNativeAgentOperatorPinned(native)`), `internal/cli/`의 `platform_omp_agent_catalog.go:42-79`, `platform_omp_profile_preview.go:90-94`, `pipeline_backend_omp_models.go:75-110`, `pipeline_backend_orca_models.go`, `doctor_omp_model_routing_probe.go`, `quality_compose_table.go`, `quality_omp_custom.go:57-58`, `quality_compose_omp.go:169,193`. `omp_model_doctor_projection.go`, `omp_model_integration_activation.go:115,192-210`은 override key를 그대로 쓴다.
- 생성: `omp.go:69-124` Generate는 다른 mapping을 모두 쓰고 manifest는 은퇴 경로 제거에만 쓴다(L106-114). Update는 `omp_rooted_lifecycle.go:98-134`가 action을 정하고 `:136-175`가 Skip만 거른다. 두 경로 모두 `applyOMPTransactionAt`을 거치고, transaction은 덮기 전 기존 bytes를 `.autopus/backup/<id>/transaction/omp/<path>`(0600)에 남긴다(`omp_rooted_transaction.go:114`, `pkg/adapter/transaction.go:114,262`). 사용자 파일 문제는 백업 부재가 아니라 manifest 밖 파일을 덮는 것이다(F-019).
- project config ledger(F-002): `.autopus/omp-model-project-ownership-v1.json`은 파일 하나의 원본 preimage와 claim을 기록한다. `mergeOMPIntegratedProjectConfig`(`omp_model_integration_project.go:14-99`)는 project-managed 프로필의 finalize에서만 불리고 `ManagedKeys` 수가 값 수와 같아야 하며(L19-21) ledger가 있으면 preimage에서 다시 만든다(L40). 현재 파일 검증은 파일 전체 commitment다(`omp_model_integration_ownership_rooted.go:49`). release는 파일 집합에 `.omp/config.yml`이 없으면 preimage를 복원한다(`omp_rooted_lifecycle_config.go:64-83`). clean은 project-managed receipt가 없으면 거부한다(`omp_model_integration_clean.go:14-32`). config mapping은 model integration이 있고 project-managed일 때만 만든다(`omp.go:208-216`).
- 렌더 경로(F-015): `ReplacePlatformReferences`(`pkg/content/skill_transformer_replace.go:89`, L96에서 `NormalizeOMPSemanticReferences`)를 `NormalizeAgentReferences`(L127-131), `skill_resources.go:33-66`(cfg 있음), `generate.go:105`, `skill_transformer.go:121-149`(options 있음), `agent_transformer_mapping.go:75`, `pkg/adapter/omp/omp_rules.go:49`, `omp_workflow_render.go:56`가 부르고 `omp_skills.go:136`은 직접 부른다. native 전용 문장: `templates/shared/omp-agent-pipeline.md.tmpl:56-65`("OMP registers five agents … There is no Autopus-specific agent to select").
- OMP 탭(F-008, F-016): pin을 다 모은 뒤 plan한다(`quality_compose_omp.go:155-199`). metadata 없는 catalog는 `installedOMPCustomModelChoices`가 안내와 함께 표시용 catalog로 대체하고 `requireOMPCustomCapability`는 capability 목록이 없는 model을 받는다(`quality_omp_custom.go:102-159`).
- 되읽기(F-004): runner가 `RunWithInput`을 가질 때만 RPC 경로다(`omp_model_integration_activation.go:192-210`). `ompModelDoctorE2ERunner`는 `Run`뿐이고(`internal/cli/doctor_omp_model_routing_e2e_test.go:110-146`) `RunWithInput` fake는 `pkg/adapter/omp/omp_model_activation_rpc_test.go`에 있다.
- EARS parser(F-020): `pkg/spec/parser.go:16` `reStateDriven = (?i)WHERE\s+.+\s+THEN\s+.+`.
### 기존 test (F-007, T1 grep 실행 결과: 27개 파일, 49건)
- `internal/cli/`: doctor_omp_model_routing_e2e, omp_dispatch, omp_platform_boundary, platform_omp_agent_catalog, platform_omp_native_agents, platform_omp_profile_preview, quality_compose_table, update_gitignore. `pkg/adapter/`: latest_cli_omp_contract, parity. `pkg/adapter/omp/`: omp_acceptance_surface, omp_latest_contract, omp_model_activation_rpc, omp_model_attested, omp_model_doctor_coverage, omp_model_integration_builtin, omp_model_integration_diversity, omp_model_integration, omp_model_projection, omp_native_agent_prompt, omp_native_registry, omp_native_task_hub_contract, omp_rules_live. `pkg/config/`: omp_native_agents, role_model_policy_native_agents. `pkg/content/`: omp_normalization, skill_transformer_replace_omp(모두 `_test.go`).
- 기본값 전환 단언(fixture로 못 지킨다): `latest_cli_omp_contract_test.go:24,29`(`.omp/agents` 없음, `.omp/config.yml` manifest 아님), `omp_acceptance_surface_test.go:52,151`, `omp_latest_contract_test.go:30`, `omp_native_registry_test.go:27`. 나머지 23개는 T1이 파일마다 native fixture·native option·무변경으로 나눈다.
### T1 분류 (OMP agent source)
grep: `rg -l 'OMPNativeAgentNames|OMPNativeAgentForRole|OMPNativeAgentRepresentative|ompRetiredAgentDir|\.omp/agents|native_agent_shadowed|There is no Autopus' -g '*_test.go' internal pkg`(27개, 53줄; 2026-10-13 5f51ba23). 분류는 넷이다. **전환**: 그 단언만 autopus 기대값으로 바꾸고 같은 파일에 `agent_source: native` 사본으로 옛 단언을 지킨다(규칙 1). **fixture**: fixture에 `agent_source: native`(Go fixture는 `RoleModelPolicy.AgentSource`) 한 줄, 단언 무변경(규칙 2). **option**: 렌더·compile 함수에 native option, 단언 무변경(규칙 2). **무변경**.

| 파일 | 분류 | 이유 |
|---|---|---|
| `internal/cli/doctor_omp_model_routing_e2e_test.go` | fixture | `report.Roles`가 native 5개다(L85) |
| `internal/cli/omp_dispatch_test.go` | 전환 | L40 preview의 `.omp/agents` 0개가 autopus에서 16개다. grep 목록의 4개 밖에서 T1이 찾은 전환 단언이다 |
| `internal/cli/omp_platform_boundary_test.go` | 무변경 | gitignore 디렉터리 형식 단언(`.omp/agents/`는 이미 있다) |
| `internal/cli/platform_omp_agent_catalog_test.go` | fixture | catalog 5행·`native_agent_shadowed`는 native 계약이다 |
| `internal/cli/platform_omp_native_agents_test.go` | fixture | plan이 bundled 5개만 이름 짓는 native 계약이다 |
| `internal/cli/platform_omp_profile_preview_test.go` | fixture | 미리보기 행이 registry 순서 5개다(L73, L262) |
| `internal/cli/quality_compose_table_test.go` | fixture | OMP 칸 = `OMPNativeAgentRoute`(MODELUX-002 S5) |
| `internal/cli/update_gitignore_test.go` | 무변경 | 패턴 전달만 본다 |
| `pkg/adapter/latest_cli_omp_contract_test.go` | 전환 | L24 `.omp/agents` 없음, L29-30 `.omp/config.yml` 없음 |
| `pkg/adapter/parity_test.go` | 무변경 | 경로 분류표다 |
| `pkg/adapter/omp/omp_acceptance_surface_test.go` | 전환 | L40-42·L52·L56·L151·L155의 agent 0개와 `.omp/config.yml` 없음 |
| `pkg/adapter/omp/omp_latest_contract_test.go` | 전환 | L30-33. 같은 파일의 `PreservesUserBaseConfigWhenNoManagedClaimsExist`는 claim 없는 native 계약이라 fixture |
| `pkg/adapter/omp/omp_model_activation_rpc_test.go` | 무변경 | L168은 RPC 단위 test의 selector 자료다 |
| `pkg/adapter/omp/omp_model_attested_test.go` | fixture | receipt가 native 5행이다(L138) |
| `pkg/adapter/omp/omp_model_doctor_coverage_test.go` | option | doctor activation expectation이 native 5 key다(L47) |
| `pkg/adapter/omp/omp_model_integration_builtin_test.go` | fixture | selector·receipt가 native 5개다 |
| `pkg/adapter/omp/omp_model_integration_diversity_test.go` | fixture | receipt가 native 5행이다 |
| `pkg/adapter/omp/omp_model_integration_test.go` | fixture | 은퇴 dir(L144)과 native 5 route(L163, L231) |
| `pkg/adapter/omp/omp_model_projection_test.go` | 무변경 | native 이름 projection(L226). T6에서 source를 비워 둔 projection은 agent 이름으로 native를 추론하므로 option이 필요 없었다 |
| `pkg/adapter/omp/omp_native_agent_prompt_test.go` | fixture, option | `generatedOMPBodies`는 native 생성, 템플릿 정규화는 native option |
| `pkg/adapter/omp/omp_native_registry_test.go` | 전환 | L27 생성 파일에 `.omp/agents` 없음. 같은 파일의 은퇴·백업 test는 fixture |
| `pkg/adapter/omp/omp_native_task_hub_contract_test.go` | 무변경 | L140은 주석, live test다 |
| `pkg/adapter/omp/omp_rules_live_test.go` | 무변경 | ignore 패턴 자료다 |
| `pkg/config/omp_native_agents_test.go` | 무변경 | native 접힘 함수는 그대로 남는다 |
| `pkg/config/role_model_policy_native_agents_test.go` | fixture | L180·L243의 load 시 `omp_native_agent_conflict`는 native 모드만 낸다 |
| `pkg/content/omp_normalization_test.go` | 무변경 | 경로 정규화는 mode와 무관하다 |
| `pkg/content/skill_transformer_replace_omp_test.go` | option | L242-267 task batch의 native 접힘(`"agent": "executor"` 없음) |

- grep 밖에서 구현 중 깨지는 test는 같은 네 분류로 이 표에 더하고, 더한 task의 커밋 본문에 이유를 적는다.

| 더한 파일 (task) | 분류 | 이유 |
|---|---|---|
| `internal/cli/doctor_omp_cli_test.go` (T4) | 전환 | L40 생성 뒤 `.omp/agents` 없음. native 사본 `TestOMP002_S10_DoctorFixtureNativeSourceWritesNoAgents` |
| `pkg/adapter/omp/omp_test.go` (T4) | 전환 | L57 `.omp/agents` 없음. native 사본 `TestOMP_S4_S5_S6_Lifecycle_NativeSource` |
| `pkg/adapter/omp/omp_validate_model_integration_test.go` (T4) | 전환 | L23 routing 뒤 `.omp/agents` 없음. native 사본 `TestOMPValidate_NativeSourceModelIntegrationGeneratedSurfaceIsClean` |
| 위 전환 파일의 `.omp/config.yml` 단언 (T5) | 전환 | `omp_acceptance_surface` L41-42·L56·L172, `omp_latest_contract` L32-33, `omp_test` L56, `latest_cli_omp_contract` L29-30. autopus 기본은 `task.disabledAgents`를 claim하므로 config가 있고 manifest가 기록한다. 옛 단언은 T4가 더한 native 사본이 지킨다 |
| `pkg/adapter/omp/omp_security_config_test.go` (T5) | fixture | `PreservesOrdinaryUserConfig`·`ReplacesStaleManagedSection`은 claim 없는 plain install 계약이다(`nativeConfigForOMP`) |
| `pkg/adapter/omp/omp_security_test.go` (T5) | fixture | `TestOMPClean_PreservesUserConfigKeys`의 "plain OMP generation must not own user config" |
| `pkg/adapter/omp/omp_workflow_parity_test.go` (T5) | fixture | `TestOMP002_S5_PlainConfigRemainsByteIdentical` |
| `pkg/adapter/omp/omp_phase4_findings_test.go` (T5) | fixture | `TestOMPConfigMarkerSpan_PlainLifecyclePreservesUserConfig` |
| `pkg/adapter/omp/omp_context_bridge_test.go` (T5) | fixture | `NoOptInPreservesExactPreparedFiles`: autopus 생성은 ledger salt가 run마다 달라 두 prepare 결과를 bytes로 비교할 수 없다 |
| `pkg/adapter/omp/omp_acceptance_ownership_test.go` (T5) | fixture | legacy marker config migration 계약 |
| `pkg/adapter/omp/omp_model_integration_lifecycle_test.go` (T5) | fixture | project-managed → overlay에서 config가 사라지는 v0.50.125 release 계약. autopus는 S22(c)가 `task.disabledAgents`만 남는 것을 판정한다 |
| `pkg/adapter/omp/omp_latest_contract_test.go` (T5) | fixture | `AllowsMissingBaseConfigButRejectsWeakSensitivePermissions`는 base config가 없는 plain install 계약이다 |
| `internal/cli/pipeline_backend_omp_security_test.go`, `pipeline_backend_orca_models_test.go` (T7) | fixture | native 접힘 receipt 자료다. phase agent는 이제 project config의 source로 정하므로 receipt fixture에 native config 한 줄을 더했다 |
| `internal/cli/platform_omp_profile_native_test.go`, `platform_omp_status_test.go` (T7) | fixture | bundled registry 행·접힌 task 행의 native 계약이다 |
| `internal/cli/quality_compose_table_fixture_test.go` (T7) | fixture | MODELUX-002의 O1·O2는 v0.50.125 OMP fixture라 native 한 줄을 더했다. 표 golden(`quality_compose_table_golden_test.go`)·표 test는 무변경으로 통과한다 |
| `internal/cli/quality_compose_omp_test.go`, `quality_compose_form_paths_test.go`(composeO3 사용), `quality_compose_form_entry_test.go`, `update_compose_form_test.go`, `update_compose_omp_test.go` (T7) | fixture | MODELUX-002 OMP 탭은 bundled 5개 탭이다. autopus 탭은 T11의 새 test가 판정한다 |
| `internal/cli/quality_omp_custom_test.go` (T7) | fixture | 줄 OMP 편집기 계약(SPEC: 그대로). autopus에서 같은 catalog는 task 접힘 없이 역할마다 route가 필요하다 |
| `internal/cli/init_compose_omp_test.go` (T7) | 전환(T11) | init은 config를 새로 만들어 native fixture를 줄 수 없다. S21 init 두 test는 T11에서 autopus 탭 입력(에이전트별 → planner)과 기대값(`agents.planner`, 되읽기 `planner`·`task`)으로 바꿨고, native 사본은 둘 수 없다 |
| `pkg/content/skill_transformer_replace_omp_collapse_test.go` (T8) | option | 은퇴한 역할 이름이 bundled 5개로 접히는 native 렌더 계약이다. `ReplacePlatformReferencesWith`에 native option을 주고 단언은 그대로다. autopus 렌더는 `skill_transformer_replace_omp_mode_test.go`가 판정한다 |
| `pkg/adapter/omp/omp_workflow_parity_test.go` (리뷰 LOW) | option | `TestOMP002_S7_ValidateReportsMissingAndTamperedManagedSets`가 `prepareRuleMappings(cfg)`에 그 test의 cfg를 넘긴다. 단언은 그대로다 |
## OMP probe 결과 (`evidence/probe-omp-18.6.1.md`)
| ID | 질문 | 결과 |
|---|---|---|
| P0 | bundled 정의 | PASS. 5개. reviewer는 `@slow`, verdict `output`(overall_correctness·explanation·confidence·findings). 이 `output`이 REQ-MUX3-08 golden이다 |
| P1 | project 파일 발견, reviewer 대체 | 정적 S:524490(project → … → bundled, 이름 선착), S:359871(name·description 필수). live는 S17(메시지 없이 `/agents`). 과거 근거: 61f72bb0 "실제 OMP 세션 발견 20개" |
| P2 | disabledAgents 숨김 | 설정 되읽기 PASS(P3b), 정적 S:564994·S:526773. live는 S17 |
| P3 | 사용자 이름 override key | PASS(설정 되읽기). dispatch 적용은 정적 S:526773·S:756787, live는 S23(동의 후 모델 호출) |
| P4 | task를 꺼도 되나 | 안 된다. 이름 없는 dispatch가 `defaultAgent`(task)로 가고 꺼진 agent는 preflight 오류다(S:526773) |
- OMP `/review`는 `reviewer`에 task 하나를 보내고 verdict 필드를 읽는다(S:780806-780816, S:338744-339163). schema는 caller > agent > session(S:526717)이고 Autopus dispatch는 호출마다 `outputSchema`를 준다(`skill_transformer_replace_omp_render.go:90-91`). plain `omp`는 project `.omp/config.yml`을 읽는다(S:466140-466160).
- override `:<thinking>`과 frontmatter `thinkingLevel`의 우선순위는 미확인이다. 그래서 route가 있는 에이전트의 frontmatter에는 `thinkingLevel`을 쓰지 않아 질문 자체를 없앤다(REQ-MUX3-07).
## 설계 결정
| 결정 | 선택 | 기각한 대안 | 근거와 trade-off |
|---|---|---|---|
| effort 저장 | `quality.presets.<name>.efforts.<agent>`, 값 다섯 단계, load 검증 | 에이전트별 ID, `quality.providers.omp` | 사용자 결정. ≤ v0.50.125는 못 읽고 처음 쓸 때 한 줄 안내 |
| effort 보존 | 프리셋을 복사·재작성·미리보기·갱신하는 모든 자리가 efforts를 옮긴다. 갱신은 등급·description만 바꾼다. 모든 도구 같게는 편집 전 binding 프리셋에 efforts가 있으면 다른 built-in 분기를 타지 않고 `custom`에 묶어 efforts를 모든 에이전트에 옮기며, 같은 이름의 Historical 갱신만 built-in에 efforts를 남긴다 | `setQualityToolPreset`만 고치기, 등급이 그대로인 에이전트만 옮기기, 다른 built-in 항목에 efforts 쓰기 | F-013. 한 자리라도 빠지면 미리보기 ≠ 생성. 등급이 바뀐 에이전트의 effort를 버리면 effort를 건드리지 않은 편집이 effort를 바꾼다(REQ-MUX3-20) |
| 명시 effort와 표준 배치 | 명시 effort가 있으면 그 에이전트는 표준 배치를 떠난다 | 배치 model 유지 | `native_balanced.go:20-24`와 같은 규칙. Codex 배치는 대체를 거부한다(`codex_resolve.go:13-20`) |
| haiku | 에이전트마다 판정. 그룹 Select는 haiku가 아닌 멤버로 묻고 모두 haiku면 묻지 않는다 | 그룹 전체 생략 | F-009 |
| 폼 test 입력 | 기존 폼 test는 새 effort Select마다 Enter 한 번을 더하고 단언은 둔다 | 기존 입력 bytes 고정 | F-014. 불변은 줄 driver와 effort 없는 생성 결과로 좁힌다 |
| OMP effort | 기존 pin thinking. 탭은 pin 없이 먼저 plan해 기본값과 selector를 얻는다 | quality effort 번역 | F-008. 탭이 더하는 catalog 읽기는 pin 없는 pre-plan 1회이고 기존 경로의 읽기 횟수는 단언하지 않는다 |
| OMP 에이전트 | 16개 파일, model 없음. `thinkingLevel`은 이번 생성의 override에 없는 에이전트만 선언 effort | concrete model, role alias, 항상 thinkingLevel | 사용자 결정. override와 겹치지 않아 우선순위가 무관해진다(F-018) |
| reviewer | P0 bundled `output`과 같은 golden | schema 없이 대체 | `/review` verdict 보존. 정합성은 golden test가, 런타임은 S23이 닫는다 |
| native 끄기 | Claim Set Contract: ledger 하나, preimage 하나, claim 집합 = 프로필 key ∪ `task.disabledAgents`, merge 한 번. 사용자가 그 key만 고치면 프로필 claim이 있어도 사용자 소유로 넘겨 preimage에 사용자 값을 넣고, 그 표시는 release 뒤에도 표시만 남은 ledger로 남는다. 처음 claim할 때 다른 값이 이미 있으면 처음부터 사용자 소유다 | 별도 merge, overlay에만, key별 commitment로 ledger 형식 변경 | F-002. plain 세션은 overlay를 읽지 않는다. 판정은 기존 whole-file commitment를 그대로 쓰고 그 key만 ADK 값으로 바꿔 비교한다 |
| route | 정규 16개 + task. task는 ① task pin ② planner pin ③ 프로필 planner route. 접힘 검증은 native 모드만 | `OMPNativeAgentRoute("task")` 재사용 | F-001. 에이전트별 탭이 pin을 흔하게 만든다 |
| 렌더 mode | OMP 렌더 진입점이 모두 mode를 받고 기본은 autopus. native는 OMP adapter가 명시 | 새 함수 하나 추가 | F-015. 생성 산출물이 oracle |
| 되돌리기 key | `role_model_policy.agent_source: native` | `future_extension`, CLI flag | route key owner가 `role_model_policy`. 정책 존재 판정(L22-25)에 넣지 않는다 |
| 사용자 agent 파일 | manifest 밖 같은 이름은 그대로 두고 건너뜀. 고친 manifest 파일은 기존 transaction 백업으로 덮음 | 무조건 덮기, 새 백업 경로 | F-019. 백업은 이미 있다 |
| 프로필 없는 프로젝트 | 세션 모델 상속 + frontmatter thinking(Q1, 사용자 결정 2026-10-13) | 등급 기반 자동 route | 자동 route는 매 생성마다 catalog 증명이 필요하다(F-010) |
| 출하 | v0.50.126, branch `feat/modelux-003` | v0.50.125 | v0.50.125는 MODELUX-001·002로 이미 발행됐다(태그 dd6d6ea2) |
## Superseded Decisions
- 61f72bb0(SPEC-ADK-MINIMALITY-DISCIPLINE-001)의 "OMP는 native agent를 재사용"을 기본값에서 뒤집는다. native는 v0.50.126 한 릴리스 동안 opt-in이다.
- SPEC-MODELUX-002 REQ-MUX2-10 "지정 단위 native 5개"와 non-goal "새 YAML key"는 autopus 모드에서 바뀐다. 결정 표의 "v0.50.125 출하"는 그 릴리스가 먼저 나가 v0.50.126이 됐다.
## Review Round 1
| ID | 반영 |
|---|---|
| F-001 | task 규칙, 접힘 검증 native 한정, autopus 검증(REQ-MUX3-11, Route Contract, T3, S21) |
| F-002 | Claim Set Contract, 전환·release·clean·실패 복원(REQ-MUX3-10, 15, T5, S22) |
| F-003 | Agent Set Contract에 production 소비처 전부, degraded 규칙(REQ-MUX3-12, T6, S21) |
| F-004 | S9는 `config get` 한 번의 17 key record, RPC 17회는 `RunWithInput` fake |
| F-005 | S13 기준 run은 v0.50.125 바이너리로 agent_source 줄 없는 파일 |
| F-006 | REQ-MUX3-09에 Generate 쓰기 loop와 Update resolver 두 자리 |
| F-007 | 27개 목록과 기본값 전환 단언 4개 파일(위 `### 기존 test`) |
| F-008 | OMP 탭 pre-plan과 blocker 처리(REQ-MUX3-13, S11) |
| F-009 | 그룹 effort Select는 haiku가 아닌 멤버로 |
| F-010 | 기각 근거를 attestation 비용으로, 주석 2곳 갱신(T4, T5) |
| F-011 | header: branch `feat/modelux-003`, v0.50.126 |
| F-012 | REQ-MUX3-09: 디스크 checksum과 manifest 기록 checksum 비교 |
| F-013 | Efforts Preservation Contract(REQ-MUX3-20, T10, S20) |
| F-014 | 폼 test 입력 규칙, 불변 범위 축소(REQ-MUX3-16, S16) |
| F-015 | OMP Render Mode Contract, 템플릿 문장, 실제 Generate oracle(REQ-MUX3-21, T8, S14) |
| F-016 | metadata 없는 catalog 규칙(REQ-MUX3-13, S10) |
| F-017 | S17을 plain 세션과 `PI_CONFIG_FILES` 세션으로 나눔 |
| F-018 | thinking 질문 제거, reviewer golden, S23과 Completion Debt |
| F-019 | 백업 사실 정정, 기존 transaction 백업 재사용 |
| F-020 | REQ-MUX3-02, 06, 11을 `WHERE … THEN` 형식으로 |
## Review Round 2
| ID | 반영 |
|---|---|
| F-002 | 섞인 상태에서 `task.disabledAgents`만 바뀐 판정, 사용자 소유 전환·표시·재claim(Claim Set Contract, S24) |
| F-008 | 탭이 더하는 catalog 읽기는 pre-plan 1회, 기존 경로의 횟수는 단언하지 않음(REQ-MUX3-13) |
| F-013 | 모든 도구 같게의 `custom`에도 편집 전 binding 프리셋 efforts를 옮김(Efforts Preservation Contract, S20 넷째) |
| F-021 | S22(e)는 내용을 바꿔 실제 transaction을 열고 `ompRootedWriteFailureHook`으로 마지막 write를 실패시킴 |
| F-022 | S21 task pin 사본은 11개 역할 + task = 12개 key |
## Review Round 3
| ID | 반영 |
|---|---|
| F-002 | 사용자 소유 표시는 C가 비어 release해도 표시만 남은 ledger로 남고 `auto clean`만 지운다(Claim Set Contract, S24 첫 사본) |
| F-008 | 설계 결정 행을 pre-plan 1회·횟수 미단언으로 맞춤 |
| F-023 | `composeBindTarget` built-in 분기: P에 efforts가 있으면 다른 built-in 대신 `custom`, 같은 이름 Historical만 갱신(Efforts Preservation Contract, S20 다섯째) |
## Review Round 4
| ID | 반영 |
|---|---|
| F-002 | 첫 claim 판정(이미 있는 다른 값은 처음부터 사용자 소유·표시, 같은 값은 preimage와 같게 claim)과 표시만 남은 ledger의 읽기 전용 판단(Claim Set Contract, S8 UC·UC2) |
## Plan Intent Ledger
Clarification Ledger unavailable. 입력은 planning context의 사용자 결정 표(answered)와 Q1 답(2026-10-13)이다.

| Field | Status | SPEC 반영 |
|---|---|---|
| effort storage · UI | answered | REQ-MUX3-01–05, 13, 20 |
| OMP agents · native built-ins | answered | REQ-MUX3-07–12, 21, P4 |
| rollback · release | answered (release는 v0.50.126로 이동) | REQ-MUX3-06, 15, header |
| 프로필 없는 프로젝트(Q1) | answered: 세션 모델 상속 | non-goal |
## Question Audit
- question_transport: coordinator · question_count: 1 · unresolved_fields: none. Q1은 2026-10-13 사용자가 세션 모델 상속으로 정했다.
## Outcome Lock
- User-visible outcome: Claude Code·Codex 탭에서 등급과 effort를 그 도구의 유효 값으로 고르고 같은 run의 생성 파일이 미리보기와 같다. OMP 탭은 thinking을 고른다. OMP 프로젝트는 기본으로 Autopus 에이전트 16개를 쓰고 scout·security-reviewer·sonic은 꺼지며 reviewer는 Autopus reviewer다. 프로필이 있으면 route가 16개 이름 + task로 활성화·되읽기된다. `auto update`가 옮기고 `agent_source: native`가 되돌린다.
- Mandatory requirements: REQ-MUX3-01–17, 20, 21 (Must). REQ-MUX3-18, 19는 Should.
- Explicit non-goals: `spec.md` `## Outcome Boundary`.
- Completion evidence: Must S1–S17, S20–S24 PASS, `auto eval harness run`, coverage 85% 이상, 300줄, `GOOS=windows GOARCH=amd64 go build ./...`, `evidence/manual-smoke-003.txt`(S17, S23), `auto spec validate --strict`.
## Visual Planning Brief
흐름도와 wireframe은 `plan.md`이고 `wireframe intent: assumed`다. 요구사항으로 묶는 것은 Effort Option Contract와 순서·기본값뿐이다.
## Technology Stack Decision
| Mode | Selected stack | Resolved versions | Source refs | Checked at | Rejected alternatives |
|---|---|---|---|---|---|
| brownfield | 기존 Go 모듈, huh 폼, OMP adapter·claim·activation | huh v1.0.0, omp 18.6.1(외부 CLI), 새 의존성 없음 | `go.mod`, `omp --version`, Claude Code 문서 | 2026-10-13 | OMP plugin 배포, role alias |
## Minimality Decision Matrix
| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | 사용자 피드백, review round 1 | proceed | effort, OMP agent source, claim set, 렌더 mode |
| existing code/helper/pattern | 61f72bb0^ 생성기, `MergeOMPProjectManagedConfig`·ledger·release, transaction 백업, activation, MODELUX-002 폼 | reuse | 판단·쓰기·백업은 기존 함수 |
| stdlib/native | `maps`, `slices` | use | 교집합, 사본 |
| existing dependency | yaml.v3, huh | reuse | frontmatter, 폼 |
| new dependency or new abstraction | 의존성 0. 새 abstraction: effort 해석·선택지, agent source, claim 집합, autopus route, 렌더 option | accepted | REQ-MUX3-01–03, 06, 10, 11, 21 |
| minimum sufficient verification | 실제 adapter `Generate`·`Update`, fake runner, golden, 수동 smoke | required checks | security·data-loss·deterministic-oracle gate 유지 |
## Semantic Invariant Inventory
| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-MUX3-01 | "Absent → today's automatic effort" | `efforts` 없는 파일의 생성 결과 = v0.50.125 | Claude·Codex 생성 파일 | S1, S2, S16 |
| INV-MUX3-02 | "values valid for that tool … not offered" | 선택지 = 지원 단계(그룹 교집합) | 선택지 목록 | S3, S4, S10 |
| INV-MUX3-03 | "Labels show real values" | 라벨·미리보기·생성 effort가 같다 | 라벨, 표, frontmatter, TOML | S3, S4, S20 |
| INV-MUX3-04 | "do NOT put concrete models into frontmatter" | `.omp/agents`에 model 없음, route는 override로만 | agent 파일, override | S7, S9 |
| INV-MUX3-05 | "keyed by the 16 Autopus names" | override key = 정규 16 + task, task는 Route Contract 규칙 | `task.agentModelOverrides` | S9, S21 |
| INV-MUX3-06 | "keep user-authored .omp/agents files" | manifest 밖 파일·사용자 소유 key bytes 불변, release 뒤에도 그 key를 다시 claim하지 않음 | SHA, ledger 표시 | S8, S22, S24 |
| INV-MUX3-07 | "agent_source: native restores" | native 뒤 OMP 표면 = v0.50.125가 agent_source 없는 같은 파일로 만든 것 | 파일 목록, bytes | S13 |
| INV-MUX3-08 | "Absent → automatic" + 보존 | 기존 efforts는 다른 편집·갱신에서 그대로 | quality key | S5, S20 |
| INV-MUX3-09 | "disable via task.disabledAgents" | claim 집합 전환 뒤 파일 = preimage + 남은 claim, C가 비어도 사용자 소유 표시는 남음 | `.omp/config.yml`, ledger | S22, S24 |
## Feature Coverage Map
| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| effort 저장·해석·보존·폼 | REQ-MUX3-01–05, 20 / S1–S5, S20 | covered |
| OMP 파일·reviewer·사용자 파일·렌더 | REQ-MUX3-07–09, 21 / S7, S8, S14 | covered |
| claim·route·소비처·OMP 탭 | REQ-MUX3-10–13 / S9–S11, S15, S21, S22, S24 | covered |
| migration·rollback·안전·smoke | REQ-MUX3-06, 14–17 / S6, S12, S13, S16, S17, S23 | covered |
## Completion Debt
| Item | Blocks | Required resolution |
|------|--------|---------------------|
| CD-1 plain 세션 발견·reviewer 대체·disabled 숨김(live) | REQ-MUX3-07, 10의 런타임 증거 | S17 PASS 기록 |
| CD-2 이름별 override dispatch와 `/review` verdict(live) | REQ-MUX3-08, 11의 런타임 증거 | S23 PASS 기록(모델 호출, 사용자 동의) |

결정적 부분(golden, 설정 되읽기, 생성 파일)은 test가 닫는다. 위 두 행은 기록이 남기 전까지 debt이고 blocker나 생략도 PASS로 닫지 않는다.
## Evolution Ideas
These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| v0.50.127에서 `agent_source: native`와 native 경로 제거 | 한 릴리스 유예가 사용자 결정이다 | 다음 릴리스 계획 |
| RPC 되읽기를 selector별 1회로 | 17회 실행 시간이 실측 전이다 | 활성화 지연 보고 |
## Sibling SPEC Decision
| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | effort와 OMP 에이전트는 같은 구성 흐름이다. task 14개라 기준(25개 초과 task이면서 40개 초과 파일)을 넘지 않는다(파일은 test 포함 약 60개) | None |
## Reference Discipline
| Reference | Type | Verification |
|-----------|------|--------------|
| 위 `pkg/config`, `pkg/content`, `pkg/adapter/{omp,codex,claude}`, `internal/cli`, `templates/shared`, `pkg/spec` 경로 | existing | Read, grep, `rg`(2026-10-13, v0.50.125) |
| `61f72bb0^` 생성기, omp 18.6.1 덤프, Claude Code 문서 | existing (history, binary, docs) | `git show`, `strings`, WebFetch |
| `QualityPreset.Efforts`, `AgentEffort`, `RoleModelPolicyConf.AgentSource`, `ReplacePlatformReferencesWith`, `validateOMPAutopusAgentRoutes`, `quality_effort*.go`, `omp_agent_source.go`, `omp_agents.go`, `omp_agent_surface_claim.go`, `role_model_policy_autopus_agents.go`, `quality_compose_form_effort.go`, `quality_compose_omp_autopus.go`, `testdata/omp-18.6.1-reviewer-output.yaml`, `evidence/manual-smoke-003.txt` | [NEW] planned addition | 부재 확인(grep) |
## Reviewer Brief
- Intended scope: effort key·해석·보존·폼, OMP 기본 agent source(16개 생성, claim 집합, route, 소비처, 렌더 mode), migration과 rollback.
- Explicit non-goals: 프로필 없는 자동 route(Q1), native 경로 제거, OMP writer 재encode 변경, OpenCode·Antigravity effort, 줄 driver 변경.
- Self-verified: 27개 test 목록, 렌더·ledger·소비처 줄 번호, parser regex, transaction 백업 경로, P0·P3.
- Reviewer should focus on: Claim Set 전환과 복원, task 규칙과 load 검증, degraded 판정, 렌더 mode 전파, efforts 보존, 그리고 Completion Debt only.
## Open Risks
- R1: CD-1·CD-2(live 미확인). R3: RPC 되읽기 5회 → 17회(exec runner). R4: 기존 test 변경(27개 중 단언 4개 파일).
- R5: user-level `disabledAgents`와 project 값의 병합 미확인. R6: OMP가 claude-plugins agent도 발견한다(S:524490). R7: ledger에 owner 필드를 더할 때 v0.50.125가 그 ledger를 읽는지 미확인(T5가 먼저 확인, 안 되면 새 ledger 이름). R8: `task.disabledAgents`만 바뀐 판정은 ADK의 삽입 위치와 encoding이 결정적이라는 가정이다(A3).
## Self-Verify Summary
- Q-CORR-01 | status: PASS | attempt: 2 | files: research.md, spec.md, plan.md | reason: 백업 사실(F-019)과 Generate 인용(F-006)을 고쳤고 v0.50.125에서 줄 번호를 다시 확인했다
- Q-CORR-02 | status: PASS | attempt: 2 | files: spec.md, plan.md, research.md | reason: 새 함수·필드·파일·golden·evidence는 [NEW]다
- Q-CORR-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: StateDriven 세 개를 parser regex의 WHERE … THEN 형식으로 고쳤다
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline이 existing, history·binary·docs, [NEW]를 나눈다
- Q-COMP-01 | status: PASS | attempt: 1 | files: spec.md, plan.md, acceptance.md, research.md, prd.md | reason: 계약, 계획, oracle, 근거, 요약이 나뉜다
- Q-COMP-02 | status: PASS | attempt: 3 | files: plan.md | reason: effort 보존, claim lifecycle, 렌더 경로, 소비처를 owner가 있는 task로 옮겼고 섞인 상태의 사용자 편집·실패 seam(T5)과 `custom` 새 이름 경로(T10)를 더했다
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: REQ마다 조건과 관측 대상이 있다
- Q-COMP-04 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: 결정 표와 Q1 답을 REQ와 non-goal로 옮겼고 REQ-MUX3-10의 사용자 소유 약속을 섞인 상태까지 계약으로 닫았다
- Q-COMP-05 | status: PASS | attempt: 3 | files: research.md, acceptance.md | reason: INV-MUX3-01–09가 S1–S24 oracle에 이어진다. S22(e)는 실제 transaction 실패를, S24는 섞인 상태 사용자 편집과 release 뒤 표시 유지를, S20 넷째·다섯째는 `custom` 새 이름과 built-in 분기를 판정한다
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md | reason: Traceability Matrix가 REQ 21개를 잇는다
- Q-COMP-07 | status: PASS | attempt: 2 | files: research.md | reason: live 미확인 둘을 CD-1·CD-2로 남겼다
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: A1 PASS, A2·A3 not-run 이유, 백업 진술 재분류
- Q-FEAS-01 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: Claim Set Contract와 task 규칙을 owner 파일에 묶었다
- Q-FEAS-03 | status: PASS | attempt: 2 | files: acceptance.md, plan.md | reason: S22(e)는 내용 변경과 test 전용 실패 seam으로 실제로 실패하고, S21 key 수를 12개로 고쳤으며, probe 횟수는 단언하지 않는다
- Q-SEC-01 | status: PASS | attempt: 1 | files: spec.md | reason: 사용자 파일·설정을 쓰지 않는 규칙과 terminalSafe가 Trust Model에 있다
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: 사용자 파일·key는 S8·S22가 SHA로 판정한다
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 구성 흐름과 OMP 표면 하나로 모인다
- Q-COH-02 | status: PASS | attempt: 2 | files: research.md | reason: live 확인이 빠지면 debt로 남아 sync를 막는다

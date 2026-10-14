# SPEC-MODELUX-003 T1 기존 test 분류

`research.md`의 `### T1 분류` 두 항목이 요약하고 이 파일을 가리킨다. 아래는 그 항목에 있던 표와 기준을 그대로 옮긴 것이다.

## effort 폼 (T2·T9·T10)

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

## OMP agent source

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
| `pkg/config/role_model_policy_overlay_test.go` (T15) | fixture | `RootAgentOverlay_FailsClosed`의 "no selected profile"은 프로필 없는 `agents`를 거부하는 v0.50.125 계약이다. REQ-MUX3-22로 그 거부는 native 모드에만 남으므로 그 case에 `agent_source: native` 한 줄을 더했고 단언(`agents_require_profile`)은 그대로다. autopus의 pins-only 판정은 `role_model_policy_pins_test.go`가 한다 |

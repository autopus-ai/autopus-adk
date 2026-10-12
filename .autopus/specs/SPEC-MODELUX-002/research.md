# SPEC-MODELUX-002 리서치

worktree `feat/agent-model-ux`에서 읽었다(2026-10-12). 줄 번호는 HEAD `44773355`(SPEC-MODELUX-001 구현) 기준이다. 첫 초안은 `0d995af8`이고, 이 판은 init OMP 결정(2026-10-12)을 반영한다. 확인하지 못한 사실은 `미확인`으로 적었다.
## 기존 코드 분석
### 줄 driver와 seam (SPEC-MODELUX-001, `internal/cli/`)
- `quality_compose_driver.go`: `run` L33-40(snapshot 1회 → 표 → `menu` L84-108), `composeSharedReader` L68-72, `review` L150-171(`y`만 적용, `e` 재편집), `ask` L219-228(cooked mode 한 줄).
- `quality_compose_driver_edit.go`: `groupStep` L67-82, `askTier` L87-103, `tierChoices` L114-120(등급 이름만, 결함 4), `advancedStep` L127-142(`agent=tier`).
- `quality_compose_driver_gate.go`: seam `composeTTYProbe`·`composeCIEnv`·`runInitWizard` L18-27, `composeGate` L39-43, `mode` L57-65, `runComposeStep` L72-81(비대화형은 stdin·snapshot 0). 인터페이스 `quality_compose_driver_types.go` L19-117, 문구 `quality_compose_text.go` L42-92, locale seam L8.
- `quality_compose_session.go`: `newComposeSession` L40-57, `composeSessionRun` L60-69(표와 planner가 같은 cfg), `persist` L191-196(파일 또는 메모리).
- `quality_compose_snapshot.go` `captureComposeSnapshot` L24-29(`cfg.Platforms`에 codex가 있을 때만), 생성 함수 L34-89. `quality_compose_table.go`: 열 순서 L11, 열은 `cfg.Platforms`에서(L91-95), OMP 칸은 요청 route(L195-220). `quality_compose_table_render.go`: 칸 표기 L107-171, 칸 escape `escapeComposeTableLayout` L201-214(`terminalSafe`는 `terminal_text.go:18`).
- `quality_compose_bind.go` L144-175·L228-242, `quality_compose_bind_write.go` `composeCandidate` L47-59·`persistComposeBinding` L66-90. `quality_compose_entry.go` L28-58, `quality_compose_entry_route.go`(대상 이름 L41-71, `omp` → `runOMPQualityInteractive` L121-123, 전역 메뉴 L130-166), `quality_compose_entry_override.go` `askOverrides` L101-125(줄 읽기). `update_compose.go`: `var updateComposeStep` L14(주석상 test가 감싼다), `runUpdateComposeStep` L29-49. `init_compose.go` `runInitComposeStep` L28-63(메모리 writer L50).
### 진입점과 생성
- `update.go`(299줄): `Long` L27-40, `addedPlatforms` L179, migration 저장 L182, 감지 출력 L185-187, 구성 단계 L217, supervisor `config.Save` L219-227, 설정 플랫폼 생성 L233-251, 감지 플랫폼 생성 L255-257.
- `platform_update.go`: `newlyDetectedPlatforms` L21-29, `updateDetectedHarnessPlatforms` L37-78(플랫폼마다 `config.Save` L64). `platform_lifecycle.go`: `updatePlatformThenSave` L60-86(`descriptor.Update` L72), `saveConfigWithGenerationRollback` L128-150. `platform_catalog.go`: 빈 option adapter L125-135, snapshot adapter `driftAdapter` L118-124, OMP descriptor `omp.NewWithRoot(root)` L86-89.
- `init_helpers.go:52-65` 감지(`omp --version` 실행). `init.go`(260줄): wizard L106-127, 구성 단계 L127, `init cancelled.` L118-121, 저장·생성 L193-197. `quality.go`(280줄) L186-192, OMP `deps`가 여기서 온다.
### 도구별 편집기와 등급 투영
- `quality_tool_targets.go`: 도구 L24-28, 상속 전용(OpenCode, Antigravity) L33-36, Model 열 `qualityToolModel` L90-95(slug), 검증 L101-121, `qualityToolPresetName` L126-128(`<provider>-custom`). `quality_tool_interactive.go` 흐름 L19-81, 이름 guard L150-177, `withQualityToolPreset` L207-231. `quality_tool_persist.go:20-57` `saveQualityToolPreset`(description L39-42, `saveQualityBlock` L43).
- `pkg/config`: `quality_tier.go` slug L8-11·`AgentTier` L76-91·`ClaudeAgentModel` L94-99·`ClaudeModelForTier` L103-114, `quality_effective.go` L11-65(`EffectiveAgentTier`, `ClaudeAgentProfile`, `ClaudeAgentEffort`: mode와 등급만 본다), `quality_provider.go` `EffectiveMode` L56-66·`ForProvider` L79-84, `codex_profile.go` 사다리 L105-124(fable astra·max, opus 6.1-sol·xhigh, sonnet luna·max, haiku luna·선언 effort, ultra는 opus 미만을 opus로), `native_balanced.go:9-31`, `role_model_policy_balanced.go` L21-69(Codex 표준 배치 = 사다리 값, Claude 표준 배치 ≠ 사다리 값). 선언 effort(`content/agents/*.md`, `MapEffort` `pkg/content/agent_transformer_mapping.go:59-70`): architect, deep-worker, planner, security-auditor, spec-writer max, reviewer high, 나머지 10개 medium. `pkg/adapter/codex/codex_resolve.go:21-31`이 표와 템플릿의 해석 함수다.
### huh (`go.mod`: huh v1.0.0, bubbletea v1.3.6, lipgloss v1.1.0, termenv v0.16.0)
- `tui/wizard_steps.go`: 단계마다 `form.Run()` L66, Windows guard L54-60, `WithTheme(AutopusTheme()).WithWidth(bannerWidth+10)` L171. 테마 `wizard_styles.go:12-53`. `tui/style.go:22-33`은 비TTY stdout만 ASCII renderer.
- huh `form.go`: `WithAccessible` L237-240, `WithOutput`·`WithInput` L332-344, accessible 실행 L662-718, 폭 0이면 터미널 폭 L533-540, `ErrUserAborted` L51·L691, 중단 키는 Ctrl+C뿐(`keymap.go:109`). accessible Select `field_select.go:712-744`(`N. label`, 묶인 값이 기본)는 `accessibility.PromptInt`를 거쳐 `PromptString`을 부르고, `PromptString`은 질문마다 `bufio.NewScanner`를 만든다(`internal/accessibility/accessibility.go:138`, EOF면 기본값 L147-152). `NewForm`의 기본 출력은 `os.Stderr`다(form.go:112).
## S21 실제 터미널 smoke 관찰 (SPEC-MODELUX-001)
원본 `<session-scratchpad>/s21-raw-v1.txt`, ANSI 제거본 `s21-stripped.txt`. 환경 `TERM_PROGRAM=Orca`, `TERM=xterm-256color`, locale en.

| # | 관찰 | 위치 | 원인 |
|---|---|---|---|
| O1 | 줄 프롬프트에 화살표가 `^[[C^[[D`로, IME 글자가 그대로 찍혔다 | stripped L150 | 줄 driver는 cooked mode 한 줄 읽기다(`ask` L219-228) |
| O2 | 그룹 선택지가 `1) fable 2) opus 3) sonnet 4) haiku`뿐이다 | stripped L151-163 | `tierChoices` L114-120(결함 4) |
| O3 | Codex와 OMP를 구성 단계에서 따로 정할 수 없다 | 사용자 피드백 | 구성 단계는 전역 등급만 바꾼다. Codex·OMP 편집기는 `auto quality`의 줄 편집기로만 닿는다 |
| O4 | 감지된 antigravity-cli, opencode, omp가 표에 없다 | stripped L219-241 | 표는 `cfg.Platforms`를 쓰고 감지 플랫폼은 구성 단계 뒤에 더해진다(update.go L255-257)(결함 1) |
| O5 | update 뒤 SHA가 바뀌었다 | stripped L256, L272-274 | 감지 저장(`config.Save`, platform_update.go L64)이다. 구성 단계는 `No changes; nothing written.`이었다(결함 2) |
| O6 | init·update 맨 앞에 `ESC]11;?ESC\` + `ESC[6n` | raw 4행(byte 314), 219행(byte 17605) | `## OSC 11 조사`(결함 3) |
## OMP 조사 결과
1. OMP 모델은 quality 등급과 별개인 `role_model_policy`(profile, family, native agent pin)로 정한다(독립성은 의도, `quality_omp_policy.go:16-17`). 2. 지정 단위는 native agent 5개(`omp_native_agents.go:6-8`)다. 13개 역할이 `task`로 접히므로(L12-27) 역할 그룹을 OMP에 쓰지 않는다.
3. 선택지는 설치된 catalog의 selector와 thinking이다(`quality_omp_custom.go` `installedOMPCustomModelChoices` L133-159, capability 거부 `requireOMPCustomCapability` L102-126, thinking L238-265). pin 계열은 선언되거나 catalog가 증명해야 한다(`platform_omp_profile_effective.go:200-205`).
4. 실제 route는 plan의 effective selector다(`platform_omp_profile_preview.go:140-154`). 쓸 수 없는 route는 blocker이고 apply 전에 막힌다(`quality_omp_interactive.go:90-92`).
5. 기존 편집기 `runOMPQualityInteractive`(L46-110): profile(L48-50) → custom pin(L57-71) → 계열(L77-79, 같은 이름의 명시 profile이면 생략 L73-76) → `newOMPProfileApplyOptions`(L81) → `planOMPProfile`(L85) → 요약 `renderOMPQualitySummary`(L89, L121-141) → `[y/N]` → `applyOMPProfile`(L105).
6. 파일 경로: `planOMPProfile`(`platform_omp_profile_apply.go:114-130`) = `loadOMPProfileConfig`(파일 snapshot, `validateOMPProfileSource`, `omp_platform_not_configured` L153-156) + `resolveOMPProfileProposal`(`platform_omp_profile_effective.go:43-71`, 넘겨받은 cfg를 고친다 L100-140) + 미리보기 payload. `applyOMPProfile`(L160-194)은 `marshalAutopusConfig`(`platform_omp_config.go:71-76`) → `replaceAutopusConfigSection`(같은 파일 L88-135, 문서 전체 재encode L120, placeholder guard L131-133)으로 쓰고 OMP adapter Update로 활성화하며(`platform_omp.go:20-32`) 실패하면 원본으로 되돌린다(L198-222).
7. init에는 아직 파일이 없어 6의 파일 경로를 쓸 수 없다. `resolveOMPProfileProposal`은 메모리 cfg를 받으므로 그대로 핵심이 된다. init의 저장(`config.Save`)과 OMP 생성(model integration: profile이 비면 아무것도 묶지 않고 `omp_model_integration.go:38-40`, 있으면 catalog probe·degrade·되읽기)이 `saveConfigWithGenerationRollback` 안에서 활성화와 rollback을 맡는다.
8. `applyOMPProfile`은 자기가 읽은 cfg로 쓰고 활성화한다. 호출자 cfg는 그대로라서, 동기화하지 않으면 update의 뒤 `config.Save`(update.go L224, platform_update.go L64)·OMP 재생성(L236)과 `--apply`가 같은 run에서 옛 선택으로 덮는다.

결론(최소 안전 다리): OMP 탭은 기존 OMP 편집기 판단의 폼 판이다. update·quality는 기존 파일 경로(`planOMPProfile`, `applyOMPProfile`) 뒤 cfg를 동기화한다. init은 `planOMPProfile`의 메모리 핵심을 `[NEW] planOMPProfileForConfig`로 꺼내 init cfg의 분리 사본에 쓰고, 확인하면 init cfg의 `RoleModelPolicy`만 바꾼다. catalog 확인, 계열 증명, capability 거부, blocker, placeholder 규칙은 두 경로가 같은 함수로 지킨다.
## OMP test seam 조사 (2026-10-12, 도구 호출 4회)
- OMP adapter: `(*omp.Adapter).WithModelIntegrationRunner`(`pkg/adapter/omp/omp_model_integration.go:21-25`)가 catalog·설정 되읽기 실행을 바꾼다. 기본값은 `NewWithRoot`의 exec runner(`omp.go:45-51`)다. `pkg/adapter/omp` test 14개 파일과 `internal/cli/doctor_omp_model_routing_e2e_test.go:36,77`(adapter를 직접 만든다)이 쓴다.
- CLI 명령 경로: OMP descriptor는 `omp.NewWithRoot(root)`를 그대로 쓰고(`platform_catalog.go:86-89`) catalog를 바꾸는 internal/cli test는 없다. 그래서 test 전용 `[NEW] platformOMPModelRunner`(nil이면 지금과 같음)를 둔다. runner로는 같은 package의 `ompModelDoctorE2ERunner`(`doctor_omp_model_routing_e2e_test.go:110-146`)를 쓴다. `--version`, `models --json --no-extensions`, `--config` 파일을 읽어 답하는 `config get <key> --json`을 처리하고, 기존 e2e test가 이것으로 `Generate`를 돌린다(L36, L77). 그래서 init OMP test는 실제 생성과 되읽기까지 확인한다(R4 닫음).
- 구성 단계의 OMP catalog: `ompPlatformDependencies.newRunner`(`platform_omp.go:14-32`)가 이미 seam이다. 기존 OMP 편집기 test는 가짜 runner `ompCLIFakeRunner`(`platform_omp_test_helpers_test.go`)와 fixture `ompCLIBalancedCatalogJSON`(`platform_omp_profile_flags_test.go:74-81`)로 돈다. 플랫폼 감지: package 변수 seam은 없다. test는 `t.Setenv("PATH", …)`로 고정한다(`update_platform_visibility_test.go:19`, `omp_identity_gate_test.go:15-24` `writeFakeOMPBinary`, `init_helpers_presence_posix_test.go:22`). `update_compose_helpers_test.go`, `update_test.go`도 PATH를 설정하므로 R1 위험은 낮아졌다(설정 값은 미확인). update 구성 단계의 wrapper는 `update_compose_helpers_test.go:143-146`(시그니처 `func(cmd, dir, cfg, composeGate) driftGenerationSnapshot`)이다.
## OSC 11 조사 (결함 3)
- 보내는 곳: bubbletea v1.3.6 `tea_init.go:7-22`의 package `init()`이 `lipgloss.HasDarkBackground()`를 부른다(주석 "removed in v2"). lipgloss `renderer.go:138-151` → termenv `termenv_unix.go:234-293`이 `OSC 11;?`+ST(L269)와 `CSI 6n`(L272)을 쓰고 echo를 끈 채 응답을 읽는다(timeout 5초). stdout 비TTY, `CI` 설정, `TERM` screen·tmux·dumb이면 보내지 않는다(termenv.go L28-40, termenv_unix.go L237-240).
- huh를 링크한 `auto`는 시작할 때마다 보낸다. SPEC-MODELUX-001 이전부터의 동작이다. 응답하는 터미널에서는 남는 글자가 없고 transcript에도 echo된 응답이 없다. Orca가 `]11;?\`를 실제로 그렸는지는 미확인이다. 막지 않는 이유: `GODEBUG=inittrace=1`에서 termenv → lipgloss → bubbletea가 연달아 초기화된다(211-213번째). 우리 package가 끼어들 수 없다. bubbletea를 `replace`하면 `go install …@version`이 막힌다(Go 규칙, 이 저장소에서 실행은 미확인). 배경을 고정하면 밝은 터미널에서 adaptive 색이 틀어진다(`AutopusTheme`은 `ThemeCharm` 기반).
- 결정: 기록하고 새 query를 더하지 않는다. smoke 비교는 OSC·CSI를 지운 사본으로 한다. 근본 해결은 v2 이전이다(Evolution Idea).
## Probe P1: 감지 플랫폼이 있는 update의 quality 블록 (결함 2)
- 2026-10-12, 이 기계(감지 대상 CLI 설치). `CODEX_HOME=<scratch> auto init --yes --platforms claude-code,codex` 뒤 `auto update --yes`, 바이너리 HEAD 44773355. 스크립트, 해시, diff, 출력은 경로를 치환해 `evidence/probe-a1.md`에 옮겼다.
- 결과: `+ 새 플랫폼 감지: antigravity-cli, opencode, omp`. 파일 SHA는 바뀌었고(`b7f6c3aa…` → `5af0a6e9…`) `quality:` 블록 SHA는 같았다(`a8ff97ba…`). diff는 `platforms` 3줄 추가뿐이다. 그래서 oracle은 "quality 블록 bytes 불변 + diff는 `platforms` 추가뿐"이다. 대화형 유지 경로가 쓰지 않는다는 것은 SPEC-MODELUX-001 A2 증거다.
## 설계 결정
| 결정 | 선택 | 기각한 대안 | 근거와 trade-off |
|---|---|---|---|
| 입력 방식 | 입력이 `os.Stdin`, stdin·stdout TTY, Windows guard 통과면 huh 폼, 그 밖은 줄 driver | 줄 driver 제거 | scripted test와 pipe가 그대로 돈다. huh를 비TTY stdout에 그리면 escape가 파일로 간다. driver 둘을 유지하는 비용이 생긴다 |
| 폼 test | `[NEW] composeRunForm` 변수, test는 huh accessible mode + `iotest.OneByteReader` | PTY 라이브러리, 값을 넣는 가짜 runner | 새 의존성 없이 진짜 field가 돈다. Scanner가 질문마다 새로 생겨 앞을 읽으므로 1byte reader가 필요하다. 실제 렌더링은 수동 smoke가 닫는다 |
| 폼 모양 | 질문마다 field 하나, `tui.AutopusTheme()`, 폭 미지정 | wizard처럼 폭 50 | 실제 모델 라벨이 50칸을 넘는다. 폭 0이면 huh가 터미널 폭을 쓴다 |
| 질문 구조 | 메뉴 → 탭 → 방식 → 편집 → 미리보기 → 확인, 적용·취소 뒤 메뉴로 돌아온다 | 한 번 적용하면 끝 | 한 run에서 Claude, Codex, OMP를 차례로 정한다. 유지는 Enter 1회다. 쓰기는 도구마다 따로 확인한다 |
| 도구 탭 | `모든 도구 같게` + 설치된 Claude Code, Codex, OMP | 플랫폼 5개 모두 | OpenCode·Antigravity는 에이전트 파일에 model이 없다(`quality_tool_targets.go:33-36`) |
| 모든 도구 같게 | SPEC-MODELUX-001 등급·Binding Contract·`quality.default` 그대로, 라벨에 도구별 칸 | 등급 이름만, 도구별 값 저장 | 결함 4는 표시 문제다. 저장 의미를 바꾸면 승인 범위를 다시 연다 |
| 라벨 계산 | 선택지를 작업 구성에 넣은 후보 config의 표 칸, run snapshot. Claude 모델은 `ClaudeAgentModel` slug(`claude-` 제거) + `ClaudeAgentProfile` effort | 등급→모델 고정 표, Claude도 표 칸처럼 alias | 표·생성과 같은 해석 함수다. 같은 등급도 binding에 따라 값이 다르다(C6). slug는 사용자가 정한 표시이고 기존 Model 열(`qualityToolModel`)과 같다. 표 칸은 C16대로 두고 표기 차이는 문서로 알린다 |
| 도구별 저장 값 | 상대 등급, 실제 이름은 표시만 | 에이전트별 모델 ID | Claude는 등급과 slug가 1:1, effort는 (mode, 등급)으로 정해진다. Codex 실제 선택지는 모두 어떤 등급의 결과다. 선언 effort가 max인 5개 에이전트는 Codex에서 sonnet과 haiku 값이 같다. 새 key가 없어 strict decode(v0.50.113+)가 깨지지 않는다 |
| 도구별 binding | `quality.presets.<provider>-custom`(16개) + `quality.providers.<provider>`, 쓰기는 `saveQualityToolPreset`, init은 메모리 | 이름 질문, `custom-codex`, `quality.default`로 우회 | 기존 편집기의 기본 이름(`qualityToolPresetName`)과 description을 쓴다. fixture P와 S12가 `codex-custom`을 쓴다. `quality.providers`는 v0.50.88부터 읽고 그 이전은 무시하므로(SPEC-MODELUX-001 C11) 미리보기에 한 줄로 알린다 |
| OMP 다리 (update·quality) | 기존 편집기 판단의 폼 판, 기존 plan·apply 경로, 쓴 뒤 `[NEW] composeReloadConfig`(기본 `config.LoadPreview`)로 다시 읽어 `cfg.RoleModelPolicy`를 맞춘다. 되읽기가 실패하면 run을 오류로 끝낸다 | OMP를 등급에 묶기, 새 writer, 메모리 typed 값으로 동기화 | OMP 조사 1-8. typed 값은 loader가 채운 값과 다를 수 있어 되읽는다. 대가: OMP 쓰기는 문서 전체를 다시 encode하고(기존 동작), update에서는 OMP가 활성화와 생성 루프에서 두 번 생성된다 |
| OMP 다리 (init) | `[NEW] planOMPProfileForConfig`로 init cfg 사본을 plan, 같은 모델 목록·계열 증명·blocker·placeholder 검사, 확인하면 init cfg의 `RoleModelPolicy`만 교체, 쓰기와 활성화는 init의 `config.Save`와 OMP 생성 | 힌트만 표시(첫 초안), 임시 파일로 `applyOMPProfile` | 사용자 결정(2026-10-12). 임시 파일은 init의 rollback 경계 밖에 쓰기를 만든다. catalog를 못 읽으면 안내하고 init을 계속한다. 대가: init 생성에 OMP model integration 실패 경로가 생기고, 실패하면 init 전체가 rollback된다 |
| OMP 노출 | update·quality는 파일의 `platforms`에 omp, init은 init cfg의 `platforms`에 omp. update에서 이번 run에 감지만 된 omp는 안내만 | 감지 직후에도 지정 | 파일 경로는 파일의 omp를 요구한다(`omp_platform_not_configured`). update의 감지 플랫폼은 구성 단계 뒤에 저장된다 |
| 감지 플랫폼 | 표·탭·snapshot은 설정+감지 view, 쓰기·동기화는 읽어 온 cfg, 감지 생성도 같은 snapshot. 감지 목록은 `[NEW] composeGate.Detected` | cfg.Platforms에 먼저 더하기, `updateComposeStep` 인자 추가 | 뒤의 `updateDetectedHarnessPlatforms` 순서를 바꾸지 않는다. view가 쓰기 대상이면 확인한 구성이 뒤 `config.Save`에서 사라진다. 시그니처를 지켜 기존 test를 고치지 않는다 |
| Ctrl+C | 이 단계는 더 쓰지 않는다. update는 명령 전체 중단(중단 줄, 0이 아닌 종료, 생성 없음, 이미 저장된 migration은 남음), init은 `init cancelled.`, quality는 오류 없이 반환 | update는 유지로 보고 계속(첫 초안) | 통합 결정(2026-10-12, review F-005). 줄 driver에서 Ctrl+C는 SIGINT로 명령 전체를 끝냈다. huh는 이를 `ErrUserAborted`로 바꾸므로 같은 결과를 명시한다. 신호는 `[NEW] composeGate.Abort`라서 `updateComposeStep` 시그니처를 지킨다 |
| 기준 상태 | 마지막으로 성공한 쓰기 뒤의 파일과 cfg. 유지·취소·실패·Ctrl+C는 그것을 보존한다 | run 시작 상태 | 메뉴 반복에서 앞서 승인한 쓰기를 되돌리지 않는다(review R04) |
| 쓰기 안전 | 경로마다 staging·atomic 교체·rollback을 Write Safety Contract로 정하고, 폼의 override 제거는 구성 쓰기와 block 하나로 묶는다. 기본 프리셋은 분리 사본으로 쓴다 | 기존 저장 함수 재사용만 적기, 두 번 쓰기 | `saveQualityDefault`는 쓰기 전에 cfg를 바꾸고(review F-006), 두 번 쓰기는 중간 실패 때 반만 남는다(review R02) |
| 폼 출력 스트림 | runner가 `WithInput(cmd.InOrStdin())`·`WithOutput(cmd.OutOrStdout())`를 건다 | huh 기본값 | `NewForm` 기본 출력이 `os.Stderr`라 `2>파일`에서 폼이 사라진다(form.go:112, review R05) |
| update 도움말 | `Long`을 `[NEW] update_help.go`로 옮기고 덧붙인다 | update.go 안에서 수정 | update.go가 299줄이다. 기존 문장은 지우지 않는다 |
## Plan Intent Ledger
Clarification Ledger unavailable. 입력은 planning context의 결정 표(answered)와 2026-10-12 추가 결정이다. 다시 묻지 않았다.

| Field | Status | SPEC 반영 |
|---|---|---|
| input (huh 폼, 줄 driver는 비TTY·scripted용, test seam) | answered | REQ-MUX2-01, 02, 03, 08, 09, 15, 17, 18 |
| per-tool (도구 탭, 실제 모델 이름, OpenCode·Antigravity 제외, 모든 도구 같게 = SPEC-MODELUX-001) | answered | REQ-MUX2-04, 05, 06 |
| storage (기존 per-provider binding, 새 key 없음) | answered | REQ-MUX2-07, 설계 결정의 저장 값·binding 행 |
| OMP (지정 가능, 기존 OMP 규칙 재사용) + init에서도 지정, 메모리 경로, catalog가 없으면 안내 후 계속 | answered (init은 2026-10-12 추가) | REQ-MUX2-10, 11, 22, 23 |
| release (v0.50.125, PR #196, 좁은 범위, 새 secret·승인·유료 CI 없음) | answered | header, Outcome Lock |
| 추가 결함 1-4 | answered (요구사항으로) | REQ-MUX2-12, 13, 19, 05 |
| update의 Ctrl+C는 명령 전체 중단 | answered (통합 결정 2026-10-12, review F-005) | REQ-MUX2-14 |
## Question Audit
- question_transport: none (planning context가 결정을 넘겼다) · question_count: 0 · unresolved_fields: none
## Outcome Lock
- User-visible outcome: 실제 터미널의 `auto init`, `auto update`, `auto quality`에서 구성 단계를 화살표와 Enter로 다룬다. 모양은 init wizard와 같다. 도구 탭에서 `모든 도구 같게`, Claude Code, Codex, OMP를 고르고 선택지에는 그 도구의 실제 모델과 effort가 보인다. Codex 결과는 `quality.providers.codex: codex-custom`(Claude는 `claude-custom`)으로 저장되고 같은 run의 생성 파일이 승인한 미리보기와 같다. OMP는 세 진입점 모두에서 설치된 catalog로 native agent별로 고르고 실제 route 미리보기를 확인한 뒤 저장된다. init은 같은 검사를 거친 메모리 cfg를 init의 저장으로 쓰고, catalog를 못 읽으면 안내하고 계속한다. update 표에는 감지 플랫폼 열도 있다. 유지·취소·실패에서는 이 단계가 쓰지 않으므로 파일 변화는 감지와 기존 migration뿐이다(canonical 파일은 quality 블록 bytes, 그 밖은 decode 값이 같다). 줄 입력은 SPEC-MODELUX-001과 똑같다.
- Mandatory requirements: REQ-MUX2-01 ~ REQ-MUX2-18, REQ-MUX2-22, REQ-MUX2-23 (Must).
- Explicit non-goals: `spec.md` `## Outcome Boundary` 목록과 같다.
- Completion evidence: Must S1–S16, S20–S22 PASS, 기존 test 수정 0건, `auto eval harness run`, 신규·변경 package coverage 85% 이상, 신규·변경 소스 300줄 이하, `GOOS=windows GOARCH=amd64 go build ./...`, `evidence/manual-smoke-002.txt`, `auto spec validate --strict`.
## Visual Planning Brief
흐름도와 wireframe은 `plan.md`에 있고 모두 `wireframe intent: assumed`다. 사용자 confirm 전이므로 라벨 문구와 배치는 risk와 reviewer focus로 둔다. 요구사항으로 묶는 것은 Label Contract와 Form Flow Contract의 순서·기본값뿐이다.
## Technology Stack Decision
| Mode | Selected stack | Resolved versions | Source refs | Checked at | Rejected alternatives |
|---|---|---|---|---|---|
| brownfield | 기존 Go 모듈, huh(wizard가 쓰는 폼), `testing/iotest`, 기존 quality·OMP writer | huh v1.0.0, bubbletea v1.3.6, lipgloss v1.1.0, termenv v0.16.0. 새 의존성·버전 변경 없음 | `go.mod`, module cache | 2026-10-12 | huh·bubbletea v2 이전, PTY test 라이브러리 |
## Minimality Decision Matrix
| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | 사용자 피드백, smoke 관찰 O1-O6, init OMP 결정 | proceed | 폼 driver, 도구 탭, 도구별 저장, OMP 탭(init 포함), 감지 열, oracle 수정 |
| existing code/helper/pattern | `tui.AutopusTheme`, wizard Windows guard, SPEC-MODELUX-001 planner·표·snapshot·writer, `withQualityToolPreset`, `saveQualityToolPreset`, `qualityToolPresetName`, OMP 편집기 함수, `resolveOMPProfileProposal`, `driftAdapter`, `WithModelIntegrationRunner`, `terminalSafe` | reuse | 판단과 쓰기는 기존 함수다 |
| stdlib/native | `testing/iotest.OneByteReader`, `os`, `slices`, `maps` | use | accessible test 입력, view 사본 |
| existing dependency | huh v1.0.0(Select, Confirm, accessible mode, `ErrUserAborted`) | reuse | 화살표·Enter 입력 |
| new dependency or new abstraction | 새 의존성 0. 새 abstraction: 폼 seam 2개, 폼 driver, 라벨, 도구별 planner, OMP 탭, `planOMPProfileForConfig`(추출), 플랫폼 view, `platformOMPModelRunner` | accepted | 각각 REQ-MUX2-01·17, 03, 05·06, 07, 10·11, 22, 12, 22 test의 실제 요구다 |
| minimum sufficient verification | accessible mode로 도는 실제 명령, 실제 `Generate` 비교, CAT-A·B, OMP 가짜 runner, 기존 test 무수정, 수동 smoke 1회 | required checks | security(raw writer와 OMP placeholder 규칙), data-loss(유지 경로, cfg 동기화, init rollback), deterministic-oracle gate를 줄이지 않는다 |
## Semantic Invariant Inventory
| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-MUX2-01 | "keep-path leaves the quality block bytes unchanged" | 유지·취소·중단·변경 없음·거부·실패에서 이 단계는 쓰지 않고 기준 상태(마지막 성공 쓰기 뒤)를 보존한다. 파일 변화는 감지·기존 migration뿐이고, canonical 파일은 quality 블록 bytes가, 손으로 쓴 파일은 decode한 quality·role_model_policy 값이 같다 | 블록 bytes, decode 값, 파일 diff | S2, S7, S9, S11, S12, S20, S22 |
| INV-MUX2-02 | "choices are shown with that tool's REAL model names" | 라벨 = 그 선택이 만드는 후보의 표 칸(같은 snapshot) | 라벨 문자열 | S4, S5, S6, S14 |
| INV-MUX2-03 | "per-tool result uses the existing per-provider binding" | 도구별 쓰기는 `quality.presets.<p>-custom`과 `quality.providers.<p>`만 바꾼다 | quality key, 표 `*` | S5, S6 |
| INV-MUX2-04 | "generated files equal the approved table" | 같은 run의 Claude·Codex 생성 = 승인한 미리보기(감지 플랫폼 포함) | 생성 `model`·`effort` | S5, S6, S10, S14 |
| INV-MUX2-05 | "keep OMP catalog/attestation rules intact" | OMP 쓰기는 update·quality는 기존 apply와 되읽기 동기화, init은 같은 plan 검사의 메모리 cfg와 init 저장·실제 OMP 생성으로만. 막히거나 catalog가 없으면 쓰기·활성화 0, 생성이 실패하면 init 전체 rollback, 되읽기가 실패하면 run 중단 | role_model_policy, 생성된 OMP 설정, 활성화 횟수, rollback 뒤 파일 | S8, S9, S21, S22 |
| INV-MUX2-06 | "line driver unchanged" | 줄 입력의 프롬프트·키·출력 bytes가 HEAD 44773355와 같다 | 출력 bytes, 기존 test | S1, S13 |
| INV-MUX2-07 | "non-interactive never reads stdin" | 비대화형과 preview는 폼을 열지 않고 stdin을 읽지 않는다 | 폼 실행, Read 횟수 | S1 |
| INV-MUX2-08 | "one Codex catalog snapshot per run" | 메뉴 반복과 감지 생성까지 catalog probe는 run당 1회 | probe 횟수 | S2, S10 |
| INV-MUX2-09 | "table/tabs must reflect the platforms the update will generate" | 표 열 = 설정 + 감지(고정 순서), OpenCode·Antigravity는 탭에 없다 | 머리말, 탭 | S3, S10 |
| INV-MUX2-10 | "older binaries must still load the file" | 새 YAML key가 없다 | quality key, decode | S5 |
| INV-MUX2-11 | "stray ]11;?\" | 폼은 터미널 query를 더하지 않고 YAML·catalog 글자는 escape한다 | 소스 검사, 라벨 bytes | S15, S18 |
## Feature Coverage Map
| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| 입력 방식과 줄 driver 불변 | REQ-MUX2-01, 15, 17 / S1, S13 | covered |
| 메뉴, 탭, 방식, 편집, 미리보기, 확인 | REQ-MUX2-03, 04, 08, 09 / S2, S3, S7 | covered |
| 실제 모델 라벨(결함 4), 도구별 저장과 생성 | REQ-MUX2-05, 06, 07, 16 / S4, S5, S6, S14 | covered |
| OMP 지정(update·quality·init) | REQ-MUX2-10, 11, 22, 23 / S8, S9, S20, S21, S22 | covered |
| 감지 플랫폼(결함 1)과 유지 경로(결함 2) | REQ-MUX2-12, 13 / S10, S11, S20 | covered |
| error/recovery: Ctrl+C, blocker, 거부, 취소, catalog 없음, 쓰기·되읽기·init 생성 실패 | REQ-MUX2-11, 14, 18, 22, 23 / S2, S8, S9, S12, S16, S21, S22 | covered |
| 터미널 안전, OSC 11(결함 3), override 폼, locale, 문서 | REQ-MUX2-02, 18–21 / S15–S19 | covered (19–21은 Should) |
## Completion Debt
| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

작성 시점에 알려진 debt는 없다. 판정을 줄이는 대안은 두지 않는다. S21의 실제 OMP 생성·되읽기와 rollback 확인, S11의 손으로 쓴 파일 decode 비교, S10의 감지 Codex 폼 편집, 수동 smoke 기록 중 하나라도 빠지면 debt가 되어 sync 완료를 막는다.
## Evolution Ideas

These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| huh·bubbletea v2 이전으로 시작 시 OSC 11 query 제거 | 의존성 major 이전이다 | 터미널 잔여 글자 재보고 |
| 여러 도구를 한 번의 쓰기로 묶기 | 메뉴 반복으로 도구마다 적용한다 | 한 번의 확인 요청 |
| 도구별 결과가 최신 balanced·ultra와 같으면 built-in 이름으로 묶기 | Binding Contract 행이 늘어난다 | 도구별 custom이 표준 배치를 잃는 문의 |
| OMP pin 해제(inherit) 선택지 | 기존 `auto platform omp profile apply --agent <a>=inherit`가 있다 | UI 해제 문의 |
| 표 OMP 칸의 catalog 반영 route | 표마다 `omp` subprocess가 필요하다 | OMP 탭 밖 실제 route 요청 |
## Sibling SPEC Decision
| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | 같은 구성 단계의 입력, 탭, 저장, 표가 한 흐름이다. task 12개, 소스 파일 약 26개(test 포함)로 sibling 기준(25개 초과 task이면서 40개 초과 파일)에 못 미친다. 같은 PR #196로 출하한다 | None |
## Reference Discipline
| Reference | Type | Verification |
|-----------|------|--------------|
| `internal/cli/` `quality_compose_*.go`, `update*.go`, `init*.go`, `quality.go`, `platform_update.go`, `platform_lifecycle.go`, `platform_catalog.go`, `doctor_drift_content.go`, `prompts.go`, `quality_tool_*.go`, `quality_omp_*.go`, `platform_omp*.go`, `terminal_text.go`, `tui/*.go`; `pkg/config/` 등급·OMP 파일, `pkg/adapter/codex/codex_resolve.go`, `pkg/adapter/omp/omp.go`·`omp_model_integration.go`, `content/agents/*.md` | existing | Read, grep, `wc -l`(2026-10-12) |
| huh v1.0.0, bubbletea v1.3.6, lipgloss v1.1.0, termenv v0.16.0 소스 | existing (module cache) | Read |
| `evidence/probe-a1.md`(probe A1, 경로 치환), `<session-scratchpad>/s21-raw-v1.txt` | existing (evidence) | probe 실행, grep -b |
| `composeFormsProbe`, `composeRunForm`, `composeGate.Detected`, `updateComposeOMPDeps`, `initComposeOMPDeps`, `updateDetectedHarnessPlatformsWith`, `planOMPProfileForConfig`, `platformOMPModelRunner`, `runInitComposeStepCancellable`, `composeGate.Abort`, `errComposeInterrupted`, `composeReloadConfig`, `quality_compose_form*.go`, `quality_compose_tool.go`, `quality_compose_omp.go`, `update_help.go`, `evidence/manual-smoke-002.txt` | [NEW] planned addition | 부재 확인(grep, 2026-10-12) |
## Reviewer Brief
- Intended scope: 세 진입점의 huh 폼 driver, 도구 탭과 실제 모델 라벨, Claude·Codex 도구별 저장, OMP 탭(update·quality는 기존 writer, init은 메모리 경로), update 표의 감지 열과 snapshot 전달, 유지 경로 oracle, OSC 11 기록.
- Explicit non-goals: 줄 driver 변경, 새 YAML key, OMP를 등급에 묶기, 표 OMP 칸의 실제 route, update 감지 직후 OMP 지정, 여러 도구 일괄 쓰기, 의존성 major 이전, OMP writer의 전체 재encode 변경.
- Self-verified: Traceability Matrix, invariant 11개와 구체 값, OMP 조사 1-8, OMP seam 조사, probe P1, 결함 1-4의 원인 위치.
- Reviewer should focus on: OMP 동기화가 update와 `--apply`의 뒤 저장·생성을 덮는지, init 메모리 경로가 파일 경로와 같은 검사를 하는지와 rollback 경계, 감지 view가 쓰기 대상이 되지 않는지, 라벨이 표와 같은 해석 함수를 쓰는지, 폼이 비대화형에서 열리지 않는지, update의 Ctrl+C 전체 중단, 기준 상태와 Write Safety Contract, 그리고 Completion Debt only.
## Open Risks
- R1: 기존 update test의 PATH 설정이 감지를 0으로 고정하는지는 미확인이다. 아니면 CLI가 깔린 기계에서 감지 열 때문에 표 단언이 바뀔 수 있다. T2에서 먼저 돌려 본다. R2: `updateComposeStep`을 감싸는 test는 주석으로만 확인했고, 시그니처를 지키는 설계로 피한다.
- R3: accessible runner와 1byte reader로 form을 이어 돌리는 것은 실행 전이다(probe A2).
- R5: init에서 OMP를 고르면 init 생성에 OMP model integration 실패 경로가 생기고, 실패하면 init 전체가 rollback된다. S21이 rollback을 확인한다. R4(가짜 runner)는 `ompModelDoctorE2ERunner`로 닫았다.
- R6: Claude 라벨(slug)과 표 칸(alias)의 표기 차이. 사용자 터미널이 `]11;?\`를 그렸는지는 미확인이다.
## Self-Verify Summary
- Q-CORR-01 | status: PASS | attempt: 3 | files: spec.md, research.md, acceptance.md | reason: review가 지적한 인용(terminalSafe, replaceAutopusConfigSection, PromptInt)과 S9 fixture를 고쳤다. 확인하지 못한 것은 미확인이다
- Q-CORR-02 | status: PASS | attempt: 2 | files: spec.md, plan.md, research.md | reason: 새 함수·변수·파일·evidence는 모두 [NEW]다(`planOMPProfileForConfig`, `platformOMPModelRunner`, `initComposeOMPDeps` 포함)
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: EARS 문장과 bare Given/When/Then/And 형식이다
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline이 existing, module cache, evidence, [NEW]를 나눈다
- Q-COMP-01 | status: PASS | attempt: 1 | files: spec.md, plan.md, acceptance.md, research.md, prd.md | reason: 계약, 계획, oracle, 근거, 요약이 나뉜다
- Q-COMP-02 | status: PASS | attempt: 2 | files: acceptance.md | reason: Must 시나리오는 실제 명령, 실제 adapter, 수동 smoke로 닫는다. init OMP는 S21, S22다
- Q-COMP-04 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: 결함 1-4, init OMP, review 판정(F-001~F-011, R01~R12)을 요구사항과 계약으로 옮겼다. REQ-MUX2-13은 이 단계가 다루는 범위로 좁혔다
- Q-COMP-05 | status: PASS | attempt: 3 | files: research.md, acceptance.md | reason: INV-MUX2-01은 canonical과 손으로 쓴 fixture를 함께, INV-MUX2-05는 실제 생성과 rollback을 판정한다
- Q-COMP-06 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Traceability Matrix가 REQ 23개를 task, 시나리오, invariant에 잇는다
- Q-COMP-07 | status: PASS | attempt: 2 | files: research.md | reason: Completion Debt는 판정 축소 없이 None이고, 빠지면 debt가 되는 항목을 적었다
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: A1은 probe P1 증거가 있는 PASS, A2·A3(init 경로 포함)는 not-run 이유가 있다
- Q-FEAS-01 | status: PASS | attempt: 3 | files: spec.md, plan.md | reason: Write Safety Contract가 경로별 staging, atomic 교체, rollback 지점을 정한다. update.go 줄 수 제약을 반영했다
- Q-SEC-01 | status: PASS | attempt: 2 | files: spec.md | reason: OMP catalog 출력과 라벨 escape, init 메모리 경로의 placeholder 검사를 Trust Model과 REQ-MUX2-22에 적었다
- Q-COMP-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: 기준 상태(마지막 성공 쓰기 뒤)를 REQ-MUX2-03, 13, 23과 S11, S22에 같은 뜻으로 적었다
- Q-FEAS-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: 변경은 internal/cli, OMP descriptor seam, 문서다. pkg/config 계약은 바꾸지 않는다
- Q-FEAS-03 | status: PASS | attempt: 2 | files: plan.md, acceptance.md | reason: init OMP는 기존 `ompModelDoctorE2ERunner`로 실제 생성까지 돈다. 판정 축소 대안을 없앴다
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: REQ 문장에 모호어가 없다
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Priority는 Must/Should만 쓰고 EARS type과 나눴다
- Q-STYLE-03 | status: PASS | attempt: 1 | files: acceptance.md | reason: step은 bare keyword로 시작하는 완결된 문장이다
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: quality는 raw writer, OMP는 기존 placeholder 거부, init은 같은 node 검사다. S9(2)는 role policy에 실제 placeholder를 둔 fixture다
- Q-SEC-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: 새 로그가 없고 증거 파일은 커밋 전에 홈·scratch 경로와 사용자 이름을 치환한다
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 대화형 구성 단계 하나로 모인다. sibling이 없다
- Q-COH-02 | status: PASS | attempt: 2 | files: research.md | reason: Outcome Lock에 필요한 작업(init OMP 실제 생성 포함)이 모두 Primary SPEC에 있고 우회 대안이 없다
- Q-COH-03 | status: PASS | attempt: 1 | files: research.md | reason: sibling이 없고 근거를 적었다

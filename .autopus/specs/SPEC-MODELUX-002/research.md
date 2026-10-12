# SPEC-MODELUX-002 리서치

worktree `feat/agent-model-ux` HEAD `44773355`(SPEC-MODELUX-001 구현, draft PR #196)에서 읽었다(2026-10-12). 줄 번호는 이 HEAD 기준이다. 확인하지 못한 사실은 `미확인`으로 적었다.

## 기존 코드 분석

### 줄 driver와 seam (SPEC-MODELUX-001)
- `internal/cli/quality_compose_driver.go`: `run` L33-40이 snapshot을 한 번 잡고(`start` L50-61 → `captureComposeSnapshot`) 표를 그린 뒤 `menu` L84-108에서 한 줄을 읽는다. `composeSharedReader` L68-72가 명령 입력을 `bufio.Reader` 하나로 감싼다. 미리보기와 확인은 `review` L150-171(`y`/`yes`만 적용, `e`는 다시 편집), 한 줄 읽기는 `ask` L219-228이다.
- `quality_compose_driver_edit.go`: `groupStep` L67-82, `askTier` L87-103, 선택지 `tierChoices` L114-120은 `1) fable 2) opus 3) sonnet 4) haiku`처럼 등급 이름만 쓴다(결함 4의 원천). 고급은 `agent=tier` 줄 입력 `advancedStep` L127-142다.
- `quality_compose_driver_gate.go`: seam `composeTTYProbe`(= `isStdinTTY`), `composeCIEnv`, `runInitWizard` L18-27. `composeGate{Yes, Check, Preview}` L39-43, `mode` L57-65, `runComposeStep` L72-81. 비대화형은 stdin을 읽지 않고 snapshot도 만들지 않는다.
- `quality_compose_driver_types.go`: 인터페이스 `composeDriverTable` L19-25, `composeDriverGroups` L37-43, `composeDriverPlanner` L63-71, `composeDriverTools` L75-83, 세션 `composeDriverSession` L106-117.
- `quality_compose_text.go`: 줄 driver 문구 ko L42-66, en L68-92, locale seam `composeLanguage` L8.
- `quality_compose_session.go`: `newComposeSession` L40-57. `composeSessionRun{dir, cfg, sources, lang, writer, snapshot}` L60-69가 표와 planner에 같은 `cfg`를 준다. 확인한 쓰기는 `persist` L191-196이 파일(`persistComposeBinding`)이나 메모리(`plan.apply`)로 한다.
- `quality_compose_snapshot.go`: `captureComposeSnapshot` L24-29는 `cfg.Platforms`에 `codex`가 있을 때만 catalog를 읽는다. `updateHarnessPlatformWith` L34-47, `generatePlatformFilesWith` L51-72, `applyQualityHarnessWith` L78-89가 그 snapshot으로 생성한다.
- `quality_compose_table.go`: 열 순서 `composeTablePlatforms` L11. 열은 `cfg.Platforms`에서 고른다(`buildComposeTable` L91-95). OMP 칸은 요청 route다(`composeTableOMPRoutes` L195-220). `quality_compose_table_render.go`: 칸 표기 L107-118, Codex 표기 L151-171, 모든 칸 `terminalSafe` L201-214.
- `quality_compose_bind.go`: `composeCustomPreset = "custom"` L25, `composeBindTarget` L144-175, `composePresetFeedsTable` L228-242는 후보 config의 `Platforms`를 본다. `quality_compose_bind_write.go`: 분리 사본 `composeCandidate` L47-59, `persistComposeBinding` L66-90.
- `quality_compose_entry.go`: `runQualityComposeEntry` L28-49, 입력 판정 `qualityComposeInteractive` L55-58. `quality_compose_entry_route.go`: 대상 이름 `Targets` L41-51, `Route` L58-71, `runQualityTarget` L109-127(`omp` → `runOMPQualityInteractive` L121-123), 전역 메뉴 `runQualityGlobalMenu` L130-166(`saveQualityDefault` L158).
- `quality_compose_entry_override.go`: override 질문 `askOverrides` L101-125는 `bufio.NewReader(cmd.InOrStdin())`로 줄을 읽는다(L104-116). 제거 `removeOverrides` L131-146.
- `update_compose.go`: `var updateComposeStep = runUpdateComposeStep` L14(주석상 test가 감싼다), `runUpdateComposeStep` L29-49. `init_compose.go`: `runInitComposeStep` L28-63, 메모리 writer L50.

### 진입점
- `internal/cli/update.go`(299줄): `Long` 도움말 L27-40, `addedPlatforms := newlyDetectedPlatforms(cfg)` L179, `persistUpdateConfigMigrations` L182, `+ 새 플랫폼 감지` 출력 L185-187, 구성 단계 L217(`composeGate{Yes, Check}`), supervisor migration `config.Save` L219-227, 설정된 플랫폼 생성 L233-251(`updateHarnessPlatformWith` L236), 감지 플랫폼 생성 `updateDetectedHarnessPlatforms` L255-257.
- `platform_update.go`: `newlyDetectedPlatforms` L21-29, `updateDetectedHarnessPlatforms` L37-78은 플랫폼마다 `updatePlatformThenSave(..., config.Save)` L64를 부른다. `platform_lifecycle.go:60-86` `updatePlatformThenSave`는 `descriptor.Update` L72를 쓰고, 이것은 `platform_catalog.go:125-135`의 빈 option adapter라서 Codex가 snapshot 없이 스스로 probe한다. snapshot을 받는 adapter는 `driftAdapter` L118-124와 Codex `newAdapter` L52-57이다.
- `init_helpers.go:52-65` `detectInstalledPlatforms`는 `omp --version`을 실행한다(주석 L49-51).
- `init.go`(260줄): wizard L106-127, 구성 단계 호출 L127, 저장과 생성 L193-197(`saveConfigWithGenerationRollback`, `platform_lifecycle.go:128-150`). wizard 취소는 `init cancelled.`를 쓰고 끝난다(L118-121).
- `quality.go`(280줄): `runQualityInteractive` L186-192 → `runQualityComposeEntry`. OMP 의존성 `deps`가 여기서 넘어온다.

### 도구별 편집기 (Claude Code, Codex)
- `quality_tool_targets.go`: 도구 목록 `qualityModelTools` L24-28(`omp`는 provider 없음), 상속 전용 `qualityInheritOnlyTools` L33-36(OpenCode, Antigravity), `qualityToolAgentRows` L74-88, 모델 열 `qualityToolModel` L90-95(`ClaudeAgentModel`, `CodexAgentModel`), `parseQualityToolTierAssignment` L101-121, 프리셋 이름 `qualityToolPresetName` L126-128 = `<provider>-custom`.
- `quality_tool_interactive.go`: `runQualityToolInteractive` L19-81(편집 → 이름 → 미리보기 → `y`), customized built-in 이름 거부 `readQualityToolPresetName` L150-177, 분리 후보 `withQualityToolPreset` L207-231.
- `quality_tool_persist.go:20-57` `saveQualityToolPreset`: description `<label> custom agent tiers`(L39-42), `saveQualityBlock`(L43)으로 프리셋과 `quality.providers.<tool>`을 한 번에 쓴다. raw decode, alias·merge 거부, placeholder guard, 블록 splice가 모두 이 writer 안에 있다.

### 등급과 실제 모델 투영
- `pkg/config/quality_tier.go`: Claude slug 상수 L8-11, 등급 `qualityTiers` L53, `AgentTier` L76-91, `ClaudeAgentModel` L94-99, `ClaudeModelForTier` L103-114(등급 하나에 slug 하나).
- `pkg/config/quality_effective.go`: `EffectiveAgentTier` L11-19, `ClaudeAgentProfile` L28-34(표준 배치면 정확한 ID, 아니면 등급 alias), `ClaudeAgentEffort` L48-65(mode와 등급만 본다: fable max, opus high·ultra면 max, sonnet medium·ultra면 high, haiku 없음).
- `pkg/config/quality_provider.go`: `NormalizeQualityProvider` L15-24(claude, codex만), `EffectiveMode` L56-66(override → default → balanced), `ForProvider` L79-84.
- `pkg/config/codex_profile.go`: 모델 상수 L6-13. `CodexAgentProfile` L105-124는 표준 배치가 아니면 fable→`gpt-6-astra`·max, opus→`gpt-6.1-sol`·xhigh, sonnet→`gpt-6-luna`·max, haiku→`gpt-6-luna`·선언 effort다(L114-123). ultra는 opus 미만을 opus로 올린다(L110-112). `codexQualityMode` L156-161은 `ForProvider(codex)`를 본다.
- `pkg/config/native_balanced.go:9-31` `NativeBalancedAgentCandidate`: provider의 mode가 `balanced`일 때만 표준 배치다. `pkg/config/role_model_policy_balanced.go`: `balancedRungByAgent` L21-40, `balancedRungCandidates` L46-69. Codex 표준 배치(top `gpt-6-astra` max, 구현·routine `gpt-6-luna` max)는 같은 등급의 사다리 값과 같다. Claude 표준 배치(`claude-sonnet-5-5` max/high)는 사다리 값(`sonnet` medium)과 다르다.
- 선언 effort(`content/agents/*.md`의 `effort:`, `pkg/content/agent_transformer_mapping.go:59-70` `MapEffort`): architect, deep-worker, planner, security-auditor, spec-writer = max, reviewer = high, 나머지 10개 = medium(2026-10-12 grep).
- `pkg/adapter/codex/codex_resolve.go:21-31` `ResolveAgentProfile`: 표와 Codex 템플릿이 같이 부르는 해석 함수다.

### huh, bubbletea, termenv (`go.mod`: huh v1.0.0, bubbletea v1.3.6, lipgloss v1.1.0, termenv v0.16.0)
- `internal/cli/tui/wizard_steps.go`: `RunInitWizard` L41-74가 단계마다 `form.Run()`(L66)을 부르고, Windows의 huh hang을 `WT_SESSION`·`TERM_PROGRAM`으로 피한다(L54-60). quality 단계 `buildQualityStep` L161-172는 `WithTheme(AutopusTheme()).WithWidth(bannerWidth + 10)`이다. 테마 `wizard_styles.go:12-53`은 `huh.ThemeCharm()` 위에 브랜드 색을 덮는다.
- huh v1.0.0 `form.go`: `WithAccessible` L237-240, `WithOutput` L332-336, `WithInput` L340-344, `RunWithContext` L662-678(accessible이면 `runAccessible` L703-718). 폭을 정하지 않으면 터미널 폭을 쓴다(L533-540). `field_select.go:712-744` accessible Select는 `N. label` 목록을 쓰고 숫자를 읽으며, 묶인 값이 기본 선택이다. `internal/accessibility/accessibility.go`의 `PromptString`은 질문마다 `bufio.NewScanner(in)`을 새로 만들고(L138) EOF면 기본값을 돌려준다(L147-152). 중단 키는 Ctrl+C 하나이고(`keymap.go:109`) 그때 `ErrUserAborted`(form.go L51, L691)를 돌려준다.
- `tui/style.go:22-33` `EnsureSafeEnv`는 stdout이 TTY가 아닐 때만 ASCII renderer를 둔다.

## S21 실제 터미널 smoke 관찰 (SPEC-MODELUX-001)

원본은 `<session-scratchpad>/s21-raw-v1.txt`(`script` 출력), ANSI를 지운 사본은 `s21-stripped.txt`다. 실행 환경은 `TERM_PROGRAM=Orca`, `TERM=xterm-256color`, locale en이었다.

| # | 관찰 | 위치 | 원인 |
|---|---|---|---|
| O1 | 줄 프롬프트에 화살표 키가 `^[[C^[[D`로, 한글 IME 글자가 그대로 찍혔다 | stripped L150 | 줄 driver는 cooked mode 한 줄 읽기다(`ask` L219-228). 사용자는 화살표와 Enter를 원한다 |
| O2 | 그룹 질문의 선택지가 `1) fable 2) opus 3) sonnet 4) haiku`뿐이었다 | stripped L151-163 | `tierChoices` L114-120. 등급 이름이 Claude 모델로 읽혔다(결함 4) |
| O3 | Codex와 OMP를 따로 고를 수 없었다 | 사용자 피드백 | 구성 단계는 전역 등급만 바꾼다. Codex는 `auto quality`의 줄 편집기, OMP는 그 안의 OMP 편집기로만 닿는다 |
| O4 | update가 antigravity-cli, opencode, omp를 감지해 더했는데 표는 Claude Code와 Codex 열뿐이었다 | stripped L219-241 | 표는 `cfg.Platforms`를 쓰고(`buildComposeTable` L91-95), 감지 플랫폼은 구성 단계 뒤 `updateDetectedHarnessPlatforms`에서야 더해진다(update.go L255-257) (결함 1) |
| O5 | update 뒤 autopus.yaml SHA가 바뀌었다 | stripped L272-274 | 감지 플랫폼 저장(`config.Save`, platform_update.go L64)이다. 구성 단계는 쓰지 않았다(`No changes; nothing written.` L256). S21 oracle이 틀렸다(결함 2) |
| O6 | `ESC ] 11 ; ? ESC \` 뒤에 `ESC [ 6 n`이 init과 update 맨 앞에 있다 | raw 4행(byte 314), 219행(byte 17605) | 아래 `## OSC 11 조사`(결함 3) |

## OMP 조사 결과

1. OMP 모델은 quality 등급과 다른 체계다. `role_model_policy`의 profile(built-in `balanced`·`ultra` 또는 `profiles`에 쓴 명시 profile), 계열(`family`: anthropic, openai), native agent pin(`agents.<name>.candidates`)으로 정해진다. 독립성은 의도된 설계다(`quality_omp_policy.go:16-17`).
2. 지정 단위는 정규 에이전트 16개가 아니라 OMP native agent 5개다(scout, reviewer, security-reviewer, task, sonic). 13개 역할이 `task` 하나로 접히므로(`omp_native_agents.go:12-27`) 역할 그룹 4개를 OMP에 그대로 옮길 수 없다. 그룹 하나를 고쳐도 대부분 `task` 한 칸만 바뀐다.
3. 고를 수 있는 모델은 설치된 OMP catalog의 실제 selector(`provider/model`)와 thinking 단계다(`installedOMPCustomModelChoices` L133-159). pin의 계열은 선언되거나 catalog가 증명해야 한다(`applyOMPProfileAgentAssignments` L200-205).
4. 실제로 돌 route는 `planOMPProfile`이 catalog로 해석한 `EffectiveSelector`·`EffectiveThinking`이다(`platform_omp_profile_preview.go:140-154`). 쓸 수 없는 route는 blocker이고 apply 전에 막힌다(`runOMPQualityInteractive` L90-92, `applyOMPProfile` L179-181).
5. 기존 쓰기 경로 `applyOMPProfile`은 파일 snapshot을 열고(`loadOMPProfileConfig`), placeholder가 든 role policy를 거부하고(`validateOMPProfileSource`), `role_model_policy`만 typed 값으로 바꿔 문서를 다시 encode하고(`replaceAutopusConfigSection` L88-135, 늘어난 placeholder 거부 L131-133), OMP adapter Update로 활성화하며(`platform_omp.go:20-32`), 활성화가 실패하면 원본 bytes로 되돌린다(L198-222).
6. 이 경로는 파일에 `omp` 플랫폼이 이미 있어야 한다(`omp_platform_not_configured`, L153-156). 그래서 init(파일이 아직 없다)과 update에서 이번에 처음 감지된 OMP에는 쓸 수 없다.
7. `applyOMPProfile`은 자기가 읽은 cfg로 쓰고 활성화한다. 호출한 명령의 cfg는 바뀌지 않는다. update는 구성 단계 뒤에 cfg 전체를 `config.Save`하고(update.go L224, platform_update.go L64) 같은 cfg로 OMP를 다시 생성하며(L236), `auto quality --apply`도 같은 cfg로 다시 생성한다(`applyQualityHarnessWith` L78-89). 동기화하지 않으면 방금 쓴 OMP 선택이 같은 run에서 옛 값으로 덮인다.
8. `replaceAutopusConfigSection`은 node tree 전체를 `yaml.Marshal`하므로(L120) 손으로 쓴 파일의 서식이 바뀔 수 있다(SPEC-MODELUX-001 probe A1과 같은 현상). `auto platform omp profile apply`와 기존 OMP 편집기가 이미 그렇게 쓴다.
9. 기존 OMP 편집기의 단계(profile → 계열 → pin → 요약 → `[y/N]` → apply)는 test로 고정돼 있다(`quality_omp_interactive_test.go`, `quality_omp_custom_test.go`, 가짜 runner `ompCLIFakeRunner`와 catalog fixture `ompCLIBalancedCatalogJSON`).

결론(최소 안전 다리): OMP 탭은 기존 OMP 편집기의 판단을 huh 폼으로 옮긴 것이다. 같은 함수(`newOMPProfileApplyOptions`, `installedOMPCustomModelChoices`, `requireOMPCustomCapability`, `planOMPProfile`, `renderOMPQualitySummary`, `applyOMPProfile`)를 그대로 부른다. 새로 더하는 판단은 쓰기 뒤 `cfg.RoleModelPolicy` 동기화와 탭 노출 조건뿐이다. catalog 확인, 계열 증명, placeholder 거부, 활성화 rollback은 바꾸지 않는다.

## OSC 11 조사 (결함 3)

- 보내는 곳: bubbletea v1.3.6 `tea_init.go:7-22`의 package `init()`이 `lipgloss.HasDarkBackground()`를 부른다(주석: "This workaround will be removed in v2"). lipgloss v1.1.0 `renderer.go:138-151`이 termenv `HasDarkBackground`를 한 번 부르고, termenv v0.16.0 `termenv_unix.go:234-293` `termStatusReport`가 `OSC 11;?` + ST(L269)와 `CSI 6n`(L272)을 TTY에 쓰고 echo를 끈 채 응답을 읽는다(timeout 5초, L18). stdout이 TTY가 아니거나, `CI`가 비어 있지 않거나(termenv.go L28-40), `TERM`이 screen·tmux·dumb이면 보내지 않는다(L237-240).
- 범위: huh를 쓰는 `auto` 바이너리 전체가 시작할 때마다 보낸다. 구성 단계와 SPEC-MODELUX-001 이전부터 있던 동작이다.
- 보이는 증상: 응답하는 터미널에서는 termenv가 응답을 읽어 화면에 남지 않는다. transcript에는 echo된 응답이 없다. `]11;?\`가 보이는 경우는 OSC를 해석하지 않는 터미널이나 CSI만 지우는 transcript 보기다. 사용자의 Orca 터미널이 실제로 그 글자를 그렸는지는 미확인이다.
- 프로그램 안에서 막을 수 없는 이유:
  1. 초기화 순서: `GODEBUG=inittrace=1 auto version`(2026-10-12)에서 `github.com/muesli/termenv` → `github.com/charmbracelet/lipgloss` → `github.com/charmbracelet/bubbletea`가 연달아 초기화된다(211-213번째). Go 1.21+는 import 경로 순서로 다음 package를 고르므로 `github.com/insajin/...` package가 그 사이에 끼어 배경색을 먼저 정할 수 없다.
  2. bubbletea를 고친 사본으로 `replace`하면 `go install ...@version` 설치가 replace 지시문 때문에 막힌다(Go 도구 규칙, 이 저장소에서 실행 확인은 미확인). 1인 메인테이너가 fork를 유지해야 한다.
  3. `lipgloss.SetHasDarkBackground`로 고정하면 huh 테마의 adaptive 색이 밝은 배경 터미널에서 틀어진다(`AutopusTheme`가 `ThemeCharm`을 바탕으로 한다, wizard_styles.go:13).
- 결정: 제품 코드는 바꾸지 않고 기록한다. 이 SPEC은 터미널 query를 하나도 더하지 않는다(REQ-MUX2-02). 수동 smoke 기록은 OSC와 CSI를 지운 뒤 읽는다(REQ-MUX2-19). 근본 해결은 huh·bubbletea v2 이전이다(Evolution Idea).

## Probe P1: 감지 플랫폼이 있는 update의 quality 블록 (결함 2)

- 명령(2026-10-12, 이 기계, 감지 대상 CLI 설치됨): `CODEX_HOME=<scratch> auto init --yes --platforms claude-code,codex` 뒤 `auto update --yes`. 바이너리는 HEAD 44773355 빌드(`<session-scratchpad>/mux-bin/auto`), 스크립트 `<session-scratchpad>/mux2-probe/run.sh`, project `mux2-probe/proj.4m2v`.
- 결과: update가 `+ 새 플랫폼 감지: antigravity-cli, opencode, omp`를 출력했다. 파일 SHA는 바뀌었다(`b7f6c3aa…` → `5af0a6e9…`). top-level `quality:` 블록 bytes는 같았다(`a8ff97ba…` 둘 다). 파일 diff는 `platforms`에 3줄(`- antigravity-cli`, `- opencode`, `- omp`)이 더해진 것뿐이다.
- 의미: S21의 "SHA가 같다"는 감지가 있으면 틀린 oracle이다. "quality 블록 bytes가 같고, 파일 diff는 감지 목록의 `platforms` 추가뿐이다"가 맞는 oracle이다. 이번 probe는 `--yes`(비대화형, 구성 단계는 안내만)로 돌렸다. 대화형 유지 경로가 쓰지 않는다는 것은 SPEC-MODELUX-001 A2 증거다.

## 설계 결정

| 결정 | 선택 | 기각한 대안 | 근거와 trade-off |
|---|---|---|---|
| 입력 방식 | stdin·stdout이 모두 터미널이고 명령 입력이 프로세스 stdin이며 wizard의 Windows guard를 통과하면 huh 폼, 그 밖은 기존 줄 driver | 줄 driver를 없애고 huh만 쓰기 | 기존 scripted test와 pipe 사용(`auto update \| tee`)이 그대로 돈다. huh를 TTY가 아닌 stdout에 그리면 escape가 파일로 간다. 두 driver를 유지하는 비용이 생긴다 |
| 폼 test | 폼 실행 하나를 `[NEW] composeRunForm` 변수로 둔다. test는 huh accessible mode(`WithAccessible(true).WithInput.WithOutput`)로 진짜 field를 돌린다 | PTY 라이브러리, 값을 직접 넣는 가짜 runner | 새 의존성이 없고 option 목록, 기본값, 값 binding을 실제 huh가 처리한다. accessible `PromptString`은 질문마다 Scanner를 새로 만들어 앞을 읽으므로 test 입력은 한 번에 1byte씩 주는 reader(`testing/iotest.OneByteReader`)로 준다. 실제 터미널 렌더링은 수동 smoke가 닫는다 |
| 폼 모양 | 질문마다 field 하나인 form, `tui.AutopusTheme()`, 폭은 정하지 않음 | wizard처럼 `WithWidth(50)` | 실제 모델 라벨이 50칸을 넘는다. 폭이 0이면 huh가 터미널 폭을 쓴다(form.go L533-540). 색과 테두리는 wizard와 같다 |
| 질문 구조 | 메뉴 → 도구 탭 → 방식(역할 그룹·에이전트별) → 편집 → 미리보기 → 확인. 적용이나 취소 뒤에는 표를 다시 그리고 메뉴로 돌아간다 | 한 번 적용하면 끝(SPEC-MODELUX-001 줄 driver) | 한 run에서 Claude, Codex, OMP를 차례로 정할 수 있다. 유지 경로는 여전히 Enter 1회다. 쓰기는 도구마다 따로 확인한다 |
| 도구 탭 | `모든 도구 같게` + 설치된 Claude Code, Codex, OMP. OpenCode·Antigravity는 고르지 않는다 | 플랫폼 5개 모두 | 둘은 에이전트 파일에 model이 없다(`quality_tool_targets.go:33-36`). 고를 수 있게 하면 아무도 읽지 않는 설정을 쓴다 |
| `모든 도구 같게` | SPEC-MODELUX-001의 상대 등급, Binding Contract, `quality.default` 그대로. 선택지 라벨에 도구별 실제 칸을 붙인다 | 등급 이름만(현재), 도구별 값을 따로 저장 | 결함 4는 표시 문제다. 저장 의미를 바꾸면 SPEC-MODELUX-001 승인 범위를 다시 연다 |
| 라벨 계산 | 선택지 하나를 작업 구성에 적용한 후보 config의 표 칸을 run snapshot으로 계산한다 | 등급→모델 고정 표를 따로 두기 | 표와 같은 해석 함수(`ClaudeAgentProfile`, `codex.ResolveAgentProfile`)를 쓰므로 라벨, 미리보기, 생성이 같은 값을 낸다. 같은 등급이라도 binding(표준 배치, custom)에 따라 값이 다르다(C6) |
| Claude 라벨의 모델 | `ClaudeAgentModel`(slug, `claude-` 제거)과 `ClaudeAgentProfile`의 effort | 표 칸과 같은 frontmatter 값(alias) | 사용자가 정한 표시(claude-opus-5-5 등)다. 기존 도구별 편집기의 Model 열도 slug다(`qualityToolModel` L90-95). 표 칸은 SPEC-MODELUX-001 C16대로 alias와 정확한 ID를 구별해 그대로 둔다. 라벨과 표의 표기 차이는 문서로 알린다 |
| 도구별 저장 값 | 상대 등급을 저장하고 실제 이름을 보여 준다 | provider 모델 ID를 에이전트마다 저장 | Claude는 등급과 slug가 1:1이고 effort는 (mode, 등급)으로 정해진다(`quality_tier.go:103-114`, `quality_effective.go:48-65`). Codex의 실제 선택지 {astra·max, 6.1-sol·xhigh, luna·max, luna·선언 effort}는 모두 어떤 등급의 결과다(`codex_profile.go:114-123`). 선언 effort가 max인 5개 에이전트는 Codex에서 sonnet과 haiku가 같은 값이라 두 선택지가 같은 라벨에 등급 표시만 다르다. 새 key가 없으므로 strict decode(v0.50.113+)도 깨지지 않는다 |
| 도구별 binding | `quality.presets.<provider>-custom`(16개) + `quality.providers.<provider>`, 쓰기는 기존 `saveQualityToolPreset`. init은 메모리 cfg에 같은 두 key | 프리셋 이름을 묻기, `custom-codex` | 기존 도구별 편집기의 기본 이름(`qualityToolPresetName` L126-128)과 description을 그대로 쓴다. SPEC-MODELUX-001 fixture P와 S12가 `codex-custom`, `claude-custom`을 쓴다. 같은 도구에 프리셋이 둘 생기지 않는다. 이름 질문을 빼서 입력 횟수를 줄인다 |
| 오래된 바이너리 | 새 key 없음. `quality.providers`는 v0.50.88부터 읽고 그 이전 바이너리는 무시한다(SPEC-MODELUX-001 C11). 미리보기에 그 사실을 한 줄로 쓴다 | 도구별 값을 `quality.default`로 우회 | 모든 출하 바이너리가 파일을 읽는다. 대신 v0.50.88 이전 auto를 쓰는 팀원에게는 도구별 선택이 적용되지 않는다 |
| OMP 다리 | 기존 OMP 편집기 판단을 huh로 옮기고 기존 plan·apply 경로로만 쓴다. 쓰기 뒤 `cfg.RoleModelPolicy`를 파일에서 다시 읽어 맞춘다 | OMP를 quality 등급에 묶기, 새 OMP writer | OMP 조사 1-9. catalog, 계열 증명, placeholder, rollback 규칙을 그대로 쓴다. 대가: OMP 쓰기는 문서 전체를 다시 encode한다(기존 동작). update에서는 OMP가 활성화와 생성 루프에서 두 번 생성된다 |
| OMP 노출 | `auto update`와 `auto quality`에서 `omp`가 파일에 설정된 플랫폼일 때만 | init과 감지 직후 update에도 | apply 경로는 파일의 `omp` 플랫폼을 요구한다(OMP 조사 6). init용 메모리 경로는 OMP adapter 실행 seam이 확인되지 않아(미확인) 범위를 넓힌다. init 뒤 `auto quality`에서 지정하라는 안내를 쓴다 |
| 감지 플랫폼 | update의 표·도구 탭·snapshot은 설정된 플랫폼 + 감지 플랫폼 view로 계산하고, 쓰기와 동기화는 읽어 온 cfg에만 한다. 감지 플랫폼 생성도 같은 snapshot | 구성 단계 전에 cfg.Platforms에 바로 더하기 | 뒤의 `updateDetectedHarnessPlatforms`가 플랫폼마다 더하고 저장하는 순서를 바꾸지 않는다. view 사본이 쓰기 대상이 되면 확인한 구성이 뒤 `config.Save`에서 사라진다 |
| 감지 정보 전달 | `composeGate`에 `[NEW] Detected` field를 더한다. `updateComposeStep`의 시그니처는 그대로 | `updateComposeStep` 인자 추가 | 이 변수를 감싸는 기존 test(미확인)를 고치지 않는다. update.go 줄 수가 늘지 않는다 |
| Ctrl+C | 폼이 `ErrUserAborted`를 돌려주면 이 단계는 더 쓰지 않는다. init은 wizard처럼 `init cancelled.`로 끝나고 파일을 만들지 않는다. update는 유지로 보고 계속 생성한다. quality는 끝난다 | 모든 명령을 중단 | SPEC-MODELUX-001은 "이 단계 때문에 update가 실패하지 않는다"를 약속했다(REQ-MUX-13). 계속 생성해도 승인하지 않은 쓰기는 없다. init은 아직 아무것도 쓰지 않았으므로 wizard와 같게 멈추는 편이 사용자 기대에 맞다 |
| update 도움말 | `Long` 문자열을 `[NEW] internal/cli/update_help.go`로 옮기고 문장을 더한다 | update.go 안에서 고치기 | update.go가 299줄이다. 옮기면 13줄이 빈다. 기존 문장은 지우지 않는다 |
| OSC 11 | 기록만 하고 새 query를 더하지 않는다 | init 순서 조작, bubbletea fork, 배경색 고정 | 위 `## OSC 11 조사` |

## Plan Intent Ledger

Clarification Ledger unavailable. 이 SPEC의 입력은 planning context의 "Decisions made with the user (answered — do not re-ask)" 표다. 다시 묻지 않았다. 셀은 근거로만 요약했다.

| Field | Status | SPEC 반영 |
|---|---|---|
| input (모든 대화형 단계를 huh 폼으로, 줄 driver는 비TTY·scripted용으로 유지, test seam) | answered | REQ-MUX2-01, 02, 03, 08, 09, 15, 17, 18 |
| per-tool (도구 탭, 도구별 실제 모델 이름, OpenCode·Antigravity 제외, `모든 도구 같게` = SPEC-MODELUX-001 동작) | answered | REQ-MUX2-04, 05, 06 |
| storage (기존 `quality.providers.<tool>` + `quality.presets.<preset>.agents`, 새 key 없음, 이름과 저장 값은 근거로 정함) | answered | REQ-MUX2-07, `## 설계 결정`의 이름·저장 값 행 |
| OMP (이번에 지정 가능, 기존 OMP writer·catalog·placeholder 규칙 재사용) | answered | REQ-MUX2-10, 11, `## OMP 조사 결과` |
| release (v0.50.125, PR #196과 함께, 범위를 좁게, 새 secret·승인·유료 CI 없음) | answered | header, `## Outcome Lock` |
| 추가 결함 1-4 | answered (요구사항으로 넣으라는 지시) | REQ-MUX2-12, 13, 19, 05 |

## Question Audit
- question_transport: none (planning context가 결정을 넘겼다) · question_count: 0 · unresolved_fields: init에서 OMP 지정(가정: 범위 밖. 틀리면 메모리 경로와 OMP adapter seam task가 하나 늘어난다)

## Outcome Lock
- User-visible outcome: 실제 터미널에서 `auto init`, `auto update`, `auto quality`의 구성 단계를 화살표와 Enter로 다룬다. 모양은 init wizard와 같다. 도구 탭에서 `모든 도구 같게`, Claude Code, Codex, OMP를 고르고, 각 선택지에는 그 도구의 실제 모델과 effort가 보인다. Codex 결과는 `quality.providers.codex: codex-custom`(Claude는 `claude-custom`)으로 저장되고 같은 run의 생성 파일이 승인한 미리보기와 같다. OMP는 설치된 catalog에서 native agent별로 고르고, 실제 route 미리보기를 확인한 뒤 기존 OMP writer로 저장된다. update의 표에는 이번에 감지해 생성할 플랫폼 열도 나온다. 유지하면 quality 블록과 `role_model_policy`가 byte 단위로 그대로다. 줄 입력(scripted, 비TTY)은 SPEC-MODELUX-001과 똑같이 동작한다.
- Mandatory requirements: REQ-MUX2-01 ~ REQ-MUX2-17 (Must).
- Explicit non-goals: `spec.md` `## Outcome Boundary` 목록과 같다.
- Completion evidence: Must S1–S15, S20 PASS, 기존 test 수정 0건, `auto eval harness run`, 신규·변경 package coverage 85% 이상, 신규·변경 소스 파일 300줄 이하, `GOOS=windows GOARCH=amd64 go build ./...`, `evidence/manual-smoke-002.txt`, `auto spec validate --strict` 통과.

## Visual Planning Brief
흐름도와 화면 wireframe은 `plan.md` `## Visual Planning Brief`에 있다. 모두 `wireframe intent: assumed`다. 사용자가 confirm하기 전이므로 라벨 문구와 배치는 risk와 reviewer focus로 둔다. 요구사항으로 묶는 것은 Label Contract(값의 원천과 순서)뿐이다.

## Technology Stack Decision
| Mode | Selected stack | Resolved versions | Source refs | Checked at | Rejected alternatives |
|---|---|---|---|---|---|
| brownfield | 기존 Go 모듈, huh(init wizard가 이미 쓰는 폼), `testing/iotest`, 기존 quality·OMP writer | `go.mod`: huh v1.0.0, bubbletea v1.3.6, lipgloss v1.1.0, termenv v0.16.0. 새 의존성 없음, 버전 변경 없음 | `go.mod`, module cache 소스 | 2026-10-12 | huh·bubbletea v2 이전(범위 밖), PTY test 라이브러리 |

## Minimality Decision Matrix
| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | 사용자 피드백(화살표·Enter, Codex·OMP 지정 불가), smoke 관찰 O1-O6 | proceed | 폼 driver, 도구 탭, 도구별 저장, OMP 탭, 감지 열, oracle 수정 |
| existing code/helper/pattern | `tui.AutopusTheme`, wizard의 Windows guard, SPEC-MODELUX-001 planner·표·snapshot·writer, `withQualityToolPreset`, `saveQualityToolPreset`, `qualityToolPresetName`, OMP 편집기 함수 전부, `driftAdapter`, `terminalSafe` | reuse | 판단과 쓰기는 모두 기존 함수다 |
| stdlib/native | `testing/iotest.OneByteReader`, `os`, `slices`, `maps` | use | accessible test 입력, view 사본 |
| existing dependency | huh v1.0.0(Select, Confirm, accessible mode, `ErrUserAborted`) | reuse | 화살표·Enter 입력 |
| new dependency or new abstraction | 새 의존성 0. 새 abstraction은 폼 seam 2개(`composeFormsProbe`, `composeRunForm`), 폼 driver, 라벨 계산, 도구별 planner, OMP 탭, 플랫폼 view | accepted | 각각 REQ-MUX2-01·17, 03, 05·06, 07, 10·11, 12의 실제 요구다. 줄 driver를 고치지 않는다는 결정이 별도 driver를 요구한다 |
| minimum sufficient verification | accessible mode로 도는 실제 명령 test, 실제 adapter `Generate` 비교, catalog fixture CAT-A·B, OMP 가짜 runner, 기존 test 무수정, 수동 smoke 1회 | required checks | security(raw writer와 OMP placeholder 규칙 재사용), data-loss(유지 경로 불변, cfg 동기화), deterministic-oracle(구체 라벨과 칸 값) gate를 줄이지 않는다 |

## Semantic Invariant Inventory
| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-MUX2-01 | "keep-path leaves the quality block bytes unchanged" | 유지·취소·중단·변경 없음은 quality 블록과 `role_model_policy` bytes를 바꾸지 않는다. 감지가 있으면 바뀌는 것은 `platforms` 추가뿐이다 | autopus.yaml 블록 bytes, 파일 diff | S2, S7, S9, S11, S12, S20 |
| INV-MUX2-02 | "choices are shown with that tool's REAL model names" | 선택지 라벨 = 그 선택이 만드는 후보의 표 칸(같은 snapshot) | 라벨 문자열 | S4, S5, S6, S14 |
| INV-MUX2-03 | "per-tool result uses the existing per-provider binding" | 도구별 쓰기는 `quality.presets.<p>-custom`과 `quality.providers.<p>`만 바꾼다. 다른 열의 raw 칸은 그 프리셋을 같이 쓰지 않는 한 그대로다 | quality key 경로, 표 `*` | S5, S6 |
| INV-MUX2-04 | "generated files equal the approved table" | 같은 run의 Claude·Codex 생성 값 = 승인한 미리보기 칸(감지 플랫폼 포함) | 생성 파일 `model`·`effort` | S5, S6, S10, S14 |
| INV-MUX2-05 | "keep OMP catalog/attestation rules intact" | OMP 쓰기는 기존 apply 경로로만, 막히면 쓰기와 활성화 0, 쓴 뒤 cfg 동기화 | role_model_policy, 활성화 횟수, 다음 표 | S8, S9 |
| INV-MUX2-06 | "line driver unchanged" | 줄 입력의 프롬프트, 키, 출력 bytes가 HEAD 44773355와 같다 | 출력 bytes, 기존 test | S1, S13 |
| INV-MUX2-07 | "non-interactive never reads stdin" | 비대화형과 preview는 폼을 열지 않고 stdin을 읽지 않는다 | 폼 실행 횟수, Read 횟수 | S1 |
| INV-MUX2-08 | "one Codex catalog snapshot per run" | 메뉴 반복과 감지 플랫폼 생성까지 catalog probe는 run당 1회 | probe 횟수 | S2, S10 |
| INV-MUX2-09 | "table/tabs must reflect the platforms the update will generate" | 표 열 = 설정 + 감지(고정 순서). OpenCode·Antigravity는 탭에 없다 | 열 머리말, 탭 선택지 | S3, S10 |
| INV-MUX2-10 | "older binaries must still load the file" | 새 YAML key가 없다. legacy strict decode가 통과한다 | quality key, decode | S5 |
| INV-MUX2-11 | "stray ]11;?\" | 폼은 터미널 query를 더하지 않고 YAML·catalog 글자는 escape한다 | 소스 검사, 라벨 bytes | S15, S18 |

## Feature Coverage Map
| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| 입력 방식과 줄 driver 불변 | REQ-MUX2-01, 15, 17 / S1, S13 | covered |
| 메뉴, 도구 탭, 방식, 편집, 미리보기, 확인 | REQ-MUX2-03, 04, 08, 09 / S2, S3, S7 | covered |
| 실제 모델 라벨(결함 4) | REQ-MUX2-05, 06 / S4, S5, S6, S14 | covered |
| Claude·Codex 도구별 저장과 생성 | REQ-MUX2-07, 16 / S5, S6, S14 | covered |
| OMP 지정 | REQ-MUX2-10, 11 / S8, S9, S20 | covered |
| 감지 플랫폼(결함 1)과 유지 경로(결함 2) | REQ-MUX2-12, 13 / S10, S11, S20 | covered |
| error/recovery: Ctrl+C, blocker, 거부, 취소 | REQ-MUX2-11, 14 / S9, S12 | covered |
| 터미널 안전과 OSC 11(결함 3) | REQ-MUX2-02, 19 / S15, S18 | covered (19는 Should) |
| override 폼, locale, 문서 | REQ-MUX2-18, 20, 21 / S16, S17, S19 | covered (Should) |

## Completion Debt
| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

작성 시점에 알려진 debt는 없다. 폼 seam, 도구별 저장, OMP 다리와 cfg 동기화, 감지 열과 snapshot 전달, 수동 smoke 기록 중 하나라도 빠지면 debt가 되어 sync 완료를 막는다.

## Evolution Ideas

These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| huh·bubbletea v2 이전으로 시작 시 OSC 11 query 제거 | 의존성 major 이전이라 이번 출하 범위를 넘는다 | 사용자가 터미널 잔여 글자를 다시 보고할 때 |
| init에서 OMP 지정(메모리 경로) | OMP apply는 파일을 요구하고 OMP adapter test seam이 미확인이다 | init 직후 OMP 지정 요청이 들어올 때 |
| 여러 도구를 한 번의 쓰기로 묶기 | 메뉴 반복으로 도구마다 적용할 수 있다 | 한 번의 확인으로 묶어 달라는 요청 |
| 도구별 결과가 최신 balanced·ultra와 같으면 built-in 이름으로 묶기 | 표준 배치를 지키는 최적화이고 Binding Contract 행이 늘어난다 | 도구별 custom이 표준 배치를 잃는 문의 |
| OMP pin 해제(inherit) 선택지 | 기존 `auto platform omp profile apply --agent <a>=inherit`가 있다 | UI에서 해제를 찾는 문의 |
| 표 OMP 칸에 catalog 반영 실제 route | 표를 그릴 때마다 `omp` subprocess가 필요하다 | OMP 탭 밖에서도 실제 route를 보고 싶다는 요청 |

## Sibling SPEC Decision
| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | 같은 구성 단계의 입력 방식, 도구 탭, 저장, 표가 한 흐름이다. task 11개, 소스 파일 약 24개(test 포함)로 sibling 기준(25개 초과 task이면서 40개 초과 파일)에 못 미친다. 같은 PR #196로 출하한다 | None |

## Reference Discipline
| Reference | Type | Verification |
|-----------|------|--------------|
| `internal/cli/` `quality_compose_*.go`, `update_compose.go`, `init_compose.go`, `update.go`, `init.go`, `quality.go`, `platform_update.go`, `platform_lifecycle.go`, `platform_catalog.go`, `doctor_drift_content.go`, `init_helpers.go`, `prompts.go`, `quality_tool_*.go`, `quality_omp_*.go`, `platform_omp*.go`, `terminal_text.go`, `tui/wizard_steps.go`, `tui/wizard_styles.go`, `tui/style.go` | existing | Read, `wc -l`(2026-10-12) |
| `pkg/config/` `quality_tier.go`, `quality_effective.go`, `quality_provider.go`, `codex_profile.go`, `native_balanced.go`, `role_model_policy_balanced.go`, `role_model_policy_builtin.go`, `role_model_policy_native_agents.go`, `omp_native_agents.go`, `loader.go`, `pkg/adapter/codex/codex_resolve.go`, `pkg/content/agent_transformer_mapping.go`, `content/agents/*.md` | existing | Read, grep |
| huh v1.0.0 `form.go`, `field_select.go`, `field_confirm.go`, `keymap.go`, `internal/accessibility/accessibility.go`; bubbletea v1.3.6 `tea_init.go`; lipgloss v1.1.0 `renderer.go`; termenv v0.16.0 `termenv_unix.go`, `termenv.go`, `output.go` | existing (module cache) | Read |
| `<session-scratchpad>/s21-raw-v1.txt`, `mux2-probe/run.sh`, `mux2-probe/proj.4m2v` | existing (evidence) | grep -b, probe P1 실행 |
| `composeFormsProbe`, `composeRunForm`, `composeGate.Detected`, `updateComposeOMPDeps`, `updateDetectedHarnessPlatformsWith`, `quality_compose_form*.go`, `quality_compose_tool.go`, `quality_compose_omp.go`, `update_help.go`, `evidence/manual-smoke-002.txt` | [NEW] planned addition | 부재 확인(grep, 2026-10-12) |

## Reviewer Brief
- Intended scope: 대화형 구성 단계의 huh 폼 driver(세 진입점), 도구 탭과 실제 모델 라벨, Claude·Codex 도구별 저장(기존 `quality.providers` binding), 기존 OMP writer로 가는 OMP 탭, update 표의 감지 플랫폼 열과 snapshot 전달, 유지 경로 oracle 수정, OSC 11 기록.
- Explicit non-goals: 줄 driver 변경, 새 YAML key, OMP를 quality 등급에 묶기, 표 OMP 칸의 실제 route, init의 OMP 지정, 여러 도구 일괄 쓰기, 의존성 major 이전, OMP writer의 전체 재encode 변경.
- Self-verified: Traceability Matrix, invariant 11개와 구체 값, OMP 조사 1-9, probe P1, 결함 1-4의 원인 위치.
- Reviewer should focus on: OMP 쓰기 뒤 cfg 동기화가 update와 `--apply`의 뒤 저장·생성을 모두 덮는지, 감지 view가 쓰기 대상이 되지 않는지, 라벨이 표와 같은 해석 함수를 쓰는지, 폼 경로가 비대화형에서 열리지 않는지, Ctrl+C의 명령별 동작, 그리고 Completion Debt only.

## Open Risks (미확인)
- R1: 기존 update test가 플랫폼 감지를 고정하는지 미확인이다. 이 기계처럼 CLI가 설치된 곳에서는 감지 열이 표에 더해져 기존 표 단언이 바뀔 수 있다. T2에서 먼저 돌려 확인하고, 바뀌면 test helper에서 감지를 고정한다(기존 test 파일은 고치지 않는다).
- R2: `updateComposeStep`을 감싸는 test가 있다는 주석(update_compose.go L12-13)만 확인했다. 시그니처를 바꾸지 않는 설계로 피한다.
- R3: huh accessible mode가 여러 form을 이어 돌릴 때 1byte reader로 입력이 빠지지 않는지 실행으로 확인하지 않았다(probe A2).
- R4: OMP adapter를 test에서 실제 `omp` 없이 돌리는 seam이 있는지 미확인이다. OMP 탭 test는 기존 OMP 편집기 test처럼 활성화 stub으로 닫는다.
- R5: 사용자 터미널이 `]11;?\`를 실제로 그렸는지 미확인이다.
- R6: Claude 라벨(slug)과 표 칸(alias)의 표기 차이가 혼란을 줄 수 있다. wireframe confirm 대상이다.

## Self-Verify Summary
- Q-CORR-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 인용한 경로와 줄은 이번 세션에 Read로 확인했다. 확인하지 못한 것은 미확인으로 표시했다
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md, research.md | reason: 새 함수, 변수, 파일, evidence는 모두 [NEW]다
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: EARS 문장과 bare Given/When/Then/And 형식이다
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline이 existing, module cache, evidence, [NEW]를 나누고 생성 루트를 원천과 구별한다
- Q-COMP-01 | status: PASS | attempt: 1 | files: spec.md, plan.md, acceptance.md, research.md, prd.md | reason: 계약, 계획, oracle, 근거, 요약이 나뉘어 있다
- Q-COMP-02 | status: PASS | attempt: 1 | files: acceptance.md | reason: Must 시나리오는 실제 명령과 실제 adapter, 수동 smoke로 닫는다
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 결함 1-4를 원인 위치와 함께 요구사항이나 기록으로 옮겼다
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, acceptance.md | reason: invariant마다 구체 라벨, 칸 값, 파일 값, 횟수를 oracle로 둔다
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix가 REQ 21개를 task, 시나리오, invariant에 잇고 Reviewer Brief가 범위를 좁힌다
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt와 Evolution Ideas가 나뉘고 Evolution Ideas에 ID가 없다
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md | reason: A1은 probe P1 증거가 있는 PASS, A2·A3는 not-run 이유가 있다
- Q-FEAS-01 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: 판단과 쓰기는 기존 함수 재사용이고, 줄 수 제약(update.go 299줄)을 계획에 반영했다
- Q-SEC-01 | status: PASS | attempt: 1 | files: spec.md | reason: Trust Model에 OMP catalog 출력과 라벨 escape를 더했다. writer는 기존 보안 경로만 쓴다
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 대화형 구성 단계 하나로 모인다. sibling이 없다

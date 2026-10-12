# SPEC-MODELUX-002: 에이전트 모델 구성 UX v2 — 화살표 선택 폼, 도구별 실제 모델 탭, OMP 지정, update 감지 플랫폼 표

**Status**: approved
**Created**: 2026-10-12
**Domain**: MODELUX
**Module**: autopus-adk
**Release**: v0.50.125 (SPEC-MODELUX-001과 같은 branch `feat/agent-model-ux`, PR #196)
**Predecessor**: SPEC-MODELUX-001(이 branch에 구현됨). 역할 그룹, Table Cell Contract, Binding Contract, Update Prompt Contract, raw quality writer는 그 SPEC의 정의를 그대로 쓴다. 두 SPEC이 다르면 이 SPEC이 우선하고, 차이는 `## Related SPECs`에 적는다.
**Source PRD**: `prd.md`(같은 디렉터리). 코드 근거와 결정은 `research.md`에 있다.

## 목적

SPEC-MODELUX-001의 실제 터미널 smoke(S21) 뒤 사용자가 세 가지를 말했다. 화살표와 Enter로 고칠 수 있어야 한다. Claude만 나오고 Codex는 지정할 수 없다. OMP도 그렇다. 같은 smoke에서 결함 네 가지가 더 보였다. update가 감지해 더한 플랫폼이 표에 없다. S21의 "SHA 불변" oracle은 감지 저장 때문에 틀렸다. 시작할 때 OSC 11 query가 나간다. 그룹 선택지가 등급 이름뿐이라 Claude 전용으로 읽힌다(`research.md` `## S21 실제 터미널 smoke 관찰`).

이 SPEC은 대화형 구성 단계를 init wizard와 같은 huh 폼으로 바꾼다. 줄 입력(scripted, pipe)은 SPEC-MODELUX-001 줄 driver를 그대로 쓴다. 폼 흐름에는 도구 탭이 있다. `모든 도구 같게`는 SPEC-MODELUX-001 동작이고, 선택지마다 도구별 실제 모델과 effort를 보여 준다. Claude Code·Codex 탭은 그 도구만 바꾸고 기존 per-provider binding(`quality.providers.<p>` + `quality.presets.<p>-custom`)에 저장한다. OMP 탭은 기존 OMP 편집기의 판단을 폼으로 옮긴다. update·quality에서는 기존 OMP writer로만, init에서는 같은 plan 검사를 거친 메모리 config를 init의 저장으로 쓴다. update 표에는 이번 run이 생성할 감지 플랫폼 열도 나온다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md` `## Outcome Lock`에 있다. 요약: 실제 터미널의 `auto init`, `auto update`, `auto quality`에서 구성 단계를 화살표와 Enter로 다룬다. 도구 탭에서 `모든 도구 같게`, Claude Code, Codex, OMP를 고르고, 선택지에는 그 도구의 실제 모델과 effort가 보인다. Claude·Codex 결과는 `quality.providers.<p>: <p>-custom`으로 저장되고 같은 run의 생성 파일이 승인한 미리보기와 같다. OMP는 세 진입점 모두에서 설치된 catalog로 native agent별로 고르고 실제 route 미리보기를 확인한 뒤 저장된다(update·quality는 기존 OMP writer, init은 같은 검사를 거친 메모리 config와 init의 저장, catalog를 못 읽으면 안내 후 계속). update 표에는 감지 플랫폼 열도 있다. 유지, 취소, 실패에서는 이 단계가 아무것도 쓰지 않으므로 파일 변화는 감지와 기존 migration뿐이다. 줄 입력은 SPEC-MODELUX-001과 똑같다.
- Mandatory requirements: REQ-MUX2-01 ~ REQ-MUX2-18, REQ-MUX2-22, REQ-MUX2-23 (Must). REQ-MUX2-19 ~ REQ-MUX2-21은 Should다.
- Explicit non-goals:
  - SPEC-MODELUX-001 줄 driver의 키, 문구, 라우팅 변경과 기존 줄 편집기(도구별, OMP, 전역 메뉴) 제거
  - 새 YAML key, 에이전트별 provider 모델 ID 저장
  - 등급 편집으로 OMP profile·pin을 직접 바꾸거나 새 등급 연동을 더하기, 표 OMP 칸의 catalog 반영 route. built-in `ultra` OMP profile이 `quality.presets.ultra`에서 route를 파생하는 기존 동작은 그대로라서, ultra 프리셋을 바꾸는 쓰기는 OMP 요청 route도 바꿀 수 있고 표에 `*`로 보인다(SPEC-MODELUX-001 REQ-MUX-11)
  - update에서 이번 run에 처음 감지된 OMP 지정(init의 OMP 지정은 REQ-MUX2-22다)
  - OMP pin 해제(inherit) 선택지, 여러 도구를 한 번의 쓰기로 묶기
  - OMP writer(`replaceAutopusConfigSection`)가 문서 전체를 다시 encode하는 기존 동작 변경
  - huh·bubbletea·lipgloss major 이전, OSC 11 query 제거
  - OpenCode·Antigravity 에이전트별 모델
- Completion evidence: `acceptance.md`의 Must 시나리오(S1–S16, S20–S22) PASS, 기존 test 수정 0건, `auto eval harness run` 통과, 신규·변경 package coverage 85% 이상, 신규·변경 소스 파일 300줄 이하, `GOOS=windows GOARCH=amd64 go build ./...`, 수동 터미널 smoke 기록(`evidence/manual-smoke-002.txt`, S20), `auto spec validate --strict` 통과.

## Trust Model

- SPEC-MODELUX-001 Trust Model을 그대로 따른다. `autopus.yaml`은 반쯤 신뢰한다. writer는 loader가 펼친 값을 쓰기 원천으로 쓰지 않는다.
- 비신뢰 입력이 하나 더 있다. OMP catalog 출력(`omp models --json`)이다. 기존 정규화(`installedOMPCustomModelChoices`)를 지난 selector만 선택지가 된다. init은 이 selector를 담은 후보를 `config.Save`로 새로 쓰므로, 저장 전에 파일 경로와 같은 placeholder 검사를 한다(REQ-MUX2-22).
- 폼 라벨에 들어가는 글자 중 autopus.yaml(프리셋 이름, description, override 이름), Codex catalog, OMP catalog에서 온 것은 모두 `terminalSafe`를 거친다. huh는 라벨을 그대로 터미널에 그리기 때문이다.
- 출력은 터미널, `autopus.yaml`, 생성 파일, 수동 smoke 기록과 probe 증거다. 새 로그나 artifact는 없다. 증거 파일은 커밋 전에 `$HOME`, scratch 경로, 사용자 이름을 placeholder로 바꾼다.

## Requirements

### REQ-MUX2-01 입력 방식: 폼과 줄 driver
Priority: Must · EARS: StateDriven

WHERE the composition step asks interactively, THEN THE SYSTEM SHALL drive every prompt of the step through huh forms when the command input is the process stdin, stdin and stdout are terminals, and the terminal passes the init wizard's Windows guard, and SHALL use the SPEC-MODELUX-001 line driver otherwise.

- 판정은 Input Mode Contract다. 비대화형과 preview는 SPEC-MODELUX-001 그대로 이 단계에 들어오지 않는다(REQ-MUX-14). 폼은 그 판정 뒤의 묻는 경로에서만 열린다.
- `auto quality`가 TTY가 아닌 stdin에서 내는 기존 오류(`interactive quality selection requires a TTY; use an explicit quality or profile command`)는 그대로다.
- 폼은 판정한 스트림에 묶는다. huh `NewForm`의 기본 출력은 `os.Stderr`라서(huh form.go:112) 그대로 두면 `auto update 2>err.log`에서 폼이 파일로 간다. 그래서 runner가 `WithInput(cmd.InOrStdin())`와 `WithOutput(cmd.OutOrStdout())`를 건다(REQ-MUX2-17).

### REQ-MUX2-02 폼 표시와 터미널 안전
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL build every composition form with tui.AutopusTheme(), pass every label text that comes from autopus.yaml, the Codex catalog, or the OMP catalog through terminalSafe, and send no terminal query of its own.

- form은 질문 하나에 field 하나다. 폭은 정하지 않아 huh가 터미널 폭을 쓴다(실제 모델 라벨이 wizard의 50칸을 넘는다).
- 시작할 때 나가는 OSC 11 query는 bubbletea package 초기화의 기존 동작이다(REQ-MUX2-19). 이 SPEC의 코드는 `HasDarkBackground`, `BackgroundColor`, 새 termenv 출력을 부르지 않는다.

### REQ-MUX2-03 메뉴
Priority: Must · EARS: EventDriven

WHEN the form driver runs, THEN THE SYSTEM SHALL probe the Codex catalog once for the run, draw the composition table from that snapshot before every menu, ask a select whose default is keep, and ask the menu again after every applied, cancelled, or unchanged edit until the operator keeps.

- 선택지와 순서는 Form Flow Contract다. `최신 기본값으로 갱신`은 built-in 프리셋이 historical일 때만, `기본 프리셋 고르기`는 `auto quality`에서만 있다.
- 유지 경로의 입력은 Enter 1회다. 반복해도 catalog probe는 run당 1회이고, 같은 run의 생성은 그 snapshot을 쓴다.
- 기준 상태(baseline)는 이 run에서 마지막으로 성공한 쓰기 뒤의 파일과 cfg다. 쓰기가 없었으면 run 시작 상태다. 유지, 취소, 변경 없음, 거부, 쓰기 실패, catalog 실패, Ctrl+C는 기준 상태를 바꾸지 않고, 앞서 승인한 쓰기를 되돌리지도 않는다.

### REQ-MUX2-04 도구 탭
Priority: Must · EARS: EventDriven

WHEN the operator chooses to edit the composition, THEN THE SYSTEM SHALL ask a tool tab listing 모든 도구 같게 followed by Claude Code, Codex, and OMP under the conditions of the Form Flow and OMP Tab contracts, SHALL never list OpenCode or Antigravity, and SHALL skip the tab when it would list one option.

- OpenCode와 Antigravity는 에이전트 파일에 model이 없다. 표에는 SPEC-MODELUX-001대로 `플랫폼 기본값` 열로 남는다.

### REQ-MUX2-05 모든 도구 같게: 실제 모델 라벨
Priority: Must · EARS: EventDriven

WHEN the tab is 모든 도구 같게, THEN THE SYSTEM SHALL keep the SPEC-MODELUX-001 relative-tier composition and Binding Contract, and SHALL label every tier option with the Claude Code and Codex cells that the option's candidate binding resolves with the run's snapshot, per the Label Contract.

- 결함 4의 수정이다. 라벨은 "지금 이 선택에서 멈추면 이 에이전트들이 받는 값"이다. 다른 그룹의 작업 상태와 binding(표준 배치, custom, ultra)을 반영한다.
- 예(fixture D, CAT-A, 설계·판단 그룹): `opus — Claude Code: opus-5-5 · high | Codex: 6.1-sol · xhigh`.

### REQ-MUX2-06 도구별 탭: 시작과 라벨
Priority: Must · EARS: EventDriven

WHEN the tab is Claude Code or Codex, THEN THE SYSTEM SHALL start every agent at the tier that tool's provider view projects and SHALL label every option with that tool's cells for the candidate the option produces, followed by the stored tier, per the Label Contract.

- 시작 등급은 `quality.ForProvider(<p>).EffectiveAgentTier(agent, fallback)`다. override가 있으면 그 프리셋, 없으면 전역 binding이다.
- 예(fixture D, Codex 탭, 설계·판단): `6.1-sol · xhigh  [opus]`. 구현 그룹의 haiku는 선언 effort가 달라 `6-luna · medium / 6-luna · max  [haiku]`다.

### REQ-MUX2-07 도구별 저장
Priority: Must · EARS: EventDriven

WHEN the operator confirms a Claude Code or Codex tab edit that differs from its start, THEN THE SYSTEM SHALL persist it by the Per-Tool Binding Contract, and SHALL write nothing when the edit equals its start.

- 파일 쓰기는 기존 `saveQualityToolPreset`(SPEC-MODELUX-001 REQ-MUX-08의 `saveQualityBlock`)이다. 사전 검사, 거부 문구, 블록 밖 bytes 보존이 그대로 걸린다.
- 이 요구사항은 SPEC-MODELUX-001 REQ-MUX-09의 `quality.providers` 금지에 대한 예외다. 그 SPEC도 구성 프롬프트에서 닿는 도구별 편집기에는 예외를 두었다.

### REQ-MUX2-08 에이전트별 편집: 목록에서 고르기
Priority: Must · EARS: EventDriven

WHEN the operator edits per agent in a form, THEN THE SYSTEM SHALL list the 16 canonical agents in Role Group Contract order with their current label behind a 완료 default, ask a value select for the picked agent, and return to the refreshed list after each pick.

- 목록 라벨은 작업 후보의 칸이다. 한 에이전트를 바꾸면 binding이 바뀌어 다른 에이전트의 칸도 바뀔 수 있고(예: Claude 탭에서 표준 배치 → `claude-custom`), 목록이 그것을 보여 준다.

### REQ-MUX2-09 미리보기와 확인
Priority: Must · EARS: EventDriven

WHEN a form edit or refresh is pending, THEN THE SYSTEM SHALL print the after-table with `*` and every changed-key line and SHALL write only after the operator selects 적용 in a select whose default is 취소, where 계속 편집 returns to the same tab's method select with the working composition kept.

- 표와 바뀌는 설정 줄은 SPEC-MODELUX-001 REQ-MUX-06과 같은 함수로 폼 밖에 출력한다. 도구별 탭의 줄은 Per-Tool Binding Contract에 있다.
- `u` 갱신의 미리보기에서 `계속 편집`은 `모든 도구 같게`의 방식 선택으로 간다(SPEC-MODELUX-001 planner가 갱신 위에 묶는다).
- 취소 출력은 SPEC-MODELUX-001과 같은 `취소했습니다. 설정을 바꾸지 않았습니다.`이고, 메뉴로 돌아간다.
- 쓰기 경로마다 staging, 쓰기, 실패 시 보존 방법은 Write Safety Contract다.

### REQ-MUX2-10 OMP 탭
Priority: Must · EARS: EventDriven

WHEN the tab is OMP, THEN THE SYSTEM SHALL ask the OMP profile, the model family, and per picked native agent one installed model and one thinking level by the OMP Tab Contract, and SHALL print the plan's effective route for every native agent before the confirmation.

- 지정 단위는 OMP native agent 5개다. 13개 역할이 `task`로 접히므로 역할 그룹을 OMP에 쓰지 않는다(`research.md` `## OMP 조사 결과` 2).
- 실제 route는 `planOMPProfile`(init은 `planOMPProfileForConfig`)의 effective selector와 thinking이고, 기존 `renderOMPQualitySummary` 출력 그대로 보인다.

### REQ-MUX2-11 OMP 저장 경로와 동기화
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL persist a confirmed OMP tab choice of auto update and auto quality only through newOMPProfileApplyOptions and applyOMPProfile, SHALL write and activate nothing when the plan reports a blocker or an existing OMP source check refuses, and SHALL set the loaded config's role_model_policy to the value re-read from the written file before any later save or generation of the same run, ending the run with an error instead when that re-read fails.

- catalog 확인, pin 계열 증명, placeholder 거부(`validateOMPProfileSource`, `replaceAutopusConfigSection`의 placeholder guard), 활성화 실패 rollback은 기존 경로가 한다. 이 SPEC은 그 규칙을 바꾸지 않는다.
- 동기화는 쓰기 뒤 `[NEW] composeReloadConfig`(기본 `config.LoadPreview`)로 다시 읽은 `RoleModelPolicy`를 cfg에 넣는 것이다. role policy에는 placeholder가 없음이 쓰기 전에 검사됐다. 동기화가 없으면 update의 뒤 `config.Save`와 OMP 재생성, `auto quality --apply`가 옛 선택으로 덮는다(`research.md` OMP 조사 8).
- 되읽기가 실패하면 파일은 이미 쓰이고 활성화됐으므로 되돌리지 않는다. Form Text의 OMP 되읽기 실패 줄을 쓰고, update는 `composeGate.Abort`로 뒤 저장과 생성 없이 오류로 끝나며, quality는 `--apply` 없이 오류를 돌려준다. 옛 cfg로 계속하지 않는다.
- OMP 쓰기는 문서 전체를 다시 encode한다(기존 동작). 확인 화면 위에 `OMP 설정은 autopus.yaml 전체를 다시 씁니다: 주석과 서식이 바뀔 수 있습니다(값은 그대로)`를 쓴다.

### REQ-MUX2-12 update 감지 플랫폼
Priority: Must · EARS: EventDriven

WHEN auto update will add detected platforms, THEN THE SYSTEM SHALL compute the table, the tool tab, and the Codex snapshot from the configured plus detected platforms by the Platform View Contract, while every write and sync targets the loaded config, and SHALL generate every detected platform with that snapshot.

- 결함 1의 수정이다. 감지 목록은 update.go가 이미 계산한 `addedPlatforms`이고, `[NEW] composeGate.Detected`로 넘긴다. `updateComposeStep`의 시그니처는 그대로다.
- 감지 플랫폼 생성은 `[NEW] updateDetectedHarnessPlatformsWith`가 run snapshot으로 만든 adapter로 한다. zero snapshot이면 지금처럼 adapter가 스스로 probe한다.

### REQ-MUX2-13 유지 경로에서 이 단계는 쓰지 않는다
Priority: Must · EARS: Unwanted

IF the operator keeps, cancels, interrupts, or makes no change, or a write, catalog read, or plan fails, THEN THE SYSTEM SHALL perform no write in the composition step beyond the writes already confirmed in the run, so that autopus.yaml differs from the baseline only through platform detection and the existing update migrations.

- 결함 2의 수정이다. SPEC-MODELUX-001 S21의 "파일 SHA 불변" oracle은 감지가 없는 경우에만 맞다. probe P1에서 감지 저장은 `platforms` 3줄만 더했고 quality 블록 bytes는 같았다(`research.md` `## Probe P1`).
- 판정 조건: `config.Save`로 만든(canonical) 파일은 감지 저장 뒤에도 quality 블록 bytes가 같다(probe A1). 손으로 쓴 파일(주석, 다른 들여쓰기, key 순서)은 감지 저장의 `config.Save`가 파일 전체를 다시 쓰므로 bytes 대신 env 없이 decode한 quality와 `role_model_policy` 값이 기준 상태와 같다. 감지와 migration이 없으면 어떤 파일이든 SHA-256이 같다. `config.Save`의 전체 재작성은 SPEC-MODELUX-001부터의 non-goal이다.
- 블록 경계는 SPEC-MODELUX-001 REQ-MUX-08 규칙(root mapping의 다음 key 직전)이다.

### REQ-MUX2-14 Ctrl+C
Priority: Must · EARS: EventDriven

WHEN a composition form returns huh.ErrUserAborted, THEN THE SYSTEM SHALL write nothing further, print the interruption line, and end the command: auto update returns an error with a non-zero exit before the supervisor migration and every platform generation, auto init prints `init cancelled.` and writes no file, and auto quality returns without error.

- 중단 줄은 `중단했습니다. 구성을 더 바꾸지 않습니다.`(en: `Interrupted; the composition is not changed further.`)다. 메뉴 반복 중 앞에서 이미 적용한 쓰기는 그대로 남는다.
- update는 SPEC-MODELUX-001 줄 driver에서 Ctrl+C가 명령 전체를 끝냈던 것과 같게 한다(통합 결정, 2026-10-12). 이미 저장된 migration과 이 run에서 앞서 승인한 쓰기는 남고, 생성은 다음 `auto update`가 한다.
- 단계는 `[NEW] composeGate.Abort`(error pointer)에 `[NEW] errComposeInterrupted`를 적고 update.go가 그것을 돌려준다. `updateComposeStep`의 시그니처는 그대로라서 `update_compose_helpers_test.go:143-146`의 wrapper가 그대로 컴파일된다. update.go는 `Long`을 옮겨 13줄이 빈 뒤 4줄이 는다.
- init은 기존 `runInitComposeStep`의 시그니처를 그대로 두고, `[NEW] runInitComposeStepCancellable`이 취소 여부를 더 돌려준다. init.go에 더하는 줄은 4줄 이하다.

### REQ-MUX2-15 줄 driver 불변
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL keep every prompt, key, output line, and route of the SPEC-MODELUX-001 line driver, the per-tool editor, the OMP editor, and the global menu unchanged for line input, so that every existing test passes without modification.

- 공유 코드의 변경은 플랫폼 view(REQ-MUX2-12)뿐이다. 감지 플랫폼이 없는 run에서 view는 `cfg.Platforms`와 같다.

### REQ-MUX2-16 생성 = 승인한 표
Priority: Must · EARS: EventDriven

WHEN a confirmed form change is applied in the same run by update generation, auto quality --apply, or init generation, THEN THE SYSTEM SHALL produce Claude Code and Codex agent files, including those of detected platforms, whose model and effort equal the after-table cells computed with the run's snapshot.

- SPEC-MODELUX-001 REQ-MUX-19를 도구별 탭과 감지 플랫폼으로 넓힌다. oracle은 같은 snapshot으로 돈 실제 adapter `Generate`다.

### REQ-MUX2-17 폼 test seam
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL run every composition form through the injectable composeRunForm and decide the input mode through the injectable composeFormsProbe, so that tests drive the real huh fields in accessible mode over scripted input while the rest of each command runs for real.

- `[NEW] composeRunForm func(cmd *cobra.Command, form *huh.Form) error`의 기본값은 `form.WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).Run()`이다. test는 `form.WithAccessible(true).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout()).Run()`을 쓰고 입력을 `testing/iotest.OneByteReader`로 준다. huh accessible `PromptString`이 질문마다 Scanner를 새로 만들어 앞을 읽기 때문이다(`research.md`).
- `[NEW] composeFormsProbe func(cmd *cobra.Command) bool`의 기본값은 Input Mode Contract의 판정이다. Ctrl+C test는 runner가 `huh.ErrUserAborted`를 돌려주게 한다.

### REQ-MUX2-22 init의 OMP 탭: 메모리 경로
Priority: Must · EARS: EventDriven

WHEN the operator confirms an OMP tab choice during auto init, THEN THE SYSTEM SHALL plan it with planOMPProfileForConfig against a detached copy of init's in-memory config under the same catalog, attestation, capability, and blocker rules as the file path, refuse it with no change when the plan reports a blocker or the candidate role_model_policy holds a `${NAME}` placeholder, and otherwise set only init's role_model_policy to the candidate's, so that init's config.Save writes it and init's OMP generation activates it under saveConfigWithGenerationRollback.

- 2026-10-12 사용자 결정으로 더했다. init에는 아직 autopus.yaml이 없어서 파일 경로(`loadOMPProfileConfig`, `applyOMPProfile`)를 쓸 수 없다(`research.md` OMP 조사 7).
- `[NEW] planOMPProfileForConfig(ctx, cfg, opts, runner)`는 지금 `planOMPProfile`이 파일을 읽은 뒤 하는 일(`resolveOMPProfileProposal`과 미리보기 payload)을 그대로 꺼낸 함수다. `planOMPProfile`은 파일을 읽고 이 함수를 부르도록 바뀌며 출력은 같다. 모델 목록(`installedOMPCustomModelChoices`, `requireOMPCustomCapability`), pin 계열 증명, blocker 규칙은 update·quality와 같은 함수다.
- placeholder 검사: 후보 `role_model_policy`를 yaml node로 만들어 기존 `yamlNodeContainsOMPProfilePlaceholder`로 본다. 걸리면 `role_model_policy_placeholder_unsupported`로 거부한다. 파일 경로의 `validateOMPProfileSource`와 같은 규칙이다.
- 활성화는 `deps.activate`가 아니라 init의 OMP 생성이다. 생성이 실패하면 `saveConfigWithGenerationRollback`이 autopus.yaml과 플랫폼 파일을 되돌리고 init은 기존처럼 `플랫폼 설정 적용 실패`로 끝난다.
- catalog runner는 `[NEW] initComposeOMPDeps`(기본 `defaultOMPPlatformDependencies()`)의 `newRunner`다. init 종단 test의 OMP 생성은 test 전용 `[NEW] platformOMPModelRunner`(nil이면 지금과 같음)에 기존 `ompModelDoctorE2ERunner`(`doctor_omp_model_routing_e2e_test.go:110-146`: `--version`, catalog, `--config` 파일을 읽는 `config get`에 답한다)를 넣어 실제 OMP 생성과 되읽기까지 돌린다.

### REQ-MUX2-23 init에서 OMP catalog를 읽지 못함
Priority: Must · EARS: Unwanted

IF the installed OMP catalog cannot be read while the OMP tab of auto init lists models or plans a profile, THEN THE SYSTEM SHALL print the init OMP hint and the cause, leave role_model_policy as the baseline holds it, and return to the menu so that init continues.

- 안내는 Form Text의 init OMP 줄이고, 원인은 terminalSafe를 거친 오류 한 줄이다.
- 오류 분류(구성으로 구별된다): 안내는 catalog를 읽는 호출(`installedOMPCustomModelChoices`, plan 안의 `probeInstalledOMPCatalogForProfile`)이 돌려준 runner 실행 오류, `ompCatalogUnavailableError`, catalog 정규화 실패다. 거부(REQ-MUX2-22)는 `ompProfilePlanError`, `ompProfileUnavailableError`(blocker), `omp_profile_validation_failed`, `role_model_policy_placeholder_unsupported`다. 어느 쪽도 쓰지 않는다.
- 기준 상태: 같은 init에서 앞서 OMP를 적용했으면 그 선택이 남는다. 처음부터 없었을 때만 `role_model_policy`가 없다.

### REQ-MUX2-18 override 질문 폼
Priority: Must · EARS: StateDriven

WHERE a provider override exists and a 모든 도구 같게 edit changes the global binding, THEN THE SYSTEM SHALL ask the SPEC-MODELUX-001 REQ-MUX-20 question as a huh confirm whose default is no, before the preview, and SHALL remove a confirmed override in the same saveQualityBlock edit as the composition write.

- 질문 문구와 후보 반영 규칙은 SPEC-MODELUX-001과 같다. 줄 입력은 기존 `askOverrides`와 두 번의 쓰기(`removeQualityProvider`) 그대로다.
- 폼 흐름은 구성과 override 제거를 block 쓰기 하나로 묶는다. 실패하면 둘 다 남지 않는다. 제거가 있으면 writer는 line이 아니라 block이다.
- Must로 올렸다(review R11). REQ-MUX2-01이 모든 질문을 폼으로 요구하므로, 이것이 없으면 폼 흐름 안에 줄 질문이 남는다.

### REQ-MUX2-19 OSC 11 기록
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL document the pre-existing OSC 11 query's source and why no in-program fix ships, and the manual smoke procedure SHALL strip OSC and CSI sequences from the transcript before it is compared.

- 원인: bubbletea v1.3.6 package `init()`의 `lipgloss.HasDarkBackground()`(`research.md` `## OSC 11 조사`). CHANGELOG의 v0.50.125 항목에 알려진 동작으로 한 줄 적는다.

### REQ-MUX2-20 locale
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL render every new form title, option, note, and change line in Korean when tui.DetectSystemLang returns ko and in English otherwise, while every written value stays locale-independent.

### REQ-MUX2-21 문서
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL document arrow-key forms, the tool tab, the per-tool binding, the OMP tab, and the label notation in auto init, auto update, and auto quality help, README, and the v0.50.125 CHANGELOG, adding text without removing the sentences existing tests check.

- update의 `Long` 도움말은 `[NEW] internal/cli/update_help.go`로 옮긴 뒤 문장을 더한다(update.go 299줄). 생성 표면 문구는 `content/`·`templates/`에서 고치고 `make generate-templates`로 만든다.

## Contracts

### Input Mode Contract

| 조건 | driver |
|---|---|
| Update Prompt Contract의 묻지 않는 행(TTY probe false, `--yes`, `-y`, `--auto`, `--check`, CI truthy, `--workspace`), preview | 없음. SPEC-MODELUX-001 그대로 stdin 0회, 폼 0회 |
| 명령 입력이 프로세스 `os.Stdin`이고, `composeTTYProbe()` true, stdout이 터미널, Windows면 `WT_SESSION`이나 `TERM_PROGRAM`이 있음 | huh 폼 |
| `cmd.SetIn`으로 준 reader(scripted, test) | 줄 driver(SPEC-MODELUX-001) |
| stdin은 터미널이고 stdout이 터미널이 아님(`auto update \| tee log`) | 줄 driver |
| Windows legacy console(위 두 변수 없음) | 줄 driver |

### Form Flow Contract

| 단계 | 형식 | 선택지(순서) | 기본값 | 다음 |
|---|---|---|---|---|
| 메뉴 | Select `구성을 바꿀까요?`, 바로 위에 표와 프리셋 상태 줄 | 유지 · 최신 기본값으로 갱신(historical일 때) · 구성 편집 · 기본 프리셋 고르기(`auto quality`만) | 유지 | 유지: 단계 끝 / 갱신: 미리보기 / 편집: 도구 탭 / 기본 프리셋: 프리셋 Select |
| 도구 탭 | Select `어느 도구를 바꿀까요?` | 모든 도구 같게 · Claude Code(view에 `claude-code`) · Codex(view에 `codex`) · OMP(OMP Tab Contract의 노출 조건) | 모든 도구 같게 | OMP: OMP Tab Contract / 나머지: 방식 |
| 방식 | Select `어떻게 고칠까요?` | 역할 그룹 · 에이전트별 | 역할 그룹 | 편집 |
| 역할 그룹 | Role Group Contract 순서로 그룹마다 Select. 제목 `<그룹> — <멤버>` | 그대로 + 등급 4개(`config.QualityTiers()` 순서, Label Contract) | 그대로 | 미리보기 |
| 에이전트별 | 목록 Select → 값 Select 반복 | 목록: 완료 + 정규 에이전트 16개(그룹 순서) / 값: 그대로 + 등급 4개 | 목록: 완료 / 값: 그대로 | 완료: 미리보기 |
| 미리보기 | 폼 밖 출력: after-table(`*`), 바뀌는 설정 줄 | - | - | 확인 |
| 확인 | Select `적용할까요?` | 취소 · 적용 · 계속 편집 | 취소 | 적용: 쓰기 → 메뉴 / 취소: 메뉴 / 계속 편집: 같은 탭의 방식 |
| 기본 프리셋 | Select | `orderedQualityPresets` 순서, `<name> — <description>`, 현재 default에 `(현재)` | 현재 default | `persistComposeBinding`의 line 경우처럼 `precheckQualityWriteFile` 뒤 cfg 분리 사본으로 `saveQualityDefault`, 성공한 뒤에만 cfg를 맞추고 `quality.default = <name>` → 메뉴 |

- 편집 결과가 시작과 같으면 `변경 없음`을 쓰고 메뉴로 돌아간다. 쓰지 않는다.
- 메뉴는 돌아올 때마다 표를 다시 그린다. 다시 그린 표는 방금 쓴 값(cfg 동기화)과 같은 snapshot으로 계산한다.
- 쓰기가 거부되거나 실패하면 기준 상태(REQ-MUX2-03)가 그대로이고 경고 1줄(`warnUpdateCompose`와 같은 형식)을 쓴 뒤 메뉴로 돌아간다. 경로별 보장은 Write Safety Contract다.

### Label Contract

| 자리 | 형식 | 값의 원천 |
|---|---|---|
| 모든 도구 같게 · 등급 선택지 | `<tier> — Claude Code: <C> \| Codex: <X>`. 설치된 열만 쓴다. 둘 다 없으면 `<tier>` | 그 등급을 작업 구성에 넣고 SPEC-MODELUX-001 `planComposeBinding`으로 묶은 후보 config |
| 도구별 · 등급 선택지 | `<cells>  [<tier>]` | 그 등급을 작업 구성에 넣은 도구별 후보(Per-Tool Binding Contract) |
| 그대로 | `그대로 — <현재>` | 모든 도구 같게: 그룹 표시(SPEC-MODELUX-001 REQ-MUX-03의 등급 또는 `혼합 (…)`)나 에이전트 등급 / 도구별: 작업 후보의 `<cells>` |
| 에이전트 목록 | `<agent> — <현재>` | 모든 도구 같게: `<tier> (Claude Code: <C> \| Codex: <X>)` / 도구별: 작업 후보의 칸 |
| `<C>` Claude 칸 | `<model> · <effort>`. model은 `ClaudeAgentModel(agent, fallback)`에서 `claude-`를 뗀 것, effort는 `ClaudeAgentProfile`의 effort, 없으면 `-` | 후보 config의 quality |
| `<X>` Codex 칸 | Table Cell Contract의 Codex 표기(`gpt-` 제거, `(대체)`, `(catalog 없음)`, `(미확인)`, `생성 거부`) | `codex.ResolveAgentProfile`(후보 quality, run snapshot) |
| `<cells>` | 영향받는 에이전트(그룹 멤버나 에이전트 하나)의 칸을 멤버 순서로, 같은 글자는 한 번만, ` / `로 잇는다 | - |

- 선택하면 변화가 없는 선택지(작업 구성과 같은 등급)는 변화 없는 config(읽어 온 config나 작업 후보)의 칸을 보인다.
- 라벨은 모두 `terminalSafe`를 거친다. locale과 무관한 것은 모델 ID, effort, 저장 등급뿐이다. `그대로`/`keep`, `완료`/`done`, 상태 표기(`(대체)`/`(substitute)`, `생성 거부`/`generation refused` 등)는 Table Cell Contract의 locale 규칙을 따른다.
- 표 칸은 SPEC-MODELUX-001 C16 그대로 frontmatter 값(alias와 정확한 ID 구별)이다. Claude 라벨의 모델은 slug다(예: 라벨 `opus-5-5 · high`, 표 칸 `opus · high`). help와 README가 이 차이를 설명한다.

### Per-Tool Binding Contract

| 결과 | 파일 쓰기(update, quality) | 메모리 쓰기(init) | 바뀌는 설정 줄 |
|---|---|---|---|
| 시작과 같음 | 없음(`변경 없음`) | 없음 | - |
| 시작과 다름 | `saveQualityToolPreset(dir, cfg, tool, "<p>-custom", agents)`: `quality.presets.<p>-custom` ← description `<label> custom agent tiers`, 정규 에이전트 16개 전부의 등급 / `quality.providers.<p>` ← `<p>-custom` | `cfg.Quality`에 같은 두 key | `quality.presets.<p>-custom`의 SPEC-MODELUX-001 문구(`composePresetNew`·`composePresetRewrite`·`composePresetOverwrite`, `quality_compose_bind_text.go:34-37`: 없으면 `새로 씀`, 그 도구의 binding이면 `고쳐 씀`, 있지만 binding이 아니면 `덮어씀`), `quality.providers.<p> <이전 값 또는 없음> → <p>-custom`, 도구의 mode가 balanced였으면 `<p>-custom은 balanced 표준 배치를 받지 않음`, 파일 쓰기면 SPEC-MODELUX-001의 블록 재작성 안내, `quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다` |

- `<p>`와 `<label>`: `claude`·`Claude Code`, `codex`·`Codex`. 이름은 `qualityToolPresetName`이 정한다.
- 시작은 `quality.ForProvider(<p>).EffectiveAgentTier(agent, fallback)` 16개다. fallback은 SPEC-MODELUX-001과 같은 embedded agent source다.
- `quality.default`와 다른 프리셋은 바꾸지 않는다. `<p>-custom`을 전역 default나 다른 provider가 같이 쓰고 있으면 그 열의 바뀌는 칸도 미리보기에 `*`로 나온다.
- 저장 값은 상대 등급이다. 근거는 `research.md` `## 설계 결정`의 "도구별 저장 값" 행이다.

### OMP Tab Contract

| 단계 | 형식 | 선택지 | 재사용하는 기존 판단 |
|---|---|---|---|
| profile | Select `OMP 구성`, 설명에 현재 `role_model_policy` 요약(없으면 `플랫폼 기본값`) | 돌아가기 · balanced · ultra · 에이전트별 지정 (balanced 위에) | `runOMPQualityInteractive`의 balanced/ultra/custom(L48-80) |
| 계열 | Select `모델 계열` | GPT(openai) · Claude(anthropic). 기본값은 저장된 family, 없으면 GPT | 같은 이름의 명시 profile이 있으면 묻지 않는다(L73-76) |
| 에이전트 목록 | Select | 완료 + scout, reviewer, security-reviewer, task, sonic(`OMPNativeAgentNames` 순서). 라벨은 `<agent> — 프로필 기본값` 또는 `<agent> — <selector>:<thinking>` | - |
| 모델 | Select | 프로필 기본값 + 그 agent의 capability를 선언한 설치 모델(메타데이터 없는 catalog면 전부), selector 순 | `installedOMPCustomModelChoices`, `requireOMPCustomCapability` |
| thinking | Select(단계가 둘 이상일 때만) | 모델이 보고한 단계, 기본은 첫 단계 | `readOMPCustomThinkingChoice`의 규칙 |
| 미리보기 | 폼 밖 출력 | - | `newOMPProfileApplyOptions` → `planOMPProfile`(update·quality) 또는 `[NEW] planOMPProfileForConfig`(init) → `renderOMPQualitySummary` |
| 확인 | Select `적용할까요?` | 취소 · 적용 | update·quality: `applyOMPProfile(ctx, dir, opts, runner, deps.activate)` / init: init cfg의 `RoleModelPolicy` 교체(REQ-MUX2-22) |

- 기본값은 단계마다 돌아가기, 완료, 프로필 기본값, 취소다. Enter만 누르면 아무것도 쓰지 않는다.
- plan에 blocker가 있거나 기존 검사가 거부하면(`omp_profile_candidate_unavailable`, `role_model_policy_placeholder_unsupported`, `omp_platform_not_configured` 등) 오류 줄을 terminalSafe로 쓰고 확인 없이 메뉴로 돌아간다. 쓰기와 활성화는 0이다.
- 에이전트별 지정에서 pin이 없으면 기존 문구 `No agent was pinned; nothing changed.`를 쓰고 메뉴로 돌아간다.
- update·quality는 적용 뒤 기존 문구 `OMP models applied. Start a new OMP session to use them.`를 쓰고 cfg를 동기화한다(REQ-MUX2-11). init은 Form Text의 init OMP 적용 줄을 쓴다.
- 노출 조건: `auto quality`와 `auto update`는 읽어 온 cfg의 `platforms`에 `omp`가 있을 때, `auto init`은 init cfg의 `platforms`에 `omp`가 있을 때다. update에서 omp가 이번 run에 감지만 됐으면 `OMP는 이번 update에서 새로 설치됩니다. 모델은 다음 auto update나 auto quality에서 지정할 수 있습니다.`를 한 번 쓴다. init에서 catalog를 읽지 못하면 REQ-MUX2-23대로 안내하고 메뉴로 돌아간다.
- OMP 의존성(runner, activate)은 `auto quality`가 받는 기존 `deps`, update는 `[NEW] updateComposeOMPDeps`, init은 `[NEW] initComposeOMPDeps`(runner만 쓴다)다. 둘의 기본값은 `defaultOMPPlatformDependencies()`다.
- 요약 표(`renderOMPQualitySummary`)와 기존 OMP 문구는 영어 그대로 다시 쓴다(`auto platform omp`와 같은 출력).

### Platform View Contract

| 사용처 | 원천 |
|---|---|
| 표 열, 도구 탭의 Claude Code·Codex, Codex snapshot 캡처, `composePresetFeedsTable`, override 질문 대상 | view = `cfg.Platforms` + `composeGate.Detected`(update만, 감지 순서). 매번 새 얕은 사본이다 |
| 쓰기와 동기화(`persistComposeBinding`, `saveQualityToolPreset`, OMP 동기화, init 메모리 writer) | 읽어 온 cfg |
| OMP 탭 노출 | update·quality는 읽어 온 cfg의 `platforms`, init은 init cfg의 `platforms` |
| 감지 플랫폼 생성 | `updateDetectedHarnessPlatformsWith`가 run snapshot으로 만든 adapter |

### Write Safety Contract

| 경로 | 쓰기 전 | 쓰기 | 실패하면 |
|---|---|---|---|
| 모든 도구 같게 line(`quality.default`), 기본 프리셋 | `precheckQualityWriteFile`, cfg 분리 사본 | 기존 line writer `saveQualityDefault`(`quality_config.go:24-75`) | 파일과 cfg가 기준 상태 그대로. cfg는 성공 뒤에만 맞춘다 |
| 모든 도구 같게 block, 도구별 탭, 폼의 override 제거 | `saveQualityBlock`의 raw decode·참조 거부·placeholder 검사 | quality 블록 splice + 기존 atomic 교체(temp 파일 → rename) 한 번 | 원본 bytes와 cfg 그대로. 구성과 override 제거는 같이 남거나 같이 안 남는다 |
| OMP(update·quality) | `loadOMPProfileConfig` snapshot, `validateOMPProfileSource`, plan의 blocker | `marshalAutopusConfig` → snapshot `Replace` → 활성화 | 활성화 실패는 원본 bytes 복원(`platform_omp_profile_apply.go:198-222`). 되읽기 실패는 이 run의 뒤 저장·생성 중단(REQ-MUX2-11) |
| OMP(init), init의 quality 쓰기 | 분리 사본 plan, placeholder 검사, `plan.apply`의 built-in 상태 확인 | init cfg만 바꾸고 init의 `saveConfigWithGenerationRollback`이 저장·생성 | autopus.yaml과 플랫폼 파일을 생성 전 snapshot으로 복원(`platform_lifecycle.go:128-150`, 기존 test `init_multi_platform_rollback_test.go`) |
| 감지 플랫폼 저장(update, 기존) | 플랫폼별 사본 | `updatePlatformThenSave` | 기존 동작(플랫폼 파일 복원) |

### Form Text (ko / en)

| 자리 | ko | en |
|---|---|---|
| 메뉴 | `구성을 바꿀까요?`: 유지 · 최신 기본값으로 갱신 · 구성 편집 · 기본 프리셋 고르기 | `Change the composition?`: keep · update to the latest defaults · edit the composition · choose the default preset |
| 도구 탭 | `어느 도구를 바꿀까요?`: 모든 도구 같게 | `Which tool?`: same for all tools |
| 방식 | `어떻게 고칠까요?`: 역할 그룹 · 에이전트별 | `How?`: role groups · per agent |
| 목록 | `에이전트를 고르세요`: 완료 | `Pick an agent`: done |
| 확인 | `적용할까요?`: 취소 · 적용 · 계속 편집 | `Apply?`: cancel · apply · keep editing |
| OMP | `OMP 구성`: 돌아가기 · 에이전트별 지정 (balanced 위에) / `모델 계열` / 프로필 기본값 | `OMP models`: back · pin per agent (on balanced) / `Model family` / profile default |
| 중단 | `중단했습니다. 구성을 더 바꾸지 않습니다.` | `Interrupted; the composition is not changed further.` |
| OMP 되읽기 실패 | `OMP 설정은 적용됐지만 다시 읽지 못했습니다: <원인>` | `OMP settings were applied but could not be re-read: <cause>` |
| init OMP | 안내 `OMP catalog를 읽지 못해 지금은 OMP 모델을 지정할 수 없습니다. init 뒤 auto quality에서 지정할 수 있습니다.` / 적용 `OMP 모델 선택을 init 설정에 넣었습니다. 파일 생성 때 함께 씁니다.` | `Could not read the OMP catalog, so OMP models cannot be set now; set them after init with auto quality.` / `OMP model choice added to the init config; it is written with the platform files.` |

## Existing Test Changes

바꾸는 기존 test는 0건으로 예상한다.

1. 기존 test는 `cmd.SetIn`으로 입력을 주므로 Input Mode Contract상 줄 driver로 돈다. 줄 driver, 도구별 편집기, OMP 편집기, 전역 메뉴의 코드는 그대로다.
2. `updateComposeStep`과 `runInitComposeStep`의 시그니처는 바꾸지 않는다. 감지 목록과 중단은 `composeGate`의 새 field(`Detected`, `Abort`)로, init 취소는 새 함수로 넘긴다. `update_compose_helpers_test.go:143-146`의 wrapper는 그대로 컴파일된다. `composeGate`를 field 이름으로 만드는 literal은 그대로 컴파일된다.
3. 도움말은 문장을 더하기만 한다. `quality_compose_docs_test.go`가 고정한 문장은 지우지 않는다.
4. `planOMPProfile`은 파일을 읽은 뒤 `planOMPProfileForConfig`를 부르도록 바뀌지만 출력은 같다. 기존 OMP profile·편집기 test가 그대로 통과해야 한다. `platformOMPModelRunner`는 nil이 기본이라 기존 OMP 생성 경로는 그대로다.
5. 위험(미확인, R1): 기존 update test가 플랫폼 감지를 고정하지 않으면, CLI가 설치된 기계에서는 감지 열이 표에 더해진다. CI(감지 0)는 영향이 없다. T2에서 이 기계로 먼저 돌려 본다. 단언이 바뀌면 기존 test 파일은 고치지 않고 test helper에서 감지를 고정하고, 이 절에 적어 review를 받는다.

## 생성 파일 상세

- `[NEW] internal/cli/quality_compose_form.go`(+test): `composeFormsProbe`, `composeRunForm`, Select·Confirm builder(테마, terminalSafe, `ErrUserAborted` 판정).
- `[NEW] internal/cli/quality_compose_form_driver.go`(+test): 메뉴 반복, 도구 탭, 방식, 미리보기·확인, 갱신, 기본 프리셋 Select, snapshot 1회.
- `[NEW] internal/cli/quality_compose_form_edit.go`(+test): 역할 그룹 Select 4개, 에이전트 목록 → 값.
- `[NEW] internal/cli/quality_compose_form_labels.go`(+test): Label Contract.
- `[NEW] internal/cli/quality_compose_form_text.go`(+test): Form Text.
- `[NEW] internal/cli/quality_compose_tool.go`(+test): Per-Tool Binding Contract(시작, 후보, 바뀌는 줄, 파일·메모리 저장).
- `[NEW] internal/cli/quality_compose_omp.go`(+test): OMP Tab Contract, update·quality의 cfg 동기화, init 메모리 경로(분리 사본 plan, placeholder 검사, `RoleModelPolicy` 교체), `updateComposeOMPDeps`, `initComposeOMPDeps`.
- `[NEW] internal/cli/update_help.go`: update `Long` 도움말.
- 기존 파일 변경: `quality_compose_session.go`(view), `quality_compose_snapshot.go`(view로 캡처), `quality_compose_driver_gate.go`(`composeGate.Detected`), `quality_compose_entry.go`(폼 경로), `quality_compose_entry_override.go`(질문 함수 주입), `update_compose.go`, `init_compose.go`(`runInitComposeStepCancellable`, OMP 탭 노출), `init.go`(4줄 이하), `update.go`(도움말 참조, `Detected`·`Abort`, 중단 반환, 감지 생성 호출. `Long`을 옮긴 뒤 290줄 안팎), `platform_update.go`(`updateDetectedHarnessPlatformsWith`), `platform_catalog.go`(snapshot을 묶은 descriptor, OMP descriptor의 `[NEW] platformOMPModelRunner`), `platform_omp_profile_apply.go`(`[NEW] planOMPProfileForConfig` 추출, 출력 불변).
- 문서: `README.md`, `CHANGELOG.md`(v0.50.125), init·quality 도움말. 증거: `evidence/probe-a1.md`(probe A1, 경로 치환), `[NEW] evidence/manual-smoke-002.txt`(S20).
- 모든 신규·변경 소스 파일은 300줄 이하다.

## Related SPECs

- SPEC-MODELUX-001(같은 branch, 구현됨): 바뀌는 점은 넷이다. (1) REQ-MUX-17의 "줄 driver 하나"는 실제 터미널에서 폼 driver가 되고, 줄 driver는 scripted·pipe 입력에 남는다. (2) REQ-MUX-09의 `quality.providers` 금지는 도구별 탭에 적용하지 않는다(REQ-MUX2-07). (3) OMP non-goal 중 "OMP를 quality 등급과 통합"은 그대로 non-goal이다. 이 SPEC은 OMP 탭을 더한다(update·quality는 기존 OMP writer, init은 같은 plan 검사를 거친 메모리 경로). (4) S21은 이 SPEC의 S20으로 대체한다.
- SPEC-PROVIDER-QUALITY-001(completed): `quality.providers`와 `EffectiveMode` 우선순위를 재사용한다.
- OMP 계열 SPEC(SPEC-OMP-005 등): profile apply transaction, pin 계열 증명, native agent 접힘을 바꾸지 않고 부른다.
- SPEC-INITUX-001(completed): wizard 테마와 Windows guard를 재사용한다.
- Sibling: 없다(`research.md` `## Sibling SPEC Decision`).

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-MUX2-01 | T1, T7 | S1, S20 | INV-MUX2-06, INV-MUX2-07 |
| REQ-MUX2-02 | T1 | S15 | INV-MUX2-11 |
| REQ-MUX2-03 | T5 | S2 | INV-MUX2-01, INV-MUX2-08 |
| REQ-MUX2-04 | T5, T6, T12 | S3 | INV-MUX2-09 |
| REQ-MUX2-05 | T3, T5 | S4 | INV-MUX2-02 |
| REQ-MUX2-06 | T3 | S5, S6, S14 | INV-MUX2-02 |
| REQ-MUX2-07 | T4 | S5, S6 | INV-MUX2-03, INV-MUX2-10 |
| REQ-MUX2-08 | T5 | S6 | INV-MUX2-02 |
| REQ-MUX2-09 | T5 | S4, S7 | INV-MUX2-01 |
| REQ-MUX2-10 | T6 | S8 | INV-MUX2-05 |
| REQ-MUX2-11 | T6 | S8, S9 | INV-MUX2-05 |
| REQ-MUX2-12 | T2 | S10 | INV-MUX2-08, INV-MUX2-09 |
| REQ-MUX2-13 | T2, T5 | S11, S20 | INV-MUX2-01 |
| REQ-MUX2-14 | T7 | S12 | INV-MUX2-01 |
| REQ-MUX2-15 | T11 | S1, S13 | INV-MUX2-06 |
| REQ-MUX2-16 | T2, T4, T5 | S5, S6, S10, S14 | INV-MUX2-04 |
| REQ-MUX2-17 | T1 | S1, S12 | INV-MUX2-07 |
| REQ-MUX2-18 | T8 | S16 | INV-MUX2-01 |
| REQ-MUX2-19 | T10, T11 | S18 | INV-MUX2-11 |
| REQ-MUX2-20 | T9 | S17 | - |
| REQ-MUX2-21 | T10 | S19 | - |
| REQ-MUX2-22 | T12 | S21 | INV-MUX2-05 |
| REQ-MUX2-23 | T12 | S22 | INV-MUX2-01, INV-MUX2-05 |

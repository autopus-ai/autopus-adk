# SPEC-ORCH-022: Claude Code 안 hook-IPC headless 멀티프로바이더 실행 경로 — 구독 세션 결정론적 수집

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-06-09
**Domain**: ORCH
**Review**: main-session adjudication — `auto spec validate --strict` PASS + spec-quality self-verify 전항목 PASS(research.md) + 4파일 직접 검증 + Option B feasibility 스모크 PASS(cmux headless surface 호스팅, claude Stop hook 발화/last_assistant_message 확인). multi-provider review는 현재 CLAUDECODE→subprocess 경로로 신뢰불가(이 SPEC이 고치는 문제)라 미사용.
**Implementation note (2026-06-10)**: T1~T8 구현 + features.Monitor 디커플링 완료. 빌드/단위 검증 전부 통과. 단 실 e2e에서 hook-IPC done-file 수집 엔게이지 미확인 → `## Completion Debt`(research.md) BLOCKING. status는 `implemented`이며 Completion Debt 해소 전 `completed` 금지.
**Completion note (2026-06-11)**: 마지막 BLOCKING(완료 done-file 미수집) 근본 원인 규명·수정·실 e2e 검증 완료 → `completed`. 원인은 타이밍이 아니라 `resolveCompletionDetector`가 `FileIPCDetector`를 `features.Monitor` 뒤에 게이팅한 것: 모니터 off 시 `ScreenPollDetector`로 떨어져 화면 렌더를 done-file보다 먼저 감지→`Execute` 반환→`defer hookSession.Cleanup()`이 Stop hook 발화 직전 세션 dir 삭제(race). 수정: HookMode+hookSession이면 monitor flag/cap과 무관하게 `FileIPCDetector`를 full-budget floor로 우선 선택(`Execute`가 done-file까지 블록) + `hook-claude-stop.sh` done-file 무조건 기록 하드닝. TRUSTED e2e에서 `SESSION_DIR exists=yes` + `claude-done`/`claude-result.json` 수집 + screen-scrape 폴백 미개입 확인. 상세는 research.md `## Completion Debt`. 잔여 비차단 항목(killed-process 시 pane orphan cleanup)은 Evolution Ideas.

## 목적

SPEC-ORCH-021이 cmux/tmux 터미널에서 인터랙티브 pane을 기본 실행 경로로 만들었고, SPEC-ORCH-007이 `Stop`/`AfterAgent` hook 파일 시그널 프로토콜(`/tmp/autopus/{session-id}/{provider}-done` + `{provider}-result.json`)을 정의·구현했다. 그러나 두 SPEC을 결합해도 Claude Code(CLAUDECODE 환경) 안에서 멀티프로바이더 오케스트레이션(`auto spec review`, `auto orchestra brainstorm`)을 돌리면 여전히 subprocess `-p`로 강제 fallback된다.

근본 동기는 SPEC-ORCH-021 spec.md:9의 사실과 동일하다. 대부분의 유저는 구독제(Claude Pro/Max 등)를 쓰며, 구독 세션은 로그인된 인터랙티브 CLI로만 접근 가능하고 `-p` API 모드는 별도 API 키/과금이 필요하다. 따라서 인터랙티브 실행이 구독 멀티프로바이더의 유일 경로다. Claude Code 안(Bash, 비-TTY)에서 이 경로가 막히면 nested-agent 자동화에서 구독 세션을 전혀 쓸 수 없다.

현재 막는 지점은 두 가지로 좁혀진다(코드 실측).

첫째, `internal/cli/orchestra_terminal.go:43-47` `paneInteractiveContext`는 `claudeCode != "" || ci != ""`이면 즉시 false를 반환해 `detectStructuredTerminal()`가 `terminal.PlainAdapter`를 돌려주고, `SelectBackend`가 subprocess backend를 강제한다. 이 가드(`f27f1b8`)는 nested pane이 화면 스크래핑 완료를 못 봐서 0/N timeout 난다는 정당한 이유로 들어갔다. 그러나 화면 스크래핑이 아니라 hook 파일 시그널로 완료를 받으면 nested-pane도 결정론적으로 완료할 수 있다.

둘째, `pkg/orchestra/pane_backend.go:122` `var hookSession *HookSession`는 nil 하드코딩이라 `HookMode=true`여도 `InteractivePaneBackend.Execute`가 hook IPC를 쓰지 않고 항상 poll/monitor 경로(`completion_detector.go:36` `ScreenPollDetector`)로 떨어진다.

추가로, hook 가용성 게이트(`internal/cli/orchestra_helpers.go:237-251` `isHookModeAvailable`)는 `~/.claude/settings.json`에 `autopus` + `Stop` 문자열이 동시에 있어야 true를 반환하는데, `auto init`의 settings.json 생성기(`pkg/content/hooks.go:46` `generateCLIHooks`)가 `Stop` 이벤트를 등록하지 않아 항상 false다. 즉 hook 수집은 코드상 구현돼 있으나(수신측 `pkg/orchestra/hook_watcher.go`, `hook_signal.go`, `completion_file_ipc.go`) 활성화 경로가 끊겨 있다.

이 SPEC은 새 백엔드·새 strategy·JSON 스키마 변경 없이, 끊긴 활성화 경로 세 곳(hook 등록, 가드 완화, PaneBackend의 실제 HookSession 사용)을 잇고 hook-only headless 수집을 추가해 Claude Code 안에서도 구독 세션 멀티프로바이더가 0/N timeout 없이 동작하게 만든다.

## Outcome Boundary

- Outcome Lock: cmux/tmux가 설치되어 있고 완료 hook이 설치된 환경에서 `auto spec review` / `auto orchestra brainstorm`을 Claude Code 안에서 실행하면 provider들이 done-file IPC로 완료 수집되어 0/N timeout 없이 결과를 반환한다. hook 또는 세션이 미가용이면 subprocess `-p`로 graceful degrade하고, 둘 다 불가하면 actionable 에러를 낸다.
- Mandatory requirements: REQ-001(claude Stop hook 등록), REQ-002(gemini AfterAgent + codex Stop 등록), REQ-003(isHookModeAvailable project-local 인식·전이), REQ-004(headless hook-IPC 수집 경로), REQ-005(CLAUDECODE 가드 완화), REQ-006(PaneBackend 실 HookSession 사용), REQ-007(bounded timeout 결정적 실패), REQ-008(hook 미설치 시 subprocess degrade 보존), REQ-009(both-failed actionable 에러), REQ-010(보안 경계).
- Explicit non-goals: provider-API 백엔드 신설(subprocess `-p`는 기존 best-effort floor로만 유지), JSON 스키마/structured 출력 계약 변경(`{provider}-result.json` 스키마는 SPEC-ORCH-007 R1 그대로), 새 strategy 추가/SCAMPER·ICE brainstorm 프롬프트 변경, plan·review·secure legacy `RunOrchestra` 경로 변경(SPEC-ORCH-021 비목표 승계), 화면 스크래핑 완료 감지 메커니즘 자체 재설계(hook IPC로 대체 활성화만, ScreenPollDetector는 degrade floor로 보존), plain/CI에서 hook 미설치 시 subprocess floor 파괴, `creack/pty` 직접 의존성 승격/자체 PTY attach 구현.
- Completion evidence: oracle 수락 — (1) `auto init` 후 project `.claude/settings.json`의 `hooks.Stop`에 `.claude/hooks/autopus/hook-claude-stop.sh` 명령이 등록됨, (2) settings.json에 `autopus` + `Stop` 존재 시 `isHookModeAvailable()`가 true로 전이, (3) CLAUDECODE + hook 가용 + cmux 설치 시 완료 detector가 `FileIPCDetector`(subprocess `-p` 아님)로 선택됨, (4) done-file 미수신 시 context deadline 기반 bounded timeout으로 결정적 false 반환, (5) hook 미설치 시 subprocess degrade 유지, (6) hook도 cmux도 불가하면 복구 지침 포함 actionable 에러.

## Requirements

### Ubiquitous / Priority: Must (REQ-001 — claude Stop hook 자동 등록)
WHEN `auto init`이 claude-code 플랫폼에서 hook 설정을 생성하면 THEN THE SYSTEM SHALL `generateCLIHooks` 결과에 `Stop` 이벤트 hook(command `.claude/hooks/autopus/hook-claude-stop.sh`, matcher 없음)을 포함하여 project `.claude/settings.json`의 `hooks.Stop`에 기록한다. 관측 지점은 `.claude/settings.json`의 `hooks.Stop[0].hooks[0].command` 값이다.

### Where / Priority: Must (REQ-002 — gemini AfterAgent + codex Stop 등록)
WHERE 플랫폼이 antigravity-cli(gemini)이면 THEN THE SYSTEM SHALL `AfterAgent` 이벤트 hook을 hooks.json `autopus` 스펙에 등록하고, WHERE 플랫폼이 codex이면 THE SYSTEM SHALL `Stop` 이벤트 hook을 등록한다. 관측 지점은 gemini hooks.json `autopus.AfterAgent`와 codex 설정의 Stop 엔트리다.

### Event-Driven / Priority: Must (REQ-003 — hook 가용성 게이트 전이)
WHEN orchestra 실행이 hook 가용성을 검사하면 THEN THE SYSTEM SHALL project-local `.claude/settings.json` 또는 user-global `~/.claude/settings.json` 중 하나에 `autopus` + `Stop` 문자열이 동시에 존재하면 `isHookModeAvailable()`가 true를 반환한다. 관측 지점은 `isHookModeAvailable()` 반환값과 `resolveCC21MonitorRuntime`의 `HookMode` 필드다.

### Event-Driven / Priority: Must (REQ-004 — headless hook-IPC 수집 경로)
WHEN 프로세스가 인터랙티브 TTY에 attach되어 있지 않지만 cmux가 설치되어 있고 HookMode가 활성이면 THEN THE SYSTEM SHALL 화면 스크래핑(ReadScreen 폴링) 대신 `WaitAndCollectHookResults` / `FileIPCDetector`의 done-file 감시로 provider 완료를 수집한다. 관측 지점은 선택된 완료 detector 타입(`FileIPCDetector`)과 수집된 `ProviderResponse.Output` 출처다.

### Event-Driven / Priority: Must (REQ-005 — CLAUDECODE 가드 완화)
WHEN `detectStructuredTerminal()`가 CLAUDECODE 환경에서 호출되고 cmux/tmux가 설치되어 있으며 HookMode가 가용이면 THEN THE SYSTEM SHALL `PlainAdapter` 대신 감지된 멀티플렉서 터미널을 반환하여 hook-IPC pane 경로로 진입한다. WHILE HookMode가 불가하거나 멀티플렉서가 없으면 THE SYSTEM SHALL 기존대로 `PlainAdapter`를 반환하여 subprocess floor를 보존한다. 관측 지점은 `detectStructuredTerminal()` 반환 터미널 `Name()`이다.

### Event-Driven / Priority: Must (REQ-006 — PaneBackend의 실제 HookSession 사용)
WHEN `InteractivePaneBackend.Execute`가 `HookMode=true` 설정으로 실행되면 THEN THE SYSTEM SHALL `pane_backend.go:122`의 nil 하드코딩 대신 실제 `HookSession`을 생성·전달하여 `waitForCompletion`이 `FileIPCDetector`를 선택하도록 한다. 관측 지점은 `Execute` 내부 `hookSession` 비-nil 여부와 해석된 detector다.

### Unwanted / Priority: Must (REQ-007 — bounded timeout 결정적 실패)
IF done-file이 설정된 timeout 내에 수신되지 않으면 THEN THE SYSTEM SHALL context deadline 기반 bounded wait 후 해당 provider를 결정적으로 미완료/timeout으로 표시하고 무한 대기하지 않는다. 관측 지점은 timeout 경과 후 `FileIPCDetector.WaitForCompletion` 반환값(false)과 `ProviderResponse.TimedOut`이다.

### Unwanted / Priority: Must (REQ-008 — hook 미설치 시 subprocess degrade 보존)
IF settings.json에 `Stop` hook이 등록되어 있지 않거나 cmux/tmux가 없으면 THEN THE SYSTEM SHALL 기존 subprocess `-p` 실행 경로로 graceful degrade하고 hook-IPC 경로를 강제하지 않는다. 관측 지점은 hook 미가용 입력에서 `SelectBackend`가 subprocess backend를 반환하고 `detectStructuredTerminal()`가 plain을 반환하는지다.

### Unwanted / Priority: Must (REQ-009 — both-failed actionable 에러)
IF hook-IPC 경로와 subprocess `-p` 경로가 모두 사용 불가하면 THEN THE SYSTEM SHALL 어떤 전제가 빠졌는지(hook 미설치 시 `auto init` 재실행, cmux 미설치, API 키 부재)를 명시한 actionable 에러를 반환한다. 관측 지점은 둘 다 불가 입력에서 반환되는 에러 메시지 텍스트다.

### Ubiquitous / Priority: Must (REQ-010 — 보안 경계)
THE SYSTEM SHALL hook 세션 디렉토리를 0o700으로, result 파일을 0o600으로 생성하고, session-id에 영숫자·하이픈·언더스코어 외 문자가 있으면 거부하여 경로 traversal을 막으며, provider가 done-file에 기록한 출력을 untrusted 입력으로 취급하고, headless에서 `--dangerously-skip-permissions`를 붙이는 위험을 문서화한다. 관측 지점은 `NewHookSession`의 MkdirAll 모드, result 파일 chmod, session-id 검증 분기, research.md 보안 섹션이다.

## Acceptance Criteria

- [ ] REQ-001 ~ REQ-010이 acceptance.md S1 ~ S10 시나리오로 oracle 검증된다.

## 생성 파일 상세

`[NEW] pkg/orchestra/headless_hook_runner.go`는 TTY 없는 headless 환경에서 cmux pane 위에 provider를 띄우고 `WaitAndCollectHookResults`로 수집하는 entry다. 수집 로직은 기존 `hook_watcher.go`를 재사용해 300줄 한도를 지킨다.

수정 대상: `pkg/content/hooks.go`(`generateCLIHooks`에 Stop/AfterAgent/Stop 추가), `internal/cli/orchestra_terminal.go`(`paneInteractiveContext` 입력 확장), `internal/cli/orchestra_helpers.go`(`isHookModeAvailable` project-local 검사), `pkg/orchestra/pane_backend.go`(line 122 nil 제거), `internal/cli/spec_review_loop.go` / `internal/cli/orchestra_run.go`(both-failed 에러).

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-001 | T1 | S1 | INV-001 |
| REQ-002 | T2 | S2 | INV-002 |
| REQ-003 | T3 | S3 | INV-003 |
| REQ-004 | T4 | S4 | INV-004 |
| REQ-005 | T5 | S5, S5b | INV-005 |
| REQ-006 | T6 | S6 | INV-006 |
| REQ-007 | T4, T6 | S7 | INV-007 |
| REQ-008 | T5, T7 | S8 | INV-008 |
| REQ-009 | T7 | S9 | INV-009 |
| REQ-010 | T1, T4 | S10 | INV-010 |

## Related SPECs

- SPEC-ORCH-007 (dependency): 파일 시그널 프로토콜 R1·Stop hook 스크립트·수신측 구현. 본 SPEC은 이를 재발명하지 않고 활성화한다.
- SPEC-ORCH-021 (dependency): 인터랙티브 pane 기본값 + subprocess best-effort floor. 본 SPEC은 그 floor를 보존하며 CLAUDECODE 진입만 연다.
- Sibling SPEC 없음 (단일 cohesive 활성화 작업, research.md `Sibling SPEC Decision` 참조).

## Out of Scope

provider-API 백엔드 신설, JSON 스키마/structured 출력 계약 변경, 새 strategy, SCAMPER·ICE 프롬프트 변경, plan·review·secure legacy 경로 변경, 화면 스크래핑 완료 감지 재설계, plain/CI subprocess floor 파괴, `creack/pty` 직접 의존성 승격은 이 SPEC의 범위 밖이다.

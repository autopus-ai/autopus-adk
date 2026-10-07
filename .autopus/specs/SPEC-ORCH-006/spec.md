# SPEC-ORCH-006: Orchestra 인터랙티브 pane 모드 — 멀티프로바이더 CLI 세션 실행 및 결과 자동 수집

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-26
**Domain**: ORCH

## 목적

SPEC-ORCH-001이 pane 분할 + sentinel 기반 비대화식 실행을 구현했으나, 현재 `buildPaneCommand()`가 프로바이더를 `-p`(pipe/print) 모드로 서브프로세스 실행하여 인터랙티브 세션이 아니다. 사용자는 각 프로바이더(claude, codex, gemini)가 실제 CLI 세션으로 진입하여 실행되는 모습을 실시간으로 모니터링하고 싶어한다.

이 SPEC은 cmux/tmux pane에서 프로바이더 CLI를 인터랙티브 세션으로 직접 실행하고, `pipe-pane`으로 출력을 스트리밍 캡처하며, `read-screen` 폴링으로 완료를 감지하고, 깨끗한 결과를 자동 수집하여 기존 merge 로직에 합류시키는 기능을 구현한다.

## 요구사항

### R1: Terminal 인터페이스 확장 (P0)

THE SYSTEM SHALL `pkg/terminal/terminal.go`의 `Terminal` 인터페이스에 다음 메서드를 추가한다:
- `ReadScreen(ctx context.Context, paneID PaneID, opts ReadScreenOpts) (string, error)` — pane 화면 내용 읽기
- `PipePaneStart(ctx context.Context, paneID PaneID, outputFile string) error` — pane 출력을 파일로 연속 스트리밍 시작
- `PipePaneStop(ctx context.Context, paneID PaneID) error` — pipe-pane 스트리밍 중지

### R2: CmuxAdapter 확장 (P0)

WHEN Terminal 인터페이스가 확장되면, THE SYSTEM SHALL `CmuxAdapter`에 다음 구현을 추가한다:
- `ReadScreen`: `cmux read-screen --surface <ref> [--scrollback] [--lines N]` 실행
- `PipePaneStart`: `cmux pipe-pane --surface <ref> --command "cat >> <file>"` 실행
- `PipePaneStop`: `cmux pipe-pane --surface <ref> --command ""` 실행 (빈 명령으로 중지)

### R3: TmuxAdapter 확장 (P0)

WHEN Terminal 인터페이스가 확장되면, THE SYSTEM SHALL `TmuxAdapter`에 다음 구현을 추가한다:
- `ReadScreen`: `tmux capture-pane -t <pane> -p` 실행
- `PipePaneStart`: `tmux pipe-pane -t <pane> -O "cat >> <file>"` 실행
- `PipePaneStop`: `tmux pipe-pane -t <pane>` 실행 (인자 없이 중지)

### R4: PlainAdapter no-op 구현 (P0)

WHEN Terminal 인터페이스가 확장되면, THE SYSTEM SHALL `PlainAdapter`에 각 새 메서드의 no-op 구현을 추가한다. `ReadScreen`은 빈 문자열, `PipePaneStart`/`PipePaneStop`은 nil error를 반환한다.

### R5: 인터랙티브 pane 실행 플로우 (P0)

WHEN `--multi` 플래그로 orchestra가 실행되고 cmux/tmux 터미널이 감지되면, THE SYSTEM SHALL 다음 순서로 인터랙티브 모드를 실행한다:
1. `splitProviderPanes()` — 기존 로직 재사용하여 pane 분할
2. `startPipeCapture()` — 각 pane에 `PipePaneStart`로 출력 스트리밍 시작
3. `launchInteractiveSessions()` — 각 pane에 프로바이더 바이너리 이름(`claude`, `codex`, `gemini`)을 `SendCommand`로 전송하여 인터랙티브 세션 진입
4. `waitForSessionReady()` — `ReadScreen` 폴링으로 세션 준비 감지 (프롬프트 입력 대기 상태)
5. `sendPrompts()` — 각 세션에 사용자 프롬프트를 `SendCommand`로 전송
6. `waitForCompletion()` — `ReadScreen` 폴링으로 완료 감지 (입력 프롬프트 재표시 또는 idle 감지)
7. `collectResults()` — `ReadScreen`으로 깨끗한 결과 수집
8. `mergeByStrategy()` — 기존 merge 로직 재사용
9. `cleanupPanes()` — 기존 정리 로직 재사용

### R6: pane_args 설정 지원 (P0)

THE SYSTEM SHALL `ProviderConfig.PaneArgs`를 인터랙티브 모드에서 활용한다. `pane_args`가 비어있으면 프로바이더 바이너리만 실행(인터랙티브 모드), `pane_args`가 설정되면 해당 인자와 함께 실행한다. `autopus.yaml`에 `pane_args` 필드를 문서화한다.

### R7: 완료 감지 전략 (P0)

WHILE 프로바이더 세션이 실행 중일 때, THE SYSTEM SHALL 다음 전략으로 완료를 감지한다:
- **Primary**: `ReadScreen` 폴링으로 CLI 입력 프롬프트 재표시 패턴 감지 (프로바이더별 프롬프트 패턴 설정 가능)
- **Secondary**: `pipe-pane` 출력 파일에서 idle 감지 (N초간 새 출력 없음, 기본 10초)
- **Fallback**: 설정된 타임아웃 도달 시 강제 종료

### R8: 기존 sentinel 모드 유지 (P0)

WHEN plain 터미널이거나 인터랙티브 모드가 실패하면, THE SYSTEM SHALL 기존 sentinel 기반 비대화식 `buildPaneCommand()` 모드로 자동 fallback한다. 기존 코드 경로는 변경하지 않는다.

### R9: 인터랙티브 세션 타임아웃 (P1)

WHEN 인터랙티브 세션이 설정된 타임아웃 내에 완료되지 않으면, THE SYSTEM SHALL 해당 pane을 종료하고, 타임아웃 시점까지의 부분 결과를 `ReadScreen`으로 수집하여 `ProviderResponse`에 `TimedOut: true`와 함께 기록한다.

### R10: 결과 품질 필터링 (P1)

WHEN `ReadScreen`으로 결과를 수집할 때, THE SYSTEM SHALL ANSI 이스케이프 시퀀스, CLI 프롬프트 장식, 불필요한 공백을 제거하여 깨끗한 텍스트만 merge 로직에 전달한다. ANSI 제거는 `\x1b\[[0-9;]*[a-zA-Z]` 정규식으로 처리하고, 프로바이더별 프롬프트 패턴(R7의 CompletionPatterns)을 사용하여 프롬프트 라인을 필터링한다.

### R10.1: ReadScreen과 PipePane 역할 분리 (P1)

- `ReadScreen`: on-demand 화면 캡처. 완료 감지(R7)와 최종 결과 수집(R5 Step 7)에 사용.
- `PipePaneStart/Stop`: 연속 출력 스트리밍. idle 감지(R7 Secondary)와 디버깅 로그 용도로 사용. 최종 결과의 primary source는 `ReadScreen`이다.

## 생성 파일 상세

### `pkg/terminal/terminal.go` (수정)
Terminal 인터페이스에 `ReadScreen`, `PipePaneStart`, `PipePaneStop` 메서드 추가. `ReadScreenOpts` 구조체 정의.

### `pkg/terminal/cmux.go` (수정)
CmuxAdapter에 `ReadScreen`, `PipePaneStart`, `PipePaneStop` 구현 추가.

### `pkg/terminal/tmux.go` (수정)
TmuxAdapter에 `ReadScreen`, `PipePaneStart`, `PipePaneStop` 구현 추가.

### `pkg/terminal/plain.go` (수정)
PlainAdapter에 새 메서드 no-op 구현 추가.

### `pkg/orchestra/interactive.go` (신규)
인터랙티브 pane 실행 로직. `RunInteractivePaneOrchestra()` 진입점, `startPipeCapture()`, `launchInteractiveSessions()`, `waitForSessionReady()`, `sendPrompts()`, `waitForCompletion()`, `collectResults()` 함수.

### `pkg/orchestra/interactive_detect.go` (신규)
완료 감지 로직. 프로바이더별 프롬프트 패턴 매칭, idle 감지, ANSI 스트립 유틸리티.

### `pkg/orchestra/pane_runner.go` (수정)
`RunPaneOrchestra()`에 인터랙티브 모드 분기 추가. `OrchestraConfig.Interactive` 플래그 확인.

### `pkg/orchestra/types.go` (수정)
`OrchestraConfig`에 `Interactive bool` 필드 추가. 프로바이더별 완료 패턴 설정 구조체 추가.

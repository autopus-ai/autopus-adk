# SPEC-ORCH-012: cmux SendLongText buffer 경로 및 launch command 통합

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-28
**Domain**: ORCH

## 목적

Orchestra interactive pane 모드에서 프롬프트가 잘려서 전달되는 버그를 수정한다. cmux의 `SendLongText`가 `SendCommand`에 직접 위임하여 CLI argument 크기 제한에 걸리고, launch command도 `SendCommand`로 전달되어 args 프로바이더(opencode)의 긴 프롬프트가 truncation된다.

- gemini: "It appears your message was cut off" 응답
- opencode Round 1: 정상 브레인스토밍 대신 "Conversation title request" 출력

tmux 어댑터는 이미 `load-buffer`/`paste-buffer` 경로를 사용하여 안전하지만, cmux 어댑터는 이 안전장치가 없다.

## 요구사항

### P0 — Must Have

- **FR-01**: WHEN cmux `SendLongText`가 500바이트 이상의 텍스트를 받으면, THE SYSTEM SHALL `cmux set-buffer --name <unique> <text>` + `cmux paste-buffer --name <unique> --surface <ref>` 경로를 사용하여 텍스트를 전달한다. 실제 truncation 원인은 `cmux send`의 PTY 버퍼 한계(~4KB)이며, `set-buffer`는 CLI arg 기반이지만 Go `exec.Command`가 shell bypass하고 OS ARG_MAX(~1MB)까지 지원하므로 500KB 텍스트까지 정상 동작 확인됨.
- **FR-02**: WHEN `launchInteractiveSessions`가 각 pane에 launch command를 전송하면, THE SYSTEM SHALL `SendCommand` 대신 `SendLongText`를 사용하여 command 본문을 전달하고, Enter(`\n`)는 별도 `SendCommand`로 분리한다.
- **FR-03**: WHILE 병렬 프로바이더 실행 중 여러 pane이 동시에 `SendLongText`를 호출하면, THE SYSTEM SHALL 고유 버퍼 이름(예: `autopus-<paneID>-<timestamp>`)을 생성하여 이름 충돌을 방지한다.

### P1 — Should Have

- **FR-10**: WHEN `set-buffer` 호출이 실패하면, THE SYSTEM SHALL warning 로그를 출력하고 기존 `SendCommand` 경로로 fallback한다.
- **FR-11**: WHEN buffer cleanup(`cmux delete-buffer`)가 실패하면, THE SYSTEM SHALL 에러를 무시하고 계속 진행한다 (best-effort).

### Out of Scope

- plain 터미널 어댑터 변경
- opencode를 interactive TUI 모드로 전환
- 비-interactive orchestra (runner.go) 경로 변경
- tmux `SendLongText` 변경 (이미 안전)
- `interactive_debate_helpers.go`의 judge `SendCommand` — judgment 텍스트는 짧음

## 생성 파일 상세

| 파일 | 변경 내용 |
|------|----------|
| `pkg/terminal/cmux.go` | `SendLongText` 구현: >=500B일 때 set-buffer/paste-buffer 경로, unique 버퍼 이름, fallback |
| `pkg/orchestra/interactive.go` | `launchInteractiveSessions`: SendCommand → SendLongText + 별도 Enter |
| `pkg/terminal/cmux_test.go` | SendLongText 테스트: short path, long path, fallback, unique naming |
| `pkg/orchestra/interactive_test.go` | launch 함수의 SendLongText 호출 검증 |

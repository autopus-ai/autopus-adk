# SPEC-ORCH-016: Interactive Pane Debate Round 2+ Surface 유효성 검증 및 Pane 재생성

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-29
**Domain**: ORCH

## 목적

Multi-round interactive debate(orchestra brainstorm --strategy debate)에서 Round 1 완료 후 일부 프로바이더 CLI(opencode, gemini)가 프로세스를 종료하여 cmux surface가 stale 상태가 되는 문제를 해결한다. Round 2에서 `paste-buffer`가 `exit status 1`로 실패하면 해당 프로바이더의 응답이 누락되어 토론 품질이 저하된다.

Claude는 세션을 유지하므로 이 문제가 발생하지 않지만, opencode와 gemini는 각 응답 후 CLI 프로세스가 종료될 수 있다.

## 요구사항

- **R1**: WHEN `executeRound` is called for round > 1, THE SYSTEM SHALL validate each pane's surface by attempting a lightweight ReadScreen call before sending the prompt. If ReadScreen returns an error, the surface SHALL be marked as invalid.

- **R2**: WHEN a pane's surface is detected as invalid in Round 2+, THE SYSTEM SHALL recreate the pane by: (a) closing the stale surface, (b) creating a new SplitPane, (c) restarting pipe capture, (d) relaunching the provider CLI session, and (e) waiting for session readiness. The new paneID SHALL replace the old paneID in the panes slice.

- **R3**: WHEN pane recreation succeeds, THE SYSTEM SHALL log the recreation event with the provider name, old paneID, and new paneID at INFO level.

- **R4**: WHEN pane recreation fails (SplitPane or session launch error), THE SYSTEM SHALL mark the provider as `skipWait = true` and log a WARNING, rather than aborting the entire debate.

- **R5**: WHERE a provider is known to maintain persistent sessions across rounds (e.g., `claude`), THE SYSTEM SHALL skip the surface validity check for that provider to avoid unnecessary overhead.

- **R6**: WHEN `SendLongText` fails on a previously-validated surface, THE SYSTEM SHALL attempt pane recreation once before marking the provider as skipWait (replacing the current retry-same-surface logic).

## 생성 파일 상세

| 파일 | 역할 |
|------|------|
| `pkg/orchestra/interactive_surface.go` | Surface 유효성 검증 및 pane 재생성 로직 |
| `pkg/orchestra/interactive_debate.go` | `executeRound` 수정: Round 2+ 진입 시 surface 검증 호출 |
| `pkg/terminal/terminal.go` | Terminal 인터페이스 변경 없음 (ReadScreen으로 검증 가능) |
| `pkg/orchestra/interactive_surface_test.go` | Surface 검증/재생성 단위 테스트 |

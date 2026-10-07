# SPEC-ORCH-015: Interactive Pane Mode 프로바이더별 완료 감지 안정화

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-28
**Domain**: ORCH

## 목적

Interactive pane mode에서 프로바이더별 프롬프트 패턴 매칭, ANSI strip, ReadScreen 타임아웃, opencode stdin 모드 전환, waitForSessionReady shell 오탐 문제를 통합 안정화한다. 이미 적용된 핫픽스(R1-R4)를 정식 요구사항으로 문서화하고 테스트를 보강하며, 미해결 이슈(R5-R8)를 신규 구현한다.

## 관련 SPEC

- SPEC-ORCH-014: opencode TUI pane 인터랙티브 모드 전환 (completed)
- SPEC-ORCH-013: Orchestra Interactive Debate 안정성 개선 (completed)

## 요구사항

### R1: isPromptVisible ANSI strip [이미 구현됨 — 테스트 보강 필요]

WHEN `isPromptVisible` is called with raw ReadScreen output containing ANSI escape codes,
THE SYSTEM SHALL strip all ANSI escape sequences via `stripANSI()` before pattern matching,
SO THAT color-coded prompts (e.g. `\x1b[32m❯\x1b[0m`) are correctly detected.

- 구현 위치: `interactive_detect.go:109`
- 커밋: `3815010`

### R2: ReadScreen fresh context on timeout [이미 구현됨 — 테스트 보강 필요]

WHEN `waitForCompletion` times out and the original context is cancelled,
THE SYSTEM SHALL create a fresh `context.Background()` with 5-second timeout for the final ReadScreen call,
SO THAT the last screen capture succeeds even after parent context cancellation.

- 구현 위치: `interactive.go:241-243`
- 커밋: `459a341`

### R3: opencode PromptViaArgs=false [이미 구현됨 — 테스트 보강 필요]

WHEN opencode provider is configured for non-pane mode,
THE SYSTEM SHALL deliver prompts via stdin (PromptViaArgs=false) instead of CLI arguments,
SO THAT ENAMETOOLONG errors are avoided when opencode uses the arg as a filename.

- 구현 위치: `defaults.go:68`, `migrate.go` (MigrateOpencodeToTUI), `orchestra_helpers.go:99`
- 커밋: `459a341`

### R4: 프로바이더별 프롬프트 패턴 [이미 구현됨 — 테스트 보강 필요]

WHEN detecting provider prompt readiness or completion,
THE SYSTEM SHALL use provider-specific patterns:
- claude: `❯` (unicode heavy right-pointing angle)
- gemini: `> Type your` or `@`
- opencode: `Ask anything`
- codex: `codex>`

SO THAT each provider's actual TUI prompt is correctly matched instead of relying on generic `^>\s*$`.

- 구현 위치: `types.go:96-102`, `interactive_detect.go:20-27`
- 커밋: `af920d7`

### R5: waitForSessionReady shell `$` 패턴 제거 [신규 구현]

WHEN `waitForSessionReady` polls for CLI startup readiness,
THE SYSTEM SHALL use a dedicated pattern set containing only CLI-specific prompts (claude `❯`, gemini `> Type your`, opencode `Ask anything`, codex `codex>`),
AND SHALL exclude shell prompts (`^\$\s*$`, `^#\s*$`),
SO THAT pane creation 직후 shell `$` 프롬프트에 false match하여 CLI 미시작 상태에서 프롬프트를 전송하는 문제가 방지된다.

### R6: 프로바이더별 startup timeout [신규 구현]

WHEN `waitForSessionReady` polls for CLI startup,
THE SYSTEM SHALL apply provider-specific startup timeouts:
- claude: 15초 (MCP 로딩)
- gemini: 10초 (인증 확인)
- opencode: 5초 (TUI 초기화)
- default: 30초

SO THAT 느린 프로바이더는 충분한 대기 시간을 갖고, 빠른 프로바이더는 불필요한 대기를 하지 않는다.

### R7: opencode 응답 완료 감지 개선 [신규 구현]

WHEN opencode provider finishes generating a response and the `Ask anything` placeholder reappears,
THE SYSTEM SHALL detect completion via the existing 2-phase consecutive match mechanism,
AND WHERE the 2-phase match fails within 30초,
THE SYSTEM SHALL fall back to pipe-pane output file idle detection (`isOutputIdle`) with 15초 threshold,
SO THAT opencode의 TUI 렌더링 타이밍 이슈로 인한 완료 감지 실패가 보완된다.

### R8: debate 멀티라운드 응답 수집 안정화 [신규 구현]

WHEN `executeRound` sends prompts to providers in round > 1,
THE SYSTEM SHALL log errors from `SendLongText` and `SendCommand` instead of silently ignoring them (`_ =`),
AND SHALL retry failed prompt delivery once (1회 재시도) before marking the provider as skipped,
AND WHEN a provider returns an empty response,
THE SYSTEM SHALL include the provider in the round result with `EmptyOutput=true` for partial result merge,
SO THAT debate 멀티라운드에서 프롬프트 전달 실패와 빈 응답이 적절히 핸들링된다.

## 영향 파일

| 파일 | 변경 유형 | 비고 |
|------|----------|------|
| `pkg/orchestra/interactive_detect.go` | 수정 | R1(확인), R5(신규 패턴셋) |
| `pkg/orchestra/interactive_completion.go` | 수정 | R7(idle fallback) |
| `pkg/orchestra/interactive.go` | 수정 | R2(확인), R5(전용 패턴), R6(프로바이더별 timeout) |
| `pkg/orchestra/interactive_debate.go` | 수정 | R8(에러 로깅, 재시도, 빈 응답) |
| `pkg/orchestra/types.go` | 수정 | R4(확인), R6(StartupTimeout 필드) |
| `pkg/config/defaults.go` | 확인 | R3(확인) |
| `pkg/config/migrate.go` | 확인 | R3(확인) |
| `internal/cli/orchestra_helpers.go` | 확인 | R3(확인) |

# SPEC-ORCH-010: Orchestra 멀티턴 토론 P0 버그 수정 및 Pane 안정성 개선

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-27
**Domain**: ORCH
**Extends**: SPEC-ORCH-009
**Origin**: BS-009

## 목적

멀티턴 토론(debate) 모드에서 발견된 3가지 P0 버그를 수정한다:
1. Claude 프로바이더가 pane 내에서 permission 프롬프트를 표시하여 실행이 무한 대기에 빠지는 문제
2. 프로바이더가 프로젝트 디렉토리의 기존 BS 파일을 읽고 주어진 토론 주제 대신 해당 파일 내용을 토론하는 topic drift 문제
3. ReadScreen이 응답 생성 중에 completion으로 오판하여 불완전한 출력을 수집하는 truncation 문제

추가로 후반 라운드의 rebuttal 프롬프트 토큰 폭발을 방지하는 컨텍스트 요약과
라운드 수를 CLI에서 제어하는 `--rounds` 플래그를 구현한다.

## 요구사항

### P0: REQ-1 — Permission bypass for orchestra pane sessions

> WHEN the system launches interactive CLI sessions in terminal panes for orchestra debate,
> THE SYSTEM SHALL include `--dangerously-skip-permissions` in the launch arguments for Claude provider
> so that tool-use permission prompts do not block pane execution.

- `buildInteractiveLaunchCmd()` in `interactive.go`에 Claude binary 감지 시 해당 플래그 추가
- 다른 프로바이더(opencode, gemini)는 영향 없음 — Claude 전용 플래그
- 플래그는 PaneArgs에 이미 포함된 경우 중복 추가하지 않음

### P0: REQ-2 — Topic isolation for brainstorm debate prompts

> WHEN the system sends a debate prompt to providers in brainstorm mode,
> THE SYSTEM SHALL prepend a topic isolation instruction that directs providers to discuss ONLY the given topic
> and explicitly prohibits reading or referencing existing project files.

- `buildRebuttalPrompt()` 및 round 1 prompt 전송 시 isolation prefix 삽입
- Isolation instruction 예시: "IMPORTANT: Discuss ONLY the topic below. Do NOT read, reference, or analyze any existing files in the project directory."
- `executeRound()` in `interactive_debate.go`에서 prompt wrapping 적용

### P0: REQ-3 — Completion detection accuracy improvement

> WHEN the system polls for provider completion via ReadScreen,
> THE SYSTEM SHALL use a two-phase detection strategy: (1) initial cooldown period, then (2) consecutive prompt pattern matches
> to prevent false-positive completion on partial output.

- `waitForCompletion()` in `interactive.go`: 단일 prompt match 대신 2회 연속 match 필요 (2초 간격)
- Round 1의 initial delay를 15초에서 configurable하게 변경 (기본 20초)
- `executeRound()`의 initial delay를 5초에서 10초로 증가
- `isPromptVisible()` 함수는 변경 없음 — 호출 측 로직만 수정

### P1: REQ-4 — Rebuttal context summarization

> WHEN building a rebuttal prompt for round 3 or later,
> THE SYSTEM SHALL summarize previous round responses to a maximum of 500 characters per provider
> to prevent token overflow in later rounds.

- `buildRebuttalPrompt()` in `debate.go`에 round number 파라미터 추가
- Round >= 3일 때 각 provider의 Output을 500자로 truncate하고 "[...truncated]" 표시
- Round 1-2는 기존 동작 유지 (전체 출력 포함)

### P1: REQ-5 — Per-round configurable timeout

> WHEN the system calculates per-round timeout for debate,
> THE SYSTEM SHALL allow a minimum per-round timeout of 45 seconds
> regardless of total timeout and round count.

- `perRoundTimeout()` in `interactive_debate_helpers.go`에 최소값 45초 적용
- 기존 공식: `totalSeconds / rounds` → 수정: `max(totalSeconds / rounds, 45)`

### P2: REQ-6 — --rounds N CLI flag for brainstorm

> WHEN the user executes the brainstorm command,
> THE SYSTEM SHALL accept a `--rounds N` flag to override the default debate round count.

- `internal/cli/orchestra_brainstorm.go`에 `--rounds` 플래그 추가
- 값 범위: 1-10 (기존 validation 재사용)
- SPEC-ORCH-009의 REQ-4에서 이미 구현되었으나, 본 SPEC에서는 동작 확인만 수행

### P2: REQ-7 — Early consensus detection with configurable threshold

> WHERE the debate strategy is active and round count >= 3,
> THE SYSTEM SHALL use a configurable consensus threshold (default 0.66) via `--consensus-threshold` flag
> to allow users to tune early termination sensitivity.

- 기존 `consensusReached()` 함수의 하드코딩된 0.66을 OrchestraConfig 필드로 이동
- CLI에서 선택적 `--consensus-threshold` 플래그 지원

## 생성 파일 상세

| 파일 | 역할 | 변경량 |
|------|------|--------|
| `pkg/orchestra/interactive.go` | `buildInteractiveLaunchCmd()` permission bypass, `waitForCompletion()` 2-phase detection | ~25줄 수정 |
| `pkg/orchestra/interactive_debate.go` | `executeRound()` topic isolation, initial delay 증가 | ~15줄 수정 |
| `pkg/orchestra/debate.go` | `buildRebuttalPrompt()` round-aware summarization | ~20줄 수정 |
| `pkg/orchestra/interactive_debate_helpers.go` | `perRoundTimeout()` 최소값 적용, `consensusReached()` threshold 파라미터화 | ~10줄 수정 |
| `pkg/orchestra/config.go` (또는 types) | `OrchestraConfig`에 `ConsensusThreshold`, `InitialDelay` 필드 추가 | ~5줄 수정 |
| `pkg/orchestra/interactive_test.go` | 2-phase completion detection 테스트 | ~40줄 추가 |
| `pkg/orchestra/debate_test.go` | round-aware rebuttal prompt 테스트 | ~30줄 추가 |

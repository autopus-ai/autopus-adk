# SPEC-ORCH-013: Orchestra Interactive Debate 안정성 개선

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-28
**Domain**: ORCH

## 목적

v0.19.3~v0.20.1 테스트 세션에서 발견된 interactive debate의 4가지 안정성 이슈를 해결한다.
핵심 문제는 timeout 기반 제어가 이벤트 기반 제어보다 우선하여 judge 실행 실패, ReadScreen
false-positive 완료 감지, diff 섹션 noise 노출, opencode TUI 잔여물 출력이 발생하는 것이다.

## 요구사항

### R1: Debate/Judge Timeout 분리 (이벤트 기반)

- WHEN debate rounds가 완료되고 judge provider가 설정되어 있을 때,
  THE SYSTEM SHALL judge 실행에 독립적인 context를 사용하되
  timeout은 hang 방지 안전망으로만 작동하고,
  `cmd.Run()` 반환(이벤트)이 primary completion signal이 되어야 한다.

- WHEN `perRoundTimeout`이 debate round에 할당될 때,
  THE SYSTEM SHALL judge timeout budget을 debate budget과 완전히 분리하여
  debate가 전체 timeout을 소진해도 judge가 독립적으로 실행될 수 있어야 한다.

- WHEN judge subprocess가 실행될 때,
  THE SYSTEM SHALL `context.Background()` 기반의 fresh context를 사용하고
  configurable timeout (최소 60초, 기본 cfg.TimeoutSeconds)을 safety net으로 설정해야 한다.

### R2: Claude Pane ReadScreen 수집 타이밍 개선

- WHEN `waitForCompletion`이 pane 완료를 감지할 때,
  THE SYSTEM SHALL 프롬프트 전송 직전의 screen snapshot을 기록하고,
  screen content가 변경된 후(snapshot과 다름) prompt가 재등장할 때만 완료로 판정해야 한다.

- WHEN 이전 라운드의 prompt `> `가 이미 화면에 보이는 상태에서 새 prompt를 전송할 때,
  THE SYSTEM SHALL 즉시 false-positive 완료 판정을 하지 않고,
  screen content 변화를 먼저 감지한 후 2-phase consecutive match를 적용해야 한다.

### R3: Gemini Diff 섹션 원본 Noise 정제

- WHEN `FormatDebate`가 "주요 차이점" diff 섹션을 생성할 때,
  THE SYSTEM SHALL `findDifferences`에 전달하는 response.Output에
  `cleanScreenOutput`을 적용하여 noise가 제거된 텍스트로 비교해야 한다.

- WHEN diff 생성 시 원본 output이 MCP noise, ANSI escape, TUI fragment를 포함할 때,
  THE SYSTEM SHALL 해당 noise가 diff 결과에 노출되지 않도록 사전 정제해야 한다.

### R4: OpenCode Interactive Pane 출력 정제

- WHEN opencode pane의 screen output을 수집할 때,
  THE SYSTEM SHALL shell login banner (`Last login:` 패턴), user@host prompt
  (`username@hostname` 패턴), opencode TUI chrome (`Build · gpt`, `⬝⬝⬝⬝ esc` 등)을
  line-level filtering으로 제거해야 한다.

- WHEN `cliNoisePatterns`에 새 패턴을 추가할 때,
  THE SYSTEM SHALL 기존 패턴과의 중복을 방지하고
  regex 컴파일 실패가 없도록 테스트를 포함해야 한다.

## 생성 파일 상세

| 파일 | 역할 | 변경 유형 |
|------|------|----------|
| `pkg/orchestra/interactive_debate_helpers.go` | R1: `runJudgeRound` judge context 분리 확인/강화 | 수정 |
| `pkg/orchestra/interactive_debate.go` | R1: debate round context와 judge context 완전 분리 | 수정 |
| `pkg/orchestra/interactive.go` | R2: `waitForCompletion`에 screen snapshot 기반 변화 감지 추가 | 수정 |
| `pkg/orchestra/interactive_detect.go` | R3+R4: cleanScreenOutput 활용 및 shell login, user@host prompt noise 패턴 추가 | 수정 |
| `pkg/orchestra/merger.go` | R3: `FormatDebate`/`findDifferences`에서 clean output 사용 | 수정 |

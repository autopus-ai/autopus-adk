# SPEC-ORCH-008: Interactive 멀티턴 핑퐁 Debate + Hook 자동주입

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-27
**Domain**: ORCH
**Extends**: SPEC-ORCH-007 (Hook 기반 멀티프로바이더 오케스트레이션)
**Origin**: BS-003 (Interactive 멀티턴 핑퐁 Debate 브레인스토밍)
**PRD**: [prd.md](./prd.md) (14 requirements: P0:7, P1:4, P2:3)

---

## 목적

SPEC-ORCH-007이 완성한 hook 파일 시그널 인프라 위에 멀티턴 debate 루프를 구축한다.
현재 `interactive.go`는 단일 프롬프트 전송 후 결과를 1회 수집하고 종료하며,
`debate.go`의 rebuttal은 새 프로세스를 spawn하여 pane 세션을 활용하지 못한다.
이 SPEC은 interactive pane에서 N라운드 핑퐁 토론을 실행하고,
라운드별 hook 시그널로 결과를 수집하며, rebuttal 프롬프트를 pane에 재주입하는 기능을 구현한다.

---

## 요구사항

> 전체 요구사항의 상세 설명은 [prd.md](./prd.md) Section 5 참조.

### P0 — Must Have

| ID | EARS 요구사항 |
|----|--------------|
| R1 | WHEN debate 전략이 interactive 모드에서 실행될 때, THE SYSTEM SHALL `DebateRounds` 횟수만큼 라운드 루프를 실행하고 각 라운드의 결과를 hook 시그널로 수집한다. |
| R2 | WHEN 멀티턴 debate(rounds >= 2)가 실행될 때, THE SYSTEM SHALL hook 시그널 파일에 라운드 번호를 포함하고(`{provider}-round{N}-done`), `AUTOPUS_ROUND` 환경변수를 설정한다. |
| R3 | WHEN Round N(N >= 2)이 시작될 때, THE SYSTEM SHALL 각 프로바이더 pane에 `buildRebuttalPrompt()`로 생성한 rebuttal 프롬프트를 주입하고, pane 입력 대기 상태를 확인한 후 전송한다. |
| R4 | WHEN 사용자가 `--rounds N`을 지정하면, THE SYSTEM SHALL N을 `OrchestraConfig.DebateRounds`에 설정하고, 유효 범위(1-10)를 검증하며, `--strategy debate` 외 전략에서는 오류를 반환한다. |
| R5 | WHEN `AUTOPUS_ROUND` 환경변수가 설정되어 있으면, THE SYSTEM SHALL 각 hook 스크립트가 시그널 파일명에 라운드 번호를 포함하고, 미설정 시 기존 형식으로 fallback한다. |
| R6 | WHEN 라운드 N이 완료되고 N+1이 시작되기 전에, THE SYSTEM SHALL 라운드 N의 done 시그널을 삭제하고, 결과 파일은 보존하며, `AUTOPUS_ROUND`를 N+1로 업데이트한다. |
| R7 | WHEN 모든 debate 라운드가 완료되면, THE SYSTEM SHALL 최종 라운드 응답으로 `mergeByStrategy()`를 호출하고, 전체 라운드 히스토리를 `OrchestraResult`에 포함한다. |

### P1 — Should Have

| ID | EARS 요구사항 |
|----|--------------|
| R8 | WHEN `JudgeProvider`가 설정되고 모든 라운드가 완료되면, THE SYSTEM SHALL judge pane에 `buildJudgmentPrompt()`로 판정 프롬프트를 전송하고, hook 시그널로 수집한다. |
| R9 | WHILE debate 라운드가 진행 중일 때, THE SYSTEM SHALL 각 라운드의 시작/완료 메시지와 전체 요약을 stdout에 출력한다. |
| R10 | WHEN 라운드 N 완료 후 모든 프로바이더 응답이 실질적으로 동일하면, THE SYSTEM SHALL 남은 라운드를 건너뛰고 조기 합의로 종료한다. |
| R11 | WHEN debate 전략이 interactive 멀티턴으로 실행될 때, THE SYSTEM SHALL `OrchestraResult`에 `RoundHistory [][]ProviderResponse` 필드를 추가하고 라운드별 응답을 저장한다. |

### P2 — Could Have

| ID | EARS 요구사항 |
|----|--------------|
| R12 | WHEN 모든 라운드가 완료되면, THE SYSTEM SHALL 라운드별 응답 변화를 요약하는 비교 뷰를 제공한다. |
| R13 | WHERE `PerRoundTimeout`이 설정되면, THE SYSTEM SHALL 전체 타임아웃과 별개로 각 라운드에 개별 타임아웃을 적용한다. |
| R14 | WHEN debate가 완료되면, THE SYSTEM SHALL 전체 토론 기록을 `.autopus/debate-history/{session-id}.json`에 저장한다. |

---

## 생성/수정 파일 상세

### 신규 파일

| 파일 | 역할 | 줄 수 목표 |
|------|------|-----------|
| `pkg/orchestra/interactive_debate.go` | Interactive debate 라운드 루프 메인 로직 (`runInteractiveDebate()`) | < 200 |
| `pkg/orchestra/round_signal.go` | 라운드 스코프 시그널 관리 (파일명 생성, 시그널 정리, env 업데이트) | < 100 |
| `pkg/orchestra/interactive_debate_test.go` | Interactive debate 단위 테스트 | < 200 |
| `pkg/orchestra/round_signal_test.go` | 라운드 시그널 단위 테스트 | < 150 |

### 수정 파일

| 파일 | 변경 내용 | 변경 규모 |
|------|----------|----------|
| `pkg/orchestra/interactive.go` | debate 전략 분기 추가 (Step 5 이후) | ~15줄 |
| `pkg/orchestra/hook_signal.go` | `WaitForDone()`에 round 파라미터 추가, 라운드별 파일명 지원 | ~25줄 |
| `pkg/orchestra/types.go` | `OrchestraResult.RoundHistory` 필드 추가 | ~5줄 |
| `internal/cli/orchestra.go` | `--rounds` 플래그 추가 및 검증 로직 | ~20줄 |
| `content/hooks/hook-claude-stop.sh` | `AUTOPUS_ROUND` 인식 파일명 분기 | ~8줄 |
| `content/hooks/hook-gemini-afteragent.sh` | `AUTOPUS_ROUND` 인식 파일명 분기 | ~8줄 |
| `content/hooks/hook-opencode-complete.ts` | `AUTOPUS_ROUND` 인식 파일명 분기 | ~8줄 |

---

## 의존성

- **SPEC-ORCH-007** (completed): hook 파일 시그널 프로토콜, HookSession, WaitAndCollectHookResults
- **debate.go**: `buildRebuttalPrompt()`, `buildJudgmentPrompt()`, `buildDebateMerged()` 재사용
- **interactive.go**: `splitProviderPanes()`, `startPipeCapture()`, `pollUntilPrompt()`, `sendPrompts()` 재사용

---

## 하위 호환성

- `rounds=1` 또는 미지정: 기존 SPEC-ORCH-007 단일 수집 흐름과 동일 동작
- `AUTOPUS_ROUND` 미설정: hook 스크립트가 기존 `{provider}-done` 형식 유지
- 비대화식 debate (`runDebate()`): 기존 `runProvider()` 기반 흐름 변경 없음

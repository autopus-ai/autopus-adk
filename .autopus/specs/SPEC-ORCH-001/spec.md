# SPEC-ORCH-001: cmux Orchestra 연동 — 멀티프로바이더 실행 시 cmux 창분할 자동화

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-25
**Domain**: ORCH

## 목적

현재 `auto orchestra brainstorm --strategy debate` 등 멀티프로바이더 오케스트레이션 실행 시, claude/codex CLI를 `-p`(non-interactive) 모드로 실행하면 빈 응답이나 에러가 발생하여 사실상 단일 프로바이더(gemini)만 동작한다. 사용자가 cmux 터미널 멀티플렉서를 메인으로 사용 중이므로, cmux 감지 시 각 프로바이더를 별도 pane에서 인터랙티브 모드로 실행하고 결과를 수집하여 기존 merge/judge 로직에 합류시킨다.

이를 통해 claude, codex, gemini 등 모든 프로바이더가 정상적인 대화형 세션으로 풍부한 출력을 생성하며, 사용자가 각 프로바이더의 실행 과정을 실시간 시각적으로 모니터링할 수 있다.

## 요구사항

### R1: cmux 감지 및 모드 분기 (P0)
WHEN orchestra 실행이 시작되면, THE SYSTEM SHALL `pkg/terminal.DetectTerminal()`을 호출하여 cmux 사용 가능 여부를 판별한다.
- cmux 감지 시: pane 분할 인터랙티브 모드로 전환
- cmux 미감지 시: 기존 `-p` 비인터랙티브 모드 유지 (fallback)

### R2: 프로바이더별 pane 자동 분할 (P0)
WHEN cmux 인터랙티브 모드로 진입하면, THE SYSTEM SHALL 프로바이더 수(N)만큼 pane을 수평 분할로 생성하고, 각 pane에 프로바이더 이름을 식별할 수 있도록 한다.

### R3: pane에서 인터랙티브 CLI 실행 (P0)
WHEN pane이 생성되면, THE SYSTEM SHALL 각 pane에서 해당 프로바이더 CLI를 인터랙티브 모드로 실행한다:
- claude: `claude` (인터랙티브, `-p` 없이)
- codex: `codex` (인터랙티브)
- gemini: `gemini` (인터랙티브)

THE SYSTEM SHALL 프롬프트를 각 pane에 stdin(send-keys)으로 전송하고, 실행 완료를 대기한다.

### R4: 결과 수집 및 merge (P0)
WHEN 모든 프로바이더가 완료되면, THE SYSTEM SHALL 각 pane의 출력을 파일 리디렉션으로 캡처하고, 기존 merge/judge 로직(consensus, debate 등)을 재활용하여 통합 결과를 메인 pane에 표시한다.

### R5: pane 정리 (P0)
WHEN orchestra 실행이 완료되면, THE SYSTEM SHALL 생성된 모든 pane을 자동으로 닫고 메인 pane으로 포커스를 복귀한다.

### R6: fallback 안전성 (P0)
WHEN cmux가 미감지되거나 pane 생성에 실패하면, THE SYSTEM SHALL 기존 `-p` 비인터랙티브 모드로 자동 fallback하여 현재 동작을 유지한다.

### R7: 출력 캡처 신뢰성 (P1)
WHILE 프로바이더가 pane에서 실행 중일 때, THE SYSTEM SHALL 각 프로바이더의 stdout을 임시 파일로 리디렉션하고, 완료 마커(sentinel)를 사용하여 실행 완료를 정확히 감지한다.

### R8: 타임아웃 처리 (P1)
WHEN 프로바이더가 설정된 타임아웃 내에 완료되지 않으면, THE SYSTEM SHALL 해당 pane을 강제 종료하고, 타임아웃된 프로바이더를 FailedProvider로 기록한다.

## 생성 파일 상세

### `pkg/orchestra/pane_runner.go` (신규)
cmux pane 기반 프로바이더 실행 로직. Terminal 인터페이스를 주입받아 pane 생성, 명령 전송, 결과 수집, pane 정리를 담당한다.

### `pkg/orchestra/pane_runner_test.go` (신규)
pane_runner의 단위 테스트. mock Terminal을 사용하여 pane 생성, 명령 전송, 결과 수집, fallback 시나리오를 검증한다.

### `pkg/orchestra/runner.go` (수정)
`RunOrchestra()` 진입점에 Terminal 감지 및 모드 분기 로직 추가. cmux 사용 가능 시 `runParallelWithPanes()`로 위임.

### `pkg/orchestra/types.go` (수정)
`OrchestraConfig`에 `Terminal` 필드 추가 (optional, nil이면 기존 모드).

### `internal/cli/orchestra.go` (수정)
`runOrchestraCommand()`에서 `DetectTerminal()`을 호출하여 `OrchestraConfig.Terminal`에 주입.

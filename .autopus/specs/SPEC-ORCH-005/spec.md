# SPEC-ORCH-005: Orchestra Relay Pane Mode

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-26
**Domain**: ORCH

## 목적

SPEC-ORCH-004는 relay 전략을 standard execution(non-interactive, `-p` 모드)으로 구현했다. 그러나 relay의 순차적 특성은 pane 기반 인터랙티브 실행에 자연스럽게 적합하다. 사용자가 각 프로바이더의 실행 과정을 pane에서 실시간으로 관찰하고, 프로바이더가 전체 TUI/인터랙티브 기능(파일 편집, 대화형 확인 등)을 사용할 수 있게 하면 relay의 가치가 크게 향상된다.

현재 pane runner(`pane_runner.go`)는 consensus/debate/fastest 등 **병렬** 전략을 위해 설계되었다. 모든 pane을 동시에 열고 병렬로 sentinel을 대기한다. relay는 **순차적**이므로 pane을 하나씩 열고, 완료를 대기한 후, 결과를 수집하여 다음 pane에 주입해야 한다.

이 SPEC은 relay 전략에서 cmux/tmux pane 기반 인터랙티브 실행을 지원하며, `-p` 플래그 없이 프로바이더 CLI를 실행하여 전체 인터랙티브 기능을 활용할 수 있게 한다.

## 요구사항

### REQ-1: Relay Pane Mode 활성화

WHEN the terminal is pane-capable (cmux or tmux) AND the strategy is relay,
THE SYSTEM SHALL pane 기반 순차 인터랙티브 실행을 수행하고, 기존 "relay pane mode not yet supported" 경고 및 standard execution fallback을 제거한다.

### REQ-2: 순차 Pane 실행

WHEN relay pane mode is active,
THE SYSTEM SHALL 프로바이더를 설정 순서대로 순차적으로 pane에서 실행한다. 구체적으로:
1. 현재 프로바이더용 pane을 SplitPane으로 생성한다
2. 프로바이더 CLI를 `-p` 플래그 없이(인터랙티브 모드) pane에서 실행한다
3. sentinel 마커를 통해 완료를 감지한다
4. 출력 파일에서 결과를 수집한다
5. 다음 프로바이더 pane의 프롬프트에 이전 결과를 주입한다

WHERE a provider fails or times out, THE SYSTEM SHALL 해당 프로바이더를 건너뛰고 다음으로 계속 진행한다 (SPEC-ORCH-004 REQ-3a 패턴 동일).

### REQ-3: 인터랙티브 프롬프트 주입

WHEN a relay pane is opened for a provider other than the first,
THE SYSTEM SHALL 이전 프로바이더들의 결과를 `## Previous Analysis by {provider}` 형식으로 프롬프트에 포함하여 pane 명령의 CLI 인수로 전달한다.

프롬프트 전달 방식은 **CLI 인수** 기반이다 (heredoc/stdin이 아닌):
- 프롬프트를 임시 파일(`{relayDir}/prompt-{provider}.md`)에 저장
- 프로바이더 CLI를 `{binary} {prompt_file_path}` 또는 프로바이더별 파일 입력 인수로 실행
- 이렇게 하면 프로바이더가 인터랙티브 세션을 유지하면서 초기 프롬프트를 받을 수 있다

보안 처리: 기존 `buildPaneCommand`의 `shellEscapeArg` (SEC-001, SEC-004, SEC-006)를 준수한다.

### REQ-4: Pane 명령 구성 — 비-agentic 인터랙티브 모드

WHEN building the pane command for relay mode,
THE SYSTEM SHALL `-p` 플래그를 제거하고 프로바이더의 기본 인터랙티브 모드 인수를 사용한다. 프로바이더별 pane relay 인수:

| Provider | Standard Relay Args (`-p` mode) | Pane Relay Args (interactive) |
|----------|-------------------------------|-------------------------------|
| claude | `claude -p` + agentic flags | `claude` (인터랙티브 TUI, stdin으로 프롬프트) |
| codex | `codex -q` + agentic flags | `codex` (인터랙티브 모드) |
| gemini | `gemini -p "{prompt}"` | `gemini` (인터랙티브 모드, 프롬프트 stdin) |

WHERE a provider has `PaneArgs` configured in `ProviderConfig`, THE SYSTEM SHALL `PaneArgs`를 우선 사용한다.

### REQ-5: Relay Pane용 Sentinel 완료 감지

WHEN a provider finishes execution in a relay pane,
THE SYSTEM SHALL 기존 sentinel 메커니즘(`__AUTOPUS_DONE__` 마커, `waitForSentinel` 폴링)을 재사용하여 완료를 감지한다. sentinel은 output 파일에 기록되며, 500ms 간격으로 폴링한다.

완료 감지 흐름:
1. pane 명령은 `{binary} {args} ; echo __AUTOPUS_DONE__ >> {output_file}` 형식으로 구성된다
2. 프로바이더가 자연 종료(exit)하면 shell이 sentinel을 output 파일에 append한다
3. `waitForSentinel`이 파일을 폴링하여 sentinel 마커를 감지한다
4. 감지 후 output 파일에서 sentinel 이전 내용을 결과로 수집한다

이 방식은 **자동 완료 감지**이다 — 프로바이더가 작업을 마치고 exit하면 자동으로 다음 단계로 진행한다. 사용자의 수동 개입은 필요하지 않다.

### REQ-6: Relay Temp 파일 관리

WHEN relay pane mode executes,
THE SYSTEM SHALL 각 프로바이더의 출력을 `os.TempDir()/autopus-relay-{jobID}/{provider}.md`에 저장한다 (SPEC-ORCH-004 REQ-3과 동일한 경로 패턴, `os.TempDir()` 사용으로 macOS/Windows 호환). pane output 파일에서 읽은 내용을 relay temp 파일에 복사하여, standard relay와 동일한 출력 구조를 유지한다.

### REQ-7: Pane 라이프사이클 관리

WHEN relay pane mode completes (성공 또는 실패),
THE SYSTEM SHALL 모든 생성된 pane과 임시 파일을 정리한다. 순차 실행이므로 이전 pane을 유지할지 닫을지는 다음 규칙을 따른다:
- 현재 실행 중인 pane만 활성 상태로 유지한다
- 완료된 이전 pane은 기본적으로 유지하되, 타임아웃이 임박하면 닫는다
- 전체 실행 완료 후 defer로 모든 pane을 정리한다

### REQ-8: runner.go Fallback 제거

WHEN this SPEC is implemented,
THE SYSTEM SHALL `runner.go`의 relay pane fallback 코드(L28-31)를 제거하고, relay 전략도 다른 전략과 동일하게 `RunPaneOrchestra`로 라우팅한다.

### REQ-9: 파일 크기 제한 준수

WHILE implementing relay pane mode,
THE SYSTEM SHALL 새로운 코드를 `relay_pane.go`로 분리하여 `pane_runner.go`의 300줄 제한 초과를 방지한다. relay pane 전용 로직은 독립 파일로 유지한다.

### REQ-10: Backward Compatibility

WHILE relay pane mode is added,
THE SYSTEM SHALL 기존 relay standard execution(터미널 없는 환경, plain 터미널)과 다른 전략의 pane 실행에 어떠한 영향도 주지 않는다.

## 생성 파일 상세

| 파일 | 위치 | 역할 |
|------|------|------|
| `relay_pane.go` | `pkg/orchestra/relay_pane.go` | relay pane mode 핵심 로직: 순차 pane 생성, 인터랙티브 실행, 결과 수집, 맥락 주입 |
| `relay_pane_test.go` | `pkg/orchestra/relay_pane_test.go` | relay pane mode 유닛 테스트 |
| `runner.go` 수정 | `pkg/orchestra/runner.go` | relay pane fallback 제거 (L28-31) |
| `pane_runner.go` 수정 | `pkg/orchestra/pane_runner.go` | `mergeByStrategy`에 relay pane 결과 병합 지원 추가 (필요시) |
| `types.go` 수정 | `pkg/orchestra/types.go` | `ProviderConfig`에 relay pane 전용 인수 필드 추가 (필요시) |

# SPEC-ORCH-014: opencode Interactive TUI Pane Mode

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-28
**Domain**: ORCH

## 목적

opencode 프로바이더를 orchestra 엔진의 interactive TUI pane 모드에서 지원하여,
claude/gemini와 동일하게 cmux pane 내 멀티턴 ping-pong debate와 세션 유지가 가능하도록 한다.

현재 opencode는 `interactive_input: "args"` 설정으로 인해 매 debate 라운드마다
`opencode run -m "openai/gpt-5.4" 'prompt'` 형태로 재실행되어 상태가 비유지된다.
claude/gemini는 TUI 세션을 유지하며 SendLongText로 프롬프트를 전달받아 멀티턴 대화가 가능하다.

검증 결과, opencode TUI는 cmux pane에서 정상 실행되고 paste-buffer로 프롬프트 전달 및
GPT-5.4 응답 생성이 확인되었으므로, 설정 변경과 코드 수정으로 인터랙티브 모드 전환이 가능하다.

## 요구사항

### R1: opencode Provider 설정 변경

- WHEN autopus.yaml의 opencode provider가 로드될 때,
  THE SYSTEM SHALL `interactive_input` 필드를 빈 문자열(기본값)로 설정하여
  sendkeys/SendLongText 기반 프롬프트 전달을 사용해야 한다.

- WHEN opencode provider의 PaneArgs가 구성될 때,
  THE SYSTEM SHALL `run` 서브커맨드를 제거하고 TUI 모드 실행에 필요한 플래그
  (예: `-m openai/gpt-5.4`)만 포함해야 한다.

### R2: Interactive Launch 커맨드 빌드 수정

- WHEN `buildInteractiveLaunchCmd`가 opencode provider 커맨드를 생성할 때,
  THE SYSTEM SHALL `run` 서브커맨드 없이 `opencode -m openai/gpt-5.4` 형태의
  TUI 모드 실행 커맨드를 빌드해야 한다.

- WHEN opencode TUI 세션이 시작될 때,
  THE SYSTEM SHALL claude/gemini와 동일하게 세션이 유지되고
  후속 라운드에서 SendLongText로 프롬프트가 전달되어야 한다.

### R3: Debate 라운드 간 세션 유지

- WHEN interactive debate의 round 2 이상에서 opencode pane에 프롬프트를 전송할 때,
  THE SYSTEM SHALL `InteractiveInput == "args"` 스킵 로직을 적용하지 않고
  SendLongText를 통해 프롬프트를 전달해야 한다.

- WHEN opencode TUI 세션에서 이전 라운드 응답 완료 후 새 프롬프트를 전송할 때,
  THE SYSTEM SHALL 프롬프트 입력란이 활성화된 상태(`> ` 패턴)를 확인한 후 전송해야 한다.

### R4: Hook 기반 완료 감지 통합

- WHEN opencode provider가 hook mode에서 실행될 때,
  THE SYSTEM SHALL `hook-opencode-complete.ts` 플러그인을 통해
  `text.complete` 이벤트를 캡처하고 결과를 세션 디렉토리에 기록해야 한다.

- WHEN `InjectOrchestraPlugin`이 호출될 때,
  THE SYSTEM SHALL opencode.json에 autopus-result 플러그인이 등록되어
  매 라운드 완료 시 `{provider}-round{N}-result.json` 및 `{provider}-round{N}-done`
  시그널 파일을 생성해야 한다.

### R5: Config Migration (기존 설정 호환)

- WHEN 기존 autopus.yaml에 opencode provider가 `interactive_input: "args"`로
  설정되어 있을 때,
  THE SYSTEM SHALL 마이그레이션 시 `interactive_input`을 빈 문자열로 변경하고
  PaneArgs에서 `run` 서브커맨드를 제거해야 한다.

## 생성 파일 상세

| 파일 | 역할 | 변경 유형 |
|------|------|----------|
| `autopus.yaml` | R1: opencode provider 설정 변경 (interactive_input 제거, pane_args 수정) | 수정 |
| `pkg/config/defaults.go` | R1: opencode 기본 PaneArgs에서 `run` 제거 | 수정 |
| `pkg/config/migrate.go` | R5: opencode interactive_input 마이그레이션 추가 | 수정 |
| `pkg/orchestra/interactive_launch.go` | R2: opencode TUI 모드 launch 커맨드 빌드 확인 | 수정 |
| `pkg/orchestra/interactive_debate.go` | R3: args 스킵 로직 제거 (opencode가 더 이상 args 모드가 아님) | 확인 |
| `pkg/adapter/opencode/opencode.go` | R4: InjectOrchestraPlugin 호출 보장 | 확인 |

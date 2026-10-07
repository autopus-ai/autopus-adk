# SPEC-ORCH-017: Hook-triggered Bidirectional File IPC

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-29
**Domain**: ORCH

## 목적

현재 Orchestra의 멀티턴 토론에서 Round 2+ 프롬프트 전달은 `SendLongText` → cmux `set-buffer` + `paste-buffer`에 의존한다. 이 경로는 PTY 4KB 한계, surface stale exit status 1, paste-buffer 경합 등 구조적 불안정성을 갖는다.

이미 결과 수집(output)은 hook 기반 파일 시그널 프로토콜로 안정적으로 동작하고 있으므로, **프롬프트 전달(input)도 동일한 파일 시그널 프로토콜로 전환**하여 paste-buffer를 완전 우회한다.

## 핵심 구조

```
Orchestra Engine
  ├─ 프롬프트 → /tmp/autopus/{session-id}/{provider}-round{N}-input.json   (NEW)
  ├─ ready   ← /tmp/autopus/{session-id}/{provider}-round{N}-ready         (NEW)
  └─ 결과    ← /tmp/autopus/{session-id}/{provider}-round{N}-result.json   (기존)
```

## 요구사항

### R1: Input Signal Protocol

WHEN Orchestra가 Round 2+ 프롬프트를 전달해야 할 때, THE SYSTEM SHALL `HookSession`에 `WriteInput(provider, prompt)` 및 `WriteInputRound(provider, round, prompt)` 메서드를 추가하여 프롬프트를 `{provider}-round{N}-input.json` 파일로 세션 디렉토리에 원자적으로(atomic write) 저장한다.

- Input JSON 포맷: `{"prompt": "<text>", "round": N}`
- 파일 퍼미션: 0o600
- 원자적 쓰기: tmp 파일 작성 후 rename

### R2: Readiness Protocol

WHEN CLI hook이 이전 라운드 결과를 출력한 후, THE SYSTEM SHALL `{provider}-round{N}-ready` 시그널 파일을 생성하여 다음 라운드 input을 수신할 준비가 되었음을 Orchestra에 알린다.

- `HookSession`에 `WaitForReady(provider, round)` 및 `WaitForReadyCtx(ctx, provider, round)` 메서드 추가
- Orchestra는 ready 시그널 확인 후에만 input 파일 작성
- Ready 타임아웃 (30초) 시 fallback 경로 진입

**done과 ready의 관계**: `done` 시그널은 "이 라운드의 결과가 준비됨"을 의미하고, `ready` 시그널은 "hook이 input 감시 루프에 진입하여 다음 라운드 프롬프트를 수신할 준비가 됨"을 의미한다. 시간 순서: `done` 파일 생성 → hook이 input 감시 루프에 진입 → `ready` 파일 생성. `ready`는 `done` 이후 별도 단계로, hook이 블로킹 감시 상태에 도달했음을 보장한다.

### R3: Hook Input Reader

WHEN 각 CLI의 hook이 실행될 때, THE SYSTEM SHALL hook 스크립트를 확장하여 input 파일 감시 및 CLI 프롬프트 주입을 수행한다.

**공통 알고리즘 (모든 hook)**:
1. 기존 결과 출력 로직 실행 (result.json + done 시그널 작성)
2. 다음 라운드 번호 계산: `NEXT_ROUND = AUTOPUS_ROUND + 1`
3. Input 파일 경로 결정: `{SESSION_DIR}/{provider}-round{NEXT_ROUND}-input.json`
4. `{provider}-round{NEXT_ROUND}-ready` 시그널 파일 생성 (빈 파일)
5. Input 파일 감시 루프 진입 (200ms polling, 120초 타임아웃)
6. Input 파일 감지 시: JSON에서 `prompt` 필드 추출
7. CLI에 프롬프트 전달 후 input 파일 삭제
8. 타임아웃 시: ready 시그널 삭제, 정상 종료 (Orchestra가 fallback 처리)

**CLI별 프롬프트 주입 방식**:
- **Claude**: Stop hook은 프로세스 종료 직전에 실행됨. 추출한 프롬프트를 stdout으로 출력 — Claude Code의 hook 응답이 다음 프롬프트로 사용됨. 또는 `--resume` 기반 재실행 시 input 파일을 CLI 인자로 전달.
- **Gemini**: AfterAgent hook 종료 후 gemini CLI가 다음 입력 대기 상태. hook이 input을 읽어 cmux `send-surface`로 gemini pane에 직접 전달 (paste-buffer보다 짧은 텍스트는 send-surface 안정적).
- **opencode**: TS 플러그인이 `setInterval`로 input 파일 감시. 감지 시 opencode 내부 API (`submitPrompt`)로 직접 전달.

### R4: executeRound 전환

WHEN `executeRound`가 Round 2+ 프롬프트를 전송할 때, THE SYSTEM SHALL `HookSession`이 활성화되어 있으면 `SendLongText` 대신 다음 순서를 실행한다:

1. `WaitForReadyCtx(ctx, provider, round, 30s timeout)` — CLI hook이 input 감시 루프에 진입할 때까지 대기. 내부적으로 `waitForFileCtx`를 사용하여 `{provider}-round{N}-ready` 파일을 200ms 간격으로 폴링.
2. `WriteInputRound(provider, round, prompt)` — `{provider}-round{N}-input.json`을 atomic write (tmp 파일 → rename). JSON 포맷: `{"prompt": "<text>", "round": N}`.
3. 기존 `WaitForDoneRoundCtx` + `ReadResultRound`로 결과 수집 — hook이 CLI에 프롬프트를 주입하고, CLI가 처리 완료하면 done + result 시그널 생성.

`SendLongText`, `SendCommand("\n")`, paste-buffer 경로를 완전 스킵한다.

### R5: Graceful Fallback

WHEN hook IPC 경로가 실패할 때, THE SYSTEM SHALL 기존 `SendLongText` + paste-buffer 방식으로 자동 전환한다.

**Fallback 트리거 조건**:
- `HookSession`이 nil이거나 `HasHook(provider)` false인 경우 → 기존 경로
- `WaitForReady` 타임아웃 (30초) → 기존 경로 + 로그 경고
- `WriteInputRound` 파일 쓰기 실패 → 기존 경로

**Deadlock 방지 (R5-SAFETY)**: Fallback 전환 시, Orchestra는 반드시 `{provider}-round{N}-abort` 시그널 파일을 생성하여 hook의 input 감시 루프를 해제한다. Hook 스크립트는 input 파일뿐 아니라 abort 파일도 감시하며, abort 감지 시 즉시 루프를 종료하고 정상 종료한다. 이를 통해 "ready 시그널은 생성되었지만 input이 오지 않는" 데드락을 방지한다.

- 두 경로(파일 IPC vs paste-buffer)가 공존하며 프로바이더별로 독립 선택

### R6: Hook 배포 확장

WHEN `auto init` 또는 `auto update`가 실행될 때, THE SYSTEM SHALL 확장된 hook 스크립트(input reader 포함)를 각 CLI의 설정 디렉토리에 배포한다.

- `pkg/adapter/claude/`, `pkg/adapter/gemini/`, `pkg/adapter/opencode/`의 hook 배포 로직 업데이트
- `content/hooks/` 아래의 3개 hook 파일이 input reader 기능을 포함한 새 버전으로 교체

### R7: Round Signal 정리

WHEN 라운드가 완료된 후, THE SYSTEM SHALL `CleanRoundSignals`를 확장하여 input/ready 시그널 파일도 정리한다.

- 기존: `*-round{N}-done` 패턴만 정리
- 확장: `*-round{N}-done`, `*-round{N}-input.json`, `*-round{N}-ready` 모두 정리

## 생성 파일 상세

### Go 코드 (pkg/orchestra/)

| 파일 | 변경 | 설명 |
|------|------|------|
| `hook_signal.go` | 수정 | `WriteInput`, `WriteInputRound`, `WaitForReady`, `WaitForReadyCtx` 메서드 추가 |
| `round_signal.go` | 수정 | `CleanRoundSignals` 확장 (input/ready 파일 정리) |
| `interactive_debate.go` | 수정 | `executeRound` Round 2+ 경로 전환 (file IPC 우선, fallback) |
| `hook_input.go` | 신규 | Input JSON 구조체, atomic write 헬퍼 |

### Hook 스크립트 (content/hooks/)

| 파일 | 변경 | 설명 |
|------|------|------|
| `hook-claude-stop.sh` | 수정 | ready 시그널 + input 감시 루프 추가 |
| `hook-gemini-afteragent.sh` | 수정 | ready 시그널 + input 감시 루프 추가 |
| `hook-opencode-complete.ts` | 수정 | ready 시그널 + input listener 추가 |

### 어댑터 (pkg/adapter/)

| 파일 | 변경 | 설명 |
|------|------|------|
| `claude/claude_hooks_test.go` | 수정 | 확장된 hook 테스트 |
| 각 어댑터 배포 로직 | 수정 | 새 hook 버전 배포 반영 |

# SPEC-ORCH-007: --multi Hook 기반 멀티프로바이더 오케스트레이션

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-26
**Domain**: ORCH
**Extends**: SPEC-ORCH-006 (인터랙티브 pane 모드)
**Origin**: BS-002 (Hook 기반 멀티프로바이더 오케스트레이션 브레인스토밍)

## 목적

SPEC-ORCH-006이 구현한 인터랙티브 pane 모드에서 ReadScreen 화면 스크래핑 + idle 감지 기반의 결과 수집을 hook/plugin 파일 시그널 방식으로 전환한다. Claude Code(Stop hook), Gemini CLI(AfterAgent hook), opencode(experimental.text.complete plugin)가 각각 제공하는 hook 시스템을 활용하여 프로바이더 완료 시 구조화된 JSON 결과를 파일로 저장하고, 오케스트레이터가 파일 감시로 이를 수집한다.

이를 통해 (1) ANSI 파싱/프롬프트 패턴 매칭의 불안정성 제거, (2) 결과 수집 성공률 95% 이상 달성, (3) 새 프로바이더 추가 비용을 hook 스크립트 1개로 축소한다. 추가로 Codex CLI를 opencode로 전환하여 plugin 기반 통합을 완성한다.

## 요구사항

### P0 — Must Have

#### R1: 파일 시그널 프로토콜 정의

THE SYSTEM SHALL 프로바이더 결과 수집을 위한 파일 시그널 프로토콜을 정의한다:
- 결과 파일: `/tmp/autopus/{session-id}/{provider}-result.json` 형식
- 결과 스키마: `{ "session_id": string, "provider": string, "response": string, "timestamp": string }`
- 완료 시그널: `/tmp/autopus/{session-id}/{provider}-done` 빈 파일
- 세션 디렉토리 권한: 0o700 (소유자만 접근)
- 결과 파일 권한: 0o600 (소유자만 읽기/쓰기)

#### R2: Claude Code Stop Hook 스크립트

WHEN Claude Code CLI가 응답을 완료하면, THE SYSTEM SHALL Stop hook 스크립트가 다음을 수행한다:
- hook 입력 JSON에서 `last_assistant_message` 필드 추출
- `AUTOPUS_SESSION_ID` 환경변수로 세션 ID 확인
- `/tmp/autopus/{session-id}/claude-result.json`에 결과 저장
- `/tmp/autopus/{session-id}/claude-done` 시그널 파일 생성
- POSIX shell 호환 (bash/zsh), `jq` 의존성 없이 기본 shell 도구만 사용

#### R3: Gemini CLI AfterAgent Hook 스크립트

WHEN Gemini CLI가 에이전트 응답을 완료하면, THE SYSTEM SHALL AfterAgent hook 스크립트가 다음을 수행한다:
- hook 입력 JSON에서 `prompt_response` 필드 추출
- `/tmp/autopus/{session-id}/gemini-result.json`에 결과 저장
- `/tmp/autopus/{session-id}/gemini-done` 시그널 파일 생성

#### R4: opencode Plugin 스크립트

WHEN opencode가 텍스트 완료를 수행하면, THE SYSTEM SHALL `experimental.text.complete` plugin이 다음을 수행한다:
- plugin 이벤트에서 `text` 필드 추출
- `/tmp/autopus/{session-id}/opencode-result.json`에 결과 저장
- `/tmp/autopus/{session-id}/opencode-done` 시그널 파일 생성

#### R5: interactive.go 파일 감시 전환

WHEN hook 모드가 활성화된 프로바이더에 대해, THE SYSTEM SHALL `waitForCompletion()`을 다음과 같이 변경한다:
- ReadScreen 폴링 대신 `{provider}-done` 파일 존재 여부를 `os.Stat()` 폴링(200ms 간격)으로 확인
- `done` 파일 감지 시 `{provider}-result.json`을 파싱하여 `ProviderResponse.Output`에 저장
- `cleanScreenOutput()` 호출을 제거하고 JSON의 `response` 필드를 직접 사용

#### R6: Hook 자동 주입

WHEN `auto init` 또는 세션 시작 시, THE SYSTEM SHALL 각 프로바이더 설정에 hook/plugin 엔트리를 자동 등록한다:
- Claude Code: `.claude/settings.json`의 `hooks.Stop`에 결과 수집 스크립트 등록
- Gemini CLI: `.gemini/settings.json`의 `hooks.AfterAgent`에 결과 수집 스크립트 등록
- opencode: `opencode.json`의 `experimental.text.complete` plugin 등록
- 기존 사용자 hook은 보존 (merge 방식으로 추가)

#### R7: ANSI 파싱 코드 격리

WHEN hook 모드가 활성화되면, THE SYSTEM SHALL hook 결과 수집 경로에서 `cleanScreenOutput()`, `stripANSI()`, `filterPromptLines()` 호출을 제거한다. 이들 함수는 fallback 경로(ReadScreen)에서만 사용되도록 격리한다.

#### R8: Graceful Degradation

WHERE 프로바이더의 hook이 설정되지 않았거나 done 파일이 타임아웃 내에 생성되지 않으면, THE SYSTEM SHALL 해당 프로바이더에 대해 기존 SPEC-ORCH-006의 ReadScreen + idle 감지 fallback으로 자동 전환한다. 다른 프로바이더의 hook 결과 수집에 영향을 주지 않는다.

### P1 — Should Have

#### R9: Codex -> opencode 프로바이더 전환

WHEN `autopus.yaml`에 `codex` 프로바이더가 설정되어 있으면, THE SYSTEM SHALL `pkg/config/migrate.go`에서 `codex`를 `opencode`로 자동 마이그레이션한다:
- `defaultProviderEntries`에 opencode 엔트리 추가
- `PlatformToProvider()`에 opencode 매핑 추가
- 마이그레이션 시 경고 메시지 출력

#### R10: opencode OAuth 인증 플로우 연동

WHEN opencode가 OAuth 인증이 필요한 경우, THE SYSTEM SHALL opencode의 ChatGPT OAuth (Codex client ID 재사용) 플로우를 지원한다.

#### R11: Debate 전략 Hook 결과 연동

WHEN debate 전략이 hook 모드에서 실행될 때, THE SYSTEM SHALL:
- Phase 1 병렬 실행 후 각 hook 결과 JSON을 직접 파싱
- Phase 2 rebuttal 시 다른 프로바이더의 hook 결과(`response` 필드)를 rebuttal 프롬프트에 주입
- Phase 3 judge에 구조화된 결과를 전달

#### R12: Relay 전략 Hook 결과 연동

WHEN relay 전략이 hook 모드에서 실행될 때, THE SYSTEM SHALL:
- 이전 프로바이더의 hook 결과(`response` 필드)를 다음 프로바이더 프롬프트에 주입
- `buildRelayPrompt()`에서 hook 결과를 직접 활용

#### R13: Consensus/Fastest 전략 Hook 결과 활용

WHEN consensus 또는 fastest 전략이 hook 모드에서 실행될 때, THE SYSTEM SHALL hook 결과 JSON의 `response` 필드를 `ProviderResponse.Output`으로 직접 사용하여 `MergeConsensus()` 및 fastest 판정에 활용한다.

### P2 — Could Have

#### R14: cmux Pane 프로바이더 라벨 및 상태 표시

WHILE 오케스트레이션이 진행 중일 때, THE SYSTEM SHALL 각 pane에 프로바이더 이름과 상태(진행/완료/에러)를 표시한다.

#### R15: 결과 비교 뷰

WHEN 모든 프로바이더 결과가 수집되면, THE SYSTEM SHALL 3개 프로바이더 응답을 나란히 비교하는 뷰를 제공한다.

#### R16: opencode 서버 모드 보조 채널

WHEN opencode가 서버 모드(`serve --port`)로 실행 가능하면, THE SYSTEM SHALL HTTP API를 통한 보조 결과 수집 채널을 지원한다.

## 생성 파일 상세

### 신규 파일

| 파일 | 역할 | 줄 수 목표 |
|------|------|-----------|
| `pkg/orchestra/hook_signal.go` | 파일 시그널 프로토콜: HookResult 타입, 세션 디렉토리 관리, result.json 파싱, done 파일 감시 | < 150 |
| `pkg/orchestra/hook_watcher.go` | Hook 모드 waitForCompletion + collectResults: 파일 폴링, fallback 분기 | < 150 |
| `content/hooks/hook-claude-stop.sh` | Claude Code Stop hook 스크립트 | < 30 |
| `content/hooks/hook-gemini-afteragent.sh` | Gemini CLI AfterAgent hook 스크립트 | < 30 |
| `content/hooks/hook-opencode-complete.ts` | opencode plugin 스크립트 (TypeScript) | < 30 |
| `pkg/adapter/opencode/opencode.go` | opencode 플랫폼 어댑터 (PlatformAdapter 구현) | < 200 |

### 수정 파일

| 파일 | 변경 내용 |
|------|----------|
| `pkg/orchestra/interactive.go` | hook 모드 분기 추가: `waitAndCollectResults()` → hook/ReadScreen 분기 |
| `pkg/orchestra/interactive_detect.go` | fallback 전용으로 격리 (hook 모드에서 미호출) |
| `pkg/orchestra/types.go` | `OrchestraConfig`에 `HookMode bool`, `SessionID string` 필드 추가 |
| `pkg/orchestra/pane_runner.go` | hook 모드 세션 디렉토리 생성/정리 로직 추가 |
| `pkg/orchestra/debate.go` | hook 결과 기반 rebuttal 프롬프트 빌드 |
| `pkg/orchestra/relay.go` | hook 결과 기반 relay 프롬프트 빌드 |
| `pkg/config/migrate.go` | codex → opencode 마이그레이션 추가 |
| `pkg/adapter/claude/claude_settings.go` | Stop hook 자동 주입 로직 |
| `pkg/adapter/gemini/gemini.go` | AfterAgent hook 자동 주입 로직 |

# SPEC-LEARN-003 수락 기준

## Test Scenarios

fixture는 모두 temp dir 안에 만든다. 저장소 단위 시나리오는 `pkg/learn`의 `newTestStore`, CLI 시나리오는 `newLearnRecordCmd`·`newLearnQueryCmd`·`newLearnPruneCmd`·`newLearnSummaryCmd`를 그 temp dir에서 실행한다. 시각 고정이 필요한 항목은 timestamp를 60일 전·40일 전처럼 명시해 최근성 가산이 끼지 않게 한다.

### S1: 근거 종류 도출
Priority: Must
Given 빈 저장소가 있다
When `RecordFixPattern`을 (a) command `go test ./x`, before 1, after 0 (b) before 0, after 0 (c) command만 (d) command 없이 before 1, after 0 (e) before 1, after 1 (f) 근거 입력 없음으로 각각 실행한다
Then 종류는 (a) `deterministic` (b)–(e) `llm_only` (f) `""`이다. (f)의 JSONL 줄에는 `"evidence"`가 없고, 여섯 줄 모두 `"scope"`가 없으며 `EffectiveScope()`는 `project`다

### S2: 잘못된 범위·근거 명령 거부
Priority: Must
Given scope `process`인 항목 하나가 든 저장소와 그 파일의 SHA-256이 있다
When scope `global`, 근거 명령 `go test\x1b[2J`, 513바이트 근거 명령으로 각각 기록한다
Then 셋 다 `*FieldError`로 끝나고 `CheckRecordOpts`도 같은 입력을 거부하며, 저장소 SHA-256이 실행 전과 같다

### S3: 조회 가산
Priority: Must
Given 60일 전 timestamp, pattern `nil map`, SPEC `S`인 세 항목이 있다: L-001 근거 없음, L-002 process·`deterministic`, L-003 `llm_only`
When keyword `nil map`으로 조회하고, SPEC `S`만으로 조회한다
Then 첫 결과는 L-002, 점수 2.00이고 나머지 둘은 1.00이다. SPEC만 조회한 점수는 [2, 1, 1]이다

### S4: 범위 필터
Priority: Must
Given S3의 저장소가 있다
When SPEC `S`와 scope `project`, scope `process`로 각각 조회하고, CLI에서 `--scope global`로 조회한다
Then project 결과는 [L-001, L-003], process 결과는 [L-002]이고, `--scope global`은 `unknown scope: must be process or project` 오류다

### S5: llm_only 정리
Priority: Must
Given 40일 전 항목 넷(L-001 근거 없음, L-002 `llm_only`, L-003 `deterministic`, L-004 `llm_only`·보호됨)과 오늘 항목 L-005 `llm_only`가 있다
When `PrunePolicyExcept`를 Days 90, LLMOnlyDays 30, 보호 {L-004}로 실행한다
Then 결과는 `{Removed: 1, KeptProtected: 1, RemovedLLMOnly: 1}`이고 남은 id는 [L-001, L-003, L-004, L-005]다. CLI에서 40일 된 `llm_only` 하나가 든 저장소에 `--days 90`을 실행하면 출력이 정확히 `Removed 1 entries older than 90 days.\nOf those, 1 had llm_only evidence older than 30 days.\n`이고, 다시 실행하면 `Removed 0 entries older than 90 days.\n`이다

### S6: 요약
Priority: Must
Given L-001 process·`deterministic`·reuse 2 "reopen outputs", L-002 process·`llm_only`, L-003 project·`deterministic`·expected·actual 있음이 있다
When `GenerateSummary(store, 5)`를 실행한다
Then ScopeCounts는 {process: 2, project: 1}, EvidenceCounts는 {deterministic: 2, llm_only: 1}, TemplateCandidates는 [{reopen outputs, 2}], IntakeEligible은 1이다. CLI 시나리오의 summary 출력에는 `Scope: process 1, project 1`, `Evidence: deterministic 1, llm_only 1, unspecified 0`, `Template candidates (process scope, deterministic evidence; review by hand):\n  [0x] reopen outputs after writing` 줄이 있다

### S7: 기존 저장소 호환
Priority: Must
Given 새 필드가 없는 기존 형식 저장소(prune S7·intake S5 fixture)가 있다
When 기존 prune·intake·query test를 그대로 실행한다
Then 출력이 이 SPEC 전과 byte 단위로 같다(`TestLearnPrune_S7_*`, `TestEvalHarnessIntake_S5_*`, `TestCLI_LearnQuery_*` 통과)

### S8: 다섯 플랫폼 생성 결과
Priority: Must
Given 이 branch로 빌드한 `auto`와 빈 git 저장소가 있다
When `auto init --yes --no-rea`를 실행한다
Then `mark-reused`는 `.claude/skills/auto-go`·`auto-fix`, `.codex/skills/codex-auto-go`·`codex-auto-fix`, `.agents/skills/auto-go`·`auto-fix`, `.omp/skills/auto-go`·`auto-fix`, `.gemini/skills/autopus/auto-go`·`auto-fix`에 있다. `evidence-before-exit`는 그 다섯 `auto-fix`에, `auto learn prune --days 90`과 `harness intake --all-eligible`은 다섯 `auto-sync`에 있다

### S9: CLI 기록 끝까지
Priority: Must
Given 빈 temp project가 있다
When `auto learn record --type fix_pattern --pattern "reopen outputs after writing" --scope process --evidence-command "go test ./pkg/x -run TestReopen" --evidence-before-exit 1 --evidence-after-exit 0`, 이어 `--evidence-command "go test ./pkg/x"`만 준 기록, `--scope global` 기록을 실행한다
Then 첫 항목은 `deterministic`·process·BeforeExit 1, 둘째는 `llm_only`이고 BeforeExit가 nil이며, 셋째는 오류이고 항목 수는 2다

## Oracle Acceptance Notes

- Must 시나리오는 concrete 값을 판정한다: 근거 종류 문자열, 점수 수치와 순서, 남은 id 목록, `PruneResult` 값, 정확한 출력 줄, 파일 SHA-256, 생성 파일 경로. 파일 존재나 종료 코드만으로 닫는 Must는 없다.
- S1–S6, S9는 `pkg/learn/evidence_test.go`와 `internal/cli/learn_evidence_test.go`가 판정한다. S7은 기존 test가, S8은 생성 프로젝트 grep이 판정한다.
- 기대 값은 2026-10-24에 이 branch 코드로 계산했다. 코드 사실과 다르면 test가 아니라 이 SPEC을 고친다.

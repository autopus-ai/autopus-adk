# SPEC-ORCH-003: Orchestra Detach Mode

**Status**: completed
**Superseded by SPEC-PANERM-001** (2026-10-07): the orchestra pane backend was retired; providers run as subprocesses or through OMP.
**Created**: 2026-03-26
**Domain**: ORCH

## 목적

orchestra brainstorm(및 review, plan, secure)이 Bash 도구 안에서 실행될 때 프로바이더 응답 시간(60-120초)이 Bash timeout(120초)을 초과하여 SIGKILL(exit 137)로 강제 종료되는 문제를 해결한다. pane 터미널(cmux/tmux) 감지 시 auto-detach 모드로 전환하여, pane 생성과 명령 전송 후 즉시 반환하고 결과는 별도 명령으로 수집하는 비동기 워크플로를 도입한다.

## 요구사항

### REQ-1: Auto-Detach 감지
WHEN the detected terminal is cmux or tmux (non-plain) AND stdout is a TTY (interactive),
THE SYSTEM SHALL automatically switch to detach mode, launching providers in separate panes and returning a job ID within 2 seconds.
WHERE stdout is NOT a TTY (piped or redirected), THE SYSTEM SHALL fall back to blocking execution regardless of terminal type, ensuring scripts and automation pipelines receive direct output on stdout.

### REQ-2: --no-detach 오버라이드
WHEN the user specifies `--no-detach` flag,
THE SYSTEM SHALL force blocking execution even on pane terminals, bypassing auto-detach.

### REQ-3: Job 디렉토리 생성
WHEN detach mode is activated,
THE SYSTEM SHALL create a temporary directory via `os.MkdirTemp("", "autopus-orch-")` containing `job.json` and per-provider output files (`{provider}.out`). The actual path is stored in `job.json` for subsequent commands to locate.

### REQ-4: job.json 스키마
WHEN a job is created,
THE SYSTEM SHALL write a `job.json` file containing: id, strategy, prompt, judge, providers (name, pane_id, output_file), created_at, timeout_at, terminal.

### REQ-5: Status 조회
WHEN the user runs `auto orchestra status {jobID}`,
THE SYSTEM SHALL report the job status as one of: running, partial, done, timeout, error.

### REQ-6: Wait 명령
WHEN the user runs `auto orchestra wait {jobID}`,
THE SYSTEM SHALL poll sentinel files until all providers complete or timeout is reached, then return the final status.

### REQ-7: Result 수집
WHEN the user runs `auto orchestra result {jobID}`,
THE SYSTEM SHALL read all provider output files, apply the configured merge strategy, and print the merged result.

### REQ-8: Cleanup
WHEN the user runs `auto orchestra result {jobID} --cleanup`,
THE SYSTEM SHALL remove all panes and the job temp directory after outputting the result.

### REQ-9: 하위 호환
WHERE the terminal is plain (non-pane),
THE SYSTEM SHALL maintain 100% backward compatibility with existing RunPaneOrchestra/RunOrchestra behavior.

### REQ-10: Sentinel 재사용
WHILE monitoring provider output,
THE SYSTEM SHALL reuse the existing `__AUTOPUS_DONE__` sentinel marker for completion detection.

### REQ-11: Abandoned Job Cleanup
WHEN any `auto orchestra` subcommand is invoked,
THE SYSTEM SHALL scan for job directories older than 1 hour (based on `created_at` in `job.json`) and remove those directories along with any associated panes. This opportunistic GC approach prevents resource leaks without requiring a background daemon.

## 생성 파일 상세

| 파일 | 역할 | 예상 줄수 |
|------|------|-----------|
| `pkg/orchestra/job.go` | Job struct, SaveJob, LoadJob, CheckStatus, CollectResults, Cleanup 함수 | ~120 |
| `pkg/orchestra/job_test.go` | Job lifecycle 테스트 (생성, 상태, 수집, 정리) | ~100 |
| `internal/cli/orchestra_job.go` | status, wait, result, cleanup CLI 서브커맨드 정의 | ~120 |
| `internal/cli/orchestra_job_test.go` | CLI 서브커맨드 단위 테스트 | ~80 |

## 수정 파일 상세

| 파일 | 변경 내용 |
|------|-----------|
| `pkg/orchestra/pane_runner.go` | `RunPaneOrchestraDetached()` 함수 추가 (~30줄) |
| `internal/cli/orchestra.go` | 서브커맨드 등록 + `runOrchestraCommand()` 내 auto-detach 분기 |

# SPEC-LEARN-003: 학습 기록의 실행 근거·범위와 전 플랫폼 조회

**Status**: implemented
**Created**: 2026-10-24
**Domain**: LEARN
**Module**: autopus-adk
**Release**: v0.50.128. 작업 branch는 `feat/learn-evidence-scope`이고 기준은 `fix/learn-store-defects`(PR #214, 350ce05c)다. 그 PR이 `QueryScored`, `--top`·`--mark-reused`, `MarkReused`, hook의 SPEC ID·파일 기록을 더했다.
**Predecessor**: SPEC-LEARN-001(저장소·조회·정리·요약), SPEC-LEARNWIRE-002(pipeline hook), SPEC-HARNEVAL-002(intake). 다르면 이 SPEC이 우선한다.

## 목적

learn 저장소(`.autopus/learnings/pipeline.jsonl`)는 교훈을 산문으로 남기고, 그 교훈이 실행으로 확인됐는지 모델의 의견인지 구분하지 않는다. 범용(에이전트 작업 방식) 교훈과 이 저장소의 분야 교훈도 구분하지 않는다. 조회와 기록은 Claude Code 템플릿에만 있고 Codex·OpenCode·OMP·Gemini 템플릿은 학습을 읽지도 쓰지도 않는다.

Harness²(arXiv 2610.14224, 2026-10)는 실행 비교로 뒷받침되는 교훈만 남기고, 범용과 분야로 나눈 제안이 ablation에서 가장 큰 효과를 냈다고 보고했다. 이 SPEC은 두 원칙을 learn 저장소에 들인다. (1) 기록은 실행 근거를 함께 받고 근거 종류는 exit code에서 도출한다. (2) 기록은 범위를 가진다. (3) 조회·정리·요약이 이 둘을 쓴다. (4) 다섯 플랫폼의 `auto-go`·`auto-fix`·`auto-sync`가 같은 명령으로 조회·기록·정리한다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md` `## Outcome Lock`이다.
- Mandatory requirements: REQ-LRN3-01 ~ 08 (Must).
- Explicit non-goals:
  - 교훈을 rule·skill·template에 자동 반영하는 것. summary는 후보를 보여 주기만 한다
  - embedding·유사도 조회, 중복 통합
  - skillevolve 내용 생성과 qualityloop 연결(Evolution Idea)
  - OpenCode의 조건부 rule 전달(별도 SPEC, Evolution Idea)
  - 이전 버전 `auto`가 저장소를 다시 쓸 때 새 필드를 보존하게 하는 것
- Completion evidence: Must S1–S9 PASS, `go test ./pkg/learn ./pkg/pipeline ./templates/... ./pkg/adapter/...`, `internal/cli -run 'Learn|Prune|Intake|Summary'`, 신규·변경 소스 파일 300줄 이하, 생성 결과 확인(S8), `auto spec validate --strict`.

## Trust Model

- 근거 명령은 데이터다. `repro`와 같이 아무것도 그것을 실행하지 않는다.
- 근거 종류는 호출자가 정하지 않는다. 호출자는 명령과 exit code만 주고 저장소가 종류를 도출한다. 템플릿은 관찰한 exit code만 쓰라고 지시한다. 지어낸 exit code는 막을 수 없으므로 `deterministic`은 "실행으로 확인됐다고 기록됨"이지 증명이 아니다.
- 근거 명령은 `expected`·`actual`·`repro`와 같은 순서(제어 문자 거부 → 원문 상한 → 비밀값 가림 → 가린 뒤 512바이트 상한)로 검사한다. scope는 enum 밖이면 거부한다. 거부되면 아무것도 쓰지 않는다.

## Requirements

### REQ-LRN3-01 근거 입력과 종류 도출
Priority: Must · EARS: EventDriven

WHEN `auto learn record` runs with any of `--evidence-command`, `--evidence-before-exit`, `--evidence-after-exit`, THE SYSTEM SHALL store an `evidence` object with `kind` `deterministic` exactly when the command is non-empty, the before exit code was given and is non-zero, and the after exit code was given and is zero, and `kind` `llm_only` otherwise; an exit code counts only when its flag was given.

### REQ-LRN3-02 근거 없는 기록의 불변
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL write no `evidence` key and no `scope` key for a record without evidence flags and without `--scope`, so such an entry is byte-compatible with the pre-SPEC schema.

### REQ-LRN3-03 범위
Priority: Must · EARS: EventDriven

WHEN `auto learn record` runs with `--scope`, THE SYSTEM SHALL accept only `process` or `project`, refuse any other value with `learning_field_invalid: scope: unknown_value` before writing, and treat an entry without a scope as `project` everywhere it is read.

### REQ-LRN3-04 근거 명령 검사
Priority: Must · EARS: EventDriven

WHEN an evidence command contains a control character, exceeds four times 512 bytes raw, or exceeds 512 bytes after secret redaction, THE SYSTEM SHALL refuse the record with `learning_field_invalid: evidence_command: <detail>` and leave the store byte-identical; otherwise it SHALL store the redacted command.

### REQ-LRN3-05 조회 가산과 범위 필터
Priority: Must · EARS: EventDriven

WHEN `auto learn query` scores entries, THE SYSTEM SHALL add 1.0 to the score of every matching entry whose evidence kind is `deterministic` (a spec-only match scores 1.0 plus that bonus), and WHEN `--scope` is given it SHALL keep only entries whose effective scope equals it and refuse a value other than `process` or `project`.

### REQ-LRN3-06 llm_only 정리
Priority: Must · EARS: EventDriven

WHEN `auto learn prune` runs, THE SYSTEM SHALL also remove entries whose evidence kind is `llm_only` and that are older than `--llm-only-days` (default 30; 0 turns the rule off), keep protected (eval-linked) entries, leave unspecified and `deterministic` entries to the `--days` rule, print the first line exactly as before, and add `Of those, N had llm_only evidence older than D days.` only when N is positive.

### REQ-LRN3-07 요약
Priority: Must · EARS: EventDriven

WHEN `auto learn summary` runs, THE SYSTEM SHALL add the lines `Scope: process P, project Q` (only when P > 0), `Evidence: deterministic D, llm_only L, unspecified U` (only when D + L > 0), a `Template candidates` list of process-scope entries with deterministic evidence ordered by reuse count then recency and limited by `--top` (only when non-empty), and an `Intake:` line counting entries with both expected and actual (only when positive); it SHALL never edit rules, skills, or templates.

### REQ-LRN3-08 전 플랫폼 템플릿
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL make the generated `auto-go`, `auto-fix`, and `auto-sync` of Claude Code, Codex, OpenCode, OMP, and Gemini run, when `.autopus/learnings/pipeline.jsonl` exists, `auto learn query --spec <SPEC_ID> --top 5 --mark-reused` (go), `auto learn query --files … --keywords … --top 5 --mark-reused` and `auto learn record … --scope … --evidence-command … --evidence-before-exit … --evidence-after-exit …` (fix), and `auto learn prune --days 90` then `auto learn summary --top 3` with an `auto eval harness intake --all-eligible` suggestion that is not run (sync).

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-LRN3-01 | T1 | S1, S9 | INV-LRN3-01 |
| REQ-LRN3-02 | T1 | S1, S7 | INV-LRN3-02 |
| REQ-LRN3-03 | T1, T2 | S2, S4 | INV-LRN3-03 |
| REQ-LRN3-04 | T1 | S2 | INV-LRN3-04 |
| REQ-LRN3-05 | T2 | S3, S4 | INV-LRN3-01, INV-LRN3-03 |
| REQ-LRN3-06 | T3 | S5, S7 | INV-LRN3-02 |
| REQ-LRN3-07 | T4 | S6 | INV-LRN3-05 |
| REQ-LRN3-08 | T5, T6, T7, T9 | S8 | INV-LRN3-05 |

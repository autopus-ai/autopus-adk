# SPEC-ORCH-020: Orchestra Reliability Kit

---
id: SPEC-ORCH-020
title: Orchestra Reliability Kit
version: 0.1.0
status: completed
note: Partially superseded by SPEC-PANERM-001 (its orchestra pane-backend parts were retired on 2026-10-07)
priority: Must
---

## Purpose

`pkg/orchestra`의 가장 큰 문제는 기능 부족이 아니라 실행 결정성 부족이다. 이 SPEC의 목적은 pane-mode, hook-mode, fallback path를 "되면 좋다" 수준의 best-effort 실행에서, preflight와 receipt가 있는 결정적 실행면으로 끌어올리는 것이다.

## Background

현재 orchestra는 warm pool, completion detector, hook collection, subprocess backend, detach job 등 강한 구성요소를 이미 갖고 있다. 그러나 이 구성요소들이 실제 런타임에서 올바른 `cwd`, intact prompt, valid collection path로 동작하는지 증명하는 contract가 약하다.

- 실측 실패 1: pane-mode brainstorm에서 Claude pane가 `autopus-adk/`가 아닌 `~/Documents/github/bitgapnam`에서 시작되어 context 파일 로드가 실패했다.
- 실측 실패 2: Codex/Gemini pane에서는 prompt transport가 절단되어 shell에 일부가 command fragment로 주입되었다.
- 재현 실패 3: `go test -timeout 120s ./pkg/orchestra`는 `TestRunPaneDebate_HookMode` timeout panic으로 실패했다.

관련 기존 SPEC:

- `SPEC-SURFCOMP-001`은 surface lifecycle과 completion detection을 보강했다.
- `SPEC-ORCH-019`는 subprocess backend와 schema-based orchestration을 도입했다.

이번 SPEC은 그 위에 reliability preflight, prompt/collection receipt, correlation-aware failure bundle을 추가하는 계층이다.

## Implementation Snapshot

2026-04-21 sync 기준 실제 반영 범위:

- `pkg/orchestra/reliability_{receipt,preflight,bundle}.go`에서 receipt/bundle schema v1, redaction, runtime artifact contract를 구현했다.
- `OrchestraResult`, detached `Job`, CLI output에 `run_id`, degraded 상태, artifact dir를 연결했다.
- pane/hook 경로에서 provider preflight receipt, prompt transport receipt, collection receipt를 기록하도록 연결했다.
- hook timeout과 missing-result 경로가 structured event + failure bundle + partial receipt를 남기도록 보강했다.
- runtime artifact 경로를 `~/.autopus/runtime/orchestra/runs/<run_id>/`로 고정하고, 홈 경로를 만들 수 없을 때 `/tmp/autopus-runtime/orchestra/runs/<run_id>/`로 폴백하도록 구현했다.
- retention 기본값은 최근 20 runs 또는 7일이며, 디렉터리는 `0700`, 파일은 `0600` 권한을 사용한다.

이번 sync에서 명시적으로 남긴 후속 과제:

- effective `cwd`를 shell-observed 값으로 검증하고 mismatch를 preflight-failed로 차단하는 경로는 후속 작업이다.
- prompt transport는 hash/byte length receipt를 기록하지만 mutation을 재검증해 mismatch로 fail시키는 단계는 후속 작업이다.
- reliability metrics와 append-only replay ledger는 이번 범위에 포함하지 않았다.

## Requirements

### Must

- **R1 - Provider Preflight Contract**  
  WHEN an orchestra run is about to start, THE SYSTEM SHALL execute a provider-specific preflight for every selected provider and persist the result as a structured receipt before the first round begins.

- **R2 - Deterministic CWD Binding**  
  WHEN a pane-backed provider session is launched, THE SYSTEM SHALL bind the provider process to the requested working directory and verify the effective `cwd` before prompt injection. IF the effective `cwd` differs from the requested value, THEN the system shall mark the provider as preflight-failed and SHALL NOT start round execution on that surface.

- **R3 - Prompt Transport Integrity**  
  WHEN a prompt is sent to a provider, THE SYSTEM SHALL generate and log a transport receipt containing prompt byte length, transport mode, and a content hash. IF the transport layer truncates, splits, or mutates the prompt, THEN the system shall detect the mismatch before waiting for completion and SHALL fail or downgrade the provider deterministically.

- **R4 - Capability-Aware Launch Decision**  
  WHEN pane-mode execution is requested, THE SYSTEM SHALL decide between pane, subprocess, or skip using declared provider capabilities and preflight evidence instead of implicit best-effort fallback.

- **R5 - Collection Receipt**  
  WHEN provider output is collected, THE SYSTEM SHALL write a collection receipt containing provider ID, run ID, round ID, collection mode, receipt status, and output provenance (`hook`, `poll`, `file_ipc`, or `subprocess_stdout`).

- **R6 - Reliability-Aware Degradation**  
  IF one or more providers fail preflight or collection integrity checks, THEN the system shall continue with the remaining healthy providers when quorum rules permit, and SHALL surface the degraded status in final output and JSON artifacts.

- **R7 - Correlation IDs**  
  THE SYSTEM SHALL assign and persist `run_id`, `round_id`, `provider_id`, and `attempt_id` across pipeline logs, orchestra job snapshots, and collection receipts so that a single failed run can be reconstructed without screen scraping.

- **R8 - Hook-Mode Timeout Guard**  
  WHEN hook-mode completion detection exceeds the configured timeout, THE SYSTEM SHALL emit a structured timeout event with provider and round context, SHALL capture a failure bundle, and SHALL either downgrade to the configured fallback mode or terminate with an actionable error.

- **R9 - Actionable Failure Summary**  
  WHEN orchestra execution fails due to reliability contract violations, THEN the system shall emit a user-facing summary that includes which preflight check or receipt failed and the exact next remediation step.

- **R10 - Prompt and Launch Artifact Sanitization**  
  WHEN prompts, launch commands, hook payloads, environment values, or other provider artifacts are persisted in receipts, bundles, or replay ledgers, THE SYSTEM SHALL redact secrets, tokens, cookie values, and credential material before persistence. The system SHALL prefer hashes, byte counts, safe previews, or masked values over raw sensitive content.

- **R11 - Receipt and Bundle Storage Boundary**  
  WHEN reliability artifacts are written to disk, THE SYSTEM SHALL store them under a deterministic runtime-owned directory with user-scoped permissions and SHALL document the exact storage path contract.

- **R12 - Artifact Versioning and Retention**  
  WHEN failure bundles or replay ledgers are emitted, THE SYSTEM SHALL version the artifact schema and SHALL apply deterministic retention and rotation rules so that secret leakage, diff noise, and unbounded artifact growth are controlled.

### Should

- **R13 - Pure Core Separation**  
  THE SYSTEM SHALL isolate round state transitions, timeout budgeting, and recovery policy into a transport-agnostic core package so that pane, hook, file IPC, and subprocess drivers become replaceable adapters before reliability work is considered structurally complete.

- **R14 - Failure Bundle Export**  
  WHEN a run fails or degrades, THE SYSTEM SHALL export a compact failure bundle containing preflight receipts, sanitized launch metadata, collection receipts, timing, and relevant log excerpts.

- **R15 - Reliability Metrics**  
  WHEN orchestration runs complete, THE SYSTEM SHALL emit per-provider reliability metrics such as preflight failure count, prompt transport mismatch count, hook timeout count, and degraded-run count.

- **R16 - Replay-Friendly Run Ledger**  
  WHEN orchestration events are persisted for diagnosis, THE SYSTEM SHALL record enough append-only execution events that an interrupted or failed run can be replayed without depending on terminal history.

### Nice

- **R17 - Orchestra Doctor Surface**  
  WHEN the reliability contract matures into a standalone diagnostic surface, THE SYSTEM SHALL expose `auto orchestra doctor` or an equivalent command to run the same preflight checks without starting an orchestration run.

- **R18 - Capability Handshake Descriptor**  
  WHEN provider capability metadata is externalized beyond inline adapter code, THE SYSTEM SHALL formalize launch mode, hook mode, schema support, readiness signal, and transport mode as a versioned descriptor shared by orchestra core and adapters.

## Acceptance Criteria

- [ ] `AC-001` pane-mode launch 전에 provider별 preflight receipt가 생성되고 잘못된 `cwd` 바인딩이 차단된다.
- [ ] `AC-002` prompt transport mutation은 completion wait 전에 탐지된다.
- [ ] `AC-003` hook timeout은 structured event와 failure bundle을 남긴다.
- [ ] `AC-004` degraded run은 surviving provider로 계속 진행되며 provider-level failure reason을 남긴다.
- [ ] `AC-005` secret-bearing prompt and launch metadata are redacted before persistence.
- [ ] `AC-006` failure bundle과 replay ledger는 versioning, permission, retention 규칙을 따른다.
- [ ] `AC-007` receipt write failure는 partial context로 표면화되고 silent drop이 없다.
- [ ] `AC-008` capability mismatch는 deterministic downgrade 또는 explicit fail로 처리된다.

## Out of Scope

- 새로운 debate 전략 추가
- 새로운 provider 자체 추가
- skill/router 텍스트 품질 개선
- subprocess backend를 기본값으로 뒤집는 정책 결정
- multi-provider scoring 로직 재설계

## Traceability

| Requirement | Test | Status |
|-------------|------|--------|
| R1 | AC-001 | implemented |
| R2 | AC-001 | partial |
| R3 | AC-002 | partial |
| R4 | AC-008 | partial |
| R5 | AC-007 | implemented |
| R6 | AC-004 | implemented |
| R7 | AC-003, AC-004 | implemented |
| R8 | AC-003 | implemented |
| R9 | AC-004 | implemented |
| R10 | AC-005 | implemented |
| R11 | AC-006 | implemented |
| R12 | AC-006 | implemented |
| R13 | AC-001, AC-008 | deferred |
| R14 | AC-003, AC-006 | implemented |
| R15 | AC-004 | deferred |
| R16 | AC-006 | deferred |
| R17 | deferred standalone doctor follow-up | deferred |
| R18 | deferred capability descriptor follow-up | deferred |

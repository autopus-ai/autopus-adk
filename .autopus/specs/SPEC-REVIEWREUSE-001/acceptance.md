# SPEC-REVIEWREUSE-001 Acceptance Criteria

Every scenario runs against fake backends, a stubbed readiness probe, and temporary SPEC directories (inside a temporary git repository where a scenario says so); no scenario starts a real provider. "Calls" counts `ExecutionBackend.Execute` requests the fake records. Unless a scenario says otherwise, reviewers are claude and codex, the judge is claude, and `max_revisions` is 0.

## Test Scenarios

### S1: AC-RR-001 - The judge mode is configured, validated, and printed
Priority: Must
Given configurations with `spec.review_gate.judge_mode` set as below
When the configuration loads and `auto spec review` plans a review
Then `on_disagreement` and `on_escalation` load, an omitted key means `always` and a saved default configuration contains no `judge_mode`, and `sometimes` fails with `spec.review_gate.judge_mode must be one of always, on_disagreement, on_escalation, got "sometimes"`
And `--judge-mode maybe` fails with `--judge-mode "maybe": expected one of always, on_disagreement, on_escalation` before any readiness probe or call
And under `on_disagreement` the plan prints `  judge: claude (on_disagreement: 판정이 갈리거나 차단·escalation 항목이 일부 리뷰어에만 있을 때) — spec.review_gate.judge` and `  호출: 라운드당 예상 2 + judge 0-1, 상한 5 (JSON 재요청 포함) · 이번 실행 상한 5 (fallback 0개 포함)`, while under `always` both lines read as on 48607ed2.

### S2: AC-RR-002 - The always mode keeps today's judge
Priority: Must
Given no `judge_mode` and both reviewers answering PASS with no finding
When one round runs
Then the calls are claude reviewer, codex reviewer, and claude judge with reason `adjudication`, the verdict is PASS, and the receipt's `judge` has `mode` `always` and `status` `ok`.

### S3: AC-RR-003 - on_disagreement calls the judge only for a disagreement
Priority: Must
Given `judge_mode: on_disagreement` and a fake judge that returns PASS rejecting every finding with reason `not supported`
When one round runs per row
Then the judge calls, judge status and reason, and verdict are as below, and stderr shows `SPEC 리뷰 judge 생략: consensus_pass (on_disagreement)` for the first row

| claude | codex | judge calls | judge status | verdict |
|--------|-------|-------------|--------------|---------|
| PASS, no finding | PASS, no finding | 0 | `skipped` `consensus_pass` | PASS |
| REVISE, `[major][completeness] REQ-001 X` | REVISE, the same finding | 0 | `skipped` `consensus_block` | REVISE |
| PASS, no finding | REVISE, `[major][completeness] REQ-001 X` | 1 | `ok` | PASS |
| PASS, no finding | PASS, `[critical][security] REQ-002 Y` | 1 | `ok` | PASS |
| REVISE, `[major][correctness] REQ-001 X` | exits 1 | 0 | `skipped` `single_reviewer` | REVISE |

### S4: AC-RR-004 - on_escalation calls the judge only for a contested escalation
Priority: Must
Given `judge_mode: on_escalation` and the same fake judge
When one round runs per row
Then

| claude | codex | judge calls | judge status | verdict |
|--------|-------|-------------|--------------|---------|
| PASS, no finding | PASS, no finding | 0 | `skipped` `no_escalation` | PASS |
| PASS, no finding | REVISE, `[major][completeness] REQ-001 X` | 0 | `skipped` `no_escalation` | REVISE |
| PASS, no finding | REVISE, `[critical][correctness] REQ-001 X` | 1 | `ok` | PASS |
| REJECT, no finding | REVISE, `[minor][data_loss] REQ-003 Z` | 0 | `skipped` `unanimous_escalation` | REJECT |

### S5: AC-RR-005 - The deterministic merge keeps minority safety claims with provenance
Priority: Must
Given reviewers claude, codex, and gemini, no judge, threshold 0.67, claude and codex voting PASS with no finding
When gemini votes REVISE with the finding in each row
Then

| gemini finding | verdict on 48607ed2 | verdict now | kept finding |
|----------------|---------------------|-------------|--------------|
| `[critical][correctness] REQ-001 Rollback loses committed rows` | PASS | REVISE | severity critical, `Sources` [gemini] |
| `[minor][data_loss] REQ-001 Purge drops the archive` | PASS | REVISE | `Sources` [gemini] |
| `[major][completeness] REQ-001 Error path undefined` | PASS | PASS | none (threshold, unchanged) |

And when claude and codex both report `[minor][correctness] REQ-001 Retry is unbounded` and `[critical][correctness] REQ-001 Retry is unbounded` respectively, the merged finding has severity `critical` and `Sources` [claude, codex].

### S6: AC-RR-006 - A judge that cannot rule never turns a safety claim into PASS
Priority: Must
Given `judge_mode: on_disagreement`, claude PASS with no finding, and codex PASS with `[critical][security] REQ-002 Token logged`
When the judge exits 1, when it times out, and when `budget.max_calls: 2` refuses it
Then each run ends REVISE with that finding open and blocking policy `open_security_finding`; the receipt `judge.status` is `failed` for the first two, and the third has `loop_status` `budget_exhausted` and a judge call row with end reason `budget_exhausted`.

### S7: AC-RR-007 - The receipt records every judge decision
Priority: Must
Given `judge_mode: always` and a judge that accepts `F-1` from Reviewer A, rejects `F-2` from Reviewer B with a 600-rune reason, and merges `F-3` from Reviewer B into `F-1`
When the round ends
Then `judge.decisions` holds three entries in judge order with reviewers [claude], [codex], [codex], decisions `accept`, `reject`, `merge`, `merge_into` `F-1` on the third, and the second reason truncated to 500 runes; a judge with no findings writes no `decisions` key.

### S8: AC-RR-008 - The fingerprint names every input
Priority: Must
Given a SPEC in a temporary git repository, claude args `--model claude-opus-5-5 --effort high`, and codex args `-m gpt-6-astra`
When one review ends PASS and promotes the SPEC to `approved`
Then the receipt `fingerprint` has `schema` `spec_review_fingerprint.v1`, `complete` true, `documents` keys exactly `acceptance.md`, `plan.md`, `research.md`, `spec.md` with 64-hex digests, the spec.md digest equal to the digest computed before the promotion, `code.head` equal to `git rev-parse HEAD`, both providers `pinned` true, and `evidence` digests equal to SHA-256 of `review-findings.json` and `review.md`
And the same review outside a git repository writes `complete` false with reason `code: not a git work tree`.

### S9: AC-RR-009 - Unchanged input returns the previous result with no call
Priority: Must
Given a first invocation that ended with each result below and an unchanged repository
When `auto spec review` runs again
Then it makes 0 calls and 0 readiness probes, prints `SPEC 리뷰 재사용:` with the first run id and `finished_at`, `판정: <verdict>`, `리뷰 상태: <state>`, `모델 호출 0회 — 다시 리뷰하려면 --force-review (같은 세션 예산 사용)`, leaves `review-receipt.json`, `review-findings.json`, `review.md`, and the SPEC status byte-identical, and appends a session attempt with end reason `reused` and no call

| first result | printed state |
|--------------|---------------|
| REVISE, one open `[major][correctness]` finding | `awaiting_changes` |
| REJECT, `provider_reject` | `revisions_exhausted` |
| PASS with pinned selectors | `converged` |

### S10: AC-RR-010 - Any change or damage invalidates the previous result
Priority: Must
Given the REVISE first invocation of S9
When exactly one item below changes before the second invocation
Then the second invocation makes 2 calls and stderr has `리뷰 재사용 안 함:` containing the text in the row

| change | text |
|--------|------|
| a line appended to plan.md | `documents: plan.md` |
| a new commit touching `internal/x.go` | `code: head` |
| an uncommitted edit of `internal/x.go` | `code: worktree` |
| the `--required-document docs/ref.md` content | `references: docs/ref.md` |
| `pass_criteria` | `policy: pass_criteria` |
| the checklist body (stubbed digest) | `checklist` |
| codex `-m gpt-6-astra` to `-m gpt-6-astra-pro` | `providers: codex` |
| claude `--effort high` to `--effort max` | `providers: claude` |
| `review-receipt.json` replaced by `{` | `receipt: 손상` |
| `fingerprint.complete` set to false | `fingerprint: 불완전` |
| one byte of `review-findings.json` | `evidence: review-findings.json` |
| `finished_at` moved 169 hours back | `168시간 초과` |

And a PASS first result whose claude args are `--model opus` is re-reviewed with `PASS: 모델 alias (claude: opus)`.

### S11: AC-RR-011 - Only complete, undegraded results are reusable
Priority: Must
Given previous receipts with matching fingerprints
When the reuse decision runs
Then PASS `passed` `converged` with pinned selectors is reused; PASS with degraded reason `provider_quorum`, PASS with `gate_status` `degraded`, REJECT with `loop_status` `budget_exhausted`, REVISE with `provider_unavailable`, and REVISE with blocking reason `no_usable_review` are not; REJECT `revisions_exhausted` and REVISE `revisions_exhausted` with `open_blocking_finding` are reused, the latter reported as `awaiting_changes`; and with a judge selector `sonnet` a PASS is not reused while a REJECT is.

### S12: AC-RR-012 - --force-review re-runs against the same budget
Priority: Must
Given the REVISE first invocation of S9 under `budget.max_calls: 4`
When the second invocation passes `--force-review`
Then it makes 2 calls in the same session, the session attempt and the receipt `session` have `forced_review` true, and a third `--force-review` invocation makes 0 calls and reports `budget_exhausted`.

### S13: AC-RR-013 - The verify input is configured and validated
Priority: Must
Given `spec.review_gate.verify_input` values `delta`, omitted, and `partial`
When the configuration loads
Then `delta` loads, omitted means `full` and a saved default configuration has no `verify_input`, `partial` fails with `spec.review_gate.verify_input must be one of full, delta, got "partial"`, and `--verify-input partial` fails with `--verify-input "partial": expected one of full, delta` before any probe or call.

### S14: AC-RR-014 - A local fix is verified from the delta input
Priority: Must
Given `verify_input: delta`, `max_revisions: 1`, a SPEC whose plan.md contains `PLAN-BODY-MARKER` and research.md `RESEARCH-BODY-MARKER`, a traceability row linking REQ-002 to S2, and round 0 opening `F-001 [major][completeness] REQ-002 error path undefined`
When the author's edit (made by the fake backend after round 0) changes only the REQ-002 paragraph and the S2 scenario
Then `review-findings.json` held after round 0 a `baseline` with `schema` `spec_review_baseline.v1`, revision 0, four documents with content and digests, and open ids [F-001]
And the round-1 reviewer prompt contains `### Delta Verify Input (spec_review_delta.v1)`, manifest lines ending `changed` for spec.md and acceptance.md, `linked (baseline <run> r0)` for plan.md and research.md, and `included` for the checklist, the removed and added REQ-002 lines with `-` and `+`, the full REQ-002 block, the S2 section, the REQ-002 traceability row, `F-001` with its changes, and `Instructions (Verify Mode)`; it contains neither body marker and is shorter than the full verify prompt for the same round.

### S15: AC-RR-015 - The delta input falls back on the first failed condition
Priority: Must
Given the S14 state after round 0 and one change per row
When the round-1 input is chosen
Then

| change | input | reason |
|--------|-------|--------|
| only the REQ-002 and S2 edits | delta | `delta` |
| `baseline` removed | full | `missing_baseline` |
| stored plan.md content edited | full | `baseline_hash_mismatch` |
| checklist digest differs | full | `context_changed` |
| an uncommitted edit outside the SPEC directory | full | `context_changed` |
| a line added under `## Outcome Boundary` | full | `interface_change` |
| a new `**REQ-004**` block | full | `interface_change` |
| a new `notes.md` in the SPEC directory | full | `interface_change` |
| F-001 `scope_ref` `REQ-999` | full | `unclear_impact` |
| an edit of REQ-003, which no open finding names | full | `out_of_scope_change` |
| no document change | full | `no_change` |
| 3000 lines added inside the REQ-002 block | full | `no_saving` |

### S16: AC-RR-016 - Each round states its input and fallbacks spend the same budget
Priority: Must
Given `verify_input: delta` and the S14 run with a `## Outcome Boundary` edit instead
When the review runs
Then stderr shows `SPEC 리뷰 입력: full — interface_change` before round 1, the receipt `rounds` read input `full` reason `discover` and input `full` reason `interface_change`, and the session counts 4 calls; with the S14 edit the second entry reads input `delta` reason `delta` and stderr shows `SPEC 리뷰 입력: delta (baseline <run> r0)`.

### S17: AC-RR-017 - A delta manifest that cannot account for a source is refused
Priority: Must
Given a delta manifest built for S14
When research.md is removed from it, when research.md stays `linked` after its current digest changes, and when spec.md is `changed` without its diff
Then validation fails each time and the round uses the full input with reason `integrity_gap`.

### S18: AC-RR-018 - A delta round still catches new safety findings and the full path is unchanged
Priority: Must
Given the S14 delta round in which codex also reports `[critical][security] REQ-001 Token logged`
When the round resolves
Then the finding is open with the escape hatch and the verdict is REVISE with `open_security_finding`
And with `verify_input: full` the round-1 prompt equals the one the 48607ed2 builder produces, and the existing prompt, context-delivery, and integrity tests pass unchanged.

### S19: AC-RR-019 - The verification commands pass
Priority: Must
Given the branch with all tasks done
When the commands in `plan.md` `## Verification` run
Then each exits 0, including `go test -race` over the judge decision, merge, fingerprint, and reuse tests.

### S20: AC-RR-020 - The change stays inside its ownership and size limits
Priority: Must
Given the branch `feat/spec-review-reuse-delta`
When `git diff --name-only feat/spec-review-policy-budget...HEAD` and `go run ./cmd/source-lines --max 300 --ext .go` over the changed Go files run
Then every path matches the owned list in `spec.md` and source-lines exits 0.

## Oracle Acceptance Notes

Every Must scenario pairs a concrete input with an exact expected output: call counts by provider and role, judge status and reason, verdicts and blocking policies, printed lines, JSON fields and digests, chosen input and reason, and error texts.

- S1 to S7 fix the conditional judge (INV-RR-01 to INV-RR-06): S3 and S4 are decision tables, S5 compares with the 48607ed2 verdicts, S6 covers failure, timeout, and budget. S8 to S12 fix reuse (INV-RR-07 to INV-RR-09): S10 changes one input per row, S11 is the reusable-result table.
- S13 to S18 fix delta verify (INV-RR-10 to INV-RR-12), with S15 as the fallback table in REQ-RR-DELTA-03 order; S19 and S20 fix verification and scope (INV-RR-13).

Tolerance statement: no value has a tolerance; times are compared as recorded strings and digests byte for byte.

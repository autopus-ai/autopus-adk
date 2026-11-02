# SPEC-REVIEWPOLICY-001 Acceptance Criteria

Every scenario runs against fake backends and temporary SPEC directories; no scenario starts a real provider. "Calls" counts `ExecutionBackend.Execute` requests the fake records. Unless a scenario states otherwise the configuration is `spec.review_gate.providers: [claude, codex, gemini]`, `judge: claude`, `max_revisions: 0`, `auto_collect_context: false`, readiness skipped, and the fake answers every reviewer with `{"verdict":"PASS","summary":"ok","findings":[]}` and the judge with `{"verdict":"PASS","findings":[],"rationale":"ok"}`.

## Test Scenarios

### S1: AC-RP-001 - The policy table is exact
Priority: Must
Given the policies returned for `compact`, `lean`, and `full`
When their fields are read
Then the lean required sections are spec.md `## Outcome Boundary`, `## Requirements`, `## Traceability Matrix`; plan.md `## Tasks`; acceptance.md `## Test Scenarios`; research.md `## Outcome Lock`, `## Reviewer Brief`
And the lean allowed omissions are plan.md `## Implementation Strategy`, `## Visual Planning Brief`, `## Feature Completion Scope`, `## Risk-First Integration Probe`; acceptance.md `## Oracle Acceptance Notes`; research.md `## Visual Planning Brief`, `## Semantic Invariant Inventory`, `## Minimality Decision Matrix`, `## Feature Coverage Map`, `## Completion Debt`, `## Evolution Ideas`, `## Reference Discipline`, `## Self-Verify Summary`, and they contain every full required section that lean does not require and none that lean requires
And the lean checklist waivers are `Q-COMP-08` for `## Risk-First Integration Probe`, `Q-COMP-07` for `## Completion Debt` and `## Evolution Ideas`, `Q-CORR-04` for `## Reference Discipline`
And the full required sections equal the set `ValidateSpecSet` enforced on 109c7ea4, the compact policy requires `change.md` with `## Referenced SPEC`, `## Intended Surface`, `## Verification Plan`, `## Gate Applicability`, and every profile lists the mandatory gates `security`, `validation`, `data_loss`, `deterministic_oracle`
And the execution shapes are lean 1 reviewer, judge none, quorum 1; full all configured reviewers, the configured judge, quorum from configuration; compact no SPEC review.

### S2: AC-RP-002 - An ineligible lean declaration faces the full gates
Priority: Must
Given four SPEC sets that contain only the lean floor and differ in frontmatter
When `ValidateSpecSet` and `EffectiveReviewPolicy` run
Then the results are

| frontmatter | effective profile | error findings |
|-------------|-------------------|----------------|
| `profile: lean`, `change_class: bugfix_existing_contract`, `surface: internal/cli/foo.go` | lean | none |
| `profile: lean`, `change_class: feature`, `surface: internal/cli/foo.go` | full | the profile error with `feature`, plus `authoring preflight 섹션 누락: ## Self-Verify Summary` and `## Risk-First Integration Probe` |
| `profile: lean`, `change_class: test_only`, `surface: pkg/auth/session.go` | full | the surface escalation error, plus the same missing-section errors |
| `profile: lean`, `change_class: docs_only`, no surface | full | the missing-surface error, plus the same missing-section errors |

And on 109c7ea4 the three ineligible rows report only the profile error (regression).

### S3: AC-RP-003 - Reviewer, judge, and runtime map one finding set to one verdict
Priority: Must
Given the finding sets below, each emitted once by a reviewer that votes the expected verdict and once by a judge that votes PASS while accepting every listed finding
When the review loop resolves one round for each
Then both paths resolve the expected verdict and blocking policy

| finding set | expected verdict | first blocking policy |
|-------------|------------------|-----------------------|
| none | PASS | - |
| `suggestion` `completeness` | PASS | - |
| `minor` `style` | PASS | - |
| `minor` `completeness` | REVISE | `open_blocking_finding` |
| two `major` `completeness` | REVISE | `open_blocking_finding` |
| `minor` `correctness` | REVISE | `open_correctness_finding` |
| structured `suggestion` `data_loss` | REVISE | `open_correctness_finding` |
| `suggestion` `security` | REVISE | `open_security_finding` |

And a reviewer that votes PASS with two open `major` findings resolves REVISE, and a judge PASS that accepts one `minor` `correctness` finding resolves REVISE with blocking reason `F-001 (correctness/minor) — open_correctness_finding` (PASS on 109c7ea4)
And a reviewer REJECT with one `critical` `correctness` finding resolves REJECT with `provider_reject`
And the default discover and verify prompts and the rendered judge template each contain every canonical rule line returned by `spec.ReviewVerdictRuleLines()`, and none contains `major <= 2`.

### S4: AC-RP-004 - The reviewer prompt states the purpose and the lean allowances
Priority: Must
Given an eligible lean SPEC, a lean SPEC with `change_class: feature`, a full SPEC, and `pass_criteria: CUSTOM_RULE`
When the discover prompt is built for each
Then every prompt contains `can implementation start`, the canonical rule lines, and `CUSTOM_RULE` after the line `Additional project criteria (they cannot relax the rules above):`
And only the eligible lean prompt contains `Allowed omissions (profile lean)`, followed by all thirteen omitted headings and the waived items `Q-COMP-08`, `Q-COMP-07`, `Q-CORR-04`.

### S5: AC-RP-005 - Lean omissions pass and real lean blockers still block
Priority: Must
Given an eligible lean SPEC whose plan.md has no `## Risk-First Integration Probe` and one reviewer that votes REVISE with no finding and `CHECKLIST: Q-COMP-08 | FAIL | missing Risk-First Integration Probe`
When the loop runs one round
Then the verdict is PASS, the outcome `Q-COMP-08` is `N/A` with a reason containing `lean` and `Risk-First Integration Probe`, and there is no blocking reason (REVISE with `reviewer_checklist_fail` on 109c7ea4)
And the same reviewer output on the full SPEC, on the `feature` lean SPEC, and on the eligible lean SPEC after a `## Risk-First Integration Probe` section is added resolves REVISE with `reviewer_checklist_fail`
And on the eligible lean SPEC a `suggestion` `security` finding resolves REVISE `open_security_finding` and a `major` `completeness` finding `acceptance.md S1 has no expected output` resolves REVISE `open_blocking_finding`
And `ValidateSpecSet` on the eligible lean SPEC whose acceptance only checks that a file exists reports the structural-only error.

### S6: AC-RP-006 - Every guidance surface states the same lean floor and omissions
Priority: Must
Given the lean required sections and allowed omissions read from `pkg/spec`
When the parity test reads `content/agents/spec-writer.md`, `content/skills/spec-review.md`, `content/rules/spec-quality.md`, the Claude workflows, Codex skill, Codex prompt, and Gemini skill auto-plan sources, and the regenerated Codex and Gemini spec-writer and spec-review templates
Then each lean passage names every required heading and every omitted heading, and no lean passage requires an omitted heading
And the judge template contains every canonical rule line, and `go run ./cmd/generate-templates` followed by `git status --porcelain templates` prints nothing.

### S7: AC-RP-007 - The printed plan names exactly the calls that run
Priority: Must
Given the configuration above and the fake backend
When `auto spec review` runs once per row
Then the printed plan and the recorded calls are

| row | SPEC | flags | profile | reviewers | fallbacks | judge | quorum | calls by provider and role | warning printed |
|-----|------|-------|---------|-----------|-----------|-------|--------|---------------------------|-----------------|
| lean default | lean `test_only` | - | lean | claude | codex, gemini | none (lean profile) | 1 | claude reviewer 1 | no |
| single override | lean `test_only` | `--providers codex` | lean | codex | - | none (lean profile) | 1 | codex reviewer 1 | no |
| multiple override | lean `test_only` | `--providers codex,gemini` | lean | codex | gemini | none (lean profile) | 1 | codex reviewer 1 | `--providers codex,gemini` |
| explicit judge | lean `test_only` | `--judge codex` | lean | claude | codex, gemini | codex (--judge) | 1 | claude reviewer 1, codex judge 1 | `--judge codex` |
| high-risk lean | lean `feature` | - | full | claude, codex, gemini | - | claude (spec.review_gate.judge) | 2 | each reviewer 1, claude judge 1 | `feature` |
| raised profile | lean `test_only` | `--profile full` | full | claude, codex, gemini | - | claude (spec.review_gate.judge) | 2 | each reviewer 1, claude judge 1 | `--profile full` |

And every row prints the plan block before its first call, with per-round expected calls equal to reviewers plus judge and the attempt upper bound equal to `(1 + fallbacks) x rounds x (reviewers x 2 + judge)`, and the recorded call count never exceeds that bound.

### S8: AC-RP-008 - Invalid combinations are refused before any probe or call
Priority: Must
Given a full SPEC and the recording fake backend and readiness probe
When `auto spec review` runs with each flag set below
Then it fails with the listed error text, zero model calls, and zero readiness probes

| flags | error contains |
|-------|----------------|
| `--profile quick` | `--profile` and `auto, full, lean` |
| `--profile lean` | `--profile lean` and `lean` eligibility |
| `--judge ""` | `--judge` |
| `--new-session --extend-budget` | `--new-session` and `--extend-budget` |

### S9: AC-RP-009 - Budget configuration loads and validates
Priority: Must
Given `autopus.yaml` files with the `spec.review_gate.budget` sections below
When the configuration loads
Then valid sections load and invalid ones fail with the listed text

| budget section | result |
|----------------|--------|
| absent | loads; all limits 0 (no limit); a saved default config contains no `budget` key |
| `{max_calls: 12, max_minutes: 30, max_tokens: 400000, max_cost_usd: 2.5}` | loads with those values |
| `{max_calls: -1}` | `spec.review_gate.budget.max_calls must be >= 0` |
| `{max_minutes: -1}` | `spec.review_gate.budget.max_minutes must be >= 0` |
| `{max_tokens: -5}` | `spec.review_gate.budget.max_tokens must be >= 0` |
| `{max_cost_usd: -0.5}` | `spec.review_gate.budget.max_cost_usd must be >= 0` |
| `{max_cost_usd: .inf}` | `spec.review_gate.budget.max_cost_usd must be finite` |
| `{max_call: 3}` | the strict decoder's unknown-key error |

### S10: AC-RP-010 - One session spans invocations until it is exhausted, extended, or replaced
Priority: Must
Given reviewers claude and codex, no judge, `budget.max_calls: 4`, and a fake reviewer pair that votes REVISE with one open `major` `correctness` finding
When `auto spec review` runs as separate invocations that share only the SPEC directory
Then invocation 1 makes 2 calls and leaves the session `open` with `calls_used` 2; invocation 2 makes 2 calls with the same `session_id` and `calls_used` 4; invocation 3 makes 0 calls, prints `budget_exhausted`, writes a receipt with verdict REVISE, `gate_status` `blocked`, `loop_status` `budget_exhausted`, and blocking reasons `F-001 ... open_correctness_finding` and `budget_exhausted`, leaves `review-findings.json` byte-identical, and marks the session `budget_exhausted`
And invocation 4 with no flag makes 0 calls; invocation 5 with `budget.max_calls: 6` and `--extend-budget` makes 2 calls in the same session and records one extension `4 -> 6`; invocation 6 with `--new-session` makes 2 calls in a new session whose `previous_session_id` is the first session id
And a session whose last attempt converged starts a new session with `previous_session_id` set on the next invocation
And a `review-session.json` holding `not json`, or schema `spec_review_session.v9`, fails with text naming `review-session.json` and `--new-session` and makes 0 calls; a lock held by another holder fails with text naming the lock and makes 0 calls; a call left `in_flight` by an earlier process is rewritten to `interrupted` and counted in `calls_used`.

### S11: AC-RP-011 - Parallel reservations never overspend
Priority: Must
Given a session ledger with `max_calls` 7
When 20 goroutines reserve concurrently under `go test -race`
Then exactly 7 reservations succeed, 13 are refused with `budget_exhausted`, `calls_used` is 7 after settlement, and the race detector reports nothing
And a round with reviewers claude and codex and judge claude, with 2 calls remaining, makes exactly 2 reviewer calls, records the judge as blocked with end reason `budget_exhausted`, and adds the degraded reason `budget_exhausted`.

### S12: AC-RP-012 - Reviewer, judge, repair, and fallback share one budget, and exhaustion keeps blockers
Priority: Must
Given reviewers claude and codex, judge claude, `max_revisions: 2`, `budget.max_calls: 5`, claude answering `not json` first and a valid REVISE on repair, codex a valid REVISE with one `major` `completeness` finding, and the judge REVISE accepting it
When the review runs
Then round 0 records 4 calls (claude `review`, claude `json_repair`, codex `review`, claude `adjudication`), round 1 does not start because 1 call remains for 2 reviewers, and the receipt shows verdict REVISE, `loop_status` `budget_exhausted`, and blocking reasons `F-001 ... open_blocking_finding` and `budget_exhausted`
And given a session marked `budget_exhausted` whose last receipt is REJECT with `provider_reject` and an open `critical` `correctness` finding, a new invocation makes 0 calls and writes verdict REJECT, blocking reasons `provider_reject` and `budget_exhausted`, the unchanged SPEC status, and a byte-identical `review-findings.json`
And given an eligible lean SPEC whose reviewer claude exits 1, the fallback reviewer codex runs and its call carries reason `provider_fallback` in the same session.

### S13: AC-RP-013 - A spent wall-clock budget cancels in-flight calls and keeps completed ones
Priority: Must
Given a session with 300 ms of wall-clock budget left, reviewer claude answering PASS after 10 ms, and reviewer codex blocking until its context ends
When one round runs
Then codex ends with end reason `budget_exhausted` within 2 s, claude's completed call and output are kept, no further round starts, `loop_status` is `budget_exhausted`, and the session is marked `budget_exhausted`.

### S14: AC-RP-014 - Usage is actual, estimated, or unknown, and unknown is never zero
Priority: Must
Given three settled calls whose responses carry an `actual` usage envelope (input 1200, output 300, cost 0.012, model `gpt-6.1-sol`, effort `high`), an `estimated` envelope (input 1000), and no envelope
When the call rows are written
Then the rows read `actual` 1200 / 300 / 0.012 / `gpt-6.1-sol` / `high`, `estimated` 1000 / null / null / `unknown` / `unknown`, and `unknown` null / null / null / `unknown` / `unknown`, and the third row's JSON contains `"input_tokens": null` and no `"input_tokens": 0`
And with `budget.max_tokens: 1000`, a session whose known usage is 1500 refuses the next reservation with `budget_exhausted`, and a session with one unknown-usage call prints and records the note `max_tokens is not enforceable for 1 call with unknown usage`.

### S15: AC-RP-015 - The receipt and the output show every sub-call, once
Priority: Must
Given `--single-pass`, reviewers claude (`not json` first, valid PASS on repair) and codex, and judge claude
When the review runs
Then `review-receipt.json` keeps `schema` `spec_review_promotion_receipt.v1`, `run_id`, `verdict`, and `loop_status`, and its `session.calls` holds 4 rows, ordered by round, reviewers before the judge, then provider, with call ids `a1-r0-claude-reviewer-1`, `a1-r0-claude-reviewer-2`, `a1-r0-codex-reviewer-1`, `a1-r0-claude-judge-1`, roles and reasons (reviewer, review), (reviewer, json_repair), (reviewer, review), (judge, adjudication), each with a non-negative `duration_ms` and end reason `completed`
And stdout prints 4 call lines, the receipt contains no text of the SPEC body or the provider output, and a receipt written without `session` still loads
And merging rows `a1-r0-claude-reviewer-1`, the same row again, and `a1-r0-codex-reviewer-1` counts 2 calls, while a second `a1-r0-claude-reviewer-1` with a different duration is reported as a conflict and counted once.

### S16: AC-RP-016 - The change stays inside its ownership and size limits
Priority: Must
Given the branch `feat/spec-review-policy-budget`
When `git diff --name-only origin/main...HEAD` and `go run ./cmd/source-lines --max 300 --ext .go` over the changed Go files run
Then every path is an owned path of REQ-RP-SCOPE-01, source-lines exits 0, and `go test ./pkg/spec/... ./pkg/config ./templates ./pkg/content ./pkg/harneval ./internal/cli -count=1 -timeout 40m -p 2` passes.

## Oracle Acceptance Notes

Every Must scenario is oracle-first: concrete inputs paired with the concrete expected output, exact verdicts, blocking policies, printed values, call counts per provider and role, error text, JSON field values, or byte identity, never a bare file-exists or exit-success check.

- S1 fixes the policy table (INV-RP-01); S2 the effective-profile and validator outcome per frontmatter (INV-RP-02); S3 an eight-row finding-set table resolved through both the reviewer and the judge path plus the prompt and template rule lines (INV-RP-03); S4 the prompt sections per profile (INV-RP-04); S5 the waiver and its four counter-cases (INV-RP-05); S6 token parity per surface and a clean regeneration (INV-RP-06).
- S7 is a six-row plan-versus-call table and S8 a four-row refusal table with zero calls and probes (INV-RP-07, INV-RP-08).
- S9 to S15 fix the budget: validation text (INV-RP-09), cumulative counts and state per invocation (INV-RP-09), exact reservation counts under the race detector (INV-RP-10), call rows by role and reason (INV-RP-10), loop status and blockers after exhaustion (INV-RP-11), the null-not-zero encoding (INV-RP-10), and duplicate handling (INV-RP-10). S16 fixes scope and size (INV-RP-12).

Tolerance statement: the only explicit tolerance is the 2 s S13 allows for the cancelled call to end; every other expected value (verdicts, policies, counts, ids, text, JSON values) is exact.

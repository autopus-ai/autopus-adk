# SPEC review policy

`auto spec review` answers one question: can implementation start from this
SPEC without guessing? This page describes the single policy that decides what
a SPEC must contain, what blocks it, and how a review of it runs. Defined by
SPEC-REVIEWPOLICY-001.

## One policy, three profiles

The policy lives in `pkg/spec` (`ReviewPolicyFor`, `EffectiveReviewPolicy`).
`auto spec validate`, the reviewer prompt, the runtime checklist handling, and
the review plan read it; the writer, review, and plan guidance restate it and a
parity test (`templates/review_policy_surface_test.go`) fails when they drift.

| Profile | Documents | Review |
| --- | --- | --- |
| `compact` | `change.md` (`## Referenced SPEC`, `## Intended Surface`, `## Verification Plan`, `## Gate Applicability`) against an existing SPEC | none of its own; the referenced SPEC's approval stands |
| `lean` | the four SPEC files at the lean floor | one reviewer, no judge, quorum 1 |
| `full` | the four SPEC files with every section | the configured reviewers and judge |

Every profile keeps the `security`, `validation`, `data_loss`, and
`deterministic_oracle` gates; none of them can be `not_applicable`.

A SPEC gets the lean policy only when it declares `profile: lean` with a
low-risk `change_class` and a `surface` the path classifier reads at or below
that class. Any other lean declaration is reviewed and validated as `full`, so
a high-risk change cannot skip a full gate by labelling itself lean.

### The lean floor and its allowed omissions

- Lean floor: spec.md `## Outcome Boundary`, `## Requirements`, `## Traceability Matrix`; plan.md `## Tasks`; acceptance.md `## Test Scenarios`; research.md `## Outcome Lock`, `## Reviewer Brief`.
- Lean allowed omissions: plan.md `## Implementation Strategy`, `## Visual Planning Brief`, `## Feature Completion Scope`, `## Risk-First Integration Probe`; acceptance.md `## Oracle Acceptance Notes`; research.md `## Visual Planning Brief`, `## Semantic Invariant Inventory`, `## Minimality Decision Matrix`, `## Feature Coverage Map`, `## Completion Debt`, `## Evolution Ideas`, `## Reference Discipline`, `## Self-Verify Summary`.

A review never reports an allowed omission. The quality checklist items that
only check an omitted section (`Q-COMP-08`, `Q-COMP-07`, `Q-CORR-04`) are
answered N/A, and when a reviewer still reports FAIL for one of them while the
section is absent, the CLI records it as N/A with the reason. A finding whose
only subject is an omitted, absent section is deferred as advisory: it must
name that section and say it is missing, and it must not be in an impact
category, use the escape hatch, name a lean-floor section, or cite a
requirement or acceptance id (`REQ-…`, `AC-…`, `S1`). A lean SPEC that did
write the section is checked as usual. Missing acceptance, security, and
data-loss problems block a lean SPEC exactly as they block a full one.

## What blocks

The reviewer prompt, the judge prompt
(`templates/shared/spec-review-judge.md.tmpl`), and the runtime apply the same
rules:

- Blocking at any severity: an open or regressed finding in category
  correctness, contract, data_loss, or security, and any finding raised through
  the critical/security escape hatch.
- Blocking: any other critical or major finding, and a minor finding outside
  the style, suggestion, and improvement categories.
- Advisory: severity suggestion outside the impact categories, and style,
  suggestion, or improvement findings below major. Wording, a nicer structure,
  and choices delegated to the implementer are reported as suggestion.
- A reviewer's REJECT vote blocks, and so does a failing checklist item of any
  usable reviewer, whatever that reviewer voted.
- PASS only when nothing blocks; REVISE when something blocks and the design can
  be fixed in place; REJECT only when a reviewer or the judge states a
  fundamental redesign.

A judge PASS never waives an accepted blocking finding: the runtime keeps it
open and names it. A judge clears a finding by rejecting it with a reason, or
by accepting it at severity suggestion when it is advice (the impact categories
have no advisory severity). A judge PASS also leaves every reviewer REJECT vote
and checklist FAIL blocking unless the judge dismisses that exact item: an
entry with `decision: "reject"`, a reason, `sources` naming that reviewer, and
`scope_ref` `verdict` (the vote) or the checklist id (for example
`Q-COMP-08`). The receipt lists the dismissals under `judge.dismissals`.

`spec.review_gate.pass_criteria` adds project criteria after these rules; it
cannot replace them.

Changed behaviour (SPEC-REVIEWPOLICY-001):

- The reviewer prompt no longer says that two open major findings are a PASS,
  or that any critical or security finding is a REJECT.
- A judge PASS that accepts a minor correctness, contract, data_loss, or
  completeness finding now resolves REVISE instead of PASS.
- A structured reviewer's `contract` and `data_loss` categories are kept
  instead of becoming `completeness`.
- A judge PASS no longer overrides a reviewer's REJECT vote or checklist FAIL it
  did not dismiss, and a reviewer that votes PASS while reporting a checklist
  FAIL now resolves REVISE.
- `auto spec validate` holds a refused lean declaration to the full section set.

## Judge modes and the merge without a judge

`spec.review_gate.judge_mode` (or `--judge-mode` for one invocation) decides
when a planned judge runs (SPEC-REVIEWREUSE-001):

| Mode | The judge runs when |
| --- | --- |
| `always` (default) | every round with a usable review, as before |
| `on_disagreement` | at least two usable reviews and their verdicts differ, or a blocking item or an escalation item appears in some reviews but not all |
| `on_escalation` | at least two usable reviews and an escalation item appears in some but not all |

A blocking item is a blocking finding, a non-waived checklist FAIL, or a
REJECT vote. An escalation item is a REJECT vote or a critical, `security`,
`data_loss`, or escape-hatch finding. Items are compared one by one: a finding
by its merge group (category, scope, description prefix, whatever its
severity), a checklist FAIL by its item id, and the REJECT vote by itself. So
when one reviewer votes REJECT and the other reports a different data-loss
finding, both items are contested and the judge runs. A skipped judge is recorded as
`judge.status: skipped` with the reason (`single_reviewer`, `consensus_pass`,
`consensus_block`, `no_escalation`, or `unanimous_escalation`) and printed as
`SPEC 리뷰 judge 생략: <reason> (<mode>)`. The plan block shows the mode with
its condition and counts a conditional judge as `+ judge 0-1` per round.

Without a valid judge (none, skipped, failed, invalid, or refused by the
budget) the reviewer findings merge deterministically. Findings with the same
category, scope, and description prefix become one finding with the most
severe severity, the escape hatch if any member had it, and `Sources` naming
every reviewer that raised it. A group carrying a critical, `security`,
`data_loss`, or escape-hatch claim is kept whatever its vote count; other
groups still need the supermajority. So a reviewer that votes PASS while
reporting a critical correctness finding, or a minority critical finding beside
a majority finding, is no longer dropped.

The receipt's `judge` object records `mode` and, for a judge that ran,
`decisions`: each ruling's id, decision, reviewers, scope, reason (truncated to
500 characters), and `merge_into`.

## Reusing a previous result

Every review that runs a round writes a `fingerprint` (`spec_review_fingerprint.v1`)
into `review-receipt.json`: SHA-256 digests of the SPEC documents (spec.md with
its status value normalized), the context-delivery sources and
`--required-document` files, the git `HEAD` and the changed or untracked files
outside the SPEC directory, the injected code context, the checklist, the
policy settings (`doc_context_max_lines` and `pinned_models` included), the
whole effective configuration (`config`, so a change in an external `--config`
file counts too), the auxiliary line budget and the excerpt each auxiliary
document delivers under it (`delivery`), the prompt contract (every prompt
variant through the structured response wrapper, the judge prompts, and the JSON
repair request), and each reviewer's and the judge's model selector and
arguments, plus the digests of `review-findings.json` and `review.md` as the
review left them. The receipt also records `spec_fingerprint`, the SPEC
fingerprint `auto status` compares to tell whether the review is still fresh.

The next `auto spec review` of the same SPEC compares a fresh fingerprint
before any readiness probe or model call. It returns the previous result with
no call when every component matches, the evidence files are unchanged, the
receipt is at most 168 hours old, and the result is reusable:

| Previous result | Reused as |
| --- | --- |
| PASS, `gate_status: passed`, converged, no degraded reason | `converged`, only if every model selector names one immutable model: a dated snapshot (`-YYYY-MM-DD`, `-YYYYMMDD`, or `@YYYYMMDD`) or a selector listed in `spec.review_gate.pinned_models` |
| REJECT, no degraded reason, not cut by the budget or a provider outage | its loop state |
| REVISE, no degraded reason, `awaiting_changes` or `revisions_exhausted`, blockers that wait for the author | `awaiting_changes` |

A reused run prints `SPEC 리뷰 재사용: ...` with the original run id and time,
`모델 호출 0회`, and the `--force-review` remedy; it rewrites no review file
and changes no status, and the session records an attempt with end reason
`reused`. Otherwise the command prints `리뷰 재사용 안 함: <reasons>` (for
example `documents: plan.md; code: worktree`) and reviews as usual. A digit is
not a version: `gpt-4o` or `claude-opus-5-5` may move to another model, so a PASS
from it is reviewed again unless you declare it:

```yaml
spec:
  review_gate:
    pinned_models: [claude-opus-5-5]   # selectors you vouch are immutable
```

An empty value, `default`, or a value ending in `latest` is refused at load. A
project outside git has no code baseline, so it never reuses. `--force-review`
re-reviews unchanged input against the same session budget and records
`forced_review: true` on the attempt and in the receipt.

## Delta verify

`spec.review_gate.verify_input: delta` (or `--verify-input delta`) lets a verify
round send what changed instead of the whole document set. Every round that
produced a usable review and was not cut by the budget stores a `baseline`
(`spec_review_baseline.v1`) in `review-findings.json`: each of spec.md, plan.md,
research.md, and acceptance.md in the sanitized form a complete review input
delivers (secrets replaced by `[REDACTED_SECRET]`, injection phrases
neutralized), with the digest of the file (`source_sha256`) and of that form
(`prompt_sha256`); whether the round delivered each one whole (`delivered`:
`complete`, or `partial` when the inline prompt compacted it); the digests of
every other input; and the ids of the findings it left open. The raw document
text is never stored.

A verify round uses the delta input only if, in this order: a baseline exists;
its stored documents match their digests and its open ids match the prior
findings; the baseline round delivered every document whole; every
non-document input, the configuration, and any other markdown file in the SPEC
directory are unchanged; no SPEC document was added or removed; every source
change shows in the sanitized form; no change touches a risk boundary (the
header before the first `##`, spec.md `## Outcome Boundary`, `## Definitions`,
`## Generated And Changed Surfaces`, `## Related SPECs`, research.md
`## Trust Boundary` and `## Outcome Lock`, or a line that opens a requirement
or a scenario); every open finding's scope resolves in the documents; every
changed line belongs to the scope of an open finding, judged per requirement
block or scenario (a sibling requirement under the same heading is out of scope
even if it mentions the finding's id) and per line elsewhere; something
changed; and the delta prompt is shorter than the full one. The first failed
condition is the reason the round uses the full input instead:
`missing_baseline`, `baseline_hash_mismatch`, `incomplete_baseline`,
`context_changed`, `interface_change`, `integrity_gap`, `unclear_impact`,
`out_of_scope_change`, `no_change`, or `no_saving`; a manifest that cannot
account for a required source is `integrity_gap` too. A project outside git
cannot prove its code unchanged, so it always uses the full input.

Every review request starts a fresh provider session, so the delta prompt
(`spec_review_delta.v1`) is complete in itself and says so. It carries the
unified diff of each changed document, the full text of every section a change
touches, of every section in any SPEC document that names an id of the open
findings or the changes, and of the header and risk-boundary sections of every
document, the matching traceability rows, per finding the changes in its scope,
and in full every other source the full input would carry: the checklist, the
code context, and under a GPT-only panel the context documents. Its manifest
lists each source with both digests as `changed`, `included`, or `linked`; a
`linked` document is one the request does not need: unchanged since a baseline
round that delivered it whole, with every section that names a delta id already
in the request. It keeps the verify instructions, so a delta round still opens a
new critical or security finding. Each round prints `SPEC 리뷰 입력: delta (baseline <run> r<n>)`
or `SPEC 리뷰 입력: full — <reason>`, the receipt's `rounds` record `input` and
`input_reason`, and a fallback round spends the same session budget. With the
default `full`, verify rounds send the full input exactly as before.

## The review plan

`auto spec review` separates three choices that used to be tangled:

| Flag | Chooses | Default |
| --- | --- | --- |
| `--profile auto\|full\|lean` | the review profile | `auto`: the SPEC's effective profile |
| `--providers a,b` | the reviewers | the configured reviewers (`spec.review_gate.providers`, plus discovery under `--multi` on a full review) |
| `--judge <provider\|none>` | the judge | none on lean, `spec.review_gate.judge` on full |

Precedence:

1. Profile: `--profile full` raises a lean SPEC to the full panel. Otherwise the
   SPEC's effective profile applies. `--profile lean` works only on a SPEC that
   qualifies for lean.
2. Reviewers on a lean review: exactly one. With `--providers`, the first name
   reviews and the remaining names are fallbacks, in order; a provider you did
   not name never runs. Without `--providers`, the configured judge reviews
   first when it is also a reviewer, and the other configured reviewers are
   fallbacks. Quorum is 1.
3. Reviewers on a full review: `--providers`, else the configuration. Quorum is
   `min_providers`, or the majority of the configured reviewers.
4. Judge: `--judge`, else none on lean, else `spec.review_gate.judge`.

Before this policy, `--providers` switched the whole lean policy off, so
`--providers codex` on a lean SPEC still ran the configured judge. Now a
setting that changes meaning prints a warning before anything runs:
`--providers` with more than one name or `--multi` on a lean review (one
reviewer, the rest fallbacks; ask for a panel with `--profile full`),
`--judge` on a lean review (adds a judge call per round), and `--profile full`
on a lean SPEC.

`--profile` with an unknown value, `--profile lean` on a SPEC that does not
qualify, and an empty `--judge` fail before any readiness probe or model call.

Before the first model call of every attempt (a lean fallback is a new
attempt) the command prints the plan:

```
SPEC 리뷰 계획 (SPEC-EXAMPLE-001):
  profile: lean — spec.md profile: lean (change_class test_only)
  위험 분류: test_only → test_only (low)
  리뷰어: claude (spec.review_gate (judge 우선 순서))
  fallback: codex, gemini
  judge: 없음 — lean profile 기본값
  quorum: 1
  라운드: 최대 3
  호출: 라운드당 예상 1, 상한 2 (JSON 재요청 포함) · 이번 실행 상한 18 (fallback 2개 포함)
```

The expected count is one call per reviewer and judge per round. The upper
bound adds one JSON re-prompt per reviewer, multiplies by the rounds, and by
every fallback attempt still possible, so it is a ceiling, not a forecast. A
single-pass review is one round, not one model call.

## The review session budget

Every model call a review makes is one call against one session budget: the
reviewers, each JSON re-prompt of a malformed reviewer, the judge, a lean
fallback reviewer, every revision round, and later `auto spec review`
invocations of the same SPEC. Readiness and capability probes are not model
calls.

```yaml
spec:
  review_gate:
    budget:
      max_calls: 12        # model calls in the session
      max_minutes: 30      # active review time across invocations
      max_tokens: 400000   # known input + output tokens
      max_cost_usd: 2.5    # known cost
```

Every key is optional; absent or 0 means no limit for that dimension, so a
configuration without the section records the ledger and limits nothing.
`--max-calls N` and `--max-minutes N` (N > 0) set the limits of a new session,
or of the current one together with `--extend-budget`. On an existing session a
flag value that differs from the session's limit is a usage error that names
`--extend-budget` and `--new-session` and makes no call; a changed
configuration only warns. Calls and minutes are hard limits. Minutes are the
wall-clock time of the invocations that made calls, summed over the session:
waiting between invocations and an invocation that made no call cost nothing,
parallel calls are not counted twice, and an interrupted invocation counts up
to the end of its last call. Tokens and cost are checked before each call
against the usage providers reported; calls that report no tokens make
`max_tokens` unenforceable and calls that report no cost make `max_cost_usd`
unenforceable, and the plan and receipt say so per dimension.

The session lives in `{SPEC_DIR}/review-session.json`
(`spec_review_session.v1`), next to `review-receipt.json`, guarded by
`.review-session.lock` so two invocations cannot spend the same budget.

- Each call reserves a slot before it starts and settles after it ends with its
  duration, usage, model, effort, and end reason. A call that cannot reserve
  does not start; it is recorded with `started: false`.
- A round starts only when the remaining calls cover its reviewers. A round in
  which the budget blocked or cancelled a call ends with `loop_status:
  budget_exhausted`, the degraded reason `budget_exhausted`, and the blocking
  reason `budget_exhausted`; it is never a PASS, never `gate_status: passed`,
  and never promotes the SPEC, and `--allow-degraded` does not override it.
- When the wall-clock budget runs out, in-flight calls are cancelled through
  their context and recorded with end reason `budget_exhausted`; completed calls
  and their outputs are kept.
- A call a crashed process left in flight is recorded as `interrupted` and
  counted as used.

Session lifecycle:

| Session state | Next invocation |
| --- | --- |
| `open` | continues the session with cumulative counts |
| `budget_exhausted` | makes no model call; reports `budget_exhausted` with the last verdict and blockers |
| `converged` | starts a new session linked by `previous_session_id` |

The limits are fixed when a session starts. `--extend-budget` applies the
current limits (configuration and flags) to the open or exhausted session and
records the extension; `--new-session` starts a new session linked to the old
one. They cannot be combined. A session file that does not parse, or has an
unknown schema, stops the review; `--new-session` replaces it.

Usage is `actual` when the provider reported it, `estimated` when the provider
reported an estimate, and `unknown` otherwise; unknown counters are JSON
`null`, never 0. Model and effort are the values the provider reported, or
`unknown`.

`review-receipt.json` keeps every existing field and gains an optional
`session` object: the session and attempt ids, state, end reason, limits, used
counters, active wall-clock time (`duration_ms`), notes, and one row per call
of the invocation, ordered by round, reviewers before the judge, then provider.
Each row has `call_id` (`<attempt>-r<round>-<provider>-<role>-<n>`, the same on
every run), `round`, `role`, `reason` (`review`, `verify`, `json_repair`,
`provider_fallback`, `adjudication`), `provider`, `model`, `effort`, `started`,
`usage`, `started_at`, `duration_ms`, and `end_reason`. The `session` object
holds no prompt, provider output, or secret; the existing fields keep what they
held before, including the judge rationale truncated to 500 characters. After
each run the command prints one line per call and the budget:

```
모델 호출 4회 (세션 orch-…, 시도 a1):
  a1-r0-claude-reviewer-1 r0 reviewer/review claude 41s completed usage=actual in=18234 out=2210
  a1-r0-claude-reviewer-2 r0 reviewer/json_repair claude 12s completed usage=actual in=18402 out=1907
  a1-r0-codex-reviewer-1 r0 reviewer/review codex 5m10s completed usage=unknown
  a1-r0-claude-judge-1 r0 judge/adjudication claude 33s completed usage=actual in=6120 out=880
예산: 호출 4/12 · 시간 5m50s/30m0s · 보고된 token 47753 (무제한) · 보고된 비용 $0.0000 (무제한)
```

Calls with `started: false` were refused by the budget and are not counted;
`interrupted` calls were left in flight by a process that ended and are
counted.

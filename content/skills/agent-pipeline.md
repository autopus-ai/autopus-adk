---
name: agent-pipeline
description: Multi-agent pipeline orchestration skill
triggers:
  - pipeline
  - multi-agent
  - 파이프라인
  - 멀티에이전트
category: agentic
level1_metadata: "Phase map, risk-based delegation, gate receipts, one-step-at-a-time references"
level3_resources:
  - "references/phases.md"
  - "references/delegation.md"
  - "references/gates.md"
  - "references/verification.md"
  - "references/review.md"
  - "references/completion.md"
  - "references/coordination.md"
  - "references/quality-modes.md"
---

# Agent Pipeline

The default execution contract for implementing a SPEC or a compact change
contract. This file is the activation and decision layer: it says which step you
are on and which reference to open for it. Open one reference per step.

| Need | Open |
|---|---|
| the work of a specific phase | `references/phases.md` |
| whether to dispatch a worker, and its ownership rules | `references/delegation.md` |
| gate applicability, evidence reuse, change classes, telemetry | `references/gates.md` |
| what to run at validation, testing, and UX verification | `references/verification.md` |
| review authority, provider tiers, loop termination | `references/review.md` |
| sync readiness and the final receipt | `references/completion.md` |
| native dispatch/messaging/checklist payload shapes | `references/coordination.md` |
| quality, permission, prompt-layer, and monitoring settings | `references/quality-modes.md` |

Worktree isolation details live in `.claude/skills/autopus/worktree-isolation.md`;
read it only when you actually run isolated parallel writers.

## Activation

| Flag | Topology |
|---|---|
| (none) | supervisor-led. Ordinary work runs inline; independent slices are dispatched as workers |
| `--solo` | single session. No workers, reported as solo, not as a degraded pipeline |
| `--team` | the platform's native team profile, when the platform has one |
| `--multi` | risk-tiered provider review on top of the selected topology |

An explicit `--solo`, `--team`, or `--multi` request is honored and reported as
requested. `--multi` is a review modifier, not a topology flag, so it composes
with `--team`. A platform without native team lifecycle support stops with an
unsupported-mode diagnostic rather than substituting another topology.

## Execution decision

Default to doing the work inline. Dispatch a worker only for a genuinely
independent slice — disjoint owned paths, no dependency edge on unfinished work,
enough context to finish alone — or when the step needs an isolated context or a
specialist role the supervisor is not running. File count, line count, and
package count never decide this. Low-risk compact-contract work (`test_only`,
`docs_only`, `small_ui`, `bugfix_existing_contract`) is inline by default: no
planner, no dedicated scaffold or validator worker, and still real verification.

When the run will dispatch workers, confirm the surface-native subagent tool
exists before the first phase, initialize `subagent_dispatch_count = 0`,
`subagent_roles_dispatched = []`, and `degraded_mode = none`, and increment only
on an observed dispatch. If no dispatch can be created or observed, stop and say
so; do not report main-session work as delegated work.

## Phase map
```
Phase 0    preflight, change class, gate receipt   → supervisor
Phase 1    planning                                 → planner (inline when the plan is one unit)
Phase 1.5  failing test scaffold                    → tester (skip: --skip-scaffold, docs_only)
Gate 1     approval                                 → supervisor (skip: --auto)
Phase 1.8  external documentation                   → supervisor (skip: no external dependency)
Phase 1.9  Risk-First Integration Probe gate        → supervisor (not_applicable for low risk)
Phase 2    implementation                           → inline units and worker units
Gate 2     validation                               → validator
Phase 3    tests and minimum sufficient verification→ tester
Phase 3.5  UX verification                          → UX role (UI change sets only)
Phase 4    correctness and security review          → reviewer + security auditor
Final      smoke test, receipts, sync gate          → supervisor
```

`auto pipeline run` dispatches five phases on the full route — plan,
test_scaffold, implement, validate, review — and only implement, validate, and
review when one validated contract and the actual changes recompute as low risk,
with test-first work folded into the implementation step. It prints
`Pipeline route: compact|full — <reason>` and records `route_phase_set`; report
that set as-is. The probe is in-session gate work, not a dispatched phase.

@AX annotation is not a phase: it runs only on explicit opt-in
(`auto spec gates ... --annotation` or a direct request), and the `annotation`
gate is `not_applicable` otherwise.

## Change contract path and gate applicability

Low-risk work takes a compact change contract by default; only high-risk work
authors the full SPEC set. `auto spec change <SPEC-ID> --class <class> --ac
<AC-ID,...> --surface <path,...> --verify "<command>"` writes one `change.md`
referencing an existing SPEC and its acceptance-criteria ids. A declared class
contradicted by the intended surface reports `escalate_to_full_spec`, writes no
contract, and exits non-zero — a routing decision, not a warning to read past.
Risk is never decided by file count. Full table: `references/gates.md`.

Every phase gate records `gate: applicability — reason` drawn from
`required | reusable | not_applicable | blocked`. The value is a deterministic
classifier decision, never an agent judgement: run `auto spec gates <SPEC-ID>
--base <ref>` before the implementation step. It writes
`{SPEC_DIR}/gate-applicability.json` over the closed gate set `spec_authoring,
risk_first_probe, build, unit_tests, integration, security, validation,
data_loss, deterministic_oracle, accessibility, ux_verification, annotation,
provider_review, doc_sync`, and every unit prompt carries those decisions.

`security`, `validation`, `data_loss`, and `deterministic_oracle` are never
`not_applicable`. `reusable` is granted only by an exact-input evidence receipt,
never self-assigned. Evidence recording, reuse conditions, and the telemetry
subrecords are in `references/gates.md`.

The risk-first probe and its evidence rules are in `references/phases.md`;
unexecuted checks remain `not-run`, never PASS.

## Minimality

Reuse existing code and native capabilities before adding dependencies or
abstractions. Choose the smallest change that satisfies the requested outcome,
without weakening the applicable safety and verification gates.

## Verification

Run applicable checks once after integration, over the changed surface, with a
verdict and evidence per acceptance criterion. Preserve explicitly configured
quality thresholds; the default `workflow.coverage_threshold` is `85`, which an
explicit project or route value overrides and `0` turns off.
Screenshot-free runs (`no-capture`) require the four UX oracles before any PASS.

The review loop ends on `loop_status`; `awaiting_changes` means the reviewed
input did not change, so resolve or explicitly defer the named findings instead
of re-reviewing. When `discovery_repeat_detected` is true, do not re-run
discovery on the same input. See `references/verification.md` and
`references/review.md`.

## Prompt Layer Discipline

Keep stable instructions, frozen snapshot recall, and ephemeral task or tool
context as separate prompt layer manifest entries. Dry-run or debug output
reports cache invalidation scope by layer without exposing raw secrets. Rely on
the runtime's own context lifecycle; do not add a second compaction pass.

## Scoped Context Receipt Contract

Keep `supervisor verified delivery` separate from `delegated-worker optional recall`.
The supervisor delivers and verifies the complete required bodies for core
project context and for the resolved SPEC requirement, plan, and acceptance
documents; architecture documents are delivered only when explicitly selected.
Delegated workers may recall only selected optional signature, learning, or
task-declared extra references and must not duplicate required bodies.

Keep worker inputs and returns concise. Use `references/delegation.md` for
context budgets, safe retrieval, and ownership. Validate required paths and
hashes before dispatch; preserve source access instead of replaying raw output.

Use exactly and only the existing five-field worker result schema.
Required return fields:

- `owned_paths`
- `changed_files`
- `verification`
- `blockers`
- `next_required_step`

Migration numbering rule: a unit creating migration files in the same owning repo and migration directory is sequential,
even when its code paths are disjoint. Ownership, isolation, and failure
handling: `references/delegation.md`.

## Completion

Run the real changed path as a smoke test, then pass the Sync Readiness Gate:
record `completion_verdict_preview`, `sync_ready`, `sync_blockers`,
`sync_evidence_refs`, `decision_receipt`, and `spec_status_after_go`. On success
the SPEC status becomes `implemented`; `completed` belongs to the documentation
step. Run `auto telemetry leadtime` with a baseline when a prior run exists and
report no increase in `escaped_defects` or `unresolved_safety_gates`. Report
dispatch counts, roles, and requested-versus-executed topology truthfully.
Receipt shape and checklist: `references/completion.md`.

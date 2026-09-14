# Completion and Sync Readiness

Read this file before reporting the run finished.

## Completion gate

Before returning success:

1. Verify every mandatory acceptance criterion and the Outcome Lock, each with
   its own evidence row.
2. Run the real changed path as a smoke test — launch the thing, exercise the
   changed path, observe the result.
3. Confirm no blocking validation, review, or security finding remains open.
4. Record the changed files, the exact verification commands, the observed
   results, and every probe row that stayed `not-run` with its reason and its
   applicability verdict.
5. Report lead time and confirm no telemetry regression.
6. Produce the sync-readiness package below.

No phase boundary, partial worker receipt, or degraded provider response is a
completion point.

## Sync Readiness Gate

Build a handoff package instead of assuming the documentation step will discover
leftover implementation work.

| Field | Content |
|---|---|
| `completion_verdict_preview` | Outcome Lock, mandatory requirements, mandatory acceptance, Completion Debt, and Evolution Ideas, in the same shape the sync step uses |
| `sync_ready` | `yes` only when the Outcome Lock is satisfied, every mandatory requirement and acceptance criterion is met, and Completion Debt is `none` |
| `sync_blockers` | `none`, or the concrete blockers that prevent marking the SPEC implemented |
| `spec_status_after_go` | `implemented` on success. Never `done` or `completed`; `completed` belongs to the sync step |
| `sync_evidence_refs` | changed files, verification commands, review verdict, and the @AX result or `@AX: not requested` |
| `decision_receipt` | the important minimality choices: reused existing code/helper/pattern, skipped dependency or abstraction, accepted expansion with evidence, and the minimum sufficient verification selected |

If `sync_ready` is not `yes`, stop before the lifecycle bar and report the
blocker. Do not hand off until the implementation scope is closed.

## Lead-time summary

Run `auto telemetry leadtime [--run <SPEC-ID>] [--baseline <SPEC-ID|dir>]
[--json]`, with `--baseline` whenever a prior run exists, and report:

- `time_to_first_slice` and completion lead time;
- `critical_path` as the longest wall-clock path through the phase DAG;
- `reread_count` and `rerun_count` by reason;
- `defects_by_discovery_phase` and `repeat_finding_rate`;
- `estimate_vs_actual`;
- explicitly, that `escaped_defects` and `unresolved_safety_gates` did not
  increase against the baseline.

`regression: true` with a non-zero exit means one of those two went up. That is
a completion blocker, not a note.

## Final receipt

```
## Pipeline Completion Summary

SPEC: <SPEC-ID>
Units: <completed> / <total>
Verification: <commands run and their observed results>
Coverage: <measured> (threshold: <declared or "none declared">)
Review: APPROVE
subagent_dispatch_count: <N>
subagent_roles_dispatched: <roles actually dispatched, or "none (inline)">
degraded_mode: none | solo | blocker
topology: default | team | solo | multi  (as requested, as executed)
completion_verdict_preview: Outcome Lock satisfied, mandatory N/N, acceptance N/N, Completion Debt none
sync_ready: yes
sync_blockers: none
spec_status_after_go: implemented
decision_receipt: <reused existing code/helper/pattern; skipped dependency or abstraction; minimum sufficient verification>
lead_time: first_slice <dur>, completion <dur>, critical_path <phase → phase → phase>
telemetry_regression: no (escaped_defects <N>, unresolved_safety_gates <N> vs baseline)

Changed files:
- <path>
```

When the run was inline, say so: `subagent_dispatch_count: 0` with
`degraded_mode: none` and an inline topology is a correct, non-degraded report.
When `--solo`, `--team`, or `--multi` was requested, report the requested
topology and the one actually executed, and name any difference.

## Completion checklist

- [ ] Every phase that applied was executed in order, and every skipped phase
      names the applicability verdict that skipped it
- [ ] Each gate returned a verdict from a real execution, or a `reusable`
      decision granted by the evidence receipt
- [ ] Phase 1.9 probe gate closed: every row executed, or carried as an explicit
      `not-run` with a reason and an applicability verdict
- [ ] Declared thresholds honored; no invented numeric bar
- [ ] `auto telemetry leadtime` reported first-slice and completion lead time and
      the critical path, with no increase in `escaped_defects` or
      `unresolved_safety_gates`
- [ ] Dispatch counts, roles, and `degraded_mode` reported truthfully
- [ ] Sync Readiness Gate passed with `completion_verdict_preview` recorded
- [ ] Final receipt records the important minimality choices
- [ ] SPEC status updated to `implemented`

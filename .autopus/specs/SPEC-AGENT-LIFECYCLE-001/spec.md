---
id: SPEC-AGENT-LIFECYCLE-001
title: Native lifecycle probes and team usage attribution
status: implemented
created: 2026-09-20
---

# Outcome

Add opt-in, bounded native lifecycle checks and supervisor/worker usage
attribution to the existing local 0.50.118 candidate. Installation, supplied
trace validation, actual native capture and billing completeness remain distinct.

## Requirements

- R1: `doctor agents` inventories six installed CLIs without starting models by
  default. Missing collectors are unverified, not vendor capability failures.
- R2: Normalized lifecycle evidence binds run, supervisor, child and challenge
  identities; evaluates spawn, result, cancellation and cleanup independently.
  A cancellation acknowledgement alone is insufficient. Host exit is not native
  cleanup. Imported traces never become independently verified runtime proof.
- R3: Explicit Codex app-server and authenticated loopback OpenCode collectors
  use native events/API state. They operate only on task-owned IDs, have bounded
  time/output, perform finally cleanup and retain incomplete results on failure.
  Codex model delegation and OpenCode controller-created linked sessions are
  different execution surfaces. Archive does not mean rollout data deletion.
- R4: `telemetry team` uses an expected roster and declared usage scope. Parent
  rollups are excluded from self-call sums; repeated receipts deduplicate,
  conflicting attribution blocks totals, and missing workers/usage stay null.
  Known subtotals, actual USD and catalog estimates remain separate.
- R5: Native OpenCode usage binds session/message identities and preserves
  missing numeric fields. Version-verified component normalization accounts for
  cache and reasoning once. Aborted default zeros do not prove zero consumption.
- R6: Actual bounded probes are executed where collectors exist and their
  outcomes are recorded without turning provider failures into capability PASS.
  No global configuration/authentication changes or release publication.

Remaining platforms have inventory and supplied-trace evaluation, not a newly
implemented live collector. This is not certification of every vendor runtime
or a productivity benchmark. No provider expenditure is inferred from an empty
usage stream.

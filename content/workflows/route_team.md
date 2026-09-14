# Route Team Workflow — Human Contract (Source of Truth)

This document and `route_team.schema.json` remain the source of truth for the
retained Route Team dynamic Workflow. It is not selected by `/auto go --team`;
that flag uses Claude's native implicit named-teammate team. The retained script
is available only through an explicit Workflow launch.

The generated adapter lives at
`templates/claude/workflows/route_team.workflow.js.tmpl` and installs as
`.claude/workflows/route_team.workflow.js`. Generation remains parity-gated.
Route Team reuses Route A's deterministic gate and release-hygiene bridges while
adding test scaffolding, testing, and review phases. Annotation requires an explicit request.

A parity gate compares the phase-id, retry, budget, result-type, and per-phase
model/effort/depth tokens across this markdown, `route_team.schema.json`, and the
derived JS. Any divergence fails generation closed and names the diverging
element.

## Phases (in execution order)

The deterministic team workflow runs exactly seven ordered phases. The phase-ids
below are authoritative and must match `route_team.schema.json` exactly.

### planning

The planning phase produces the implementation plan and task breakdown. It runs
the `planner` agent and does not mutate the repository working tree beyond plan
artifacts.

### test_scaffold

The `test_scaffold` phase produces failing test skeletons (the RED stage) ahead
of implementation. It runs the `test_scaffold` agent so the executor fan-out has
a concrete failing target to satisfy.

### implementation

The implementation phase performs repository-mutating work through a bounded
executor fan-out (`fan_out_cap`) in one shared working tree. Planner assignments
must own disjoint path sets and group interdependent files into the same task.
The Workflow owns sequencing and the bounded loop; it does not request per-call
worktree isolation or a merge lifecycle.

### gate_build_test

The `gate_build_test` phase is the **deterministic gate**. Its verdict derives
from build and test command **exit codes** (`verdict_source: exit_code`), not
from an LLM verdict. The gate is executed outside the JS via the Go runtime (calling
`auto workflow gate`), which runs build and test through an injectable `CommandRunner`
seam and emits a structured `{verdict, verdict_source, build_exit, test_exit}` JSON.
A non-zero build or test exit code yields `verdict: fail`.

When the gate returns a failed verdict, the system runs a RALF remediation loop
computed by `pkg/workflow.RunGateRemediation`. It spawns a fixer (executor) agent
and re-runs the failed segment, bounded by the schema gate retry budget (capped at `MaxRetry`=3).
The loop circuit-breaks and aborts early (Aborted=true, AbortReason="circuit_break_no_progress")
if two consecutive gate evaluations produce the same build/test exit-code signature.

### testing

The testing phase runs the `tester` agent to round out coverage beyond the
initial scaffold and confirm the suite is green.

Once testing completes, the system evaluates the deterministic coverage gate using
`pkg/workflow.EvaluateCoverageGate` with the schema-declared threshold (default 85).
It parses the coverage percentage from the stdout of the coverage command (using the
injectable `CoverageRunner.RunOutput` seam, entirely LLM-free). A measured coverage percentage
below the threshold yields `verdict: fail` (exit-code style) and runs a fixer remediation loop.

### review

The review phase runs the `reviewer` agent (bounded by `verify_votes`) followed
by the `security_auditor` agent. An optional `synthesis` pass merges reviewer
findings when the schema enables it.

If findings trigger a barrier, the system runs a review barrier loop computed by
`pkg/workflow.RunReviewBarrier`. It spawns an executor to fix the findings and re-runs the review
up to the review retry budget (capped at `MaxRetry`=3), aborting with `review_budget_exhausted` if spent.
The verdict is consolidated by `pkg/workflow.ConsolidateReviewVerdict` so a security-auditor
FAIL (Barrier=true, Reason="security_fail") outranks and blocks a reviewer APPROVE.

### release_hygiene

The `release_hygiene` terminal phase enforces release safety before sync:

- **Generated-surface drift gate**: blocks the run when generated surfaces are
  staged without a matching source-of-truth change, and always blocks runtime
  artifacts. The Go runtime executes `auto check --hygiene` outside the JS.
- **Project source limit**: enforces a configured `architecture.max_file_lines`
  through `auto check --hygiene --arch --quiet --staged`; otherwise size is advisory.

## External Go Execution

The deterministic gates for `gate_build_test` and `release_hygiene` are executed
outside the JS Workflow environment by the Go runtime framework. `gate_build_test`
uses `auto workflow gate` CLI command, while `release_hygiene` uses `auto check --hygiene --arch --quiet --staged`.
JS owns sequencing; Go owns policy and exit-code adjudication.

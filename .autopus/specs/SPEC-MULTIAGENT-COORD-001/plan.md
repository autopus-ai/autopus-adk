# Plan

| Owner | Paths | Work |
| --- | --- | --- |
| diagnostics_map | pkg/pipeline/runner.go ParallelRunner, parallel_* | Graph validation, dependency-ready scheduler and deterministic tests |
| root | pkg/workerreceipt/ownership*, parser.go, pipeline/worker_ownership_test.go | Ownership invariants and consumer integration |
| value_audit | OpenCode custom workflow/util + focused tests; Antigravity deferred-tools template + test | Team request fidelity in generated artifacts |
| efficiency_map | Read-only sources and scheduler review | Primary research, frozen findings, verification |
| root | content coordination references, docs, candidate binary | Integration and scope-specific validation |

TDD precedes changes. Use channel-controlled concurrent tests. Review discovery
is followed by verification of frozen findings. Preserve prior source changes
and generated OMP outputs. No source/test file exceeds 300 lines.

Verification: full affected packages with race, source generation convergence,
vet/build and cross-platform compile. Focused CLI pipeline receipt regressions
exercise the consumer; don't repeat the previously passing entire CLI suite for
unrelated commands. The prior 87.6% local whole-repo measurement remains dated
2026-09-19, not a new full-repo measurement for this delta.

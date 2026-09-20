# Acceptance

- AC1: Invalid graphs cause zero backend calls; parent completion gates child
  admission; independent work overlaps within the cap; input-order results hold.
- AC2: Parent error/failed gate blocks descendants. Cancellation stops queued
  work and late success. Delegation-denial evidence survives early return.
- AC3: Declared out-of-scope file changes fail the marked parser and block engine
  acceptance; trusted-root validator rejects scope expansion/prefix collisions.
- AC4: Generated OpenCode and Antigravity surfaces preserve team topology intent
  without invented cross-platform primitives or silent fallback.
- AC5: Research claims have primary links and caveats. Local candidate and
  affected tests pass; no unexecuted live team or productivity claim is made.

## Executed evidence, 2026-09-20

All AC1-AC5 pass for the implemented scope. Logs are under
`/tmp/autopus118-coordination/`.

| Criterion | Evidence |
| --- | --- |
| AC1 | parallel_dependencies_test.go and parallel_admission_test.go: invalid graph zero-dispatch, dependency results, channel-controlled ready admission, cap and order |
| AC2 | Parent error/failed-gate tests; PG-01 denial evidence test; PG-02 cancel-before-response regression repeated 100 times |
| AC3 | workerreceipt/ownership_test.go and pipeline/worker_ownership_test.go; out-of-scope report blocks engine acceptance after one dispatch |
| AC4 | Generated OpenCode team contract and Antigravity legacy/plugin rule contract tests |
| AC5 | Primary-source research review; candidate build, vet, Windows/amd64 and Linux/arm64 builds, source limit, template convergence, sync verify |

Integrated race run passed full packages: workerreceipt, pipeline, adapter/opencode,
adapter/antigravity, adapter/codex, content and templates. CLI pipeline-route
consumer regressions (`TestPipelineRunCmd_|TestResolvePlatform`) also passed with
race enabled. No other passing CLI surfaces were rerun without a change reason.

Review findings PG-01 (denial evidence lost on early return) and PG-02 (late
cancellation could appear successful) were repaired and closed by diff-only
verification. Worker ownership and platform wording review found no open issue.

The native candidate `bin/auto-0.50.118-candidate` was rebuilt from the local
4282114a baseline plus all current uncommitted changes, dated 2026-09-20. No
release tag, provider activation, or new live team benchmark was performed.
The previous 87.6% whole-repo coverage figure remains a September 19 snapshot;
it is not claimed as a new measurement of this delta.

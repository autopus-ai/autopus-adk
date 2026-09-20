# Acceptance

- Corpus: 12 unique seeded behaviors, each original PASS/seeded FAIL verified.
- Runner: no prior correct Git history, no oracle modification accepted, missing
  allowed source cannot fall back to original solution, bounded process cleanup.
- Conditions: matching model/effort/task/deadline and order balance; explicit
  treatment differences and global/cache limitations; scope not generalized.
- Measurement: all scheduled runs accounted for, usage missing remains unknown,
  cached and reasoning subsets not added twice, failures retained.
- Conclusions: no automatic winner or default change from unsupported evidence.

## Executed, 2026-09-20

All required pilot criteria PASS. This is experiment completion, not a finding
that the harness is universally better.

- Corpus: 12 unique mutated source states; 12 original PASS and 12 seeded
  behavioral FAIL before model trials. Duplicate a05/b02 removed before trials.
- Runner/observer/export/permissions: 25 Python tests PASS, Go generator package
  builds, diff check and sync verify PASS. No default configuration changed.
- Sandbox: eight actual no-model Seatbelt checks PASS; original source,
  sibling protocol and symlink escape denied; workspace writes and Go tests work.
- Execution: 36/36 observations, no missing/duplicate task-arm combination,
  equal prompt hash across arms for each task. Serial order frozen before runs.
- All 36 candidate patches pass focused independent original tests. Native
  accepted 12/12; compact/full each 11/12 with one 180s timeout on a06.
- a06 compact also has 2,092 .tmp artifacts under the strict original scope
  rule; protected source changes zero. The classification was not changed later.
- Elapsed sums: native 895.817s, compact 983.576s, full 1044.775s, including timeout.
- Complete usage exists for all arms on the same 11 tasks: total tokens
  1666098 / 1815050 / 1920267. The two timeout totals remain unknown. Their
  observed subtotal zero does not mean zero spend.
- Independent audit reparsed all 36 JSONL records and recomputed all summary
  metrics, matching saved records/analysis exactly. Requested model alias is
  fixed; provider-resolved snapshot is unverified.
- Existing candidate `telemetry harness` validates the exported session
  aggregate evidence. Its complete=false preserves missing timeout usage.

Sources: [report](../../../docs/benchmarks/harness-2026-09-20.md),
[portable observations](../../../docs/benchmarks/harness-2026-09-20.json).
Private full logs and protocol: /tmp/autopus-harness-bench-20260920/trials/.
Sanitized local runtime copies: .autopus/runtime/harness-benchmark/2026-09-20/.
No commit, push, release or production default change was performed in this
experiment. Earlier a05ce69d is the frozen input, not a commit of these results.

# Acceptance

- AC1 / R1: Default/full/explicit/pinned/platform audit cases identify reasons;
  local missing, duplicate and alias paths are reported; no body text is emitted.
- AC2 / R2-R3: Allowed, excluded, missing marker, unknown version and mismatched
  version cases replay deterministically; a wrong expected result fails.
- AC3 / R4-R5: Read-only planning changes neither config nor receipt. Existing
  successful evidence can be reused; input changes invalidate it. No-reuse
  forces required work. Workflow prose does not weaken safety checks.
- AC4 / R6: Comparable corpus produces per-arm counts/cost/time and valid paired
  comparisons. Missing/incompatible rows never masquerade as wins; failed and
  retry usage remains visible. Inputs are observations, not generated claims.
- AC5 / R7: Malformed/unknown/oversized inputs, unsafe paths and symlinks have
  tested rejection or explicit incomplete diagnostics. No provider calls occur.
- AC6 / R8: Candidate builds, relevant tests and vet pass, generated templates
  are current, source/test line limits hold, and release readiness accurately
  distinguishes local validation from CI and signed cohort/publication.

## Executed evidence (2026-09-19)

All AC1-AC6 are satisfied for the local implementation scope. Logs are under
`/tmp/autopus118-upgrade.BKeBpC/`; no live productivity gain or publication is
inferred from these results.

| Criterion | Evidence |
| --- | --- |
| AC1 | TestSkillAudit + TestExplainSkillSelection; candidate CLI audit on all five platforms; audit-final.json |
| AC2 | pkg/skillpolicy race tests; TestSkillSelect/TestSkillPolicyCheck; executable policy-select/replay and deliberately wrong expected case nonzero exit |
| AC3 | TestSpecGatesReadOnlyPreservesConfigAndReceipt, TestSpecGatesNoReuseKeepsFreshEvidenceRequired, existing reuse suite; real config/receipt before-after smoke |
| AC4 | pkg/experiment tests, TestTelemetryHarness, failure/retry/unknown and identity mismatch coverage; empty template prints incomplete/null totals |
| AC5 | Strict input/duplicate keys/path bounds, symlink/FIFO/oversize tests; AUD-001/HC-01 closed by diff review |
| AC6 | Native candidate build; Windows/amd64 and Linux/arm64 full cross-builds; go vet; arch gate; template generation convergence; fixture upgrade canary PASS |

Verification sequence, retaining the initial failure:

1. Repository-wide `go test -race -count=1 -timeout=20m -skip <Makefile PROCESS_HEAVY_TESTS> -coverprofile=... ./...`
   passed 115 packages and failed only internal/cli's two existing macOS
   deny-default sandbox fixtures because coverage output was denied.
2. The complementary isolated race lane (`-p 1 -run <same selector>`) passed.
3. Both failing cases were reproduced and then passed with `-race -coverprofile`
   after a test-only fixture repair. Child counter inclusion was checked.
4. Final changed-domain and CLI-focused tests passed with race enabled.
5. Current `internal/cli`, `pkg/experiment`, and `pkg/skillpolicy` full suites
   passed with atomic coverage instrumentation (without race for this refresh).
   Stale shared coverage for exactly those package directories was removed and
   replaced; the isolated coverage lane was retained. Final local coverage:
   **87.6%**, above the unchanged **85%** threshold. This is local evidence,
   not an exact-commit protected CI result.

Final source binary: `bin/auto-0.50.118-candidate` (baseline 4282114a plus local
changes). The upgrade fixture uses its historical v0.50.108 fixture and required
0.50.109-canary source-version label; it proves current source compatibility,
not public v0.50.117-to-v0.50.118 signed asset admission.

`auto sync verify` from the candidate passes and excludes 17 pre-existing OMP
generated/runtime paths. Those files remain local and are not part of this
upgrade's canonical change set. The native PATH-installed release is unchanged.

Implementation blockers: none. Publication, a signed live OMP cohort, and real
native/current/reduced model trials remain outside this local implementation
acceptance. The A29 release coordinate and trust policy were not changed.

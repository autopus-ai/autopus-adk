# v0.50.118 (A29) local candidate preparation

Status: local upgrade implemented and validated, not published by this work.
Source baseline: `4282114a`, with the local changes described by
`SPEC-HARNESS-EFFICIENCY-001`. Existing local generated OMP files are preserved.

## Included changes

- Read-only skill exposure diagnostics with configured/local/runtime boundaries.
- Explicit skill applicability policies and positive/negative/version replay.
- Three-arm observational comparison retaining unknowns, failures and retries.
- Read-only gate planning and an explicit no-reuse option.
- Focused verification instructions, corrected README claims, duplicate prompt
  identity rejection, and runnable examples in `docs/examples/harness-efficiency`.
- September 20 coordination follow-up: dependency-ready local ParallelRunner,
  marked worker ownership consistency, and explicit team-request fidelity in
  OpenCode/Antigravity instructions. See [coordination scope](../multiagent-coordination.md).
- Native lifecycle and team usage follow-up: opt-in Codex/OpenCode collectors,
  strict event-chain evidence, and rollup-safe usage attribution. See
  [lifecycle results and boundaries](../agent-lifecycle.md).

- OpenCode V2 adapter follow-up: version-aware plugin and native subagent
  generation, option-preserving config, unknown-version preservation, and
  generated JS/native host tests. See [V2 scope](../opencode-v2-compatibility.md).

- Evidence-based task routing: read-only workflow triage and generated native
  router policy select inline/guided/planned without weakening gates or changing
  model presets. See [task routing](../task-routing.md).

See [usage](../harness-efficiency.md) and [research](../harness-assessment.md).
The new comparison command does not generate live model evidence or replace the
signed OMP promotion verifier.

## Existing release boundary

A29/v0.50.118 is already selected in release scripts and workflows. Do not
advance it to A30 without publishing A29 and measuring its predecessor pins.
The existing OMP runbook records a failed 18.1.13 measurement attempt and the
retained 17.2.7 pin. This upgrade changes neither that pin nor the required
promotion threshold, signing authority or immutable release controls.

Use a separately built `0.50.118-candidate` binary for local verification.
The PATH-installed 0.50.117 binary cannot validate features not in its source.
Do not install a candidate over the signed user binary as a validation shortcut.

## Verification ledger

The final local result is recorded in the
[SPEC acceptance document](../../.autopus/specs/SPEC-HARNESS-EFFICIENCY-001/acceptance.md).
Candidate build, vet, cross-platform compile, CLI smoke and fixture upgrade
checks passed. The first shared race/coverage lane exposed two existing macOS
sandbox fixture failures; test-only repairs passed with race/coverage enabled.
The isolated race lane and refreshed full CLI/domain suites passed. Final local
coverage is **87.6%** against the unchanged **85%** threshold. The acceptance
document records which runs used race and how coverage profiles were refreshed.
This whole-repository percentage is the September 19 efficiency snapshot. The
September 20 coordination delta has its own affected-package evidence in
[SPEC-MULTIAGENT-COORD-001](../../.autopus/specs/SPEC-MULTIAGENT-COORD-001/acceptance.md);
its affected full-package race tests, CLI regressions, vet and cross-builds
passed. The earlier percentage is not a fresh measurement
for that later source state.
Synthetic policy/comparison fixtures prove CLI behavior, not agent productivity
or a passing signed OMP cohort.

Publication remains a separate release operation: validate the exact final
commit, obtain the protected CI and signed live cohort evidence, then use the
existing release-prep workflow. No tag, remote release or Homebrew publication
is created by the local diagnostics commands.

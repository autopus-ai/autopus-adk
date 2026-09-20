# Acceptance

- AC1: Default inventory performs version checks only; supplied trace reports
  neither local installation nor independent native proof.
- AC2: Missing/contradictory/out-of-order/foreign lifecycle observations cannot
  produce an overall PASS; failure and cleanup can be independently reported.
- AC3: Probe cleanup stays inside proven ownership, reconciles late/nested
  descendants, and never equates archive ACK or process kill with native closure.
- AC4: Parent rollups + worker calls do not double count; partial coverage keeps
  total null; zero-call declarations and estimated catalog USD remain distinct.
- AC5: Real probe reports preserve both successes and external/protocol failures;
  new candidate builds and relevant tests pass. No live result is fabricated.

## Executed evidence, 2026-09-20

AC1-AC5 pass for the implementation and honest observation scope. Logs are in
`/tmp/autopus118-agent-lifecycle/`; sanitized native reports are in
`.autopus/runtime/agent-lifecycle/2026-09-20/` and remain untracked.

| Criterion | Evidence |
| --- | --- |
| AC1 | Installed inventory for all six CLIs; supplied trace installation=null and lifecycle_verified=false regressions |
| AC2 | Full agentprobe state-machine, strict input and protocol fixture tests with race |
| AC3 | CPROBE-001/002 repairs and review: root closure/unload, late/nested ownership reconciliation, no foreign cleanup; actual final Codex lifecycle PASS |
| AC4 | Full telemetry tests; rollup/copy/conflict/missing/cost tests; OpenCode native usage bridge and nullable component tests |
| AC5 | CLI doctor/team regression tests with race, candidate build, vet, Windows/amd64 and Linux/arm64 compile, source size and sync checks |

Actual native outcomes:

- Codex 0.155.1: final `codex-live-filtered.json`, exit 0, spawn/result/cancel/
  cleanup PASS, eight normalized native events. Prior captures remain separate
  unknown/blocked records, never retrospectively promoted.
- Real tracing exposed V2 subAgentActivity and a reproducible observer failure:
  300 irrelevant token deltas could exhaust the 256-notification queue during
  child reads. Discriminator-first parsing, lifecycle-only queue admission,
  meaningful-event refresh and bounded transient-read retry repaired these
  concrete code paths. Queue/output limits were not increased. The exact missing
  historical transient frame is not claimed to have been reconstructed.
- OpenCode 1.18.7: native root/child sessions created; model result/cancel chain
  not verified. In `--pure` mode (external plugins disabled), the diagnostic
  attempt recorded APIError HTTP403. Twelve owned sessions across four attempts
  were confirmed absent. A separate normal-plugin server with existing config
  and openai/gpt-5.4 returned UnknownError; its three sessions were confirmed
  absent. Both task-owned servers stopped. No account-level cause is established.
  Configured opencode/gpt-5.4 was absent from that provider's available model list;
  global config/auth was not changed to bypass failures.
- Claude, OMP, Antigravity and Gemini: inventory only, no newly executed lifecycle
  certification. Their missing collectors are not vendor capability failures.

Usage limitations are explicit: OpenCode's catalog estimate is not billing;
incomplete worker capture leaves totals null. Codex call attribution remains
uncollected. No observed total savings or signed release admission is claimed.

Final affected-package race runs: agentprobe and telemetry PASS; CLI selectors
TestDoctorAgents, TestTelemetryTeam and TestDoctorCmd PASS. This delta does not
reuse the earlier 87.6% figure as a new full-repository coverage measurement.

## Orca latest-CLI follow-up

See [the follow-up ledger](../../../docs/orca-cli-multiagent-check.md).
Claude 2.1.278 and OMP 18.2.6 produced native spawn/result/cancel observations;
OpenCode 2.0.10 failed model access and exposed V2 adapter gaps. These manual
checks do not change the implemented collector scope or release cohort. The
version-prefixed OpenCode output regression is covered by CLI tests.

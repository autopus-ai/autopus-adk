---
id: SPEC-HARNESS-EFFICIENCY-001
title: Evidence-driven harness diagnostics and selection
status: implemented
created: 2026-09-19
---

# Evidence-driven harness upgrade for the v0.50.118 candidate

## Outcome

Make the local ADK's skill exposure, selection decisions, repeated verification,
and three-arm evaluation inspectable without adding mandatory model calls or
claiming unmeasured savings. The user authorized implementing the research
recommendations against the existing local checkout; publication is separate.

## Requirements

- R1: `auto skill audit` SHALL distinguish catalog/configured exposure from local
  files and actual session loading. Report policy reasons, aliases, duplicate
  content, missing outputs, byte sizes and explicitly heuristic token estimates.
  Session load remains unknown without runtime observation.
- R2: `auto skill select` SHALL evaluate explicit task-class, exclusion,
  repository-marker and exact-version contracts. Missing facts remain unknown;
  no language-model or keyword guess constitutes suitability proof.
- R3: `auto skill policy-check` SHALL replay positive, negative and incompatible
  cases and return nonzero for a mismatch. Selection SHALL NOT execute skills.
- R4: `auto spec gates --read-only` SHALL preserve project configuration and
  evidence files. `--no-reuse` SHALL keep required gates required even when a
  matching prior receipt exists. Existing input-closure invalidation stays intact.
- R5: Workflow instructions SHALL reuse applicable evidence only with unchanged
  inputs and execution conditions; failed, partial, stale, external-state or
  uncertain results require fresh verification. Required safety checks remain.
- R6: `auto telemetry harness` SHALL compare native/current/reduced arms against
  a frozen task corpus, separating harness identity from shared task/model/
  environment/oracle identity. Failed and retried attempts remain in spend;
  missing actual usage is unknown. No performance or promotion verdict is invented.
- R7: New diagnostics and evaluators SHALL use bounded, strict inputs and emit
  body-free structured results. Filesystem inspection SHALL reject traversal and
  symlink escapes and SHALL NOT mutate project files or invoke providers.
- R8: Preserve local edits, OMP trust/measurement policies, 85% project CI floor,
  public compatibility, and existing generation mechanisms. Source/test files
  stay <=300 lines. Release coordinate remains v0.50.118/A29.

## Non-goals

No new agent host, automatic skill pruning, inferred semantic selection,
provider installation, publication/tagging, fabricated live benchmarks, or
weakening of signed OMP promotion gates. Explicit declared version facts are
not represented as a probed installed runtime version.

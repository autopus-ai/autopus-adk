---
id: SPEC-MULTIAGENT-COORD-001
title: Dependency-aware coordination and ownership evidence
status: implemented
created: 2026-09-20
---

# Outcome

Apply current multiagent research to concrete coordination failures in the
existing local v0.50.118 candidate. Preserve the preceding efficiency upgrade.

## Required behavior

- R1: ParallelRunner validates a unique acyclic graph with known dependencies
  before backend calls; dispatches only dependency-ready phases within the cap;
  supplies direct-parent outputs and retains input-order results.
- R2: Failed dependency gates/errors prevent descendants. Cancellation prevents
  queued admission and cannot be hidden by the final completion. Safety evidence
  records denied delegation and actual admission, not claimed execution-start FIFO.
- R3: Marked worker receipts reject changed files outside declared ownership.
  A public validator can additionally accept supervisor-assigned literal roots.
  Prefix collisions and scope expansion are rejected. Parser self-consistency
  is not advertised as filesystem enforcement or trusted assignment binding.
- R4: OpenCode and Antigravity generated instructions preserve explicit team
  requests. Current schema and integration support must be verified; unavailable
  modes are reported instead of silently substituting ordinary subagents.
- R5: Current official capabilities and empirical results are documented with
  dates, limitations and a distinction between vendor support and ADK integration.

No CLI strategy promotion to native parallel execution, automatic vendor feature
activation, capacity guessed from configuration, or universal agent-count threshold.

# Existing evidence and source boundaries

Reuses processprobe bounded process handling, existing UsageEnvelope normalization,
AggregateUsage, and current native schemas. Ordinary provider transport marker
smoke and OMP provider-free readiness are not lifecycle proof and remain unchanged.

Installed inventory: Codex 0.155.1, Claude2.1.272, OpenCode1.18.7, OMP18.2.2,
Antigravity1.1.26, Gemini0.52.0. Version presence does not certify behavior.
Codex V2 native subAgentActivity differs from older collaboration-only events.
OpenCode v1.18.7 input excludes cache read/write, output excludes reasoning, and
catalog-derived cost is an estimate. Its abort usage can contain default zeros.

Official sources and live limitations are collected in docs/agent-lifecycle.md.
No raw model bodies, credentials, or authentication headers belong in reports.

The exact [1.18.7 plugin loader](https://github.com/anomalyco/opencode/blob/v1.18.7/packages/opencode/src/plugin/index.ts)
disables external plugin origins in `--pure` mode, while built-in Codex OAuth
remains separately loaded. Pure-mode provider failures cannot establish normal
configuration failures. A separate normal-mode observation also failed with
UnknownError; the underlying cause remains unverified.

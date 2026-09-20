# Plan

- Core worker owns pkg/taskroute: strict facts decoder, deterministic route,
  reasons, required steps, narrow suggested skills and parallel recommendation.
- CLI worker owns workflow triage registration and read-only JSON/human command.
- Template worker owns existing Claude/Codex/Gemini router triage sections and
  delegation reference; independent review follows implementation.
- Main owns root guidance, generated adapter parity, docs and integration.

Tests precede behavior changes. Validate route boundary cases, missing facts,
single-file high-risk escalation, repeated failure, explicit route/solo,
independent ownership and malformed input. Verify actual generated surfaces
across supported adapters and candidate CLI examples. Review once and verify
only open findings. No model-based triage call, release or automatic push.

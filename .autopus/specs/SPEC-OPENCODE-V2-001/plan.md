# Plan

Independent ownership:
- Plugin worker: opencode_plugin.go and new V2 plugin/runtime contract tests.
- Configuration worker: opencode_config.go, opencode_lifecycle.go and new helpers/tests.
- Main: runtime version selection, generated Markdown adaptation, integration,
  acceptance and docs. Existing V1 tests receive explicit runtime pins.
- Read-only worker: installed native API verification procedure.

Stages: regression tests; parallel bounded implementation; adapter integration
and actual runtime probe; one discovery review; focused fixes and verification.
All new source/test files remain at most 300 lines.

Risk-first checks:
| Assumption | Boundary | Oracle | Status |
| --- | --- | --- | --- |
| V2 plugin contract loads | Installed 2.0.10 | Generated object dispatched native hooks without SDK | PASS |
| Before failure prevents shell execution | Native tool hook | Denied marker absent in actual 2.0.10 | PASS |
| V1 generation remains available | Existing suite pinned to V1 | Pinned tests and executable V1 contract pass | PASS |

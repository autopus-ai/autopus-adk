# Implementation plan

| Unit | Owner | Paths | Completion |
| --- | --- | --- | --- |
| Exposure audit | diagnostics_map | pkg/content/skill_audit*, skill_catalog_explain*, internal/cli/skill_audit*, skill.go registration | Configured vs local audit with safe bounded scanning |
| Selection/replay | value_audit | pkg/skillpolicy/**, internal/cli/skill_select*, skill_policy* | Deterministic contracts and negative replay |
| Three-arm comparison | efficiency_map | pkg/experiment/harness_compare*, internal/cli/telemetry_harness*, telemetry.go registration | Strict observational comparison with unknown and failure accounting |
| Verification/integration | root | spec_gates.go, pkg/spec/gates/decide.go, focused tests, canonical workflow content, generated templates, docs | Safe read-only planning, no-reuse option, integration and release readiness |

Workers share a filesystem and cannot overwrite other ownership. Write failing
behavior tests first, then implement. Parent integrates constructor registrations.
Local focused tests precede integrated build, vet and repository tests. Process-
heavy fixtures run with the existing Makefile isolation policy. Do not count
synthetic fixtures as live provider evidence. One discovery review freezes
findings; later verification targets only repairs.

## Risk-first integration probe

| assumption | boundary | oracle | status |
| --- | --- | --- | --- |
| Source-built candidate validates its own surface | CLI binary identity | bin/auto-0.50.118-candidate version plus local smoke; single-repo sync verify passes | PASS |
| Gate planning is read-only when requested | config/evidence filesystem | Snapshot before/after in tests with absent and existing receipt; real CLI smoke | PASS |
| Three-arm data is comparable without survivor bias | usage/experiment seam | Missing/failed/retry/mismatched-identity tests and unmeasured-template CLI smoke | PASS |

## Release boundary

A29 already exists in scripts/workflows. Existing runbook records a failed OMP
18.1.13 cohort and retained 17.2.7 pin. Do not change pins or assertions merely
to make release checks green. Prepare a local 0.50.118 candidate, report exact
checks and remaining operator release steps separately.

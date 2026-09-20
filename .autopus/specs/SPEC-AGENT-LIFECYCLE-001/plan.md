# Plan and ownership

| Owner | Paths | Responsibility |
| --- | --- | --- |
| diagnostics_map | pkg/agentprobe core and codex* | Strict lifecycle state machine and native Codex protocol |
| value_audit | pkg/agentprobe/opencode* | Native session API collector, identity and cleanup |
| efficiency_map | pkg/telemetry/team_usage*, telemetry_team*, agentprobe/team_usage* | Attribution, no double count, native usage bridge |
| root | doctor_agents*, doctor.go registration, docs, real probes | Integration and provenance |

TDD + affected-package race tests. Real native probes use task-owned runtimes,
explicit platforms, bounded models/turns, and body-free reports. Preserve the
original failed traces. Do not retry a provider denial as a capability workaround.
Review discovery freezes findings; verify only repaired findings thereafter.

Codex installed schema may differ from web docs: use generated local schema and
native observations. Test local protocol fixtures without billing; live execution
is explicit and separately recorded. Production signing/cohort policies unchanged.

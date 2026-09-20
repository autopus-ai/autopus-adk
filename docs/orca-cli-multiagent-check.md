# Latest CLI checks through Orca, 2026-09-20

This follow-up used Orca 1.4.204 managed terminals to execute the installed CLIs.
It supplements the earlier native driver checks in [agent-lifecycle.md](agent-lifecycle.md).
These are live smoke observations, not a new four-gate certificate, release
admission, or a controlled productivity comparison.

## Updates applied

| CLI | Before | After | Update source |
| --- | --- | --- | --- |
| Claude Code | 2.1.272 | 2.1.278 | npm `@anthropic-ai/claude-code@latest` |
| OMP | 18.2.2 | 18.2.6 | Homebrew `can1357/tap/omp`; `omp update --check` confirmed current |
| OpenCode | 1.18.7 | 2.0.10 | npm `@opencode/cli@latest`, following the official V2 migration |

The old `opencode-ai` package's latest was 1.18.31. It was updated first, then
replaced with the official V2 package after discovering that split. The existing
OpenCode configuration was copied to a private local backup before replacement.
No authentication values or provider configuration were manually rewritten.
The ADK release's pinned OMP cohort was not advanced by this machine-level update.

## What actually ran

Each probe used an empty temporary directory, bounded runtime, native delegation,
a random exact-response token, and no requested repository edits. Claude parent
tools were limited to Agent/TaskOutput/TaskStop, and its workers had no tools.
OMP's final run enabled task and hub; worker assignments forbade file, shell and
network operations. This is an instruction boundary, not an OS sandbox.
OpenCode's separate direct TUI check used the existing checkout with an explicit
read-only, no-file-inspection prompt. Its model request failed.

| CLI | Native observations | Limits |
| --- | --- | --- |
| Claude Code 2.1.278 | Two child tasks; matching SubagentHandback; completed success task; TaskStop ACK plus stopped task notification; parent exit 0 | Full native empty-worker inventory was not collected |
| OMP 18.2.6 | Two task workers; hub wait returned the matching token and completed status; hub cancel returned cancelled terminal status; parent exit 0 | Full native empty-worker inventory was not collected |
| OpenCode 2.0.10 | JSON CLI request failed with provider.auth HTTP 401; direct Orca TUI session also had a failed native outcome; TUI displayed four plugin failures | No successful model-native delegation or cancellation; root cause of authentication failure unverified |

Orca input acceptance was not treated as proof of a model turn. Claude/OMP
results came from native JSON events, and OpenCode outcomes from CLI events and
session exports. The standalone CLI and direct TUI paths were both exercised.

The initial OMP probe enabled task alone and exited without collecting the
worker's result; the corrected probe included hub. The initial Claude cancellation
fixture requested a calculation explanation and encountered a provider safeguard
error before cancellation. It remains a failed observation. The corrected fixture
used ordinary fictional prose, and cancellation was observed while that worker
was running. Parent exit 0 alone would not have established either result.

All four task-created Orca terminals were closed with confirmed PTY shutdown.
Both owned OpenCode sessions were identified by their probe prompts, deleted,
and confirmed absent from native inventory. The V2 service started during this
check was stopped and its listener was absent. Claude and OMP transcripts remain
local. These cleanup checks are not promoted to the stricter agentprobe cleanup
gate because a complete native worker inventory was not collected.

## Concrete ADK compatibility findings

1. The V2 `opencode v2.0.10` version output exposed an inventory parser defect:
   the old word-boundary pattern missed a version prefixed with `v`. The parser
   now strips the optional prefix, with regressions for all observed CLI formats.
2. ADK's generated OpenCode hook plugin uses the V1 hook API. Official V2 docs
   explicitly require porting V1 plugins. File presence and registration cannot
   establish that hooks execute in V2.
3. Generated OpenCode instructions hardcode `task`, while V2 exposes `subagent`.
   The mapping needs a version/tool-catalog-aware implementation and native
   schema tests, not an unconditional rename that breaks V1.
4. The configuration reader checks legacy `plugin`, while native V2 uses
   `plugins` and also accepts object entries. Registration compatibility and
   executable plugin compatibility must be tested separately.

At the end of this smoke check, only the inventory parser was fixed.
The subsequent [V2 adapter implementation](opencode-v2-compatibility.md) addresses
the plugin, tool-mapping and configuration findings with separate acceptance
evidence. It does not retrospectively turn this failed model probe into PASS.

Sanitized local evidence: `.autopus/runtime/agent-lifecycle/2026-09-20/orca/ledger.json`.
Raw logs and the private config backup are under `/tmp/autopus-orca-probes-20260920/`
and are not committed or published. Provider cost fields were not interpreted
as invoices or added across parent/child scopes.

## Primary references

- [OpenCode V1 migration](https://opencode.ai/v2/docs/migrate-v1/): package replacement,
  shared config locations, plugin and server API boundaries.
- [OpenCode V2 commands](https://opencode.ai/v2/docs/cli/commands/): standalone JSON
  execution and native session export/delete.
- [OpenCode V2 subagent tool](https://opencode.ai/v2/docs/tools/#subagent).
- [OpenCode V2 plugin configuration](https://opencode.ai/v2/docs/plugins/#configure).
- [OMP upstream](https://github.com/can1357/oh-my-pi), plus installed 18.2.6 CLI help
  and native task/hub event schemas observed in this run.
- [Claude Code releases](https://github.com/anthropics/claude-code/releases), plus
  installed 2.1.278 CLI help and native Agent/TaskStop event schemas.

Validation: focused CLI race tests (version parser, doctor agents, doctor root,
and telemetry team) passed; candidate build, actual post-build version inventory,
`git diff --check`, and `auto sync verify` passed. No new full-repository test or
coverage measurement is claimed for this follow-up.

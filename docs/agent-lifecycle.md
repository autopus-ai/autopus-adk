# Native lifecycle checks and team usage

The local 0.50.118 candidate adds `auto doctor agents` and `auto telemetry team`.
Use `bin/auto-0.50.118-candidate` when the installed release predates these commands.

## Inventory is not a live capability certificate

```sh
auto doctor agents --format json
auto doctor agents --platform codex --format json
```

Default execution only discovers executables and reads their versions. It does
not start model conversations. Installation, vendor documentation, an imported
trace, and a successfully observed lifecycle are different facts.

The installed inventory on 2026-09-20 was Codex 0.155.1, Claude Code 2.1.272,
OpenCode 1.18.7, OMP 18.2.2, Antigravity 1.1.26, Gemini CLI 0.52.0. The new live
collectors currently cover Codex and OpenCode V1 HTTP APIs. V2 adapter generation
is documented [separately](opencode-v2-compatibility.md) and is not a V2 lifecycle
collector. Other platforms remain
`lifecycle_not_run` in inventory and `probe_not_implemented` if live execution
is requested. This does not mean the vendor lacks multiagent support.

## Explicit native checks

```sh
auto doctor agents --live --platform codex --timeout 120s --format json
```

Codex uses a dedicated app-server process and a temporary working directory by
default. It requests one successful child and one child to interrupt, binds
native events to the created supervisor, and verifies native child source,
result, interruption and cleanup. It never trusts the parent's claim that a
subagent ran. The collector accepts the installed V2 `subAgentActivity` surface
as well as supported legacy collaboration items. A transient unreadable child
is not granted ownership until its native parent binding can be checked.

Native archive, close and loaded-thread state are checked separately. Only
proven descendants are cleanup targets; unrelated loaded threads are not. The
cleanup protocol reconciles late/nested native spawns. Archived rollout history
is retained: this is runtime cleanup, not deletion of saved conversation data.

For OpenCode, start an authenticated **task-owned loopback server** in an empty
probe directory, then pass its endpoint and a password environment variable:

```sh
auto doctor agents --live --platform opencode \
  --endpoint http://127.0.0.1:4096 --username opencode \
  --password-env AUTOPUS_PROBE_SERVER_PASSWORD \
  --provider PROVIDER_ID --model MODEL_ID --dir /path/to/probe-directory \
  --timeout 90s --format json
```

The selected provider/model must actually be available on that server. The
collector checks its native health version and API schema, creates a root and
two linked child sessions, requires deny-all tool permissions, and checks an
actual result. Cancellation requires observed busy work, an abort acknowledgement,
and native aborted terminal state. Finally it deletes only its recorded sessions
and verifies their absence. Foreign children prevent cascading root deletion.
The caller owns server startup and shutdown; the collector does not stop an
existing user's server.

These collectors test different surfaces. Codex observes a model supervisor's
native delegation tools. OpenCode tests controller-created native linked-session
APIs with a passive root session. `execution_surface` makes that difference
explicit; it is not a head-to-head productivity experiment.

`--live` may use the configured provider's quota and requires one explicit
platform. Default inventory and supplied-trace analysis do not call models.
No global provider configuration, credentials or release pin is rewritten.

## Read the result without promoting unknowns

Live execution returns nonzero unless the collector finishes and every lifecycle
gate passes. JSON remains available on a failed run. The report distinguishes
`spawn`, `result`, `cancel`, and `cleanup`; unknown means the required native
observation was not obtained. An abort ACK or local process exit is insufficient.
Confirmed resource cleanup can coexist with failed work; overall PASS still
requires every gate. A transport cleanup fact is also recorded independently
when a partial chain cannot satisfy the stricter lifecycle gate.

`lifecycle_verified` applies to the observed protocol and execution surface.
It is not executable attestation, universal version certification, or billing
verification. Safe failure categories and native HTTP status codes are retained
without provider bodies, credential values or authorization headers.

```sh
auto doctor agents --trace-json docs/examples/agent-lifecycle/trace-unmeasured.json \
  --format json
```

The supplied-trace mode checks a bounded, strict normalized event schema. It
always leaves `lifecycle_verified` false and installation unknown, even if the
supplied event sequence evaluates to PASS. Exit zero means evaluation completed;
inspect the report, not just the exit status. The example is intentionally empty
and must produce UNKNOWN. A `source: native_protocol` label is not a signature.

## Attribute supervisor and worker usage

```sh
auto telemetry team \
  --evidence-json docs/examples/agent-lifecycle/team-unmeasured.json --format json
```

The input names an expected roster and each agent's observation:
`usage_scope` is `self_calls`, `inclusive_rollup`, or `unknown`;
`capture_complete` declares whether that agent's calls were fully collected;
`usage` contains existing normalized UsageEnvelope records.

- Only disjoint `self_calls` enter arithmetic. Parent-inclusive rollups are
  listed separately and never added to child usage or subtracted to guess a
  parent's own spend.
- Identical `(run_id, call_id)` copies under the same owner count once. Changed
  values or competing owner claims invalidate totals and exclude the conflicting
  call from known subtotals. Separate retries remain separate spend.
- Missing roster members, unknown scope or incomplete capture keep totals null.
  Only an explicit complete empty self-call observation means zero calls.
- `known_actual_tokens` and `known_actual_cost_usd` are partial observations.
  `known_estimated_cost_usd` is a separate catalog-derived estimate. Estimates
  never become actual billing. Token and USD completeness are separate.

OpenCode live reports include `team_usage_evidence` and `team_usage` when the
native session/message binding validates. For the verified 1.18.7 shape,
inclusive input is input + cache read + cache write, and inclusive output is
output + reasoning. Missing components and abort-default zeros stay unknown.
Its catalog-derived cost is an estimate. The bridge does not claim complete
worker capture from a partial message enumeration, even when a result succeeded.
A passive root is zero-call only when its native message inventory is empty.

Codex native call attribution is not yet collected by this driver and is
reported as such. Do not add parent and child cumulative counters without a
verified inclusion contract. Likewise, caller-declared rosters and capture flags
are not independent proof that every actual model call was supplied.

## Follow-up through Orca

[Latest CLI smoke checks](orca-cli-multiagent-check.md) updated Claude Code, OMP
and OpenCode after the observations below. Claude and OMP native delegation,
result and cancellation were observed through Orca terminals. OpenCode V2
authentication and adapter compatibility gaps remain explicit. These manual
checks do not add automated collectors or certify the full cleanup gate.

## Sources and runtime observations

The final local Codex 0.155.1 probe observed two distinct workers, a matching
child result, a native interrupted terminal and an empty owned-thread inventory
with native supervisor closure. All four lifecycle gates passed. Earlier failed
captures remain preserved; actual V2 events and a reproduced notification-flood
failure informed the adapter repairs.

OpenCode 1.18.7 proved linked-session creation and cleanup, but model calls did
not produce the required result/cancel chain. A diagnostic run observed native
HTTP **403** in `--pure` mode (external plugins disabled). All 12 sessions
created across those four bounded attempts were confirmed absent. A separate
normal-plugin server using existing configuration and `openai/gpt-5.4` produced
`UnknownError`; its three sessions were also confirmed absent. Both task-owned
servers were terminated. The configured `opencode/gpt-5.4` was absent from that
provider's model list; no global configuration was changed. These observations
do not establish an account-level cause or an OpenCode lifecycle PASS. Other
listed platforms received inventory only.

Sanitized local evidence is stored under
`.autopus/runtime/agent-lifecycle/2026-09-20/` (untracked runtime output).
`ledger.json` is a summary; original failed and final successful reports are
separate files. Codex archived history is retained. No global authentication,
provider configuration, or release artifact was changed.

- [Codex app-server](https://learn.chatgpt.com/docs/app-server), plus the local
  0.155.1 generated experimental JSON schema.
- [OpenCode server API](https://opencode.ai/docs/server/), its installed `/doc`,
  and [exact 1.18.7 usage normalization](https://github.com/anomalyco/opencode/blob/v1.18.7/packages/opencode/src/session/session.ts).
- [OpenCode 1.18.7 plugin loader](https://github.com/anomalyco/opencode/blob/v1.18.7/packages/opencode/src/plugin/index.ts):
  `--pure` disables external plugins, but does not itself disable built-in Codex
  OAuth. Record this execution condition separately from normal plugin loading.
- Other surfaces assessed for future collectors:
  [Claude subagents](https://code.claude.com/docs/en/agent-sdk/subagents),
  [OMP RPC](https://github.com/can1357/oh-my-pi/blob/main/docs/rpc.md),
  [Antigravity headless](https://www.antigravity.google/docs/cli/headless/),
  [Gemini headless](https://geminicli.com/docs/cli/headless/).

The execution ledger is in
[SPEC-AGENT-LIFECYCLE-001](../.autopus/specs/SPEC-AGENT-LIFECYCLE-001/acceptance.md).
It preserves partial and failed real probes separately from deterministic
fixture tests. Neither category proves productivity improvements or admits a
signed release cohort.

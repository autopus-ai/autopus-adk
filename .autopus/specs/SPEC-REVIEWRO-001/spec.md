# SPEC-REVIEWRO-001: Read-only SPEC review providers and provider readiness preflight

**Status**: approved
**Created**: 2026-10-06
**Domain**: REVIEWRO
**Module**: autopus-adk
**PRD**: `prd.md` (same directory; FR-01–FR-13, Q1–Q6)

## 목적

On the default CLI backend, `auto spec review` launches its reviewers and judge with write-capable argv. An executed
probe on `config.DefaultFullConfig` shows claude `--print --model claude-fable-5-1 --effort max` (no plan mode, no tool
limit), codex `exec --json --sandbox workspace-write ...`, and agy `--print ""`. On cmux/tmux the pane backend also
appends `--dangerously-skip-permissions` to claude and agy launches, because spec review never sets
`OrchestraConfig.ReadOnly`, and the pane poller auto-approves permission prompts. Provider breakage is invisible until
lanes fail mid-review: an OMP Anthropic account stayed disabled for 8 days and the claude lane and the judge failed with
"Model not found". This SPEC runs every reviewer and the judge through the existing `applyReadOnlyProviderPolicy` on the
subprocess or OMP backend only, closes the Claude tool and MCP gap in that policy, rejects widening config before any
configured binary runs, records the sandbox mode the executed argv proves, and adds a readiness preflight that makes no
model calls, in `auto spec review` and in `auto doctor`.

## Outcome Boundary

- Outcome Lock: every `auto spec review` on the default CLI backend runs each CLI reviewer and the judge as a subprocess
  under native read-only enforcement (claude: plan mode, safe mode, no MCP servers, read-only built-in tools; codex:
  read-only sandbox; agy: plan mode plus sandbox) and never in an interactive pane; OMP-backed providers keep the OMP
  review backend. It refuses any config that would widen this, before any configured binary runs, with an error that
  names the provider, the offending item, the config key, and a working remedy. It records the sandbox mode the
  executed argv proves for each reviewer and the judge in the promotion receipt. Before the first provider execution,
  and in `auto doctor`, a readiness preflight with no model calls reports logged-out or disabled-account providers with
  the remedy command, and never blocks on ambiguous evidence.
- Mandatory requirements: REQ-01–REQ-19 (Priority Must).
- Explicit non-goals: OMP backend removal or changes; review strategy, threshold, quorum rule, or judge semantics
  changes (only the REQ-11 degraded reason, pre-run exclusion, and denominator rules are added); read-only interactive
  pane review; Claude Code socket or remote-control integration; read-only `auto orchestra review`; plan/brainstorm pane
  hardening; a runtime worktree-mutation guard; paid model calls in the preflight; automatic login, token refresh, or
  cache invalidation; per-provider policy knobs; widening the strict argv allowlist.
- Completion evidence: acceptance S1–S17 pass; the argv oracle reads the real subprocess backend's executed command;
  the receipt fixture shows `read-only` only for argv that proves it and `unrestricted` when a bypass flag reaches the
  executed argv; the incident fixture (OMP Anthropic account expired) aborts before the first provider execution with
  0 model calls; plan, brainstorm, and SPEC-ORCH-021 argv oracles change only by the trailing claude items; RFP-1
  (claude) and RFP-2 (codex) PASS and RFP-3 (agy) PASS (completion-blocking); a redacted real OMP fixture replaces the
  assumed element shape (CD-5); coverage of new files is at least 85%; every source file stays at or under 300 lines.

## Requirements

Priority uses Must only. Type is the EARS type that `pkg/spec/parser.go` `detectEARSType` assigns (WHEN…THEN event-driven, WHERE…THEN state-driven, IF…THEN unwanted, WHEN…IF…THEN optional). PRD is the source FR. The Error, Execution Boundary, and Readiness Contract sections are part
of the requirements that cite them.

| ID | Priority | Type | PRD | EARS requirement |
|----|----------|------|-----|------------------|
| REQ-01 | Must | EventDriven | FR-01 | WHEN `auto spec review` assembles reviewer providers, THEN THE SYSTEM SHALL project every reviewer through `applyReadOnlyProviderPolicy` as the final argv-mutating step after `resolveCodexProviderCapabilities`, `configureSpecReviewProviders`, and `applySpecReviewExecutionTimeout`, for both `Args` and `PaneArgs`, and THE SYSTEM SHALL keep the existing accept-as-read-only path for OMP-backed providers. |
| REQ-02 | Must | EventDriven | FR-02 | WHEN the judge is resolved, either reused from a projected reviewer or resolved separately through `resolveSpecReviewJudgeConfig`, THEN THE SYSTEM SHALL apply the same checks and projection, and projecting an already projected config SHALL return identical `Args`, `PaneArgs`, and `SandboxMode` with no error. |
| REQ-03 | Must | StateDriven | FR-03 | WHERE a claude provider is projected read-only (spec review, plan, brainstorm), THEN THE SYSTEM SHALL add `--strict-mcp-config` followed by the single token `--tools=Read,Grep,Glob` as the last projected item in addition to `--permission-mode plan` and `--safe-mode`, SHALL accept `--tools` in config only with the value `Read,Grep,Glob` in either the inline or the separated form, SHALL keep rejecting `--mcp-config`, and the emitted tokens SHALL NOT consume a following positional prompt. |
| REQ-04 | Must | Unwanted | FR-04 | IF an explicitly selected claude, codex, or gemini provider carries a binary, argv, or `subprocess.schema_flag` that the policy rejects, THEN THE SYSTEM SHALL abort before any provider execution with the Error Contract message, SHALL NOT drop the item, run the provider writable, or let `--allow-degraded` bypass the check, and SHALL replace rather than reject values that the projection narrows toward read-only. |
| REQ-05 | Must | EventDriven | research | WHEN a codex argv carries the short sandbox alias `-s <mode>` or `-s=<mode>` with a non-dangerous mode, THEN THE SYSTEM SHALL replace it so that the projected argv holds exactly one sandbox item, `--sandbox read-only`. |
| REQ-06 | Must | Unwanted | FR-05 | IF a CLI-backend provider other than claude, codex, or gemini is selected explicitly (`--providers`, `spec.review_gate.providers`, or `spec.review_gate.judge`), THEN THE SYSTEM SHALL fail closed with the Error Contract message even when its binary is not installed; WHEN a provider that the policy rejects enters only through `--multi` discovery (`orchestra.commands.review.providers`, `orchestra.providers` keys, or built-in defaults), THE SYSTEM SHALL exclude it before execution, print a warning, record the exclusion in the receipt, and remove it from the quorum denominator. |
| REQ-07 | Must | EventDriven | FR-06 | WHEN a review run has started provider execution, THEN THE SYSTEM SHALL record one `provider_policy` receipt row per reviewer, per excluded provider, and for the judge with `provider`, `role`, `sandbox_mode`, `readiness`, and `excluded`, taking `sandbox_mode` from `orchestra.ProviderSandboxMode` over the executed argv, in which a permission-bypass flag yields `unrestricted` regardless of the policy stamp, and leaving it empty for a provider that never executed. |
| REQ-08 | Must | EventDriven | FR-07 | WHEN `auto doctor --provider-smoke` exercises review-gate providers, THEN THE SYSTEM SHALL build them through the same assembly helper and projection as `auto spec review` and execute each through the same routed backend, so that OMP-backed providers run only through the OMP review backend. |
| REQ-09 | Must | EventDriven | FR-08 | WHEN reviewers and the judge are projected, THEN THE SYSTEM SHALL run the Readiness Contract probes before any provider execution, using status commands only and no model inference, and WHERE `--skip-provider-readiness` is set THE SYSTEM SHALL run no probe and record readiness `skipped`. |
| REQ-10 | Must | Ubiquitous | FR-09 | THE SYSTEM SHALL classify each probe result as `ready`, `not_ready(<state>)`, `unknown(<reason>)`, or `skipped` exactly as the Readiness Contract defines, run probes concurrently with a 5 s timeout each, and never block a review on `unknown` or `skipped`. |
| REQ-11 | Must | Optional | FR-10 | WHEN a reviewer is `not_ready`, THEN THE SYSTEM SHALL exclude it before execution, print the provider, state, and remedy, keep it in the quorum denominator with Provider Health status `error` and note `excluded: not_ready(<state>)`, and add the degraded reason `provider_unready:<provider>:<state>` after the per-revision observation reasons; IF no reviewer remains, THEN THE SYSTEM SHALL fail and print every remedy. |
| REQ-12 | Must | Unwanted | FR-11 | IF the judge is `not_ready`, THEN THE SYSTEM SHALL fail before any provider execution and print the judge, state, and remedy. |
| REQ-13 | Must | EventDriven | FR-12 | WHEN an OMP-backed provider is probed, THEN THE SYSTEM SHALL run `usage --json --redact` once per review with the canonical OMP executable and the `normalizePipelineOMPEnvironment` environment that the OMP review backend uses, without `--profile`, and classify the family before the first `/` of each model exactly as the Readiness Contract OMP rules define. |
| REQ-14 | Must | EventDriven | FR-13 | WHEN `auto doctor` runs in text or `--json` mode, THEN THE SYSTEM SHALL report check `doctor.provider_readiness.<provider>` for each distinct review-gate provider and the judge (ready → pass, not_ready → fail with remedy, unknown or skipped → skip) by default and with no model calls, using the JSON severity and report-status mapping of `doctor.provider_transport.*` plus one `provider_unready` warning per failing provider, while `--provider-smoke` stays the only path that makes model calls. |
| REQ-15 | Must | Ubiquitous | security | THE SYSTEM SHALL treat probe output as untrusted: execute only the three Readiness Contract argv without a shell, read at most 65,537 bytes per stream in code outside the runner seam, classify a stream longer than 65,536 bytes as `unknown(oversized_output)`, parse only the named fields, never print raw output, redact emails, account ids, and tokens in stderr, receipts, and doctor output, and never invoke a credential-mutating command (`auth login`, `auth logout`, `login` without `status`, `logout`, `usage invalidate`, `token`). |
| REQ-16 | Must | EventDriven | research | WHEN `auto spec review` runs, THEN THE SYSTEM SHALL execute every CLI-backend reviewer and the judge on the subprocess backend regardless of terminal pane capability, `orchestra.subprocess.enabled`, `--subprocess`, or `--plain`, print `spec review: read-only review runs providers in subprocess mode` once when the terminal is pane-capable, and launch no interactive pane, while OMP-backed providers keep routing to the OMP review backend. |
| REQ-17 | Must | Ubiquitous | research | THE SYSTEM SHALL set `OrchestraConfig.ReadOnly` for every spec review run, so that any pane launch command built from that config carries no permission-bypass flag. |
| REQ-18 | Must | EventDriven | research | WHEN providers are resolved, THEN THE SYSTEM SHALL apply the policy's provenance, provider-name, native-binary, argv, and schema-flag checks before `filterInstalledProviders` and before any configured binary executes, including the codex catalog probe and the readiness probes. |
| REQ-19 | Must | EventDriven | research | WHEN a reviewer or the judge executes on the subprocess backend, THEN THE SYSTEM SHALL launch exactly the projected `Args` followed only by the runtime items that the Execution Boundary Contract lists. |

## Error Contract

Rendered by the spec-review assembly helper from a typed policy violation. The shared policy keeps its current
`Error()` text, so plan and brainstorm messages stay byte-identical.

```text
spec review: provider "<name>" rejected by the read-only policy: <reason> (config key: <key>; remedy: <remedy>)
```

- `<reason>` renders the typed violation's reason as one of: `contains unsupported argv "<item>"`, `contains unsafe argv
  "<item>"`, `contains unsafe value for "<flag>"`, `has incomplete argv "<item>"`, `requires native binary "<binary>"`,
  `unsupported schema flag "<flag>"`, or `unsupported provider "<name>"`.
- `<key>` is `orchestra.providers.<name>.args`, `.pane_args`, `.binary`, or `.subprocess.schema_flag`; for an
  unsupported provider it is the selection source (`--providers`, `spec.review_gate.providers`, `spec.review_gate.judge`).
- `<remedy>` is `remove "<item>" from <key>` for an argv item, `remove "<flag> <value>" from <key>` (or
  `"<flag>=<value>"` for the inline form, value cut at 64 runes) for an unsafe value, `set <key> to "<binary>"`, `remove
  <key>` for a schema flag, or `remove "<name>" from <key>`.

## Execution Boundary Contract

| Provider | Executed argv after the binary |
|----------|-------------------------------|
| claude | projected `Args`; the prompt arrives on stdin, or as one final positional item when `prompt_via_args` is true |
| codex | projected `Args`, then only `--output-schema <path>` (when `subprocess.schema_flag` is `--output-schema`) and `--output-last-message <path>` |
| gemini | projected `Args` with the `""` slot replaced by the prompt |

Accepted `subprocess.schema_flag` values: codex `--output-schema` or empty; claude and gemini empty. No other item.

## Readiness Contract

| Provider | Probe argv | Classification |
|----------|-----------|----------------|
| claude | `claude auth status --json` | `loggedIn` true → ready; `loggedIn` false → not_ready(logged_out), remedy `claude auth login`, except unknown(env_credentials) WHERE `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, `CLAUDE_CODE_USE_BEDROCK`, or `CLAUDE_CODE_USE_VERTEX` is non-empty; other output → unknown(unparsable) |
| codex | `codex login status` | exit 0 → ready; exit 1 with `Not logged in` on stdout or stderr → not_ready(logged_out), remedy `codex login`, except unknown(env_credentials) WHERE `CODEX_API_KEY` or `OPENAI_API_KEY` is non-empty; other → unknown(probe_failed) |
| OMP-backed | `<canonical omp> usage --json --redact`, once per review | OMP rules below |
| gemini (agy) | none | unknown(no_status_command), probe not executed |

OMP rules: exit non-zero → unknown(probe_failed); a top level that is not an object with arrays `reports`,
`accountsWithoutUsage`, and `disabledCredentials` → unknown(unparsable). An element counts for family F only when its
string field `provider` equals F. usable = matching `reports` + matching `accountsWithoutUsage`; disabled = matching
`disabledCredentials`. usable ≥ 1 → ready, plus the warning `omp <F>: <d> of <u+d> accounts unusable (<state>); run "omp
usage --redact" for details` when disabled ≥ 1. usable 0 and disabled ≥ 1 → not_ready(auth_expired) when every
matching `reason` contains `expired` (case-insensitive), else not_ready(account_disabled). usable 0 and disabled 0 →
unknown(no_account_evidence). Remedy `omp login <F>`, plus ` (same PI_CODING_AGENT_DIR as this review)` when that
variable is set.

Every probe: timeout → unknown(timeout); start failure → unknown(probe_failed); a stream over 65,536 bytes →
unknown(oversized_output); environment presence checks never read values into output. Output lines use
`preflight: <provider> <status>` on stderr.

## 생성 파일 상세

All source files stay at or under 300 lines; tests sit next to each file.

| Path | Role |
|------|------|
| `internal/cli/orchestra_readonly_policy.go` | REQ-03 claude items, REQ-05 `-s` normalization, schema-flag check |
| [NEW] `internal/cli/orchestra_readonly_violation.go` | typed violation (provider, field, item, value, reason) with the current `Error()` text |
| [NEW] `internal/cli/spec_review_readonly.go` | assembly helper: pre-execution gate, provenance, filter, capabilities, configure, timeout, projection, Error Contract |
| [NEW] `internal/cli/provider_readiness.go`, `provider_readiness_omp.go`, `provider_readiness_stream.go` | allowlisted runner seam, bounded stream reader outside the seam, Readiness Contract classification, redaction |
| [NEW] `internal/cli/spec_review_readiness.go` | preflight gating, exclusion, denominator rules, degraded reason merge, receipt rows |
| [NEW] `internal/cli/doctor_provider_readiness.go` | doctor text and JSON checks |
| [NEW] `pkg/orchestra/provider_sandbox_mode.go` | exported `ProviderSandboxMode(ProviderConfig, []string) string` |
| `pkg/orchestra/provider_execution.go` | `providerSandboxMode`: a bypass flag in the argv beats the stamp |
| `internal/cli/spec_review.go`, `spec_review_loop.go`, `spec_review_runtime.go`, `spec_review_receipt.go` | helper call, judge projection, `ReadOnly`, subprocess mode, loop fields, merge call, receipt rows, `--skip-provider-readiness`, flag help |
| `internal/cli/doctor_provider_smoke.go`, `doctor_text.go`, `doctor_json.go`, `doctor_remediation.go` | shared helper and routed backend, one wiring line each, remedy text |
| [NEW] `internal/cli/testdata/provider_readiness/` | redacted status fixtures |
| `CHANGELOG.md` | behavior changes and new flag |

## Related SPECs

None as sibling (see `research.md` Sibling SPEC Decision). SPEC-ORCH-021 (completed): its argv oracles stay green except
for the trailing claude items. SPEC-ORCH-022: hook-IPC pane completion is untouched; spec review no longer uses the pane
path. SPEC-OMP-006: OMP read-only allowlist unchanged. SPEC-SIGMABAND-001 (draft): REQ-11 consumes
`applyReadOnlyProviderPolicy` and inherits REQ-03 by design; both SPECs edit `orchestra_readonly_policy.go`, so T1 of
this SPEC merges before SIGMABAND T10 or the two are serialized.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-01 | T2, T4 | S1 | INV-01 |
| REQ-02 | T4 | S3 | INV-02 |
| REQ-03 | T1 | S4, S5, S17 | INV-03, INV-11 |
| REQ-04 | T1, T2 | S6 | INV-04, INV-13 |
| REQ-05 | T1 | S7 | INV-04 |
| REQ-06 | T2 | S8 | INV-05, INV-13 |
| REQ-07 | T4, T5 | S9 | INV-06 |
| REQ-08 | T6 | S14 | INV-09, INV-12 |
| REQ-09 | T3, T4 | S10, S13 | INV-07, INV-08 |
| REQ-10 | T3 | S10 | INV-07 |
| REQ-11 | T4 | S12 | INV-08 |
| REQ-12 | T4 | S13 | INV-08 |
| REQ-13 | T3, T8 | S11 | INV-07 |
| REQ-14 | T7 | S15 | INV-09 |
| REQ-15 | T3, T7 | S16 | INV-10 |
| REQ-16 | T4 | S2 | INV-12 |
| REQ-17 | T4 | S2 | INV-12 |
| REQ-18 | T2 | S6, S8 | INV-13 |
| REQ-19 | T2, T4 | S1 | INV-01 |

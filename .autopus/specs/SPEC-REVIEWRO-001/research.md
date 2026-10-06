# SPEC-REVIEWRO-001 리서치

## 기존 코드 분석

- Assembly: `internal/cli/spec_review.go:162-172` runs `specReviewConfigProviders` (= `buildReviewProvidersWithConfig`, which calls `filterInstalledProviders`) → `resolveCodexProviderCapabilities` → `configureSpecReviewProviders` → `applySpecReviewExecutionTimeout`, with no projection; `:269-291` resolves the judge unprojected (line 286). `resolveCodexProviderCapabilities` runs `<binary> debug models` for quality codex (`pkg/codexruntime/probe.go:57`), so today a configured binary runs before any policy.
- Policy: `orchestra_readonly_policy.go:39-74` stamps `SandboxMode`; `:76-168` strict validator (no `--tools`; codex accepts `-s`); `:170-193` projections; `orchestra_readonly_argv.go:7` `upsertArgValue` matches the exact flag only. Callers: `orchestra.go:102,127`, `orchestra_brainstorm_isolation.go:92`. `subprocess_runner.go:111-127` appends `SchemaFlag` + schema path after the policy.
- Pane path: `spec_review_runtime.go:13-17` routes through `selectRoutedBackend` (`omp_review_backend.go:180-187`) → `orchestra.SelectBackend` (`backend.go:36-41`), which picks the pane backend when `paneCapable` (`pane_capable.go:12`). `pane_backend.go:140` calls `buildPaneLaunchCommand(paneLaunchFor(b.cfg), ...)`; `interactive_launch.go:32-34,63-66` appends `--dangerously-skip-permissions` for claude and agy unless `cfg.ReadOnly`; only `orchestra.go:219` sets `ReadOnly`, not `spec_review_loop.go:72-90`. `completion_poll.go:78-84` answers `interactive_detect.go:147-152` prompts with `1`; `reviewer_response_file.go:13-18` and `completion_poll.go:86` make reviewer completion wait for the response file that `prompt_file.go:84-94` asks for.
- OMP: `omp_review_backend_session.go:42-45` adds `--no-skills --no-lsp --config <overlay> --tools ... --approval-mode yolo`; `omp_review_backend_process.go:61` inherits `normalizePipelineOMPEnvironment(os.Environ())` (`pipeline_backend_omp_identity.go:112-130`), no `--profile`; `omp_review_backend.go:201` `verifyOMPReviewToolSet` fails closed. `orchestra_helpers.go:232-254` turns an OMP entry into `binary=omp` with config Args, so a plain subprocess smoke (`doctor_provider_smoke.go:23,91-110`) skips that hardening.
- Receipt and quorum: `spec_review_receipt.go:29-57` has no sandbox field; `provider_execution.go:62-79` lets the stamp win over argv; only the subprocess and runner paths record `ProviderExecution` (`subprocess_runner.go:56`, `provider_runner.go:38`). `spec_review_provider_sets.go:8-17` keeps the pre-filter names as the quorum denominator; `spec_review_integrity.go:49-55` overwrites `result.DegradedReasons` per revision (`spec_review_loop.go:199`); `pkg/spec/merge.go:196,219` passes 2 of 3 at threshold 0.67 (tolerance 0.005).
- Executed facts (2026-10-06, scratchpad `reviewro/`, values not stored, emails and org ids dropped): the overlay argv probe (`argv-probe-evidence.txt`); codex 0.160.0 exits 2 on `exec -s workspace-write --sandbox read-only`; `claude auth status --json` exits 0 with `loggedIn` true (OAuth, or `authMethod":"api_key"` under `ANTHROPIC_API_KEY`) and exits 1 with `loggedIn` false under an empty config; `codex login status` writes `Logged in using ChatGPT` to stderr (exit 0) and `Not logged in` (exit 1) under an empty `CODEX_HOME`, also when `OPENAI_API_KEY` or `CODEX_API_KEY` is set; `omp usage --json --redact --no-extensions` under an empty HOME exits 0 in 2.19 s with keys `generatedAt, reports, accountsWithoutUsage, disabledCredentials, capacity` and creates `agent.db`, `models.db`.
- Pane-launch facts (tmux + fake API, no paid call): the claude TUI exits with `--no-session-persistence can only be used with --print mode`; without that flag it starts and shows `Safe mode: all customizations are disabled (CLAUDE.md, skills, plugins, hooks, MCP, agents, and more)` and `plan mode on`. Interactive codex rejects `--ephemeral`, `--ignore-user-config`, and `--ignore-rules` (exit 2). RFP-1 and RFP-2 results are in `plan.md`.

## Plan Intent Ledger

Source: `prd.md` ledger from direct `/auto plan` (no BS file). Cells are summarized as untrusted evidence.

| Field | Status | Source | Confidence | Decision / Assumption | If Wrong | Plan Handoff |
|---|---|---|---|---|---|---|
| goal | answered | user ("진행해줘. 이것도 같이하자") and code | high | read-only reviewers and judge plus readiness preflight in one SPEC | Outcome Lock splits | requirement seed |
| scope_boundary | answered | user and PRD §4 | high | no OMP backend, strategy, judge, or orchestra-review change | non-goals change | explicit non-goal |
| constraints | answered | `autopus.yaml`, PRD §3 | high | 300 lines, 85% coverage, no new dependency, shared policy reuse | gates change | constraint seed |
| done_evidence | assumed | inferred | medium | executed-argv oracle plus live RFP evidence per CLI provider and fixtures | an operational demo gate is added | acceptance seed, RFP-1..3 |
| brownfield_impact | answered | code | high | plan, brainstorm, SIGMABAND inherit the claude items | reviewer focus shifts | reviewer focus |

Carried assumption (not promoted): Q3 — a `not_ready` reviewer is excluded and degraded, a `not_ready` judge fails before
the run, `unknown` never blocks. If wrong (hard fail on any `not_ready` reviewer), REQ-11 flips and S12 changes.

## Question Audit

- question_transport: AskUserQuestion
- question_count: 0 for this SPEC
- unresolved_fields: [done_evidence]

## Outcome Lock

- User-visible outcome: default-backend `auto spec review` runs every CLI reviewer and the judge as a read-only subprocess (OMP providers on the OMP review backend), refuses widening config before any configured binary runs, records the sandbox mode the executed argv proves, and reports logged-out or disabled-account providers with the remedy before the first execution and in `auto doctor`, with 0 model calls and no block on ambiguous evidence.
- Mandatory requirements: REQ-01–REQ-19.
- Explicit non-goals: see `spec.md` Outcome Boundary (incl. read-only pane review and plan/brainstorm pane hardening).
- Completion evidence: S1–S17 pass; RFP-1, RFP-2, RFP-3 PASS; CD-5 fixture captured; coverage ≥ 85%; files ≤ 300 lines.

## Visual Planning Brief

`wireframe intent: not applicable` (CLI only). Command flow (mermaid in `plan.md`): config → gate → resolve → read-only
projection → readiness preflight → subprocess/OMP run → receipt `sandbox_mode`. One probe:

```text
runner(allowlisted argv, no shell, 5 s) -> raw streams -> bounded read (65,537 B) -> classify (pure, env presence)
  -> stderr "preflight: <p> <status>" -> exclusion / judge fail -> receipt provider_policy row -> doctor check
```

## Technology Stack Decision

| Mode | Selected stack | Resolved versions | Source refs | Checked at |
|---|---|---|---|---|
| brownfield | existing Go module and stdlib (`os/exec`, `io`, `encoding/json`); external CLIs as runtime contracts | `go.mod` unchanged; Claude Code 2.1.289, codex-cli 0.160.0, agy 1.2.17, omp 18.6.1 | local `--version`/`--help`, executed probes | 2026-10-06 |

## 설계 결정

- Direct policy call from one helper, not `readOnlyOrchestraCommand("review")` (PRD Q1).
- Subprocess-only read-only review (REQ-16) over pane hardening: six verified pane blockers (`plan.md` Implementation Strategy) would need changes in five `pkg/orchestra` files and a new completion protocol; the subprocess path is already covered end to end by RFP-1/RFP-2. Cost: no live panes during spec review on cmux/tmux. `ReadOnly` is still set (REQ-17).
- Claude: `--tools=Read,Grep,Glob` inline (RFP-1: the separated form swallows a prompt) plus `--strict-mcp-config` (RFP-1: without `--safe-mode`, user and project MCP write tools are offered; with safe mode or strict mode, none). `--restricted` and deny lists rejected (PRD Q2).
- Gate before execution: the check order alone decides whether a wrapper binary ever runs; the projection stays last.
- Executed-argv receipt: a bypass flag in the executed argv beats the stamp, so `read-only` only appears where the argv proves it.
- agy: completion-blocking live probe (RFP-3) instead of fail-closed exclusion. Excluding gemini would degrade every default review (PASS blocked without `--allow-degraded`), while the projected agy argv is strictly narrower than today's.
- Readiness: not_ready only on unambiguous evidence; environment credentials and missing OMP account evidence give `unknown`; `--skip-provider-readiness` is the operator escape. OMP probes all families once in the review environment because the `--provider` ids are unverified.
- Not prompt-state work: the review prompt and its layers do not change.

## Minimality Decision Matrix

| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | executed probe shows writable argv; pane launch adds bypass flags; no readiness probe exists (`rg`) | proceed | gate, projection, subprocess-only, preflight, doctor check |
| existing code/helper/pattern | `applyReadOnlyProviderPolicy`, `upsertArgValue`, `ensureBoolArg`, `selectRoutedBackend`, `OrchestraConfig.ReadOnly`, `SubprocessMode`, `providerSandboxMode`, `newCommand` seam, `jsonCheck`, `evaluateIntegrityGate`, `normalizePipelineOMPEnvironment` | reuse | one policy, existing backends and seams |
| stdlib/native | `os/exec` with context timeout, `io.LimitReader`, `encoding/json`, native CLI status commands | use | runner, bounded reader, parsing |
| existing dependency | `cobra`, `testify`, `pkg/orchestra` | reuse | no new module |
| new dependency or abstraction / new dependency or new abstraction | typed violation (field and value for the key and remedy), exported `ProviderSandboxMode`, readiness runner seam; pane hardening rejected as larger | accepted | three small additions, no dependency |
| minimum sufficient verification | S1–S17 on real backends with recorded seams, RFP-1..3, race, coverage, file size, lint, hygiene, strict validate | required checks | `plan.md` Verification |

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-01 | "projection SHALL be the final argv-mutating step"; closed runtime items | ordering / closed set | executed argv per provider | S1 |
| INV-02 | "projection SHALL be idempotent" | idempotency | judge argv, SandboxMode | S3 |
| INV-03 | "restrict built-in tools"; MCP closed; variadic must not swallow the prompt | parser / argv ordering | claude argv, prompt delivery | S4, S5 |
| INV-04 | "rejected ... replaced, not rejected" | classification (reject / replace / keep) | error text, projected argv | S6, S7 |
| INV-05 | "explicitly selected → fail closed; --multi discovery → exclude and record" | provenance grouping | error, exclusion row, denominator | S8 |
| INV-06 | "sandbox_mode SHALL come from the executed provider config" | report rows | receipt `provider_policy` | S9 |
| INV-07 | "ready, not_ready(<state>), or unknown" from status probes | parser / classification | preflight status, remedy | S10, S11 |
| INV-08 | "exclude ... degraded reason ... judge fails before execution ... unknown never blocks" | state transition / reason order | executions, Provider Health, DegradedReasons | S12, S13 |
| INV-09 | "same assembly helper" and doctor status mapping | parity / report rows | smoke argv, doctor checks | S14, S15 |
| INV-10 | "CLI status outputs are untrusted; redact emails/tokens" | transformation / bounding | stderr, receipt, doctor output | S16 |
| INV-11 | "existing oracles change only for the FR-03 flag" | regression parity | plan, brainstorm, ORCH-021 argv | S17 |
| INV-12 | "launches each reviewer and the judge only under native read-only enforcement" on every backend | routing | selected backend, pane launches | S2, S14 |
| INV-13 | "abort before any provider execution" | ordering of checks vs process starts | catalog probe, readiness, backend calls | S6, S8 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| Happy path: read-only reviewers and judge on subprocess/OMP | REQ-01–REQ-03, REQ-16, REQ-19 / T1, T2, T4 / S1–S5 | covered |
| Pane path | REQ-16, REQ-17 / T4 / S2 (never launched; bypass flag absent if built) | covered |
| Error and recovery: conflicting config, unsupported or discovered providers | REQ-04–REQ-06, REQ-18 / T1, T2 / S6–S8 | covered |
| Integration boundary per CLI: claude tools and MCP, codex sandbox, agy plan + sandbox, OMP backend | REQ-03, REQ-05, REQ-08, REQ-13 / T1, T3, T6 / S1, S7, S11, S14, RFP-1..3 | covered (RFP-3 pending, CD-4) |
| Readiness: probes, gating, judge, operator escape | REQ-09–REQ-13 / T3, T4 / S10–S13 | covered |
| Receipt evidence | REQ-07 / T4, T5 / S9 | covered |
| CLI surface: spec review stderr and flags, doctor text and JSON | REQ-09, REQ-11, REQ-14, REQ-16 / T4, T7 / S2, S10, S12, S15 | covered |
| Security: untrusted output, allowlisted probes, no shell, no credential commands | REQ-15 / T3, T7 / S16 | covered |
| Verification, regression parity, docs | all / T9, T10 / S14, S17, `plan.md` Verification | covered |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| CD-1 claude tool and MCP restriction | Outcome Lock under permissive allow rules or MCP servers | T1; RFP-1 PASS for the planned argv; T10 re-run |
| CD-2 judge separate resolution path | "reviewers and judge never write" | T4, S3 |
| CD-3 doctor smoke parity incl. OMP routing | smoke would validate argv or hardening the review never runs | T6, S14 |
| CD-4 live read-only evidence per CLI provider | claiming read-only without execution | RFP-1 (claude) and RFP-2 (codex) PASS; RFP-3 (agy) must PASS before sync; FAIL returns the SPEC to planning |
| CD-5 OMP element fields `provider` and `reason` | incident detection (S11, S13) on real output | T8 captures a redacted real fixture |

## Evolution Ideas

These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| read-only interactive pane review (print-only flags in the TUI, hooks under safe mode, response-file protocol, no auto-approval) | REQ-16 closes the Outcome Lock on the subprocess path | users ask for live panes during spec review |
| plan/brainstorm pane hardening: same TUI flag rejections and prompt auto-approval exist there today | outside this Outcome Lock (spec review) | a plan or brainstorm pane run fails or is audited |
| read-only `auto orchestra review` (`readOnlyOrchestraCommand` predicate) | separate user-visible surface | user request |
| per-round worktree-mutation guard reusing `workspaceGuard` | needs a false-positive design (the loop reloads edited SPECs) | a mutation slips past native enforcement |
| Claude Code stream-json or remote-control backend; codex app-server backend | CLI status probes close the Outcome Lock | a backend migration is planned |
| allowlist widening for proven non-widening claude flags; codex `--ignore-user-config` knob | strict list and remedies suffice | per-flag evidence and user demand |
| agy readiness probe; readiness cache across reviews | no agy status command; 5 s bound is acceptable | agy ships auth status; latency complaints |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | Primary SPEC closes the Outcome Lock; 10 tasks and about 24 files with tests are below both thresholds; the preflight consumes the projected set and writes the same receipt | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `internal/cli/spec_review.go:162-172,269-291`; `spec_review_loop.go:72-95,199`; `spec_review_runtime.go:13-17,59`; `spec_review_provider_sets.go:8-17`; `spec_review_integrity.go:49-55,70` | existing | Read 2026-10-06 |
| `internal/cli/orchestra_readonly_policy.go:21-206`; `orchestra_readonly_argv.go:7,43`; `orchestra.go:102,127,219`; `orchestra_brainstorm_isolation.go:92`; `orchestra_helpers.go:232-254` | existing | Read and rg |
| `pkg/orchestra/backend.go:36-41`; `pane_capable.go:12`; `pane_backend.go:131-140,169-173`; `interactive_launch.go:32-34,45-79,128`; `completion_poll.go:78-90`; `interactive_detect.go:147-152`; `reviewer_response_file.go:13-18`; `prompt_file.go:84-94` | existing | Read |
| `pkg/orchestra/provider_execution.go:11-13,62-79`; `subprocess_runner.go:56,111-137`; `command.go:30`; `pkg/codexruntime/probe.go:57`; `pkg/spec/merge.go:196,217-231` | existing | Read and rg |
| `internal/cli/omp_review_backend.go:28,180-201`; `omp_review_backend_session.go:42-45`; `omp_review_backend_process.go:61`; `pipeline_backend_omp_identity.go:112-130` | existing | Read |
| `internal/cli/doctor_provider_smoke.go:23,91-110`; `doctor_text.go:154`; `doctor_json.go:131`; `doctor_remediation.go` | existing | Read and rg |
| external CLI flags `--tools`, `--strict-mcp-config`, `--mcp-config`, `--safe-mode`, `auth status --json`, `login status`, `usage --json --redact`, agy `--mode`, `--sandbox` | existing runtime contract | local `--help` and executed probes |
| `orchestra_readonly_violation.go`, `spec_review_readonly.go`, `provider_readiness*.go`, `spec_review_readiness.go`, `doctor_provider_readiness.go`, `pkg/orchestra/provider_sandbox_mode.go`, `testdata/provider_readiness/` | [NEW] planned addition | n/a |
| `ProviderSandboxMode`, `providerReadinessRunner`, `--skip-provider-readiness`, receipt field `provider_policy`, check `doctor.provider_readiness.<provider>` | [NEW] planned addition | n/a |

## Reviewer Brief

- Intended scope: the Outcome Lock via REQ-01–REQ-19; one Primary SPEC, no sibling.
- Explicit non-goals: the Outcome Lock non-goals, especially read-only pane review and plan/brainstorm pane hardening.
- Self-verified: the pane-path, OMP-backend, catalog-probe, schema-flag, and quorum facts at the cited lines; RFP-1 (built-in, MCP, prompt delivery) and RFP-2 PASS; TUI and interactive-codex flag rejections; status shapes incl. API-key environments; strict validate.
- Reviewer should focus on: the subprocess-only decision (REQ-16), gate ordering (REQ-18), the closed execution set (REQ-19), bypass-beats-stamp receipts (REQ-07), OMP conservative rules and the assumed element fields (CD-5), the agy completion gate (CD-4), PRD Q3 (assumed), and Completion Debt only.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 2 | files: research.md, spec.md, plan.md | reason: pane, OMP, catalog, schema-flag, quorum refs re-read
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: new files, symbols, flag, and fields carry [NEW]
- Q-CORR-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: real `ParseEARSWithWarnings` (scratchpad `earscheck-rro`): 19 rows, 0 warnings, 0 Type mismatches
- Q-CORR-04 | status: PASS | attempt: 2 | files: research.md, spec.md, plan.md | reason: Reference Discipline separates existing and [NEW]
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: contract, plan and probes, oracles, evidence are split
- Q-COMP-02 | status: PASS | attempt: 2 | files: spec.md, plan.md, acceptance.md | reason: REQ-01–REQ-19 each map to a task and a Must scenario
- Q-COMP-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: OMP, env-credential, oversize, and skip outcomes are explicit
- Q-COMP-04 | status: PASS | attempt: 2 | files: research.md, spec.md | reason: pane, judge, doctor, and agy slices close or block sync via CD-4
- Q-COMP-05 | status: PASS | attempt: 2 | files: research.md, spec.md, plan.md, acceptance.md | reason: INV-01–INV-13 map to Must oracles on real backends
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix and Reviewer Brief bound review scope
- Q-COMP-07 | status: PASS | attempt: 2 | files: research.md | reason: agy evidence is Completion Debt; Evolution Ideas carry no IDs
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: RFP-1/2 PASS with evidence and controls; RFP-3 not-run with reason
- Q-FEAS-01 | status: PASS | attempt: 2 | files: plan.md, spec.md | reason: every touched file has one owning task; OMP routing owned by T6
- Q-FEAS-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: source-of-truth paths only; no generated surface
- Q-FEAS-03 | status: PASS | attempt: 2 | files: plan.md | reason: hygiene line exits 0 on a clean tree; probes need no paid call
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: no ambiguous wording in REQ text
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Priority Must only, separate from EARS type
- Q-STYLE-03 | status: PASS | attempt: 1 | files: acceptance.md | reason: complete sentences, parseable steps
- Q-SEC-01 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: pane bypass, MCP, injection, and untrusted output closed
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: no configured binary runs before the gate; redaction; fake keys
- Q-SEC-03 | status: PASS | attempt: 1 | files: spec.md | reason: receipt rows hold status tokens; probe artifacts stay in scratchpad
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md | reason: one provider-execution boundary story
- Q-COH-02 | status: PASS | attempt: 2 | files: research.md | reason: agy evidence and the OMP fixture block sync instead of being deferred
- Q-COH-03 | status: PASS | attempt: 1 | files: research.md | reason: no sibling; shared-file order with SIGMABAND stated

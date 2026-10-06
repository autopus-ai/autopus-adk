# SPEC-REVIEWRO-001 수락 기준

## Test Scenarios

Fixture F-default (verified default assembly, catalog probe stubbed): claude `binary=claude`, Args = PaneArgs =
`--print --model claude-fable-5-1 --effort max`, prompt via stdin; codex `binary=codex`, Args = `exec --json --sandbox
workspace-write -m gpt-5.6-sol -c model_reasoning_effort="max"`, PaneArgs = `-m gpt-5.6-sol -c
model_reasoning_effort="max"`, schema flag `--output-schema`; gemini `binary=agy`, Args = `--print ""`, PaneArgs empty,
prompt via args. P_claude = `--print --model claude-fable-5-1 --effort max --permission-mode plan --safe-mode
--no-session-persistence --disable-slash-commands --strict-mcp-config --tools=Read,Grep,Glob`. Process starts are
recorded through the real backends' seams (`newCommand`, `runtimeCodexCatalogProbe`, `providerReadinessRunner`); model
backends, terminals, and the clock are fakes. Argv compares as exact string slices; messages and lines compare exactly.

### S1: Executed argv at the subprocess boundary
Priority: Must
Given F-default as reviewers on a pane-capable terminal and the real subprocess backend with `newCommand` recorded
When each reviewer executes
Then claude's `Execution.Command` is `claude` followed by P_claude, with the prompt on stdin
And codex's is `codex exec --json --sandbox read-only -m gpt-5.6-sol -c model_reasoning_effort="max" --ephemeral --ignore-user-config --ignore-rules` followed only by `--output-schema <path>` and `--output-last-message <path>`
And gemini's is `agy --print <prompt> --mode plan --sandbox --disable-slash-commands`
And a codex entry with empty args and `model_policy: quality`, whose assembly seeds `exec --sandbox workspace-write` through `applyCodexProfileArgs`, executes exactly one sandbox item `--sandbox read-only`
And no executed argv contains `workspace-write`, `--dangerously`, `bypass`, `--allowedTools`, `--mcp-config`, or a `--tools` value other than `Read,Grep,Glob`
### S2: Read-only review never launches a pane
Priority: Must
Given a fake cmux terminal, `orchestra.subprocess.enabled: false`, no `--subprocess` flag, F-default reviewers, and judge claude
When `auto spec review` builds `orchCfg` and the routed backend
Then `orchCfg.ReadOnly` and `orchCfg.SubprocessMode` are true, `BackendNameFor` is `subprocess` for claude, codex, gemini, and the judge, and the fake terminal records 0 pane creations and 0 `SendLongText` calls
And stderr has `spec review: read-only review runs providers in subprocess mode` exactly once, and a plain terminal prints no notice
And an OMP-backed claude reports `BackendNameFor` = `omp`
And the real `buildPaneLaunchCommand(paneLaunchFor(orchCfg), p, "")` for projected claude and gemini returns a command without `--dangerously-skip-permissions`, while the same call with `ReadOnly` false ends with ` --dangerously-skip-permissions`
### S3: Judge projection on both paths and idempotency
Priority: Must
Given `spec.review_gate.judge: claude` with F-default reviewers (path A), and `--providers codex` with judge claude resolved separately (path B)
When the judge config is resolved
Then on both paths the judge Args equal P_claude and its SandboxMode is `read-only`
And projecting the projected F-default again returns identical Args, PaneArgs, and SandboxMode for all three providers with a nil error
And on path B a judge entry with args `--print --verbose` fails with the S6 claude `--verbose` message and 0 executions
### S4: Claude tool and MCP restriction accepts only the policy values
Priority: Must
Given claude Args (a) `--print --tools Read,Grep,Glob`, (b) `--print --tools=Read,Grep,Glob --model m`, (c) `--print --strict-mcp-config --model m`, (d) `--print --tools Bash,Read`, (e) `--print --tools=default`, (f) `--print --tools Read Grep`, (g) `--print --mcp-config {"mcpServers":{}}`
When `applyReadOnlyProviderPolicy` projects each input
Then (a) yields `--print --permission-mode plan --safe-mode --no-session-persistence --disable-slash-commands --strict-mcp-config --tools=Read,Grep,Glob`
And (b) yields `--print --model m --permission-mode plan --safe-mode --no-session-persistence --disable-slash-commands --strict-mcp-config --tools=Read,Grep,Glob`
And (c) yields `--print --strict-mcp-config --model m --permission-mode plan --safe-mode --no-session-persistence --disable-slash-commands --tools=Read,Grep,Glob`
And (d), (e), and (f) fail with `read-only provider policy: provider "claude" contains unsafe value for "--tools"`
And (g) fails with `read-only provider policy: provider "claude" contains unsupported argv "--mcp-config"`
### S5: The restriction never swallows a positional prompt
Priority: Must
Given a claude entry with `prompt_via_args: true` and the prompt `Review SPEC-X`
When the subprocess backend builds the argv
Then the argv equals P_claude followed by the single item `Review SPEC-X`
And no separated `--tools` item exists, matching RFP-1, where the inline form delivered the prompt and the separated form exited 1
### S6: Conflicting explicit config fails closed before any binary runs
Priority: Must
Given each explicit config row below (wrapper binaries are installed in a temp dir), once without and once with `--allow-degraded`
When `auto spec review` runs
Then it exits non-zero with exactly the listed message, and the fake backend, `runtimeCodexCatalogProbe`, and `providerReadinessRunner` record 0 calls

| Config | Message |
|--------|---------|
| claude args `--print --verbose` | `spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--verbose" (config key: orchestra.providers.claude.args; remedy: remove "--verbose" from orchestra.providers.claude.args)` |
| claude args `--print --allowedTools Bash(git *)` | `spec review: provider "claude" rejected by the read-only policy: contains unsupported argv "--allowedTools" (config key: orchestra.providers.claude.args; remedy: remove "--allowedTools" from orchestra.providers.claude.args)` |
| claude pane_args `--print --dangerously-skip-permissions` | `spec review: provider "claude" rejected by the read-only policy: contains unsafe argv "--dangerously-skip-permissions" (config key: orchestra.providers.claude.pane_args; remedy: remove "--dangerously-skip-permissions" from orchestra.providers.claude.pane_args)` |
| claude binary `<tmp>/claude-wrapper` | `spec review: provider "claude" rejected by the read-only policy: requires native binary "claude" (config key: orchestra.providers.claude.binary; remedy: set orchestra.providers.claude.binary to "claude")` |
| codex binary `<tmp>/codex-wrapper`, `model_policy: quality` | `spec review: provider "codex" rejected by the read-only policy: requires native binary "codex" (config key: orchestra.providers.codex.binary; remedy: set orchestra.providers.codex.binary to "codex")` |
| codex args `exec --sandbox danger-full-access` | `spec review: provider "codex" rejected by the read-only policy: contains unsafe value for "--sandbox" (config key: orchestra.providers.codex.args; remedy: remove "--sandbox danger-full-access" from orchestra.providers.codex.args)` |
| codex args `exec -c sandbox_mode=workspace-write` | `spec review: provider "codex" rejected by the read-only policy: contains unsafe value for "-c" (config key: orchestra.providers.codex.args; remedy: remove "-c sandbox_mode=workspace-write" from orchestra.providers.codex.args)` |
| claude `subprocess.schema_flag: --permission-mode=bypassPermissions` | `spec review: provider "claude" rejected by the read-only policy: unsupported schema flag "--permission-mode=bypassPermissions" (config key: orchestra.providers.claude.subprocess.schema_flag; remedy: remove orchestra.providers.claude.subprocess.schema_flag)` |
| gemini args `--print "" --yolo` | `spec review: provider "gemini" rejected by the read-only policy: contains unsafe argv "--yolo" (config key: orchestra.providers.gemini.args; remedy: remove "--yolo" from orchestra.providers.gemini.args)` |
### S7: Narrowing values are replaced and listed benign values are kept
Priority: Must
Given the inputs below
When `applyReadOnlyProviderPolicy` projects each input
Then the projected Args equal the expected value exactly

| Input Args | Expected projected Args |
|------------|-------------------------|
| claude `--print --permission-mode acceptEdits` | `--print --permission-mode plan --safe-mode --no-session-persistence --disable-slash-commands --strict-mcp-config --tools=Read,Grep,Glob` |
| claude `--print --model claude-opus-5-5 --output-format json` | `--print --model claude-opus-5-5 --output-format json --permission-mode plan --safe-mode --no-session-persistence --disable-slash-commands --strict-mcp-config --tools=Read,Grep,Glob` |
| codex `exec -s workspace-write` | `exec --sandbox read-only --ephemeral --ignore-user-config --ignore-rules` |
| codex `exec -s=workspace-write --json` | `exec --json --sandbox read-only --ephemeral --ignore-user-config --ignore-rules` |
| gemini `--print "" --mode default` | `--print "" --mode plan --sandbox --disable-slash-commands` |

And the pre-change output `exec -s workspace-write --sandbox read-only ...` fails this scenario; codex-cli 0.160.0 rejects that argv with exit 2
### S8: Provenance decides between fail-closed and exclusion
Priority: Must
Given a CLI-backend provider `opencode` and the selections below
When `auto spec review` runs
Then `--providers claude,opencode` with opencode not installed fails with `spec review: provider "opencode" rejected by the read-only policy: unsupported provider "opencode" (config key: --providers; remedy: remove "opencode" from --providers)` and prints no missing-binary warning first
And `spec.review_gate.providers: [claude, opencode]` and `spec.review_gate.judge: opencode` fail with the same message using config keys `spec.review_gate.providers` and `spec.review_gate.judge`
And `--multi` with reviewers `[claude, codex]`, where opencode appears only in `orchestra.commands.review.providers` or `orchestra.providers`, prints `spec review: excluding discovered provider "opencode": unsupported provider "opencode"`, executes claude and codex only, records the row (opencode, reviewer, "", "", excluded), shows no opencode Provider Health row, and uses quorum denominator 2
And `--multi` with reviewers `[claude, codex]` and a discovered gemini whose binary is `<tmp>/agy-wrapper` prints `spec review: excluding discovered provider "gemini": requires native binary "agy"` and continues with claude and codex
### S9: Receipt rows come from the executed argv
Priority: Must
Given F-default reviewers, judge claude, probe results claude ready and codex ready, and a fake model backend that returns PASS
When the review finishes
Then `review-receipt.json` `provider_policy` holds exactly (claude, reviewer, read-only, ready), (codex, reviewer, read-only, ready), (gemini, reviewer, unverified, unknown(no_status_command)), (claude, judge, read-only, ready) in this order
And a test seam that skips the projection yields sandbox_mode `unrestricted` for claude, `workspace-write` for codex, `unrestricted` for gemini, and `unrestricted` for the judge
And `orchestra.ProviderSandboxMode` over a `read-only`-stamped claude config returns `unrestricted` for argv `--print --dangerously-skip-permissions` and `read-only` for P_claude
And a receipt fixture from a run without provider execution keeps its existing bytes because the field is `omitempty`
### S10: Readiness classification for heterogeneous probe results
Priority: Must
Given the fake runner returns each result below and the listed variable is present in the probe environment
When the preflight classifies it
Then the status equals the expected value, the runner received exactly the Readiness Contract argv, and the fake model backend records 0 calls

| ID | Provider | Probe result | Expected |
|----|----------|--------------|----------|
| R1 | claude | exit 0, `{"loggedIn":true,"authMethod":"claude.ai","apiProvider":"firstParty","email":"dev@example.com","orgId":"org-123"}` | ready |
| R2 | claude | exit 1, `{"loggedIn":false,"authMethod":"none","apiProvider":"firstParty","analyticsDisabled":false}` | not_ready(logged_out), remedy `claude auth login` |
| R3 | claude | R2 output with `ANTHROPIC_API_KEY` non-empty | unknown(env_credentials) |
| R4 | claude | exit 0, `{"loggedIn":true,"authMethod":"api_key","apiProvider":"firstParty","apiKeySource":"ANTHROPIC_API_KEY"}` | ready |
| R5 | claude | exit 0, stdout `Logged in` | unknown(unparsable) |
| R6 | claude | no exit within 5 s | unknown(timeout), returned within 5.5 s |
| R7 | codex | exit 0, stdout empty, stderr `Logged in using ChatGPT` | ready |
| R8 | codex | exit 1, stderr `Not logged in` | not_ready(logged_out), remedy `codex login` |
| R9 | codex | R8 output with `CODEX_API_KEY` non-empty | unknown(env_credentials) |
| R10 | codex | exit 2, stderr `error: unexpected argument 'status'` | unknown(probe_failed) |
| R11 | claude | start error `executable file not found` | unknown(probe_failed) |
| R12 | gemini | not executed | unknown(no_status_command), 0 runner calls for agy |

And with `--skip-provider-readiness` the runner records 0 calls and every receipt row has readiness `skipped`
And three probes that each take 1 s finish in under 2.5 s wall time
### S11: OMP classification uses the review backend's environment
Priority: Must
Given an OMP-backed claude with model `anthropic/claude-opus-5-5:max`, the outputs below, and the assumed element shape `{"provider","account","reason"}` until T8 replaces it
When the preflight probes it
Then the status equals the expected value and every not_ready remedy is `omp login anthropic`, with ` (same PI_CODING_AGENT_DIR as this review)` appended when that variable is set

| ID | `omp usage --json --redact` result | Expected |
|----|------------------------------------|----------|
| O1 | `{"generatedAt":1,"reports":[],"accountsWithoutUsage":[],"disabledCredentials":[],"capacity":{}}` | unknown(no_account_evidence) |
| O2 | O1 with disabledCredentials `[{"provider":"anthropic","account":"c3d4","reason":"Refresh token expired"}]` | not_ready(auth_expired) |
| O3 | O1 with disabledCredentials `[{"provider":"anthropic","account":"c3d4","reason":"Account disabled"}]` | not_ready(account_disabled) |
| O4 | O2 plus reports `[{"provider":"anthropic","account":"a1b2"}]` | ready, warning `omp anthropic: 1 of 2 accounts unusable (auth_expired); run "omp usage --redact" for details` |
| O5 | O1 with accountsWithoutUsage `[{"provider":"anthropic","account":"a1b2"}]` | ready |
| O6 | O1 with reports `[{"provider":"openai-codex","account":"e5f6"}]` | unknown(no_account_evidence) |
| O7 | O1 with disabledCredentials `[{"account":"c3d4","reason":"Refresh token expired"}]` | unknown(no_account_evidence) |
| O8 | `{"reports":[]}` | unknown(unparsable) |
| O9 | exit 1 | unknown(probe_failed) |

And the runner argv is `<canonical omp> usage --json --redact` with no `--profile` and no `--provider`, and its environment equals `normalizePipelineOMPEnvironment(os.Environ())`
And an OMP claude reviewer, an OMP codex reviewer, and an OMP claude judge produce exactly one probe execution
### S12: A not-ready reviewer is excluded and degrades promotion
Priority: Must
Given F-default, fully observed documents, probe results R1 for claude and R8 for codex, judge claude, the default quorum, and a fake model backend returning PASS for claude, gemini, and the judge
When `auto spec review` runs without and then with `--allow-degraded`
Then stderr has `preflight: codex not_ready(logged_out) - run "codex login" (excluded; degraded: provider_unready:codex:logged_out)` and codex records 0 executions
And Provider Health has (claude, success), (codex, error, `excluded: not_ready(logged_out)`), (gemini, success), the quorum denominator is 3, and the merged verdict is PASS
And the final `DegradedReasons` equals `["provider_unready:codex:logged_out"]`, and the `review.md` verdict line ends with `(degraded: provider_unready:codex:logged_out)`
And without `--allow-degraded` the SPEC status stays `draft`, while with it the status becomes `approved` and `override_applied` is true
And the receipt holds (codex, reviewer, "", not_ready(logged_out), excluded) between the claude and gemini rows
And reviewers `[claude, codex]` with R2 and R8 and no judge fail with `spec review: no ready reviewer remains; run "claude auth login"; run "codex login"` and 0 executions
### S13: A not-ready judge fails before the first execution (incident fixture)
Priority: Must
Given the repo-style config where claude, codex, and gemini use `backend: omp` with models `anthropic/...`, `openai-codex/...`, `google-antigravity/...`, judge claude, and one `omp usage` result holding O2 for anthropic and usable reports for the other two families
When `auto spec review` runs
Then it exits non-zero with `spec review: judge "claude" is not ready: not_ready(auth_expired); run "omp login anthropic"`
And the fake model backend records 0 executions, no `review-receipt.json` is written, and the runner made exactly 1 probe execution
### S14: Doctor smoke uses the spec review assembly and routing
Priority: Must
Given F-default plus an OMP-backed provider and `auto doctor --provider-smoke` with the routed smoke factory recorded
When the smoke runs
Then the recorded Args equal the S1 projected Args for claude, codex, and gemini, executed by the subprocess backend
And the OMP-backed provider executes through the OMP review backend (`BackendNameFor` = `omp`) and never as a plain `omp` subprocess
And a claude entry with args `--print --verbose` yields one result (review_gate, fail, the S6 claude `--verbose` message) and 0 executions
### S15: Doctor readiness check in text and JSON
Priority: Must
Given an OMP-backed claude returning O2, a CLI codex returning R7, a CLI gemini (agy), judge claude, and no `--provider-smoke`
When `auto doctor --json` and `auto doctor` run
Then the JSON checks contain {doctor.provider_readiness.claude, error, fail, `not_ready(auth_expired): run "omp login anthropic"`}, {doctor.provider_readiness.codex, info, pass, `ready`}, and {doctor.provider_readiness.gemini, info, skip, `unknown(no_status_command)`}, the report status is `warn`, and warning code `provider_unready` names claude
And the text output under `Provider Readiness` has `claude readiness: not_ready(auth_expired) - run "omp login anthropic"` as a FAIL line
And judge codex with reviewers `[claude]` yields exactly the checks for claude and codex, the smoke check stays `skip`, and the fake model backend records 0 calls
### S16: Untrusted probe output is bounded and redacted
Priority: Must
Given R1, an O4 variant whose account fields hold `dev@example.com` and `sk-ant-oat01-abc123`, a valid R1 output padded with spaces to exactly 65,536 bytes, and a 70 KiB claude stdout behind a counting reader
When the preflight, receipt, and doctor JSON are produced
Then none of `dev@example.com`, `org-123`, `sk-ant-` appears in stderr, `review-receipt.json`, or doctor output
And the 65,536-byte output classifies as ready, while the 70 KiB output classifies as unknown(oversized_output) after the counting reader served exactly 65,537 bytes
And across S10–S15 the runner received only `claude auth status --json`, `codex login status`, and `<canonical omp> usage --json --redact`, never through a shell
### S17: Existing read-only consumers change only by the claude items
Priority: Must
Given the plan, brainstorm, and SPEC-ORCH-021 S15–S20 argv tests
When they run after T1
Then each claude expectation differs from the pre-change golden only by the trailing `--strict-mcp-config --tools=Read,Grep,Glob`, codex and gemini expectations are unchanged, `readOnlyOrchestraCommand("review")` is false, the brainstorm worktree guard test passes, and the six existing policy reason texts are byte-identical

## Oracle Acceptance Notes

- Oracles are exact argv slices, messages, receipt rows, Provider Health rows, and status tokens; R6, S16, and the
  concurrency row use explicit byte or time bounds. Existence, headings, exit codes, or non-empty output close nothing.
- Heterogeneous entities: three provider CLIs, the OMP backend, explicit and discovered provenance, two judge paths,
  pane-capable and plain terminals, OAuth and environment credentials, text and JSON doctor output.
- Wrong-implementation discriminators: S1 seeded `workspace-write`, S2 `ReadOnly` false, S6 catalog probe of a wrapper,
  S7 `-s` duplicate, S9 unprojected seam and bypass-after-stamp, S12 degraded-reason overwrite, S5 separated `--tools`.
- Live evidence: RFP-1 and RFP-2 PASS; RFP-3 (agy) is completion-blocking. Unit tests never call a real provider.

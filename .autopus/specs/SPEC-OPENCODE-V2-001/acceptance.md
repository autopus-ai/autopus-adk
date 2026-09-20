# Acceptance

- AC1: V1/V2 version selection is deterministic when pinned, probes bounded by
  timeout/output limits otherwise, and rejects unknown future majors before writes.
- AC2: V1 and V2 hooks execute only appropriate tools; V2 resolves the operation's
  working directory, propagates before failure, bounds execution, cleans up hooks.
- AC3: Generation preserves plugin options and effective configuration shape;
  validation identifies managed registration and disabled state truthfully.
- AC4: V2 managed instructions contain native subagent schema/lifecycle guidance,
  preserve user-owned root-document text, and V1 examples remain supported.
- AC5: Relevant tests and installed native hook probe have recorded outcomes;
  model authentication is not promoted into successful delegation evidence.

## Executed evidence, 2026-09-20

- AC1 PASS: version response tests, future-major rejection, unknown-runtime
  preservation of an existing V2 plugin, and real CLI init against installed
  2.0.10. Fresh V2 config contains plugins and the V2 native guidance.
- AC2 PASS: generated V1/V2 JS runtime tests (no SDK shim); V2 seven scenarios
  cover cwd, before/after, failure, timeout/descendants, output bounds, and
  disposal. Native host test below confirms actual dispatch and failure blocking.
- AC3 PASS: legacy tuple/native object options, native precedence, explicit
  disable ordering, relative/absolute/file URL equivalence and no duplicate
  managed entry; malformed config fails before InstallHooks writes its plugin.
- AC4 PASS: native agent/description/prompt and sessionID/background guidance;
  user-owned text outside AGENTS marker survives both V2 and shared team-name
  normalization. V1 syntax remains covered by explicitly pinned existing tests.
- AC5 PASS: relevant race tests, vet, candidate build and local generation
  checked; final integrated test outcome recorded below. Full-repo coverage is
  not remeasured and model authentication remains outside these checks.

### Actual OpenCode 2.0.10 host, no manually installed SDK

Command: `AUTOPUS_OPENCODE_V2_LIVE=1 go test ./pkg/adapter/opencode -run TestNativeOpenCodeV2HookDispatch -count=1 -v`.

The final run used only the generated native plugin object and an isolated
fixture config. No package.json, node_modules, npm installation, SDK shim or
real model credentials were provided. A deterministic loopback provider emitted
native shell tool calls:

- allowed command: before -> tool -> after, command marker present;
- rejected before hook: before only, command marker absent;
- four synthetic requests, zero stub errors;
- task-owned OpenCode server and synthetic provider both stopped.

Sanitized report: `.autopus/runtime/agent-lifecycle/2026-09-20/opencode-v2/native-no-sdk.json`.
Original report: `/tmp/autopus-opencode-v2-research/native-generated-object-final/report.json`.

Earlier setup failures and the missing-SDK failure remain separate records.
The missing dependency defect was reproduced against the actual host: the shell
ran while hook markers were absent. Removing the unnecessary SDK import closed
that defect; the final strong oracle ran again with no manual SDK.

Review: OC2-01 (relative path identity) and OC2-02 (unknown-version downgrade)
closed with regressions and focused verification. Hook dispatch verification is
not a successful real-model delegation/cancellation certificate and does not
resolve the earlier 401. Local-only generated surfaces and evidence are not
release source artifacts.

Final current-source race run: `go test -race -p 2 ./pkg/adapter ./pkg/adapter/opencode ./pkg/content -count=1` PASS for all three packages.
`go vet -p 2 ./pkg/adapter/opencode`, candidate build, `git diff --check` and
`auto sync verify` PASS. The current repository received an OpenCode-only adapter
transaction (100 generated mappings), with config loaded read-only and managed
files backed up by the transaction layer. Adapter Validate returned no findings.
The installed local plugin has no external SDK import. Other platform renderers
and the global installed auto binary were not updated by this transaction.

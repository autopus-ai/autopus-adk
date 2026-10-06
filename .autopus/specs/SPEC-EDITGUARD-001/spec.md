# SPEC-EDITGUARD-001: Deterministic Blocking Edit Guard

**Status**: approved
**Created**: 2026-10-06
**Domain**: EDITGUARD
**Target module**: autopus-adk
**Source**: `prd.md` in this directory (direct `/auto plan` Plan Intent Ledger, no BS file); user decision D2 (2026-10-06); review round 1 resolutions in research.md `Review Resolution`
**Change class**: `feature` (new exported CLI commands and a cross-platform hook contract). Gate applicability comes only from `gate-applicability.json` written by `auto spec gates`.

## Purpose

Autopus only advises agents not to hand-edit generated harness files and not to weaken the reproduction test during `/auto fix`. Nothing deterministic sits between an agent's file-editing tool call and the write: the drift gate runs at commit time as a warn-only Bash hook (`pkg/content/hooks.go`), and the only Edit/Write hook, `auto rules fire`, is advisory by contract (`pkg/rulecond/fire.go`). This SPEC adds a fail-open pre-tool edit guard, `auto guard edit`, and an `auto fix lock|unlock` lifecycle whose SHA-256 verdict reports content changes or deletion still present at unlock time, whatever channel made them.

## Outcome Boundary

- Outcome: on every platform whose native hook can block a tool call, a file-editing tool call is denied with an actionable reason when it targets (a) a file that a platform manifest lists with policy `always` inside the guard namespace, (b) a reproduction test locked by an in-progress `/auto fix`, or (c) guard state. Every other edit is allowed, and a guard fault never makes a decision more restrictive than the guard without the faulty part (D2).
- Mandatory platform results: Claude Code and OpenCode reach `enforced`; a probe failure on either blocks completion until the lane is fixed or the user explicitly re-approves the scope. Codex and Gemini CLI are `enforced` only where their probe passes, else `none`.
- Guard namespace: `.claude/`, `.codex/`, `.gemini/`, `.opencode/`, `.agents/`, `.omp/`, `.autopus/plugins/`, minus `.claude/worktrees/`. It is the `generated` category of the unified surface table (REQ-EG-01); no current consumer verdict changes.
- Mandatory requirements: REQ-EG-01 to REQ-EG-19 and REQ-EG-21 (Must). REQ-EG-20, REQ-EG-22, REQ-EG-23, and REQ-EG-24 have Priority Should.
- Explicit non-goals: Bash-command write detection; user-configurable protected paths; native blocking on Antigravity `.agents/hooks.json` and OMP; any change to `auto rules fire` or `auto check`; a sandbox against an agent with shell access; detection of a change that is reverted before unlock; manifest `always` entries outside the namespace (today five `.git/hooks/*` entries); locks spanning checkouts or worktrees.
- Completion evidence: CE-1 to CE-6 in research.md `Outcome Lock`.

## Requirements

Each requirement is one EARS line under a `#### ID · Priority · type` heading. The type is the name `pkg/spec` ParseEARS derives from the line (Ubiquitous, EventDriven, StateDriven, Unwanted, Optional), checked by research.md V15.

#### REQ-EG-01 · Must · Ubiquitous
THE SYSTEM SHALL classify every project-relative path as `generated`, `runtime_state`, or `agent_artifact` from one categorized surface table in `pkg/workflow`, and the drift gate (`GeneratedSurfacePrefixes`, `GeneratedSurfaceExactPaths`, `hasGeneratedPrefix`), the status hygiene check (`isRuntimeUnignoredRisk` with `runtimeUnignoredExtraPrefixes` and `runtimeUnignoredExtraExactPaths`), and `pkg/qualityloop` (`isGeneratedSurfacePath`) SHALL source their members from that table while each consumer keeps its current matching rules and verdicts.

#### REQ-EG-02 · Must · EventDriven
WHEN an agent CLI runs `auto guard edit --platform <id>` as a pre-tool hook THEN THE SYSTEM SHALL read the payload from stdin bounded at 1 MiB, extract every target path of the platform's file-editing tool input (Claude Code: `tool_input.file_path` of `Edit`, `Write`, and `MultiEdit` through `rulecond.ConditionSubject`; OpenCode: the `targets` array of the synthesized payload), and emit exactly one decision in the platform encoding of the Decision Output Contract section, without calling `auto rules fire`.

#### REQ-EG-03 · Must · EventDriven
WHEN a target lies inside the guard namespace and at least one `.autopus/<platform>-manifest.json` lists it with policy `always` while no manifest lists it with policy `merge` or `marker` THEN THE SYSTEM SHALL deny the call with class `generated_surface` and reason GS-SRC in the ADK source repo (markers `content/`, `templates/`, `cmd/generate-templates` per `internal/cli/doctor_drift_source.go`) or GS-CON in any other project, naming the first such manifest in lexical order.

#### REQ-EG-04 · Must · EventDriven
WHEN a target matches none of REQ-EG-03, REQ-EG-07, and REQ-EG-09 THEN THE SYSTEM SHALL allow it, which explicitly covers `.autopus/brainstorms/**` even with a manifest entry, `.autopus/specs/**`, `.claude/worktrees/**`, manifest `merge` and `marker` files, unmanifested files under namespace prefixes, root `config.toml`, files outside the namespace, and new files.

#### REQ-EG-05 · Must · Ubiquitous
THE SYSTEM SHALL classify each target against the nearest ancestor directory that contains `autopus.yaml` rather than the session cwd, after resolving relative paths against the payload `cwd`, cleaning `.` and `..` segments and duplicate or OS-specific separators, resolving symlinks of the deepest existing ancestor, and folding case only on volumes detected as case-insensitive.

#### REQ-EG-06 · Must · EventDriven
WHEN `auto fix lock <path>...` runs THEN THE SYSTEM SHALL hold the exclusive store lock (`.autopus/runtime/fix-locks/.store.lock` through the OS file-lock helper, waiting at most 5 seconds) for the whole command, validate every named path before writing anything (an existing regular file inside the project root and usable lock state), publish one `autopus.fix_lock.v1` record per file (normalized relative path, SHA-256 of content, `created_at`, `expires_at`) by hard-linking a complete temp file into place with `os.Root.Link`, keep an existing unexpired record unchanged so the first hash survives re-locking, remove the records this invocation published on a later publish failure, and exit 0, or exit 1 with the store left as it was.

#### REQ-EG-07 · Must · StateDriven
WHERE an unexpired lock exists whose recorded path equals the normalized target or whose recorded file is `os.SameFile` with the target THEN THE SYSTEM SHALL deny the call with class `fix_lock` and reason FL, which covers a Write that recreates a deleted locked path.

#### REQ-EG-08 · Must · EventDriven
WHEN `auto fix unlock <path>...` or `auto fix unlock --all` runs THEN THE SYSTEM SHALL hold the same store lock for the whole command, check that every named path has a lock record and otherwise exit 1 removing nothing, compute every verdict before removing anything (`unchanged`, `modified`, or `missing` from the current versus lock-time SHA-256, `unverifiable` for a corrupt record), remove the matching records, print the verdicts (`--json` schema `autopus.fix_unlock.v1`), and exit 0 for all `unchanged`, exit 3 for any other verdict, and exit 1 on a removal I/O error with the unremoved locks still active for a rerun to finish.

#### REQ-EG-09 · Must · EventDriven
WHEN a file-editing tool call targets `.autopus/runtime/fix-locks/**` or `.autopus/*-manifest.json` THEN THE SYSTEM SHALL deny it with class `guard_state` and reason GST, and the system SHALL create and read lock state only through the `os.Root` component-wise descent of `openStateRoot` and `descendStateComponent` (`pkg/rulecond/sticky_state_dir.go`).

#### REQ-EG-10 · Must · Unwanted
IF a call-level fault occurs (unreadable, empty, oversized, or malformed stdin; a payload with no extractable target; a recovered panic) THEN THE SYSTEM SHALL allow the call by exiting 0 with empty stdout and at most one stderr line.

#### REQ-EG-11 · Must · Ubiquitous
THE SYSTEM SHALL emit a Claude Code deny only as exit 0 with the stdout JSON of the Decision Output Contract written in one write immediately before exit, emit an allow as exit 0 with empty stdout and never as `permissionDecision` `allow` (that value skips the user's permission flow), never exit 2 by design, and register a command line that forwards the guard's stdout only after the guard process exits 0, discards it on any other exit (including the Go runtime's exit 2 on an unrecovered panic and a crash after output), and itself always exits 0.

#### REQ-EG-12 · Must · StateDriven
WHERE `hooks.edit_guard` is enabled (default `true`, assumed) THEN THE SYSTEM SHALL register one Claude Code PreToolUse entry whose only handler is the REQ-EG-11 command line, with matcher `Edit|Write|MultiEdit` and timeout 5 seconds, recognized by `isManagedClaudeHookCommand` through the anchored prefix `out=$(auto guard edit `, so that two consecutive `auto update` runs produce byte-identical `.claude/settings.json`, existing Autopus hooks keep firing, and disabling the flag retracts the guard handler.

#### REQ-EG-13 · Must · Ubiquitous
THE SYSTEM SHALL change both generated OpenCode plugins (`opencode_plugin.go` with its `bash` hooks, `opencode_plugin_v2.go` with its `shell` hooks) so that each before-hook carries its own tool filter (existing shell-tool hooks unchanged, the guard only for the file-editing tools named by the per-version native payload fixtures), the guard receives a synthesized JSON payload on stdin carrying `cwd`, the native tool name, and every target including later patch targets and move destinations, and only a stdout decision `deny` from a guard process that exited 0 throws, while a timeout, a spawn failure, or any other exit resolves.

#### REQ-EG-14 · Must · Optional
WHEN `auto update` generates Codex or Gemini CLI hooks IF the lane's probe (A3 for Codex, the T11 probe for Gemini CLI) recorded PASS with evidence THEN THE SYSTEM SHALL register the guard in `.codex/hooks.json` or `.gemini/settings.json` and check every target of a multi-file patch including move destinations, and a lane without PASS evidence gets no guard entry and the matrix state `none`.

#### REQ-EG-15 · Must · Ubiquitous
THE SYSTEM SHALL change the canonical `/auto fix` sources (`templates/claude/commands/auto-workflows.md.tmpl` section `fix`, `templates/codex/skills/auto-fix.md.tmpl`, `templates/codex/prompts/auto-fix.md.tmpl`, `templates/gemini/skills/auto-fix/SKILL.md.tmpl`) so that the workflow runs `auto fix lock -- <test-path>` once the reproduction test compiles and fails on the bug assertion, forbids agent-initiated unlock before verification, runs `auto fix unlock --json -- <test-path>` at completion, accepts only verdict `unchanged` in Pre-Completion Verification (any other verdict means not complete: stop and ask the user), and records `lock: not applicable` for a fix without a test, and SHALL regenerate every surface from those sources.

#### REQ-EG-16 · Must · Ubiquitous
THE SYSTEM SHALL publish in [NEW] `docs/edit-guard.md` a platform enforcement matrix with one state per platform (`enforced`, `advisory-only`, or `none`) for Claude Code, OpenCode, Codex, Gemini CLI, Antigravity (`advisory-only`, always-allow wrapper in `pkg/content/hooks_antigravity.go`), and OMP (`none`, `SupportsHooks()` is false), plus the non-sandbox limitations of research.md Reviewer Brief and the rollback steps.

#### REQ-EG-17 · Must · Ubiquitous
THE SYSTEM SHALL treat hook stdin as untrusted data that is never executed, never interpolated into a shell, and never used to choose the platform, ignore manifest entries outside the namespace, and write reasons and stderr diagnostics that contain only project-relative paths (other absolute paths become `<redacted>`), with control characters removed, the displayed path capped at 256 bytes, every command argument POSIX single-quoted after a `--` end-of-options marker, the FL-X form for any FL reason that would exceed 1024 bytes, and the whole reason capped at 1024 bytes.

#### REQ-EG-18 · Must · Unwanted
IF a fault makes part of the classification input untrustworthy THEN THE SYSTEM SHALL drop exactly that part (the record for a corrupt lock record, the lock stage of the root for unusable lock state, the manifest stage of the root for any unreadable or corrupt manifest, the target for a malformed multi-target entry or a target without a project root), keep evaluating everything else, and never return a decision more restrictive than the guard returns without the dropped part.

#### REQ-EG-19 · Must · Ubiquitous
THE SYSTEM SHALL add, replace, and remove Claude Code hook handlers one handler at a time in every Autopus writer of `.claude/settings.json` (`retractManagedHookEntries` in `pkg/adapter/claude` and `WriteHookConfig` and `RemoveHookConfig` in `pkg/worker/security`, the latter in the current handler object format), so that user handlers and other Autopus handlers sharing an entry or an event survive setup, regeneration, flag-off retraction, and cleanup.

#### REQ-EG-20 · Should · StateDriven
WHERE the hook process environment contains `AUTOPUS_EDIT_GUARD=off` THEN THE SYSTEM SHALL allow every edit with empty stdout and no stderr output.

#### REQ-EG-21 · Must · EventDriven
WHEN `auto fix lock --list` runs with or without `--json` THEN THE SYSTEM SHALL list every lock with its path, state `active` or `stale`, `created_at`, `expires_at`, and current integrity `unchanged`, `modified`, `missing`, or `unverifiable`.

#### REQ-EG-22 · Should · Ubiquitous
THE SYSTEM SHALL apply a default lock TTL of 24 hours (assumed), accept `--ttl` between 1 minute and 168 hours, ignore expired locks in the guard as a normal state rather than a fault, report them as `stale`, and replace an expired record on re-lock.

#### REQ-EG-23 · Should · EventDriven
WHEN `auto doctor` runs THEN THE SYSTEM SHALL report for each installed platform whether the guard entry is registered and the platform's matrix state.

#### REQ-EG-24 · Should · Ubiquitous
THE SYSTEM SHALL keep guard process wall time, excluding host spawn overhead such as the OpenCode node `spawn`, at or below 150 ms at p95 over 200 warm invocations on the autopus-adk repository by reading manifests only for targets inside the namespace and lock records only for an existing lock directory.

## Decision Output Contract

Per target, stages run in the order guard state, lock, manifest; precedence is `guard_state`, then `fix_lock`, then `generated_surface`, then allow. A call-level fault (REQ-EG-10) allows the whole call. Any other fault (REQ-EG-18) drops exactly the untrustworthy part: one corrupt lock record, the lock stage of a root whose lock state is unusable, the manifest stage of a root with any unreadable or corrupt manifest (that manifest may have carried the `merge` or `marker` entry that prevents a false deny), or one malformed target. Expired locks are a normal state. Lock-store mutations run under one exclusive store lock; readers (the guard and `--list`) take no lock, skip `.store.lock` and temp files, see only complete records, and can briefly see a record of an in-flight batch that is later rolled back. The first denied target in payload order decides. `{path}` is the sanitized display path; `{arg}` is the original project-relative path, POSIX single-quoted.

| ID | Class | Reason text (exact) |
|---|---|---|
| GS-SRC | generated_surface | `autopus edit-guard [generated_surface]: {path} is generated (manifest {manifest}, policy always). Change the canonical source (content/, templates/, pkg/adapter/) and run: make generate-templates && auto update` |
| GS-CON | generated_surface | `autopus edit-guard [generated_surface]: {path} is generated (manifest {manifest}, policy always). Change autopus.yaml or the upstream Autopus source, then run: auto update` |
| FL | fix_lock | `autopus edit-guard [fix_lock]: {path} is the locked reproduction test of an in-progress /auto fix. Fix the code under test instead. If the test itself is wrong, stop and ask the user to run: auto fix unlock -- {arg}` |
| FL-X | fix_lock | FL with the final sentence replaced by `If the test itself is wrong, stop and ask the user to find the path with auto fix lock --list --json and unlock it.`; used for a path with control characters, invalid UTF-8, or more than 256 bytes, and for any path whose complete FL reason would exceed 1024 bytes |
| GST | guard_state | `autopus edit-guard [guard_state]: {path} is edit-guard state. Use auto fix lock, auto fix unlock, or auto update instead.` |

| Platform | Deny | Allow and every fault |
|---|---|---|
| `claude-code` | exit 0; stdout `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"<reason>"}}` plus one newline | exit 0; empty stdout |
| `opencode` | exit 0; stdout `{"decision":"deny","reason":"<reason>"}`; the plugin throws `Error(<reason>)` | the plugin resolves |
| `codex`, `gemini` | the encoding verified by A3 or the T11 probe | nothing registered without PASS evidence |

- Registered Claude command line (canonical, POSIX shell): `out=$(auto guard edit --platform claude-code) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0`. It prints the buffered decision only after the guard exited 0 and always exits 0.
- OpenCode synthesized payload: `{"platform":"opencode","cwd":"<dir>","tool_name":"<native tool>","targets":["<path>", "..."]}` with targets in native argument order, move destinations included.
- Lock record: `{"schema":"autopus.fix_lock.v1","path":"<rel>","sha256":"<hex>","created_at":"<RFC3339>","expires_at":"<RFC3339>"}`. File identity is evaluated at decision time with `os.SameFile` against the recorded path.
- Unlock output: `{"schema":"autopus.fix_unlock.v1","results":[{"path":"<rel>","verdict":"<verdict>","locked_sha256":"<hex or empty>","current_sha256":"<hex or empty>"}]}`.
- List output: `{"schema":"autopus.fix_lock_list.v1","locks":[{"path":"<rel>","state":"<active or stale>","created_at":"<RFC3339>","expires_at":"<RFC3339>","integrity":"<verdict>"}]}`.
- Exit codes: `auto guard edit` is always 0; `auto fix lock` is 0 or 1; `auto fix unlock` is 0, 3, or 1.
- Sanitization: absolute paths outside the project become `<redacted>`; bytes below 0x20 and 0x7f are removed from `{path}`; a `{path}` over 256 bytes is cut on a UTF-8 boundary to 253 bytes plus `...`; the reason never exceeds 1024 bytes.

## Generated File Details

| Path | Change | Role |
|---|---|---|
| [NEW] `pkg/workflow/surface_class.go` | add | categorized surface table and per-consumer member sets |
| `pkg/workflow/drift_gate.go`, `pkg/qualityloop/safety.go`, `internal/cli/status_hygiene.go` | modify | source members from the table; matching rules and exported names stay |
| [NEW] `pkg/editguard/resolve.go`, `classify.go`, `lock.go`, `lock_store.go`, `decide.go`, `dialect.go` | add | guard logic, each file at most 300 lines |
| `pkg/rulecond/sticky_state_dir.go` | modify | [NEW] exported `OpenRuntimeStateDir(projectRoot, name)` over the existing helpers |
| [NEW] `pkg/oslock` (from `pkg/companionmanifest/signed_pair_lock_{unix,windows,other}.go`) | move | exported try-lock and unlock over `flock` and `LockFileEx`; companionmanifest calls it with unchanged behavior |
| [NEW] `internal/cli/guard.go`, [NEW] `internal/cli/fix_lock.go` | add | `auto guard edit`, `auto fix lock/unlock` |
| `pkg/config/schema.go`, `pkg/config/defaults.go` | modify | [NEW] `hooks.edit_guard` |
| `pkg/content/hooks.go` | modify | guard `HookConfig` and matcher translation |
| `pkg/adapter/claude/claude_settings_hooks.go`, `pkg/worker/security/hook_template.go` | modify | handler-level ownership (REQ-EG-19) and the guard prefix |
| `pkg/adapter/opencode/opencode_plugin.go`, `opencode_plugin_v2.go`, [NEW] per-version payload fixtures under `pkg/adapter/opencode/testdata/` | modify, add | per-hook filter, synthesized payload, deny-only throw |
| `pkg/adapter/codex/codex_hooks.go`, `pkg/adapter/antigravity/antigravity_settings.go` | modify only on probe PASS | Codex and Gemini CLI lanes |
| `internal/cli/doctor*.go` | modify | registration report |
| the four auto-fix templates of REQ-EG-15 | modify | workflow lock steps |
| [NEW] `docs/edit-guard.md` | add | enforcement matrix, limitations, rollback |
| `.claude/**`, `.codex/**`, `.gemini/**`, `.opencode/**`, `.agents/**` | regenerate only | never hand-edited |

## Related SPECs

Siblings: None (research.md `Sibling SPEC Decision`). Behavior preserved, no conflict: SPEC-QALOOP-001 (qualityloop safety list), SPEC-CONDRULE-001 (shares the Edit matcher; both hooks run in parallel), SPEC-STICKYRULE-001 (runtime-state containment precedent). Independent Primary SPECs from the same planning session: SPEC-HARNEVAL-001, SPEC-SIGMABAND-001.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|---|---|---|---|
| REQ-EG-01 | T1 | S10 | INV-EG-07 |
| REQ-EG-02 | T6, T7 | S1, S7, S9 | INV-EG-06 |
| REQ-EG-03 | T3, T5 | S1, S2, S3 | INV-EG-01, INV-EG-02 |
| REQ-EG-04 | T3 | S1, S3 | INV-EG-01 |
| REQ-EG-05 | T2 | S2, S3, S4 | INV-EG-01, INV-EG-03 |
| REQ-EG-06 | T4, T7 | S4, S6, S17 | INV-EG-03, INV-EG-12 |
| REQ-EG-07 | T4, T5 | S4, S5 | INV-EG-02, INV-EG-03 |
| REQ-EG-08 | T4, T7 | S6, S17 | INV-EG-04, INV-EG-12 |
| REQ-EG-09 | T4, T5 | S5 | INV-EG-02 |
| REQ-EG-10 | T5, T7 | S7 | INV-EG-05 |
| REQ-EG-11 | T5, T6, T8 | S1, S7 | INV-EG-05, INV-EG-06 |
| REQ-EG-12 | T0, T8, T9 | S11, S12 | INV-EG-09, INV-EG-10 |
| REQ-EG-13 | T0, T10 | S9, S12 | INV-EG-05, INV-EG-10 |
| REQ-EG-14 | T0, T11 | S12 | INV-EG-10 |
| REQ-EG-15 | T13 | S13 | INV-EG-11 |
| REQ-EG-16 | T14 | S12 | INV-EG-10 |
| REQ-EG-17 | T5, T6 | S4, S8 | INV-EG-06, INV-EG-08 |
| REQ-EG-18 | T3, T4, T5 | S16 | INV-EG-05 |
| REQ-EG-19 | T9, T12 | S11 | INV-EG-09 |
| REQ-EG-20 | T7 | S15 | INV-EG-05 |
| REQ-EG-21 | T4, T7 | S17 | INV-EG-03, INV-EG-12 |
| REQ-EG-22 | T4 | S15 | INV-EG-03 |
| REQ-EG-23 | T14 | S15 | INV-EG-10 |
| REQ-EG-24 | T15 | S14 | - |

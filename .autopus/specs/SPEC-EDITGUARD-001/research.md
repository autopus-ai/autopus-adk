# SPEC-EDITGUARD-001 Research

## Reference Discipline
Existing references were read or grepped in autopus-adk on 2026-10-06; this table doubles as the existing-code analysis.

| Reference | Type | Fact used by this SPEC | Verification |
|---|---|---|---|
| `pkg/content/hooks.go` (`GenerateProjectHookConfigs`, `generateCLIHooks`, `translateHookMatcher`) | existing | PreToolUse Bash `auto check --hygiene ... --warn-only` gated by `PreCommitArch`; per-platform matcher translation; `Bash(auto *)` auto-approved (:176) | grep |
| `pkg/rulecond/fire.go`, `pkg/rulecond/scope.go` (`ConditionSubject`) | existing | `auto rules fire` always exits 0 with context only; `file_path` for Edit, Write, MultiEdit | read |
| `pkg/workflow/drift_gate.go` (`GeneratedSurfacePrefixes`, `GeneratedSurfaceExactPaths`, `hasGeneratedPrefix`) | existing | 9 prefixes, 3 exact paths, root manifests; `path.Clean` then `HasPrefix` | read |
| `pkg/qualityloop/safety.go` (`isGeneratedSurfacePath`) | existing | 11 prefixes with nested `/prefix` match, `*/config.toml`, `.autopus/*-manifest.json`, `/plugins/cache/` | read |
| `internal/cli/status_hygiene.go` (`isRuntimeUnignoredRisk`, `runtimeUnignoredExtraPrefixes`, `runtimeUnignoredExtraExactPaths`) | existing | third member set: 16 extra prefixes and 5 extra exact paths on top of both drift-gate sets, `HasPrefix` only | read |
| `pkg/adapter/claude/claude_settings_hooks.go` (`managedClaudeHookCommandPrefixes`, `retractManagedHookEntries`, `isManagedClaudeHookCommand`) | existing | prefix-based ownership; see V12 | read |
| `pkg/adapter/opencode/opencode_plugin.go`, `opencode_plugin_v2.go` | existing | see V11; stdin ignored; timeout and non-zero exit reject | grep |
| `pkg/worker/security/hook_template.go`, `autonomous.go` | existing | see V13 | read, grep |
| `pkg/rulecond/sticky_state_dir.go` (`openStateRoot`, `descendStateComponent`) | existing | `os.Root` component-wise descent refusing symlinks | read |
| `pkg/companionmanifest/signed_pair_lock.go`, `signed_pair_lock_{unix,windows,other}.go` (`lockSignedPairFile`, `unlockSignedPairFile`) | existing | see V16 | read |
| `pkg/content/hooks_antigravity.go`, `pkg/adapter/omp/omp.go:56`, codex and antigravity writers, `pkg/config/schema.go` (`PreCommitArch`), `internal/cli/doctor_drift_source.go:18` | existing | always-allow wrapper; `SupportsHooks()` false; flag pattern; source-repo markers | read, grep |
| four auto-fix templates, `content/skills/agent-pipeline.md`, `pkg/promptlayer` (`KindStable`, `KindSnapshot`), `internal/cli/workflow_context_runtime_managed_rpc_product_command.go:123` | existing | Step 1 at `auto-workflows.md.tmpl:2302`, Pre-Completion at :2339; prompt layer contract; no `auto guard` or `auto fix` CLI command today | grep |
| `surface_class.go`, `pkg/editguard/*`, `pkg/oslock`, `guard.go`, `fix_lock.go`, `OpenRuntimeStateDir`, OpenCode testdata fixtures, `docs/edit-guard.md`, `hooks.edit_guard`, three JSON schemas | [NEW] planned addition | - | not verified by design |
| `.claude/settings.json`, `.codex/hooks.json`, `.gemini/settings.json`, `.opencode/plugins/*.js`, generated `/auto fix` skills | generated surface | changed only by regeneration in T8 to T13 | - |

## Claude Code Hook Contract Verification
Source for V1 to V6: https://code.claude.com/docs/en/hooks through WebFetch, checked_at 2026-10-06; excerpts were extracted by the fetch model (confidence medium-high). Codex, Gemini CLI, and OpenCode host contracts stay unverified (A2, A3, T11).

| ID | Class | Fact | Evidence |
|---|---|---|---|
| V1 | verified_fact | On exit 0, stdout JSON is read; `hookSpecificOutput.permissionDecision` is allow, deny, ask, or defer, with `permissionDecisionReason`; with no JSON the call goes through the normal permission flow | docs, exit codes and decision control |
| V2 | verified_fact | Exit 2 blocks PreToolUse even when JSON says allow, and stderr is shown to Claude; other non-zero exits do not block | docs, exit code 2 behavior per event |
| V3 | verified_fact | Default command timeout is 600 s; a timed-out PreToolUse hook does not block | docs, common fields |
| V4 | verified_fact | All matching hooks run in parallel; how conflicting decisions combine is not documented | docs, hook handler fields |
| V5 | verified_fact | Direct edits to hooks in settings files are normally picked up by the file watcher; `disableAllHooks: true` disables all hooks | docs, configuration |
| V6 | verified_fact | A hook process inherits the parent environment; handlers run in the current directory | docs, environment |
| V7 | verified_fact | An unrecovered Go panic exits 2; `sh -c` masking turns it into exit 0 | scratchpad `panicprobe` build and run |
| V8 | verified_fact | The macOS root volume is case-insensitive APFS | `diskutil info /` |
| V9 | verified_fact | `.autopus/*-manifest.json` is untracked and `/.claude/` is ignored, so fresh worktrees carry no manifests | `git ls-files`; `.gitignore:39` |
| V10 | verified_fact | `pkg/workflow` deps exclude `pkg/qualityloop`; `pkg/qualityloop` imports only stdlib | `go list -deps ./pkg/workflow` |
| V11 | verified_fact | OpenCode v1 before-hooks filter tool `bash`; v2 filters tool `shell` | `opencode_plugin.go:125,129`; `opencode_plugin_v2.go:133,148` |
| V12 | verified_fact | `retractManagedHookEntries` drops a whole matcher entry when any of its handlers is managed | `claude_settings_hooks.go:16-46` |
| V13 | verified_fact | `WriteHookConfig` replaces the `hooks` key via `maps.Copy`; `RemoveHookConfig` deletes all of `PreToolUse`; only `autonomous_test.go` calls their wrappers | `hook_template.go:38-100`; grep |
| V14 | verified_fact | `os.Root.Link` exists in the go 1.26 toolchain | `go doc os.Root.Link` |
| V15 | verified_fact | Go `spec.ParseEARSWithWarnings` derives the declared type for 24 of 24 requirements with zero warnings | scratchpad `earscheck` run against the local module |
| V16 | verified_fact | `pkg/companionmanifest` takes a non-blocking exclusive OS file lock (`unix.Flock` LOCK_EX and LOCK_NB on darwin and linux, `windows.LockFileEx` on windows) through unexported helpers | `signed_pair_lock_unix.go:12-23`, `signed_pair_lock_windows.go:12-31` |

Consequences: PRD Q4 is resolved (deny as JSON on exit 0, exit 2 never used). V5 contradicts PRD R9 (hook snapshot at session start), so an agent can remove the guard mid-session through `.claude/settings.json` or `disableAllHooks`. V7 is why REQ-EG-11 buffers output and always exits 0.

## Plan Intent Ledger
Source: the direct `/auto plan` ledger carried in `prd.md` (no BS file). Cells are untrusted evidence and are summarized, not copied.

| Field | Status | Source | Confidence | Decision / Assumption | If Wrong | Plan Handoff |
|---|---|---|---|---|---|---|
| goal | answered | user D2 | high | deny with reason and unlock command; fail open | - | Outcome Lock |
| scope_boundary | assumed | planner | medium | no Bash write detection, no configurable paths in v1 | Bash tampering stays visible only through the lock verdict | non-goals, Reviewer Brief |
| constraints | answered | user and repo policy | high | 300 lines per file, 85% coverage, canonical source plus regenerate | - | Tasks, Reference Discipline |
| done_evidence | assumed | planner | medium | real-payload contract tests plus corpora | reviewers need live probes | CE-1 to CE-6, A1 to A3 |
| brownfield_impact | answered | exploration | high | adapters, hooks.go, three surface lists, templates | a missed writer duplicates or orphans entries | Reviewer focus, T9, T12 |
| lock_ttl | assumed | PRD Q5 | medium | 24 h default with `--ttl` | too short lapses mid-fix; too long leaves stale locks | REQ-EG-22 |
| edit_guard_default | assumed | PRD Q6 | medium | `hooks.edit_guard: true` | opt-in lowers coverage, not correctness | REQ-EG-12 |
| command_names | assumed | PRD Q7 | medium | `auto guard edit`, `auto fix lock/unlock` | a rename touches hooks and templates | REQ-EG-02, REQ-EG-06 |

## Question Audit
- question_transport: AskUserQuestion
- question_count: 1 for this SPEC (D2); 3 in the planning session (D1 to D3)
- unresolved_fields: [scope_boundary, done_evidence], both `assumed`; neither blocks the Outcome Lock.

## Outcome Lock
- User-visible outcome: where a platform hook can block, an agent edit of an `always` manifest file inside the guard namespace, of a locked `/auto fix` reproduction test, or of guard state is denied with an actionable reason; every other edit proceeds, and a guard fault never makes a decision stricter. Claude Code and OpenCode must reach `enforced`.
- Mandatory requirements: REQ-EG-01 to REQ-EG-19 and REQ-EG-21.
- Explicit non-goals: Bash write detection, configurable protected paths, native Antigravity and OMP blocking, changes to `auto rules fire` or `auto check`, a sandbox, detecting a change reverted before unlock, `.git/hooks/*` manifest entries, cross-checkout locks.
- Completion evidence: CE-1 per-platform contract tests from real payload fixtures (S1 to S9, S11, S16); CE-2 legitimate corpus 0 deny and protected corpus 100% deny (S1 to S3); CE-3 fault corpora behave as specified (S7, S16); CE-4 unlock verdict and batch fixtures (S6, S17); CE-5 drift-clean regeneration, unchanged consumer verdicts, coverage at least 85% (S10, S11, S13); CE-6 published enforcement matrix with A1 and A2 PASS (S12).

## Visual Planning Brief
The sequence diagram (payload to wrapper to `auto guard edit` to decision), the lock lifecycle state diagram, and the generation command flow are in plan.md `Visual Planning Brief`. Stage order: env bypass, decode, root resolution, guard state, lock, manifest, allow. UX wireframe gate: not applicable (CLI and hook surface only).

## Design Decisions
- Namespace is the unified table's `generated` category; manifest `always` is the "regenerated" oracle, and any `merge` or `marker` listing wins, because a false deny costs more than a missed deny (PRD anti-goal).
- Fault scope: a call-level fault allows the call; any other fault drops exactly the untrustworthy part (one corrupt record, the lock stage for unusable lock state, the root's manifest stage for any manifest fault because that file may hold the overriding `merge` entry, one malformed target). Expired locks are normal state. This resolves PRD FR-09 versus FR-10.
- Allow is silence, never `permissionDecision: allow` (V1). The command line buffers output and forwards it only after exit 0 (V7), and OpenCode applies the same clean-exit rule.
- File identity is checked at decision time with `os.SameFile`, which is portable and covers live aliases but not a hardlink left after the recorded path is deleted (documented limitation).
- Every lock or unlock command holds one exclusive store lock (the moved V16 helper, released by the OS if the holder dies), so batches serialize and a rollback can never undo another call's success; records publish by `os.Root.Link` of a complete temp file, and unlock computes verdicts before removing. PRD Q8: exit 3 for any verdict but `unchanged`.
- Unlock instructions carry the original path POSIX-quoted after `--`, so a file named `--all` cannot become a flag, and fall back to FL-X with `auto fix lock --list --json` for unsafe paths and for any reason that would pass 1024 bytes.

## Minimality Decision Matrix
| Ladder step | Evidence | Decision | Receipt item |
|---|---|---|---|
| actual need | Outcome Lock (a) to (c), D2; no deterministic Edit control today (`fire.go` advisory, drift gate at commit time) | proceed | guard and lock CLI |
| existing code/helper/pattern | `rulecond.ConditionSubject`; `openStateRoot`, `descendStateComponent`; `managedClaudeHookCommandPrefixes`; `generateCLIHooks`; `lockSignedPairFile` for the store lock (V16); the three surface member lists become one table in `pkg/workflow`, not a fourth list | reuse | T1, T4, T6, T8, T9 |
| stdlib/native | `crypto/sha256`, `os.Root` (`Link`), `os.SameFile`, `encoding/json`, `path/filepath`, `io.LimitReader`; POSIX shell for the command line | use | T2 to T8 |
| existing dependency | cobra CLI and go 1.26 toolchain; OpenCode plugin stays dependency-free JS | reuse | T7, T10 |
| new dependency or new abstraction | no new dependency; one package `pkg/editguard` (decision logic outside cobra, as `pkg/rulecond` does for rules); one exported helper `rulecond.OpenRuntimeStateDir`; `pkg/oslock` is a move of the V16 helpers, not new code | accepted, bounded | T2 to T7 |
| minimum sufficient verification | hermetic tables S1 to S8, S10, S16, S17; node test S9 with captured fixtures; writer fixtures S11, S13; probes A1 to A3; benchmark S14; security, validation, data-loss, deterministic-oracle, and generated-surface-hygiene gates unchanged | required checks | T0, T15 |

## Semantic Invariant Inventory
| ID | source clause | invariant type | affected outputs | acceptance IDs |
|---|---|---|---|---|
| INV-EG-01 | "manifest-registered generated policy files (bounded by GeneratedSurfacePrefixes)" | set intersection after path normalization | per-target deny or allow | S1, S2, S3 |
| INV-EG-02 | "deny on match" for locks, generated files, guard state | ordering (precedence) | reason class | S1, S5 |
| INV-EG-03 | "repro-test lock, runtime state under .autopus/runtime/" | state transition and alias matching | deny or allow, list state | S4, S5, S15, S17 |
| INV-EG-04 | "SHA-256 tamper detection" | formula (hash equality at unlock) | unlock verdict JSON and exit status | S6, S17 |
| INV-EG-05 | "fail-open on internal error" | fault-scope mapping (call allows, the untrustworthy part drops) | exit status, stdout bytes, plugin resolve | S7, S9, S16 |
| INV-EG-06 | "actionable reason" in the platform deny contract | parser and report encoding, quoted argument | stdout JSON, reason text | S1, S4, S8 |
| INV-EG-07 | "unify the two generated-surface lists" | set equality (parity per consumer) | drift-gate, qualityloop, hygiene verdicts | S10 |
| INV-EG-08 | "hook stdin is untrusted; redact" | sanitization formula | reason and stderr text | S8 |
| INV-EG-09 | managed-entry idempotence | deduplication at handler granularity | settings.json bytes | S11 |
| INV-EG-10 | "platform-equivalent" hooks | platform-to-state mapping | matrix rows, doctor output | S9, S12 |
| INV-EG-11 | "/auto fix repro-test lock" workflow | ordering of workflow steps | generated workflow text | S13 |
| INV-EG-12 | "auto fix lock/unlock" lifecycle | serializable transitions: all-or-nothing lock validation and publish, resumable unlock after a removal I/O error | lock listing, exit status | S17 |

## Feature Coverage Map
| Outcome slice | Covered by | Status |
|---|---|---|
| Happy path: generated deny, legitimate allow | REQ-EG-02 to 05; S1 to S3 | covered |
| Happy path: lock, deny, unlock, allow | REQ-EG-06 to 08, 21; S4 to S6, S17 | covered |
| Error and recovery: malformed stdin, crash after output, stage faults | REQ-EG-10, 11, 18; S7, S16 | covered |
| Error and recovery: stale lock, TTL, env bypass | REQ-EG-20, 22; S15 | covered (Should) |
| Integration boundary: Claude Code contract and writers | REQ-EG-11, 12, 19; V1 to V6; A1; S11, S12 | covered, live probe pending |
| Integration boundary: OpenCode native host | REQ-EG-13; A2 on the T0 spike; S9, S12 | covered, live probe pending |
| Integration boundary: Codex, Gemini CLI | REQ-EG-14; A3, T11; S12 | covered, probe-gated |
| CLI surface and verification | `auto guard edit`, `auto fix lock/unlock`; T15; S1 to S17 | covered |
| Docs and ops: capability matrix, doctor, `/auto fix` workflow | REQ-EG-15, 16, 23; S12, S13, S15 | covered |

## Completion Debt
| Item | Blocks | Required resolution |
|---|---|---|
| CD-1 probes A1 to A3 and the T11 Gemini probe | REQ-EG-11 to 14, CE-1, CE-6 | A1 and A2 must PASS (or the user re-approves the scope); A3 and T11 FAIL become `none`. Closed 2026-10-07: A1, A2 on the V2 plugin API, A3, and T11 PASS; for the unprobed V1 plugin the operator chose option (a): OpenCode V2 `enforced`, OpenCode 1.x V1 `host-unverified` |
| CD-2 Claude contract check | PRD Q4 | closed by V1 to V6; re-confirmed live by A1 |
| CD-3 enforcement matrix | CE-6 | T14 |
| CD-4 canonical auto-fix sources regenerated with zero drift | REQ-EG-15 | T13 |
| CD-5 legacy worker hook writer and remover | REQ-EG-19 | T12 handler-level fix with setup and cleanup tests |
| CD-6 four-consumer parity test | REQ-EG-01 | T1 |

## Evolution Ideas
These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|---|---|---|
| Bash write-target detection, or change-time stamps to catch a change reverted before unlock | non-goals; the lock verdict and the drift gate cover the outcome | user asks for stronger tamper evidence |
| persisted file identity so a hardlink left after deleting the locked path stays blocked | path match already blocks recreating the locked path | user asks for it |
| a `source` field in manifests for an exact canonical file; user-configurable protected paths | the reason names the source roots; non-goal | explicit request |
| human-confirmed unlock (TTY or out of band) | needs a non-agent completion path | explicit request |
| deny telemetry for eval corpora or sigma-band monitoring; session-scoped locks | enforcement works without them | user promotes them |
| `.git/hooks/*` entries in the namespace; content-aware deny of guard removal in hook settings; PRD P2 items | outside the PRD bound or user-owned files | user promotes them |

## Sibling SPEC Decision
| Decision | Reason | Sibling SPEC IDs |
|---|---|---|
| none | The Primary SPEC closes the Outcome Lock. The PRD rejected platform-wiring, guard-versus-lock, and classifier splits because they share the command, payload parsing, deny contracts, and fail-open behavior; 16 tasks and about 26 to 34 files stay below 25 and 40 | None |

## Prompt Layer Manifest Contract
The `/auto fix` template edits (T13) are prompt-state work under `content/skills/agent-pipeline.md` Prompt Layer Discipline and `pkg/promptlayer` (`KindStable`, `KindSnapshot`).
- stable: generated `/auto fix` workflow bodies for Claude, Codex (skill and prompt, reused by OpenCode), and Gemini; only T13 regeneration changes them, observable as changed per-file checksums in `.autopus/<platform>-manifest.json`, with no content exposed.
- snapshot: unchanged; no frozen recall or SPEC snapshot is touched.
- ephemeral: deny reasons and `auto fix unlock --json` verdicts, per tool call, project-relative paths only, never written into a stable layer.

## Review Resolution
Round 1 (codex and gemini; claude lane and judge failed for an environment reason) and round 2 (all providers and judge). Rows P4-R2 are the Phase 4 fix round 2 findings of the security audit and the correctness review (2026-10-07); round 3 (2026-10-07) fixed the two limits round 2 had documented, in P4-R2-2 and P4-R2-5, and round 4 (2026-10-07) the two host path transforms the next security audit found, in P4-R2-2 and P4-R4-1. CD-1 (OpenCode V1 lane) was closed by the operator decision of 2026-10-07, option (a); see Completion Debt. No finding was factually wrong; F-005 overstated current reachability but is fixed anyway.

| Finding | Resolution | Where |
|---|---|---|
| F-001 deny bytes then crash | fixed: the command line forwards buffered stdout only after exit 0 and always exits 0; one write right before exit | REQ-EG-11, contract, S7 |
| F-002 fault scope conflicts | fixed: call-level versus stage-level faults, manifest fault drops the root's manifest stage, expired lock is normal | REQ-EG-10, 18, 22, S16; round 2: REQ-EG-18 and the contract now name the dropped part (record, stage, or target), matching S16 row 3 |
| F-003 droppable mandatory lanes | fixed: only Codex and Gemini CLI are conditional; A1 and A2 FAIL block completion; A1 PASS needs the deny next to a user allow hook | T0, CD-1, A1, S12 |
| F-004 OpenCode evidence | fixed: native-host probe A2, captured per-version fixtures, payload-asserting stub plus real guard, later target and move destination, v1 `bash` versus v2 `shell` (V11); round 2: T0 builds a throwaway plugin and settings spike that A1 and A2 probe, A2 saves host-native event fixtures, and T10 implements the verified seam | REQ-EG-13, T0, T10, A2, S9 |
| F-005 destructive legacy writer | fixed: handler-level merge and remove for setup and cleanup; partial rebuttal: no production caller today (V13) | REQ-EG-19, T12, S11 |
| F-006 mixed-entry retraction | fixed: handler-level retraction with a mixed-entry fixture (V12) | REQ-EG-19, T9, S11 |
| F-007 status hygiene set | fixed: hygiene members in the table; four-consumer parity with 13 probe paths | REQ-EG-01, T1, S10 |
| F-008 EARS body grammar | fixed: bodies use the parser grammar; Go parser confirms 24 of 24 (V15) | spec.md Requirements |
| F-009 batch and concurrency | fixed: pre-validation, `os.Root.Link` exclusive publish, rollback, verdict-first idempotent unlock, `unverifiable`; round 2: one exclusive store lock serializes lock and unlock (V16), INV-EG-12 states all-or-nothing lock and resumable unlock, S17 adds paused interleavings | REQ-EG-06, 08, T4, S17 |
| F-010 unlock argument | fixed: quoted original path, FL-X fallback to `auto fix lock --list --json`; round 2: `--` before the argument and FL-X for any reason over 1024 bytes | contract, REQ-EG-17, S4, S8, S13 |
| F-011 Should inside Must | fixed: REQ-EG-21 promoted (FL-X depends on it); TTL, env bypass, doctor moved to Should S15 | spec.md, S5, S7, S12, S15 |
| F-012 tamper guarantee | fixed: guarantee limited to changes present at unlock; revert-before-unlock and post-deletion hardlink limits documented | Purpose, Reviewer Brief |
| F-013 spawn overhead (deferred) | acknowledged: REQ-EG-24 and S14 measure the guard process only | REQ-EG-24, S14 |
| F-014 fixed path in S8 (deferred) | fixed: R is a temp dir ending in `alice/secret-project`; the oracle checks for R's absolute path | S8 |
| P4-R2-1 `..` after a symlink on a lexical host (regression of 1111f2b7) | fixed: a path with `..` is judged where the kernel walk leads and at its lexical clean, deny if either is protected; the kernel cases stay denied | REQ-EG-05 |
| P4-R2-2 Gemini CLI path transforms | fixed: NUL bytes and a leading `@` removed, `file://` converted, percent-escapes decoded as Gemini CLI 0.52.0 does, raw and normalized both judged; an undecodable spelling stays raw. Round 3 fixed the `replace` `correctPath` search that round 2 documented as a limit (bundle `chunk-7LQRUKPT.js:289558`; `write_file` never searches): without a workspace walk, the guard checks the active locks, generated manifest entries, and guard-state files of every root enclosing the literal path, file name whole and the rest a string suffix, and denies any match, a path several files end with included (fail-closed); a root that does not enclose the literal path is not searched. Round 4 also judges an absolute `replace` path decoded before its `..` is resolved, as `resolveToRealPath` writes it (bundle `:307921`, `:308375`; `write_file` and a relative `replace` resolve first), and `docs/edit-guard.md` recommends launching Gemini CLI from the nearest project root, since a file-name-only `replace` from a parent root does not search a submodule's locks (N3) | REQ-EG-05, `docs/edit-guard.md`, `pkg/editguard/search.go`, `dialect_gemini_paths.go` |
| P4-R2-3 root `autopus.yaml` deleted or moved by a patch | fixed: a Delete File or either end of a Move to that names a root's `autopus.yaml` is GST (Codex `apply_patch`, OpenCode payload `displaced`); an edit in place stays allowed | REQ-EG-09, REQ-EG-13, contract |
| P4-R2-4 registration wording | fixed: hook generation registers the guard on the `enforced` and `host-unverified` lanes | REQ-EG-16, `docs/edit-guard.md` |
| P4-R2-5 Cherokee case pairs | documented in round 2; round 3 fixed it: after the x/text fold each Cherokee letter maps to its uppercase, the form Unicode case folding gives both letters, so all 86 pairs share one key and a case variant of a deleted locked test is denied | REQ-EG-05, `pkg/editguard/resolve.go` `FoldKey`; `docs/edit-guard.md` Limitations entry removed |
| P4-R2-6 SPEC text drift | fixed: 64 MiB cap with target-field decoding, FL-X for any character outside `[A-Za-z0-9._/@+-]`, the `host-unverified` lane, S7 stderr scope, REQ-EG-05, 06, 09, 23 | spec.md, acceptance.md, plan.md |
| P4-R4-1 Codex patch path TAB and CR | fixed: Codex 0.160.0 removes every TAB and CR from an Add, Update, Delete, or Move to path once the line has matched its marker, and keeps VT, FF, NBSP, ZWSP, BOM, ESC, and DEL (differential run of `codex --codex-run-as-apply-patch`); each path is judged as sent and stripped, and a delete or move displaces both | REQ-EG-05, `pkg/editguard/dialect.go`, `docs/edit-guard.md` |

## Reviewer Brief
- Intended scope: the Outcome Lock above, delivered by REQ-EG-01 to REQ-EG-24; explicit non-goals as listed there (no Bash detection, configurable paths, or sandbox).
- Not a sandbox. Documented limits (REQ-EG-16): Bash writes bypass the hook; the agent can run `auto fix unlock` itself because `Bash(auto *)` is auto-approved; the agent can edit `.claude/settings.json` or set `disableAllHooks`, applied mid-session (V5); SHA-256 at unlock reports only changes still present then, so a weaken-then-restore sequence reads `unchanged`; a hardlink that outlives the deleted locked path is not matched. The commit-time drift gate stays the backstop for generated files.
- Self-verified: Traceability Matrix, Semantic Invariant Inventory, oracle acceptance with exact bytes and hashes, existing versus [NEW] references, the Claude contract (V1 to V6), panic exit status (V7), parser types (V15).
- Reviewer focus: correctness of the INV-EG-01 to INV-EG-12 oracles, fail-open completeness, regression risk in the three surface consumers, the hook writers, and the OpenCode shell hooks, and Completion Debt only.

## Self-Verify Summary
- Q-CORR-03 | PASS | 3 | spec.md | round 1 FAIL (EG-R08): bodies now use parser grammar; Go ParseEARS derives 24 of 24 declared types (V15)
- Q-CORR-04 | PASS | 3 | research.md, spec.md, plan.md | refs re-read 2026-10-06 incl. status_hygiene, opencode filters, worker writer; planned items carry [NEW]
- Q-COMP-02 | PASS | 4 | spec.md, plan.md, research.md | round 2 FAIL: INV-EG-12 now says all-or-nothing lock and resumable unlock, the same contract as REQ-EG-06, REQ-EG-08, and S17
- Q-COMP-03 | PASS | 3 | spec.md, plan.md, acceptance.md | round 1 FAIL (EG-R02): fault scope fixed per call and per stage, pinned by S16
- Q-COMP-04 | PASS | 3 | spec.md, plan.md, research.md | round 1 FAIL (EG-R03/04): Claude and OpenCode mandatory, native-host A2, FAIL blocks completion
- Q-COMP-05 | PASS | 4 | acceptance.md, spec.md | round 2 FAIL: S17 adds rollback paused against another call's success and unlock paused against re-lock; S8 adds a `--all` file and a 221-byte quote-heavy path
- Q-COMP-06 | PASS | 3 | spec.md, research.md | Traceability Matrix covers all 24 requirements; Reviewer Brief bounds scope and limits
- Q-COMP-07 | PASS | 3 | research.md | Completion Debt CD-1 to CD-6 is separate; Evolution Ideas carry no IDs or tasks
- Q-COMP-08 | PASS | 3 | plan.md | three probe rows not-run with reasons and explicit PASS and FAIL criteria; statements tagged RI, IA, VF
- Q-FEAS-03 | PASS | 4 | plan.md, acceptance.md | round 2 FAIL: T0 builds the plugin and settings spike that A1 and A2 probe, so T10 no longer precedes its own probe
- Q-COH-02 | PASS | 3 | plan.md, research.md | round 1 FAIL (EG-R03): FAIL to `none` applies only to Codex and Gemini CLI
- Q-SEC-01, Q-SEC-02 | PASS | 3 | spec.md, acceptance.md | untrusted stdin and paths (REQ-EG-17, S7, S8, S16); redaction, symlink-refusing state, forged entries ignored

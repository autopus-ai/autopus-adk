# SPEC-EDITGUARD-001 Acceptance Criteria

## Fixtures

- `R`: temp consumer project with `autopus.yaml`; `.autopus/claude-code-manifest.json` lists `.claude/skills/auto-fix/SKILL.md` (always), `.claude/settings.json` (merge), `CLAUDE.md` (marker), `pkg/main.go` (always, forged), `.autopus/brainstorms/BS-001.md` (always, forged), `.agents/skills/x/SKILL.md` (always); `.autopus/opencode-manifest.json` lists `.agents/skills/x/SKILL.md` (merge). Every listed file exists.
- `T`: `internal/foo/foo_repro_test.go` with content `package foo\n` (SHA-256 `1b63a92736f126a00f521c0ef804e67d0cf949b5ff790d6d4c3a4b7681da8d21`, "the T hash").
- `P(tool, path)`: payload `{"session_id":"s1","cwd":"<R>","hook_event_name":"PreToolUse","tool_name":"<tool>","tool_input":{"file_path":"<path>"}}`. `DENY(id)`: the exact claude-code deny JSON of spec.md Decision Output Contract with reason `id` for the payload's path. `EMPTY`: exit 0 and zero stdout bytes.
- Command under test: `auto guard edit --platform claude-code`, cwd `R`, `AUTOPUS_EDIT_GUARD` unset, unless a scenario names the registered command line.

## Test Scenarios

### S1: Decision table over heterogeneous targets
Priority: Must
Given fixture R
When each payload of the table is sent on stdin
Then every call exits 0 and stdout equals the expected stdout byte for byte

| # | payloads | expected stdout |
|---|---|---|
| 1 | P(Edit, .claude/skills/auto-fix/SKILL.md) | DENY(GS-CON), exact bytes in the notes |
| 2 | P(MultiEdit, absolute path of row 1) | same bytes as row 1 |
| 3 | P(Edit, .claude/settings.json); P(Edit, CLAUDE.md); P(Write, .claude/commands/my-cmd.md), listed in no manifest | EMPTY each |
| 4 | P(Write, .autopus/brainstorms/BS-001.md); P(Write, .autopus/specs/SPEC-X-001/spec.md); P(Edit, pkg/main.go) with its forged entry | EMPTY each |
| 5 | P(Edit, .agents/skills/x/SKILL.md), always plus merge; P(Edit, config.toml); P(Write, .claude/skills/new/SKILL.md), new file | EMPTY each |
| 6 | `{"tool_name":"Bash","tool_input":{"command":"ls"}}` | EMPTY |

And the protected corpus (every distinct in-namespace path the five repository manifests list as always and none lists as merge or marker, times Edit, Write, MultiEdit) yields 100% deny
And the five `.git/hooks/*` always entries yield EMPTY.

### S2: Normalized and aliased paths reach the same verdict
Priority: Must
Given fixture R on the case-insensitive APFS dev volume and a symlink `<R>/alias-skills` pointing to `.claude/skills`
When P(Edit, X) is sent for X = `pkg/../.claude/skills/auto-fix/SKILL.md`, `./.claude//skills/auto-fix/SKILL.md`, `alias-skills/auto-fix/SKILL.md`, and `.CLAUDE/Skills/auto-fix/SKILL.md`
Then each stdout equals S1 row 1 byte for byte, with `{path}` = `.claude/skills/auto-fix/SKILL.md`
And on a case-sensitive volume (Linux CI) X = `.CLAUDE/Skills/auto-fix/SKILL.md` yields EMPTY
And X = `<R>/pkg/../../outside.txt`, which has no `autopus.yaml` above it, yields EMPTY.

### S3: The nearest project root decides
Priority: Must
Given workspace `W` with `autopus.yaml`, submodule `W/M` with `autopus.yaml`, the S1 claude manifest, and the markers `content/`, `templates/`, `cmd/generate-templates/`, and worktree `W/.claude/worktrees/agent-x/` with `autopus.yaml` and no manifests
When the guard runs with cwd `W` for P(Edit, `W/M/.claude/skills/auto-fix/SKILL.md`), P(Edit, `W/M/pkg/foo.go`), P(Edit, `W/.claude/worktrees/agent-x/pkg/foo.go`), and P(Edit, `W/.claude/worktrees/agent-x/.claude/skills/auto-fix/SKILL.md`)
Then the first prints the deny JSON whose reason is exactly `autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md is generated (manifest .autopus/claude-code-manifest.json, policy always). Change the canonical source (content/, templates/, pkg/adapter/) and run: make generate-templates && auto update`
And the other three yield EMPTY.

### S4: A lock denies every alias of the reproduction test
Priority: Must
Given fixture R with T, a hardlink `internal/foo/hl_test.go` and a symlink `t-link_test.go` to T, and `auto fix lock internal/foo/foo_repro_test.go` exited 0
When P(Edit, X) is sent for X = T's relative path, its absolute path, `internal/bar/../foo/foo_repro_test.go`, `t-link_test.go`, `internal/foo/hl_test.go`, and `INTERNAL/foo/foo_repro_test.go` on the case-insensitive volume
Then each stdout is the deny JSON whose reason is exactly `autopus edit-guard [fix_lock]: internal/foo/foo_repro_test.go is the locked reproduction test of an in-progress /auto fix. Fix the code under test instead. If the test itself is wrong, stop and ask the user to run: auto fix unlock -- 'internal/foo/foo_repro_test.go'`
And P(Edit, `internal/foo/foo.go`) yields EMPTY
And after `rm internal/foo/foo_repro_test.go`, P(Write, `internal/foo/foo_repro_test.go`) still yields the same deny JSON.

### S5: Precedence, guard state, and the unlock transition
Priority: Must
Given the S4 lock and a second lock on `.claude/skills/auto-fix/SKILL.md`
When P(Edit, `.autopus/runtime/fix-locks/any.json`), P(Edit, `.autopus/claude-code-manifest.json`), and P(Edit, `.claude/skills/auto-fix/SKILL.md`) are sent
Then the first two print the deny JSON with reason `autopus edit-guard [guard_state]: <path> is edit-guard state. Use auto fix lock, auto fix unlock, or auto update instead.` and the third prints class `[fix_lock]`, not `[generated_surface]`
And after `auto fix unlock internal/foo/foo_repro_test.go`, P(Edit, T) yields EMPTY.

### S6: SHA-256 verdicts and laundering
Priority: Must
Given T locked by `auto fix lock`
When Bash rewrites T to `package foo // weakened\n` (SHA-256 `e4303e1b170ed51f64829282b577efdb87a5334d86f2b07f9185084fea2f6b96`), `auto fix lock` runs again on T, and `auto fix unlock internal/foo/foo_repro_test.go --json` runs
Then the unlock exits 3 with expected stdout `{"schema":"autopus.fix_unlock.v1","results":[{"path":"internal/foo/foo_repro_test.go","verdict":"modified","locked_sha256":"1b63a92736f126a00f521c0ef804e67d0cf949b5ff790d6d4c3a4b7681da8d21","current_sha256":"e4303e1b170ed51f64829282b577efdb87a5334d86f2b07f9185084fea2f6b96"}]}`
And an untouched locked file unlocks with `"verdict":"unchanged"` and exit 0, and a deleted one with `"verdict":"missing"`, `"current_sha256":""`, and exit 3
And `auto fix lock pkg`, `auto fix lock ../outside_test.go`, and `auto fix lock missing_test.go` each exit 1 and add no record.

### S7: Call-level faults and the exit-masking command line
Priority: Must
Given fixture R, where S1 row 1 would deny
When the guard, or the registered command line run through `sh -c`, handles each case of the table
Then each run exits 0 with EMPTY stdout and at most one stderr line

| case | input |
|---|---|
| bad stdin | `{"tool_input":`; zero bytes; 1048577 bytes of `a` |
| no extractable target | `{"tool_name":"Edit","tool_input":{}}`; `{"tool_name":"Edit","tool_input":{"file_path":42}}` |
| recovered panic | S1 row 1 with the [NEW] test-only panic seam inside the decision |
| crash after output | command line with a stub `auto` that prints S1 row 1 deny bytes and exits 2; the same stub exiting 1 |
| unrecovered panic | command line with a build that panics outside the recover barrier (bare exit status 2) |
| version skew | command line with an `auto` binary that lacks the `guard` command |

And the guard binary built without fault seams exits 0, never 2, for every stdin input of S1, S7, and S16.

### S8: Reasons are sanitized and unlock arguments stay exact
Priority: Must
Given R in a temp dir whose absolute path ends in `alice/secret-project`, an always entry `.claude/skills/evil<ESC>[31m<LF>name/SKILL.md` that exists, an always entry whose path is 5000 bytes long, and locks on `internal/foo/my repro_test.go`, `internal/foo/it's_test.go`, `internal/foo/b<ESC>d_test.go`, the root file `--all`, and the 221-byte path made of `internal/foo/`, 200 single quotes, and `_test.go`
When P(Edit) targets each by absolute path and a corrupt manifest triggers a diagnostic
Then the first deny reason contains `.claude/skills/evil[31mname/SKILL.md`, no byte below 0x20, and no occurrence of R's absolute path
And the long-path reason echoes a path of exactly 256 bytes ending in `...`, stays within 1024 bytes, and still ends with `then run: auto update`
And the lock reasons end with `auto fix unlock -- 'internal/foo/my repro_test.go'`, `auto fix unlock -- 'internal/foo/it'\''s_test.go'`, and `auto fix unlock -- '--all'`
And running exactly `auto fix unlock -- '--all'` through `sh -c` removes only the `--all` lock while the other four locks stay listed
And the control-character path and the 221-byte single-quote path (whose quoted FL reason would be 1248 bytes) both end with the FL-X sentence `If the test itself is wrong, stop and ask the user to find the path with auto fix lock --list --json and unlock it.` within 1024 bytes
And the corrupt-manifest stderr is exactly `autopus edit-guard: allow (manifest unreadable: .autopus/claude-code-manifest.json)`.

### S9: OpenCode plugins use native payloads and deny only on a clean exit
Priority: Must
Given the regenerated v1 and v2 plugins under node with a fake ctx, the per-version host-native event fixtures that A2 saved from the T0 spike plugin, and on PATH either a recording stub or the real guard binary as `auto`
When each fixture's file-editing call runs, including a two-file patch whose second target is `.claude/skills/auto-fix/SKILL.md` and a move whose destination is that path
Then the recording stub receives exactly `{"platform":"opencode","cwd":"<R>","tool_name":"<native tool>","targets":[...]}` with every target in fixture order, the second target and the move destination included
And with the real guard those calls reject with an Error whose message is the GS-CON reason for `.claude/skills/auto-fix/SKILL.md`, while a call that edits only `pkg/foo.go` resolves
And stubs that (a) print `{"decision":"deny","reason":"R1"}` and exit 0, (b) print that line and exit 2, (c) print nothing and exit 0, (d) sleep past the hook timeout, or (e) are absent give an Error with message exactly `R1` for (a) and resolve for (b) to (e)
And a v1 `bash` call and a v2 `shell` call still run the existing pre-commit and react hooks without the guard, and file-editing calls run no shell-tool hook.

### S10: The unified table preserves all four current verdict sets
Priority: Must
Given the probe paths `.claude/x`, `.agents/skills/x`, `.agents/plugins/marketplace.json`, `a/.codex/x`, `.autopus/runtime/x`, `.omp/x`, `.autopus/specs/x`, `sub/config.toml`, `x/plugins/cache/y`, `.autopus/claude-code-manifest.json`, `.autopus/backup/x`, `.mcp.json`, `.agents/hooks.json`
When `hasGeneratedPrefix`, `isGeneratedSurfacePath`, `isRuntimeUnignoredRisk`, and the guard namespace test evaluate them after T1
Then the drift-gate verdicts are exactly [T, F, T, F, F, F, F, F, F, T, F, F, F]
And the qualityloop verdicts are exactly [T, T, T, T, T, F, F, T, T, T, F, F, T]
And the status-hygiene verdicts are exactly [T, T, T, F, T, F, F, F, F, T, T, T, T]
And the guard namespace verdicts are exactly [T, T, T, F, F, T, F, F, F, F, F, F, T]
And `GeneratedSurfacePrefixes`, `GeneratedSurfaceExactPaths`, `runtimeUnignoredExtraPrefixes`, and `runtimeUnignoredExtraExactPaths` keep their current members and order.

### S11: Hook writers own single handlers
Priority: Must
Given `.claude/settings.json` whose PreToolUse holds the user entry `{"matcher":"Edit","hooks":[{"type":"command","command":"./my-hook.sh"}]}` and a mixed entry with matcher `Edit|Write|MultiEdit` whose handlers are `./mixed-hook.sh` and then the guard handler, with `hooks.edit_guard` unset
When `auto update` runs twice
Then both runs leave identical bytes in which the guard handler appears exactly once, alone in its own entry with matcher `Edit|Write|MultiEdit`, `"timeout": 5`, and the canonical command line of the Decision Output Contract
And the mixed entry keeps its matcher with only `./mixed-hook.sh`, the `./my-hook.sh` entry is unchanged, and the `auto check --hygiene` Bash entry and the `auto rules fire` dispatcher entry are still present
And after `hooks.edit_guard: false` and `auto update`, no handler contains `auto guard edit` while both user handlers remain
And `SetupAutonomousMode` and then `CleanupAutonomousMode` on that file leave every other handler identical as parsed JSON to the state before setup.

### S12: Mandatory lanes are enforced and conditional lanes follow their probes
Priority: Must
Given A1 and A2 recorded PASS, and A3 and the T11 Gemini probe recorded PASS or FAIL in plan.md
When `auto update` regenerates every platform
Then `docs/edit-guard.md` lists exactly: Claude Code `enforced`; OpenCode `enforced`; Codex `enforced` on A3 PASS else `none`; Gemini CLI `enforced` on T11 PASS else `none`; Antigravity `advisory-only`; OMP `none`
And each `enforced` lane has one guard handler whose real-payload fixture yields its deny encoding, and each `none` lane has zero guard handlers
And an A1 or A2 FAIL leaves this scenario failing, which blocks completion until the lane is fixed or the user explicitly re-approves the scope.

### S13: The /auto fix workflow carries the lock contract
Priority: Must
Given T13 edited the four canonical templates and ran `make generate-templates` and `auto update`
When the generated Claude `/auto fix` surface, the Codex skill and prompt, and the Gemini skill are read
Then each contains these exact lines in this order: `auto fix lock -- <test-path>` after the Step 1 failing-assertion check; `Do NOT run auto fix unlock before Step 4 verification passes.`; `auto fix unlock --json -- <test-path>` in the completion step; `Lock verdict is unchanged (any other verdict: not complete, stop and ask the user)` in Pre-Completion Verification; and `lock: not applicable` for fixes without a test
And the template parity tests pass and `detectTemplateRegenDrift` reports zero drift.

### S14: Guard latency budget
Priority: Should
Given the autopus-adk repository with its five manifests
When 200 warm invocations each of S1 row 1 and the `.claude/settings.json` payload are timed as guard process wall time
Then the measured p95 is at most 150 ms for each payload, with p50 and p95 reported (explicit tolerance: p95 at most 150 ms; host spawn overhead excluded).

### S15: TTL, environment bypass, and doctor
Priority: Should
Given T locked at time t0, `internal/foo/u_test.go` locked at t0 + 24h, an expired lock on `.claude/skills/auto-fix/SKILL.md`, and the clock at t0 + 25h with the default 24 h TTL
When P(Edit) is sent for T, for `internal/foo/u_test.go`, and for S1 row 1, and `auto fix lock --list --json` runs
Then T yields EMPTY and is listed `"state":"stale"`, u_test.go yields the FL deny and is listed `"state":"active"`, and S1 row 1 still yields DENY(GS-CON)
And re-locking T replaces its record with the current hash and `"state":"active"`
And with `AUTOPUS_EDIT_GUARD=off` S1 row 1 yields EMPTY with no stderr, and `auto doctor` prints per installed platform the registration state and the matrix state of `docs/edit-guard.md`.

### S16: A fault never makes the decision stricter than dropping the faulty part
Priority: Must
Given fixture R plus T and the state of each row
When the row's payload is sent with the platform named in the row
Then stdout equals the expected value and stderr has at most one line

| state | payload | expected stdout |
|---|---|---|
| `.autopus/runtime` is a symlink to a directory holding a valid lock for the S1 row 1 file | S1 row 1 | DENY(GS-CON) |
| the same symlinked state holding a valid lock for T | P(Edit, T) | EMPTY |
| a record containing `not json` next to the valid lock of T | P(Edit, T) | DENY(FL) |
| T locked and `.autopus/claude-code-manifest.json` contains `{` | P(Edit, T) | DENY(FL) |
| `.autopus/opencode-manifest.json` contains `{` | P(Edit, .agents/skills/x/SKILL.md) | EMPTY |
| the same corrupt opencode manifest | S1 row 1 | EMPTY (manifest stage of the root skipped) |
| none, platform opencode | targets `[".claude/skills/auto-fix/SKILL.md", 42]` | opencode deny JSON with the GS-CON reason |
| none, platform opencode | targets `[42, ".claude/skills/auto-fix/SKILL.md"]` | the same opencode deny JSON |
| none, platform opencode | targets `[42]` | EMPTY |

### S17: Batch and concurrent lock transitions
Priority: Must
Given T, `internal/foo/u_test.go`, `internal/foo/x_test.go`, and no locks
When the steps of the table run in order, with paused steps driven by test seams
Then each expected result holds, observed through exit status, `auto fix lock --list --json`, and P(Edit, T)

| step | expected |
|---|---|
| `auto fix lock internal/foo/foo_repro_test.go missing_test.go` | exit 1; zero locks |
| lock T and u_test.go with the publish fault seam on the second record | exit 1; zero locks |
| A = lock u_test.go and x_test.go with the publish fault seam on x_test.go, paused after publishing u_test.go; B = `auto fix lock internal/foo/u_test.go` started meanwhile | B finishes only after A; A exits 1; B exits 0; exactly one lock, for u_test.go, created after A exited |
| 8 concurrent `auto fix lock internal/foo/foo_repro_test.go` | all exit 0; one lock for T with the T hash, `"state":"active"`, `"integrity":"unchanged"` |
| 4 concurrent pairs of rewriting T to `package foo // weakened\n` and `auto fix lock` T | T's record keeps the T hash |
| `auto fix unlock internal/foo/foo_repro_test.go not_locked_test.go` | exit 1; T still locked |
| C = `auto fix unlock internal/foo/foo_repro_test.go` paused after computing its verdict; D = `auto fix lock internal/foo/foo_repro_test.go` started meanwhile | D finishes only after C; C exits 3 with `"verdict":"modified"`; D exits 0; T is locked by a new record with SHA-256 `e4303e1b170ed51f64829282b577efdb87a5334d86f2b07f9185084fea2f6b96` |
| `auto fix unlock --all` with the removal fault seam on the second removal | exit 1; one lock still listed |
| rerun `auto fix unlock --all` after adding a record containing `not json` | exit 3; `"verdict":"unverifiable"` for that record; zero locks |
| P(Edit, T) after the exit-3 unlock | EMPTY |

## Oracle Acceptance Notes

- Every Must scenario pins concrete expected output: expected stdout bytes, expected JSON, SHA-256 values, ordered verdict lists, or exit status together with stdout and lock listings. None closes on a file or heading being present, on an exit code alone, or on non-empty output.
- S1 row 1 expected stdout, exact, plus one trailing newline: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md is generated (manifest .autopus/claude-code-manifest.json, policy always). Change autopus.yaml or the upstream Autopus source, then run: auto update"}}`
- Heterogeneous inputs: S1 mixes tools and manifest policies; S2 to S4 mix path forms; S3 mixes root kinds; S16 mixes faults with active protections; S17 mixes batch, fault-seam, paused-interleaving, and concurrent steps.
- Hermetic: S1 to S8, S10, S11, S13, S16, and S17 are Go tests with temp dirs and an injected clock; S9 runs under node with A2's captured fixtures; S12 depends on the probe rows; S14 is a benchmark.
- Untrusted input: S7, S8, and S16 feed hostile stdin and paths, and no expected output contains an absolute path outside the project.

# Edit guard

`auto guard edit` is a pre-tool hook (SPEC-EDITGUARD-001). It denies an agent's
file-editing tool call before the write when a target is:

- a generated harness file: inside the guard namespace and listed with policy
  `always` by a platform manifest (`.autopus/<platform>-manifest.json`), unless a
  manifest lists it with `merge` or `marker`;
- the reproduction test of an in-progress `/auto fix`, locked with `auto fix lock`;
- edit-guard state: `.autopus/runtime/fix-locks/**`, `.autopus/*-manifest.json`,
  and a project root's `autopus.yaml` when the call deletes it, moves it away,
  or moves a file onto it (a Codex or OpenCode patch); editing it in place is
  allowed.

Every enclosing project root decides: a target is checked against the nearest
directory with `autopus.yaml` and against every one above it, so an
`autopus.yaml` planted below a project root hides nothing. A target with `..`
is judged both where the kernel's walk leads, past every symlink before the
`..`, and at its lexically cleaned path, where hosts that clean the path before
they open it write; a deny of either denies the call.

Every other edit is allowed. The guard namespace is `.claude/`, `.codex/`,
`.gemini/`, `.opencode/`, `.agents/`, `.omp/`, and `.autopus/plugins/`, minus
`.claude/worktrees/`. The guard fails open: unreadable, empty, or oversized
input, a payload without a target, a crash, a timeout, or a missing or older
`auto` binary allows the call, and a corrupt manifest or lock record only drops
that part of the decision. A fault never makes a decision stricter.

## Enforcement matrix

`pkg/editguard/matrix.go` holds the same rows; hook generation registers the
guard exactly on the `enforced` and `host-unverified` lanes, and `auto doctor`
reports them. OpenCode
generates the V1 or the V2 plugin for the installed OpenCode major version (V1
when it cannot tell); both carry the guard, and `auto doctor` reports the lane
of the plugin it finds.

| Platform | State | Registration | Evidence |
|---|---|---|---|
| Claude Code | enforced | `.claude/settings.json` PreToolUse, matcher `Edit\|Write\|MultiEdit`, timeout 5 s | A1 PASS on Claude Code 2.1.289 |
| OpenCode | enforced | `.opencode/plugins/autopus-hooks.js` V2 plugin, `EDIT_GUARD` literal, timeout 5 s | A2 PASS on OpenCode 2.0.10 (V2 plugin API) |
| OpenCode 1.x | host-unverified | `.opencode/plugins/autopus-hooks.js` V1 plugin, `EDIT_GUARD` literal, timeout 5 s | the V1 plugin is generated with the guard, but no OpenCode 1.x host was probed (CD-1 closed by the operator decision of 2026-10-07) |
| Codex | enforced | `.codex/hooks.json` PreToolUse, matcher `apply_patch`, timeout 5 s | A3 PASS on Codex CLI 0.160.0; each patch path is judged as sent and with every TAB and CR removed, the path Codex writes; runs only after the user trusts the project hooks |
| Gemini CLI | enforced | `.gemini/settings.json` BeforeTool, matcher `^(write_file\|replace)$`, timeout 5000 ms | T11 PASS on Gemini CLI 0.52.0; the guard judges every spelling Gemini writes `file_path` as (NUL and a leading `@` removed, `file://` converted, percent-escapes decoded, and an absolute `replace` path decoded before its `..` is resolved) and repeats the `correctPath` search of `replace` over the files it protects; project hooks run only in a trusted folder |
| Antigravity | advisory-only | none: `.agents/hooks.json` gets no guard | its PreToolUse hooks run through the always-allow wrapper of `pkg/content/hooks_antigravity.go` |
| OMP | none | none | the OMP adapter has no native hooks (`SupportsHooks()` is false) |

- `enforced`: the native hook blocks a denied edit, verified by a probe against the real host.
- `host-unverified`: the guard is generated for the lane, but no probe has confirmed that the host blocks a denied edit; treat it as not enforced.
- `advisory-only`: Autopus hooks run on the platform but cannot block.
- `none`: no guard runs.

Claude Code, Codex, and Gemini CLI run the guard through this command line,
which forwards the decision only after the guard exited 0 and always exits 0,
so a crash or a timeout never blocks an edit:

```sh
out=$(auto guard edit --platform claude-code) && [ -n "$out" ] && printf '%s\n' "$out"; exit 0
```

The OpenCode plugin spawns `auto guard edit --platform opencode` without a shell
and throws only for a deny decision from a guard that exited 0.

## Reproduction test lock

| Command | Effect | Exit status |
|---|---|---|
| `auto fix lock [--ttl <duration>] -- <path>...` | records each file's SHA-256; TTL 24 h by default, 1 m to 168 h | 0 locked, 1 nothing locked |
| `auto fix lock --list [--json]` | lists each lock as `active` or `stale` with its integrity | 0 |
| `auto fix unlock [--json] -- <path>...` or `auto fix unlock --all` | computes every verdict (`unchanged`, `modified`, `missing`, `unverifiable`), then releases | 0 all unchanged, 3 any other verdict, 1 nothing released or a removal error |

Run both commands from the project root nearest to the test, the closest
directory above it with `autopus.yaml`: from an enclosing project they exit 1
and name that root. `auto fix unlock` resolves its paths against the root it
runs in, so an `autopus.yaml` planted after the lock does not detach it. The
deny reason echoes the exact unlock command only for a path of
`[A-Za-z0-9._/@+-]`; for any other path it points to `auto fix lock --list --json`.

The `/auto fix` workflow locks the reproduction test once it fails on the bug
assertion, releases it after verification, and accepts only `unchanged`; an
unlock exit 1 means the fix is not complete either.

## Limitations

The guard is not a sandbox against an agent with shell access.

- Writes through a shell tool (Bash, Codex `exec_command`, and the like) bypass the hook. The unlock verdict and the commit-time drift gate stay the backstop.
- The agent can run `auto fix unlock` itself, because `Bash(auto *)` is auto-approved. The workflow forbids it before verification; nothing enforces that.
- The agent can turn the hook off: hook settings files such as `.claude/settings.json` are `merge` files the guard allows, and a settings edit or `disableAllHooks: true` takes effect mid-session in Claude Code.
- The unlock verdict reports only changes still present at unlock, so a test weakened and then restored reads `unchanged`.
- A hard link that outlives the deleted locked path is not matched; recreating the locked path is still denied.
- Locks do not span checkouts or worktrees, and a fresh worktree has no manifests, so its generated files are not protected.
- Codex and Gemini CLI block only after the user trusts the project hooks or folder.
- Manifest `always` entries outside the namespace, such as `.git/hooks/*`, are ignored.
- The guard reads at most 64 MiB of hook input. It decodes only the target fields and skips every other value, so a large content body is decided normally, but a payload over 64 MiB is allowed.
- A manifest must be a regular file: an entry named like a manifest that is a directory, symlink, or FIFO is ignored, and paths below such a name are guard state.
- Case-insensitive volumes compare paths in their canonical caseless form (Unicode full case folding and NFD), which is how APFS matches names; a volume whose matching differs, such as NTFS, may see a rare spelling denied that it would treat as another file.
- Gemini CLI's `replace` tool swaps a relative `file_path` that names no file for the one workspace file whose path ends with it (its `correctPath` search; `write_file` writes the literal path). The guard repeats that search without walking the workspace: it denies the call when an active lock, a generated manifest entry, or a guard-state file of a project root that encloses the literal path ends with the path, its file name matching whole. A protected file of a root that does not enclose the literal path, such as a project nested elsewhere below the session directory or a directory added with `--include-directories`, is not found. Launch Gemini CLI from the nearest project root of the files it edits, in a workspace of submodules the submodule: launched from a parent root, a `replace` that names only a file name resolves its literal path in the parent root, so the guard does not search the submodule's locks while Gemini can still pick the submodule's file. A path that several files end with is denied too, although Gemini then writes the literal path: the guard sees only the files it protects.
- Gemini CLI also percent-decodes the file a `replace` search picks, so a workspace file whose own name holds an escape such as `%2e%2e` can redirect the edit to a protected path; the guard compares the literal suffix and does not follow that second decode.
- Host path parity is verified against Claude Code 2.1.289, Codex 0.160.0 (`apply_patch` drops TAB and CR from header paths), Gemini CLI 0.52.0, and OpenCode 2.0.10. The OpenCode `patch` tool's handling of TAB and CR in header paths has not been measured, and a newer host release may transform paths in ways the guard does not mirror.

## Rollback

1. Without regenerating anything, start the agent CLI with `AUTOPUS_EDIT_GUARD=off` in its environment: the guard then allows every edit with no output.
2. For the project, set `hooks.edit_guard: false` in `autopus.yaml` and run `auto update`. That removes only the guard handler from `.claude/settings.json`, `.codex/hooks.json`, and `.gemini/settings.json`, and sets the OpenCode `EDIT_GUARD` to `null`; user handlers and other Autopus hooks stay.
3. Release leftover locks with `auto fix lock --list`, then `auto fix unlock -- <path>` or `auto fix unlock --all`. Expired locks are ignored by the guard anyway.
4. Run `auto doctor`: its Edit Guard section shows, per installed platform, whether the guard is registered and the platform's matrix state; `auto doctor --json` reports the same rows as `doctor.edit_guard.<lane>` checks.

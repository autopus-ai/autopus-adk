# Edit guard

`auto guard edit` is a pre-tool hook (SPEC-EDITGUARD-001). It denies an agent's
file-editing tool call before the write when a target is:

- a generated harness file: inside the guard namespace and listed with policy
  `always` by a platform manifest (`.autopus/<platform>-manifest.json`), unless a
  manifest lists it with `merge` or `marker`;
- the reproduction test of an in-progress `/auto fix`, locked with `auto fix lock`;
- edit-guard state: `.autopus/runtime/fix-locks/**` and `.autopus/*-manifest.json`.

Every other edit is allowed. The guard namespace is `.claude/`, `.codex/`,
`.gemini/`, `.opencode/`, `.agents/`, `.omp/`, and `.autopus/plugins/`, minus
`.claude/worktrees/`. The guard fails open: unreadable, empty, or oversized
input, a payload without a target, a crash, a timeout, or a missing or older
`auto` binary allows the call, and a corrupt manifest or lock record only drops
that part of the decision. A fault never makes a decision stricter.

## Enforcement matrix

`pkg/editguard/matrix.go` holds the same rows; hook generation registers the
guard exactly on the `enforced` lanes, and `auto doctor` reports them.

| Platform | State | Registration | Evidence |
|---|---|---|---|
| Claude Code | enforced | `.claude/settings.json` PreToolUse, matcher `Edit\|Write\|MultiEdit`, timeout 5 s | A1 PASS on Claude Code 2.1.289 |
| OpenCode | enforced | `.opencode/plugins/autopus-hooks.js`, `EDIT_GUARD` literal, timeout 5 s | A2 PASS on OpenCode 2.0.10 (V2 plugin API); the V1 plugin is generated but no 1.x host was probed |
| Codex | enforced | `.codex/hooks.json` PreToolUse, matcher `apply_patch`, timeout 5 s | A3 PASS on Codex CLI 0.160.0; runs only after the user trusts the project hooks |
| Gemini CLI | enforced | `.gemini/settings.json` BeforeTool, matcher `^(write_file\|replace)$`, timeout 5000 ms | T11 PASS on Gemini CLI 0.52.0; project hooks run only in a trusted folder |
| Antigravity | advisory-only | none: `.agents/hooks.json` gets no guard | its PreToolUse hooks run through the always-allow wrapper of `pkg/content/hooks_antigravity.go` |
| OMP | none | none | the OMP adapter has no native hooks (`SupportsHooks()` is false) |

- `enforced`: the native hook blocks a denied edit, verified by a probe against the real host.
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

The `/auto fix` workflow locks the reproduction test once it fails on the bug
assertion, releases it after verification, and accepts only `unchanged`.

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

## Rollback

1. Without regenerating anything, start the agent CLI with `AUTOPUS_EDIT_GUARD=off` in its environment: the guard then allows every edit with no output.
2. For the project, set `hooks.edit_guard: false` in `autopus.yaml` and run `auto update`. That removes only the guard handler from `.claude/settings.json`, `.codex/hooks.json`, and `.gemini/settings.json`, and sets the OpenCode `EDIT_GUARD` to `null`; user handlers and other Autopus hooks stay.
3. Release leftover locks with `auto fix lock --list`, then `auto fix unlock -- <path>` or `auto fix unlock --all`. Expired locks are ignored by the guard anyway.
4. Run `auto doctor`: its Edit Guard section shows, per installed platform, whether the guard is registered and the platform's matrix state; `auto doctor --json` reports the same rows as `doctor.edit_guard.<lane>` checks.

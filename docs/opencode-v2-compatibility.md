# OpenCode V2 adapter compatibility

This local 0.50.118 candidate selects OpenCode V1 or V2 generation from a bounded
`opencode --version` probe. Library callers can pin the response with
`opencode.WithCLIVersion("2.0.10")` for reproducible offline generation. Existing
V1 tests pin 1.18.7 explicitly; the host's installed CLI no longer changes their
contract. Unknown versions use legacy V1 only when doing so cannot overwrite an
existing V2 plugin; future unsupported majors stop before generation.

## Generated behavior

- V1 retains its function plugin and task invocation contract.
- V2 emits a native plugin object with id `autopus.hooks`, registers tool execute hooks,
  and handles the native `shell` tool. It resolves the operation's actual session
  directory and optional workdir. Failure of a before hook propagates; after
  hooks use the previously verified per-call directory.
- Hook subprocess execution has a timeout and 64 KiB combined output cap.
  Errors omit arbitrary command output. Unloading disposes hook registrations
  and stops owned hook subprocesses. POSIX timeout termination targets the
  owned process group; Windows only directly terminates the child process.
- V2 instructions use `subagent` with required agent, description and prompt.
  They distinguish foreground completion from background dispatch, require the
  returned sessionID for continuation, and preserve explicit model/variant intent.
  Native tools must still be present in the runtime catalog.
- New V2 configuration uses plugins; supported existing legacy plugin arrays
  remain in their current form. Native objects and legacy tuple options are
  preserved. Native plugins takes precedence when both keys exist. Explicit
  disable controls are respected rather than silently re-enabled.
- Managed path aliases (relative, absolute and file URL) do not create duplicate
  registration. Validation checks registration, not proof of hook execution.
- User text outside the root AGENTS.md managed marker is preserved. InstallHooks
  validates the target config before writing its generated plugin.

This change does not port unrelated global plugins or repair provider credentials.
The previous OpenCode 2.0.10 model-authentication 401 remains a failed observation.
The existing `doctor agents --live --platform opencode` driver targets the V1
HTTP lifecycle; V2 generation does not claim that the V1 driver now certifies V2.

The generated file has no external SDK import. The published 2.0.10
Plugin.define helper is an identity function; the native object carries the same
contract. A real-host regression verifies loading without a manually installed
SDK, because a missing plugin dependency can otherwise leave hooks inactive.

## Verification

The source and acceptance ledger are in
[SPEC-OPENCODE-V2-001](../.autopus/specs/SPEC-OPENCODE-V2-001/acceptance.md).
Generated JS contracts, affected adapter/content race tests and the installed
2.0.10 native host check passed. In the no-SDK host check, a synthetic loopback
model triggered before -> shell -> after; an intentionally failing before hook
prevented shell execution. Both task-owned processes stopped.

The local repository also received an OpenCode-only generated-surface update
through the adapter transaction and validated without findings. This updates
Autopus-owned surfaces, not unrelated global plugins or authentication.

## Sources

- [V2 migration](https://opencode.ai/v2/docs/migrate-v1/)
- [Plugin setup and hook API](https://opencode.ai/v2/docs/build/plugins)
- [Plugin configuration](https://opencode.ai/v2/docs/plugins/)
- [Native subagent tool](https://opencode.ai/v2/docs/tools/#subagent)

The API shapes were also checked against the published @opencode/plugin,
@opencode/client and @opencode/schema 2.0.10 packages and the installed 2.0.10
subagent schema. A newer runtime still requires its own compatibility check.

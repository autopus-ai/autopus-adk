# stale_hooks fixtures (SPEC-PANERM-001 T3)

Workspaces for the hook retraction matrix (acceptance.md S11-S13, RFP-3). Each one is the hook surface of a workspace
that binary O (`auto` v0.50.123 built from `c447badc`) generated with `auto init --platforms <p> --yes`, run with a
scratch `HOME`, `PATH` holding only recording fakes for `claude`, `codex`, `agy`, `gemini` (and `opencode` where
listed), and `codex debug models` failing so no host catalog is read.

Kept per workspace: `autopus.yaml`, the platform manifest trimmed to the kept files (`generated_at` fixed to
`2026-10-07T00:00:00Z`), the settings files, and the hook scripts. Skills, agents, rules, and instruction files are
dropped. W-codex also keeps O's `.agents/plugins/marketplace.json` (merge policy): B creates that file in template key
order but rewrites an existing one in sorted order, so without it the second update of S12 would differ from the
first for a reason unrelated to the hooks. Added user content: a separate `user-*` entry before the managed ones in every settings file, a `mixed`
Stop entry (group S handler, then `./scripts/notify.sh`) in W-claude and W-codex, a `user-hooks` set in
`.agents/hooks.json`, the user scripts under `scripts/`, and `plugins/mine.ts`.

| Workspace | Platform | `opencode` on PATH | Group S members |
|-----------|----------|--------------------|-----------------|
| W-claude | claude-code | none | 7 `.claude/hooks/autopus/hook-*.sh`, legacy `hook-opencode-complete.ts`, Stop and SessionStart handlers |
| W-codex | codex | none | 2 `.codex/hooks/autopus/` scripts, Stop and SessionStart handlers |
| W-agy | antigravity-cli | none | 2 `.gemini/hooks/autopus/` scripts, `.agents/hooks.json` Stop, `.gemini/settings.json` AfterAgent |
| W-oc2 | opencode (V2 `plugins`) | `fakebin/opencode-2.0.0` | the `.ts` and its `file://{{ROOT}}/...` object entry |
| W-oc1 | opencode (legacy `plugin`) | `fakebin/opencode-1.0.0` | the `.ts`, its tuple entry, and its absolute-path entry |
| W-oc-bad | opencode (V2 with a tuple) | `fakebin/opencode-2.0.0` | none removable: `validatePluginEntries` rejects the config |
| W-mix | claude-code | none | the `.ts` stays: `opencode.json` still references it |

`{{ROOT}}` in JSON files stands for the workspace copy's absolute path; tests substitute it when they copy a fixture.
`stale_hooks_fixture_test.go` holds the integrity checks, the W-mix and W-oc-bad guards that hold at B, and the S11
retraction oracle, which is red at B and skips with a reason naming its owner task (T11) until it lands;
`stale_hooks_s12_test.go` holds the S12 idempotency and fault-injection oracles (T11).
`AUTOPUS_PANERM_RED=1 go test ./internal/cli -run TestStaleHookFixtures` runs it anyway.

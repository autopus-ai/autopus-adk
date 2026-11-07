<div align="center">

# 🐙 Autopus-ADK

### Verify what your AI agent changed by running it.

Autopus-ADK installs a verification harness into your AI coding CLI (Claude Code, Codex,
Antigravity CLI, OpenCode, Oh My Pi). Your agent still plans and writes the code. Autopus
builds it, tests it, reviews the diff, and has a separate QA agent run the product to look for bugs
before the session ends.

[![GitHub Stars](https://img.shields.io/github/stars/autopus-ai/autopus-adk?style=social)](https://github.com/autopus-ai/autopus-adk/stargazers)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Platforms](https://img.shields.io/badge/Platforms-5-orange)](#platforms)
[![Agents](https://img.shields.io/badge/Agents-4-blueviolet)](#what-gets-installed)
[![Skills](https://img.shields.io/badge/Skills-9-ff69b4)](#what-gets-installed)

```bash
# macOS / Linux
curl -sSfL https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.sh | sh

# Windows (CMD or PowerShell)
powershell -c "irm https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.ps1 | iex"
```

[Why](#why-verification-and-not-a-workflow) · [Stop hook](#the-stop-hook-build-and-test-before-the-session-ends) · [QA bug hunt](#qa-bug-hunt) · [Routes](#routes) · [Install](#install) · [Configuration](#configuration) · [Security](#security)

[🇰🇷 한국어](docs/README.ko.md)

</div>

---

## Why verification and not a workflow

Up to v0.50.x, Autopus shipped a full implementation harness: plan/SPEC documents, `/auto go`,
`/auto fix`, `/auto idea`, 17 agents, and a set of discipline rules. Frontier models (Opus-class)
now plan and implement well on their own, and that scaffolding mostly added cost.

A small pilot pointed the same way. On 4 real upstream PR tasks graded by hidden tests, plain
Claude Code passed 4/4 in about 17 minutes for $3.00, and the old full harness passed 0/4 in
about 3.6 hours for at least $37 (a lower bound: its plan stage hit the time cap on every task and
reported no cost). With n=4, all small Go changes, this is directional, not a benchmark.

What agents still do badly is check their own work. They report "done" on code that does not build,
tests they did not run, and behavior they never exercised. So the harness now does one job: it runs
the change, and it sends the agent back when the change does not hold up.

## What it does

| Stage | What runs | When |
| --- | --- | --- |
| Build and test | `auto verify stop`: the toolchains of the changed files only | Every time a Claude Code session tries to stop with a changed tree |
| Review | The `reviewer` agent on `git diff HEAD` | Automatically, after a passing build of 40+ changed source lines |
| QA bug hunt | The `qa-explorer` agent drives the product; `auto qa hunt verify` re-runs each repro | Same follow-up round, when the change has runnable behavior; or on demand with `/auto qa` |
| Fix | The working agent fixes blockers and confirmed bugs, then re-runs the tests | Same follow-up round |

None of this needs a prompt from you. The agent is told to carry the round through and to ask you
only for decisions that are really yours, as a multiple-choice question.

## The Stop hook: build and test before the session ends

`auto init` registers a Claude Code `Stop` hook, `auto verify stop`. When the agent tries to end the
session and the work tree differs from `HEAD`, the hook:

1. Plans commands only for the toolchains the changed files belong to:
   - **Go**: `go build ./...` and `go test` on the changed packages
   - **Node**: the `typecheck` and `test` scripts, run with the package manager the lockfile names
   - **Python**: `python3 -m pytest -q`
   - **Rust**: `cargo test --quiet`
2. Runs them within a total budget (`--budget`, default 8m).
3. On failure, blocks the stop and hands the agent the failing command with the last 60 lines of
   output.
4. After 3 blocks in one session, stops blocking and ends with a warning that the build is still
   failing, so a session cannot loop forever.
5. Remembers a passing tree. If nothing changed since the last pass, the next stop does not re-run.

Doc-only changes and tools that are not installed are skipped.

**Follow-up round.** When the build passes and the change has 40 or more changed source lines
(added plus deleted against `HEAD`, untracked source files included), the hook blocks once more with
instructions to:

1. run the `reviewer` agent on `git diff HEAD` and report only blockers;
2. if the change has runnable behavior, run `qa-explorer` in hunt mode and judge the findings with
   `auto qa hunt verify`;
3. fix every blocker and confirmed bug, re-run the tests, and summarize what was checked.

A session gets at most 2 follow-up rounds, and a new round starts only after another 40 lines of
change, so fixing review findings does not trigger a third.

**Opting out.**

| Scope | How |
| --- | --- |
| Don't register the hook | `hooks.verify_stop: false` in `autopus.yaml`, then `auto update` |
| Disable for one session or shell | `AUTOPUS_VERIFY_STOP=off` |
| Keep build/test, skip the follow-up round | `AUTOPUS_FOLLOWUP=off`, or `auto verify stop --followup=false` |

The Stop hook is Claude Code only. On the other platforms, run `/auto review` and `/auto qa`
(or the platform's equivalent, below) yourself.

## QA bug hunt

`/auto qa` with no subcommand, or with a plain-language request, starts the **Bug Hunt** section.
It works the way an AI playtester hunts game bugs: the `qa-explorer` agent reads the code only to
learn how to operate the product, then actually runs and drives it (CLI, server, UI, library API)
looking for failures the author did not think of.

- Every suspicion is filed as `.autopus/qa/hunt/findings/<id>/finding.json` plus `repro.sh`.
  `repro.sh` exits 1 while the bug is present and 0 once it is fixed.
- `qa-explorer` writes only under `.autopus/qa/hunt/`. It never edits product code, tests, or
  config. The author and the checker stay separate.
- `auto qa hunt verify` re-runs every repro and classifies it:

| Verdict | Meaning |
| --- | --- |
| `confirmed` | The repro fails every run. This is a bug. |
| `flaky` | The repro fails on some runs only. |
| `not_reproduced` | The repro passes. Either it never was a bug, or it is fixed. |
| `broken_repro` | The repro itself could not run correctly. |

Only `confirmed` findings count as bugs; the rest are reported as suspicions. The working agent
fixes each confirmed bug and re-runs `auto qa hunt verify` until it reports `not_reproduced`. It
never edits a repro to make it pass.

```bash
auto qa hunt verify                       # re-run every finding (2 runs each by default)
auto qa hunt verify --id <id> --runs 3    # one finding, more runs
auto qa hunt verify --json --fail-on-confirmed   # CI: exit 1 when any bug is confirmed
```

Other flags: `--dir` (project root), `--timeout` (per repro run, default 2m).

The rest of `auto qa` (QAMESH journeys, `auto qa run`, `auto qa release`, `auto qa report`, and
so on) is still available for declared journeys and release evidence. See `auto qa --help`.

## Routes

Run these in your coding agent's chat.

| Route | What it does |
| --- | --- |
| `/auto review` | Reviews the changed code against TRUST 5 (Tested, Readable, Unified, Secured, Trackable), with the project's own build, test, and lint commands as evidence. Can fan out to multiple providers. |
| `/auto qa` | QA bug hunt by default; QAMESH for declared journeys and release evidence. |
| `/auto verify` | Frontend UX verification: Playwright screenshots plus visual analysis of the changed UI. |
| `/auto test` | Runs the E2E scenarios in `.autopus/project/scenarios.md` through `auto test run`. |
| `/auto sync` | Reads the diff and updates only the docs it made stale (README, ARCHITECTURE, CHANGELOG, `docs/`, `.autopus/project/*.md`), then checks that the commands, flags, and paths those docs name exist. No SPEC needed. |
| `/auto update` | Refreshes the generated harness files. |
| `/auto doctor` | Checks the installed platform wiring. |
| `/auto browse` | Alias for the browser-automation skill. |

Platform syntax:

- **Claude Code, Antigravity CLI, Oh My Pi**: `/auto <route>`
- **OpenCode**: `/auto <route>` or `/auto-<route>`
- **Codex**: `$codex-auto-<route>`, or `@auto <route>` after installing the generated local plugin from
  `.agents/plugins/marketplace.json`

## What gets installed

| Kind | Installed |
| --- | --- |
| Agents (4) | `reviewer`, `qa-explorer`, `frontend-specialist`, `ux-validator` |
| Rules (4) | `language-policy`, `lore-commit`, `file-size-limit`, `autonomy` |
| Skills (9) | `review`, `verification`, `qa-hunt`, `qa-autopilot`, `frontend-verify`, `browser-automation`, `playwright-cli`, `testing-strategy`, `lore-commit` |
| Routes | `review`, `qa`, `verify`, `test`, `sync`, `update`, `doctor`, plus the `browse` alias |
| Hooks | Claude Code Stop hook `auto verify stop`; a Lore `commit-msg` git hook |

The `autonomy` rule tells the agent to carry work through without asking permission for each step.
When a decision really belongs to you, it must ask a multiple-choice question (in Claude Code, the
`AskUserQuestion` tool with the recommended option first), never an open question. Destructive or
outward-facing actions (deleting data, force-pushing, deploying, sending messages) still need your
confirmation.

On Claude Code, `lore-commit` is loaded as a conditional rule when the agent runs a shell command
rather than as an always-on rule.

### Lore commits

The `commit-msg` hook checks the Lore format: a typed, scoped subject
(`feat`, `fix`, `refactor`, `test`, `docs`, `chore`, `perf`, `merge`, `revert`), the
`🐙 Autopus <noreply@autopus.co>` sign-off, the trailers named in `lore.required_trailers`
(default `Constraint`), and none of the trailers in `lore.forbidden_trailers`
(default `Co-Authored-By`). `auto check --lore` runs the same check.

### Multi-provider review

`/auto review` can send the review to several provider CLIs through `auto orchestra review`, with
consensus or debate strategies and a judge. Providers are configured under `orchestra.providers`;
a new config lists `claude`, `codex`, and `gemini` (the `agy` binary), and a provider can also run
through OMP (`backend: omp`). Change the review model per provider with
`auto quality review model <provider> <model>`.

<a id="platforms"></a>
## Platforms

One `autopus.yaml` generates native files for each supported CLI that `auto init` detects.

| Platform | Generated surface |
| --- | --- |
| **Claude Code** | `.claude/agents/autopus/`, `.claude/rules/autopus/`, `.claude/skills/<name>/SKILL.md`, `.claude/hooks/autopus/`, `.claude/settings.json` (Stop hook), `CLAUDE.md` |
| **Codex** | `.codex/skills/codex-<name>/SKILL.md`, `.codex/agents/`, `.codex/hooks.json`, `.codex/config.toml`, `.agents/plugins/marketplace.json`, `.autopus/plugins/auto/`, `AGENTS.md` |
| **Antigravity CLI** | `.agents/plugins/autopus/`, `.agents/hooks.json`, plus `.gemini/` / `GEMINI.md` compatibility files |
| **OpenCode** | `.opencode/agents/`, `.opencode/commands/`, `.opencode/rules/`, `.opencode/plugins/`, `.agents/skills/`, `AGENTS.md`, `opencode.json` |
| **Oh My Pi (OMP)** | `.omp/agents/`, `.omp/rules/autopus-*.md`, `.omp/skills/<name>/SKILL.md`, `.omp/commands/` |

`auto platform add|remove|list` manages the platform list. `auto doctor` checks that the generated
files are wired up. Antigravity also keeps the `rea-mcp-guard` rule for the optional REA
reverse-engineering MCP, which `auto init` registers only with `--rea` (or a Yes in the wizard) when Node fits.

## Install

Paste the one-liner from the top of this page into your agent's chat, or run it in a terminal. The
installer installs the `auto` CLI plus an `autopus` alias, checks required tools, and installs
missing essentials such as `git` and the GitHub CLI. It does not run `auto init`.

<details>
<summary>Other install methods</summary>

```bash
# Homebrew (macOS)
brew install --cask Insajin/autopus/auto

# go install (Go 1.26+)
go install github.com/insajin/autopus-adk/cmd/auto@latest

# From source
git clone https://github.com/autopus-ai/autopus-adk.git
cd autopus-adk && make build && make install
```

`make install` copies into `$(go env GOPATH)/bin`; pass `INSTALL_DIR=...` for another destination.

Installer environment variables: `INSTALL_DIR` (default `/usr/local/bin`), `VERSION` (default latest).

On Homebrew 6 or later, an existing install may need explicit trust for the ADK cask. If Homebrew
reports `Refusing to load cask insajin/autopus/auto from untrusted tap`, run
`brew trust --cask insajin/autopus/auto`. `auto doctor` reports this as
`doctor.homebrew.tap_trust`.

</details>

Then initialize a project:

```bash
cd your-project
auto init          # interactive; --yes for defaults, --platforms claude-code,codex to choose
auto doctor        # check the installed wiring
```

Useful `auto init` flags: `--yes`, `--platforms`, `--project`, `--dir`, `--no-review-gate`,
`--statusline-mode keep|merge|replace`, `--rea` / `--no-rea`.

### Keeping it up to date

```bash
auto update --self   # update the auto binary (publisher signature + SHA256 verified)
auto update          # regenerate this project's harness files from the installed binary
```

`auto update --preview` (also `--plan`, `--dry-run`) lists what would change without writing.
Your content outside the `AUTOPUS:BEGIN` / `AUTOPUS:END` markers in files like `CLAUDE.md` and
`AGENTS.md` is kept. Other generated files are overwritten, with the originals backed up under
`.autopus/backup/<timestamp>/`. `--workspace` also updates nested Git repos in a meta workspace.

### Upgrading from 0.50.x

From v0.51.0 the harness installs only the verification surface described above. Run
`auto update --self && auto update` in each project:

- `auto update` prunes the retired surface recorded in each platform manifest: the routes `plan`,
  `go`, `fix`, `idea`, `setup`, `status`, `goal`, `map`, `why`, `secure`, `dev`, `canary`, and
  `reverse`; the 14 retired agents (planner, executor, tester, architect, spec-writer and the rest);
  the retired skills and rules (such as `branding`, `subagent-delegation`, `worktree-safety`); and
  Claude `.claude/workflows/*.js`.
- An install that is never updated can be cleaned with `auto platform remove <platform>`.
- New configs default to `methodology.mode: none`. An older config with `tdd` is reported as a
  notice by `auto doctor` and `auto check`, not as a failure.
- CLI commands stay in the binary: `auto spec`, `auto pipeline`, `auto orchestra`, `auto workflow`,
  and the rest of `auto --help` keep working. Only the generated agent-chat surface changed.

## Configuration

Everything lives in `autopus.yaml`. A trimmed example with keys that matter for verification:

```yaml
project_name: my-project
platforms: [claude-code, codex]

language:
  comments: en          # code comments
  commits: en           # commit messages
  ai_responses: en      # agent replies

architecture:
  max_file_lines: 300   # optional ceiling for file-size-limit; unset or 0 is advisory

hooks:
  verify_stop: true     # Claude Code Stop hook (default on when unset)
  pre_commit_lore: true # Lore commit-msg hook
  pre_commit_arch: true

lore:
  enabled: true
  required_trailers: [Constraint]
  forbidden_trailers: [Co-Authored-By]   # default; [] allows them

orchestra:              # providers for multi-provider review
  enabled: true
  default_strategy: consensus
  judge: claude
  providers:
    claude:
      binary: claude
    codex:
      binary: codex
```

`auto config --help` lists the config subcommands, and `auto quality` picks quality presets and
review models.

Presets, per-agent models, OMP model pins, review models, and the REA MCP flags are covered in
[docs/quality.md](docs/quality.md).

## Security

- **Signed releases.** Every release since v0.50.73 ships `checksums.txt` and an ECDSA P-256
  publisher envelope `checksums.txt.signatures`. The POSIX installer, the Windows installer, and
  `auto update --self` authenticate the checksum manifest before downloading or extracting, then
  verify the archive's SHA256. Missing or untrusted signing data fails closed; there is no
  checksum-only fallback.
- **Inspect before you install.** Download `install.sh`, read it, then run it:

  ```bash
  curl -sSfL https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.sh -o install.sh
  less install.sh && sh install.sh
  ```

- **Trust boundary.** The one-line installers are served from the repository's `main` branch.
  Publisher verification protects release assets, but it does not authenticate the installer
  bootstrap itself. A compromise of `main` could replace the verifier.
- **QA writes stay contained.** `qa-explorer` writes only under `.autopus/qa/hunt/`, and repro
  scripts run on your machine with your permissions, so read a repro before running it by hand.
- **No API keys.** Autopus drives the provider CLIs you already have; it does not hold your
  provider API keys. Network calls happen only in explicit commands such as `auto update --self`,
  `auto orchestra`, multi-provider review, `auto search`, and `auto connect`.

## Contributing

Autopus-ADK is open source under the MIT license. PRs are welcome.

```bash
make test       # tests with race detection
make lint       # go vet
make coverage   # coverage report
```

CI splits the `internal/cli` tests into name-hash shards. A failing job named like
`test-cli (3/5)` can be re-run locally with `make test-cli-shard SHARD=3/5`.

Commits use the Lore format described above. See [CHANGELOG.md](CHANGELOG.md) for release notes.

## License

[MIT](LICENSE)

---

<div align="center">

**🐙 Autopus** — verify what the agent built by running it.

</div>

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
| Review | The `reviewer` agent on `git diff HEAD` | On demand with `/auto review`, or in the opt-in follow-up round |
| QA bug hunt | The `qa-explorer` agent drives the product; `auto qa hunt verify` re-runs each repro | On demand with `/auto qa`, or in the opt-in follow-up round when the change has runnable behavior |
| Fix | The working agent fixes blockers and confirmed bugs, then re-runs the tests | Same round |

The build and test gate needs no prompt from you. Review and QA cost far more, so they run when you
ask for them, or automatically once you opt in to the follow-up round. Either way the agent is told
to ask you only for decisions that are really yours, as a multiple-choice question.

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

**Credential check.** Before the build runs, the hook scans the lines this turn adds against `HEAD`
(tracked edits plus untracked files that are not gitignored) with a small built-in pattern set:
private key headers, AWS access key ids, GitHub tokens (`ghp_`, `gho_`, `ghu_`/`ghs_`, `ghr_`,
`github_pat_`), Slack tokens and webhooks, Stripe live keys, Google API keys, and a JWT only when it
is assigned to a credential-named key or sent as a bearer token. The patterns favor precision over
recall and need no external tool. A hit blocks the stop once, naming `file:line (rule)` and never the
value, and asks the agent to move the value to an environment variable or secret storage. The same
set of hits again only leaves a warning, so the gate does not loop. Binary files, files over 1 MiB,
and harness directories are skipped. A deliberate fixture passes with an `autopus:allow-secret` (or
`gitleaks:allow`) comment on the line, or a `file:rule:line` entry in `.gitleaksignore`.

**Version check.** Agents pin what they remember from training (`postgres:13`, `"react": "^18"`,
`go 1.22`, `node:20-alpine`), not what is current. Before the build, the hook reads the manifests this
turn changed or created (`go.mod`, `package.json`, `requirements*.txt`, `pyproject.toml`, `Cargo.toml`,
`.nvmrc`, `.python-version`, `.tool-versions`, Dockerfiles, and YAML `image:` keys) and keeps only the
pins it adds or changes against `HEAD`; an old pin the turn did not touch is never reported. Each one is
looked up (see [Dependency versions](#dependency-versions)). A pin more than
`hooks.version_stop.max_major_behind` release lines behind the latest stable (default 0, so any whole line)
blocks the stop once, naming the item, the chosen and the latest version, and the source. Minor-behind
pins only warn. A deliberate pin passes with a reply line `version-pin: <name> <reason>`, or a
`version-pin: <reason>` comment on the manifest line where the format allows comments. Lookups share a
10-second deadline; an offline, private, or slow registry leaves a warning and never blocks.

**Follow-up round (opt-in).** With `AUTOPUS_FOLLOWUP=on` (or `auto verify stop --followup`), when
the build passes and the change has 40 or more changed source lines
(added plus deleted against `HEAD`, untracked source files included), the hook blocks once more with
instructions to:

1. run the `reviewer` agent on `git diff HEAD` and report only blockers;
2. if the change has runnable behavior, run `qa-explorer` in hunt mode and judge the findings with
   `auto qa hunt verify`;
3. fix every blocker and confirmed bug, re-run the tests, and summarize what was checked.

A session gets at most one follow-up round, so fixing the review's findings does not start another.

It is off by default because it is expensive. In a small pilot on real upstream Go PRs (5 tasks run on
both arms, one run each), it multiplied cost about 14x on the four tasks plain Claude Code already
solved, with no change in pass rate. On a harder task it found and fixed three confirmed robustness bugs
that the hidden tests did not cover, at about 12x the cost. The build/test gate never had to block.
Turn the round on where that trade is worth it.

**Opting out.**

| Scope | How |
| --- | --- |
| Don't register the hook | `hooks.verify_stop: false` in `autopus.yaml`, then `auto update` |
| Disable for one session or shell | `AUTOPUS_VERIFY_STOP=off` |
| Turn off only the credential check | `hooks.secret_stop: false`, or `AUTOPUS_SECRET_STOP=off` |
| Turn off only the version check | `hooks.version_stop: false`, or `AUTOPUS_VERSION_STOP=off`; allow N lines behind with `hooks.version_stop: {max_major_behind: N}` |
| Turn the follow-up round on | `AUTOPUS_FOLLOWUP=on` (for example in the `env` block of `.claude/settings.json`), or `auto verify stop --followup` |
| Force it off even where it is on | `AUTOPUS_FOLLOWUP=off` |

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
| `/auto secure` | Security hunt: `auto secure scan --json` for scanner leads, a short threat model of the changed scope, then concrete attacks against your own local or staging target. Only findings whose repro script `auto qa hunt verify` confirms are reported; the rest go to a separate "not confirmed" list. `--multi` adds `auto orchestra secure`; a path narrows the scope. |
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
| Skills (13) | `review`, `verification`, `qa-hunt`, `secure-hunt`, `disk-triage`, `ci-optimize`, `stack-freshness`, `qa-autopilot`, `frontend-verify`, `browser-automation`, `playwright-cli`, `testing-strategy`, `lore-commit` |
| Routes | `review`, `qa`, `secure`, `verify`, `test`, `sync`, `update`, `doctor`, plus the `browse` alias |
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
  `reverse` (`secure` later returned as the security hunt route); the 14 retired agents (planner, executor, tester, architect, spec-writer and the rest);
  the retired skills and rules (such as `branding`, `subagent-delegation`, `worktree-safety`); and
  Claude `.claude/workflows/*.js`.
- An install that is never updated can be cleaned with `auto platform remove <platform>`.
- New configs default to `methodology.mode: none`. An older config with `tdd` is reported as a
  notice by `auto doctor` and `auto check`, not as a failure.
- CLI commands stay in the binary and keep working. Sixteen implementation-harness commands that no
  generated skill, rule, or hook calls (`auto pipeline`, `auto fix`, `auto experiment`, `auto learn`, and
  others) no longer appear in `auto --help`; a later release removes them.

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
  secret_stop: true     # credential check inside the Stop hook (default on when unset)
  version_stop: true    # version check inside the Stop hook; or {max_major_behind: N} (default on, N=0)
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

## Security scan

`auto secure scan` runs the security scanners you already have and prints one summary:

```bash
auto secure scan                    # current directory
auto secure scan --dir ./service --json
auto secure scan --staged           # gitleaks: staged changes only
auto secure scan --log-opts origin/main..HEAD --fail-on critical
```

It detects the project's ecosystems from files in `--dir` and runs each scanner that is installed:

| Scanner | When | Command |
| --- | --- | --- |
| gitleaks | always | `gitleaks git` in a git work tree, `gitleaks dir` otherwise (`detect`/`protect` on releases before 8.19) |
| govulncheck | `go.mod` | `govulncheck -format json ./...` |
| npm / pnpm / yarn | `package.json` and its lockfile | `npm audit --json`, `pnpm audit --json`, `yarn audit --json`, `yarn npm audit --json` (Berry) |
| cargo audit | `Cargo.lock` | `cargo audit --json` |
| pip-audit | `requirements*.txt` | `pip-audit -f json --no-deps --disable-pip -r ...` |

A scanner that is not installed, or a project without a lockfile or requirements file, is noted and
skipped. Nothing is installed: pip-audit checks the pins as written instead of building an environment.
The audit commands and govulncheck query their advisory databases over the network. Each scanner runs
with the project as its working directory and a time limit (`--timeout`, default 5m).

Findings share one shape (`tool`, `kind` secret or vuln, `id`, `severity`, `package`/`file`,
`location`, `fixed_version`, `summary`). Secret values are never printed: gitleaks runs with
`--redact`, the parser does not keep its `Secret` or `Match` fields, and a secret shows only as
`file:line`, rule, and fingerprint (the form `.gitleaksignore` takes).

Exit status: 1 when a finding reaches `--fail-on` (default `high`: any secret, or a vulnerability rated
high or critical; govulncheck, cargo audit, and pip-audit do not grade, so their findings count as high),
2 when a scanner failed and nothing else did, 0 otherwise, including when no scanner is installed.
`--fail-on` takes `none`, `secret`, `critical`, `high`, `medium`, or `low`.

## Dependency versions

`auto deps` asks the registries instead of the model's memory:

```bash
auto deps latest npm:react             # 19.x, with the source URL and fetch time
auto deps latest image:postgres        # Docker Hub official image, numeric release tags only
auto deps latest toolchain:node        # current and active LTS
auto deps latest django                # a bare name is resolved through the project's manifests
auto deps outdated                     # direct deps, toolchain pins, and images vs latest stable
auto deps outdated --dir ./service --json
```

| Ecosystem | Source |
| --- | --- |
| `go:` modules | `GOPROXY` (default proxy.golang.org) `/@latest` |
| `npm:` | `dist-tags.latest` from registry.npmjs.org, or the registry a simple `.npmrc` names (global or per scope) |
| `pypi:` | pypi.org JSON API |
| `crates:` | crates.io API (`max_stable_version`) |
| `image:` | Docker Hub tags; `node`, `golang`, and `python` images use the toolchain feeds below |
| `toolchain:go` | go.dev/dl |
| `toolchain:node` | nodejs.org/dist/index.json; `engines`, `.nvmrc`, and `node:` images are compared with the active LTS |
| `toolchain:python` | endoflife.date (release cycles) |

Pre-releases are never "latest". `outdated` classifies each pin as `current`, `minor-behind`,
`major-behind (N)` (N whole release lines; for Go and Python toolchains and `0.x` packages a minor is a line),
`unknown`, or `skipped`, and always exits 0. Privacy: a Go module matching `GOPRIVATE`, `GONOPROXY`, or
`GONOSUMDB` (environment or `go env -w`) is never sent to a proxy, `GOPROXY=off`/`direct` means unknown,
a PyPI lookup is skipped when `PIP_INDEX_URL` points at a private index, and image references on any
registry other than Docker Hub, digests, and variables are skipped. Requests time out after 5s each and run
four at a time. Answers are cached for 24h in `$XDG_STATE_HOME/autopus/deps` (or
`~/.local/state/autopus/deps`; `AUTOPUS_DEPS_CACHE_DIR` overrides), and `AUTOPUS_DEPS_OFFLINE=1` answers
from the cache only. The `stack-freshness` skill tells the agent when to look up and how to pin on purpose.

## Disk space

Agents build, test, install packages, and create worktrees, and several sessions on one machine can move
free space by tens of GiB in an hour. When the disk fills, builds fail with `no space left on device`,
which looks like a code failure. `auto disk` shows what is growing, keeps heavy commands from starting
while space is short, and cleans up finished work as you go.

```bash
auto disk status                       # free space, every consumer, and what grew in the last hour
auto disk check --need 8               # exit 3 when 8 GiB (or disk.min_free_gb) is not free
auto disk reclaim                      # dry run: what reclaim would remove
auto disk reclaim --apply              # remove it
auto disk trim --apply                 # drop idle Go build cache entries
auto disk lease --need 8 -- go test ./...   # run a heavy step under a machine-wide reservation
auto disk log                          # what trims, reclaims, alerts, and the gate did
```

**What status measures.** The Go build cache and work dirs, linked worktrees (with their `node_modules`
and Cargo `target`), session scratchpads (and files of 1 GiB or more in them), and whichever of these
exist on your machine: the Go module cache, npm, pnpm, yarn, bun, the Cargo registry, pip, uv, Gradle,
Maven, Xcode DerivedData and simulators (macOS), Docker Desktop, colima, lima, and podman VM disks, and
`/var/lib/docker` when readable (Linux). VM disk images are sparse, so their allocated size is what
counts. Each walk has a time budget; a cut walk is shown as a lower bound and kept out of the growth
table. Add machine-specific roots, such as a CI runner's work directory, under
`disk.consumers.extra_roots`; one that cannot be read is reported `unreadable`, never as zero:

```yaml
disk:
  min_free_gb: 10              # the floor the alert, the gate, and leases use
  consumers:
    extra_roots: [/path/to/runner/_work]
```

Every status, `auto disk check`, and per-prompt alert adds a sample to a small ring buffer in the disk
state dir (`$XDG_STATE_HOME/autopus/disk` or `~/.local/state/autopus/disk`). Status then ranks the
consumers by growth, for example `colima +38.0 GiB in 52m`, and the Claude Code low-space alert names the
top grower.

**The disk gate (Claude Code).** A PreToolUse hook, `auto disk gate`, makes one statfs per Bash call. Below
`disk.min_free_gb` it denies builds, tests, and installs of go, cargo, npm, pnpm, yarn, bun, pip, uv,
poetry, Gradle, and Maven, docker and podman builds and `compose up`, xcodebuild, and `git worktree add`,
and tells the agent to pause, run `auto disk status`, and reclaim or wait. Subagent tool calls are gated
too. Every `auto` command passes, so `auto disk lease -- <command>` is how heavy work runs anyway. `make`
is not matched by default; add heavy targets with `hooks.disk_gate.extra_commands: ["make release"]`.
Turn the gate off with `hooks.disk_gate: false` or `AUTOPUS_DISK_GATE=off`. OMP has no pre-tool hook, and
the other platforms do not register the gate yet; run `auto disk check` before heavy steps there.

**Reclaim.** Everything is a dry run unless `--apply` is given, symlinks are never followed, and anything
that changed since the scan is skipped. `auto disk reclaim` removes:

- linked worktrees anywhere whose HEAD is in the default remote branch (`origin/HEAD` or `origin/main`),
  clean with no untracked files, idle beyond `disk.reclaim.worktree_idle_minutes` (default 360), and not
  the working directory of any running process. `git worktree remove` runs without `--force`, which still
  deletes git-ignored files, so such a worktree is kept while it holds any ignored file outside the
  regenerable build directories below (a `.env`, a local database). The branch is kept.
- Claude Code agent worktrees, locked as `claude agent agent-<id> (pid N start T)`, when they are clean and
  merged: right away when the locking process is gone (pid and start time checked), or, while it runs,
  once the worktree is idle and holds no process. They are unlocked, then removed. A dirty or unmerged
  worktree is never removed.
- `node_modules`, `.next`, `dist` (beside a `package.json`) and Cargo `target` directories that git
  ignores, inside idle worktrees.
- idle session scratchpads. Files of 1 GiB or more in the scratchpad root are listed but removed only
  with `--include-large-files`, because they may be backups.

**Automatic cleanup (Claude Code).** The per-turn Stop hook (`auto verify stop`) starts
`auto disk reclaim --auto --apply` in the background at most once per
`disk.reclaim.auto_interval_minutes` (default 30) across all your sessions. It removes only merged, clean
worktrees and idle regenerable directories, never scratchpads or large files, and never delays the turn.
Turn it off with `disk.reclaim.auto: false` or `AUTOPUS_DISK_AUTO_RECLAIM=off`. The `disk-triage` skill
(core, installed by default) walks an agent through the same steps and tells it what to ask you before deleting:
other sessions' work, backups, VM disks, and caches other builds are using.

## Leftover wait loops

An agent that waits with `until ! pgrep -f "job"; do sleep 10; done` never finishes: `pgrep -f` matches the
waiting shell's own command line. A `until grep -q DONE log` loop with no deadline waits forever when the text
never appears. Each such shell costs little, but they pile up across sessions. The always-on autonomy rule tells
agents to give every loop a deadline and to wait on a PID instead. `auto proc reap` cleans up the ones that exist:

```bash
auto proc reap                         # dry run: idle polling shells older than 2h, with the reason
auto proc reap --older-than 30m --json
auto proc reap --apply                 # SIGTERM each shell and its sleep child, SIGKILL after --grace (3s)
auto proc reap --session-pid <pid>     # only descendants of one session
```

A candidate is a sh/bash/zsh/dash/ksh process of the current user running an `until`/`while` + `sleep` loop
whose only live descendants are `sleep`. The reason is `self-matching pgrep` or `no deadline, waiting <age>`.
Other users' processes, shells running anything but sleep (a build or test), and the command's own ancestors
are never touched.

## CI: measure first, then share the host

`auto ci report` measures a GitHub Actions repository before anything is changed. It reads through your
own `gh` login (it fails with what to do when `gh` is missing or logged out), uses only the jobs API
timestamps (`filter=all`, so reruns count; the retired `/timing` endpoint is not used), and never writes.

```bash
auto ci report                          # origin repo, last 14 days, up to 200 runs sampled evenly
auto ci report --repo owner/name --since 30d --workflow ci.yaml --json
```

It ranks findings by severity, then minutes per day, each with its numbers: jobs that wait longer for a
runner than they run, the most expensive jobs (median and p90), minutes lost to failed, cancelled, and
rerun attempts, push runs whose commit tree a successful PR run of the same workflow already tested, PR
runs that kept running after a newer push, and self-hosted runners that are saturated or unevenly
loaded. A lint of the local `.github/workflows` adds jobs without `timeout-minutes`, PR workflows without
a cancelling concurrency group, groups that would also cancel push, release, or `workflow_call` runs,
public-repo PR jobs on self-hosted runners that fork PRs can reach, `pull_request_target` checking out the
PR head, and release or publish jobs sharing a `runs-on` label with PR lanes. `--json` carries every table.
Requests are bounded (`--max-requests`, default 1000; at most 4 at a time) and cached for the run only.
When runs exceed `--limit-runs`, totals are scaled by the reported factor.

**CPU leases for shared hosts.** GitHub Actions has no job priority, so on a self-hosted machine with
several runners the real problem is contention: every job sets `GOMAXPROCS` and `go test -p` to the full
core count, and macOS has no cgroups to stop it. `auto disk lease --cpu N` reserves N of the machine's CPU
slots for one command, on top of (or instead of) the disk reservation. Slots default to the logical CPU
count (`disk.lease.cpu_slots`, or `AUTOPUS_LEASE_CPU_SLOTS` per machine); a larger N is clamped. This is
admission control for a shared host: runners still start jobs in parallel, and only the heavy phases wait
to be admitted by priority. `--priority interactive|ci|background` sets the class (default `ci`; inside
GitHub Actions it follows `GITHUB_EVENT_NAME`: `pull_request`, `pull_request_target`, and `merge_group` are
interactive, `schedule` is background, everything else ci; the flag wins). The highest class goes first,
first in, first out within a class, and a waiter rises one class every `disk.lease.aging_minutes`
(default 10, 0 turns aging off) so background work cannot starve. Running holders are never preempted.
`--wait` bounds the wait and exit 3 means it ran out. A holder or waiter that dies frees its place. The
command runs with `AUTOPUS_LEASE_CPUS=N`, and `GOMAXPROCS=N` unless you set your own. `auto disk status`
lists the holders and the queue in grant order, with each waiter's class, aged class, and wait.

Slots alone cannot tell an idle machine from a busy one, and other sessions and runners load it without
taking a lease. So heavy work runs in parallel when the machine has room, and is admitted by priority
otherwise: the head waiter also waits while the 1-minute load average is above
`disk.lease.max_load_ratio` × logical CPUs (default 0.85), counting grants the average does not show yet
so a burst of admissions cannot overshoot; while available memory minus the `--mem` reservations of
running leases is below its own `--mem SIZE` (such as `8G`, default none) plus `disk.lease.min_free_mem_gb`
(default 8); and on macOS while the kernel reports critical memory pressure. Readings are taken without
spawning a process (`sysctl` on macOS, `/proc` on Linux) and re-checked about every 2 seconds;
`AUTOPUS_LEASE_MAX_LOAD_RATIO` and `AUTOPUS_LEASE_MIN_FREE_MEM_GB` override the keys per machine. `auto disk
status` names what holds the head waiter (`waiting: load 22.1 > 20.4`, `waiting: mem 6.2 GiB < 8+4 GiB`).
A `--mem` that could never fit beside the floor fails at once. When no lease runs and load alone has held
the head waiter for `disk.lease.max_load_wait_minutes` (default 10), it starts anyway, one at a time, and
the start is logged (`lease_forced`); give such a lease a `--wait` longer than that. Windows has no
lease, so this degrades to running the command unleased.

```yaml
# .github/workflows/ci.yaml (self-hosted job)
- name: Test
  run: auto disk lease --cpu 8 --mem 8G --wait 30m --label test -- sh -c 'go test -p "$AUTOPUS_LEASE_CPUS" ./...'
```

Migrating from a shell wrapper that serialises heavy jobs with a single lock (`flock` around every build):
replace the wrapper with `auto disk lease --cpu N --mem SIZE --priority … -- cmd`. The single lock runs one
job at a time however idle the machine is; the lease runs as many as the slots, load, and memory allow.

Leases live in your own state dir by default, so they only coordinate one user's jobs. When several
runner accounts share a machine, point them all at one directory with `AUTOPUS_LEASE_DIR` (for example in
each runner's `.env`) or `disk.lease.dir`. A missing directory is created sticky and world-writable, like
`/tmp`; an existing one that group or others can write must be sticky (`chmod +t`). Where no lease can be
held (Windows), `--cpu` alone warns and runs the command with the same environment.

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

# Quality and model configuration

This is the reference for choosing the models Autopus agents and review providers run on:
quality presets, the `auto quality` composition step and its tool tabs, OMP agent model pins,
multi-provider review models and the SPEC review judge, and the REA MCP flags of `auto init`.
Every command named here has its own `--help`, which is the final word on its behavior.

[🇰🇷 한국어](quality.ko.md)

## Commands

```bash
auto quality                          # Compose per-agent tiers by role group (interactive)
auto quality show                     # Show the persisted mode and supervisor policy
auto quality show --agents            # Also print the agent × platform model table, read-only
auto quality ultra --apply            # Persist Ultra and refresh this project's managed agents
auto quality balanced --apply         # Persist Balanced and refresh this project's managed agents
auto quality provider claude ultra --apply   # Claude only (claude-code is also accepted)
auto quality provider codex balanced --apply # Codex only
auto quality provider claude inherit --apply # Remove the Claude override
auto quality supervisor inherit --apply  # Use the user's Codex model for the primary session
auto quality review                   # Review providers: part in review, backend, model and effort, judge
auto quality review model claude claude-opus-5-5 --effort high  # One review provider's model, no terminal needed
auto quality review off gemini        # Leave a provider out of auto spec review (on adds it back)
auto quality review judge codex       # Choose the review judge (none for a review without one)
auto quality review judge claude --model claude-opus-5-5 --effort max  # The judge's own model; reviewers keep theirs
```

## Presets and provider overrides

`quality.default` names the preset every tool falls back to. A fresh `auto init` writes two
built-in presets, `balanced` (the default) and `ultra`, in the tier vocabulary `fable`, `opus`,
`sonnet`, `haiku`. Autopus ships four agents:

| Agent | balanced | ultra |
|---|---|---|
| frontend-specialist | sonnet | opus |
| reviewer | opus | fable |
| qa-explorer | opus | fable |
| ux-validator | sonnet | opus |

Each tool projects a tier onto its own model ids. With `balanced`, generation writes the
following (`.claude/agents/autopus/<agent>.md` `model`/`effort`, and
`.codex/agents/<agent>.toml` `model`/`model_reasoning_effort`, the Codex column as resolved from
a readable Codex catalog):

| Agent | Claude Code balanced | Codex balanced |
|---|---|---|
| frontend-specialist | `claude-sonnet-5-5` / `max` | `gpt-6-luna` / `max` |
| reviewer | `claude-opus-5-5` / `max` | `gpt-6-astra` / `max` |
| qa-explorer | `claude-opus-5-5` / `high` | `gpt-6.1-sol` / `xhigh` |
| ux-validator | `claude-sonnet-5-5` / `high` | `gpt-6-luna` / `max` |

Claude Code and Codex can follow different presets:

```yaml
quality:
  default: balanced
  providers:
    claude: ultra
    codex: balanced
```

`quality.providers` accepts the canonical keys `claude` and `codex`; a provider without an
override follows `quality.default`. A per-run `--quality` flag is an explicit global override
that wins over both persisted provider values without rewriting YAML. Provider-specific
`--apply` refreshes only that platform; `auto quality <mode> --apply` refreshes every configured
platform. Antigravity CLI and OpenCode agents carry no model and run on the session model.

`quality.supervisor_model_policy: inherit`, the default for new projects, leaves the primary
Codex session on the user's own model; quality still controls the managed agents.
`auto quality supervisor quality --apply` opts an Autopus-managed primary config into the
selected quality profile. User-owned project `model` or `model_reasoning_effort` settings stay
and take precedence. Start a new Codex session after applying.

## The composition step

`auto quality` without arguments, every interactive `auto update`, and the `auto init` quality
step share one composition step. It shows one row per agent and one column per installed
platform:

| Column | Cell |
|---|---|
| Tier | the tier generation uses |
| Claude Code | the `model · effort` written to `.claude/agents/autopus/<agent>.md` |
| Codex | the `model · effort` written to `.codex/agents/<agent>.toml`, resolved from one Codex catalog read that the same run also generates from |
| OMP | the route the OMP profile requests, headed `requested route · before catalog check`; `platform default` without a profile or pins |
| OpenCode, Antigravity | `platform default`: their agents carry no model |

Edits go through role groups that exist only on screen and are never written to `autopus.yaml`:

| Role group | Agents |
|---|---|
| Implementation | frontend-specialist |
| Verification·Review | reviewer, qa-explorer, ux-validator |

At `Change the composition?`, Enter keeps everything and writes nothing, `g` picks one tier per
group (Enter keeps a group), `a` takes one `agent=tier` line per agent (an empty line finishes),
`t` opens the per-tool settings, and `u` (shown only when one exists) updates previous-version
default presets. The preview marks every changed cell with `*` and lists every key it writes;
only `y` writes, `e` keeps editing. A result equal to the current `balanced` or `ultra` default
binds that preset, writing the preset too when it is missing or a previous-version copy; any
other result is saved as `quality.presets.custom` with `quality.default: custom`. Each written
`balanced` and `ultra` preset is labelled `current default`, `previous-version default (can
update)`, or `customized (N differ)`; a customized preset is never modified. When
`quality.providers.claude` or `.codex` names another preset, the step first asks whether that
tool follows. `auto quality --apply` regenerates right after a confirmed change; otherwise run
`auto update`. `auto quality show --agents` prints the same table read-only on any stdin.

`auto update` asks once on every interactive run, after its project settings prompts. Enter or
input end keeps `autopus.yaml` byte-identical, and a confirmed change is generated in the same
run. `--yes`/`-y`, `--auto`, `--check`, a truthy `CI`, non-TTY input, and `--workspace` neither
ask nor change anything; `--plan`, `--preview`, and `--dry-run` skip the step.

The composition step and the per-tool editors refuse to write, leaving `autopus.yaml` unchanged,
while the quality block holds a `${NAME}` value of a set environment variable or a YAML alias or
merge key; edit it by hand then.

### Forms and the tool tab

In a terminal the step runs as forms, like the init wizard: arrow keys and Enter choose, and
scripted or piped input keeps the line prompts. The menu offers keep, update to the latest
defaults (when a built-in preset is a previous-version default), edit the composition, and, in
`auto quality`, choose the default preset. **edit the composition** opens a tool tab:

- **same for all tools** is the role-group composition above.
- **Claude Code** and **Codex** change only that tool. The result is saved as
  `quality.presets.<provider>-custom` with `quality.providers.<provider>: <provider>-custom`.
- **OMP** appears in `auto update` and `auto quality` when the platforms saved in autopus.yaml
  include omp, and in `auto init` when the platforms this init chose include omp.
- **Multi-provider review** is the last option under the conditions in
  [Review models](#review-models).

Every option names the models and efforts that tool would get. A Claude label names the model
slug (`opus-5-5`), while the table cell is the frontmatter value. Ctrl+C in a form ends
`auto update` before any generation, cancels `auto init` before it writes any file, and leaves
`auto quality` without writing more.

On the **Claude Code** and **Codex** tabs every tier select is followed by that tool's effort
select (SPEC-MODELUX-003). keep is the default; Claude Code offers `low`, `medium`, `high`,
`xhigh`, and `max` and leaves haiku agents out; Codex offers the levels the catalog reports for
the agents' models, and with an unreadable catalog prints `Codex catalog unreadable; effort stays
automatic`. The automatic level is marked `(automatic)` and stores nothing. Chosen efforts are
written to `quality.presets.<provider>-custom.efforts`; the preview then warns `auto older than
v0.50.127 cannot read quality.presets.*.efforts`. A file without `efforts` generates exactly as
before.

## OMP agents and model pins

OMP serves the 4 Autopus agents from `.omp/agents/<agent>.md` and claims
`task.disabledAgents: [scout, security-reviewer, sonic, designer, librarian, seance]` in the
project `.omp/config.yml`, disabling every bundled agent but `task`. `auto update` announces the
move with `OMP now uses the 4 Autopus agents.` Without a profile or pins every agent inherits the
OMP session model.

The **OMP** tab asks no profile or family. From the current pins or profile routes it edits the
2 role groups or each agent (`task` too while it has its own pin). Each model select offers
keep, `keep the OMP session model`, and the installed models of every provider Autopus can
attest; a catalog without family or capability metadata lists only the built-in declared ones.
The thinking select offers only the chosen model's levels. The result is saved as pins without a
profile:

```yaml
role_model_policy:
  version: v1
  agents:
    reviewer:
      candidates:
        - {selector: <provider>/<model>, thinking: <level>, family: <family>}
```

and the preview adds `auto older than v0.50.127 cannot read role_model_policy.agents without a
profile`. `task` without its own pin uses the OMP session model, and a pinned model the catalog
no longer has fails closed with `required_route_unresolved`. Applying over a profile prints
`Moving the routes of OMP profile <name> to per-agent pins; unchanged agents keep their routes.`
before the confirmation, and an ultra profile stops following `quality.presets.ultra`.

The OMP line editor and the profile commands keep using profiles:

```bash
auto platform omp models                 # installed model catalog
auto platform omp profile apply balanced --family gpt --plan
auto platform omp profile apply balanced --agent reviewer=<provider/model>:<thinking>
auto platform omp profile apply balanced --agent reviewer=inherit
auto platform omp explain
```

`role_model_policy.agent_source: native` was removed in v0.50.131 and fails to load with
`role_model_policy.agent_source_removed`; delete the key, then run `auto update`.

## Review models

`auto spec review` and the orchestra commands run on `orchestra.providers`. These models are
separate from the agent presets: `auto quality show` names the claude review provider model on
its own line.

The **Multi-provider review** tab ends the tool tab while `spec.review_gate.enabled` is true,
autopus.yaml holds at least two `orchestra.providers` entries, and `CI` is not set
(SPEC-REVMODEL-001). A summary line under the table names each review provider with its model and
effort as written, and the judge. The tab sets:

- which providers take part in `auto spec review` (`spec.review_gate.providers`); fewer than two
  is refused;
- each provider's model and effort. A provider with `backend: omp` picks an installed model
  Autopus attests for its family and a thinking level (`orchestra.providers.<p>.model`). On a CLI
  backend, `claude` picks a Claude tab model (`claude-fable-5-1`, `claude-opus-5-5`,
  `claude-sonnet-5-5`, `claude-haiku-4-5`) and effort (`--model`, `--effort`), `codex` a model of
  the Codex catalog and one of its efforts (`-m` and `model_reasoning_effort`, with
  `model_policy: pinned`), and `agy` an id `agy models` lists (`--model`). Only those args values
  change. When omp attests a model of its family, any CLI provider can switch to the OMP backend
  (`backend: omp` and `model`);
- the judge (`spec.review_gate.judge`), and optionally the judge's own model.

Turning a provider off changes only `auto spec review`: orchestra commands (review,
secure) and `auto spec review --multi` keep every provider. An optional live check sends
one request to each of n review providers (at most 3n model calls with retries) and writes
nothing. The preview shows changed CLI values as written and never other args; a value taken from
`${NAME}` is left alone. Only the changed values are rewritten, so comments and layout stay.

### The judge model

The judge select ends with `<judge> — choose the judge model`, and a new judge asks it at once:
the judge's own model and effort from the same lists, written as `spec.review_gate.judge_model`
and `judge_effort` (SPEC-JUDGEMODEL-001). Only the judge call of `auto spec review` uses it; every
reviewer keeps its own model. `same as the reviewer model` removes both keys, and an auto release
without them refuses a file that sets them.

```yaml
spec:
  review_gate:
    enabled: true
    providers: [claude, codex, gemini]
    judge: claude
    judge_model: claude-opus-5-5   # optional
    judge_effort: max              # optional
```

### Without a terminal

`auto quality review` (or `show`, `--json`) prints each provider's part in review, backend, model,
and effort as written, and the judge. These change them with the tab's checks:

```bash
auto quality review model <provider> <model> [--effort <effort>] [--dry-run]
auto quality review on <provider>... [--dry-run]
auto quality review off <provider>... [--dry-run]
auto quality review judge <provider|none> [--model <model>|default] [--effort <effort>] [--dry-run]
```

`model` takes only what the tab offers that provider and lists the accepted values when it
refuses one. `judge --model default` goes back to the reviewer model. Every edit prints the
settings that change first; nothing is written when nothing changes or with `--dry-run`.
Non-interactive `auto init` and `auto update` print one line naming where review models change.

## Navigation and the init review

Every form of the `auto init` wizard, of the composition step, and of the init review shows a
header with the path that reached it, such as `init · 4/6 Review Gate` or
`quality · Model composition › edit the composition › Codex`, the values chosen so far
(`Chosen so far`), and the footer `Arrows move · Enter select · Esc back · Ctrl+C cancel`, all
inside the terminal height (SPEC-TUIUX-001). Esc goes back one form, and a form shown again keeps
the earlier answer selected. On a first form Esc prints `This is the first step; press Ctrl+C to
cancel.` and stays. Esc never undoes a write already applied. In accessible line mode
(`TERM=dumb`) a line `b`, `back`, or `뒤로` is Esc; the line prompts have no back.

Before it writes any file, an interactive `auto init` on the form path shows `Settings to apply`:
the answers (`Answers`), every setting key they change (`Changed settings`), and the REA decision
with its action (for example `REA MCP: on · add`). It then asks `Proceed with these settings?`
with `apply` (the default), `back`, and `cancel`; cancel and Ctrl+C print `init cancelled.` and
leave the project as it was. The review does not show what comes after it: the orchestra
normalization, the statusLine and parent rule prompts, the `features.rea` key written after
generation, and the generated files. `--yes`, non-terminal input, and the line prompts have
neither back nor the review.

## REA MCP init flags

`auto init` registers the project-scope REA reverse-engineering MCP (`rea-agents@6.5.0`) for
Claude Code, Codex, and Antigravity CLI (SPEC-REA-001). Since v0.51.0 it is opt-in: a new init leaves it
off, `--rea` and `--no-rea` decide explicitly, and passing both stops before anything is written.
Registration happens only when Node is `^22.19.0 || ^24.11.0 || >=26.0.0`. The Claude Code and
Antigravity servers start through POSIX `/bin/bash`, so they do not start on Windows; OMP and
OpenCode get no entry.

`features.rea.enabled` is written only when the managed entries cannot carry the decision, and
auto older than v0.50.127 cannot read features.rea (the affected releases are v0.50.113–v0.50.125).
`auto update` asks once on an interactive run without a decision and otherwise only prints a hint;
`--no-rea` removes the managed entries and leaves user-owned ones. `auto doctor` reports the
decision, the Node probe, and each platform's entry without starting the server.

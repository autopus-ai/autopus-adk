package cli

// qualityCmdLong is the help of `auto quality`; the composition step it
// describes is SPEC-MODELUX-001 (REQ-MUX-23) and SPEC-MODELUX-002 (REQ-MUX2-21).
const qualityCmdLong = `Show or change quality.default, quality.providers, and
quality.supervisor_model_policy in autopus.yaml.

Use ` + "`auto quality <preset> --apply`" + ` for the global fallback,
` + "`auto quality provider <provider> <preset|inherit> --apply`" + ` for one provider,
or run ` + "`auto quality`" + ` without arguments to compose per-agent models.

Without arguments, in a terminal, the agent model composition comes first:
one row per agent with its tier and one column per installed platform.
Claude Code and Codex cells are the model · effort generation would write
(Codex resolved with one catalog read). The OMP column is the route its
profile requests (requested route · before catalog check); OpenCode and
Antigravity show platform default. Each written balanced or ultra preset is
labelled current default, previous-version default (can update), or
customized (N differ). ` + "`auto quality show --agents`" + ` prints the same
table read-only.

Change the composition? takes:
  Enter  keep; nothing is written
  u      update previous-version default presets (shown only when one exists)
  g      one tier (fable, opus, sonnet, haiku) per role group, Enter keeps:
           Design·Judgment: planner, architect, spec-writer, security-auditor
           Implementation: executor, debugger, deep-worker, devops,
             frontend-specialist, perf-engineer
           Verification·Review: reviewer, validator, tester, ux-validator
           Exploration·Support: explorer, annotator
  a      advanced: one agent=tier line per agent, an empty line finishes
  t      per-tool settings; global, claude-code, codex, and omp work too

The preview marks every changed cell with * and lists the keys it writes;
only y writes, e keeps editing. A result equal to the current balanced or
ultra default binds that preset, writing the preset too when it is missing
or a previous-version copy; any other result is saved as
quality.presets.custom with quality.default: custom. A customized balanced
or ultra preset is never modified. When quality.providers.claude or .codex
names another preset, the step first asks whether that tool follows (y
removes the override). With --apply a confirmed change regenerates the
harness from the same catalog read; otherwise run ` + "`auto update`" + `.

The composition step and the per-tool editors refuse to write, leaving
autopus.yaml unchanged, while the quality block holds a ${NAME} value of a
set environment variable or a YAML alias or merge key; edit it by hand then.

In a terminal (the process stdin, with stdin and stdout both terminals), the
composition step runs as forms like the init wizard: arrow keys and Enter.
The menu offers keep, update to the latest defaults (when a built-in preset
is a previous-version default), edit the composition, and choose the default
preset. edit the composition opens the tool tab: same for all tools, Claude
Code and Codex when installed, and OMP when the platforms saved in
autopus.yaml include omp. Every option names the models and efforts that
tool would get; a Claude label names the model slug (opus-5-5) while the
table cell is the frontmatter value (opus). A Claude Code or Codex tab
result is saved as quality.presets.<provider>-custom with
quality.providers.<provider>; the OMP tab pins one installed model per native
agent through the OMP writer and shows the plan's real routes first. Ctrl+C
in a form ends the command without writing more. Scripted or piped input
keeps the line prompts above.

On the Claude Code and Codex tabs every tier select is followed by that
tool's effort select (SPEC-MODELUX-003). keep is the default; Claude Code
offers low, medium, high, xhigh, and max and leaves haiku agents out, Codex
offers the levels the catalog reports for the agents' models, and the
automatic level is marked (automatic) and stores nothing. Chosen efforts are
written to quality.presets.<provider>-custom.efforts, which auto older than
v0.50.126 cannot read; a file without efforts generates exactly as before.

OMP serves the 16 Autopus agents from .omp/agents by default and disables
every bundled agent but task (scout, security-reviewer, sonic, designer,
librarian). In that mode the OMP tab asks no profile or family: from the
current pins or profile routes it edits the 4 role groups or each agent
(task too while it has its own pin). Each select
offers keep, keep the OMP session model, and the installed models of every
provider that Autopus can attest (a catalog without metadata lists only the
built-in declared ones); thinking offers only the chosen model's levels.
The result is saved as pins without a profile (role_model_policy:
{version: v1, agents: ...}), and task follows planner unless pinned.
Applying over a profile converts it after a preview line and the
confirmation: unchanged agents and task keep their routes, and an ultra
profile stops following quality.presets.ultra. Native mode, the OMP line
editor, and auto platform omp profile apply keep profiles;
role_model_policy.agent_source: native keeps the five bundled agents during
v0.50.126 (removal planned in v0.50.127).`

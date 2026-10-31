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
           Exploration·Support: explorer, annotator, reverse-engineer
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
quality.providers.<provider>; the OMP tab pins one installed model per
agent through the OMP writer and shows the plan's real routes first. Ctrl+C
in a form ends the command without writing more. Scripted or piped input
keeps the line prompts above.

On the Claude Code and Codex tabs every tier select is followed by that
tool's effort select (SPEC-MODELUX-003). keep is the default; Claude Code
offers low, medium, high, xhigh, and max and leaves haiku agents out, Codex
offers the levels the catalog reports for the agents' models, and the
automatic level is marked (automatic) and stores nothing. Chosen efforts are
written to quality.presets.<provider>-custom.efforts, which auto older than
v0.50.127 cannot read; a file without efforts generates exactly as before.

OMP serves the 17 Autopus agents from .omp/agents and disables every bundled
agent but task (scout, security-reviewer, sonic, designer, librarian,
seance). The OMP tab asks no profile or family: from the
current pins or profile routes it edits the 4 role groups or each agent
(task too while it has its own pin). Each select
offers keep, keep the OMP session model, and the installed models of every
provider that Autopus can attest (a catalog without metadata lists only the
built-in declared ones); thinking offers only the chosen model's levels.
The result is saved as pins without a profile (role_model_policy:
{version: v1, agents: ...}), and task follows planner unless pinned.
Applying over a profile converts it after a preview line and the
confirmation: unchanged agents and task keep their routes, and an ultra
profile stops following quality.presets.ultra. The OMP line editor and auto
platform omp profile apply keep profiles. role_model_policy.agent_source:
native was removed in v0.50.131 and fails to load: delete the key, then run
auto update to restore the Autopus agents.

The tool tab ends with Multi-provider review while spec.review_gate is
enabled, autopus.yaml holds at least two orchestra.providers entries, and CI
is not set (SPEC-REVMODEL-001); a summary line under the table names the
review providers and the judge. The tab sets the providers auto spec review
uses (spec.review_gate.providers), each provider's model and effort, and the
judge (spec.review_gate.judge). A provider with backend: omp picks an
installed model Autopus attests for its family and a thinking level
(orchestra.providers.<p>.model). On a CLI backend, claude picks a Claude tab
model and effort (--model, --effort), codex a model of the Codex catalog and
one of its efforts (-m and model_reasoning_effort, with model_policy:
pinned), and agy an id agy models lists (--model); only those args values
change. When omp attests a model of its family, any CLI provider can switch
to the OMP backend (backend: omp and model). The preview shows changed CLI
values as written in autopus.yaml and never other args; a value taken from
${NAME} is left alone. Only those values are rewritten, so comments and
layout stay. Orchestra commands (brainstorm, plan, review, secure) and
auto spec review --multi keep every provider. An optional live check sends
one request to each of n review providers (at most 3n model calls with
retries) and writes nothing. Without a terminal, ` + "`auto quality review`" + `
shows and changes the same settings (auto quality review --help).

Every form shows a header with the path that reached it, for example
quality · Model composition › edit the composition › Codex, the values
chosen so far in an edit pass (Chosen so far), and the footer Arrows move ·
Enter select · Esc back · Ctrl+C cancel, all inside the terminal height
(SPEC-TUIUX-001). Esc goes back one form, and a form shown again keeps the
earlier answer selected: an edit pass returns to the form it asked before,
the method to the tool tab, and the tool tab to the menu. Esc on Apply? or
on the override question keeps editing, and the OMP and Multi-provider
review tabs go back to the tool tab as a whole. The menu is the first form:
Esc there prints "This is the first step; press Ctrl+C to cancel." and
stays. Esc never undoes a write already applied. In accessible line mode
(TERM=dumb) a line b, back, or 뒤로 is Esc; the line prompts have no back.
Before it writes any file, auto init shows Settings to apply (the answers,
every changed setting key, and the REA decision) and asks apply, back, or
cancel (auto init --help).`

// qualityReviewCmdLong is the help of `auto quality review`, the review tab
// of SPEC-REVMODEL-001 without a terminal.
const qualityReviewCmdLong = `Show or change, without a terminal, what the Multi-provider review tab of
auto quality edits: the orchestra.providers that take part in auto spec
review (spec.review_gate.providers), each provider's model and effort, and
the judge (spec.review_gate.judge).

  auto quality review [show] [--json]
  auto quality review model <provider> <model> [--effort <effort>] [--dry-run]
  auto quality review on <provider>... [--dry-run]
  auto quality review off <provider>... [--dry-run]
  auto quality review judge <provider|none> [--dry-run]

show prints, per orchestra provider, whether it takes part, its backend,
and its model and effort as autopus.yaml writes them, then the judge.

model takes only what the tab offers the provider. A provider with backend:
omp takes an installed model the OMP catalog attests for its family, as
<provider>/<model>[:<thinking>]; --effort is the thinking level. A CLI
provider takes, by its binary: claude a Claude tab model with low to max
(no effort for haiku), written after --model and --effort; codex a model of
the Codex catalog with one of its efforts, written after -m and as
model_reasoning_effort with model_policy: pinned; agy an id agy models lists
for the provider's family, or that id without its -low, -medium, or -high
suffix and the suffix as --effort. Without --effort the current effort or
thinking level stays when the new model takes it. Switching a CLI provider
to the OMP backend is left to the tab. A refused model lists the accepted
values.

on and off take several providers at once, so a review below two providers
can reach two in one edit; they change auto spec review only, while
orchestra commands and auto spec review --multi keep every provider. judge
takes a provider that takes part, or none for a review without a judge.

Every edit is checked like the tab's: at least two providers take part,
the judge takes part, and a changed model is attested. A value taken from
${NAME} is left alone, as the tab leaves it. The settings that change are
printed first; nothing is written when nothing changes or with --dry-run.
Only the changed values are rewritten in autopus.yaml, so comments and
layout stay. auto spec review and orchestra commands read them when they
run, so --apply is refused.`

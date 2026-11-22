# Harness pilot C: does the verification layer catch what review catches?

Status: design, 2026-11-22. No trial has run.

## Why another pilot

The [2026-09-20 pilot](harness-2026-09-20.md) found no quality benefit and
10-17% more elapsed time for the harness arms. It measured less than it
appears to:

- Every arm ran `codex exec --disable hooks`. The enforcement layer — the
  `auto verify stop` Stop hook, the edit guard, the verification round that
  is on by default since #335 — never ran. Only instruction and skill
  exposure was compared.
- `auto verify stop` is registered for Claude Code only
  (`pkg/content/hooks.go`, `generateVerifyStopHook`). A Codex arm cannot
  measure it even with hooks enabled.
- The tasks were 12 single-file seeded regressions with visible tests. Every
  arm passed 12/12, so the oracle had a ceiling and could not show a defect
  the harness prevents.

Pilot C asks one question: **on feature-sized work, does the full harness
leave fewer defects than the native CLI, and at what cost?**

## Arms

| Arm | Surface | Hooks |
| --- | --- | --- |
| `native` | no project harness | none |
| `instructions` | generated harness, default compact skills | disabled (`--settings` with `disableAllHooks`) — reproduces the 09-20 condition |
| `full` | generated harness, default compact skills | enabled as generated (verify stop, guard, sticky rules, verify language off) |

All arms: Claude Code `claude -p` print mode, same model and effort, same
permission profile, fresh isolated checkout per trial, no human input.
`verify language` is disabled in every arm, because the task prompts are
English and a language retry would measure the policy, not verification.

## Task source rule

A task is a feature request that an agent with this harness already
implemented and that an **independent review then found defective**. The
oracle is the set of tests written for the review-confirmed defects. This
targets exactly the gap the verification layer claims to close.

Each task records:

- `base`: the revision the original feature started from.
- `prompt`: the request as the user stated it, plus the public interface the
  oracle calls (command and flags, or exported function). The interface is
  pinned so an oracle failure means a defect, not a naming difference.
- `oracle`: hidden tests, copied into the candidate tree only at grading.
  They must be black-box through the pinned interface.
- `allowed_paths`: directory globs, not single files; new files and tests are
  allowed.

Validity gates, checked before any model trial and frozen with the corpus
hash:

1. The oracle passes on the reviewed-and-fixed head (it is satisfiable).
2. Each oracle case fails on the original pre-review head (it discriminates:
   it is a defect that passed every check at the time).
3. The oracle passes against no stub: on `base` with only the interface
   stubbed it fails (it is not vacuous).

A case that fails gate 2 is dropped, not rewritten after seeing results.

## Candidate tasks

| ID | Feature (base) | Oracle source | Size |
| --- | --- | --- | --- |
| c01 | `auto ci report` lints the remote default branch's workflows when the checkout is behind (#347, base 7b017f17) | 10220e08: unrelated local commit, index not rewritten, remote not named origin, remote without workflows, remote symlink, other-repository guard | ~300 lines |
| c02 | `auto doctor` text verdict matches what it printed (#350, base 7b017f17, pre-review eb26e657, fixed dcda6485) | verdict invariant: FAIL box only under an ERROR line; warn-only OMP degradation does not advise `auto update`; repeated diagnostics print once | ~100 lines |
| c03 | `cpu_gate` refuses CPU-heavy Bash commands outside a lease under load (#349, base 7b017f17) | #349 review fixes: heredoc body not classified, `mvn install`/`pytest -v`/`uv run --frozen` classified, env switch, no-lease-to-protect allows | ~450 lines |
| c04 | `auto verify stop` time budget for large Go packages (#348, base 7b017f17) | #348 review fixes: deadlock blocks, signal is not a verdict, unrun non-bound step not recorded, agent not told tests passed | ~650 lines |
| c05 | checklist candidate ids and approval (#344 subset) | b828883f H2: `../` in a candidate id must not escape; H3: approve shows the verify that will run | subset |
| c06 | checklist refutation record (#344 subset) | 343cd3d6 H3: a forged `survived` line in the candidate file is not trusted | subset |
| c07 | qa loop baseline of ignored files (SPEC-QALOOP-001 subset) | e831272e: an agent `.gitignore` edit does not reclassify ignored user files | subset |

c03 and c04 wait for their review fixes to land, because those fixes are the
oracle. c05-c07 are subsets of SPEC-sized changes; each prompt carries only
the slice the oracle checks.

Seven tasks are fewer than the 12-task floor of the 09-20 design. Pilot C is
a first look at whether the effect exists, not a significance test; more
tasks come from the next review-confirmed defects (`auto checklist record`
is the intake).

## Measures

Primary: oracle cases passed per arm (escaped defects = cases failed).
Secondary, per trial: completed within the deadline, elapsed wall time
including hook time, input/output/cached tokens, number of Stop-hook blocks
and what they named (`full` only), scope violations. Failed and timed-out
attempts stay in the report; unknown usage stays unknown.

## Budget

Deadline 30 minutes per trial (the verify-stop budget alone is 8 minutes).
7 tasks x 3 arms x K=1 = 21 trials, serial: up to ~10 hours wall time,
expected 4-6. K=2 doubles it; decide after K=1 shows whether any arm differs.

## What is built

Command lines and the isolation boundary are in the
[harness README](../../scripts/benchmarks/harness/README.md#pilot-c-verification-layer-on-claude-code).

1. Claude Code driver: `claude_agent.py` (arm settings, also written as the
   isolated user settings so nested sessions follow the arm; isolated
   `HOME`/`CLAUDE_CONFIG_DIR`; an allowlisted environment; the
   `claude -p --output-format stream-json --verbose --include-hook-events`
   argv), `stream_c.py` (usage, Stop outcomes and blocks, contamination,
   arm validity), `claude_agent.sb` (writes only in the trial; reads closed
   below the homes, temp roots, run output and checkouts except the trial,
   module cache, pinned bin and tool installs; Unix sockets, the keychain,
   signals to other processes, LaunchServices opens and cfprefsd writes
   closed; the Bash tool's login shell replaced by a wrapper so the host
   PATH never comes back), `sources_c.py` and `surface_c.py`
   (`auto` built from `--harness-revision`, resolved to a SHA and recorded per
   trial, generates the claude-code surface into the task's base with
   `hooks.language_stop: false`; hook commands are pinned to it by absolute
   path), `guard_c.py` (refusals below; frozen copy of profiles, corpus and
   oracles), `trial_c.py`, `grade_c.py`, `report_c.py` and `run_c.py`
   (serial schedule, deadline, whole-tree kill, sandboxed grading, per-trial
   exception records, `--resume`, `--dry-run` without a model call). The
   login is never copied: the trial's only credential is a
   `claude setup-token` token or an API key from a mode-0600 file.
2. `corpus_c.json` with c01 and c02, `oracles/c01/`, `oracles/c02/`, and
   `gate_c.py`, which checks the gates without a model call, grading each
   head exactly as a trial is graded, and records the corpus and oracle hash
   in `gates.json`. A real run refuses to start without a passing `gates.json`
   for that hash. Each case has a kind: `review` (a review-confirmed defect;
   gate 2 against the pre-review head), `feature` (the feature's own test;
   must fail on base) or `guard` (behaviour base already had and the feature
   must keep; must pass on base). Five c01 cases passed on the pre-review head
   or on base, so they moved from `review`/`feature` to `guard` before any
   trial; c01 keeps 5 `review` cases, c02 has 2. c02 uses PR #350: pre-review
   head eb26e657, fixed head dcda6485. Its `review` cases are what dcda6485
   fixed; its `feature` cases are the defects eb26e657 already fixed relative
   to base.
3. The launch stays a maintainer `nohup` line, because an agent session's
   permission classifier may refuse `claude -p` and `go test` on generated
   trial checkouts.

Grading. The graded tree is the task's base plus only the candidate's
regular non-test files under `allowed_paths`; the oracle build and every case
run under grader.sb with a grading build cache no agent can read, in a grade
root outside every trial, and only after the trial's process tree is empty.

Refusals. A run does not start when its harness revision has any task's
pre-review or fixed head as an ancestor (that `auto` would carry the answer)
or lacks a commit the task lists in `harness_requires`, when `--output` lies
below a `CLAUDE.md`, `CLAUDE.local.md` or `.claude/` (parent instructions load
into every arm) or in a shared temporary root, or without passing gates.

Harness revision. For c01 and c02 it is 30722ad9: the last commit of PR #348,
branched from 7b017f17, before main was merged into that branch. #348 is
needed: before it, `auto verify stop` blocks every stop after a change under
`internal/cli`, whose tests take about 32 minutes, and c01 and c02 both change
`internal/cli`, so the `full` arm would measure that bug as timeouts, not the
verification layer. Both tasks list 30722ad9 in `harness_requires`, so
7b017f17 is refused. #348 is merged; its branch head fcfbeff4 and `origin/main`
contain the c01 and c02 heads and are refused. The runner records the SHA.

Primary measure per arm: `review` cases passed. `feature` and `guard` cases
are reported with it.

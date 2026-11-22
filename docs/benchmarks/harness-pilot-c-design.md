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
| c02 | `auto doctor` text verdict matches what it printed (branch fix/doctor-verdict-and-sticky-dedup, base 7b017f17) | verdict invariant: FAIL box only under an ERROR line; warn-only OMP degradation does not advise `auto update`; repeated diagnostics print once | ~100 lines |
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

## What must be built

1. A Claude Code agent driver for `scripts/benchmarks/harness` (today
   `golden_agent.py` and `run.py` drive `codex exec` only): `claude -p`
   with `--output-format stream-json`, a per-arm `--settings` file, an
   isolated `HOME`/`CLAUDE_CONFIG_DIR` with only the credential, and
   observation of hook blocks from the stream.
2. Corpus file `corpus_c.json` and `oracles/<id>/` with the gates above as
   a runnable check.
3. The launch: an agent session's permission classifier may refuse to run
   `claude -p` and `go test` on generated trial checkouts, so the maintainer
   starts the run from a terminal with one `nohup` line and the report is
   read back afterwards.

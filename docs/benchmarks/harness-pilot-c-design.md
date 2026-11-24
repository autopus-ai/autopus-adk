# Harness pilot C: does the verification layer catch what review catches?

Status: corpus c01-c06 gated, 2026-11-23. One smoke run (c01, c02; below); no scored run yet.

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

## Tasks

| ID | Feature (base) | Heads (pre-review, fixed) | review / feature / guard |
| --- | --- | --- | --- |
| c01 | `auto ci report` lints the remote default branch's workflows when the checkout is behind (#347, base 7b017f17) | 19721f08, 10220e08 | 5 / 5 / 5 |
| c02 | `auto doctor` text verdict matches what it printed (#350, base 7b017f17) | eb26e657, dcda6485 | 2 / 2 / 0 |
| c03 | opt-in `cpu_gate` refuses unleased CPU-heavy Bash commands under load (#349, base 7b017f17) | 32debe26, 3b194190 | 25 / 10 / 9 (one case per command group) |
| c04 | lesson and checklist ids stay unique across branches, union-merged store reads back (#354, base a93c4717) | aca0228d, c9149d98 | 2 / 9 / 1 |
| c05 | `pkg/procgroup`: verify-stop step cancellation reaches the whole tree (#355, base a93c4717) | 52e6c55c, a7f5d43e | 4 / 3 / 0 |
| c06 | disk gate follow-ups: subdirectory config, `--help`, here-document scripts, lookups (#356, base a93c4717) | ded4a2f5, 25530eb7 | 10 / 7 / 6 |

Every task is a real defect an independent review found after the change passed its own checks; the
`review` cases are those defects. Kinds come from the gate results, not from the commit that added
a test: a review-fix test that already passed on the pre-review head is a `feature` (c04) or, when
base passed it too, a `guard` (c06's backquote in a redirection target, a bug only the intermediate
commit 3529cade had). c06's fixed head is 25530eb7, the PR's own last commit before main was merged
into its branch (e9e80092); `a93c4717..25530eb7` and `bc6bed02..e9e80092` change the same files.

Interfaces. c03 and c06 call `diskgate.Classify`/`ClassifyCPU(line string, extra ...string)
Verdict` and run `auto disk gate`, `lease`, `check` and `reclaim` through the test seams base
already has (load sampler, CPU count, state path, free space); env names are literal. c04 uses only
exported learn, checklist and memindex API that base has, and the checklist commands through
`NewRootCmd()`. c05 uses the exported `procgroup` API the pre-review head shares with the fixed one
(`New`, `Options{Grace}`, `Start`, `Terminate`, `Kill`, `Close`) and `verifygate.Run`; c03 and c05
carry a base stub so their new-package or new-function cases compile and fail on base. Assertions on
wording the prompt cannot pin were relaxed to behaviour (deny-reason details, note phrases, the
floor message); c05 pins a timeout-reason form base already uses and checks the reported duration.

Not graded, so not promised by a case: c03 F8 (deferred in the PR); c04 F3 (the id suffix is computed
from the redacted entry: needs an unexported function) and the `auto checklist status` duplicate
warnings and `auto mem` skip line added by the review (wording only the fix defines); c05 item 2
(execcheck restart) and 4 (vanished PIDs), which need internals, the Windows items, and the three
cases that need a descendant in its own session: the fixed head finds it through `ps`, which is
setuid root and cannot be exec'd under grader.sb or claude_agent.sb, and opening it would let graded
code read other processes' environments (SPEC-HARNEVAL-003 S1). A c05 agent that tests its own
ps-based walk in the trial sees the same denial; that friction is the same in every arm.

Dropped candidates. The harness for a run is 30722ad9 plus at most cherry-picked harness fixes, and
the run refuses a harness that has a task's pre-review or fixed head as an ancestor (that `auto`
would put the answer on the agent's PATH):

- the earlier c04, `auto verify stop` time budget (#348): 30722ad9 is its fixed head;
- the earlier c05 and c06, checklist candidate ids and refutation record (#344 subsets; b828883f,
  343cd3d6) and c07, qa loop ignored-file baseline (e831272e): all three are ancestors of 7b017f17
  and so of 30722ad9.

They are replaced by #349, #354, #355 and #356, all branched after or beside 30722ad9. Checked with
a scratch commit on 30722ad9 standing in for the cherry-picked fix: `guard_c` accepts it for all six
tasks and refuses `origin/main` (every task head is its ancestor) and 7b017f17 (lacks 30722ad9).

## Measures

Primary: oracle cases passed per arm (escaped defects = cases failed).
Secondary, per trial: completed within the deadline, elapsed wall time
including hook time, input/output/cached tokens, number of Stop-hook blocks
and what they named (`full` only), scope violations. Failed and timed-out
attempts stay in the report; unknown usage stays unknown.

## Budget

Deadline 90 minutes per trial (`--timeout 5400`; agents run the ~32-minute `internal/cli` suite,
and at 30 minutes every smoke trial timed out). 6 tasks x 3 arms x K=2 = 36 trials, serial. From
the smoke (mean elapsed 1822 s native, 3402 s instructions, 4778 s full per trial): about
6 x 2 x (1822 + 3402 + 4778) s = 33 hours of agent time, plus grading and set-up, so about 34-36
hours; at most 36 x 5400 s = 54 hours if every trial ran to the deadline. c04-c06 change fewer
`internal/cli` files than c01/c02, so their trials may run shorter.

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
2. `corpus_c.json` with c01-c06, `oracles/<id>/`, and
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

Harness revision. For every task it is 30722ad9 (or 30722ad9 plus cherry-picked harness fixes): the last commit of PR #348,
branched from 7b017f17, before main was merged into that branch. #348 is
needed: before it, `auto verify stop` blocks every stop after a change under
`internal/cli`, whose tests take about 32 minutes, and most tasks change
`internal/cli`, so the `full` arm would measure that bug as timeouts, not the
verification layer. Every task lists 30722ad9 in `harness_requires`, so
7b017f17 is refused. #348 is merged; its branch head fcfbeff4 and `origin/main`
contain the task heads and are refused. The runner records the SHA.

Primary measure per arm: `review` cases passed, reported raw and without the review cases that
already pass on base (c04 1/2, c05 3/4, c06 6/10; the defect came with the feature, so an
unchanged candidate passes them). `feature` and `guard` cases are reported with it.

Agent sandbox. Since PR #364 review, claude_agent.sb also denies the `kern.procargs` sysctls and
limits proc_info to its own sandbox instance, as grader.sb does: before, an agent could read the
environment of a sibling process and of the runner's parent shell (the maintainer's tokens).

## Smoke 2026-11-23

One replicate of c01 and c02 per arm (6 trials), harness 30722ad9, `--timeout 5400`, report
`/Users/Shared/pilot-c/smoke-20261123-0720/report.md`. A smoke checks the machinery, not the effect;
two tasks and K=1 decide nothing.

| arm | review | feature | guard | elapsed s | timeouts | Stop blocks |
| --- | --- | --- | --- | --- | --- | --- |
| `native` | 6/7 | 7/7 | 3/5 | 3643 | 0 | 0 |
| `instructions` | 5/7 | 7/7 | 3/5 | 6803 | 0 | 0 |
| `full` | 7/7 | 7/7 | 3/5 | 9555 | 1 (c02, 5400 s) | 3 |

- The review case only `full` passed was c01 `RemoteNotNamedOriginIsNoted`; `instructions` also
  missed c02 `ErrorLineAlwaysFails`.
- All three `full` Stop-hook blocks named tests that fail on the task's HEAD inside the trial
  sandbox (`TestEvalHarnessE2E_S5…`, `TestInitReview_S6…`), unrelated to the change. They cost time
  and say nothing about the candidate.
- An earlier run at `--timeout 1800` timed out in every trial: agents run the ~32-minute
  `internal/cli` suite. The default deadline is now 5400 s.

Measurement fixes made from it, before any further trial:

- c01 guard cases `OtherRepositoryReadsLocalWithNote` and `NoRemoteAtAllHasNoComparisonNote` failed
  identically in every arm. Both now assert behaviour only: which files were linted (the local
  checkout) and that no note claims a comparison (`commits behind`, `differs from`). The no-remote
  case had failed on the note `origin/HEAD does not resolve`, which the pinned interface itself
  requires when `origin/HEAD` is missing. The other-repository case failed because every arm linted
  the other repository's own remote branch instead of the local files; that is behaviour, so the
  case still fails the smoke candidates, and it stays a `guard` (base kept the local lint).
- The contamination detector flagged every trial: a Go import path `github.com/insajin/...` in a
  here-document or a sed edit matched `github\.com`. It now judges only commands that run
  (`network_c.py`). Of the 19 smoke flags one is left: a `git -C s push` to a local bare fixture the
  same command created. A push or fetch names a remote whose URL the command line does not show, so
  it stays flagged (conservative); a `git clone` of a local path is no longer flagged.


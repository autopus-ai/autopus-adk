# Design: `auto harness` (session waste → approved rules)

PRs #382, #383 (2026-11-27). Acceptance criteria: `.autopus/specs/INTENT-harness-feedback/`.

## Problem

The harness could not see its own cost. Pilot C found test waits taking 56-64% of a trial and a second
review of the same change 41-58%, but only because a person ran a benchmark and read the transcripts.
`auto checklist` already learns from missed defects; nothing learned from wasted time.

## Decisions

| Decision | Why |
| --- | --- |
| Measure from the Claude Code transcript, in a detached `auto harness observe` the Stop hook starts | No model call and no change to the hook's answer or timing (measured 0.1 s). The transcript is the only record of tool spans, reviews and hook blocks. |
| Read the transcript incrementally (byte offset plus open spans in the session record) | Transcripts reach 430 MB; a full read per stop would cost seconds every turn. |
| A pattern repeats at 3 sessions | The user's choice: one or two sessions can be chance. |
| New patterns are drafted and refuted automatically, once per pattern | The user's choice. Once per pattern bounds the cost: a handful of pattern kinds per project. |
| A draft fails into a backoff (6 h, doubling, 3 automatic tries) | Otherwise every stop pays again for the same failure. |
| Refutation by a provider other than the drafter; any refuted verdict refutes | Same rule as `auto checklist`; with no other provider the candidate waits. |
| Refutation and approval records live in the git directory, with a digest of the rule text | A candidate file is a working-tree file anyone or any agent can edit; a commit cannot write the git directory. |
| Approval writes `.claude/rules/harness-lessons.md` into every worktree; never a file git tracks or a person wrote | The user chose a direct rule edit. `auto update` neither writes nor prunes that path, so the rule survives updates. Approvals are per machine, so a committed team file is left alone. |
| Providers run in an empty temporary git repository | In the project, a claude draft browsed the repository with its read tools (~51k tokens); in an empty repository it costs 11-20k. codex exec refuses to run outside a git repository. |
| Effect of an approved rule: sessions before vs after, judged after 5 sessions, a fifth less is "reduced" | A rule that does not reduce its waste should be retired, the way `auto checklist` retires unused checks. |
| Every background write goes through `os.Root` and refuses a symlink | The writer runs unattended at every stop; a committed symlink must not send it outside the project. |

## Rejected

- **Benchmark-validated approval** (run Pilot C per candidate): $60-80 and hours per run; left to a person.
- **Drafting only on request**: offered; the user chose automatic drafting.
- **Editing the generated rule files** under `.claude/rules/autopus/`: `auto update` backs them up and overwrites them.
- **Stripping line numbers / counting by text match** in parsing: each heuristic was tested against this machine's real
  transcripts and replaced when it miscounted (agent messages as prompts, fix agents as reviews, parallel reviews as
  repeats, sibling Stop hooks ending a redo).

## Limits

- Claude Code only: it is the one platform whose Stop hook hands over a transcript path.
- The classification reads Claude Code's transcript format, which is not a published contract.
- "Reduced" can be regression to the mean: the sessions before an approval include the ones that made the pattern
  repeat. "No effect" is the stronger result.
- No rule has yet been approved from real sessions; the evidence so far is synthetic sessions and real refuters
  rejecting thin drafts.

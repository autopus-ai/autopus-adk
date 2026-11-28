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

## Lessons become checks (`auto harness encode`)

Acceptance criteria: `.autopus/specs/INTENT-harness-encode/`. The trust pyramid from "All Code Is Slop (Except Mine)"
(AI Engineer, 2026-11): a mistake is cheapest to stop where code makes it impossible, and costliest where a person or a
text rule has to catch it. An approved lesson was only text.

| Decision | Why |
| --- | --- |
| The harness gives the order and the proof; the work agent writes the check | ADK runs in every user's repository and cannot know its lint stack. The work agent can read the codebase. The drafting provider stays in an empty repository (above). |
| Layers in order: api, lint, test, hook, text | The pyramid, strongest first. A lint diagnostic reaches the agent at the moment it makes the mistake; a text rule may not be read. |
| Proof is three runs by the harness: current tree passes, snapshot copy passes, copy plus a reproduction patch fails | Self-report is not proof. The copy run ties the failure to the patch rather than to files the copy lacks (gitignored builds, dependencies). |
| The copy is a new repository in a temporary directory: the tree is read through a throwaway index (`git add -A --sparse`, `write-tree`, `commit-tree`) and checked out there with the project's objects as alternates, hooks off | The check the agent just wrote is usually uncommitted and untracked. A linked worktree shared the user's stash, refs and hooks, and its cleanup (`worktree prune`) could delete unrelated worktrees (review of this change). |
| Git's repository variables (`git rev-parse --local-env-vars`) are removed for every git call and check run | Run from a git hook, an inherited `GIT_INDEX_FILE` made the snapshot overwrite the user's index. |
| The proof runs in the caller's worktree; records and the rule file stay with the repository | Agents work in linked worktrees; proving the main checkout proved a tree without the check. |
| The patch is committed in the copy, and a patch that edits the script the check runs, or leaves the check unable to start (126/127), is refused | Otherwise `git diff --quiet`, or a patch that rewrites the lint script to `exit 1`, proved anything. |
| The proof record lives in the git directory, bound to the lesson's digest | Same reason as approvals: a working-tree file cannot claim a proof, and an edited lesson is unproven again. |
| A proven lesson leaves the rule file automatically; a failed proof leaves it temporary and retryable; text with a reason stays until a later proof replaces it | The defaults the intent questions recommended. Text with a reason is no worse than before this change. |

Limits:
- A patch that breaks the build, or edits a config file the check reads, makes a check fail for a reason unrelated to
  the lesson. The record keeps the patch, the files it touched and the check's failing output, and
  `auto harness status` shows them for a person to read.
- The proof applies to every worktree. If the check exists only on the branch where it was proven (shown in
  `status`) and that branch is abandoned, the lesson is enforced nowhere; retire and re-approve it, or prove it again.
- The next step adds repeated review findings as a lesson source, since those are the mistakes lint rules usually catch.

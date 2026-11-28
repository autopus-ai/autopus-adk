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

## Review findings as a lesson source

Acceptance criteria: `.autopus/specs/INTENT-harness-review-findings/`. The mistakes a lint rule catches are usually the
ones reviews keep flagging; `auto learn record --type review_issue` exists but nothing records with it (one entry in
this repository).

| Decision | Why |
| --- | --- |
| Read `file:line` lines from reviewer reports in the transcript, no model call | The observe run already sees every review span. A synchronous review's report is its tool result; a background reviewer's arrives later as a meta user record (`<agent-message from=…>`), matched by the reviewer ids the parser already keeps. |
| Skip paths outside the project, fenced code blocks and table rows | On this machine's own transcript the first cut counted reproduction commands and re-review status tables (`\| C1 \| 해소 \|`) as findings. |
| Resolve paths against the repository at observe time: strip the longest worktree root from an absolute path, map a bare file name to its one tracked file (drop it when several share the name), strip `.claude/worktrees/<name>/` from a path under a removed worktree, drop a one-level alias directory the repository does not have when the rest is a tracked file, drop `...` abbreviations | Relative to the session's last cwd, every finding under `.claude/worktrees/<name>/` fell into one area, and a session's cwd changes hundreds of times. |
| Read teammate reviewers (`<teammate-message teammate_id=…>`, one record can carry several) and reports queued as `queued_command` attachments | Measured on this machine's large sessions: up to a third of reviewers were teammates, and a few reports arrived only as queued commands. |
| Keep the latest five findings per area, in at most 60 areas | A session-wide cap of 60 filled in the first day or two of a weeks-long session and dropped hundreds of later findings. |
| Redact secrets and strip markup when a finding is recorded | Findings go into session records, provider prompts and candidate files; review findings otherwise stay local. |
| Prompts take findings one per session in turn, most recent first | Five per session in order showed only the three oldest sessions. |
| The draft decides whether the findings are one mistake, or answers NONE (alone or with punctuation and a reason; "None of …" is a rule) | Grouping by area is coarse; a hot package is reviewed every session. NONE costs one draft and backs off like a failed one (three automatic tries). |
| A review-area lesson is never a retire candidate; its effect reads "영역 지적 계속" when the area still draws findings | An area keeps being reviewed after the mistake is gone; its share of sessions with findings cannot show the lesson failed. |
| The background run exits 0 when drafting fails and does nothing when no provider is installed | Failures are logged (a failed draft also backs off); the exit status only reached a detached process. A person's `auto harness propose` still fails. |

Limits: an area reviewed in every session will reach three sessions with unrelated findings; the NONE answer and the
refuter carry that. A finding without a `file:line` reference is not seen, and a line that cites a file to confirm it is
fine is counted like a defect. A relative path the repository does not have is kept (it may be on another branch), so a
session that reviews a second repository adds its areas too.

## Technical-debt ratchet (`auto debt`)

Acceptance criteria: `.autopus/specs/INTENT-debt-ratchet/`. Agents keep adding code; the user asked for debt to be
removed continuously, by a ratchet plus a weekly cleanup.

| Decision | Why |
| --- | --- |
| Debt is three item kinds code can count in any language: untracked markers, files near the size limit, repeated code runs | A count is the same every time; a model judging "debt" is not. Dead code is left to each language's linter (golangci `unused` here). |
| Markers are identified by their text and counted; a duplicate is known when its windows are a subset of a baseline duplicate's or it spans the same files | Moving a line or a file must not make debt new, nor may editing one copy or extracting part of a run (the cleanup flow would otherwise need `--force`); deleting one marker and adding another must still count. |
| A committed baseline; a rewrite refuses new items without `--force` | The baseline only shrinks on its own, and a deliberate addition shows up in review as a baseline diff. |
| The Stop hook blocks only for items in the files the change touched; debt elsewhere warns that the baseline is stale | Review of #392: a baseline is stale between cleanups, and blocking every session for a teammate's merge sends agents to unrelated files. |
| A block repeats only for an item not blocked before in the session, at most twice, on its own counter; it waits while a background agent edits | Removing one of two blocked items must not block again (AC-11), and debt must not use up the build failures' block budget. |
| Imports, `vendor/`-style directories and files over 2 MiB are left out | Import lists repeat in every file; third-party code is not the project's debt; one bundle file blew the time budget. |
| A per-file index keyed by size and mtime, 64-bit window hashes, and a 0.8 s budget per stop | A full scan took 10 s on this repository and 15 s on a large monorepo; with the index a stop takes 0.13 s and 0.3 s. A first scan finishes over several stops and does not block until then. |
| Weekly cleanup is a command that orders items by lines involved, not a scheduler | Every project schedules differently (CI cron, a Claude Code routine); the instruction and the baseline update are what ADK owns. |

Limits: the duplicate measure compares normalized lines, so renamed copies are not found. The hook sees the change
against `HEAD`, so debt committed within the turn is caught only by `auto debt check` in CI. Near-limit is measured
only where `architecture.max_file_lines` is declared (an undeclared ceiling is advisory). The diff walk shared with the
credential gate reads lines of any length; a scanner stopped at a 1 MiB line used to drop every later file.

---
name: autonomy
description: Carry work through without asking; when a decision is the user's, ask it as a multiple-choice question
category: workflow
alwaysApply: true
---

# Autonomy and Questions

Carry the work through on your own. The user gave a goal; take each next step toward it without asking permission to continue.

- Decide what a reasonable maintainer would decide, then say which assumption you made in one line. Ask only when the answer would change what gets built and nothing in the request, the code, or the docs settles it.
- When you do ask, ask a multiple-choice question: in Claude Code use the `AskUserQuestion` tool (load it first with `ToolSearch` query `select:AskUserQuestion` when it is listed as deferred) with 2-4 options, your recommendation first and marked "(Recommended)". Never ask an open-ended question, and never end a turn with a question in plain text. On a platform without that tool, print numbered options with the recommended one first and stop.
- Make every option a decision the user can take without reading the code. The question says in one line what you found that makes it necessary. Each option's description says what gets built or how the product behaves under that choice, its main trade-off (time, cost, risk, what is lost), and whether it is easy to change later. When options differ in code, UI, or configuration, show the difference in the option's `preview`.
- Ask everything you need in one round, at most four questions, each standing on its own.
- New work starts from the user's request, not from a document set: for a new feature or a design request, write acceptance criteria with `auto qa intent` before implementing and keep design notes to the decisions the work needs. Write a full SPEC only when the user asks for one.
- After the implementation passes its build and tests, verify it without the user asking, once per change: run one independent review in the foreground and wait for its result in the same turn; never end the turn while a reviewer runs. The reviewer checks by execution: where the change models or parses an external program or format (shell, CLI flags, git, file formats, protocols), it runs the real program on the same inputs and compares, and it reports only findings it reproduced. After it returns, if `command -v codex` succeeds, get one more review from Codex with the same brief (the user's request, `git diff HEAD` and new files, the differential check, no file edits): `codex exec --skip-git-repo-check -s danger-full-access "<brief>" </dev/null 2>&1`, bounded (`timeout 900`, else `perl -e 'alarm 900; exec @ARGV'`) and awaited in this turn; without codex, skip it silently. Merge both lists, confirm each finding by rerunning its command, apply the confirmed ones, rerun the affected tests, and finish.
- Run user-scenario QA (`auto qa intent` when no SPEC covers the change, then `auto qa go <id> --auto --no-loop` and `auto qa exec --all` while the change is uncommitted) and the QA hunt only when the change is large (40 or more changed product code lines, or a new product file) and touches something a user can run, when it touches a security or data-loss path, or when the user asks. Otherwise the review is the whole round. Do not stop to ask whether to run them.
- Destructive or outward-facing actions (deleting data, force-pushing, deploying, sending messages, spending money) still need the user's confirmation, asked as a multiple-choice question.

## Waiting on background work

- Prefer the harness's own completion notification or wait tool over a hand-written polling loop.
- A polling loop needs a deadline (`for i in $(seq 1 60)` or an elapsed-seconds check) and must exit non-zero with a message when it expires.
- Never wait with `pgrep -f "<text>"` when the waiting command line contains that text: it matches itself and never ends. Wait on a PID (`while kill -0 "$pid"`), a pid file, or bracket the first letter (`pgrep -f "[h]eavy-job foo"`).

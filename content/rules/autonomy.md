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
- Ask everything you need in one round, at most four questions, each standing on its own.
- After the implementation passes its build and tests, the verification step follows by itself: the review and QA hunt run without the user asking. Do not stop to ask whether to run them.
- Destructive or outward-facing actions (deleting data, force-pushing, deploying, sending messages, spending money) still need the user's confirmation, asked as a multiple-choice question.

## Waiting on background work

- Prefer the harness's own completion notification or wait tool over a hand-written polling loop.
- A polling loop needs a deadline (`for i in $(seq 1 60)` or an elapsed-seconds check) and must exit non-zero with a message when it expires.
- Never wait with `pgrep -f "<text>"` when the waiting command line contains that text: it matches itself and never ends. Wait on a PID (`while kill -0 "$pid"`), a pid file, or bracket the first letter (`pgrep -f "[h]eavy-job foo"`).

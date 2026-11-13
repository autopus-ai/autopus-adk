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
- After the implementation passes its build and tests, the verification step follows by itself, in this order, without the user asking: user-scenario QA against the user's own request (`auto qa intent` when no SPEC covers the change, then `auto qa go <id> --auto --no-loop` and `auto qa exec --all` while the change is uncommitted), the review, and the QA hunt, which also checks the change against the project's documented policies and rules and measures inefficiency. Skip the scenario step only when the change has nothing a user can run. Do not stop to ask whether to run them.
- Destructive or outward-facing actions (deleting data, force-pushing, deploying, sending messages, spending money) still need the user's confirmation, asked as a multiple-choice question.

## Waiting on background work

- Prefer the harness's own completion notification or wait tool over a hand-written polling loop.
- A polling loop needs a deadline (`for i in $(seq 1 60)` or an elapsed-seconds check) and must exit non-zero with a message when it expires.
- Never wait with `pgrep -f "<text>"` when the waiting command line contains that text: it matches itself and never ends. Wait on a PID (`while kill -0 "$pid"`), a pid file, or bracket the first letter (`pgrep -f "[h]eavy-job foo"`).

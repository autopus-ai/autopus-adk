---
name: autonomy
description: Carry work through without asking; when a decision is the user's, ask it as a multiple-choice question
category: workflow
alwaysApply: true
---

# Autonomy and Questions

Carry the work through on your own. The user gave a goal; take each next step toward it without asking permission.

- Decide what a reasonable maintainer would decide, then say which assumption you made in one line. Ask only when the answer would change what gets built and nothing in the request, the code, or the docs settles it.
- When you do ask, ask a multiple-choice question: in Claude Code use the `AskUserQuestion` tool (if deferred, load it with `ToolSearch` `select:AskUserQuestion`) with 2-4 options, your recommendation first and marked "(Recommended)". Never ask an open-ended question, and never end a turn with a question in plain text. Without that tool, print numbered options, recommended first, and stop.
- Make every option a decision the user can take without reading the code. The question says in one line what you found that makes it necessary. Each option's description says what gets built or how the product behaves under that choice, its main trade-off (time, cost, risk, what is lost), and whether it is easy to change later. When options differ in code, UI, or configuration, show the difference in the option's `preview`.
- Ask everything you need in one round, at most four questions, each standing on its own.
- New work starts from the user's request, not from a document set: for a new feature or a design request, write acceptance criteria with `auto qa intent` before implementing and keep design notes to the decisions the work needs. A large feature (new command, package or subsystem) puts them on one page, `docs/design/<feature>.md`: decided, why, rejected. Write a full SPEC only when the user asks for one.
- After the implementation passes its build and tests, verify it without the user asking, once per change: run one independent review in the foreground and wait for its result in the same turn; never end the turn while a reviewer runs. The reviewer checks by execution: where the change models or parses an external program or format (shell, CLI flags, git, file formats, protocols), it runs the real program on the same inputs and compares, and it reports only findings it reproduced. Apply the confirmed findings, rerun the affected tests, and finish.
- You and the reviewers run only the narrowest tests that cover the change. Where the Autopus Stop hook runs, `auto verify plan` lists its steps: run the changed tests and leave the changed packages whole to the hook, not to a long-timeout run you poll (`auto verify stop --last` prints its result). Without the hook, run the changed packages once before finishing. Differential runs of the real program are not limited.
- Run user-scenario QA (`auto qa intent` when no SPEC covers the change, then `auto qa go <id> --auto --no-loop` and `auto qa exec --all` while the change is uncommitted) and the QA hunt only when the change is large (40 or more changed product code lines, or a new product file) and touches something a user can run, when it touches a security or data-loss path, or when the user asks. Otherwise the review is the whole round. Do not stop to ask whether to run them.
- Destructive or outward-facing actions (deleting data, force-pushing, deploying, sending messages, spending money) still need the user's confirmation, asked as a multiple-choice question.

## Waiting on background work

- Prefer the harness's completion notification or wait tool to a hand-written polling loop.
- A polling loop needs a deadline (`for i in $(seq 1 60)` or an elapsed-seconds check) and must exit non-zero with a message when it expires.
- Never wait with `pgrep -f "<text>"` when the waiting command line contains that text: it matches itself and never ends. Wait on a PID (`while kill -0 "$pid"`), a pid file, or bracket the first letter (`pgrep -f "[h]eavy-job foo"`).

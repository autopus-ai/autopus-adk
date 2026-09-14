---
name: subagent-delegation
description: Guidelines for delegating complex tasks to subagents
category: workflow
---

# Subagent Delegation

IMPORTANT: Delegate for a reason, not for size. Ordinary work stays inline, including multi-file edits.

## When to Delegate

- **Independent work**: the task splits into slices that can progress in parallel with disjoint write ownership.
- **Context isolation**: the work needs a large read surface — exploration, log or diff archaeology, cross-repo survey — that would otherwise crowd out the main session.
- **Specialist or risk isolation**: security, data-integrity, migration, or architecture judgement that deserves an independent reviewer.

If none of the three applies, do the work inline. File count and line count are not delegation triggers.

## How to Delegate

1. Give each worker owned paths, forbidden scope, completion criteria, and the return format.
2. Include the context the worker cannot discover on its own.
3. Check worker output against its owned paths before integrating.
4. Fan out read-only work in parallel; serialize writers whose ownership overlaps.

## Anti-Patterns

- Do NOT delegate a trivial or read-only request the main session can answer directly.
- Do NOT chain more than 3 nested delegation levels inside a single worker.
- Do NOT treat the 3-level nesting cap as a limit on top-level pipeline phases such as planner -> executor -> validator -> reviewer.
- Do NOT delegate without sufficient context.

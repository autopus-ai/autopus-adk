---
name: worktree-isolation
description: Worktree isolation for parallel agent execution in the pipeline
triggers:
  - worktree
  - isolation
  - 워크트리
  - 격리
  - parallel agent
category: agentic
level1_metadata: "worktree isolation, file ownership, merge strategy, parallel executor"
---

# Worktree Isolation Skill

Integrated skill for worktree isolation in the multi-agent subagent pipeline. Ensures parallel executor agents work in independent git worktrees and safely merge results back into the working branch.

## Overview

When the default subagent pipeline runs Phase 2 with parallel tasks, each executor agent receives `isolation: "worktree"` so Claude Code places it in a separate git worktree. After all parallel agents complete, their branches are merged sequentially (Phase 2.1) before Gate 2 validation.

**Ref**: SPEC-WORKTREE-001

## Activation Conditions

Applied when:
- Default subagent pipeline mode (no `--team` or `--solo` flag)
- Phase 2 has tasks with `Mode = "parallel"`

NOT applied when:
- `--solo` mode (main session works directly)
- `--team` mode (Agent Teams manages isolation independently)
- Tasks with `Mode = "sequential"` (use main worktree)

## Disk Preflight

Before spawning parallel executors, run `auto disk check --need 10` once and fan out only on exit 0; stop with a report when it exits 3 (insufficient free space) or with any other non-zero code (the measurement failed). Each worktree adds a checkout, build output, and test caches on a disk every worker shares, and a full disk fails all of them with misleading errors.

A worktree created outside the harness worktree directories is registered with `auto disk register <path>` and released with `auto disk release <path>` when its work ends, so `auto disk reclaim` can remove it once it is clean. Run a full build or test suite as `auto disk lease --need <GiB> -- <command>` so parallel workers reserve disk instead of racing for it; exit 3 means the wait ran out. When a disk alert or a failed preflight appears, the lead tells every running worker to pause heavy work.

## Agent Tool Usage (R1)

For parallel tasks in Phase 2, add `isolation: "worktree"` to Agent() calls:

```python
# Parallel execution with worktree isolation
result_t1 = Agent(
    description = "Implement task T1 in its isolated owned paths",
    prompt = "Implement T1: ...",
    subagent_type = "executor",
    isolation = "worktree"
)
result_t2 = Agent(
    description = "Implement task T2 in its isolated owned paths",
    prompt = "Implement T2: ...",
    subagent_type = "executor",
    isolation = "worktree"
)
```

**Constraints:**
- Max concurrent worktrees / worktree slot cap: **5**. Overflow tasks are queued by `queue_discipline = "fifo_task_id"` and spawned as slots free.
- Slot evidence records `active_task_ids`, `queued_task_ids`, `slot_count`, `cap`, and reason `worktree_slot_cap`.
- Required worktree isolation must fail closed with `worktree_isolation_unavailable` unless an explicit `override_reason` is present.
- Slot reclaim records exactly one terminal state: `merged`, `discarded`, `preserved_for_manual_review`, or `cleanup_failed`.
- Collect `worktree_path` and `branch` from each agent return value for Phase 2.1 merge.

Sequential tasks do NOT use `isolation: "worktree"`:

```python
# Sequential — main worktree
result_t3 = Agent(description="Implement dependent task T3", subagent_type="executor", prompt="Implement T3 using T1 result: ...")
```

## File Ownership Conflict Detection (R2)

Before spawning parallel agents, the planner checks all file ownership patterns for conflicts.

**Algorithm:**
1. Split each pattern into directory prefix and file glob (e.g., `pkg/auth/*.go` → prefix `pkg/auth/`, glob `*.go`)
2. **Check 1 — Prefix containment**: If prefix A contains prefix B (or vice versa), mark as conflict
3. **Check 2 — Glob intersection**: In the same directory, check if one glob is a subset of the other (`*_test.go ⊂ *.go`)
4. **Check 3 — Different directories**: Non-conflicting by definition
5. **Check 4 — Migration numbering lane**: If both tasks may create SQL migrations in the same owning repo and migration directory, mark as conflict even when code ownership is otherwise disjoint

**Conflict examples:**

| Pattern A | Pattern B | Result |
|-----------|-----------|--------|
| `pkg/core/*.go` | `pkg/core/handler.go` | Conflict (glob contains literal) |
| `pkg/auth/*.go` | `pkg/auth/*_test.go` | Conflict (`*_test.go` ⊂ `*.go`) |
| `pkg/auth/*.go` | `pkg/api/*.go` | No conflict (different directories) |
| `pkg/core/` | `pkg/core/sub/` | Conflict (prefix containment) |
| `Autopus/backend/migrations/*.sql` | `Autopus/backend/migrations/*.sql` | Conflict (same migration numbering lane) |
| `Autopus/backend/migrations/*.sql` | `other-service/migrations/*.sql` | No conflict (different owning repo/directory) |

**On conflict**: downgrade both tasks to sequential execution (lower task-ID first) and log:

```
[CONFLICT] T{a}, T{b} → sequential (overlapping: {pattern})
```

## Worktree Lifecycle (R3)

### Creation

Claude Code automatically creates a worktree when `isolation: "worktree"` is passed to Agent(). No manual `git worktree add` is needed.

### Preservation / Cleanup

- Agent returns with `worktree_path` / `branch` (changes exist) → record for Phase 2.1 merge
- Agent returns without worktree info (no changes) → Claude Code auto-cleans the worktree

### Sequential Task Merge

Merge immediately after the sequential task completes, before spawning the next dependent task. This guarantees the downstream task sees the prior task's changes:

```bash
git -c gc.auto=0 merge worktree/SPEC-XXX-001/T3
git worktree remove <path>
```

### Parallel Task Batch Merge — Phase 2.1

After **all** parallel executors complete, merge their branches sequentially in ascending task-ID order into the working branch:

```
Phase 2 (parallel)  →  Phase 2.1 (merge T1, T2, … in ID order)  →  Gate 2 (validation)
```

```bash
# Phase 2.1 example for T1, T2
git -c gc.auto=0 merge worktree/SPEC-XXX-001/T1
git worktree remove <path_t1>
git -c gc.auto=0 merge worktree/SPEC-XXX-001/T2
git worktree remove <path_t2>
```

### Post-Merge Cleanup

```bash
git worktree remove <path>   # after each successful merge
```

## Merge Conflict Handling (R4)

R2's ownership validation should prevent conflicts. A conflict signals a validation gap.

**On merge conflict:**
1. `git merge --abort` — restore clean state
2. Log conflicting files and task IDs
3. Abort the pipeline; report to user:

```
[MERGE ERROR] T{id} merge failed: ownership validation gap detected. Files: {list}
```

4. Do **not** attempt automatic resolution — even in `--auto` mode — to prevent data loss.

## Safety Rules (R5)

### GC Suppression

Prepend `git -c gc.auto=0` to all git commands while parallel executors are running (both main worktree and each isolated worktree). This prevents garbage collection triggered by `git add` or `git commit`.

### Shared Resource Lock Retry

Each worktree has its own index, so `index.lock` conflicts do not occur between worktrees. However, shared resources can still lock:

- `.git/refs.lock`
- `.git/packed-refs.lock`
- `.git/objects/` pack files
- `.git/shallow.lock`

On lock error, retry with exponential backoff:

| Attempt | Wait |
|---------|------|
| 1st retry | 3 s |
| 2nd retry | 6 s |
| 3rd retry | 12 s |

After 3 retries (4 total attempts), abort the affected agent, log the error, and let other parallel agents continue.

### Failure Cleanup

When an agent fails or is aborted:

```bash
git worktree remove --force <path>
```

Exclude the failed agent's changes from the Phase 2.1 merge list.

## Branch Naming Convention (R7)

Recommended format (for manual worktrees or future custom branch support):

```
worktree/{SPEC-ID}/{task-id}
```

Examples:
- `worktree/SPEC-AUTH-001/T1`
- `worktree/SPEC-WORKTREE-001/T2`

Note: Claude Code auto-generates branch names when `isolation: "worktree"` is used. This convention is advisory for log traceability and manual worktree creation.

## Multi-Session Parallel Workflow Guide

For large SPECs requiring multiple tmux/terminal sessions:

**Session isolation pattern:**
- Each session works on a distinct SPEC or non-overlapping task set
- Apply file ownership separation: session A owns `pkg/auth/`, session B owns `pkg/api/`
- Never share a task-ID across sessions

**Example — 2 tmux sessions:**

```
Session A: /auto go SPEC-AUTH-001   # owns pkg/auth/
Session B: /auto go SPEC-API-001    # owns pkg/api/
```

Each pipeline uses its own worktree branches (`worktree/SPEC-AUTH-001/T*` vs `worktree/SPEC-API-001/T*`). Merge to the shared base branch after both pipelines complete, in SPEC completion order.

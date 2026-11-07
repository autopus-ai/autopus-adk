---
name: worktree-safety
description: Worktree safety rules for parallel executor agents running in isolated git worktrees
category: harness
condition: '\bgit\s+(gc|prune|repack)\b'
scope: tool:bash
---
# Worktree Safety Rules

IMPORTANT: These rules apply whenever parallel executor agents run in isolated worktrees during pipeline Phase 2.

## Prohibited Commands During Parallel Execution

WHILE parallel executors are running in worktrees, the following commands are PROHIBITED in both the main worktree and all isolated worktrees:

- `git gc` — may delete objects referenced by other worktrees
- `git prune` — may remove unreachable objects still in use
- `git repack` — may cause pack file lock contention

## GC Suppression

WHILE parallel executors are active, ALL git commands MUST include the gc suppression flag:

```
git -c gc.auto=0 <command>
```

This prevents automatic garbage collection triggered by `git add`, `git commit`, or other commands.

## Shared Resource Lock Handling

Each worktree has its own independent index, so `index.lock` conflicts do not occur between worktrees. However, lock conflicts on shared resources are possible.

### Shared Resources Subject to Lock Contention

- `.git/refs.lock`
- `.git/packed-refs.lock`
- `.git/objects/` (pack files)
- `.git/shallow.lock`

### Retry Strategy

WHEN an agent encounters a lock file error on shared resources:

1. Wait 3 seconds, retry (attempt 2)
2. Wait 6 seconds, retry (attempt 3)
3. Wait 12 seconds, retry (attempt 4)

Exponential backoff: base = 3s, factor = 2.

### On Retry Exhaustion

WHEN the 3rd retry also fails (total 4 attempts):
- Abort the affected agent
- Log the error with lock file path and agent ID
- Allow other parallel agents to continue unaffected
- Do NOT retry further or force-remove lock files

## Failure Cleanup

WHEN an agent fails or is aborted:
1. Execute `git worktree remove --force <worktree-path>` to clean up
2. Exclude the failed agent's changes from the merge target
3. Log: `[CLEANUP] Worktree removed: <path> (agent: <id>, reason: <failure>)`

## Concurrent Worktree Limit

Maximum simultaneous worktrees: **5**

WHEN more than 5 parallel tasks are assigned:
- Execute the first 5 in worktrees
- Queue remaining tasks
- As each worktree completes and is cleaned up, dequeue the next task

## Worktree Removal Safety

Before removing a worktree:
1. Verify no active processes are using the worktree directory
2. Check for uncommitted changes (warn if present)
3. Use `--force` only for failed/aborted agents
4. For successful merges, use standard `git worktree remove <path>`

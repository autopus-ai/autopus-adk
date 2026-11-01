package codex

func codexWorktreeIsolationSkillBody() string {
	return `
# Shared Workspace Ownership Skill

Codex Multi-Agent V2 workers use one shared cwd and filesystem. A spawned worker
is not placed in an implicit worktree, separate checkout, or isolated branch.
` + "`fork_turns`" + ` changes conversation history only.

## Parallel Safety Gate

Parallel writes are safe only with disjoint write ownership. Before dispatch:

1. Assign exact owned paths to every writer.
2. Reject same-file, parent/child, generated-output, and migration-number overlap.
3. Name forbidden paths and the focused verification each worker may run.
4. Run overlapping writers sequentially.
5. Run ` + "`auto disk check --need 10`" + ` and dispatch only on exit 0. Stop with a
   report when it exits 3 (insufficient free space) or with any other non-zero
   code (the measurement failed): every writer builds and tests on the one disk
   they share.

Tasks that may create SQL migrations in the same owning repo and migration directory
share one migration numbering lane and must run sequentially. Assign
the final migration number only after every earlier task in that lane has
completed and its files are visible in the shared cwd.

` + "```python" + `
spawn_agent(
    task_name="executor-auth",
    message="""
    Shared cwd/filesystem.
    Disjoint write ownership: pkg/auth/** only.
    Forbidden: migrations/** and all other packages.
    Return exactly: owned_paths, changed_files, verification, blockers,
    next_required_step.
    """,
)
` + "```" + `

Do not merge or copy spawned-worker changes. They are already visible in the
shared filesystem. The supervisor reviews the returned path receipt and runs the
integration gate only after all writers have stopped.

## V2 Coordination

Use ` + "`spawn_agent(...)`" + `, ` + "`send_message(...)`" + `,
` + "`followup_task(...)`" + `, target-less ` + "`wait_agent()`" + `,
` + "`interrupt_agent(...)`" + `, and ` + "`list_agents()`" + `.
Do not invent other collaboration lifecycle calls.
`
}

// @AX:NOTE [AUTO]: the six-tool Multi-Agent V2 surface is a contract, not a release note; update it with native collaboration schema changes.
func codexSubagentDevSkillBody() string {
	return `
# Codex Multi-Agent V2 Development Skill

Design workers around the six Multi-Agent V2 collaboration tools:

- ` + "`spawn_agent(task_name, message, ...)`" + ` for a new scoped task
- ` + "`send_message(...)`" + ` for coordination
- ` + "`followup_task(...)`" + ` for additional work
- target-less ` + "`wait_agent()`" + ` for the next event
- ` + "`interrupt_agent(...)`" + ` to stop unsafe or obsolete work
- ` + "`list_agents()`" + ` to inspect current state

All workers share the same cwd and filesystem. Context forking never creates a
separate workspace. Give parallel writers disjoint write ownership and serialize
any overlapping changes.

Harness role definitions live under ` + "`.codex/agents/`" + `. Every worker
prompt includes the SPEC or requirement, exact owned paths, forbidden paths,
completion criteria, and this exact five-field receipt:
` + "`owned_paths`" + `, ` + "`changed_files`" + `, ` + "`verification`" + `,
` + "`blockers`" + `, ` + "`next_required_step`" + `.

Use fan-out only for independent slices. Use a sequential planner → executor →
validator pipeline when later work depends on earlier output. The main session
remains the supervisor and owns integration decisions.
`
}

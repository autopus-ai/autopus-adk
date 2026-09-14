# Quality, Permissions, and Run Observability

Read this file at Phase 0, or when a run needs a non-default quality, permission,
or monitoring setting.

## Quality mode resolution

Resolve the effective quality mode in this order:

1. explicit per-run `--quality`;
2. the provider override for the active platform under `quality.providers.*`;
3. `quality.default`;
4. `balanced` as the safety fallback.

The canonical persisted provider keys are the installed platform names, so a
mixed project can keep one provider on Ultra while another uses Balanced.
`auto quality provider <name> <preset|inherit> --apply` changes one provider and
refreshes only its configured platform. `auto quality <preset> --apply` keeps
its existing behaviour and refreshes every configured platform.

Quality is projected into the installed agent definitions as a model-and-effort
pair before the session starts. Ordinary worker dispatches inherit that pair:
they do not carry a per-call model, effort, or permission field. A user-supplied
per-run model override is the only reason to send an explicit model.

Never translate tier labels into guessed provider or model ids, and never force
a cheaper model than the one the definition resolves.

## Balanced, Ultra, and adaptive selection

- **Balanced** uses each installed definition's authored model-and-effort pair.
  Task complexity selects between the premium and standard tier for that role.
- **Ultra** uses the Ultra role profile for every installed agent definition.
  Complexity does not lower an assigned tier; the role-specific assignment
  stays intact.
- An agent that is not defined in the selected preset keeps its authored
  frontmatter defaults.

Platform projection notes live with the `adaptive-quality` skill; this pipeline
consumes the resolved pair rather than re-deriving it.

## Effort override

Session-level effort controls exist on some platforms while ordinary worker
dispatches accept no per-call effort field. Resolve the explicit `--effort`
value and any session effort environment variable before generation, then
project the result into the agent definition's effort alongside its model.

## Permission mode

`auto permission detect` inspects the parent process tree and returns:

- `bypass` — a permission-bypass flag is present in the parent session;
- `safe` — no such flag, or detection failed.

| Detected mode | Effect |
|---|---|
| `bypass` | every agent runs in the bypass permission mode |
| `safe` | the per-agent modes authored in the definitions are preserved |

Detection failure defaults to `safe`. The pipeline never widens permissions on
its own; it only mirrors what the parent session already granted.

## Prompt layer discipline

Keep stable instructions, frozen snapshot recall, and ephemeral task or tool
context as separate prompt layer manifest entries. Dry-run and debug output
report cache invalidation scope by layer, without exposing raw secrets.

Do not add a second compaction pass on top of the runtime's own context
lifecycle. Let the platform manage its window and keep artifacts retrievable
through stable refs instead of re-summarising them into the prompt.

## Run observability

When the platform exposes a monitoring surface, inject its log path into each
dispatched prompt:

```
## Pipeline Monitor
Log file: <resolved pipeline log path>
Write structured entries: [timestamp] [role] [phase] message
```

Emit an event on each transition. The event vocabulary is closed:

| Event | When |
|---|---|
| `phase_start` | a phase begins |
| `phase_end` | a phase completes |
| `agent_spawn` | a worker is dispatched |
| `agent_done` | a worker finishes |
| `checkpoint` | a checkpoint is saved |
| `error` | an error occurs |
| `blocker` | a blocker is detected |

Session lifecycle: start the monitor session at run start, log and refresh on
each phase transition, and close it at run end so panes and temporary files are
removed. A platform without a monitoring surface skips this section entirely; it
is observability, not a gate.

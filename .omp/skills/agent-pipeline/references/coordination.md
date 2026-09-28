# Native Coordination Payloads

Read this file only when you are about to dispatch, message, or track workers on
a runtime with native coordination tools, and you need the exact field shapes.

**Inspect the runtime's current dynamic tool schema first.** The payloads below
show the contract's shape, not a field list to copy. A field that the live
schema does not expose must be omitted, and a field the live schema requires
must be supplied even if it is absent here. Never infer a capability from this
example.

## Typed worker receipt

Every dispatched worker returns exactly the five-field receipt, and nothing
else. Where the runtime supports a per-item output schema, attach this one and
set the strictest schema mode it offers, so a malformed return fails at the
boundary instead of being parsed out of prose.

```json
{
  "type": "object",
  "additionalProperties": false,
  "required": ["owned_paths", "changed_files", "verification", "blockers", "next_required_step"],
  "properties": {
    "owned_paths": {"type": "array", "items": {"type": "string"}},
    "changed_files": {"type": "array", "items": {"type": "string"}},
    "verification": {"type": "array", "items": {"type": "string"}},
    "blockers": {"type": "array", "items": {"type": "string"}},
    "next_required_step": {"type": "string"}
  }
}
```

`verification` records actual execution. The supervisor decides acceptance from
that evidence, not from a completion keyword or the absence of reported blockers.

## Batch dispatch

When the schema exposes a batch form with a top-level shared context and an
array of items, use one call per independent fan-out wave. Shared goals,
constraints, frozen context references, owned-path rules, and cross-task
interfaces go in the shared context exactly once; only the per-item delta
repeats.


- A stable item `name` is worth setting whenever you may need to address that
  worker again.
- Set a custom agent/role field only to select a non-default role; omit it for
  the runtime's default general worker.
- Isolation and effort are conditional fields. Add them only after confirming
  the live schema exposes that exact field.
- When the schema exposes only a flat single-item form, dispatch one item per
  call and reference one shared context artifact from each item.

## Intent on every call

While the runtime's intent tracing is enabled, every model-authored coordination
call carries a concise top-level intent string. It is not decoration: it is how a
human reading the run reconstructs why a worker exists.

## Follow-up messaging

Keep common facts in stable, versioned references and send task-specific deltas,
blocking decisions and result locations. Choose files or messages to fit the
workload; neither an all-to-all chat nor a shared file is mandatory for every
team. Record integration failures and rework alongside dispatch counts: more
messages, workers or merged branches do not establish a better outcome.

Retain the agent id each dispatch returns. For a worker that is still revivable,
every follow-up goes to that same id through the runtime's messaging tool:


Do not spawn a replacement merely to ask a question. Check the runtime's actual
worker state and resumption capability before messaging or replacing a worker.
Never recreate or delete a runtime-owned isolated workspace manually.

## Parent-owned progress

The supervising session owns the checklist. Workers report receipts; they do not
mutate it. Each checklist call carries one top-level operation plus its intent:


Advance a phase only after the previous phase's receipt and gate are verified.

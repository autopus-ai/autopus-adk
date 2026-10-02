---
name: harness-workflow
description: Opt-in deterministic 4-phase Route A workflow skill for Claude Code only
triggers:
  - workflow
  - deterministic workflow
  - 워크플로우
  - 결정적 워크플로우
category: agentic
platforms:
  - claude
visibility: claude-only
level1_metadata: "Deterministic Route A, --workflow opt-in, 4-phase, exit-code gate, claude-only, fallback taxonomy"
---

# Harness Workflow Skill

## Overview

The harness workflow (`--workflow`) runs the `/auto go` Route A pipeline as a
**deterministic, opt-in Claude Code Workflow** instead of the default
LLM-sequenced subagent pipeline. Phase order, the build/test gate, and release
hygiene checks are fixed by a manifest, so the run is reproducible and its
adjudication does not depend on model judgment.

**Activation flag**: `/auto go SPEC-ID --workflow`

> **claude-code only.** This skill and the `--workflow` route are scoped to the
> claude-code platform. codex, antigravity-cli (gemini), and opencode render
> their own routers and never emit the workflow surface. The skill itself is
> compiled only into the claude surface (`visibility: claude-only`).

## Why claude-only

The deterministic route is built on Claude Code Workflows
(`.claude/workflows/route_a.workflow.js`) — a primitive only Claude Code
executes. The non-claude adapters produce **zero** `workflow*.js` files, **zero**
`--workflow` tokens, and no `harness-workflow` skill. Their Route A `/auto go`
behavior is unchanged (regression-0 guarantee). Adding `--workflow` to the claude
router template therefore cannot leak to other platforms.

## Capability Gate — `auto workflow doctor --route route_a`

Before dispatching, Route A runs `auto workflow doctor --route route_a` to
confirm the runtime can execute the workflow. The gate distinguishes
**required** primitives (a missing one fails the gate) from **advisory**
primitives (reported, non-fatal).

| Requirement | Value | Class |
|-------------|-------|-------|
| Claude Code version | **2.1.246** or later (`RouteAMinVersion=2.1.246`) | required |
| Platform | `claude` only | required |
| Workflow JS present | `.claude/workflows/route_a.workflow.js` | required |
| `auto workflow gate` bridge | resolvable on PATH | required |
| Resumable checkpoint store | available | advisory |

If `auto workflow doctor --route route_a` reports `overall: fail` OR the
platform is not claude, the route emits a `fail-fast` fallback log line and
enters Route A **without executing any workflow**.

Inspect the planned run without executing it:

```
auto workflow render --dry-run
```

## Deterministic 4-Phase Flow

The workflow runs exactly four ordered phases. The phase-ids are authoritative
and match the route manifest schema in the autopus-adk source repo.

1. **planning** — produces the implementation plan and task breakdown; does not
   mutate the working tree beyond plan artifacts.
2. **implementation** — performs the repository-mutating work. Worktree
   creation, branch naming, the worktree slot cap, and worktree reclaim stay in
   the Go runtime (`pkg/pipeline`); the workflow JS owns sequencing only.
3. **gate_build_test** — the deterministic gate. Its verdict derives from build
   and test command **exit codes** (`verdict_source: exit_code`), not from an
   LLM verdict. The gate is executed outside the JS via the Go runtime (calling
   `auto workflow gate`), which runs build and test through an
   injectable `CommandRunner` seam and emits
   `{verdict, verdict_source, build_exit, test_exit}` JSON. A non-zero build or
   test exit code yields `verdict: fail`. The JS carries this phase as a sequencing
   marker with no shell-out primitives embedded.
4. **release_hygiene** — terminal phase that enforces release safety before
   sync:
   - **Generated-surface drift gate** — blocks when generated surfaces
     (`.claude` / `.codex` / `.gemini` / `.opencode` / `.autopus/plugins`) are
     staged without a matching source-of-truth change, and always blocks runtime
     artifacts such as `.autopus/txns` / `.autopus/orchestra`.
   - **300-line source limit** — `auto check --hygiene --arch --quiet --staged`.
   - **Lore commit format** — the commit-msg hook uses
     `auto check --lore --message <msgfile>` for the pending message boundary.

## Fallback Taxonomy

Every workflow failure maps to exactly one class — silent opt-out is forbidden.

| Failure kind | Class | Behavior |
|--------------|-------|----------|
| `non_claude_platform` | `fail-fast` | Abort immediately, fall back to Route A |
| `doctor_fail` | `fail-fast` | Abort immediately, fall back to Route A |
| `parity_drift` | `fail-closed` | Refuse to proceed and block |
| `execution_abort` | `resumable` | Resume from a recorded checkpoint |
| `api_unavailable` | `explicit` | Surface to the operator for a decision |

## Manifest is the Source of Truth

The route manifest in the autopus-adk source repo (markdown plus JSON schema) is
the human- and machine-authoritative definition. The installed Workflow JS
(`.claude/workflows/route_a.workflow.js`) is a **generated, edit-forbidden
surface** derived from that manifest by `auto generate-templates`. Do not edit the
JS by hand — edit the manifest in the ADK source repo and regenerate. A parity
gate compares the
phase-id, retry, budget, and result-type sets across the markdown, the schema,
and the derived JS; any divergence fails generation closed and names the
diverging element.

## When to Use

| Scenario | Use `--workflow`? |
|----------|-------------------|
| Reproducible, gate-driven Route A run on Claude Code | Yes |
| Exit-code-based build/test adjudication required | Yes |
| Running on codex / gemini / opencode | No — flag is ignored, Route A runs as usual |
| Doctor reports `overall: fail` | No — auto fail-fast into Route A |

**Ref**: SPEC-HARNESS-WORKFLOW-001

---

## Retained Route Team Workflow

`route_team` remains a Claude-only generated Workflow for explicit internal use.
It is not the substrate behind `--team`; native named teammates own that flag.

The retained script runs eight manifest-defined phases and uses bounded
`parallel()` executor calls in one shared working tree. Planner tasks must have
disjoint file ownership and keep interdependent implementation and test files in
one task. The script does not request worktree isolation or a merge lifecycle.

Before an explicit retained launch, run
`auto workflow doctor --route route_team`. The route-aware pin is
`RouteTeamMinVersion=2.1.284`; a failed doctor blocks this internal launch and
does not change native `--team` routing.

Launch each required segment with the current Workflow input shape:

```javascript
Workflow({
  scriptPath: ".claude/workflows/route_team.workflow.js",
  args: { spec, workingDir, quality, segment: "A" },
})
```

Subsequent segments use `segment: "B"`, `"C"`, and `"D"`. Deterministic Go
gates still run between segments. `route_team.md` and
`route_team.schema.json` remain authoritative; generated JS must pass parity.

The public `/auto go SPEC-ID --workflow` path launches Route A with the same
object shape:

```javascript
Workflow({
  scriptPath: ".claude/workflows/route_a.workflow.js",
  args: { spec, workingDir, quality, segment: "A" },
})
```

`--workflow` is explicit. There is no workflow-default configuration field or
negative workflow flag, and team routing never implies a Workflow launch.

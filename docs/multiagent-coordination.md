# Multiagent coordination: research and implementation

Reviewed 2026-09-20 against primary sources and the local v0.50.118 candidate.
Reported research results were not reproduced here. Vendor documentation is not
proof of account availability, installed runtime tools, or an ADK adapter mapping.

## Current research

| Source | Finding relevant to ADK | Limit |
| --- | --- | --- |
| [When Agents Coordinate, August 17](https://arxiv.org/html/2608.16801v1) | Shared files reduced output tokens about 42% in a message-heavy eight-agent condition, but added overhead to work already coordinated through files | Synthetic Python workloads on one model/runtime family; not a universal shared-file policy or optimal team size |
| [Towards a Science of Scaling Agent Systems v3, April 8](https://arxiv.org/html/2512.08296v3) | Coordination gains depend on task structure and baseline capability across 260 configurations and six benchmarks | Coding benchmark subsets are small; aggregate relative gains/losses are not Autopus predictions or routing thresholds |
| [CooperBench v2, January 26](https://arxiv.org/html/2601.13295v2) | Communication can reduce conflicts without improving final joint success on interacting feature pairs | Evaluates partially overlapping work, not arbitrary independent fan-out; planning associations are observational |
| [Anthropic, August 13](https://www.anthropic.com/research/multiagent-systems) | Independent discovery and interdependent development have different coordination problems; merged PR activity is not product quality | The vulnerability comparisons use different costs and search scopes; core-only token efficiency is comparable, not a blanket swarm win |

Use a small supervisor/worker structure when the deliverables can be separated.
Make shared interface ownership and dependency edges explicit. Pass stable
artifact references and decision deltas instead of repeating the same bodies in
every message, but do not force a shared-file communication bus on every task.
Evaluate successful integrated outcomes alongside total tokens, elapsed time,
rework and human corrections. These are design inferences, not measured ADK gains.

## Platform support and integration boundaries

| Platform | Primary source | ADK interpretation |
| --- | --- | --- |
| Codex | [Official subagents documentation](https://learn.chatgpt.com/docs/agent-configuration/subagents) | Native subagent support exists. Inspect current schemas and capacity evidence; a configuration request does not establish effective capacity |
| Claude Code | [Agent teams](https://code.claude.com/docs/en/agent-teams) | Native team workflow differs from a single session using subagents. Same-file and tightly dependent work still needs explicit coordination |
| OpenCode | [Agents](https://opencode.ai/docs/agents/) | Primary/subagent workflows and parallel tasks are supported. Preserve explicit team intent; verify the requested lifecycle rather than silently substituting the default task pipeline |
| Antigravity | [Subagents](https://www.antigravity.google/docs/subagents/), [Teamwork](https://www.antigravity.google/docs/teamwork/) | Vendor supports concurrent subagents and a Teamwork workflow. ADK mapping and active-session tools must still be verified; no automatic vendor workflow invocation was added |
| OMP | [Local native integration](../pkg/adapter/omp/omp_readiness_behavior.go) | Existing native task/hub integration and readiness evidence are retained; this change does not advance the pinned runtime or claim a newly executed live cohort |

Gemini CLI also documents [subagents](https://geminicli.com/docs/core/subagents/).
That is separate evidence, not a substitute for Antigravity's own API contract.
Workspace isolation, isolated model context, subagent spawning, and a persistent
team task board are different capabilities; do not infer one from another.

## Implemented coordination fixes

### Dependency-aware local parallel runner

`pkg/pipeline.ParallelRunner` now rejects empty/duplicate IDs and unknown,
self, duplicate or cyclic dependencies before any backend call. It admits only
ready phases within the configured slot cap, choosing ready IDs deterministically.
Direct-parent results are included in the child prompt without silent truncation.
Results remain in input order.

An upstream backend error or failed gate blocks its descendants. Cancellation
stops queued admission and is reconciled at final return. Already-running
backends must still honor context cancellation. Safety records distinguish
`dependency_ready_task_id_admission` from goroutine execution-start ordering;
delegation-denial evidence is retained even on preflight failure.

This exported runner does not create worktrees, grant native host capacity or
prove writer paths are disjoint. Its caller still owns that partitioning. The
default CLI pipeline remains sequential; this patch is not a native team engine.

### Worker ownership evidence

The marked worker-receipt parser checks that each reported changed file is
inside the receipt's declared roots. `pkg/a` does not cover `pkg/ab`. Literal
roots and a trailing `/**` subtree are supported; other globs are rejected.
An empty ownership set cannot accompany nonempty changes. Markerless legacy
output remains compatible.

`workerreceipt.ValidateOwnership(receipt, assignedRoots)` additionally checks
that the worker has not widened a supervisor-supplied scope. A non-nil empty
assignment grants no scope; nil means only self-consistency is checked. The
current parser uses the latter mode: it does not invent trusted assignments.

This validates lexical reports. It does not inspect the actual Git diff, resolve
filesystem symlinks, or enforce OS write permissions. Supervisors must compare
actual artifacts and diffs against the assignment before integration.

### Generated platform contracts

OpenCode no longer describes `--team` as a reserved flag silently downgraded to
ordinary task execution. Antigravity no longer implies that vendor team support
is absent. Both require a verified current execution path for explicit team
requests and report `unsupported-mode` when that path cannot be established.
These are generated agent instructions, not a new live capability probe.

## Next evaluation

Use the existing observational comparison tools with fixed task/model/budget
conditions. Include a single-agent baseline, an independent-worker plan and a
dependency-aware plan where appropriate. Count failed integration, duplicate
implementation and coordination overhead; do not promote a policy because it
spawned more workers or merged more branches. The current release does not
hardcode paper-specific agent counts or claim the authors' savings as its own.

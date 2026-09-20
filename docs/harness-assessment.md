# Autopus-ADK: harness value and comparison

Assessment date: 2026-09-19. Local source baseline: `4282114a`.
This is a source and documentation review, not a head-to-head performance trial.
Upstream links track moving branches; recheck them before future decisions.

## Judgment

Autopus has practical value as a delivery and verification layer for teams using
multiple coding tools. Its strongest assets are platform adapters, executable
validation, provenance records, and repository-aware delivery. Agent counts,
TDD instructions, and SPEC documents alone are weak differentiators because
other projects provide similar workflows.

The main adoption risk is complexity: the README already exceeds 1,600 lines,
and a broad command catalog can obscure the first useful task. Prefer exposing
existing small-change paths and showing concrete evidence over adding another
mandatory phase, agent role, or parallel runtime.

## Comparison

These are documented strengths, not claims that other projects lack a feature.
No repositories were installed or executed for this comparison.

| Project and primary source | Documented focus | Lesson for Autopus |
| --- | --- | --- |
| [Superpowers](https://github.com/obra/superpowers/blob/main/README.md) | Composable skills connecting design, plans, TDD, and subagent implementation | Make the first workflow understandable; explain methodology separately from executable enforcement |
| [GitHub Spec Kit](https://github.com/github/spec-kit/blob/main/docs/index.md) | Spec-driven development with independent bug-fixing and idea-assessment entry points and extensibility | Route users by intent; avoid requiring a full specification set for every task |
| [BMad Method](https://github.com/bmad-code-org/BMAD-METHOD) | Explicit decisions, durable context, and planning depth proportional to the change | Show a small-change entry path before the comprehensive pipeline |
| [GSD Core](https://github.com/open-gsd/gsd-core) | Context engineering, fresh-context workers, and a discuss/plan/execute/verify/ship loop | Preserve continuation context and bound worker tasks; test the actual host's context and filesystem semantics |

The [archived GSD repository](https://github.com/gsd-build/get-shit-done) directs
readers to GSD Core as its current home. Reviewing only the archived project
would miss its current direction.
Multi-platform support is also present upstream; five platforms alone do not
establish a competitive advantage.

## What the local code supports

| Capability | Inspectable implementation | Limit of the evidence |
| --- | --- | --- |
| Platform-specific integration | [`pkg/adapter`](../pkg/adapter), [`pkg/workflow/doctor.go`](../pkg/workflow/doctor.go) | Generated configuration does not prove that every installed host hook ran |
| Prompt ordering and provenance | [`pkg/promptlayer/layer.go`](../pkg/promptlayer/layer.go), [`context_delivery_verify.go`](../pkg/promptlayer/context_delivery_verify.go) | A manifest cannot prove model comprehension or compliance |
| Typed pipeline gate decisions | [`pkg/pipeline/phase_gate.go`](../pkg/pipeline/phase_gate.go) | Only executed, correctly configured gates constrain an execution |
| Signed regression artifact verification | [`internal/cli/eval_regression.go`](../internal/cli/eval_regression.go) | [`pkg/evalregression/report.go`](../pkg/evalregression/report.go) consumes an external producer's verdict; it is not proof that ADK beats another harness |
| Paired efficiency and quality evaluation | [`pkg/experiment/efficiency_pair.go`](../pkg/experiment/efficiency_pair.go), [`efficiency_quality.go`](../pkg/experiment/efficiency_quality.go) | Evaluator implementation and synthetic test results are not measured savings |
| Lead time and rework reporting | [`pkg/telemetry/leadtime.go`](../pkg/telemetry/leadtime.go) | Results depend on complete events recorded during actual execution |

## Changes adopted in this review

1. Add task-based entry points to both READMEs, including the existing low-risk
   fix path. Keep the split catalog and existing risk gates.
2. Label illustrative pipeline output, distinguish workflow instructions from
   executable checks, and correct universal claims about worktree isolation.
3. Reject duplicate prompt layer IDs at `Render`. Previously, two entries with
   the same ID could render successfully; a later ID-based manifest comparison
   could hide removal of one entry. Add regression coverage for identical
   content, changed content, and IDs reused across layer kinds.

The third change protects an existing provenance boundary. It is an original
fix found during the audit, not copied upstream code. The comparison motivated
prioritizing reliable context records over adding more workflow instructions.
`CompareManifests` remains a diagnostic API; this change does not validate
arbitrary externally constructed manifests or make it a security verifier.

## Next investment: demonstrate net value

Run a controlled pilot before claiming faster delivery, lower costs, or better
quality. Reuse the existing telemetry and experiment evaluators instead of
introducing another benchmark framework.

1. Freeze a task corpus before execution: existing-contract bugs, small features,
   cross-module changes, and a security-sensitive regression. Include both easy
   and difficult tasks; keep every attempted task in the report.
2. Compare the native agent against Autopus with the same starting repository,
   model/version, reasoning effort, tools, permissions, and acceptance tests.
   Record the harness revision separately. Run AB and BA orderings in fresh
   isolated checkouts; retain failed attempts and retry spend.
3. Measure acceptance pass rate, escaped defects, human corrections, total token
   usage, elapsed time, and time to the first verified vertical slice. Missing
   provider usage remains unknown; do not replace it with a token estimate.
4. Use independent acceptance tests and review of the resulting diff. A generated
   summary saying PASS is not an acceptance oracle.
5. Report sample size, failures, exclusions, task-level results, and uncertainty.
   Add a competitor arm only after matching its configuration and task scope.
   Do not generalize one successful task into a product-wide percentage.

For recorded runs, `auto telemetry leadtime --run <SPEC-ID> --baseline <directory>
--json` reports existing lead-time evidence. `auto telemetry efficiency --help`
describes the stricter efficiency evidence input. That evaluator currently
targets compatible Ultra rollout evidence; it is not a turnkey cross-harness
benchmark runner. A real pilot still needs execution and evidence collection.

This review does not claim a measured quality, latency, or cost improvement over
the native agent or any competing harness. The immediate priority is reliable
evidence and a clearer adoption path; a comparative pilot is separate work.

## Research update: skills and harness architecture

Primary-source follow-up checked on 2026-09-19. Research results below are
author-reported; we did not reproduce their experiments. Paper versions matter:
search snippets can describe an older abstract or an earlier benchmark inventory.
Preprints, vendor engineering reports, and repository documentation provide
different kinds of evidence and should not be treated as one leaderboard.

### What the evidence supports

| Source and version | Reported result | Interpretation boundary |
| --- | --- | --- |
| [SkillsBench v4, June 14](https://arxiv.org/abs/2602.12670v4) | 87 tasks, 18 model/harness configurations; curated skills raise mean pass rate from 33.9% to 50.5%, a 16.6 percentage-point increase. Focused bundles of at most three modules outperform larger bundles | Bundle-size groups contain different tasks: this is not a causal test of adding a fourth skill to the same task or a universal installation limit |
| [SWE-Skills-Bench, March 16](https://arxiv.org/abs/2603.15401v1) | 39 of 49 public SWE skills show no pass-rate improvement; some increase tokens substantially. Three harmful skills contain guidance incompatible with repository versions | Claude Code with Haiku 4.5; baseline pass rate is already 89.8%. Public skills and partly generated task specifications limit generalization |
| [Evaluating AGENTS.md v2, June 23](https://arxiv.org/abs/2602.11988v2) | No general task-success improvement, with average inference cost rising by more than 20%; repository overviews are not helpful in the tested settings | An always-loaded repository context file differs from an on-demand specialist skill. The revised abstract is more cautious than the original negative-success headline |
| [On the Impact of AGENTS.md v2, March 30](https://arxiv.org/abs/2601.20404v2) | Across 10 repositories and 124 PRs, median runtime is 28.64% lower and output tokens 16.58% lower with context files | Different task selection and efficiency measures; output tokens are not total cost, and comparable completion behavior is not independent proof of equal patch quality |
| [Agent Skills Can Be Harmful, August](https://www.microsoft.com/en-us/research/publication/agent-skills-can-be-harmful-an-empirical-study-of-skill-induced-failures-in-llm-agents/) | Failure analysis includes excessive verification and heavy implementation procedures; regressions are not explained by prompt length alone | A study of attributed failure cases, not a population-wide probability that a skill causes harm |
| [ContextBench, results updated September 14](https://contextbench.github.io/) | Separates retrieval precision, recall, efficiency, and task success; reports substantial explored-but-unused context | More retrieval is not automatically better. Historical leaderboard entries may use different coverage and adaptations |

The defensible conclusion is conditional: task-relevant procedural knowledge can
help, while generic, redundant, outdated, or over-prescriptive guidance can add
work without improving outcomes. These studies do not establish a universal
optimal catalog size or prove an Autopus-specific regression.

### Count four different things

1. Available library packages.
2. Names and descriptions advertised to the model.
3. Skill bodies and references actually loaded for this task.
4. Extra actions induced by those instructions, including verification and retries.

[Anthropic's skill design](https://www.anthropic.com/engineering/equipping-agents-for-the-real-world-with-agent-skills)
explicitly separates startup metadata, on-demand bodies, and deeper references.
Its bundled scripts can execute without loading their full source into context.
Reducing library size alone is therefore a poor target; reducing irrelevant
exposure and unnecessary induced work is a testable target.

### Compare different architectural families

The earlier comparison covers methodology packages. Runtime and infrastructure
projects operate at different layers, so they are useful design references
without necessarily being direct replacements for Autopus.

| Family | Primary implementation | Design worth considering | Tradeoff |
| --- | --- | --- | --- |
| Minimal execution loop | [mini-swe-agent](https://github.com/SWE-agent/mini-swe-agent) | Bash-only actions and linear trajectories form an understandable baseline | A minimal loop does not itself supply team delivery policy |
| Small extensible coding runtime | [Pi](https://github.com/earendil-works/pi/blob/main/packages/coding-agent/README.md) | Four default tools, optional skills/extensions; planning and subagents are extension choices | Additional workflows need configuration and validation |
| Selective repository context | [Aider repository map](https://aider.chat/docs/repomap.html) | Rank symbols and dependencies within a context budget; expand into files as needed | A map is selective, and its usefulness depends on the task |
| Programmable agent harness | [Deep Agents architecture](https://github.com/langchain-ai/deepagents/blob/main/libs/ARCHITECTURE.md) | Planning, filesystem offload, subagent context isolation, and summarization as middleware | Middleware order, storage lifetime, and execution isolation still need engineering |
| Execution and workspace infrastructure | [OpenHands SDK](https://github.com/OpenHands/software-agent-sdk) | Separate tools, conversations, events, and local or ephemeral workspaces | Running infrastructure is a broader responsibility than distributing skills |
| Long-running application workflow | [Anthropic, March 24](https://www.anthropic.com/engineering/harness-design-long-running-apps) | Structured handoffs and an independent evaluator that exercises the running app | Vendor case study; extra evaluators have latency and cost, and model upgrades can make an old workaround unnecessary |
| Evidence-driven harness evolution | [AHE v4, May 18](https://arxiv.org/abs/2604.25850v4) | Predict the effect of each change, retain execution traces, compare outcomes, and roll back | Research prototype; benchmark tuning and weak prediction of regressions limit generalization |

AHE reports Terminal-Bench 2 performance of 69.7% for its seed and 77.0% after
evolution. Its component experiment attributes improvements to tools, middleware,
and memory; swapping only the evolved system prompt regresses. This is a reason
to test executable changes, not a guarantee that rewriting prompts never helps.
See its [component analysis and limitations](https://arxiv.org/html/2604.25850v4).

### Autopus priorities inferred from the comparison

The local [`skill_catalog_policy.go`](../pkg/content/skill_catalog_policy.go)
already declares ten reusable core skills and keeps optional bundles separate.
However, all command routes and referenced dependencies also qualify for the
default surface. Ten core names therefore do not mean ten advertised entries.
The [compact-catalog tests](../pkg/content/skill_catalog_compact_test.go) protect
packaging size, not real task performance or actual host prompt exposure.

Recommended order, subject to paired evaluation:

1. Measure what each host actually receives: advertised entries, duplicate aliases,
   estimated metadata size, loaded bodies, and induced actions. Keep estimates
   distinct from provider-reported token usage.
2. Audit generic procedural overlap before expanding the library. Keep required
   project constraints; make optional specialty guidance task-selected. Test
   negative triggers and repository/version compatibility as well as invocation.
3. Make repeated checks conditional on changed inputs or unresolved findings.
   Preserve mandatory security, data-loss, and acceptance checks. Fewer checks
   are useful only if defect escape does not increase.
4. Strengthen small executable helpers, fresh verification evidence, and recovery
   state. Avoid rebuilding a host's compaction or worker runtime without a
   demonstrated gap.
5. Compare native baseline, current Autopus, and a reduced-exposure Autopus arm
   on the same tasks. Keep model, budget, acceptance oracle, and permissions
   matched; separately measure catalog selection and skill-body usefulness.

This follow-up records research and design recommendations. It does not remove
skills, change runtime defaults, or claim that a reduced catalog has already
improved Autopus performance.

## Implementation follow-through

The local v0.50.118 candidate adds `skill audit`, explicit `skill select` and
`skill policy-check`, `spec gates --read-only --no-reuse`, and observational
`telemetry harness` comparison. See [commands and examples](harness-efficiency.md)
and [candidate readiness](runbooks/release-v0.50.118.md). These implement the
measurement and validation mechanisms; real cross-harness productivity trials
remain distinct from deterministic software tests.

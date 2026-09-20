# Choose execution depth from task evidence

Autopus distinguishes the amount of workflow from model quality. The router now
selects an inline, guided or planned approach from scope, risk, uncertainty and
known acceptance. It does not switch the user's model or reasoning settings.

| Route | Admission | Work |
| --- | --- | --- |
| inline | Low risk, known scope/requirements/acceptance, small existing-contract fix or docs | Implement directly and run targeted verification; no default planning worker |
| guided | Missing evidence, moderate work or one failed attempt | Inspect the missing facts, narrow the target, implement and verify |
| planned | High risk, sensitive paths, multiple domains, unresolved requirements, broad change or repeated failure | Plan scope/interfaces, review the changes and verify integration |

This is workflow guidance, not an autonomous task scheduler. Generated Claude,
Codex, Gemini, OpenCode and OMP routers carry the same short policy. Explicit
plan/review/team/solo requests remain meaningful. An implementation request does
not require loading every skill or making an extra model call to classify it.

## Read-only decision command

```sh
auto workflow triage --facts-json docs/examples/task-routing/small-fix.json --format json
auto workflow triage --facts-json docs/examples/task-routing/sensitive.json --format human
```

Use the local `bin/auto-0.50.118-candidate` if the installed release predates the
command. The command reads supplied facts only: no model call, file edit,
worker spawn, config rewrite, or gate receipt mutation. Its output records
caller-declared provenance and path-based risk inference, not independently
verified repository facts or available capacity.

Minimal example:

```json
{
  "version": 1,
  "kind": "bugfix",
  "paths": ["pkg/parser/value.go"],
  "scope_complete": true,
  "requirements_clear": true,
  "acceptance_known": true,
  "estimated_changed_lines": 20,
  "risk": "low"
}
```

Optional facts include `failed_attempts`, `requested` (auto/inline/guided/planned),
`solo`, `native_parallel_available`, and `workers` with id, owned_paths and
independent. Kinds are docs, bugfix, feature, refactor and investigate. Risk is
unknown, low, medium, high or critical. Missing boolean/line estimates stay
unknown; missing facts never silently qualify a task for inline.

The decision contains route, reasons, required_steps, suggested_skills,
execution, model_policy and advisory. Required steps are obligations, not an
ordered executable plan. Suggested skills are narrow optional recall; they do
not replace required project context or acceptance evidence.

## Escalation and parallel work

- A single auth/payment/migration or other sensitive path can require planned
  work even if the caller says low risk or requests inline. Path inference is
  conservative; an unrecognizable path does not prove absence of risk.
- One failed attempt removes inline eligibility; two require planned diagnosis.
  Reassess with updated facts after scope growth or new risk. The command does
  not track attempts automatically or certify that supplied counts are complete.
- A higher explicit route is honored. A lower route cannot override the current
  minimum. Solo keeps execution serial while preserving risk checks.
- Parallel is a separate recommendation, available only for planned work with
  at least two distinct independent workers, disjoint declared ownership within
  the parent's declared scope, positively known requirements/acceptance/scope,
  native availability, and no solo request. Unknown or overlapping work stays
  serial. Actual capacity, authorization and ownership must still be verified
  immediately before dispatch; lexical path checks are not a filesystem sandbox.
- Existing SPEC gates, security/data-loss checks, UX requirements and configured
  quality thresholds remain authoritative. Triage grants no gate waiver.

Initial size boundaries are <=2 paths and <=80 estimated changed lines for
inline, and >=6 paths or >300 estimated changed lines for planned. They are
policy heuristics, not statistically calibrated difficulty estimates. File
counts do not imply independence or force worker spawning. Workers are bounded
at 32, with at most 256 total owned-path claims to bound classification work.

## Evidence and limits

The [36-run exposure pilot](benchmarks/harness-2026-09-20.md) motivated avoiding
unnecessary default process on small changes. It did not evaluate this new
router or prove the best threshold. Functional tests cover escalation, unknown
facts, explicit intent, ownership and generated-platform parity. No speed,
cost or quality improvement is claimed until this policy is separately tested.

See [SPEC-TASK-ROUTING-001](../.autopus/specs/SPEC-TASK-ROUTING-001/acceptance.md)
for implementation acceptance. No new always-loaded skill or model preset is
introduced.

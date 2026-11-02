# SPEC review policy

`auto spec review` answers one question: can implementation start from this
SPEC without guessing? This page describes the single policy that decides what
a SPEC must contain, what blocks it, and how a review of it runs. Defined by
SPEC-REVIEWPOLICY-001.

## One policy, three profiles

The policy lives in `pkg/spec` (`ReviewPolicyFor`, `EffectiveReviewPolicy`).
`auto spec validate`, the reviewer prompt, the runtime checklist handling, and
the review plan read it; the writer, review, and plan guidance restate it and a
parity test (`templates/review_policy_surface_test.go`) fails when they drift.

| Profile | Documents | Review |
| --- | --- | --- |
| `compact` | `change.md` (`## Referenced SPEC`, `## Intended Surface`, `## Verification Plan`, `## Gate Applicability`) against an existing SPEC | none of its own; the referenced SPEC's approval stands |
| `lean` | the four SPEC files at the lean floor | one reviewer, no judge, quorum 1 |
| `full` | the four SPEC files with every section | the configured reviewers and judge |

Every profile keeps the `security`, `validation`, `data_loss`, and
`deterministic_oracle` gates; none of them can be `not_applicable`.

A SPEC gets the lean policy only when it declares `profile: lean` with a
low-risk `change_class` and a `surface` the path classifier reads at or below
that class. Any other lean declaration is reviewed and validated as `full`, so
a high-risk change cannot skip a full gate by labelling itself lean.

### The lean floor and its allowed omissions

- Lean floor: spec.md `## Outcome Boundary`, `## Requirements`, `## Traceability Matrix`; plan.md `## Tasks`; acceptance.md `## Test Scenarios`; research.md `## Outcome Lock`, `## Reviewer Brief`.
- Lean allowed omissions: plan.md `## Implementation Strategy`, `## Visual Planning Brief`, `## Feature Completion Scope`, `## Risk-First Integration Probe`; acceptance.md `## Oracle Acceptance Notes`; research.md `## Visual Planning Brief`, `## Semantic Invariant Inventory`, `## Minimality Decision Matrix`, `## Feature Coverage Map`, `## Completion Debt`, `## Evolution Ideas`, `## Reference Discipline`, `## Self-Verify Summary`.

A review never reports an allowed omission. The quality checklist items that
only check an omitted section (`Q-COMP-08`, `Q-COMP-07`, `Q-CORR-04`) are
answered N/A, and when a reviewer still reports FAIL for one of them while the
section is absent, the CLI records it as N/A with the reason. A lean SPEC that
did write the section is checked as usual. Missing acceptance, security, and
data-loss problems block a lean SPEC exactly as they block a full one.

## What blocks

The reviewer prompt, the judge prompt
(`templates/shared/spec-review-judge.md.tmpl`), and the runtime apply the same
rules:

- Blocking at any severity: an open or regressed finding in category
  correctness, contract, data_loss, or security, and any finding raised through
  the critical/security escape hatch.
- Blocking: any other critical or major finding, and a minor finding outside
  the style, suggestion, and improvement categories.
- Advisory: severity suggestion outside the impact categories, and style,
  suggestion, or improvement findings below major. Wording, a nicer structure,
  and choices delegated to the implementer are reported as suggestion.
- A failing checklist item in a REVISE vote blocks.
- PASS only when nothing blocks; REVISE when something blocks and the design can
  be fixed in place; REJECT only when a reviewer or the judge states a
  fundamental redesign.

A judge PASS never waives an accepted blocking finding: the runtime keeps it
open and names it. A judge clears a finding by rejecting it with a reason, or
by accepting it at severity suggestion when it is advice (the impact categories
have no advisory severity).

`spec.review_gate.pass_criteria` adds project criteria after these rules; it
cannot replace them.

Changed behaviour (SPEC-REVIEWPOLICY-001):

- The reviewer prompt no longer says that two open major findings are a PASS,
  or that any critical or security finding is a REJECT.
- A judge PASS that accepts a minor correctness, contract, data_loss, or
  completeness finding now resolves REVISE instead of PASS.
- A structured reviewer's `contract` and `data_loss` categories are kept
  instead of becoming `completeness`.
- `auto spec validate` holds a refused lean declaration to the full section set.

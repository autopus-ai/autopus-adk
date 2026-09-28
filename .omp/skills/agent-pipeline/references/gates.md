# Gate Applicability, Evidence, and Telemetry

Read this file when you run `auto spec gates`, record gate evidence, or write a
telemetry subrecord.

## The closed gate set

`auto spec gates <SPEC-ID> --base <ref>` (or `--changed p1,p2,...`) writes
`{SPEC_DIR}/gate-applicability.json` over exactly these gates:

`spec_authoring, risk_first_probe, build, unit_tests, integration, security,
validation, data_loss, deterministic_oracle, accessibility, ux_verification,
annotation, provider_review, doc_sync`

Every phase gate records `gate: applicability — reason` in its handoff. The
value is a deterministic classifier decision, never an agent judgement.

| Value | Meaning |
|---|---|
| `required` | the gate applies and must return a verdict from a real execution |
| `reusable` | prior exact-input evidence is still valid; see below |
| `not_applicable` | the gate has no surface in this change |
| `blocked` | the gate applies but its input is unavailable |

Run the classifier once, before the implementation fan-out, and carry every
decision into each unit's prompt.

## Flags that change the decision

- `--change-class <class>` uses the class a compact change contract declares.
  Without it the class is derived from the change set, so it can never be
  understated.
- `--new-contract` declares a new exported API or contract.
- `--annotation` opts the run into @AX annotation. Without it the `annotation`
  gate is `not_applicable` and no tagging step runs.
- `--max-age <dur>` bounds evidence reuse; the default is 168h.

## What `reusable` requires

`reusable` is granted only by a prior `{SPEC_DIR}/gates/evidence-<gate>.json`
receipt that satisfies all of:

- `input_closure_sha256` recomputed from the current tree still matches;
- `status: pass` and `complete: true`;
- `observed_at` inside `--max-age`.

The receipt's `input_globs` are re-expanded against the current tree, so an
added file invalidates evidence exactly like an edit or a deletion. Any
dependency change, `fail`, `partial`, missing input, or stale receipt yields
`required` whose reason names the failed condition: `no prior evidence`,
`input closure changed`, `prior status fail`, `prior evidence partial`,
`evidence older than max-age`, or `missing input <path>`. Agents never
self-assign `reusable`.

After each real build, test, or UX execution, record its evidence:

```
auto spec gates record <SPEC-ID> --gate <id> --status pass|fail|partial \
  --inputs <glob,...> [--dynamic-deps <path,...>] [--command "<text>"]
```

That is what lets the next run reuse exact-input evidence instead of repeating
the work.

## Gates that can never be waived

`security`, `validation`, `data_loss`, and `deterministic_oracle` are never
`not_applicable`. They may only be `required`, `reusable`, or `blocked`.

`accessibility` and `ux_verification` are `required` when the change set
contains UI paths, and `not_applicable` with the reason `no UI surface in change
set` otherwise — only the classifier may decide that.

## The annotation gate specifically

Without `--annotation` the decision is
`annotation: not_applicable — @AX annotation not requested for this change set`,
and no tagging step runs. With `--annotation` it becomes `required` for a code
change set. `blocked` is reachable only under `--annotation`, when the reference
source is genuinely missing; then report the named fallback "record modified
files, defer tagging" rather than stalling.

## `not_applicable` versus `blocked`

An auxiliary step that is genuinely a no-op — nothing to annotate, no doc to
sync — is `not_applicable`. A step that was requested but whose reference source
or tool surface is missing is `blocked`, reported with a named fallback rather
than a stall. Collapsing the two hides a setup gap behind a green gate.

## Change contract classes

| Declared class | Risk | Authoring path |
|---|---|---|
| `test_only`, `docs_only`, `small_ui`, `bugfix_existing_contract` | low | compact `change.md`; `spec_authoring` and `risk_first_probe` are `not_applicable` |
| `feature`, `multi_domain`, `security_or_data` | high | full SPEC set plus the Phase 1.9 probe; both gates `required` |

`auto spec change <SPEC-ID> --class <class> --ac <AC-ID,...> --surface
<path,...> --verify "<command>" [--change-id <id>] [--new-contract] [--json]`
writes one `change.md` that references an existing SPEC and its
acceptance-criteria ids. It refuses when the SPEC or any referenced id does not
exist, and it never restates requirements.

Risk is never decided by file count. A path under an auth, billing, data,
migration, or security glob raises the class to `security_or_data`. Production
code spanning two module roots raises it to `multi_domain`; documentation and
test material never raise that signal on their own. A new exported API or
contract raises it to `feature`.

A declared class contradicted by the intended surface — `test_only` including
non-test source, `docs_only` including code, `small_ui` including non-UI source
— reports `escalate_to_full_spec` with the reason, writes no `change.md`, and
exits non-zero. Escalation is a routing decision, not a warning to read past.

A `test_only` contract forbids production-source edits. New tests failing
against current behaviour is a contract violation to report, not a licence to
change production code.

## Telemetry subrecords

Every record is written at the moment the event happens, never reconstructed at
the end:

| Event | Command |
|---|---|
| planning produces an estimate | `auto telemetry record --spec-id <ID> --action estimate --min 30m --max 2h` |
| first probe PASS or first real integration execution | `--action milestone --name first_vertical_slice` |
| an already-verified input is re-read or a passing check re-run | `--action action --kind reread\|rerun --target <path\|cmd> --reason <text>` |
| a defect is found | `--action defect --id <id> --discovered-phase <phase> [--fixed-phase <phase>] [--files N] [--escaped] [--repeat]` |
| any gate decision | `--action gate --gate <id> --applicability required\|reusable\|not_applicable\|blocked [--resolved]` |
| a phase starts with known predecessors | `--action start --phase <phase> --depends-on <phase,...>` |

`--depends-on` (valid on `--action start` and `--action agent`, alongside
`--phase`) is what makes the phase DAG real: the critical path is the longest
wall-clock path through that DAG, not the sum of phase durations, so a parallel
fan-out must not be reported as serial cost.

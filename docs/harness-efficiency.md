# Diagnose and evaluate harness overhead

These commands inspect local files or supplied observations. They do not run
models, load skills into a live session, prune an installation, or change defaults.
Use the source-built candidate when testing unreleased changes; a PATH-installed
release can have different behavior.

## Inspect skill exposure

```sh
auto skill audit --dir . --platform codex --format json
```

Platforms: `codex`, `claude-code`, `antigravity-cli`, `opencode`, `omp`.
The report separates the embedded library, compiler-selected outputs, and
project-local files. Selection reasons include `core`, `route`, `dependency`,
`opt_in`, `not_selected`, and `unsupported`. Default and full counts describe
compiler configurations for the embedded reusable-skill catalog, not measured
session prompts. Generated command routes outside that catalog are not included
in those predicted counts; their local files can still appear in the scan.

`summary` reports counts and UTF-8 byte sizes. Every `*_token_estimate` uses
`ceil(bytes/4)` per item, summed for totals; it is not a model tokenizer or a
billing estimate. Catalog metadata measures names and descriptions; installed
metadata measures the actual file frontmatter. Those are different bases.

`missing_configured_paths`, `same_name_paths`, `duplicate_content_paths`, and
`skipped` help locate old or overlapping installations. Multiple files are not
proof that a host loads them simultaneously. The report always marks
`session_loaded: "UNKNOWN"`; external plugins, global skill paths and actual
host calls are outside the scan. An empty duplicate list does not prove that
different wording contains no overlapping instructions.

No file bodies are printed. The scan is bounded and skips unsafe skill paths.
Invalid configuration is an error, not an empty successful inventory.

## Select against an explicit contract

The example below is a project-authored demonstration policy, not an official
compatibility matrix for the named skill. Run it from this repository, which
contains the required `go.mod` marker:

```sh
auto skill select --dir . \
  --policy-json docs/examples/harness-efficiency/policy.json \
  --task-json docs/examples/harness-efficiency/task.json --format json
auto skill policy-check --dir . \
  --policy-json docs/examples/harness-efficiency/policy.json \
  --cases-json docs/examples/harness-efficiency/cases.json --format json
```

Policies contain candidate IDs, allowed/excluded task classes, required relative
file markers and supported exact version strings. Explicit exclusion wins;
missing version facts are `unknown`, mismatches are `excluded`. Candidates with unknown required facts are not selected. Here, "unknown" means insufficient
facts for a declared candidate; IDs themselves are not checked against an
installed skill registry.

Facts carry provenance: `declared` for task classes/version strings and
`local_file` for inspected marker existence. A caller-supplied `go: "1.26"`
does not prove the installed Go binary is that version. Exact string matching
does not implement semver ranges. Selection is neither semantic suitability
proof nor authorization to execute a skill.

Policy replay checks the supplied expected selected-name sets for positive,
negative, incompatible and unknown cases. Mismatches return nonzero and a JSON
case report. A passing replay proves those cases, not real coding effectiveness.
Strict JSON rejects unknown fields, duplicate keys, oversized input and unsafe
file marker paths. No natural-language keyword heuristic is used.

## Plan verification without repeating it

```sh
auto spec gates SPEC-ID --read-only --json
auto spec gates SPEC-ID --read-only --no-reuse --json
```

`--read-only` does not normalize/write `autopus.yaml` or replace a gate receipt.
The normal command retains its existing persistence behavior. File-closure
reuse remains available for complete, successful, fresh evidence whose inputs
still match. Changed, added, missing and deleted inputs are evaluated by the
existing closure logic, including declared dynamic dependencies.

Use `--no-reuse` when command flags, toolchain, environment or external state
changed or are uncertain. It disables prior-evidence reuse; it never waives a
mandatory check. `gates record` records a caller assertion and file hashes,
not a subprocess execution attestation. Declare the full input scope and record
only actually executed checks. Network/browser/live service state generally
needs fresh verification even when the source tree is unchanged.

## Compare native, current and reduced harness observations

```sh
auto telemetry harness \
  --evidence-json docs/examples/harness-efficiency/evidence.json --format json
```

The bundled input is an **unmeasured template**: metrics are `null` and `runs`
are empty. It must produce an incomplete report with unknown totals, not a
productivity result. Replace the placeholders with actual observations before
drawing conclusions.

The input declares `version: 1`, a frozen `expected_task_ids` corpus and one
observation per task/arm (`native`, `current`, `reduced`). Each observation has:

- `harness_revision` and `harness_config_hash`: the intentionally different arm.
- `identity`: matching task, oracle, source revision, environment, budget,
  provider/model versions, effort and cache stratum for pairwise comparison.
- `accepted`, `elapsed_ms`, `human_corrections`: observed values or null.
- `runs`: existing telemetry `AgentRun` records with normalized `UsageEnvelope`
  entries. Actual usage requires provider provenance and matching model/cache
  identity. Include failed attempts and retries, not only the final success.

Output is observational and does not produce a winner or promotion. Pair deltas
are candidate minus baseline, include rejected tasks, and exclude incompatible
or incomplete pairs with reasons. Token savings on a failed task do not imply
better effectiveness; inspect acceptance counts alongside costs.

`actual_tokens: null` means a total is not known. `known_actual_tokens` preserves
the observed subtotal, including measured retry spend in an incomplete task;
`measured_tasks` counts tasks with complete usage. `complete` requires all
expected observations, metrics and compatible pairs. Pair token/time deltas may
still be available when human corrections are unknown, but overall completeness
remains false. No signature authentication is implied by provider-labelled input.

Keep all attempts in the corpus, use fresh isolated checkouts and alternate
execution order across repeated trials. The tool does not launch or randomize
trials and does not estimate statistical significance. See
[the research assessment](harness-assessment.md) for the experiment rationale.

## Executed pilot

The [2026-09-20 instruction/skill exposure pilot](benchmarks/harness-2026-09-20.md)
ran 12 regression tasks under three fixed single-agent configurations. It found
no focused code-oracle advantage, and native had the lowest total elapsed time.
This does not evaluate the full workflow or multiagent effectiveness. The report
preserves timeout usage as unknown and includes runnable benchmark source.

The subsequent [task routing policy](task-routing.md) separates execution depth
from model quality: evidenced small fixes can stay inline, uncertainty prompts
focused inspection, and high risk requires planning/review. This new policy has
functional verification; the exposure pilot does not measure its performance.

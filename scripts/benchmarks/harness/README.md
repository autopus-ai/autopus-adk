# Harness instruction/skill exposure pilot

This is a single-agent regression-repair pilot, not a full workflow or multiagent
benchmark. `native` has no project harness, `reduced` is the existing compact
split default, and `current` is explicit full-catalog generation (not default).
The first local surfaces advertise 0, 38 and 71 project skills respectively.

## Reproduce

From the repository root:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts/benchmarks/harness -p 'test_*.py'
go run scripts/benchmarks/harness/generate.go --output /absolute/new/surfaces
python3 scripts/benchmarks/harness/run.py \
  --repo "$PWD" --revision a05ce69df9dc03493b8c5de0ab9299459195e8d8 \
  --surfaces /absolute/new/surfaces --output /absolute/new/results \
  --model gpt-6-astra --effort medium --timeout 180
```

The runner invokes real model calls and consumes the current Codex account's
quota. Generated artifacts and raw prompts/logs belong outside source control.
Use a new output directory; no automatic retry or overwrite of a frozen run.

Before a run, verify the named Codex filesystem permission profile allows local
work and the Go toolchain while denying the original source, oracle metadata,
and sibling trials. Legacy --sandbox flags must not override that profile.
Existing global skills and account state are shared controls, not removed;
`--ignore-user-config` does not imply a globally pristine Codex installation.

## Corpus and scoring

Twelve tasks seed one behavior regression into an exact committed source file.
Every original oracle passes and every seeded oracle fails on a behavior
assertion. Corpus hashes and run order freeze before trials. A duplicate seeded
ownership task was removed before any model trial, not in response to outcomes.

All six arm permutations occur twice. Model/effort/deadline are constant and
trials run serially. Existing tests and files outside the assigned source are
protected. Additional tests may be written, but grading uses immutable original
tests in a seeded independent snapshot with only candidate production edits.
Missing/symlink candidate files fail; they cannot restore correct baseline code.

Acceptance requires scope integrity, normal agent completion and oracle success.
Elapsed time excludes setup, cache warming and independent grading. Go cache is
shared/warmed; test-result caching is disabled by -count=1. Provider cache is not
controlled or flushed: report cached tokens separately and do not equate total
token differences with invoice savings. Failed or timed-out attempts remain in
reports, with missing usage unknown rather than zero. No human interventions are
made; command counts do not measure human rework.

The tasks are small, source-local regressions in ADK itself. Results do not
establish general software-development effectiveness or statistical significance.
One trial per task/arm cannot establish repeatability. No automated winner or
configuration promotion is performed.

Observed pilot: [2026-09-20 report](../../../docs/benchmarks/harness-2026-09-20.md).
Export a completed result directory to the existing comparison command:

```sh
python3 scripts/benchmarks/harness/export.py --input /absolute/new/results --output /absolute/new/evidence.json
bin/auto-0.50.118-candidate telemetry harness --evidence-json /absolute/new/evidence.json --format json
```

Usage receipts are explicitly labelled session aggregates, not individual
provider-call counts. The pilot exported report remains incomplete when timeout
usage is unavailable, even though all 36 scheduled trials finished.

## Golden live lane grader (SPEC-HARNEVAL-001)

`prepare_grader.py`, `grader.py` and `grader.sb` grade the golden mode's agent
tasks on a maintainer macOS host. None of them calls a model.

- Trusted preparation runs outside any sandbox and before any agent. It fills
  the session module cache with `go mod download` from the workspace `go.mod`
  and `go.sum`. Every downloaded hash must already be pinned in `go.sum`; then
  `go mod verify` runs and the cache becomes read-only. The oracle packages are
  compiled once into a warm build cache. The default download source is a file
  proxy over the local module cache, so this step needs no network.
- Every grading run gets a fresh grade root: clonefile (`cp -c`) copies of the
  snapshot (`ws/`) and of the warm cache (`gocache/`), plus its own `HOME`,
  `TMPDIR` and `GOPATH`. The oracle runs as
  `env -i <allowlist> sandbox-exec -f grader.sb -D GRADE_ROOT=<root> -D ACCOUNT_HOME=<home> -D CODEX_HOME_DIR=<codex home> go test -json ...`.
  The profile denies all network and every write outside that root. The whole
  environment is `PATH` (the Go toolchain directory only), `HOME`, `TMPDIR`,
  `GOPATH`, `GOCACHE`, `GOMODCACHE`, `GOFLAGS=-mod=mod`, `GOPROXY=off`,
  `GOSUMDB=off` and `GOTOOLCHAIN=local`.
- The profile also keeps the account's credential stores unreadable: the Codex
  home (`$CODEX_HOME`, or `~/.codex` when unset), `~/.codex`, `~/.ssh`,
  `~/.config/gh`, `~/.aws`, `~/.claude`, `~/.claude.json`, `~/.omp`,
  `~/.gnupg`, `~/.docker`, `~/Library/Keychains` and `/Library/Keychains`, and
  it denies the Security server lookups, so the keychain is closed through IPC
  as well. `ACCOUNT_HOME` comes from the password database, not `$HOME`. This
  is a T12 scope expansion of REQ-HE-08 (the T11 profile allowed every read);
  the 12-task calibration passes unchanged under it.
- The trusted parser in `grader.py` reads only the captured stdout (at most
  1 MiB). A run is accepted only with exit 0, valid test2json events, a
  top-level pass for every `expected_tests` name and no fail for any of them.
- Calibration grades every agent task twice under the same profile: the clean
  workspace must be accepted and the mutated one must not. Anything else
  refuses the session with `oracle_calibration_failed` before any agent call.

Check the grader environment without any agent:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/benchmarks/harness/prepare_grader.py \
  --repo "$PWD" --output /absolute/new/calibration
```

It snapshots the manifest's `live.workspace_revision`, prepares both caches,
writes `calibration.json` and exits 1 with the failing task ids when the
calibration fails. The macOS-only unit tests in `test_grader.py`,
`test_grader_credentials.py` and `test_prepare_grader.py` exercise the same
profile on small fixture modules.

## Golden live lane runner (SPEC-HARNEVAL-001)

`run.py --mode golden` (`golden.py`, `golden_trial.py`, `golden_agent.py`,
`golden_protocol.py`)
runs one advisory session: every active agent task of the golden set in a
baseline and a candidate arm, K times each, on one pinned workspace revision.
The pilot mode above is the default and does not change.

```sh
python3 scripts/benchmarks/harness/run.py --mode golden \
  --output /absolute/new/session --surfaces /absolute/arm-surfaces
go run ./cmd/auto eval harness report --input /absolute/new/session --format json
```

`--surfaces` holds the generated `baseline/` and `candidate/` arm surfaces.
Until the per-revision surface driver (T13) lands, the maintainer generates
them; the protocol records each arm's surface digest (the Go `SurfaceDigest`
twin in `golden_protocol.py`). Every input comes from the committed manifest:
`live.workspace_revision`, `live.k`, `live.model`, the run cap and `pins`.

Refusals happen before any agent call and print one JSON document
(`{"status": "refused", "reason": ...}`) with exit 1: `unsupported_os`,
`invalid` (the Go and Python loaders must agree on the agent set),
`protocol_exists`, `output_not_empty`, `run_cap_exceeded`,
`codex_cli_version_mismatch` (`codex --version` against
`pins.codex_cli_version`), `workspace_setup_failed` (the snapshot),
`workspace_mutation_mismatch` and `oracle_calibration_failed`. Only the last
writes files: `protocol.json` with the scheduled order and `calibration.json`
with the failed `before`, and no record.

A session that starts freezes `protocol.json` with `O_EXCL` before the first
trial, runs the balanced order (trial `t`, tasks by id, baseline first when
`t + task index` is even) once without retries, appends each record to
`records.jsonl` from the runner process, and recalibrates into
`calibration.json` `after`. `trials/<n>-<task>-<arm>-<trial>/` keeps the
body-free `trial.json`, the warmup log and `grader.jsonl`. Workspaces, caches
and the agent transcript live in `scratch/` and are removed at the end unless
`--keep-scratch`.

Trial stages are snapshot, mutation, warmup, arm surface, agent, scope audit,
grading. Only the first three yield `error` (`workspace_setup_failed`,
`mutation_failed`, `warmup_failed`). Grading runs even after an agent failure
and is skipped only for `scope_violation` or `forbidden_construct` (a new
`func init(`, `os.Exit`, `syscall.`, `unsafe.` or `//go:linkname` literal in
an allowed file). An agent that exits nonzero before printing any event never
started (for example codex rejecting the surface's configuration) and is
`agent_launch_failed`.

The agent step runs `codex exec --strict-config` under the pilot permission
profile (plus the read-only session module cache) with exactly the grader
allowlist environment, `PATH` widened by the codex and system directories, and
the credentials named by `--credential-env` (default `CODEX_HOME`,
`CODEX_API_KEY`, `OPENAI_API_KEY`; `CODEX_HOME` falls back to its default).
`shell_environment_policy.include_only` passes only the allowlist keys to the
commands codex runs, so no credential reaches them; `--strict-config` makes
codex refuse a policy key it does not know. After the agent exits, the runner
SIGKILLs every process left in the agent's session, codex's per-command
process groups included, before reaping it.

`runner_sha256` is the tree digest (sorted `path\0sha256\n` rows) of the
runner file set in `golden_protocol.RUNNER_FILES`: the golden modules,
`grader.py`, `grader.sb`, `prepare_grader.py` and the pilot modules the runner
loads. REQ-HE-07 names `golden.py` alone; the union is the T11 handover.
`grader_profile_sha256` is the SHA-256 of `grader.sb`.

Tests: `test_golden.py` and `test_golden_runner.py` run everywhere;
`test_golden_session.py` runs whole sessions on a fixture module under the
real profile (macOS) and judges them with a freshly built `auto`. The
real-corpus end-to-end session is opt-in:

```sh
HARNEVAL_GOLDEN_E2E=1 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v test_golden_e2e
```

run from this directory. Its agent is a scripted fake codex; no model is called.

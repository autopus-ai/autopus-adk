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
  `env -i <allowlist> sandbox-exec -f grader.sb -D GRADE_ROOT=<root> -D MODCACHE=<module cache> -D GOROOT=<toolchain> -D ACCOUNT_HOME=<home> -D CODEX_HOME_DIR=<codex home> go test -json ...`.
  The profile denies all network, every Mach service lookup and every write
  outside that root. The whole environment is `PATH` (the Go toolchain
  directory only), `HOME`, `TMPDIR`, `GOPATH`, `GOCACHE`, `GOMODCACHE`,
  `GOFLAGS=-mod=mod`, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local` and
  `PWD` (the workspace, so `go` finds its working directory without listing
  the denied directories above it).
- Reads are denied by default: no file below the account home (from the
  password database, not `$HOME`), any other home under `/Users`, or the
  temporary roots `/private/var/folders`, `/private/tmp` and `/private/var/tmp`
  can be opened or listed, except below the grade root, the session module
  cache and the Go toolchain root (`go env GOROOT`). That closes every
  credential store, named or not (`~/.netrc`, `~/.npmrc`, `~/.git-credentials`,
  `~/.config`, `~/.kube`, `~/.gemini`, `~/.local/share`, `~/Library/Application
  Support`, shell history, ...), and the files of other sessions. Metadata
  stays readable, so a denied file can be stat'ed but not read. The Codex home
  (`$CODEX_HOME`, or `~/.codex` when unset) and the keychains are closed with
  their metadata wherever they lie, and with no Mach lookup the keychain is
  closed through IPC as well. `grader.sandbox_argv()` refuses a module cache
  or toolchain root that is or holds a home, since the profile reopens reads
  below them. Every rule names its read operations: a rule on a named
  operation outranks a `file-read*` rule whatever their order. This is a scope
  expansion of REQ-HE-08 (T12, widened by the Phase 4 review).
- Every process of a sandboxed run gets hard rlimits (soft = hard): CPU
  seconds twice the wall timeout, 1 GiB per written file and, on macOS, the
  account's process count at launch plus 512 (`RLIMIT_NPROC` counts every
  process of the user id, so a fixed cap would either block the oracle's
  forks on a busy host or bound nothing).
- The trusted parser in `grader.py` reads only the captured stdout (at most
  1 MiB). A run is accepted only with exit 0, valid test2json events, a
  top-level pass for every `expected_tests` name and no fail for any of them.
  The captured stderr (`grader.stderr`, `warmup.stderr`, calibration logs) is
  kept to its first 1 MiB and ends with a `[grader.py: stderr cut at 1048576
  bytes]` stamp when cut. It is not redacted (the harness has no sanitizer):
  the oracle gets no credential in its environment and cannot open the
  account's files, so a stderr can carry only what the grade root, the module
  cache, the toolchain and the readable system files hold.
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
`golden_protocol.py`, `golden_surface.py`, `surface_driver/main.go`)
runs one advisory session: every active agent task of the golden set in a
baseline and a candidate arm, K times each, on one pinned workspace revision.
The pilot mode above is the default and does not change.

```sh
python3 scripts/benchmarks/harness/run.py --mode golden --output /absolute/new/session
go run ./cmd/auto eval harness report --input /absolute/new/session --format json
```

Each arm surface is generated by the surface driver built from that arm's
revision in `--repo`: `live.baseline_ref` for the baseline and
`--candidate-ref` (default `HEAD`, the committed tree) for the candidate.
`golden_surface.py` extracts the revision with `git archive` into a fresh
build root and makes this checkout's `surface_driver/main.go` the only file of
its package there. A trusted step outside the sandbox fills a session module
cache with `go mod download` from a file proxy over the local module cache
(`--proxy` names another source); the arm's `go.sum` verifies every module and
may not change. The build then runs under `grader.sb` with the build root as
its only writable tree: `go build -trimpath -ldflags "-X
github.com/insajin/autopus-adk/pkg/version.version=<pins.generator_version>"`
with exactly `PATH` (the toolchain), `HOME`, `TMPDIR`, `GOPATH` and `GOCACHE`
in the build root, the session `GOMODCACHE`, `GOFLAGS=-mod=readonly
-buildvcs=false`, `GOPROXY=off`, `GOSUMDB=off`, `GOWORK=off`,
`GOTOOLCHAIN=local`, `CGO_ENABLED=0` and `PWD`. The build is offline; when the
local module cache lacks one of the arm's modules the download fails and the
session is refused (`baseline_ref_unsupported` for the baseline arm) until
`--proxy` supplies it. The driver runs under `grader.sb` as well, from a copy
in its own run root with a copy of the codex model catalog, and writes the
default five-platform surface with the candidate manifest's `pins` for both
arms under an empty `PATH` and `HOME`; the runner then moves that surface to
the session. The driver refuses to run when the linked-in version is not the
pin. It uses only API that v0.50.122 already has (`test_golden_surface.py`
builds and runs it there under `grader.sb`, and builds it plainly where
`sandbox-exec` is missing).
`--surfaces <dir>` with `baseline/` and `candidate/` skips the driver. The
protocol records each arm's surface digest (the Go `SurfaceDigest` twin in
`golden_protocol.py`). Every other input comes from the committed manifest:
`live.workspace_revision`, `live.k`, `live.model` and the run cap.

Refusals happen before any agent call and print one JSON document
(`{"status": "refused", "reason": ...}`) with exit 1: `unsupported_os`,
`invalid` (the Go and Python loaders must agree on the agent set; also a
candidate revision the driver cannot serve or an unreadable `--surfaces`),
`protocol_exists`, `output_not_empty`, `run_cap_exceeded`,
`codex_cli_version_mismatch` (`codex --version` against
`pins.codex_cli_version`), `workspace_setup_failed` (the snapshot),
`workspace_mutation_mismatch`, `baseline_ref_unsupported` (the baseline
revision cannot be extracted, or the driver cannot be built or run there) and
`oracle_calibration_failed`. Only the last writes files: `protocol.json` with
the scheduled order and `calibration.json` with the failed `before`, and no
record.

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
`grader.py`, `grader.sb`, `prepare_grader.py`, `surface_driver/main.go` (the
source compiled into every arm revision) and the pilot modules the runner
loads. REQ-HE-07 names `golden.py` alone; the union is the T11 handover.
`grader_profile_sha256` is the SHA-256 of `grader.sb`.

Tests: `test_golden.py`, `test_golden_runner.py` and `test_golden_surface.py`
run everywhere (the v0.50.122 bootstrap needs `go` and the tag);
`go test ./scripts/benchmarks/harness/surface_driver` proves the driver built
with the version linked in writes the surface `harneval.Generate` writes
in-process; `test_golden_session.py` runs whole sessions on a fixture module
under the real profile (macOS), one of them with both arms built by the
driver, and judges them with a freshly built `auto`. The real-corpus
end-to-end session is opt-in:

```sh
HARNEVAL_GOLDEN_E2E=1 PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v test_golden_e2e
```

run from this directory. Its agent is a scripted fake codex; no model is called.

# Release v0.50.130 (A38)

Status: **burned 2026-10-31, never approved.** The coordinate moved to
`v0.50.131`; see [release-v0.50.131.md](release-v0.50.131.md) and
[Burned on 2026-10-31](#burned-on-2026-10-31) below. A37 is `v0.50.128`,
immutable release `427217669` (Latest); see
[release-v0.50.128.md](release-v0.50.128.md).

> **PREDECESSOR ATTEMPT BURNED 2026-10-30.** `v0.50.129` is failed release
> history. The R2-signed tag object `c9fb24309f43d5912da0e5f86a62d20b102c5ee8`
> (commit `fd7009109c5915bdf0e6dfab8b37a502ef764f4d`, tree
> `46c6d8b3fa42ec29463125ba78f740016d979f7e`) and the evidence tag
> `omp-context-evidence-v0.50.129` (object
> `7eca49082b781c29efdae434dde54345b9e73eff`, commit
> `d86e8ff620160778827cb7a48d5d310465ea7934`) exist and must never be moved,
> deleted, recreated, or reused. GitHub release `428858942` remains an
> unpublished draft with zero assets. Ruleset `26187534`
> (`autopus-v0.50.129-release-authority`) and deployment tag policy `64879249`
> (`v0.50.129`) stay as history.
>
> Nothing was published, so no consumer saw a partial release and the Homebrew
> tap never moved. What was lost is the coordinate. This runbook is the next
> attempt.

## What burned v0.50.129

The final signed companion execution smoke in the protected `release` job
failed twice with `managed active OMP version probe failed signal: trace/BPT
trap` on the hosted macOS 15.8.2 image. On macOS 15.8.2 and 26.6.2,
`PT_DETACH` from the exec `SIGTRAP` stop redelivers that trap to the untraced
child, so the Darwin verified-exec gate kills every child it releases. The
measurements are in
[release-v0.50.129.md](release-v0.50.129.md#burned-on-2026-10-30).

**Prerequisite:** the Darwin verified-exec gate fix lands in the same PR as
this coordinate move. Without it, any release job scheduled on a 15.8.2 or
26.x runner burns `v0.50.130` the same way, and users who update to those
macOS versions hit the same failure in the managed OMP path.

## Coordinate move notes (A38 = v0.50.129 -> A38 = v0.50.130)

This is a same-phase re-binding, done by hand the way the A36
`v0.50.126 -> v0.50.127` move was (`410522ba`): every `0.50.129` coordinate
reference outside the runbooks and the CHANGELOG moved to `0.50.130`. The
receipt row was swapped in place (`v0.50.130 0.50.130 A38`), not appended,
because `advance-release-coordinate.sh` always appends and would leave the
burned row beside the new one.

- The burned tag is recorded as rejected: `burned_A38_tag_129` in both lineage
  contracts, `v0.50.129` forbidden in the A38 release workflow contract,
  `omp-context-evidence-v0.50.129` in the evidence-lane forbidden list, the
  `v0.50.129` deployment policy refused by the strict sealed verifier test,
  and the `v0.50.129` receipt row asserted absent. Like every burned
  coordinate, it never enters the phase table. The `v0.50.124` and
  `v0.50.126` records are unchanged.
- The release trigger pin in `pkg/companionmanifest/release_signing_wiring_test.go`
  moved with everything else.
- Phase labels, the A38 ancestor (`842f8bf2`, the A37 source commit), the A37
  predecessor pins, and the Homebrew `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB`
  pins are unchanged. `verify-homebrew-tap-pins.sh` still reports a match
  (head `cfe21549`, Cask `2016dc4f`).
- User-facing notes that named `v0.50.129` as the release that removes
  `role_model_policy.agent_source: native` (help text, the load error,
  README, `docs/README.ko.md`, comments) now name `v0.50.130`, because
  `v0.50.129` never shipped. The CHANGELOG entry of 2026-10-26 that deferred
  the removal to `v0.50.129` is left as written; it records what was announced.

The A38 source commit is not pinned; `release-prep.sh --apply` binds it to
`origin/main` at publication.

## Before `--apply`

| Item | Required state |
|---|---|
| Ruleset `autopus-v0.50.130-release-authority` | absent; create it armed (`26187534` for `v0.50.129` is burned history) |
| Deployment tag policy `v0.50.130` on `adk-companion-release` | absent; create it (`64879249` is the `v0.50.129` history) |
| `v0.50.130` and `omp-context-evidence-v0.50.130` | absent on origin |
| Draft release named `v0.50.130` | absent; Latest is `427217669` (`v0.50.128`) |

Create both before `--preflight`, in the exact shape
`verify-release-tag-ruleset.sh --armed` accepts: target `tag`, enforcement
`active`, `ref_name.include == ["refs/tags/v0.50.130"]`, empty exclude, rules
`creation`, `update`, `deletion`, and the single bypass actor
`{actor_id: 204883817, actor_type: "User", bypass_mode: "always"}`. Never
widen an older ruleset to cover the new tag.

A38 needs fresh evidence. Never reuse any `v0.50.129` report, attestation,
evidence commit, evidence tag object, draft, or workflow artifact.

`--preflight` drops the sudo authorization when it finishes, so authorize
again immediately before `--apply`, in the same terminal:

```bash
export OMP_AUTH_BROKER_URL=http://127.0.0.1:47311
export ADK_GATEWAY_URL=http://127.0.0.1:47312
export ADK_RELEASE_MODEL=gpt-6.1-sol
export ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144
scripts/release-tools/release-prep.sh --preflight
sudo -v && scripts/release-tools/release-prep.sh --apply
```

## Stop conditions

- **Approve the release job within 24 hours of `--apply`.** If the window is
  missed, do not rerun the job: carry the coordinate to `v0.50.131` and
  record the burn here.
- If the tap head moves off `cfe21549` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.50.130`; carry the coordinate to `v0.50.131`.
- If the final signed companion execution smoke fails again with `trace/BPT
  trap`, the gate fix did not cover the runner's macOS; record the image
  version before any rerun.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.50.130`). v0.50.130 changes
  the generated surface (OpenCode conditional rules, the security audit
  section, the removed `agent_source` native path), so a canary failure on the
  *previous* surface expectations is a canary fix, not a release fault.

## Burned on 2026-10-31

Append-only record. `v0.50.130` was tagged and its tag CI ran, but the
protected `release` job was never approved, so nothing was published and the
Homebrew tap never moved.

| Item | Value |
|---|---|
| Release tag `v0.50.130` | R2-signed object `db7e966d062685570224d14d77a90b09b949d2ca`, source `e346c6470d6c150bdfdd7d207fbf6fd0a4c69779` (tree `1894e68da04c300dc9a97f5b930a28026604b45c`) |
| Evidence tag `omp-context-evidence-v0.50.130` | object `79b0c134be1dd22b3b1a43b695518a902bbefca5`, commit `e5a267fbbe9a9676950885218fc00ce1f0bfd714` |
| GitHub release | `429605860`, unpublished draft |
| Ruleset | `26247047` (`autopus-v0.50.130-release-authority`) |
| Deployment tag policy | `64975392` (`v0.50.130`) |
| Release run | `41148530560` |

None of these may be moved, deleted, recreated, or reused.

### First `--apply`: exit 75 on a paginated read

`--preflight` passed on `e346c647`. The first `--apply` created the evidence
tag and its prep lock, then stopped with `release coordinate publish: tag
policy response is invalid` and exit 75 (`rollback incomplete; prep lock
retained for reconciliation`). `publish-release-coordinates.sh` and
`release-coordinate-transaction-lib.sh` read `deployment-branch-policies`
without pagination, so they saw only GitHub's default first page of 30. The
environment held 58 policies; sorted by name, `v0.50.130` was the 31st, so the
publisher found no policy, tried to create the one that already existed
(`64975392`, made before `--preflight` as this runbook required), and could
not parse the answer. `verify-release-tag-ruleset.sh` reads with
`?per_page=100` and had passed.

With the user's approval the policy of the burned `v0.50.129` (`64879249`)
was deleted, which moved `v0.50.130` onto the first page (57 policies, 30th by
name). The retained retry of `--apply` on the same source then committed: tag
object `db7e966d`, draft `429605860`. The readers are fixed in `v0.50.131`
(see its runbook).

### Release run 41148530560: macos-runtime stalled on macOS 15.8.2

Tag CI failed `ci / macos-runtime` on the hosted image
`macos-15-arm64/20261028.0489` (macOS 15.8.2):
`TestPipelineOMPVerifiedExecCommand_DarwinGateDetachesLongRunningChild` timed
out with `gated child pid 25940 did not finish after the gate released it`.
That is the post-detach stall the PR #241 gate fix accepted as roughly 0.5%
per long-running launch: after `PT_DETACH` from the `SIGCONT` stop the child
sometimes stays in `S<` and neither `SIGCONT`, `SIGSTOP`+`SIGCONT`, nor
`PT_ATTACH` wakes it. The same release path runs the long-running RPC smoke
under that gate, and users' managed OMP launches carry the same risk.

The user chose to remove the stall before releasing rather than rerun the
failed job and hope. That needs new source, and the tag is immutable, so
`v0.50.130` is burned and the release job was never approved. A38 moves to
`v0.50.131`.

# Release v0.50.131 (A38)

Status: coordinate re-bound, not yet published. A37 is `v0.50.128`, immutable
release `427217669` (Latest); see [release-v0.50.128.md](release-v0.50.128.md).

> **PREDECESSOR ATTEMPTS BURNED.** `v0.50.129` (2026-10-30) and `v0.50.130`
> (2026-10-31) are failed release history. Their tags, evidence tags, drafts,
> rulesets, and deployment policies are recorded in
> [release-v0.50.129.md](release-v0.50.129.md#burned-on-2026-10-30) and
> [release-v0.50.130.md](release-v0.50.130.md#burned-on-2026-10-31) and must
> never be moved, deleted, recreated, or reused. The `v0.50.129` deployment
> policy `64879249` was deleted on 2026-10-31 with the user's approval; see
> below. Nothing from either attempt was published and the Homebrew tap never
> moved.

## What burned v0.50.130

Two things, in order.

1. The first `--apply` stopped with exit 75 because the coordinate publisher
   read only the first page of 30 deployment-branch policies; `v0.50.130` was
   the 31st by name.
2. After a workaround let the retained retry commit, tag CI failed
   `ci / macos-runtime` on macOS 15.8.2: the long-running child of the Darwin
   verified-exec gate stalled after `PT_DETACH`, the residual risk the PR #241
   fix had accepted. The user chose to remove that risk before releasing.

**Prerequisites in the same PR as this coordinate move:**

- The Darwin verified-exec gate no longer detaches long-running children: it
  stays their tracer and forwards their signals until they exit (main session
  change in `internal/cli`). Without it, a release job scheduled on a 15.8.2
  or 26.6.2 runner can burn `v0.50.131` the same way.
- Every deployment-branch-policy read is paginated:
  `publish-release-coordinates.sh` and `release-coordinate-transaction-lib.sh`
  go through `deployment_policies_json` (`gh api --paginate --slurp
  ...?per_page=100`, merged, failing closed when the merged count differs from
  `total_count`), and `verify-release-tag-ruleset.sh` reads every page the
  same way. `release-deployment-policy-pagination-test.sh` holds the
  regression with 150 policies. The environment held 57 policies at this
  move, so `v0.50.131` would again have fallen past the first page.

## Coordinate move notes (A38 = v0.50.130 -> A38 = v0.50.131)

The same hand re-binding as `v0.50.129 -> v0.50.130` (`cfe10ccd`): every
`0.50.130` coordinate reference in that file set moved to `0.50.131`, and the
receipt row was swapped in place (`v0.50.131 0.50.131 A38`).

- The burned tag is recorded as rejected next to `v0.50.129`:
  `burned_A38_tag_130` in both lineage contracts, `v0.50.130` forbidden in the
  A38 release workflow contract, `omp-context-evidence-v0.50.130` in the
  evidence-lane forbidden list, the `v0.50.130` deployment policy refused by
  the strict sealed verifier test, and the `v0.50.130` receipt row asserted
  absent. It never enters the phase table.
- The release trigger pin in `pkg/companionmanifest/release_signing_wiring_test.go`
  moved with everything else.
- Phase labels, the A38 ancestor (`842f8bf2`), the A37 predecessor pins, and
  the Homebrew `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins are unchanged.
  `verify-homebrew-tap-pins.sh` still reports a match (head `cfe21549`, Cask
  `2016dc4f`).
- User-facing notes that named `v0.50.130` as the release that removes
  `role_model_policy.agent_source: native` now name `v0.50.131` (the same 23
  files as `2527b61f`). The CHANGELOG is unchanged.

The A38 source commit is not pinned; `release-prep.sh --apply` binds it to
`origin/main` at publication.

## Before `--apply`

| Item | Required state |
|---|---|
| Ruleset `autopus-v0.50.131-release-authority` | absent; create it armed (`26187534` and `26247047` are burned history) |
| Deployment tag policy `v0.50.131` on `adk-companion-release` | absent; create it (`64975392` is the `v0.50.130` history) |
| `v0.50.131` and `omp-context-evidence-v0.50.131` | absent on origin |
| Draft release named `v0.50.131` | absent; Latest is `427217669` (`v0.50.128`) |

Create both before `--preflight`, in the exact shape
`verify-release-tag-ruleset.sh --armed` accepts: target `tag`, enforcement
`active`, `ref_name.include == ["refs/tags/v0.50.131"]`, empty exclude, rules
`creation`, `update`, `deletion`, and the single bypass actor
`{actor_id: 204883817, actor_type: "User", bypass_mode: "always"}`. Never
widen an older ruleset to cover the new tag.

A38 needs fresh evidence. Never reuse any `v0.50.129` or `v0.50.130` report,
attestation, evidence commit, evidence tag object, draft, or workflow
artifact.

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
  missed, do not rerun the job: carry the coordinate to `v0.50.132` and
  record the burn here.
- `--apply` exit 75 leaves the prep lock retained on purpose. Inspect the
  remote state, fix only what is outside the source, and retry `--apply` on
  the identical source; never delete the lock or the evidence tag by hand.
- If the tap head moves off `cfe21549` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.50.131`; carry the coordinate to `v0.50.132`.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.50.131`). v0.50.131 changes
  the generated surface (OpenCode conditional rules, the security audit
  section, the removed `agent_source` native path), so a canary failure on the
  *previous* surface expectations is a canary fix, not a release fault.

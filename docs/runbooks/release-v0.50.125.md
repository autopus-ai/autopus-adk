# Release v0.50.125 (A35)

Status: coordinate re-bound, not published. A35 is `v0.50.125`; nothing in
this change creates a tag, an evidence object, a release, or a tap commit.

> **PREDECESSOR ATTEMPT BURNED 2026-10-09.** `v0.50.124` is failed release
> history. The R2-signed tag object `d829956cbc91cb76b87ea17f6f66e4af33a4509e`
> (commit `31af44efe4d33a352bb88c6d8085732adc0efd3f`) and the evidence tag
> `omp-context-evidence-v0.50.124` (object
> `a3e2a0190c92f3f4c37b879330100fccddaf3e8f`, commit
> `1b22ed4c3f9052c4a021a86c5aa58caea046984e`) exist and must never be moved,
> deleted, recreated, or reused. GitHub release `407914733` remains an
> unpublished draft with zero assets. Ruleset `24782887`
> (`autopus-v0.50.124-release-authority`) is sealed and deployment tag policy
> `62473696` (`v0.50.124`) stays as history.
>
> Nothing was published, so no consumer saw a partial release and the Homebrew
> tap never moved. What was lost is the coordinate. This runbook is the next
> attempt.

## What burned v0.50.124

Release run `37933329466` had CI, security, and `omp-production-evidence`
green on both attempts. The protected `release` job failed both times in
`Run GoReleaser`, inside the `scripts/companion-release/produce.sh` post-hook:

| Attempt | Target | Failure |
|---|---|---|
| 1, `2026-10-09T15:05:53Z` | `darwin_arm64` | `companion release execution smoke: verified OMP smoke command failed ... authority-free OMP version probe failed` |
| 2, `2026-10-09T22:18:51Z` | `darwin_amd64` | `codesign verify: test-requirement: code failed to satisfy specified code requirement(s)`, then `code-sign designated requirement verification failed` |

Both are hosted-runner timing assumptions, not source or evidence defects. The
sandboxed `omp --version` probe had a 5-second ceiling that the first launch of
a freshly installed binary on a loaded runner can exceed; it is now 15 seconds
and the smoke reports the probe's own error (`fix(omp): sandbox OMP version
probe 한도를 15초로 늘리고 release smoke가 원인을 보고한다`). `codesign
--check-notarization` ran right after an `Accepted` notarization verdict,
before Apple's ticket lookup answered; only the `notarized` clause is now
retried with backoff, and identity, anchor, and team mismatches still fail at
once (`fix(release): notarization ticket 조회만 backoff로 재시도하고 서명 신원
검사는 즉시 실패시킨다`).

## Coordinate move notes (A35 = v0.50.124 -> A35 = v0.50.125)

This is a same-phase re-binding, so it was done by hand the way the A24
`v0.50.112 -> v0.50.113` move was. `advance-release-coordinate.sh v0.50.124 A35
v0.50.125 A35` handles most of it: it measures `v0.50.124` as unpublished and
moves the history rows in place. Its receipt step, however, always appends, so
it would leave `v0.50.124 0.50.124 A35` beside the new row, and
`TestReleaseCoordinateTable_ProducerRowsMatchDeclaredPhases` would fail. A run
in a scratch copy produced output byte-identical to the manual move for every
other file the script owns.

- The burned tag is recorded as rejected: `burned_A35_tag_124` in both lineage
  contracts, `v0.50.124` forbidden in the A35 release workflow contract,
  `omp-context-evidence-v0.50.124` in the evidence-lane forbidden list, the
  `v0.50.124` deployment policy refused by the strict sealed verifier test, and
  the `v0.50.124` receipt row asserted absent. Like every burned coordinate, it
  never enters the phase table.
- The release trigger pin in `pkg/companionmanifest/release_signing_wiring_test.go`
  moved with everything else. Tag CI runs `go test ./...`, so a stale pin there
  burns the tag by itself.
- Phase labels, the A35 ancestor (`c447badc`), and every A34 predecessor pin
  are unchanged.

## A34 predecessor pins, re-measured 2026-10-10

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `402971620`, `draft=false`, `immutable=true`, 15 assets, author `204883817`; `releases/tags/v0.50.123` equals `releases/402971620` |
| Tag object | `8ac711f82b6879b4bbf582de88658315b037a1b7` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ`, local and origin equal) |
| Source commit / tree | `c447badc28e393b19984d2eeea159a81d609acd9` / `13dcb08d81c59ad0677e824b0785bc7e1fffcbe7` |
| `checksums.txt` | `f15071f66e5056e5e483a03c85f49ebe1fc4e3cc35df24368ca3ca3f35213538` |
| Darwin amd64 / arm64 archive | `74c48a1686ecbeee155bfdaa002a4f1f2e553d222ec351e196507f7f470be071` / `4c8809fda4bede93601711892fc91cbcc334cf59a7e0200848b3d4d9b16b2d94` |
| Linux amd64 / arm64 archive | `864ee59cfbb076941a5c4bc0839b12fb0c7c2ccd6bd99718ba54774dad81cc6f` / `c62d0e10714600d9ce4e9bbdfee668c1e1f828c98489213b1ba8c7f4aed75249` |
| Darwin amd64 / arm64 manifest | `b45f736729725e62a54cec87dce7841723d7a7aec6269345f2ff05849cd8190d` / `b7973fe43ae90398dd14bbc8370822d21be2dbb15df2be31ab760b0abdfa8757` |
| Homebrew tap head | `79860fd05a05c09e954af9ab0315ca26759d86f0` "Publish signed Cask for v0.50.123" (parent `975e205b`) |
| Cask / Formula blob | `a02be82ec113a357b3b1b0c33c26d235f18a3d00` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded darwin archives and `checksums.txt` hash to the API digests, and
the manifests are measured inside those archives. The Cask blob is the same
from `git/trees`, the contents API, and `git hash-object`.

## Before `--apply`

| Item | State on 2026-10-10 |
|---|---|
| Ruleset `autopus-v0.50.125-release-authority` | absent; create it armed |
| Deployment tag policy `v0.50.125` on `adk-companion-release` | absent; create it |
| `v0.50.125` and `omp-context-evidence-v0.50.125` | absent locally and on origin |
| Draft release named `v0.50.125` | absent |

Create the ruleset in the open shape of `24782887` at creation: target `tag`,
enforcement `active`, `ref_name.include == ["refs/tags/v0.50.125"]`, empty
exclude, rules `creation`, `update`, `deletion`, and the single bypass actor
`{actor_id: 204883817, actor_type: "User", bypass_mode: "always"}`. Prep seals
it right after the tag exists. Never widen an older ruleset to cover the new
tag.

A35 needs fresh evidence. Never copy or reuse any `v0.50.124` report,
attestation, evidence commit, evidence tag object, draft, or workflow artifact.

Publish only through the release tooling:

```bash
scripts/release-tools/release-prep.sh --preflight
sudo -v && scripts/release-tools/release-prep.sh --apply
```

If the tap head moves off `79860fd0` before publication, re-measure
`PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first
(`scripts/companion-release/verify-homebrew-tap-pins.sh` reports the drift).
Stop condition unchanged: the tag is immutable once pushed. If anything
mismatches after that, do not move, delete, or reuse `v0.50.125`; carry the
coordinate to `v0.50.126` and record the burn here.

# Release v0.51.1 (A42)

Status: prepared 2026-11-09, not published. A42 is `v0.51.1`. A41 is
`v0.51.0`, immutable release `439021686`; see
[release-v0.51.0.md](release-v0.51.0.md).

## Preparation: room in the lineage coordinates

The `v0.51.0` runbook predicted that A42 would not fit:
`verify-public-key-lineage-coordinates.sh` stood at 295 counted lines and one
coordinate adds 15 (the A41 declaration with its eleven pins, and the A42
branch), and `verify-public-key-lineage-pins.sh` stood at 296, too full for
another 11-line block. Both were measured again before the move and were
unchanged.

This move takes the structural route that runbook named, a second pins helper
sourced before the coordinates helper:

- The A31 through A40 predecessor pin blocks (ten blocks, each a three-line
  comment and eleven `readonly` pins, 140 physical lines) move unchanged and
  in source order to the new `verify-public-key-lineage-shipped-pins.sh`. The
  `A<n>_REPOSITORY`/`TAG`/`VERSION` lines stay in the coordinates file, and its
  pointer comment now names both pins helpers.
- `verify-public-key-lineage.sh` sources the new helper after the pins helper
  and before the coordinates helper, behind the same `-f` and `! -L` gate as
  the others (`lineage shipped pin source is invalid`).
- The executable lineage fixture in
  `pkg/companionmanifest/release_lineage_fixture_io_test.go` provisions the new
  helper beside the others, the no-runtime-override test reads it as runtime
  source, and `release-lineage-pins-hardening-test.sh` checks its gate and
  copies it into the symlink-refusal fixture.

The move changes no value. Sourcing the old pins and coordinates helpers, and
then the new pins, shipped-pins, and coordinates helpers, for every tag in the
phase table plus `v0.50.124`, `v0.50.126`, `v0.50.129`, `v0.50.130`,
`v0.50.134`, `v0.51.1`, and `v0.50.999` (49 tags) dumps byte-identical `A*_`,
`release_phase`, and `prior_*` state; the `readonly` declaration multiset of
the helpers is unchanged; and the pins helper is byte-identical. That
comparison was taken before the A41 declaration and the A42 branch were added.

Line counts toward the 300 limit after the A42 move: coordinates 200, pins
296, shipped pins 111, `validate-source.sh` 249, the lineage verifier 235 (the
release debt headroom ceiling is 250), shipped phases 184. The coordinates
file now takes about six more coordinates before the next block has to move.

## Coordinate move notes (A41 = v0.51.0 -> A42 = v0.51.1)

`advance-release-coordinate.sh v0.51.0 A41 v0.51.1 A42` measured `v0.51.0` as
published and kept the A41 history rows in place. As for A40 and A41, these
were added by hand before it ran:

- `validate-source.sh`: the `v0.51.1) release_phase='A42'` row,
  `A42_A41_ANCESTOR_SHA` (`ce109c5ae958f86b624bc40e91aaf7f6b1bf0f6d`, the A41
  source commit read from the published tag) and its ancestor case arm, and
  `A42` in the tag-object and R2-signature phase lists and the frozen-policy
  message.
- `verify-public-key-lineage-coordinates.sh`: the A41 declaration with the
  eleven pins below, and the A42 branch.
- `verify-public-key-lineage.sh`: `A42` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.51.1` and appended
`v0.51.1 0.51.1 A42` to the receipt table. The rest replays the A41 move
(`7b725b32`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.51.0 0.51.1` and the A41 release id,
  tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.51.0`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`ed215c4e` / `43ae0314`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A42 row in the
  shipped phase table, and the A42 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.50.133` becomes
  `v0.51.0`).

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130`, all four tags forbidden in the
release workflow contract, their evidence tags in the evidence-lane forbidden
list, their deployment policies refused by the strict sealed verifier test, and
their receipt rows asserted absent. The A41 history rows (`v0.51.0` phase row,
receipt row, matrix row, shipped-phase row) stay in place. The `v0.50.134`
ruleset `26722970` and tag policy `65786937` stay as they are.

Apart from the runbook and the lineage helper split, every file the A41 move
touched changes here with the same added and removed line count.

`v0.51.0` was published from `ce109c5a`, which is still `origin/main` when
this was prepared: nothing has merged since. `[Unreleased]` in the CHANGELOG
is not renamed to a version heading, as in earlier releases.

The A42 source commit is not pinned. The ancestor is the A41 source commit;
`release-prep.sh --apply` binds the A42 source to `origin/main` at
publication.

## A41 predecessor pins, measured 2026-11-09

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `439021686`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-11-09T11:27:04Z, `target_commitish` `ce109c5a`; `releases/tags/v0.51.0` and `releases/latest` both resolve to `439021686` |
| Tag object | `30a77fb9a291fc33d76db2f6b89d7e1e86f92a27` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies; local, origin `ls-remote`, `git/ref`, and `git/tags` agree) |
| Source commit / tree | `ce109c5ae958f86b624bc40e91aaf7f6b1bf0f6d` / `c857ba9ff2ba14743ba605e9d822c36f4280d048` |
| `checksums.txt` | `489b3846eb6f98558b4e19b38c32381e87fa8e7cbbb41b6e512280c42bd5e078` |
| Darwin amd64 / arm64 archive | `445789e557180fb3a15210aed76482318c9526644597548141883ff5c7255fb9` / `c355c6c841172df59aa76784996648ad86ddc4f8e949ec37fbd315b6c25629ad` |
| Linux amd64 / arm64 archive | `7db5fa4cd36e3aac584d1a53b87c2de55a81a817874a6cfb07ce642e66e75413` / `88ca3ae74aab3213a61f1895e543a119d43a7046aa624412e9c3370e62623dc9` |
| Darwin amd64 / arm64 manifest | `8cb887d87bcd1d66be134bb960b9aab89e30f3517f16788226ffe3648ad421f9` / `b70a778b4e0d9904f87dcb5506c05c96950027b5b39d901681e5c0ba398c00cc` |
| Evidence tag | `omp-context-evidence-v0.51.0`, object `7bb0f4865d9612ba17010b5799097b400f1c0dea`, commit `1228b0c44b3a389422dc0d175922d1f71a7dc9f6` |
| Homebrew tap head | `ed215c4ebdbdebdb09f96d7d71a7ae8da4fc6298` "Publish signed Cask for v0.51.0" (parent `3d3ee5d3`) |
| Cask / Formula blob | `43ae0314283499f2d68898f3afa647d41fa08d39` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests, and the
four archives hash to their `checksums.txt` entries. The manifests are measured
inside the darwin archives (`adk-companion-manifest.json`), and each manifest's
`artifact_digest` is the `auto` binary beside it (amd64 `06567b7a`, arm64
`2fa61f4d`); both manifests say version `0.51.0`. The tag header names exactly
`ce109c5a`, `commit`, and `v0.51.0`, and the tag's SSH signature verifies
against the R2 trust root (`release-tag-signing-2026-q3-r2.pub`). The A40
source `f78feef6` is an ancestor of `ce109c5a`. The tap head's parent is the
previous `PRIOR_TAP_COMMIT` (`3d3ee5d3`), so the pin chain continues, and the
Formula blob is frozen. The Cask's four sha256 values equal the
`checksums.txt` entries. `release-homebrew-hardening-test.sh` renders the Cask
from the A41 digests and reproduces `43ae0314` byte for byte, and
`verify-homebrew-tap-pins.sh` reports a match.

## Upgrade canary for 0.51.0 -> 0.51.1

The published `v0.51.0` canary run `42609327630` (0.50.133 -> 0.51.0) passed.
For this coordinate the previous release is `0.51.0` itself, so the canary's
previous-surface checks take the verification-harness branch that
`posix-upgrade-canary-omp.sh` already has (`VERIFY_SURFACE_SINCE=0.51.0`: the
four agents `frontend-specialist qa-explorer reviewer ux-validator`). No
canary expectation changes in this move. Unlike the A41 move, a scratch
`live 0.51.0 0.51.1` run against a branch build was not done here; run it, or
rely on the post-publication run below, before treating the previous-surface
branch as proven against the real `0.51.0` binary.

## Before `--apply`

| Item | State on 2026-11-09 |
|---|---|
| Ruleset `autopus-v0.51.1-release-authority` | **missing**. `26746556` for `v0.51.0` is sealed history (no bypass actor) |
| Deployment tag policy `v0.51.1` on `adk-companion-release` | **missing**. `65818617` is the `v0.51.0` history |
| `v0.51.1` and `omp-context-evidence-v0.51.1` | absent on origin, and so is `omp-context-evidence-v0.51.1-source` |
| Draft release named `v0.51.1` | absent; Latest is `439021686` (`v0.51.0`) |

`verify-release-tag-ruleset.sh --armed` fails until both authorities exist.
Create them in the exact shape it accepts, then verify. Prep seals the ruleset
right after the tag exists. Never widen an older ruleset to cover the new tag.

```bash
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.51.1-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.51.1"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.51.1 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.51.0`.

A42 needs fresh evidence. Never reuse any earlier report, attestation,
evidence commit, evidence tag object, draft, or workflow artifact.

Since #291 the `adk-companion-release` environment has no required reviewer,
and since #292 the local live canary's root work goes through the installed
release canary helper instead of an interactive `sudo -v`. Follow the
procedure on current main for both; the auth environment is unchanged:

```bash
export OMP_AUTH_BROKER_URL=http://127.0.0.1:47311
export ADK_GATEWAY_URL=http://127.0.0.1:47312
export ADK_RELEASE_MODEL=gpt-6.1-sol
export ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144
scripts/release-tools/release-prep.sh --preflight
scripts/release-tools/release-prep.sh --apply
```

## Stop conditions

- `--apply` exit 75 leaves the prep lock retained on purpose. Inspect the
  remote state, fix only what is outside the source, and retry `--apply` on
  the identical source; never delete the lock or the evidence tag by hand.
- If the tap head moves off `ed215c4e` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.51.1`; carry the coordinate to `v0.51.2` and
  record the burn here.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.51.1`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

## Next coordinate

A43 fits without structural work: one coordinate adds 15 counted lines to the
coordinates file (200 now) and one 15-line row to the shipped phase table
(184 now).

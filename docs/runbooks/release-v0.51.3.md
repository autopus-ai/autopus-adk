# Release v0.51.3 (A44)

Status: prepared 2026-11-10, not published. A44 is `v0.51.3`. A43 is
`v0.51.2`, immutable release `439814195`; see
[release-v0.51.2.md](release-v0.51.2.md).

## Coordinate move notes (A43 = v0.51.2 -> A44 = v0.51.3)

No structural work was needed, as the `v0.51.2` runbook predicted. The A43
declaration (eleven pins) and the A44 branch went into
`verify-public-key-lineage-coordinates.sh`; no pin block moved to
`verify-public-key-lineage-shipped-pins.sh`.

`advance-release-coordinate.sh v0.51.2 A43 v0.51.3 A44` measured `v0.51.2` as
published and kept the A43 history rows in place. As for A42 and A43, these
were added by hand before it ran:

- `validate-source.sh`: the `v0.51.3) release_phase='A44'` row,
  `A44_A43_ANCESTOR_SHA` (`276d4bd16a9aec143688ab19cfd6affd01f28b00`, the A43
  source commit read from the published tag) and its ancestor case arm, and
  `A44` in the tag-object and R2-signature phase lists and the frozen-policy
  message.
- `verify-public-key-lineage-coordinates.sh`: the A43 declaration with the
  eleven pins below, and the A44 branch.
- `verify-public-key-lineage.sh`: `A44` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.51.3` and appended
`v0.51.3 0.51.3 A44` to the receipt table. The rest replays the A43 move
(`3515b077`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.51.2 0.51.3` and the A43 release id,
  tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.51.2`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`0beaca2c` / `4628f42c`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A44 row in the
  shipped phase table, and the A44 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.51.1` becomes
  `v0.51.2`).

Every file the A43 move touched changes here with the same added and removed
line count.

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130`. The A43 history rows
(`v0.51.2` phase row, receipt row, matrix row, shipped-phase row) stay in
place. The `v0.51.2` ruleset `26903137` and tag policy `66120453` stay as
they are.

`v0.51.2` was published from `276d4bd1`. Since then `origin/main` gained #299
(harness regenerated with the v0.51.2 configuration), #302 (verify stop in
pnpm monorepos and when it cannot run), #303 (no default maxTurns for
long-running agents), #304 (the response language hook checks the turn's
progress notes), #305 (the Stop gate enforces methodology tdd), and #306
(the Stop gate extension for omp sessions), and this move is based on
`1a06f0da`. `[Unreleased]` in the CHANGELOG is not renamed to a version
heading, as in earlier releases.

The A44 source commit is not pinned. The ancestor is the A43 source commit;
`release-prep.sh --apply` binds the A44 source to `origin/main` at
publication.

Line counts toward the 300 limit after the move: coordinates 230, pins 296,
shipped pins 111, `validate-source.sh` 255, the lineage verifier 235, the
shipped phase table 214. Under the 250 release debt headroom ceiling,
`release-lineage-pins-hardening-test.sh` stands at 249 (unchanged by this
move, which only shifts one line in it).

## A43 predecessor pins, measured 2026-11-10

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `439814195`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-11-10T01:13:42Z, `target_commitish` `276d4bd1`; `releases/tags/v0.51.2` and `releases/latest` both resolve to `439814195` |
| Tag object | `83b79d29a8fc82d8a7e38cd1fd122e69bb19f641` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies; local, origin `ls-remote`, `git/ref`, and `git/tags` agree) |
| Source commit / tree | `276d4bd16a9aec143688ab19cfd6affd01f28b00` / `70918efee2ca1a129dc575804359e20bc31a9826` |
| `checksums.txt` | `6beb50075b7c6cf19873d6499c2ccb50dd2bfcf6d3750582dff79026d26821be` |
| Darwin amd64 / arm64 archive | `b5cea2f82cc3970c40c83a94fd641c416cbf31915e01fd1622969339f7cf95d0` / `4696f73a1a57202353a7ed3a157b92294f4e736fcc8401e229c3a5775104c86f` |
| Linux amd64 / arm64 archive | `66a9e1973ec211f861354d2701b214a3afb20e4a91803c84d99ecd02dfd641bc` / `c157b14822fb6601bcd093b4aefba92865bd81302efb791273c82a08e0df3076` |
| Darwin amd64 / arm64 manifest | `5e998973604386c94fe608ccaceb908b8d8e9e816a5c2e949ea2f8b58f91cd70` / `fe99e5eca171c2cdc8f5b28eaaa6a8d882298c4d8c9d71a79d871675498ee1f8` |
| Evidence tag | `omp-context-evidence-v0.51.2`, object `2c4549cd5187d94a180a549e6cb878fda4c17bd6`, commit `743c857c834d0727b72f880b00dc306cabceac18` |
| Homebrew tap head | `0beaca2c5b9aa56f70947f3e92c90315461a3ed0` "Publish signed Cask for v0.51.2" (parent `911176e9`) |
| Cask / Formula blob | `4628f42c643200849a08aade9c5032e947ddba38` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests, and the
four archives hash to their `checksums.txt` entries. The manifests are measured
inside the darwin archives (`adk-companion-manifest.json`), and each manifest's
`artifact_digest` is the `auto` binary beside it (amd64 `19dfcdbf`, arm64
`2c1326f6`); both manifests say version `0.51.2` and build provenance
`276d4bd1`. The tag header names exactly `276d4bd1`, `commit`, and `v0.51.2`,
and the tag's SSH signature verifies against the R2 trust root
(`release-tag-signing-2026-q3-r2.pub`). The A42 source `48c4a165` is an
ancestor of `276d4bd1`. The tap head's parent is the previous
`PRIOR_TAP_COMMIT` (`911176e9`), so the pin chain continues, and the Formula
blob is frozen. The Cask's four sha256 values equal the `checksums.txt`
entries. `release-homebrew-hardening-test.sh` renders the Cask from the A43
digests and reproduces `4628f42c` byte for byte, and
`verify-homebrew-tap-pins.sh` reports a match.

## Upgrade canary for 0.51.2 -> 0.51.3

The `v0.51.2` canary run `42734413976` (0.51.1 -> 0.51.2, at `276d4bd1`)
passed. `assert_previous_claude_surface` expects the verification harness
surface (`auto-review` present, `auto-go` absent) when the previous release is
`0.51.0` or later, which covers a `0.51.2` previous release. No canary
expectation changes in this move. A scratch `live 0.51.2 0.51.3` run against a
branch build was not done here; the post-publication run below is the proof.

## Before `--apply`

| Item | State on 2026-11-10 |
|---|---|
| Ruleset `autopus-v0.51.3-release-authority` | **missing**. `26903137` for `v0.51.2` is sealed history (no bypass actor) |
| Deployment tag policy `v0.51.3` on `adk-companion-release` | **missing**. `66120453` is the `v0.51.2` history |
| `v0.51.3` and `omp-context-evidence-v0.51.3` | absent on origin, and so is `omp-context-evidence-v0.51.3-source` |
| Draft release named `v0.51.3` | absent; Latest is `439814195` (`v0.51.2`) |

`verify-release-tag-ruleset.sh --armed` fails until both authorities exist.
Create them in the exact shape it accepts, then verify. Prep seals the ruleset
right after the tag exists. Never widen an older ruleset to cover the new tag.

```bash
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.51.3-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.51.3"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.51.3 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.51.2`.

A44 needs fresh evidence. Never reuse any earlier report, attestation,
evidence commit, evidence tag object, draft, or workflow artifact.

The `adk-companion-release` environment has no required reviewer (#291), so
the protected release job does not wait for an approval. The local live
canary's root work goes through the installed release canary helper (#292,
#294) instead of an interactive `sudo -v`; install it first if this machine
does not have it. The auth environment is unchanged:

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
- If the tap head moves off `0beaca2c` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.51.3`; carry the coordinate to `v0.51.4` and
  record the burn here.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun. A `ci / test-isolated` 30-minute timeout, as
  on the first `v0.51.1` attempt, is rerun once before it is treated as a
  fault.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.51.3`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

## Next coordinate

A45 fits without structural work: one coordinate adds 15 counted lines to the
coordinates file (230 now) and one 15-line row to the shipped phase table
(214 now).

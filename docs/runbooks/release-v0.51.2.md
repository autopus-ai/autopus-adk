# Release v0.51.2 (A43)

Status: prepared 2026-11-10, not published. A43 is `v0.51.2`. A42 is
`v0.51.1`, immutable release `439292420`; see
[release-v0.51.1.md](release-v0.51.1.md).

## Coordinate move notes (A42 = v0.51.1 -> A43 = v0.51.2)

No structural work was needed, as the `v0.51.1` runbook predicted. The A42
declaration (eleven pins) and the A43 branch went into
`verify-public-key-lineage-coordinates.sh`; no pin block moved to
`verify-public-key-lineage-shipped-pins.sh`.

`advance-release-coordinate.sh v0.51.1 A42 v0.51.2 A43` measured `v0.51.1` as
published and kept the A42 history rows in place. As for A41 and A42, these
were added by hand before it ran:

- `validate-source.sh`: the `v0.51.2) release_phase='A43'` row,
  `A43_A42_ANCESTOR_SHA` (`48c4a16538ef4ab829c50b7852f8217477300f53`, the A42
  source commit read from the published tag) and its ancestor case arm, and
  `A43` in the tag-object and R2-signature phase lists and the frozen-policy
  message.
- `verify-public-key-lineage-coordinates.sh`: the A42 declaration with the
  eleven pins below, and the A43 branch.
- `verify-public-key-lineage.sh`: `A43` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.51.2` and appended
`v0.51.2 0.51.2 A43` to the receipt table. The rest replays the A42 move
(`1792aaa5`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.51.1 0.51.2` and the A42 release id,
  tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.51.1`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`911176e9` / `9360715e`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A43 row in the
  shipped phase table, and the A43 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.51.0` becomes
  `v0.51.1`).

Apart from the A42 move's lineage helper split (the shipped-pins helper, its
fixture, and its gate), every file the A42 move touched changes here with the
same added and removed line count.

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130`. The A42 history rows
(`v0.51.1` phase row, receipt row, matrix row, shipped-phase row) stay in
place. The `v0.51.1` ruleset `26860161` and tag policy `66060511` stay as
they are.

`v0.51.1` was published from `48c4a165`. Since then `origin/main` gained #296
(the upgrade canary previous-surface fix) and #297 (QA service cases), and
this move is based on `55120897`. `[Unreleased]` in the CHANGELOG is not
renamed to a version heading, as in earlier releases.

The A43 source commit is not pinned. The ancestor is the A42 source commit;
`release-prep.sh --apply` binds the A43 source to `origin/main` at
publication.

Line counts toward the 300 limit after the move: coordinates 215, pins 296,
shipped pins 111, `validate-source.sh` 252, the lineage verifier 235, the
shipped phase table 199. Under the 250 release debt headroom ceiling,
`release-lineage-pins-hardening-test.sh` stands at 249 (unchanged by this
move, which only shifts one line in it).

## A42 predecessor pins, measured 2026-11-10

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `439292420`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-11-09T16:08:10Z, `target_commitish` `48c4a165`; `releases/tags/v0.51.1` and `releases/latest` both resolve to `439292420` |
| Tag object | `6df9e3dc4ed4deac09d6527ae253bd055c3c9538` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies; local, origin `ls-remote`, `git/ref`, and `git/tags` agree) |
| Source commit / tree | `48c4a16538ef4ab829c50b7852f8217477300f53` / `6e7b9c8b44980ef935deb96fa41e52c87e169196` |
| `checksums.txt` | `f5d09a7ba471f7ade69cbce3060d2fc72d9ae7ca621ca08a904f808628cafd07` |
| Darwin amd64 / arm64 archive | `5a13a7cb3c3820be047d7f4e9716c7884b1b58896e02ceadc1ff9929367b47f3` / `ca9a128cc17f023a98cd754e5b4c2841268bf766668589e168b55aa4af77a525` |
| Linux amd64 / arm64 archive | `f8236176b6dbf8ea54c178b0061c3125f3836c46a19129167ec455a2e387e8ee` / `393a453a9727cf5422f9237d3f22bc6bc19aa934788d19946e556dad95d6dc7b` |
| Darwin amd64 / arm64 manifest | `15c467efce430bea6059b18bc6503713e4c693ab76c23418bc19822dcdc5c9c5` / `cedaa9534c923e68d522ab1da0cbd3fdcacee97bf92b13abc2bd63a84c56495d` |
| Evidence tag | `omp-context-evidence-v0.51.1`, object `20a90943effa2cbbe0f0aeae25dd02621a9ed928`, commit `5ccadee27eee21e26b34903d9e52b2c568b95f09` |
| Homebrew tap head | `911176e9b96dbe0d0147a932c96933d7a5405e22` "Publish signed Cask for v0.51.1" (parent `ed215c4e`) |
| Cask / Formula blob | `9360715e4ce313e41fc5de493cbc8d2dc35010ac` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests, and the
four archives hash to their `checksums.txt` entries. The manifests are measured
inside the darwin archives (`adk-companion-manifest.json`), and each manifest's
`artifact_digest` is the `auto` binary beside it (amd64 `373c6dad`, arm64
`f154c991`); both manifests say version `0.51.1` and build provenance
`48c4a165`. The tag header names exactly `48c4a165`, `commit`, and `v0.51.1`,
and the tag's SSH signature verifies against the R2 trust root
(`release-tag-signing-2026-q3-r2.pub`). The A41 source `ce109c5a` is an
ancestor of `48c4a165`. The tap head's parent is the previous
`PRIOR_TAP_COMMIT` (`ed215c4e`), so the pin chain continues, and the Formula
blob is frozen. The Cask's four sha256 values equal the `checksums.txt`
entries. `release-homebrew-hardening-test.sh` renders the Cask from the A42
digests and reproduces `9360715e` byte for byte, and
`verify-homebrew-tap-pins.sh` reports a match.

## Upgrade canary for 0.51.1 -> 0.51.2

The `v0.51.1` canary run `42657029502` (0.51.0 -> 0.51.1) passed after #296.
That fix makes `assert_previous_claude_surface` expect the verification
harness surface (`auto-review` present, `auto-go` absent) when the previous
release is `0.51.0` or later, which covers a `0.51.1` previous release. No
canary expectation changes in this move. A scratch `live 0.51.1 0.51.2` run
against a branch build was not done here; the post-publication run below is
the proof.

## Before `--apply`

| Item | State on 2026-11-10 |
|---|---|
| Ruleset `autopus-v0.51.2-release-authority` | **missing**. `26860161` for `v0.51.1` is sealed history (no bypass actor) |
| Deployment tag policy `v0.51.2` on `adk-companion-release` | **missing**. `66060511` is the `v0.51.1` history |
| `v0.51.2` and `omp-context-evidence-v0.51.2` | absent on origin, and so is `omp-context-evidence-v0.51.2-source` |
| Draft release named `v0.51.2` | absent; Latest is `439292420` (`v0.51.1`) |

`verify-release-tag-ruleset.sh --armed` fails until both authorities exist.
Create them in the exact shape it accepts, then verify. Prep seals the ruleset
right after the tag exists. Never widen an older ruleset to cover the new tag.

```bash
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.51.2-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.51.2"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.51.2 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.51.1`.

A43 needs fresh evidence. Never reuse any earlier report, attestation,
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
- If the tap head moves off `911176e9` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.51.2`; carry the coordinate to `v0.51.3` and
  record the burn here.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun. A `ci / test-isolated` 30-minute timeout, as
  on the first `v0.51.1` attempt, is rerun once before it is treated as a
  fault.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.51.2`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

## Next coordinate

A44 fits without structural work: one coordinate adds 15 counted lines to the
coordinates file (215 now) and one 15-line row to the shipped phase table
(199 now).

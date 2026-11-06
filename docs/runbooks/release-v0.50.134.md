# Release v0.50.134 (A41)

Status: prepared 2026-11-06, not published. A41 is `v0.50.134`. A40 is
`v0.50.133`, immutable release `432232109`; see
[release-v0.50.133.md](release-v0.50.133.md).

## Preparation: room in the lineage coordinates

The `v0.50.133` runbook predicted both limits A41 would hit, and both were
real:

- `verify-public-key-lineage-coordinates.sh` stood at 291 counted lines and
  one coordinate adds 15 (the A40 declaration with its eleven pins, and the A41
  branch), so A41 alone would reach 306.
- `internal/companionmanifest/release_lineage_shipped_phases_test.go` stood at
  287 and one row adds 15, so A41 would reach 302.

The first commit makes room without changing any value:

- The A30 predecessor pin block (14 physical lines with its comment, 11 counted
  lines) moves unchanged to the end of `verify-public-key-lineage-pins.sh`,
  after the A29 block, the way `da143f2f` moved A28 and A29. The verifier
  sources the pins helper first, so every phase sees the same variables. The
  `A<n>_REPOSITORY`/`TAG`/`VERSION` lines stay in the coordinates file, and the
  pointer comment now says A23 through A30. Sourcing both helpers for every tag
  in the phase table plus `v0.50.124`, `v0.50.126`, `v0.50.129`, `v0.50.130`,
  `v0.50.134`, and `v0.50.999` (47 tags) dumps byte-identical `A*_`,
  `release_phase`, and `prior_*` state before and after the move, the old pins
  file is an exact byte prefix of the new one, and the `readonly` declaration
  multiset of both files is unchanged.
- The A22..A30 rows of the shipped phase table move unchanged into
  `settledReleasePhases` in the new
  `internal/companionmanifest/release_lineage_settled_phases_test.go`, and
  `releasePhases` concatenates frozen, settled, and shipped in that order, so
  the table order and every row are the same.

Line counts toward the 300 limit after the A41 move: coordinates 295, pins
296, `validate-source.sh` 246, shipped phases 169, settled phases 137.

A42 does not fit as-is: coordinates would reach 310, and the pins file cannot
take another 11-line block (296 + 11 = 307). The preparation for A42 needs a
second pins helper sourced before the coordinates helper, or another
structural change. The shipped phase table has room for several more rows.

## Coordinate move notes (A40 = v0.50.133 -> A41 = v0.50.134)

`advance-release-coordinate.sh v0.50.133 A40 v0.50.134 A41` measured
`v0.50.133` as published and kept the A40 history rows in place. As for A39
and A40, these were added by hand before it ran:

- `validate-source.sh`: the `v0.50.134) release_phase='A41'` row,
  `A41_A40_ANCESTOR_SHA` (`f78feef6824a58aa534f355f2b49763b9b256bbc`, the A40
  source commit) and its ancestor case arm, and `A41` in the tag-object and
  R2-signature phase lists and the frozen-policy message.
- `verify-public-key-lineage-coordinates.sh`: the A40 declaration with the
  eleven pins below, and the A41 branch.
- `verify-public-key-lineage.sh`: `A41` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.50.134` and appended
`v0.50.134 0.50.134 A41` to the receipt table. The rest replays the A40 move
(`77ebcb4b`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.50.133 0.50.134` and the A40
  release id, tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.50.133`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`3d3ee5d3` / `72164d69`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A41 row in the
  shipped phase table, and the A41 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.50.132` becomes
  `v0.50.133`).

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130` in both lineage contracts, all
four tags forbidden in the release workflow contract, their evidence tags in
the evidence-lane forbidden list, their deployment policies refused by the
strict sealed verifier test, and their receipt rows asserted absent. The A40
history rows (`v0.50.133` phase row, receipt row, matrix row, shipped-phase
row) stay in place.

Apart from the runbook and the CHANGELOG, the move touches the same 42 files as
the A40 move, with the same added and removed line count in every file.

Since `v0.50.133`, main gained #264 and #265 (review provider configuration),
#266 (a SPEC, docs only), #267 (agy and claude review providers on
machine-readable output, completion reaper, usage records), #268 (a test date
fix, test only), #269 (the migration no longer rewrites a former-default
review model), and #270 (balanced top rung and claude review default move to
Opus). #270 brought its own `[Unreleased]` entry; #267 and #269 get one each in
this preparation. `[Unreleased]` is not renamed to a version heading, as in
earlier releases.

The A41 source commit is not pinned. The ancestor is the A40 source commit;
`release-prep.sh --apply` binds the A41 source to `origin/main` at
publication.

## A40 predecessor pins, measured 2026-11-06

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `432232109`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-11-03T04:47:09Z, `target_commitish` `f78feef6`; `releases/tags/v0.50.133` and `releases/latest` both resolve to `432232109` |
| Tag object | `3c98be7c8f5ec29a785c5f687a4bf0fb06dc8340` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies; local, origin `ls-remote`, `git/ref`, and `git/tags` agree) |
| Source commit / tree | `f78feef6824a58aa534f355f2b49763b9b256bbc` / `842739577a0c92ee5de0305f0452ad50f258c72d` |
| `checksums.txt` | `987faad8fcc3b141214b79f4d825d55774278f38b432aa212241dd02010f128c` |
| Darwin amd64 / arm64 archive | `27c17cc0bdc2b96d6bd8c35bf1a04757e3315fae49556f1dc8e852b19b4b17b0` / `07379cf39b95f20974905395bd3b03fcea6633d66ac1bc1c428ca69af2767c87` |
| Linux amd64 / arm64 archive | `e6fdfa177f9b1e71871567641823fd87b1160de49cef565fc739c4b784bd70c3` / `a6926d12690f85975ffa2c78597d76dc56de34a870545cda9243e3a737d06663` |
| Darwin amd64 / arm64 manifest | `2740a5e370b1ddb6ef4390f74e15bf4398a2c39bf1d601397fb726e5cbd8ed03` / `bff62f1a82fd1f8bd7ef3cca047269134a0035d2036f4e485e080fd25aa5b85e` |
| Evidence tag | `omp-context-evidence-v0.50.133`, object `4b2f4b4bb681a2e4a29f7c900a73b0818715cc73`, commit `cefb90c32a6ab5c870b958f24e09ff7fa99ea04f` |
| Homebrew tap head | `3d3ee5d38796cf09d150872153b945470b425850` "Publish signed Cask for v0.50.133" (parent `2b30f6d2`) |
| Cask / Formula blob | `72164d696da3cc08806272e0e83c04252d7bc26e` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests, and the
four archives hash to their `checksums.txt` entries. The manifests are measured
inside the darwin archives (`adk-companion-manifest.json`), and each manifest's
`artifact_digest` is the `auto` binary beside it (amd64 `19b5ac5f`, arm64
`dda6ebfa`). The tag header names exactly `f78feef6`, `commit`, and
`v0.50.133`, and `git verify-tag` against the R2 trust root reports a good
signature. The A39 source `2b49d429` is an ancestor of `f78feef6`. The tap
head's parent is the previous `PRIOR_TAP_COMMIT` (`2b30f6d2`), so the pin chain
continues, and the Formula blob is frozen. The Cask's four sha256 values equal
the `checksums.txt` entries. `release-homebrew-hardening-test.sh` renders the
Cask from the A40 digests and reproduces `72164d69` byte for byte, and
`verify-homebrew-tap-pins.sh` reports a match.

## Upgrade canary for 0.50.133 -> 0.50.134

Unlike the A40 move, main changed generated surface since `v0.50.133`: #267
moves the agy and claude review provider defaults in a generated
`autopus.yaml` to machine-readable output and migrates existing entries, and
#269/#270 change the quality presets, the claude review default, and the
notes `auto update` prints. None of these is a canary expectation. The
previous-release checks are the skill and command files, `.codex/config.toml`,
the OMP manifest, and the Autopus OMP agent surface, and those come from the
published `0.50.133` binary, which does not change. The candidate checks are
the current surface after `auto update`, the Lore commit hooks, and the
`doctor --json` config, hooks, and OMP validation checks; #267/#269/#270 touch
none of them. The canary's expectations are therefore unchanged.

This was reproduced rather than assumed. Before publication `live 0.50.133
0.50.134` cannot install the unpublished candidate, so a scratch copy of
`posix-upgrade-canary.sh` (with `posix-upgrade-canary-omp.sh` and `install.sh`
beside it) replaced only the candidate install block with a `cp` of a branch
build (`go build -ldflags "-X
github.com/insajin/autopus-adk/pkg/version.version=0.50.134" ./cmd/auto`). The
previous release was installed by the real `install.sh` with its signature
checks. The run printed `upgrade-canary: PASS mode=live`, with provenance
`previous_public_version 0.50.133`, `candidate_public_version 0.50.134`,
status `admitted`. `auto update` printed the #270 notes (`previous-version
default presets: balanced`, and the claude review provider model
`claude-fable-5-1` is a previous-version default), which the canary does not
assert. This is not the published-asset run; the post-publication run below
is still required.

## Before `--apply`

| Item | State on 2026-11-06 |
|---|---|
| Ruleset `autopus-v0.50.134-release-authority` | **absent**. `26415189` for `v0.50.133` is sealed history. `verify-release-tag-ruleset.sh --armed` from this branch reports the ruleset missing until it is created |
| Deployment tag policy `v0.50.134` on `adk-companion-release` | **absent**. `65295748` is the `v0.50.133` history; the environment holds 59 policies |
| `v0.50.134` and `omp-context-evidence-v0.50.134` | absent on origin, and so is `omp-context-evidence-v0.50.134-source` |
| Draft release named `v0.50.134` | absent; Latest is `432232109` (`v0.50.133`) |

Create both authorities in the exact shape `verify-release-tag-ruleset.sh
--armed` accepts, then verify. Prep seals the ruleset right after the tag
exists. Never widen an older ruleset to cover the new tag.

```bash
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.50.134-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.50.134"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.50.134 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.50.133`.

A41 needs fresh evidence. Never reuse any earlier report, attestation,
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
  missed, do not rerun the job: carry the coordinate to `v0.50.135` and
  record the burn here.
- `--apply` exit 75 leaves the prep lock retained on purpose. Inspect the
  remote state, fix only what is outside the source, and retry `--apply` on
  the identical source; never delete the lock or the evidence tag by hand.
- If the tap head moves off `3d3ee5d3` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.50.134`; carry the coordinate to `v0.50.135`.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.50.134`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

# Release v0.51.5 (A46)

**Published 2026-11-11.** Immutable release `441700303` (15 assets, Latest: yes),
published 2026-11-11T13:34:55Z, tag object
`12941460a548c4bf09014a1c2486937121c14d7a`, source commit
`8ab8b182c425ab8a68112293e8ab272ff3765dcb`, tree
`cc3d28854ecf838a391092162e38d196b955a9c4`.

Release run `43025412326` concluded `success`. The Homebrew tap head is
`80f72b6d` ("Publish signed Cask for v0.51.5", parent `dbdafe4b`), and its Cask
carries the four `checksums.txt` archive digests. Upgrade Canary
`43100603729` (0.51.4 -> 0.51.5) concluded `success`. Ruleset
`27031354` is sealed (no bypass actor), tag policy `66317032`.

The next coordinate is A47 = `v0.51.6`; see
[release-v0.51.6.md](release-v0.51.6.md).

Status: prepared 2026-11-11, not published. A46 is `v0.51.5`. A45 is
`v0.51.4`, immutable release `441368606`; see
[release-v0.51.4.md](release-v0.51.4.md).

## Coordinate move notes (A45 = v0.51.4 -> A46 = v0.51.5)

No structural work was needed, as the `v0.51.4` runbook predicted. The A45
declaration (eleven pins) and the A46 branch went into
`verify-public-key-lineage-coordinates.sh`; no pin block moved to
`verify-public-key-lineage-shipped-pins.sh`.

`advance-release-coordinate.sh v0.51.4 A45 v0.51.5 A46` measured `v0.51.4` as
published and kept the A45 history rows in place. As for A44 and A45, these
were added by hand before it ran:

- `validate-source.sh`: the `v0.51.5) release_phase='A46'` row,
  `A46_A45_ANCESTOR_SHA` (`4312105591a74f79ed3f62a7023423d49a2944f0`, the A45
  source commit read from the published tag) and its ancestor case arm, and
  `A46` in the tag-object and R2-signature phase lists and the frozen-policy
  message.
- `verify-public-key-lineage-coordinates.sh`: the A45 declaration with the
  eleven pins below, and the A46 branch.
- `verify-public-key-lineage.sh`: `A46` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.51.5` and appended
`v0.51.5 0.51.5 A46` to the receipt table. The rest replays the A45 move
(`61c53128`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.51.4 0.51.5` and the A45 release id,
  tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.51.4`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`dbdafe4b` / `43462e2e`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A46 row in the
  shipped phase table, and the A46 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.51.3` becomes
  `v0.51.4`).

Every file the A45 move touched changes here with the same added and removed
line count, except `release-homebrew-hardening-test.sh`, which also moves the
three #320 messages below. Three files that the script does not know name the current
coordinate since #319 and #320, and moved by hand as well:

- `tests/release-tag-ruleset-pagination-test.sh` (#319): the fixture's
  deployment policy and ruleset name move to `v0.51.5`, because the ruleset
  gate it drives now looks for `autopus-v0.51.5-release-authority`. Its
  opening comment about the `v0.51.4` ruleset being the 34th is history and
  stays.
- `tests/release-homebrew-hardening-test.sh` (#320): the three stale-read and
  moved-head scenario messages now say `A46`, like the rest of the file. The
  comment that records the `v0.51.4` stale read (08:13:08) is history and
  stays.
- `verify-release-tag-ruleset.sh` (#319): the script's blanket replace also
  rewrote its history comment ("the v0.51.4 ruleset was the 34th"); that
  comment was put back to `v0.51.4`. The `publish-homebrew-formula-bridge-git.sh`
  comment about the `v0.51.4` stale read is not a replace target and is
  unchanged.

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130`. The A45 history rows
(`v0.51.4` phase row, receipt row, matrix row, shipped-phase row) stay in
place. The `v0.51.4` ruleset `27011377` and tag policy `66282493` stay as
they are.

`v0.51.4` was published from `4312105591a7` (the #319 merge), so #318 and
#319 are already inside it:

- #318: the disk lease admits work by load and free memory and runs in
  parallel while there is room.
- #319: every ruleset reader reads all pages instead of the first 30 (the
  `v0.51.4` ruleset was the 34th).

Since then `origin/main` gained only:

- #320: the Cask publisher retries a final tap ref read that lags the PATCH
  (the `v0.51.4` release run failure), and the canary helper install no
  longer leaves a temporary folder behind because of the read-only module
  cache.

This move is based on `04bafae0`. `[Unreleased]` in the CHANGELOG is not
renamed to a version heading, as in earlier releases.

#320 matters for the release itself. It changed
`scripts/release-tools/install-canary-helper.sh`, and the passwordless canary
helper on this machine was built from an older main, so reinstall it before
`--apply` (see below).

The A46 source commit is not pinned. The ancestor is the A45 source commit;
`release-prep.sh --apply` binds the A46 source to `origin/main` at
publication.

Line counts toward the 300 limit after the move: coordinates 260, pins 296,
shipped pins 111, `validate-source.sh` 261, the lineage verifier 235, the
shipped phase table 244. Under the 250 release debt headroom ceiling,
`release-lineage-pins-hardening-test.sh` stands at 249 (unchanged by this
move, which only shifts one line in it).

## A45 predecessor pins, measured 2026-11-11

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `441368606`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-11-11T08:12:54Z, `target_commitish` `43121055`; `releases/tags/v0.51.4` and `releases/latest` both resolve to `441368606` |
| Tag object | `9130626401937001ccb2366faf62f0f4184534d9` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies; local and origin `ls-remote` agree) |
| Source commit / tree | `4312105591a74f79ed3f62a7023423d49a2944f0` / `77458576f6e16a10312d5222e08a04e35000efc6` |
| `checksums.txt` | `610ef4032e3b7ce28e4845825c6b55091d5ded8b40c9e2c4ea635c6cd23e1140` |
| Darwin amd64 / arm64 archive | `08c5de85142d36b35f79f09f0d35433c83b2cf5a1b252e3bdc1d40f041fdeb44` / `97d26ea4da3df491e79d0e338eb0ae8d5ef7c529fc09172ebc148dc13464ab92` |
| Linux amd64 / arm64 archive | `c8a71813f4f1c7b6822c2353003bdd60a99adcd07fceeb91237844b18e09698d` / `ab90b16f104a9766cde740246f15183c6fc880d2e6cd187b65bac4b67134207d` |
| Darwin amd64 / arm64 manifest | `380c4b20060924832e223e8841dbc2bb4db57d7fcd7e3dfd2fece9cee613efd5` / `18499b51e5bd4413b8ee50223ea4343de036859bc81b4063344a02b9ca6cab95` |
| Evidence tag | `omp-context-evidence-v0.51.4`, object `ba5f6880718246e2b23c23615393c2c74b47f26f`, commit `1773c7cfef81996c7dc9a20358aad4d7e3fc87a9` (orphan, tree `85999da1`) |
| Homebrew tap head | `dbdafe4b5779ff593d07b3f004960f31d7cd2cf4` "Publish signed Cask for v0.51.4" (parent `2c44448e`) |
| Cask / Formula blob | `43462e2e685579697e7ebeae4cefba8ecba58340` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The archives and `checksums.txt` were downloaded into a fresh `mktemp -d`
directory; they hash to the API digests, and the four archives hash to their
`checksums.txt` entries. The manifests are measured inside the darwin archives
(`adk-companion-manifest.json`), and each manifest's `artifact_digest` is the
`auto` binary beside it (amd64 `5e5bde4a`, arm64 `3f0198eb`); both manifests
say version `0.51.4` and build provenance `4312105591a7`. The tag header names
exactly `4312105591a7`, `commit`, and `v0.51.4`, and the tag's SSH signature
verifies against the R2 trust root (`release-tag-signing-2026-q3-r2.pub`).
The A44 source `4a0fb20c` is an ancestor of `4312105591a7`, and
`4312105591a7` is an ancestor of `origin/main`. The tap head's parent is the
previous `PRIOR_TAP_COMMIT` (`2c44448e`), so the pin chain continues, and the
Formula blob is frozen. The Cask's four sha256 values equal the
`checksums.txt` entries. `release-homebrew-hardening-test.sh` renders the
Cask from the A45 digests and reproduces `43462e2e` byte for byte, and
`verify-homebrew-tap-pins.sh` reports a match.

## Upgrade canary for 0.51.4 -> 0.51.5

The `v0.51.4` canary run `42985845286` (0.51.3 -> 0.51.4, at `4312105591a7`)
passed. `assert_previous_claude_surface` expects the verification harness
surface (`auto-review` present, `auto-go` absent) when the previous release is
`0.51.0` or later, which covers a `0.51.4` previous release. No canary
expectation changes in this move. A scratch `live 0.51.4 0.51.5` run against a
branch build was not done here; the post-publication run below is the proof.

## Before `--apply`

| Item | State on 2026-11-11 |
|---|---|
| Ruleset `autopus-v0.51.5-release-authority` | **missing**. `27011377` for `v0.51.4` is sealed history (no bypass actor) |
| Deployment tag policy `v0.51.5` on `adk-companion-release` | **missing**. `66282493` is the `v0.51.4` history |
| `v0.51.5` and `omp-context-evidence-v0.51.5` | absent on origin, and so is `omp-context-evidence-v0.51.5-source` |
| Draft release named `v0.51.5` | absent; Latest is `441368606` (`v0.51.4`) |
| Release canary helper | built from a main older than #320; **reinstall** |

`verify-release-tag-ruleset.sh --armed` fails until both authorities exist.
Create them in the exact shape it accepts, then verify. Prep seals the ruleset
right after the tag exists. Never widen an older ruleset to cover the new tag.

```bash
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.51.5-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.51.5"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.51.5 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.51.4`. The gate paginates since #319, so the new ruleset
is found however many rulesets precede it.

A46 needs fresh evidence. Never reuse any earlier report, attestation,
evidence commit, evidence tag object, draft, or workflow artifact.

The `adk-companion-release` environment has no required reviewer (#291), so
the protected release job does not wait for an approval. The local live
canary's root work goes through the installed release canary helper (#292,
#294) instead of an interactive `sudo -v`. #320 changed
`install-canary-helper.sh`, so reinstall the helper after this move is on
`origin/main` and before `--apply`. The script builds the helper from GitHub's
`main` (not from the checkout), installs it root-owned at
`/Library/PrivilegedHelperTools/co.autopus.release-canary`, writes
`/etc/sudoers.d/autopus-release-canary`, and asks for the password itself.
`sudo` needs a TTY, so run it in a real terminal from your own account (not
root), from a checkout of the repository; a `!` command in Claude Code cannot
answer the password prompt. Then exercise the installed helper without a
release:

```bash
scripts/release-tools/install-canary-helper.sh
scripts/release-tools/canary-helper-smoke.sh
```

The auth environment is unchanged:

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
- If `prepare-release.sh` reports that the installed canary helper differs
  from the source commit, or the helper is missing, stop and rerun
  `install-canary-helper.sh` in a terminal before retrying; do not fall back
  to an interactive `sudo -v`.
- If the tap head moves off `dbdafe4b` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- If `Publish Homebrew Cask` still fails after #320's retry, read the tap
  before anything else: when the head is a single "Publish signed Cask for
  v0.51.5" commit on `dbdafe4b` whose four sha256 values equal
  `checksums.txt`, the Cask is published and the failure is the read, as on
  `v0.51.4`. A head that names any other commit is a real concurrent move.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.51.5`; carry the coordinate to `v0.51.6` and
  record the burn here.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun. A `ci / test-isolated` 30-minute timeout, as
  on the first `v0.51.1` attempt, is rerun once before it is treated as a
  fault.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.51.5`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

## Next coordinate

A47 fits without structural work: one coordinate adds 15 counted lines to the
coordinates file (260 now) and one 15-line row to the shipped phase table
(244 now). A48 still fits (290 and 274); A49 would put the coordinates file
over 300, so that move must first carry older pin blocks to
`verify-public-key-lineage-shipped-pins.sh`.

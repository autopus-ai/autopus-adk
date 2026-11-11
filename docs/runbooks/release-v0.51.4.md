# Release v0.51.4 (A45)

**Published 2026-11-11.** Immutable release `441368606` (15 assets, Latest),
published 2026-11-11T08:12:54Z, tag object
`9130626401937001ccb2366faf62f0f4184534d9`, source commit
`4312105591a74f79ed3f62a7023423d49a2944f0` (the #319 merge), tree
`77458576f6e16a10312d5222e08a04e35000efc6`.

Release run `42981720987` concluded `failure`, and only because the final tap
ref read lagged the PATCH that had just succeeded: `Publish Homebrew Cask`
failed with "Homebrew tap Cask head moved after publication". The Cask commit
`dbdafe4b` ("Publish signed Cask for v0.51.4", parent `2c44448e`) landed at
08:13:08Z, and its four sha256 values equal the `checksums.txt` entries.
Cask publication is the last step of the release job, so nothing after it
was skipped (only post-run steps follow). #320 retries that stale read.
Upgrade Canary `42985845286` (0.51.3 -> 0.51.4) passed. Ruleset `27011377` is
sealed (no bypass actor), tag policy `66282493`.

The armed-ruleset gate first failed: `autopus-v0.51.4-release-authority` was
the 34th ruleset and the readers saw only the first page of 30. #319 made
every ruleset reader paginate, and it merged before `--apply`, so the
published source contains it. The next coordinate is A46 = `v0.51.5`; see
[release-v0.51.5.md](release-v0.51.5.md).

Status: prepared 2026-11-11, not published. A45 is `v0.51.4`. A44 is
`v0.51.3`, immutable release `440878772`; see
[release-v0.51.3.md](release-v0.51.3.md).

## Coordinate move notes (A44 = v0.51.3 -> A45 = v0.51.4)

No structural work was needed, as the `v0.51.3` runbook predicted. The A44
declaration (eleven pins) and the A45 branch went into
`verify-public-key-lineage-coordinates.sh`; no pin block moved to
`verify-public-key-lineage-shipped-pins.sh`.

`advance-release-coordinate.sh v0.51.3 A44 v0.51.4 A45` measured `v0.51.3` as
published and kept the A44 history rows in place. As for A43 and A44, these
were added by hand before it ran:

- `validate-source.sh`: the `v0.51.4) release_phase='A45'` row,
  `A45_A44_ANCESTOR_SHA` (`4a0fb20ca37ea54efe611484d697e1e79ba0b8f1`, the A44
  source commit read from the published tag) and its ancestor case arm, and
  `A45` in the tag-object and R2-signature phase lists and the frozen-policy
  message.
- `verify-public-key-lineage-coordinates.sh`: the A44 declaration with the
  eleven pins below, and the A45 branch.
- `verify-public-key-lineage.sh`: `A45` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.51.4` and appended
`v0.51.4 0.51.4 A45` to the receipt table. The rest replays the A44 move
(`1e540a3c`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.51.3 0.51.4` and the A44 release id,
  tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.51.3`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`2c44448e` / `cd3ae3fd`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A45 row in the
  shipped phase table, and the A45 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.51.2` becomes
  `v0.51.3`).

Every file the A44 move touched changes here with the same added and removed
line count.

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130`. The A44 history rows
(`v0.51.3` phase row, receipt row, matrix row, shipped-phase row) stay in
place. The `v0.51.3` ruleset `26977749` and tag policy `66236974` stay as
they are.

`v0.51.3` was published from `4a0fb20c`. Since then `origin/main` gained:

- #308: a new push to the same PR cancels the CI and security checks still
  running for it.
- #310: `auto secure scan` and the Stop gate credential check.
- #309: the rule reinjection hook timeout goes to 15 seconds.
- #311: security verification returns as the secure route and the
  secure-hunt skill.
- #312: the disk harness (growth tracking, heavy command gate, reclaim while
  working).
- #313: the ci-optimize skill, which fixes the measured bottleneck first.
- #315: `auto deps` and the Stop hook version check, so a turn uses the
  current version instead of a remembered one.
- #314: `auto ci report` and the priority CPU lease.
- #316: the Go toolchain moves to `go1.27.4` (closes GO-2026-6859).

This move is based on `defce5e0`. `[Unreleased]` in the CHANGELOG is not
renamed to a version heading, as in earlier releases.

#316 matters for the release itself. It moved `expected_go_toolchain` in
`prepare-release.sh` and `build-omp-context-candidate.sh` to `go1.27.4`, and
it changed `scripts/release-tools/install-canary-helper.sh` (the helper is
now built with `GOTOOLCHAIN=go1.27.4`). The passwordless canary helper on
this machine was built from an older main, so reinstall it before `--apply`
(see below).

The A45 source commit is not pinned. The ancestor is the A44 source commit;
`release-prep.sh --apply` binds the A45 source to `origin/main` at
publication.

Line counts toward the 300 limit after the move: coordinates 245, pins 296,
shipped pins 111, `validate-source.sh` 258, the lineage verifier 235, the
shipped phase table 229. Under the 250 release debt headroom ceiling,
`release-lineage-pins-hardening-test.sh` stands at 249 (unchanged by this
move, which only shifts one line in it).

## A44 predecessor pins, measured 2026-11-11

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `440878772`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-11-10T21:02:48Z, `target_commitish` `4a0fb20c`; `releases/tags/v0.51.3` and `releases/latest` both resolve to `440878772` |
| Tag object | `e274a155aef5790be8ba769657bd5a19bacf9c10` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies; local and origin `ls-remote` agree) |
| Source commit / tree | `4a0fb20ca37ea54efe611484d697e1e79ba0b8f1` / `56804d3f8118d08fa536a0171b472717c9a7cf33` |
| `checksums.txt` | `fed21d1c14597bc37b2f6231076859362fc881f18fbae3d7137666c61e09c7ee` |
| Darwin amd64 / arm64 archive | `018ccdc92179faadcce32f24a89277021cfde1ae548320160d248015445cb0b9` / `5c3377ebd802e6806bb94010288d82ead5910444e225cdbc9b10a56cdd2b6dfd` |
| Linux amd64 / arm64 archive | `6512d19fc2ff316ebecabe749d3c887f57a2be313037df7445e97a74408f6e76` / `3364759308c582118e696342dd6a3852b17a84ac41ed787106249723f6b8ac73` |
| Darwin amd64 / arm64 manifest | `2db97fd2fbd5e74c992916a3353833b3397b1de187b9c419d57431cfd2c21462` / `8e82aba2309f6bc3260515c47c9c8360cc1fed57d4f26b53f9f126b3105ef51d` |
| Evidence tag | `omp-context-evidence-v0.51.3`, object `18b12f576dccca7767f1a8c87aa92a163354c322`, commit `73f7ed7fe736d7c02087cffd835f3ff4e7122746` |
| Homebrew tap head | `2c44448e0f17c004df0941890f100e4f71489b2d` "Publish signed Cask for v0.51.3" (parent `0beaca2c`) |
| Cask / Formula blob | `cd3ae3fd9d940dd1ef16c50b5f373263c41a127b` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The archives and `checksums.txt` were downloaded into a fresh `mktemp -d`
directory; they hash to the API digests, and the four archives hash to their
`checksums.txt` entries. The manifests are measured inside the darwin archives
(`adk-companion-manifest.json`), and each manifest's `artifact_digest` is the
`auto` binary beside it (amd64 `c3ae3e1a`, arm64 `ba695d40`); both manifests
say version `0.51.3` and build provenance `4a0fb20c`. The tag header names
exactly `4a0fb20c`, `commit`, and `v0.51.3`, and the tag's SSH signature
verifies against the R2 trust root (`release-tag-signing-2026-q3-r2.pub`).
The A43 source `276d4bd1` is an ancestor of `4a0fb20c`, and `4a0fb20c` is an
ancestor of `origin/main`. The tap head's parent is the previous
`PRIOR_TAP_COMMIT` (`0beaca2c`), so the pin chain continues, and the Formula
blob is frozen. The Cask's four sha256 values equal the `checksums.txt`
entries. `release-homebrew-hardening-test.sh` renders the Cask from the A44
digests and reproduces `cd3ae3fd` byte for byte, and
`verify-homebrew-tap-pins.sh` reports a match.

## Upgrade canary for 0.51.3 -> 0.51.4

The `v0.51.3` canary run `42901364379` (0.51.2 -> 0.51.3, at `4a0fb20c`)
passed. `assert_previous_claude_surface` expects the verification harness
surface (`auto-review` present, `auto-go` absent) when the previous release is
`0.51.0` or later, which covers a `0.51.3` previous release. No canary
expectation changes in this move. A scratch `live 0.51.3 0.51.4` run against a
branch build was not done here; the post-publication run below is the proof.

## Before `--apply`

| Item | State on 2026-11-11 |
|---|---|
| Ruleset `autopus-v0.51.4-release-authority` | **missing**. `26977749` for `v0.51.3` is sealed history (no bypass actor) |
| Deployment tag policy `v0.51.4` on `adk-companion-release` | **missing**. `66236974` is the `v0.51.3` history |
| `v0.51.4` and `omp-context-evidence-v0.51.4` | absent on origin, and so is `omp-context-evidence-v0.51.4-source` |
| Draft release named `v0.51.4` | absent; Latest is `440878772` (`v0.51.3`) |
| Release canary helper | built from a main older than #316; **reinstall** |

`verify-release-tag-ruleset.sh --armed` fails until both authorities exist.
Create them in the exact shape it accepts, then verify. Prep seals the ruleset
right after the tag exists. Never widen an older ruleset to cover the new tag.

```bash
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.51.4-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.51.4"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.51.4 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.51.3`.

A45 needs fresh evidence. Never reuse any earlier report, attestation,
evidence commit, evidence tag object, draft, or workflow artifact.

The `adk-companion-release` environment has no required reviewer (#291), so
the protected release job does not wait for an approval. The local live
canary's root work goes through the installed release canary helper (#292,
#294) instead of an interactive `sudo -v`. #316 changed
`install-canary-helper.sh`, so reinstall the helper after this move is on
`origin/main` and before `--apply`. The script builds the helper from GitHub's
`main` (not from the checkout), installs it root-owned at
`/Library/PrivilegedHelperTools/co.autopus.release-canary`, writes
`/etc/sudoers.d/autopus-release-canary`, and asks for the password itself.
Run it from your own account (not root), from a checkout of the repository,
then exercise the installed helper without a release:

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
  `install-canary-helper.sh` before retrying; do not fall back to an
  interactive `sudo -v`.
- If the tap head moves off `2c44448e` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.51.4`; carry the coordinate to `v0.51.5` and
  record the burn here.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun. A `ci / test-isolated` 30-minute timeout, as
  on the first `v0.51.1` attempt, is rerun once before it is treated as a
  fault.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.51.4`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

## Next coordinate

A46 fits without structural work: one coordinate adds 15 counted lines to the
coordinates file (245 now) and one 15-line row to the shipped phase table
(229 now).

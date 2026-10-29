# Release v0.50.128 (A37)

Status: published 2026-10-29. A37 is `v0.50.128`, immutable release
`427217669` (Latest, 15 assets, published 2026-10-29T01:54:41Z) from Release
run `40773521963`. Source commit `842f8bf2` (tree `a13f508c`), tag object
`04fd5405` (R2 signature verifies). Evidence tag
`omp-context-evidence-v0.50.128` (object `8c26a1af`, commit `71d7ddf6`),
ruleset `26072210` sealed (`verify-release-tag-ruleset.sh --sealed` passes),
deployment tag policy `64707948`, Homebrew Cask `cfe21549`, Upgrade Canary
`40805745082` passed.

## Publication record

| Time (UTC) | Event |
|---|---|
| 2026-10-28 21:37:46 | tag run `40773521963` starts; `omp-production-evidence` finishes 21:38:45 |
| 2026-10-29 01:47:52 | `adk-companion-release` approval lands and the `release` job starts, about 4h10m after the tag run, well inside the 24-hour window |
| 2026-10-29 01:54:41 | release `427217669` published, 15 assets |
| 2026-10-29 01:55:06 | `release` job finishes; the tap moves to `cfe21549` "Publish signed Cask for v0.50.128" |

- `release-prep.sh --apply` printed `release tag ruleset differs from exact
  sealed authority policy` once, between the tag push and its final JSON.
  That is the pre-seal probe. The ruleset now has no bypass actor and
  `--sealed` passes.
- The Cask's darwin amd64 and arm64 sha256 equal the `checksums.txt` entries
  (`f4bf3bdb...`, `6f455de9...`).
- The first Upgrade Canary after publication, run `40805061730`, failed with
  `legacy path remains: .omp/agents/executor.md`. The release was not at
  fault. The canary still expected the previous release to generate no OMP
  agents and no `.omp/config.yml`, which held for v0.50.118 through
  v0.50.125. v0.50.127 generates both. PR #234 checks the previous surface by
  version and adds the user setting beside the claimed `task` block instead
  of replacing the file. Replacing the file hands it to the user, and the
  update then warns that the bundled agents stay enabled, which is the
  designed behaviour. Run `40805745082` on that branch installs the public
  0.50.127 and 0.50.128 releases and passed.

A36 is `v0.50.127`, published on 2026-10-23 as immutable release `421737046`
(Latest). The burned `v0.50.126` attempt stays recorded as rejected history
beside `v0.50.124`; see [release-v0.50.127.md](release-v0.50.127.md).

## Coordinate move notes (A36 = v0.50.127 -> A37 = v0.50.128)

`advance-release-coordinate.sh v0.50.127 A36 v0.50.128 A37` measured
`v0.50.127` as published, so it kept the A36 history rows in place. For a
published predecessor the script only checks that the new phase already
exists, so these were added by hand before it ran:

- `validate-source.sh`: the `v0.50.128) release_phase='A37'` row,
  `A37_A36_ANCESTOR_SHA` (`81186d0c`, the A36 source commit) and its ancestor
  case arm, and `A37` in the tag-object and R2-signature phase lists.
- `verify-public-key-lineage-coordinates.sh`: the A36 declaration with the
  eleven pins below, and the A37 branch.
- `verify-public-key-lineage.sh`: `A37` in the three accumulating phase lists.

The script then moved its sixteen replace targets to `v0.50.128` and appended
`v0.50.128 0.50.128 A37` to the receipt table. What it leaves for review was
moved by hand, the same way A36 was:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.50.127 0.50.128` and the A36
  release id, tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.50.127`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins.
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`. Tag CI runs
  `go test ./...`, so a stale pin there burns the tag by itself.
- The workflow contract's predecessor forbidden string (`v0.50.127`). The
  `v0.50.124` and `v0.50.126` burn records are unchanged: `burned_A35_tag_124`
  and `burned_A36_tag_126` in both lineage contracts, both tags forbidden in
  the release workflow contract, `omp-context-evidence-v0.50.126` in the
  evidence-lane forbidden list, both deployment policies refused by the strict
  sealed verifier test, and both receipt rows asserted absent.

### Room for A37

`TestReleaseScripts_SourceFilesStayBelowLimit` caps every release script at
300 counted lines, and tag CI runs it. After the A36 move `validate-source.sh`
was at 300 and the coordinates file at 297, while A37 adds 3 and 15 lines. The
preceding `refactor(release)` commit made room without changing a value:

- `validate-source.sh`: the per-phase ancestor `if/elif` chain (108 lines)
  became a `case` statement with one arm per phase (41 lines). Every
  `readonly` ancestor declaration, every
  `git merge-base --is-ancestor "$A<n>_A<n-1>_ANCESTOR_SHA" "$GITHUB_SHA"`
  call, and every per-phase `fail` message is byte for byte the same; the
  unreachable `*)` arm fails closed. 300 became 231, and A37 brings it to 234.
- The A24..A27 predecessor pin blocks (56 lines with their comments) moved,
  unchanged and in source order, from the coordinates file to the end of
  `verify-public-key-lineage-pins.sh` after the A23 block that A36 moved.
  The lineage verifier sources pins before coordinates, so every phase sees
  the same variables. Coordinates went from 297 to 253 (268 after A37) and
  pins from 219 to 263.

Proof that the refactor changed no value:

- The moved 56 lines hash the same (sha256
  `c283cfb29c3f15a81ddd3c8cf7afcb19479e1a55afb28c8bd30214136e881dbb`) where
  they left and where they landed, the old pins file is an exact prefix of the
  new one, and the multiset of `readonly` declarations across the two files is
  unchanged.
- Sourcing both helpers and dumping every `A*` pin and `release_phase` /
  `prior_*` value for all 41 tags (the 37 phase tags, the burned `v0.50.124`
  and `v0.50.126`, `v0.50.128`, and an unknown tag) gave byte-identical output
  before and after.
- `validate-source.sh` before and after was run against the same three
  fixtures for every one of those tags: a `--shared` clone of HEAD with every
  tag annotated (the accept path, since every ancestor is in history), a
  one-commit repository with annotated tags (the missing-ancestor path), and
  one with lightweight tags (the annotation path). All 123 runs matched in
  combined output, exit status, and `GITHUB_OUTPUT`.

A38 fits as-is (coordinates 283, `validate-source.sh` 237) and so does A39
(298, 240). A40 needs the next pin block (A28) moved to the pins file, which
has room for three more.

The A37 source commit is not pinned. The ancestor is the A36 source commit;
`release-prep.sh --apply` binds the A37 source to `origin/main` at
publication.

## A36 predecessor pins, measured 2026-10-25

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `421737046`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-10-23T22:53:17Z; `releases/tags/v0.50.127` equals `releases/421737046` byte for byte |
| Tag object | `ac00440870377496345d4b104e8554aef7f62c09` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies, local and origin equal) |
| Source commit / tree | `81186d0c73bb6eb633e52a7cb6a2af23d0f54d12` / `7f66f3b2394f0df4c696e961cbd70f4be754b6f6` |
| `checksums.txt` | `19234799128ef9e6cf90be1fa00519ac86c02b4df393f6eeff6185256b77d265` |
| Darwin amd64 / arm64 archive | `c45588b9d88362632646039de8b5475823e8164fb8bd86694c134654906cc42d` / `a37e263dafe09966ef25e2705065a4ff04ad781a43fe8db8754daf20b8f702d2` |
| Linux amd64 / arm64 archive | `4ac9204d6087ea63b03b525f7bb4e9f0a0686d6df11174e7eab56f8c16a4c78d` / `72aeab8e2f7a49bcc2a028d652281d4588637feb6975fad79b6147c063b9d512` |
| Darwin amd64 / arm64 manifest | `0db6e95b614709e844800ca00443feeabca56facb433e0c331b675ac918e2041` / `1252174cf8b2517d9bbc7cd89f70b5067eb3814d777a44128741b3fbc962f082` |
| Homebrew tap head | `e92f7e1943940faf1e287ba9d7b31240afd06b90` "Publish signed Cask for v0.50.127" (parent `37fec98d`) |
| Cask / Formula blob | `68a22308cf408ed965c726257b4d905f73c06009` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests and to the
`checksums.txt` entries, and the manifests are measured inside the darwin
archives. The tag headers name exactly `81186d0c`, `commit`, and `v0.50.127`,
and `git verify-tag` against the R2 trust root reports a good signature. The
Cask blob is the same from `git/trees`, the contents API, and
`git hash-object`, and rendering the A36 digests with
`render_homebrew_cask` reproduces it byte for byte. The parent's Cask blob is
the previous `PRIOR_CASK_BLOB` (`aa3898fb`), so the pin chain continues, and
the Formula blob is frozen in both the head and the parent tree. Re-measuring
`v0.50.125` the same way, archives and manifests included, reproduced all
eleven A35 pins.

## Before `--apply`

| Item | State on 2026-10-25 |
|---|---|
| Ruleset `autopus-v0.50.128-release-authority` | absent; create it armed (`25454106` for `v0.50.126` and `25699619` for `v0.50.127` are sealed history) |
| Deployment tag policy `v0.50.128` on `adk-companion-release` | absent; create it (`63663329` and `64051990` are the `v0.50.126` / `v0.50.127` history) |
| `v0.50.128` and `omp-context-evidence-v0.50.128` | absent locally and on origin |
| Draft release named `v0.50.128` | absent; Latest is `421737046` (`v0.50.127`) |

Create both before `--preflight`. Create the ruleset in the exact shape
`verify-release-tag-ruleset.sh --armed` accepts: target `tag`, enforcement
`active`, `ref_name.include == ["refs/tags/v0.50.128"]`, empty exclude, rules
`creation`, `update`, `deletion`, and the single bypass actor
`{actor_id: 204883817, actor_type: "User", bypass_mode: "always"}`. Prep seals
it right after the tag exists, as it sealed `25699619` for `v0.50.127`. Never
widen an older ruleset to cover the new tag.

A37 needs fresh evidence. Never copy or reuse any `v0.50.127`, `v0.50.126`,
`v0.50.125`, or `v0.50.124` report, attestation, evidence commit, evidence
tag object, draft, or workflow artifact.

Publish only through the release tooling, with the release model and the
production context window named explicitly:

```bash
export OMP_AUTH_BROKER_URL=http://127.0.0.1:47311
export ADK_GATEWAY_URL=http://127.0.0.1:47312
export ADK_RELEASE_MODEL=gpt-6.1-sol
export ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144
scripts/release-tools/release-prep.sh --preflight
sudo -v && scripts/release-tools/release-prep.sh --apply
```

## Stop conditions

- **Approve the release job within 24 hours of `--apply`.** The attestation
  `--apply` signs expires at most 24 hours after it is issued, and the
  same-run evidence artifact is kept for one day; `v0.50.126` burned because
  the approval came after both had expired. Approve the
  `adk-companion-release` deployment as soon as the tag CI finishes. Run
  `--apply` only when someone can approve inside that window. If the window is
  missed, do not rerun the job: the coordinate is burned. Carry it to
  `v0.50.129` and record the burn here.
- If the tap head moves off `e92f7e19` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first
  (`scripts/companion-release/verify-homebrew-tap-pins.sh` reports the drift).
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.50.128`; carry the coordinate to `v0.50.129` and
  record the burn here.

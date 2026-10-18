# Release v0.50.126 (A36)

Status: coordinate advanced, not published. A36 is `v0.50.126`; nothing in
this change creates a tag, an evidence object, a release, or a tap commit.

A35 is `v0.50.125`, published on 2026-10-12 as immutable release `410986343`
(Latest). The burned `v0.50.124` attempt stays recorded as rejected history;
see [release-v0.50.125.md](release-v0.50.125.md).

## Coordinate move notes (A35 = v0.50.125 -> A36 = v0.50.126)

`advance-release-coordinate.sh v0.50.125 A35 v0.50.126 A36` measured
`v0.50.125` as published, so it kept the A35 history rows in place. For a
published predecessor the script only checks that the new phase already
exists, so these were added by hand before it ran:

- `validate-source.sh`: the `v0.50.126) release_phase='A36'` row,
  `A36_A35_ANCESTOR_SHA` (`dd6d6ea2`, the A35 source commit) and its ancestor
  branch, and `A36` in the tag-object and R2-signature phase lists.
- `verify-public-key-lineage-coordinates.sh`: the A35 declaration with the
  eleven pins below, and the A36 branch.
- `verify-public-key-lineage.sh`: `A36` in the three accumulating phase lists.

The script then moved its sixteen replace targets to `v0.50.126` and appended
`v0.50.126 0.50.126 A36` to the receipt table. What it leaves for review was
moved by hand, the same way A35 was:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.50.125 0.50.126` and the A35
  release id, tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.50.125`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins.
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`. Tag CI runs
  `go test ./...`, so a stale pin there burns the tag by itself.

Two scripts hit the 300-line limit that
`TestReleaseScripts_SourceFilesStayBelowLimit` enforces on every release
script. Tag CI runs that test too, so this alone would have burned the tag. The
pattern-faithful move left `validate-source.sh` at 302 counted lines and
`verify-public-key-lineage-coordinates.sh` at 308. Two fixes, neither of which
changes a value:

- `validate-source.sh` lists `A36` on the same line as `A34` and `A35` in both
  phase lists, as A35 shared the A34 line. It is now at 300.
- The A23 predecessor pin block moved, unchanged and with its comments, from
  the coordinates file to the end of `verify-public-key-lineage-pins.sh`,
  right after A22's pins. `verify-public-key-lineage.sh` sources the pins
  helper before the coordinates helper, so every phase still sees the same
  variables. Coordinates is now at 297 and pins at 219. Sourcing both helpers
  for every tag before and after the move produced byte-identical pins and
  prior coordinates.

A37 does not fit as-is: it adds 5 lines to `validate-source.sh` and 15 to the
coordinates file. Before the next move, shrink `validate-source.sh` (the
per-phase ancestor `elif` chain is the largest repeated block) and give the
coordinates file room again.

The `v0.50.124` burn records are unchanged: `burned_A35_tag_124` in both
lineage contracts, `v0.50.124` forbidden in the release workflow contract,
`omp-context-evidence-v0.50.124` in the evidence-lane forbidden list, the
`v0.50.124` deployment policy refused by the strict sealed verifier test, and
the `v0.50.124` receipt row asserted absent.

The A36 source commit is not pinned. The ancestor is the A35 source commit;
`release-prep.sh --apply` binds the A36 source to `origin/main` at
publication.

## A35 predecessor pins, measured 2026-10-19

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `410986343`, `draft=false`, `immutable=true`, 15 assets, author `204883817`; `releases/tags/v0.50.125` equals `releases/410986343` |
| Tag object | `0616d14060c9bbd53d5196faeaa0bc8e108edb2b` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ`, local and origin equal) |
| Source commit / tree | `dd6d6ea2435d759144ec093b9b464a831f47703d` / `8d6aa0c4cf84f2c806205d3210505d100b863233` |
| `checksums.txt` | `5f2bf32db0d96266a7fc84b9bc652163fe16184ce2a8aaeefbedb637d22e0a48` |
| Darwin amd64 / arm64 archive | `52538f80e3f88c3c530e279ab72ed780c33da7045b3b175337ed6a28f4672513` / `5253996a619c350348870fb240301833e69803b2a2ea5e430c0663ab2aa84747` |
| Linux amd64 / arm64 archive | `0dc07d1e74889dc2b3fd511cd4d3c4a470e34b7d26a30b7f55cdbb08df86ae44` / `67c6c9f379b782bb0cea7a39c7a42a6372995e88c8dfbd295e8cfbeb32ee8363` |
| Darwin amd64 / arm64 manifest | `5a4e5cdc55b479eb1fd1942c5885ba2f4599ed9fe9151aebb1c44b95640e1b98` / `79acf762c930a7ad7a1f19f2bf79b2e1dacc31daa8255515f6cd20c2761f0836` |
| Homebrew tap head | `37fec98d1155e061e8f31f3efed05b8455fd5a90` "Publish signed Cask for v0.50.125" (parent `79860fd0`) |
| Cask / Formula blob | `aa3898fbfbe434ef9777a2db7af39811ed1fbf2a` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests and to the
`checksums.txt` entries, and the manifests are measured inside the darwin
archives. The tag headers name exactly `dd6d6ea2` and `v0.50.125`. The Cask
blob is the same from `git/trees`, the contents API, and `git hash-object`,
and rendering the A35 digests reproduces it byte for byte. The parent's Cask
blob is the previous `PRIOR_CASK_BLOB` (`a02be82e`), so the pin chain
continues. Re-measuring `v0.50.123` the same way reproduced all eleven A34
pins.

## Before `--apply`

| Item | State on 2026-10-19 |
|---|---|
| Ruleset `autopus-v0.50.126-release-authority` | absent; create it armed |
| Deployment tag policy `v0.50.126` on `adk-companion-release` | absent; create it |
| `v0.50.126` and `omp-context-evidence-v0.50.126` | absent locally and on origin |
| Draft release named `v0.50.126` | absent |

Create the ruleset in the exact shape `verify-release-tag-ruleset.sh --armed`
accepts: target `tag`, enforcement `active`,
`ref_name.include == ["refs/tags/v0.50.126"]`, empty exclude, rules
`creation`, `update`, `deletion`, and the single bypass actor
`{actor_id: 204883817, actor_type: "User", bypass_mode: "always"}`. Prep seals
it right after the tag exists, as it sealed `24822546` for `v0.50.125`. Never
widen an older ruleset to cover the new tag.

A36 needs fresh evidence. Never copy or reuse any `v0.50.125` or `v0.50.124`
report, attestation, evidence commit, evidence tag object, draft, or workflow
artifact.

Publish only through the release tooling:

```bash
scripts/release-tools/release-prep.sh --preflight
sudo -v && scripts/release-tools/release-prep.sh --apply
```

If the tap head moves off `37fec98d` before publication, re-measure
`PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first
(`scripts/companion-release/verify-homebrew-tap-pins.sh` reports the drift).
Stop condition unchanged: the tag is immutable once pushed. If anything
mismatches after that, do not move, delete, or reuse `v0.50.126`; carry the
coordinate to `v0.50.127` and record the burn here.

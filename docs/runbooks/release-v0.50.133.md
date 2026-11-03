# Release v0.50.133 (A40)

Status: prepared 2026-11-03, not published. A40 is `v0.50.133`. A39 is
`v0.50.132`, immutable release `432079301`; see
[release-v0.50.132.md](release-v0.50.132.md).

## Preparation: room in the lineage coordinates

The `v0.50.132` runbook expected A40 to need only the A28 pin block moved out of
`verify-public-key-lineage-coordinates.sh`. That is not enough. The A39 move
added 15 counted lines there (the A38 declaration with its eleven pins, and the
A39 branch), and A40 adds the same 15. The A28 block is 11 counted lines, so
moving only A28 leaves the file at 298 - 11 + 15 = 302, over the 300 limit.

The first commit therefore moves both the A28 and the A29 predecessor pin blocks
(30 physical lines with their comments) unchanged and in source order to the
end of `verify-public-key-lineage-pins.sh`, the way `5ee80a5c` moved A24..A27.
The verifier sources the pins helper first, so every phase sees the same
variables. Sourcing both helpers for every tag in the phase table plus
`v0.50.124`, `v0.50.126`, `v0.50.129`, `v0.50.130`, `v0.50.133`, and
`v0.50.999` dumps byte-identical `A*_`, `release_phase`, and `prior_*` state
before and after the move.

Line counts toward the 300 limit after the A40 move: coordinates 291,
`validate-source.sh` 243, pins 285, and
`internal/companionmanifest/release_lineage_shipped_phases_test.go` 287. A41
does not fit as-is either:

- coordinates: 291 + 15 = 306. Moving the A30 block (11 counted lines) to the
  pins file first leaves coordinates at 295 and pins at 296 after A41. That is
  the last move the pins file can absorb; the preparation after it needs a
  second pins helper or another structural change.
- the shipped-phase table gains 15 counted lines per phase and reaches 302 with
  A41, so it needs a split before the A41 row is added.

## Coordinate move notes (A39 = v0.50.132 -> A40 = v0.50.133)

`advance-release-coordinate.sh v0.50.132 A39 v0.50.133 A40` measured
`v0.50.132` as published and kept the A39 history rows in place. As for A38
and A39, these were added by hand before it ran:

- `validate-source.sh`: the `v0.50.133) release_phase='A40'` row,
  `A40_A39_ANCESTOR_SHA` (`2b49d429`, the A39 source commit) and its ancestor
  case arm, and `A40` in the tag-object and R2-signature phase lists.
- `verify-public-key-lineage-coordinates.sh`: the A39 declaration with the
  eleven pins below, and the A40 branch.
- `verify-public-key-lineage.sh`: `A40` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.50.133` and appended
`v0.50.133 0.50.133 A40` to the receipt table. The rest was moved by hand by
replaying the A39 move (`d957da8b`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.50.132 0.50.133` and the A39
  release id, tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.50.132`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`2b30f6d2` / `a3fb141f`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A40 row in the
  shipped phase table, and the A40 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.50.131` becomes
  `v0.50.132`).

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130` in both lineage contracts, all
four tags forbidden in the release workflow contract, their evidence tags in
the evidence-lane forbidden list, their deployment policies refused by the
strict sealed verifier test, and their receipt rows asserted absent. The A39
history rows (`v0.50.132` phase row, receipt row, matrix row, shipped-phase
row) stay in place.

Apart from the runbook, the move touches the same 42 files as the A39 move,
with the same added and removed line count in every file.

No CHANGELOG entry is added. Since `v0.50.132`, main gained #260 (this
runbook series, docs only) and #261 (`auto disk trim` skips the cache walk when
inactive), which brought its own `[Unreleased]` entry.

The A40 source commit is not pinned. The ancestor is the A39 source commit;
`release-prep.sh --apply` binds the A40 source to `origin/main` at
publication.

## A39 predecessor pins, measured 2026-11-03

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `432079301`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-11-03T00:09:03Z; `releases/tags/v0.50.132` and `releases/latest` both resolve to `432079301` |
| Tag object | `f2e9aae7ec35d7aa327f9bcf4818903088be7d93` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies, local, origin `ls-remote`, and `git/tags` agree) |
| Source commit / tree | `2b49d429f15be98aecb719e3abefc19a3612a346` / `ed7763a8eaeb3f0252f687586b0a26c39fe97ab4` |
| `checksums.txt` | `49c24324fde2889d61470232e4b84a336719440f1b1a60adc4692907fe5c3199` |
| Darwin amd64 / arm64 archive | `3db537db44b97862446db3e0a8330dd90933725f6ddb4f62205083dae690f69b` / `60998788805d0a9f970adad8a7bb454fe2ee660fba77c15aceb4398b559ca34f` |
| Linux amd64 / arm64 archive | `6534ffe5af52e43548beeb0c1a90dc300bcc5839f3dfd189dd26f11ca5a1c1fb` / `13d4b759809b892cd9b36a815b0f7d00b4e8c88b918e672dbebed917df14ab29` |
| Darwin amd64 / arm64 manifest | `dd8ed1e4763fee5be5e44c5590d357ef8b5abe968bcfbcf3796d5cc2e5784ef6` / `01b152e4f088c652ac22172f43a9d0307386bd475fde214a90eb7ebf93a214e9` |
| Homebrew tap head | `2b30f6d25caa18db2200a5bbf7eff02229c3a893` "Publish signed Cask for v0.50.132" (parent `1e3daa5d`) |
| Cask / Formula blob | `a3fb141fa61b8e2f2d9586b4222b107a73944da6` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests, and the
four archives hash to their `checksums.txt` entries. The manifests are measured
inside the darwin archives (`adk-companion-manifest.json`), and each manifest's
`artifact_digest` is the `auto` binary beside it. The tap head's parent is the
previous `PRIOR_TAP_COMMIT` (`1e3daa5d`), so the pin chain continues, and the
Formula blob is frozen. `release-homebrew-hardening-test.sh` renders the Cask
from the A39 digests and reproduces `a3fb141f` byte for byte, and
`verify-homebrew-tap-pins.sh` reports a match. The tag header names exactly
`2b49d429`, `commit`, and `v0.50.132`, and `git verify-tag` against the R2
trust root reports a good signature.

## Upgrade canary for 0.50.132 -> 0.50.133

Since `v0.50.132` (source `2b49d429`), main changed only `CHANGELOG.md`, the
`v0.50.132` runbook, and `internal/cli/disk*.go`. None of them changes the
generated harness surface, and none is a previous-surface expectation of the
canary: the previous-release checks are the skill and command files,
`.codex/config.toml`, the OMP manifest, and the Autopus OMP agent surface that
every release since `v0.50.127` generates. The canary's expectations are
therefore unchanged.

As a spot check, the published `0.50.132` darwin arm64 binary (from the
downloaded immutable archive) ran `auto init --platforms claude-code,codex,omp
--no-review-gate --yes` in a scratch project. It generated
`.claude/skills/auto-go/SKILL.md`, `.codex/skills/codex-auto/SKILL.md`,
`.omp/skills/auto/SKILL.md`, `.omp/commands/auto.md`,
`.autopus/omp-manifest.json` (without `.agents/skills/`), `.omp/agents/executor.md`,
and `.omp/config.yml`, and `.codex/config.toml` carries
`[features.multi_agent_v2]`. This is not a full local canary run; the
post-publication run below is still required.

The workflow's previous-provenance `jq` checks (release `432079301`, ref, tag
object, commit and tree) pass against the live API as written.

## Before `--apply`

| Item | State on 2026-11-03 |
|---|---|
| Ruleset `autopus-v0.50.133-release-authority` | **already present**: `26415189`, created 2026-11-03T10:36:42+09:00, armed shape (target `tag`, `active`, include `refs/tags/v0.50.133`, rules `creation`/`update`/`deletion`, bypass actor `204883817`). `verify-release-tag-ruleset.sh --armed` from this branch passes. `26408332` for `v0.50.132` is sealed history |
| Deployment tag policy `v0.50.133` on `adk-companion-release` | **already present**: `65295748`. `65286920` is the `v0.50.132` history; the environment holds 59 policies |
| `v0.50.133` and `omp-context-evidence-v0.50.133` | absent on origin, and so is `omp-context-evidence-v0.50.133-source` |
| Draft release named `v0.50.133` | absent; Latest is `432079301` (`v0.50.132`) |

Both authorities were expected to be absent; they were created before this
preparation finished. Do not create them again. If either is ever deleted,
recreate it with the exact shape `verify-release-tag-ruleset.sh --armed`
accepts. Prep seals the ruleset right after the tag exists. Never widen an
older ruleset to cover the new tag.

```bash
# Only if the ruleset or the policy is missing:
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.50.133-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.50.133"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.50.133 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.50.132`.

A40 needs fresh evidence. Never reuse any earlier report, attestation,
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
  missed, do not rerun the job: carry the coordinate to `v0.50.134` and
  record the burn here.
- `--apply` exit 75 leaves the prep lock retained on purpose. Inspect the
  remote state, fix only what is outside the source, and retry `--apply` on
  the identical source; never delete the lock or the evidence tag by hand.
- If the tap head moves off `2b30f6d2` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.50.133`; carry the coordinate to `v0.50.134`.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.50.133`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

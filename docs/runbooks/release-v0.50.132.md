# Release v0.50.132 (A39)

Status: prepared 2026-11-02 and rebased onto main `ef07deb7` (#258, #257) on
2026-11-03; not yet applied. A39 is `v0.50.132`. A38 is
`v0.50.131`, immutable release `429669574` (Latest), published on 2026-10-31;
see [release-v0.50.131.md](release-v0.50.131.md).

## Coordinate move notes (A38 = v0.50.131 -> A39 = v0.50.132)

`advance-release-coordinate.sh v0.50.131 A38 v0.50.132 A39` measured
`v0.50.131` as published and kept the A38 history rows in place. As for A37
and A38, these were added by hand before it ran:

- `validate-source.sh`: the `v0.50.132) release_phase='A39'` row,
  `A39_A38_ANCESTOR_SHA` (`b27d93ab`, the A38 source commit) and its ancestor
  case arm, and `A39` in the tag-object and R2-signature phase lists.
- `verify-public-key-lineage-coordinates.sh`: the A38 declaration with the
  eleven pins below, and the A39 branch.
- `verify-public-key-lineage.sh`: `A39` in the three accumulating phase lists.

The script moved its sixteen replace targets to `v0.50.132` and appended
`v0.50.132 0.50.132 A39` to the receipt table. The rest was moved by hand by
replaying the A38 move (`86a25bfa`) one coordinate forward:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.50.131 0.50.132` and the A38
  release id, tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.50.131`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins (`1e3daa5d` / `8dde665a`).
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A39 row in the
  shipped phase table, and the A39 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.50.128` becomes
  `v0.50.131`).

The burn records are unchanged: `burned_A35_tag_124`, `burned_A36_tag_126`,
`burned_A38_tag_129`, and `burned_A38_tag_130` in both lineage contracts, all
four tags forbidden in the release workflow contract, their evidence tags in
the evidence-lane forbidden list, their deployment policies refused by the
strict sealed verifier test, and their receipt rows asserted absent.

Apart from the runbook, the move touches the same 42 files as the A38 move,
with the same added and removed line count in every file. Normalising
versions, digests, and phase labels, the only remaining differences are the
identifiers that carry a phase number (test names, `A39_A38_ANCESTOR_SHA`, the
`A38_*` pins), the one longer phase list in each accumulator, and the two
burned A38 tags in the forbidden list and its comment.

User-facing notes that name `v0.50.131` as the release that removed
`role_model_policy.agent_source: native` stay as written: `v0.50.131` shipped.
The CHANGELOG gains `[Unreleased]` entries for the PRs merged since
`v0.50.131` that had none (#244, #246, #248 to #254, and #258, which replaced
#255); #247 and #257 brought their own.

Line counts toward the 300 limit after the move: coordinates 298,
`validate-source.sh` 240, pins 263. A40 does not fit as-is: the coordinates
file needs the next pin block (A28) moved to the pins file first, the way the
A37 preparation moved A24..A27.

The A39 source commit is not pinned. The ancestor is the A38 source commit;
`release-prep.sh --apply` binds the A39 source to `origin/main` at
publication.

## A38 predecessor pins, measured 2026-11-02

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `429669574`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-10-31T03:11:32Z; `releases/tags/v0.50.131` equals `releases/429669574` byte for byte |
| Tag object | `a2c7b4794547f4fb9953d8f9cbc84792c89c051a` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies, local and origin equal) |
| Source commit / tree | `b27d93ab0fababe08152ca18807232075a8d1ac6` / `fe82983167e9fb284957f3670bb0edd105370f75` |
| `checksums.txt` | `73b6854d8d0e1a016f59be1bd5996ec6208a40ac7ff6b64684ac763e6ed590ae` |
| Darwin amd64 / arm64 archive | `34a574ada880fcf9744253b0e3ee1279ee56f9b9e1775737730b791c80caf552` / `b568e224786b96b0b48088b42e610f8b1170bf3cebc9876ea08b52274cf750ed` |
| Linux amd64 / arm64 archive | `2b5477171c38f8b8cd429452b8a4d60de421ef051f3aac175de69f3da8e8306a` / `bf67cab3beb98f9bfec511d55966c3308abea6eb69ecfeee6925b660fb89e27a` |
| Darwin amd64 / arm64 manifest | `b12036dabe87f34570ac75b0291a2326b9c7fb90ad65ae496b5eef3c4ba0ae98` / `f1d65865dc3ee7df02d4b511d8a0e4f375e3c79eaee36a5083ee277c9a48dff1` |
| Homebrew tap head | `1e3daa5d9ffc426204f1d66ffef8f614ed88f8a2` "Publish signed Cask for v0.50.131" (parent `cfe21549`) |
| Cask / Formula blob | `8dde665ad305bed80631e5f3020456dcdc5a98e7` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests and to the
`checksums.txt` entries. The manifests are measured inside the darwin archives
(`adk-companion-manifest.json`), and each manifest's `artifact_digest` is the
`auto` binary beside it; the same extraction reproduces the A37 manifest pins
(`6dc13493`, `2be72a57`). The tag header names exactly `b27d93ab`, `commit`,
and `v0.50.131`, and `git verify-tag` against the R2 trust root reports a good
signature. The parent's Cask blob is the previous `PRIOR_CASK_BLOB`
(`2016dc4f`), so the pin chain continues, the Formula blob is frozen in both
trees, and `render_homebrew_cask` with the A38 digests reproduces the
`8dde665a` blob byte for byte. `verify-homebrew-tap-pins.sh` reports a match,
and still did on 2026-11-03 after the rebase.

## Upgrade canary for 0.50.131 -> 0.50.132

Since `v0.50.131`, main changed the generated surface: two managed
`.gitignore` patterns (`**/.autopus/specs/**/review-session.json` and
`.review-session.lock`), the agent-pipeline, delegation, and verification
references, the worktree-isolation skill, the status and init output, the
`auto disk` commands with the `doctor.disk.free_space` row, and (#258, #257)
the spec-review, adaptive-quality, and using-autopus skills plus the optional
`spec.review_gate` keys `judge_mode`, `verify_input`, `pinned_models`,
`judge_model`, and `judge_effort`. None of them is a
previous-surface expectation of the canary: the previous-release checks are
the skill and command files, `.codex/config.toml`, the OMP manifest, and the
Autopus OMP agent surface that every release since `v0.50.127` generates.

Measured on 2026-11-02 against main `27297e2a` and again on 2026-11-03
against main `ef07deb7`, with a scratch copy of `posix-upgrade-canary.sh`
whose only changes replace the candidate installer with a binary built from
that main plus this move as `0.50.132` and keep the work directory for
inspection. Both times the public signed `0.50.131` installed and
initialised, `assert_public_previous_fixture` passed, the update added the two
`.gitignore` patterns, and every current-surface, commit-hook, and doctor
assertion passed (`upgrade-canary: PASS mode=live`). The updated
`autopus.yaml` carries none of the new `spec.review_gate` keys, and the public
`0.50.131` binary still loads it (`doctor.config.autopus_yaml` passes). No
canary change is needed. The run does not cover
anything merged after `ef07deb7`, and the signed `0.50.132` assets exist only
after publication, so the post-publication run below is still required.

## Before `--apply`

| Item | State on 2026-11-03 |
|---|---|
| Ruleset `autopus-v0.50.132-release-authority` | absent; create it armed (`26253379` for `v0.50.131` is sealed history; `26187534` and `26247047` are the burned `v0.50.129` and `v0.50.130`) |
| Deployment tag policy `v0.50.132` on `adk-companion-release` | absent; create it (`64983926` is the `v0.50.131` history). The environment holds 57 policies; the `v0.50.130` policy `64975392` that the `v0.50.131` runbook lists as history no longer exists (404) |
| `v0.50.132` and `omp-context-evidence-v0.50.132` | absent on origin, and so is `omp-context-evidence-v0.50.132-source` |
| Draft release named `v0.50.132` | absent; Latest is `429669574` (`v0.50.131`) |

Create both before `--preflight`. The ruleset must have the exact shape
`verify-release-tag-ruleset.sh --armed` accepts: target `tag`, enforcement
`active`, `ref_name.include == ["refs/tags/v0.50.132"]`, empty exclude, rules
`creation`, `update`, `deletion`, and the single bypass actor
`{actor_id: 204883817, actor_type: "User", bypass_mode: "always"}`. Prep seals
it right after the tag exists. Never widen an older ruleset to cover the new
tag.

```bash
gh api -X POST repos/autopus-ai/autopus-adk/rulesets --input - <<'JSON'
{"name":"autopus-v0.50.132-release-authority","target":"tag","enforcement":"active",
 "bypass_actors":[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}],
 "conditions":{"ref_name":{"include":["refs/tags/v0.50.132"],"exclude":[]}},
 "rules":[{"type":"creation"},{"type":"update"},{"type":"deletion"}]}
JSON
gh api -X POST repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies \
  -f name=v0.50.132 -f type=tag
scripts/companion-release/verify-release-tag-ruleset.sh --armed
```

Run the verifier from a checkout that contains this move; the copy on an older
main still names `v0.50.131`.

A39 needs fresh evidence. Never reuse any earlier report, attestation,
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
  missed, do not rerun the job: carry the coordinate to `v0.50.133` and
  record the burn here.
- `--apply` exit 75 leaves the prep lock retained on purpose. Inspect the
  remote state, fix only what is outside the source, and retry `--apply` on
  the identical source; never delete the lock or the evidence tag by hand.
- If the tap head moves off `1e3daa5d` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.50.132`; carry the coordinate to `v0.50.133`.
- If `ci / macos-runtime` or the final signed companion execution smoke fails
  with `trace/BPT trap` or a gated child that does not finish, record the
  runner image before any rerun.
- After publication, run the Upgrade Canary by hand
  (`gh workflow run upgrade-canary.yaml --ref v0.50.132`). A failure on the
  *previous* surface expectations is a canary fix, not a release fault.

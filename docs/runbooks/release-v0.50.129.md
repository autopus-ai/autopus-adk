# Release v0.50.129 (A38)

Status: **burned 2026-10-30, never published.** A38 moved to `v0.50.130`; see
[Burned on 2026-10-30](#burned-on-2026-10-30) and
[release-v0.50.130.md](release-v0.50.130.md). A37 is `v0.50.128`, published on
2026-10-29 as immutable release `427217669` (Latest); see
[release-v0.50.128.md](release-v0.50.128.md).

## Coordinate move notes (A37 = v0.50.128 -> A38 = v0.50.129)

`advance-release-coordinate.sh v0.50.128 A37 v0.50.129 A38` measured
`v0.50.128` as published and kept the A37 history rows in place. As for A37,
these were added by hand before it ran:

- `validate-source.sh`: the `v0.50.129) release_phase='A38'` row,
  `A38_A37_ANCESTOR_SHA` (`842f8bf2`, the A37 source commit) and its ancestor
  case arm, and `A38` in the tag-object and R2-signature phase lists.
- `verify-public-key-lineage-coordinates.sh`: the A37 declaration with the
  eleven pins below, and the A38 branch.
- `verify-public-key-lineage.sh`: `A38` in the three accumulating phase lists.

The script moved its seventeen replace targets to `v0.50.129` and appended
`v0.50.129 0.50.129 A38` to the receipt table. The rest was moved by hand the
same way as A37, by replaying the A37 commit (`21d2e2a8`) one coordinate
forward and comparing the two diffs file by file:

- Phase labels that name the shipped release in the workflows and verifiers.
- The upgrade canary predecessor: `live 0.50.128 0.50.129` and the A37
  release id, tag object, commit, tree, and darwin arm64 digest.
- The preflight predecessor default (`v0.50.128`) and the Homebrew
  `PRIOR_TAP_COMMIT` / `PRIOR_CASK_BLOB` pins.
- Tests that pin the current coordinate, including the release trigger in
  `pkg/companionmanifest/release_signing_wiring_test.go`, the A38 row in the
  shipped phase table, and the A38 row in the lineage fail-closed matrix.
- The workflow contract's predecessor forbidden string (`v0.50.128`). The
  `v0.50.124` and `v0.50.126` burn records are unchanged.

The only differences between the replayed A37 diff and this one are the lines
A37 removed: A37 moved its predecessor from `v0.50.125` past the burned
`v0.50.126`, while A38 moves from `v0.50.127` to the adjacent `v0.50.128`.

Line counts toward the 300 limit after the move: coordinates 282,
`validate-source.sh` 236, pins 262. A39 still fits; A40 needs the A28 pin block
moved to the pins file first.

The A38 source commit is not pinned. The ancestor is the A37 source commit;
`release-prep.sh --apply` binds the A38 source to `origin/main` at
publication.

## A37 predecessor pins, measured 2026-10-30

Read only, against the immutable release and the live tap:

| Pin | Value |
|---|---|
| Release | `427217669`, `draft=false`, `prerelease=false`, `immutable=true`, 15 assets, author `204883817`, published 2026-10-29T01:54:41Z; `releases/tags/v0.50.128` equals `releases/427217669` byte for byte |
| Tag object | `04fd540532546f2651e63c0de007fbbac1ea3b93` (R2 `SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ` verifies, local and origin equal) |
| Source commit / tree | `842f8bf286aa4c26554cc0d29f3c79c412069a0b` / `a13f508cd95e6a902b3353d8e08c06dedbfae809` |
| `checksums.txt` | `f8d28b4961e79f54470ddd234dbfa3827b897ce57bc0c64d1da4293b11e4543e` |
| Darwin amd64 / arm64 archive | `f4bf3bdbd1f284d327801c5fd8693d4396b696bd2e83ef1331ee40e5456be605` / `6f455de994198a21b0e36abb71fde92f5aa54905418b6bd4942c8b339d227fe2` |
| Linux amd64 / arm64 archive | `9b96e505b019dba5f26501c92a7053d9f175d1eb2cbd7dd410254ca00cc9003f` / `d555b4e0e74b981c0521e50e47a2dda7e1d9952dcf99130a342dbe40d8785845` |
| Darwin amd64 / arm64 manifest | `6dc134938b0c88bdfa52e374dc27ff4185d5bd8dc9597fc4f8cb1c7a20e0a73c` / `2be72a5784bdee43874d41ae69bf29c28bf07f38f9b11afb85092304496953d3` |
| Homebrew tap head | `cfe21549f2c2b2df2d6db54940b761ad9f6c7c07` "Publish signed Cask for v0.50.128" (parent `e92f7e19`) |
| Cask / Formula blob | `2016dc4fd517f01df88d80d6979e24976969b8d8` / frozen `4ebc6c38925002dec00759823d4dd847a499818a` |

The downloaded archives and `checksums.txt` hash to the API digests and to the
`checksums.txt` entries, and the manifests are measured inside the darwin
archives. The tag header names exactly `842f8bf2`, `commit`, and `v0.50.128`,
and `git verify-tag` against the R2 trust root reports a good signature. The
parent's Cask blob is the previous `PRIOR_CASK_BLOB` (`68a22308`), so the pin
chain continues, the Formula blob is frozen in both trees, and
`release-homebrew-hardening-test.sh` renders the A37 digests into exactly the
`2016dc4f` blob. `verify-homebrew-tap-pins.sh` reports a match.

## Before `--apply`

| Item | State on 2026-10-30 |
|---|---|
| Ruleset `autopus-v0.50.129-release-authority` | absent; create it armed (`26072210` for `v0.50.128` is sealed history) |
| Deployment tag policy `v0.50.129` on `adk-companion-release` | absent; create it (`64707948` is the `v0.50.128` history) |
| `v0.50.129` and `omp-context-evidence-v0.50.129` | absent on origin |
| Draft release named `v0.50.129` | absent; Latest is `427217669` (`v0.50.128`) |

Create both before `--preflight`, in the exact shape
`verify-release-tag-ruleset.sh --armed` accepts: target `tag`, enforcement
`active`, `ref_name.include == ["refs/tags/v0.50.129"]`, empty exclude, rules
`creation`, `update`, `deletion`, and the single bypass actor
`{actor_id: 204883817, actor_type: "User", bypass_mode: "always"}`. Never
widen an older ruleset to cover the new tag.

A38 needs fresh evidence. Never reuse any earlier report, attestation,
evidence commit, evidence tag object, draft, or workflow artifact.

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
  missed, do not rerun the job: carry the coordinate to `v0.50.130` and record
  the burn here.
- If the tap head moves off `cfe21549` before publication, re-measure
  `PRIOR_TAP_COMMIT` and `PRIOR_CASK_BLOB` first.
- The tag is immutable once pushed. If anything mismatches after that, do not
  move, delete, or reuse `v0.50.129`; carry the coordinate to `v0.50.130`.
- After publication, run the Upgrade Canary by hand. v0.50.129 changes the
  generated surface (OpenCode conditional rules, the security audit section,
  the removed `agent_source` native path), so a canary failure on the
  *previous* surface expectations is a canary fix, not a release fault.

## Burned on 2026-10-30

`v0.50.129` was armed as A38 and applied, and nothing was published. The
coordinate is burned and carried to `v0.50.130`
([release-v0.50.130.md](release-v0.50.130.md)). The objects below are
immutable history: do not move, delete, or reuse any of them.

| Object | Value |
|---|---|
| Release tag `v0.50.129` | tag object `c9fb24309f43d5912da0e5f86a62d20b102c5ee8` -> commit `fd7009109c5915bdf0e6dfab8b37a502ef764f4d`, tree `46c6d8b3fa42ec29463125ba78f740016d979f7e` |
| Evidence tag `omp-context-evidence-v0.50.129` | tag object `7eca49082b781c29efdae434dde54345b9e73eff` -> commit `d86e8ff620160778827cb7a48d5d310465ea7934` |
| Draft release | `428858942`, zero assets, never published; Latest stays `427217669` (`v0.50.128`) |
| Ruleset `autopus-v0.50.129-release-authority` | `26187534` |
| Deployment tag policy `v0.50.129` | `64879249` |
| Release run | `41035414211` |

`release-prep.sh --apply` reconciled at about 09:42Z. Every CI, security, and
`omp-production-evidence` job of run `41035414211` passed, and the protected
`release` job was approved. Attempt 1 (10:16Z) and attempt 2 (10:47Z, a
`--failed` rerun) both failed in `Run GoReleaser` at the darwin/arm64 post
hook:

```
companion release execution smoke: verified OMP smoke command failed: exit status 255
(stderr: "Error: workflow context-runtime verified-exec-smoke: authority-free OMP
version probe failed: managed active OMP version probe failed signal: trace/BPT trap ")
companion release: final signed companion execution smoke failed
```

Both attempts ran on the hosted image `macos-15-arm64/20261028.0489`, which is
macOS 15.8.2 (24H232). The `v0.50.128` release job ran on
`macos-15-arm64/20260907.0337` (macOS 15.7.9).

### Cause

The Darwin verified-exec gate starts `sandbox-exec` under `PT_TRACE_ME`,
checks the live image at each exec `SIGTRAP` stop, and releases the child with
`PT_DETACH`. On macOS 15.8.2 and 26.6.2 that detach redelivers the exec stop's
pending `SIGTRAP` to the now-untraced child, which dies of it. The OMP binary
is not involved: `/bin/echo` dies the same way.

A standalone replica of the gate, run on the probe branch
`probe/macos158-ptrace`:

| Run | Image | Plain exec | Exec gate with `PT_DETACH` |
|---|---|---|---|
| `41044538480` | macOS 15.7.9 (5 runners) | pass | pass |
| `41044538480` | macOS 15.8.2 (1 runner) | pass | `signal: trace/BPT trap` for `omp` and `/bin/echo`, as the runner user and as `nobody` |
| `41130737478` | `macos-26-arm64/20261026.0455`, macOS 26.6.2 (3 runners) | pass | `signal: trace/BPT trap` |
| local | macOS 26.5.2 | pass | pass |

A rerun cannot help once the pool serves 15.8.2, and moving the job to
`macos-26` does not help either. The tag pins source `fd700910`, so the fix
needs a new source commit and therefore a new coordinate.

### 2026-10-31: deployment tag policy 64879249 deleted

With the user's approval, the `v0.50.129` deployment tag policy `64879249` was
deleted so that the `v0.50.130` policy fell on the first page the unpaginated
coordinate publisher read (see
[release-v0.50.130.md](release-v0.50.130.md#burned-on-2026-10-31)). A burned
tag never deploys, so the deletion also closes the approval path of release
run `41035414211`. Everything else above is unchanged.

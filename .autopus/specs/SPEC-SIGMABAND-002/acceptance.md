# SPEC-SIGMABAND-002 수락 기준

## Test Scenarios

Fixtures reuse SPEC-SIGMABAND-001 (O3 at sample key 1042, episode e1042, BS-BAND-001, `<h8>` of `ci.failure_rate:CI` = c6d37d0a, and the injected claim id a1b2c3d4e5f60708 gives `<c8>` = a1b2c3d4, so `<key>` = ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4). Every test points the user cache directory at a temp dir, so `<lp>` = `<temp cache>/autopus/local-patches/<repo-hash>`. gh, provider, and clock are fakes; S3–S9 and S12 use real git in temp repos with `GIT_TRACE2_EVENT`. The "S4 setup" means: a real repo with a dirty tracked file, an untracked `.env` holding a synthetic ghp_ token, a local bare remote whose main matches refs/remotes/origin/main, marker-writing hooks of every hook type in .git/hooks and in a relative core.hooksPath, allow_local_patch true, and a claude fake with the confined projection whose diagnosis reply is a short text and whose patch reply is one diff fence changing 2 lines of pkg/foo/foo.go. IDs, codes, refs, argv, hashes, file bytes, and counts compare exactly.

### S1: Flag off adds nothing to SPEC-SIGMABAND-001
Priority: Must
Given the O3 fixture and an autopus.yaml without health_band.allow_local_patch
When band runs
Then .autopus/metrics/localpatch-events.jsonl and <lp> do not exist, the diagnosis provider ran with cwd equal to the project directory exactly as SPEC-SIGMABAND-001 defines, and after removing leading -c options the git argv recorder holds 0 invocations whose subcommand is worktree, apply, commit, update-ref, or format-patch
And the default generated autopus.yaml contains no allow_local_patch substring
### S2: Decisions live in their own log and 001's evaluation fields stay the same
Priority: Must
Given the S4 setup, the O3 fixture, and a second copy of the same store run with the flag off under the same injected clock
When band runs on both copies
Then every evaluation event in 001's band-events.jsonl is byte-identical between the copies after the owner field of its claims is removed, except lease_until, which is claim + 990 s in the flag-on copy and claim + 930 s in the flag-off copy, and localpatch-events.jsonl holds exactly one decision record (decision claim, evaluation_seq of the key-1042 event) and one claim record whose depends_on is the key-1042 diagnose claim id and whose key, worktree_path, patch_path, and branch hold the S4 values
And the flag-on BS differs from the flag-off BS only by the three pointer lines at the end of its 추천 방향 section
### S3: Decision order and claim transitions after preparation and diagnosis
Priority: Must
Given the S4 setup and one case at a time
When band runs
Then the recorded reasons are: a --no-agent run whose episode opens at tier 3 = local_patch_skipped:no_agent (row 1 wins over row 4); an older episode of a batch run = local_patch_skipped:superseded_in_batch; a second tier-3 position of a patched episode = local_patch_skipped:episode_already_patched; an episode that opened at tier 2 and later reaches tier 3 = local_patch_skipped:bs_not_tier3; a later tier-3 position of an episode whose tier-3 opening ran with --no-agent = local_patch_skipped:no_opening_claim
And a repo without refs/remotes/origin/main or refs/heads/main writes a prep record with code base_unavailable, gives the diagnosis unavailable(worktree_unavailable), and ends the local_patch claim failed:base_unavailable, not diagnosis_unavailable
And a git worktree add that fails after the prep record (a fake git returning exit 128) writes stage records worktree_intent and worktree_failed, gives the diagnosis unavailable(worktree_unavailable), and ends the claim failed:worktree_failed
And a diagnosis that ends unavailable(provider_timeout) ends the claim failed:diagnosis_unavailable, and a diagnose claim that ends failed:bs_lock_timeout ends it failed:no_bs; in each of these three cases the run made at most 1 provider call and 0 apply, commit, update-ref, or format-patch invocations, and no worktree remains
And the result record of a done claim carries the bs_id that 001 recorded for the diagnose claim before the patch stage started
### S4: The flag on produces one local patch outside the repository and nothing remote
Priority: Must
Given the S4 setup and the O3 fixture
When band runs
Then refs/heads/autopus/band/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4 exists locally, its commit's parent is the base SHA of the prep record, <lp>/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4/worktree/ is a worktree at that commit, and <lp>/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4.patch equals `git format-patch --stdout <base-sha>..autopus/band/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4` byte for byte
And the SHA-256 list of every file under the repository root outside .git/ is identical before and after except .autopus/brainstorms/BS-BAND-001.md and files under .autopus/metrics/, so no worktree or patch file exists inside the repository, and inside .git/ only refs/heads/autopus/band/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4 with its reflog, one worktrees/<name>/ entry, and new objects appear
And the trace2 log holds 0 events with child_class hook, no marker file exists, and the user repo's status, HEAD, index file hash, stash list, and every ref except the new branch are byte-identical before and after
And the result record has status done, bs_id BS-BAND-001, the base_sha, the commit_sha, and the three paths, the commit object's message equals the -F file bytes, including Related: BS-BAND-001 and the lore sign-off, BS-BAND-001's 추천 방향 section ends with the three pointer lines with absolute <lp>, <key>, and <claim-id> filled in, and the run made exactly 2 provider calls
### S5: Every guard of the flow stops before the next step and cleans up
Priority: Must
Given the S4 setup and one reply, configuration, or repository state at a time
When band runs
Then the expected codes are: .github/workflows/ci.yaml, .claude/settings.json, .autopus/specs/x.md, .omp/x.ts, .husky/pre-commit, .HUSKY/x.js, Makefile, package.json, go.mod, AGENTS.md, scripts/run.py, build.rs, conftest.py, vite.config.ts, .eslintrc.cjs, Package.swift, buildSrc/x.kt, .env, config/.env.production, deploy/prod.env, .npmrc, .netrc, .aws/config.py, .ssh/x.py, kubeconfig.py, secrets.py, or tools/run.sh = path_denied; Scripts/run.py beside tracked scripts/ = path_denied:case_collision; a new file with mode 100755, a gitlink, a symlink, a rename, a deletion, a mode change, ../outside.go, a C-quoted path "tools/a\033b.go", a header path set that differs from the `git apply --numstat -z --check` set, or GIT binary patch = patch_invalid; an added line with a ghp_ token, ignore previous instructions, or a 48-character base64 run = patch_content_denied; 11 files, 401 changed lines, or a 70 KiB diff = patch_too_large; zero or two diff fences = no_patch; a staged set differing from the parsed set = patch_invalid; a required trailer Related or Scope-risk = lore_unsupported_required:<trailer>; forbidden_trailers [Related] = lore_rejected; a branch autopus/band/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4 created by the test between step 1 and step 4 = branch_exists; a git commit error = commit_failed
And a C-quoted path "pkg/foo/\303\251t\303\251.go" decodes to pkg/foo/été.go and passes the path checks, and commit.cleanup=strip with core.commentChar=# still yields a commit message equal to the -F file bytes
And every row ends the claim failed:<code> with a result record, leaves no patch file, no worktree, and no branch created by the claim, keeps BS-BAND-001, and exits 0
And a pre-existing directory <lp>/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4/ holding a user file, a pre-existing <key>.patch, or a pre-existing branch autopus/band/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4 each yield the prep code artifact_exists before any worktree add, and that artifact is byte-identical afterwards
And for every row decided at steps 4–7 the `git count-objects -v` output is unchanged and no object holds the reply text, and for the staged-set and commit rows no ref points to a new object
### S6: Attributes, drivers, LFS, and environment cannot run repository code
Priority: Must
Given the S4 setup and one fixture at a time
When band runs
Then each fixture writes a prep record with the code below and ends the local_patch claim failed:<code> before any worktree exists: `*.py filter=fmt` in .git/info/attributes = git_config_unsafe:info_attributes; filter.mark.clean=tools/clean.py = git_config_unsafe:filter.mark.clean; filter.mark.smudge=/usr/bin/python3 tools/smudge.py = git_config_unsafe:filter.mark.smudge; filter.lfs.process=/usr/bin/python3 tools/lfs.py = git_config_unsafe:filter.lfs.process; diff.tc.textconv=/bin/sh tools/tc.sh = git_config_unsafe:diff.tc.textconv; merge.m.driver=tools/m.sh = git_config_unsafe:merge.m.driver; lfs.extension.x.clean=tools/x.sh = git_config_unsafe:lfs.extension.x.clean; lfs.customtransfer.y.path=tools/y = git_config_unsafe:lfs.customtransfer.y.path
And filter.lfs.process=git-lfs filter-process and filter.lfs.process=<absolute path>/git-lfs filter-process each pass, with a fake git-lfs that records GIT_LFS_SKIP_SMUDGE=1 and 0 network requests, while a value with an embedded newline (git-lfs, newline, filter-process), a value with two spaces, and /usr/local/$(id)/git-lfs filter-process each yield git_config_unsafe:filter.lfs.process
And a global core.attributesFile with `*.go filter=fmt` and filter.fmt configured is refused by the driver rule, and every band git command carries -c core.attributesFile=<empty band-owned file> and the environment GIT_ATTR_NOSYSTEM=1
And core.fsmonitor=tools/fsmonitor.py with a patch editing tools/fsmonitor.py to write a marker ends done with no marker, because every git command carries -c core.fsmonitor=false
And inherited GIT_DIR, GIT_INDEX_FILE, and GIT_CONFIG_PARAMETERS pointing at the user's repository appear in no band git environment and leave the user's index file hash unchanged, the git argv recorder holds 0 fetch invocations, and a tier-2 diagnosis with the flag on and an unsafe setting writes a prep record with that code and reports unavailable(worktree_unavailable)
### S7: Diagnosis and patch providers are confined to the band worktree
Priority: Must
Given the S4 setup and one provider configuration at a time
When band runs
Then the diagnosis and the patch request each use a claude argv that holds --restricted, --strict-mcp-config, and --tools=Read,Grep,Glob, with cwd equal to <lp>/ci-failure-rate-ci-c6d37d0a-e1042-a1b2c3d4/worktree/ and no --add-dir, and that worktree holds no .env file
And a codex, gemini, or OMP-backed provider yields the diagnosis status unavailable(provider_unconfined) and the local_patch result failed:diagnosis_unavailable, and a claude projection without --restricted yields unavailable(provider_unconfined) as well
And a tier-2 diagnosis with the flag on runs in <lp>/<key>/worktree/ under its own claim-unique <key>, writes prep and worktree_intent records, and is removed right after the diagnosis only through Cleanup Rule 3, while an artifact already at that <key> yields artifact_exists and stays untouched
### S8: The patch prompt is layered, fenced, and kept in memory
Priority: Must
Given the S4 setup with a run 4242 attempt 1 evidence layer
When band requests the patch
Then the result record's prompt_manifest lists exactly band.patch_instructions.v1 and band.patch_rules.v1 (stable), band.evaluation.<event-hash> (snapshot), and band.diagnosis.<claim-id>, band.evidence.run.4242.a1, and band.base.<base-sha> (ephemeral)
And no layer contains BS-BAND, every ephemeral layer sits in a fence whose info string is untrusted-<32 hex nonce> that occurs nowhere inside the layers, and a refused reply appears in no file, record, or git object
### S9: Chained leases and the Recovery State Table
Priority: Must
Given the S4 setup with three series due in one run (A and C open at tier 3, B is a tier-2 diagnosis), an injected clock, and a crash injected at one point at a time
When band runs and the next run starts
Then the execution order is A diagnose, A local_patch, B diagnose, C diagnose, C local_patch with lease_until at claim + 990 s, 1,800 s, 2,790 s, 3,780 s, and 4,590 s, and a run at +4,591 s marks C's local_patch interrupted and never retries it
And the next run's recovery step takes .autopus/metrics/.recovery.lock, runs every git call with a 30 s timeout outside the store lock, and appends its results before any phase A record of that run, and a second process holding the recovery lock makes that run record recovery_locked
And a crash after the patch-file rename and before patch_done ends with result done, recovered true, the diff file deleted, and every other artifact kept, because the patch file starts with From <claim-commit> and the branch and worktree HEAD are at the claim commit
And a crash after git commit and before commit_done finds the claim commit by its parent and message hash, removes the clean worktree, and ends failed:interrupted; a crash after update-ref and before branch_done deletes the branch with update-ref -d at the claim commit; a crash after git apply --index and before apply_done removes the worktree because its index equals the base plus the recorded diff
And a crash inside git worktree add that leaves the admin entry's locked file removes that worktree with --force --force for the recorded path only, and another worktree of the user stays registered
And a branch the user moved, a worktree in which the user edited or staged another file, and a HEAD that is neither the base nor the claim commit are kept with reasons branch_moved, worktree_modified, and head_unrecognized, all listed in kept[]
### S10: The flag decodes strictly and amends SPEC-SIGMABAND-001
Priority: Must
Given a config with health_band.allow_local_patch true and health_band.diagnosis_provider codex, and a default config saved again
When both are decoded with decodeStrict and the default is read as text
Then AllowLocalPatch is true and DiagnosisProvider is codex, the saved default contains no allow_local_patch substring, and health_band.allow_draft_pr still fails with an unknown-field error naming allow_draft_pr
### S11: Docs and the BS state the local-only boundary
Priority: Must
Given the built binary, the repository docs, and BS-BAND-001 of S4
When auto react band --help, docs/health-band.md, CHANGELOG.md, and the BS are read
Then each of the first three contains health_band.allow_local_patch, autopus/local-patches under the user cache directory, the reviewer warning sentence, the sentence that band never pushes, and the upgrade-before-enable note, and the BS contains the reviewer warning sentence
### S12: Nothing remote ever changes
Priority: Must
Given every run of S3–S9
When the git and gh recorders and the bare remote are inspected
Then the git argv recorder holds 0 invocations whose subcommand is push, fetch, ls-remote, or remote, the gh recorder holds 0 pr calls and 0 api calls with a method other than GET, and the bare remote's refs and objects are byte-identical before and after every run

## Oracle Acceptance Notes

- Every Must scenario carries concrete expected output: refs, argv, codes, record fields, file bytes, hashes, counts, and lease values; there is no statistic, so the explicit tolerance is exact match.
- The step order of the Local Patch Flow fixes each oracle: S3 and S6 take the prep code first, S5 separates object-free refusals (steps 4–7) from post-apply refusals, S4 compares the commit object's message with the -F bytes, and S2 compares 001's evaluation fields that REQ-12 does not amend.
- S6 and S7 are also the acceptance targets of Completion Debt CD-1 and CD-2; they must pass against real git, git-lfs, and claude behavior before sync.
- No Must scenario closes on file existence, a heading, an exit code, or non-empty output alone; every ghp_ string in fixtures is synthetic.

# SPEC-SIGMABAND-002 수락 기준

## Test Scenarios

Fixtures reuse SPEC-SIGMABAND-001 (O3 at sample key 1042, episode e1042, `<h8>` of `ci.failure_rate:CI` = c6d37d0a). gh, provider, and clock are fakes; S3, S4, and S7 use real git in temp repos with `GIT_TRACE2_EVENT`. IDs, codes, refs, argv, and counts compare exactly.

### S1: Flag off keeps the 001 diagnosis-only outcome
Priority: Must
Given the O3 fixture and an autopus.yaml without health_band.allow_draft_pr
When band runs
Then the evaluation event has action diagnose, tier 3, and exactly one claim of kind diagnose, and no claim of kind draft_pr exists
And the recorder holds 0 argv starting with git worktree, git commit, git push, or gh pr, and the default generated autopus.yaml contains no allow_draft_pr substring
### S2: draft_pr claims attach only after a BS and at most once per episode
Priority: Must
Given allow_draft_pr true and one series at a time
When band evaluates the positions described below
Then a first tier-3 position of a new episode records action diagnose with claims [diagnose, draft_pr], and the draft_pr runs after this run's diagnose created BS-BAND-001
And a tier-3 position while another run's diagnose claim of the same episode is still claimed records draft_pr_deferred:diagnose_in_flight with 0 provider calls, and the next tier-3 position after that diagnose is done attaches one draft_pr claim whose commit names that BS ID
And a tier-3 position of an episode whose diagnose ended interrupted without a BS records draft_pr_skipped:episode_without_bs with 0 git or gh calls, and a tier-3 position after a draft_pr claim records draft_pr_skipped:episode_already_drafted
And the evaluation event action is one of log, diagnose, suppressed in every case
### S3: The flag on opens one draft PR while no repository code runs
Priority: Must
Given a real temp repo with a dirty tracked file and an untracked file, an origin bare remote that the gh fake maps to acme/app, push.followTags true and an unpushed annotated tag v9.9.9 on main, marker-writing hooks of every hook type in .git/hooks and in a relative core.hooksPath, default-branch workflows using only push and pull_request, a gh fake reporting only the github-actions check suite (total_count matching) and no status, allow_draft_pr true, the O3 fixture, and a provider fake whose patch reply is one diff fence changing 2 lines of pkg/foo/foo.go and adding a commitlint.config.js that writes a marker when loaded
When band runs
Then the remote refs are exactly refs/heads/main with an unchanged sha and refs/heads/autopus/band/ci-failure-rate-ci-c6d37d0a-e1042, the remote has no tag, the trace2 log holds 0 events with child_class hook, and no marker file exists
And every band git argv carries -c core.hooksPath=<an empty band-owned directory> and -c core.fsmonitor=false, commit and push carry --no-verify, lore.Validate returned no error before the commit, and the message holds [skip ci], Constraint, Related: BS-BAND-001, and the lore sign-off under the subject fix(band): ci-failure-rate-ci anomaly draft (e1042)
And gh pr create runs once with -R acme/app --draft --base main --head autopus/band/ci-failure-rate-ci-c6d37d0a-e1042, the PR body says checks stay skipped until a human pushes, the patch request ran with cwd inside the band worktree, and the episode made exactly 2 provider calls
And no argv holds merge, review, --approve, --auto, --admin, --tags, or --mirror, --force appears only in git worktree remove of the band path, the user's status, HEAD, and stash list are byte-identical, no local branch is added, and the worktree is gone
### S4: Every guard failure stops before the next step
Priority: Must
Given allow_draft_pr true, the S3 setup, and one provider reply, repository state, or gh response at a time
When band runs
Then the expected codes are: .github/workflows/ci.yaml, .claude/settings.json, .autopus/specs/x.md, .omp/x.ts, .husky/pre-commit, .pre-commit-config.yaml, Makefile, package.json, go.mod, AGENTS.md, scripts/run.py, config/.env.production, or tools/run.sh = path_denied; ../outside.go, a rename, a deletion, a mode change, or GIT binary patch = patch_invalid; an added line holding a ghp_ token or ignore previous instructions = patch_content_denied; 11 files or 401 changed lines = patch_too_large; zero or two diff fences = no_patch; a git apply --index --check failure or a staged set differing from the parsed set = patch_invalid; git fetch failure = remote_unavailable; a required trailer Ticket = lore_unfillable:Ticket; forbidden_trailers [Related] = lore_rejected; a git commit error = commit_failed; a remote pre-receive rejection = push_rejected; gh pr create exit 1 = gh_failed
And every row records draft_pr_guard:<code>, opens 0 PRs, keeps the 001 BS, removes the worktree even when it is dirty, and exits 0
And push attempts are 0 for rows before the push step and 1 for push_rejected and gh_failed, and the remote ref set is unchanged for every row except gh_failed, which adds only the band ref and records orphan_branch:refs/heads/autopus/band/ci-failure-rate-ci-c6d37d0a-e1042
### S5: The CI Skip-Coverage Scan refuses unless every lookup is complete and clean
Priority: Must
Given allow_draft_pr true, the S3 setup, and one repository state or gh response at a time
When band runs
Then the expected codes are: a workflow with pull_request_target, workflow_run, or create = ci_skip_unverifiable:trigger:<name>@<file>; an unparsable workflow = ci_skip_unverifiable:parse:<file>; .circleci/config.yml or vercel.json = ci_skip_unverifiable:config:<path>; check suites github-actions and railway-app = ci_skip_unverifiable:check_suite:railway-app; a status context ci/jenkins = ci_skip_unverifiable:status:ci/jenkins
And check suites spread over two pages (30 github-actions suites on page 1, a railway-app suite on page 2, total_count 31) yield ci_skip_unverifiable:check_suite:railway-app, and a fake that returns page 1 only while total_count is 31 yields ci_skip_unverifiable:scan_incomplete
And every refusal makes 0 patch requests (1 provider call in the episode), 0 pushes, and 0 PRs, and keeps the 001 BS
### S6: Triggers that ignore [skip ci] and react to pull-request activity refuse the draft PR
Priority: Must
Given allow_draft_pr true and the S3 setup with one extra default-branch workflow at a time
When band runs
Then workflows triggered by pull_request_review, pull_request_review_comment, or issue_comment each yield ci_skip_unverifiable:trigger:<name>@<file>, and a workflow with only push, pull_request, schedule, workflow_dispatch, workflow_call, merge_group, release, or repository_dispatch passes the trigger check
### S7: Git configuration cannot execute patched worktree code
Priority: Must
Given allow_draft_pr true, the S3 setup, repository config core.fsmonitor=tools/fsmonitor.py and filter.mark.clean=tools/clean.py with a default-branch .gitattributes entry `*.go filter=mark`, and a patch that edits tools/fsmonitor.py and tools/clean.py to write marker files
When band runs
Then no marker file exists, and either the trace2 log holds no child_start event whose argv starts outside git itself, or band refuses with draft_pr_guard:git_config_unsafe:core.fsmonitor or git_config_unsafe:filter.mark.clean before the apply step
### S8: Open-PR enumeration is complete or refuses
Priority: Must
Given allow_draft_pr true, the S3 setup, and a gh fake serving 1,050 open PRs on 11 pages of 100
When band runs
Then a band PR head autopus/band/ci-failure-rate-ci-c6d37d0a-e0999 on page 11 yields open_pr_exists, a pagination error on page 6 yields open_pr_lookup_incomplete, and neither makes a patch request, a push, or a PR
And an open PR head autopus/band/ci-failure-rate-ci-lint-b9d992bd-e7 does not trigger open_pr_exists
### S9: draft_pr leases follow the chained budget
Priority: Must
Given allow_draft_pr true, an injected clock, and the S2 first-tier-3 case
When band claims diagnose and draft_pr in one run, and on a fresh copy claims a draft_pr alone after an earlier done diagnose
Then the draft_pr lease_until is claim + 2,160 s in the first case and claim + 1,230 s in the second
And a run at +2,100 s leaves the first draft_pr claimed, and a run at +2,161 s marks it interrupted and never retries it
### S10: The flag decodes strictly and stays out of default files
Priority: Must
Given a config with health_band.allow_draft_pr true and health_band.diagnosis_provider codex, and a default config saved again
When both are decoded with decodeStrict and the default is read as text
Then AllowDraftPR is true and DiagnosisProvider is codex, the saved default contains no allow_draft_pr substring, and health_band.allow_draft_prs fails with an unknown-field error naming allow_draft_prs
### S11: Docs state the flag, the precondition, and the skipped checks
Priority: Must
Given the built binary and the repository docs
When auto react band --help, docs/health-band.md, and CHANGELOG.md are read
Then each names health_band.allow_draft_pr, the help and docs contain [skip ci], ci_skip_unverifiable, the sentence that enabling the flag asserts no other system builds pushed branches, and the upgrade-before-enable note

## Oracle Acceptance Notes

- Every Must scenario carries concrete expected output: refs, argv, codes, counts, lease values, and expected JSON values; there is no numeric tolerance because no scenario computes a statistic (explicit tolerance: exact match).
- S5 (pagination), S6 (review and comment triggers), and S7 (git configuration) are the oracles of Completion Debt CD-2, CD-3, and CD-1; they fail until that debt is resolved, which blocks sync by design.
- No Must scenario closes on file existence, a heading, an exit code, or non-empty output alone; the git oracles use trace2 events and marker files, not the presence of a branch.
- Every ghp_ string in fixtures is synthetic.

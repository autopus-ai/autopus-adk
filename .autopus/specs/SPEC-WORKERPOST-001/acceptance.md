# SPEC-WORKERPOST-001 Acceptance Criteria

Fixtures are built per test under `t.TempDir()`: a work repository on `main` with one empty base commit `A0` and a bare `origin` holding `main`. Every git command of a test and of the verifier runs with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`. `A`, `B`, `C`, `D`, `U` name commits; `U` is an orphan commit unrelated to `A0`. A contract is shown as its JSON object; omitted keys take the REQ-WPOST-INPUT-01 defaults. "Report" is the decoded `postconditions.json`.

## Test Scenarios

### S1: AC-WPOST-001 - A read-only task completes without git changes
Priority: Must
Given the base fixture with no change after the baseline
When the payload has no contract and the prompt is `Review only; do not commit or push`
Then the verifier returns no error and the report has `source` `legacy_prompt`, `outcome` `not_required`, and `commit`, `push`, and `branch` all `not_required`
And with the contract `{"forbidden_actions":["commit","push"]}` and `origin` pointing at a directory that is not a repository, the report has `source` `contract`, `outcome` `satisfied`, `commit` and `push` `satisfied` with requirement `forbidden`, `branch` `not_required`, and verification ran no `ls-remote`
And through `handleTask` with a mock provider that writes nothing, both payloads end `completed`, while on `109c7ea4` the first ended `failed` with `postcondition failed: commit: HEAD did not advance after execution`
And a digest of every file under the work repository's `.git` and under `origin` is the same before and after.

### S2: AC-WPOST-002 - A stale remote branch fails the push and the exact SHA passes
Priority: Must
Given `feature/stale` pushed to `origin` at `A`, the baseline taken at `A`, and a local commit `B` on `feature/stale` that is not pushed
When the contract is `{"required_actions":["commit","push"],"ref":"feature/stale"}`
Then `push` is `unsatisfied` with `ref` `refs/heads/feature/stale`, `expected` `B`, `observed` `A`, and the error starts `postcondition failed: `
And the fallback prompt `commit and git push origin feature/stale` gives the same `push` result, where `109c7ea4` returned no error
When `B` is pushed to `origin feature/stale`
Then `commit` and `push` are `satisfied`, `observed` is `B`, and the error is nil
And with `"expected_commit":"<A>"` the push is `unsatisfied` under `exact` and `satisfied` under `"push_match":"contains"`.

### S3: AC-WPOST-003 - Another remote, another ref, a suffix ref, and a repointed remote never satisfy a push
Priority: Must
Given HEAD at `B` on `feature/x` and the contract `{"required_actions":["push"],"ref":"feature/x"}`
When `B` is pushed only to a second remote `upstream`
Then `push` is `unsatisfied` with detail `remote branch "feature/x" not found on origin`
And when `B` is pushed to `origin` only as `refs/heads/feature/y`, or only as `refs/heads/other/feature/x`, `push` is `unsatisfied`
When, after the baseline, `origin` is pointed at a second bare repository whose `refs/heads/feature/x` is `B`, or a repository-level `url.<second>.insteadOf` rewrite of the pinned URL is added
Then `push` is `unsatisfied`, because the lookup reads the URL recorded at the baseline outside the repository
When `origin` is the relative path from the work tree to the bare repository and `B` is pushed to it
Then the pinned URL is absolute and `push` is `satisfied`.

### S4: AC-WPOST-004 - Unverifiable pushes are unknown
Priority: Must
Given HEAD at `B` on `feature/x`, `B` pushed to `origin feature/x`
When the contract requires `push` without a `ref`
Then `push` is `unknown` with detail `the contract names no ref`
And with `"remote":"upstream"` and no `upstream` at the baseline, `push` is `unknown`
And with HEAD detached at `B` and no `expected_commit`, `push` is `unknown`, and adding `"expected_commit":"<B>"` makes it `satisfied`
When the pinned URL points at a directory that is not a repository, at an HTTP server answering 401, or at an HTTP server that answers after the verifier's timeout
Then `push` is `unknown` with a detail starting `remote lookup failed`, and the error starts `postcondition unverified: `
And an HTTP server answering 401 to `http://user:pw@…` gives a detail containing `Authentication failed`, and one without credentials `terminal prompts disabled`.

### S5: AC-WPOST-005 - An unrelated commit fails and contains needs trusted ancestry
Priority: Must
Given the contract `{"required_actions":["push"],"ref":"feature/x","expected_commit":"<B>"}` with `B` local
When `origin feature/x` is `U` and `U` is present locally
Then `push` is `unsatisfied` with `observed` `U` under `exact` and under `contains`
And with a `git replace --graft U B` ref in the repository, `contains` stays `unsatisfied`
And with an `info/grafts` line making `U` a child of `B`, `contains` is `unknown` with detail containing `graft`
When `origin feature/x` is `C`, a child of `B` pushed from another clone and absent locally
Then `contains` is `unknown` with detail containing `not available locally`
When `origin feature/x` is `D`, a child of `B` present locally
Then `contains` is `satisfied` and `exact` is `unsatisfied`
And with `D` listed in the repository's `shallow` file, `contains` with expected `B` is `unknown` with detail containing `shallow`
When the work repository is a blob-less partial clone whose promisor is `origin`, and `origin feature/x` moved to `C`, which the clone lacks
Then `contains` is `unknown` with detail containing `not available locally`, the clone's `.git` digest is unchanged, and a plain `git cat-file -e <C>` run afterwards fetches `C`.

### S6: AC-WPOST-006 - Commit checks follow HEAD
Priority: Must
Given the base fixture and the contract level below
When the change below happens after the baseline
Then `commit` has the expected value below as its status, and its detail contains the listed text

| level | change | expected status | detail contains |
|---|---|---|---|
| required | none | unsatisfied | `HEAD did not advance after execution` |
| required | one new commit `B` | satisfied | `B` |
| required | `git reset --hard U` | unsatisfied | `does not descend from the baseline` |
| required | work dir with no commit at the baseline | unknown | `no baseline commit` |
| forbidden | none | satisfied | `HEAD unchanged` |
| forbidden | one new commit `B` | unsatisfied | `HEAD advanced` |
| forbidden | `git reset --hard U` | unknown | `does not descend from the baseline` |
| forbidden | plain directory before and after | satisfied | `no commit` |
| forbidden | configuration damaged during baseline capture | unknown | `baseline HEAD could not be read` |
| forbidden | configuration damaged during verification | unknown | `cannot read HEAD` |

And with worktree isolation and one queue slot held, a task with the prompt `commit your work` that commits nothing while the main checkout gains a commit during its wait fails with `commit: HEAD did not advance after execution`, where `109c7ea4` returned no error

### S7: AC-WPOST-007 - Branch checks and the forbidden push
Priority: Must
Given the base fixture
When the contract and state below are judged
Then the named check has the expected value below as its status

| contract | state after execution | check | expected status |
|---|---|---|---|
| required `branch`, ref `feature/x` | local `feature/x` exists | branch | satisfied |
| required `branch`, ref `feature/x` | no local `feature/x` | branch | unsatisfied |
| required `branch`, ref `feature/x`, expected `A0` | `feature/x` at `B` | branch | unsatisfied |
| required `branch`, no ref | any | branch | unknown |
| forbidden `branch`, ref `feature/new` | `feature/new` created | branch | unsatisfied |
| forbidden `branch`, ref `main` | `main` existed at the baseline | branch | satisfied |
| forbidden `push` | HEAD unchanged | push | satisfied, no `ls-remote` run |
| forbidden `push` | new commit `B`, not pushed | push | satisfied |
| forbidden `push` | `B` pushed as `refs/heads/sneaky` | push | unsatisfied, detail names `refs/heads/sneaky` |
| forbidden `push` | `main` fast-forwarded to `P`, which `origin side` held at the baseline | push | satisfied |
| forbidden `push` | new commit, pinned URL not a repository | push | unknown |
| forbidden `push` | `B` pushed as `refs/heads/sneaky`, no head snapshot | push | unknown |
| forbidden `push` | no `origin` before or after | push | satisfied |
| forbidden `push` | `origin` added during execution | push | unknown |
| forbidden `push` | `origin` added during execution, HEAD unchanged | push | unknown |
| forbidden `push`, unborn repository without remote | first commit made | push | satisfied |
| forbidden `push` | HEAD reset to `U`, `U` pushed as `refs/heads/sneaky`, commit-graph rewritten so `A0` claims `U` as parent | push | unsatisfied |
| forbidden `push` | the same with an `info/grafts` line `A0 U` instead | push | unknown |
| forbidden `branch`, ref `feature/new` | created, baseline lookup failed | branch | unknown |
| required `branch`, ref `main` | configuration damaged during verification | branch | unknown |

### S8: AC-WPOST-008 - Fallback inference never widens and never claims success without evidence
Priority: Must
Given a payload without a contract
When the prompt below is planned
Then the levels are exactly the expected values below

| prompt | commit | push | branch |
|---|---|---|---|
| `Review only; do not commit or push` | none | none | none |
| `Commit your changes but do not push` | required | none | none |
| `Don't forget to commit and push` | required | required | none |
| `Never git push. Commit the fix` | required | none | none |
| `Commit the fix. Do not commit generated files` | ambiguous | none | none |
| `Improve branch coverage` | none | none | none |
| `create branch feature/login` | none | none | required |
| `Do not create a branch named release-2` | none | none | none |
| `Pushing to main is not allowed` | none | none | none |
| `commit 하지 마세요` | none | none | none |
| `commit_message cleanup and pushed state` | none | none | none |

And an `ambiguous` commit with no new commit is `unknown` and the error starts `postcondition unverified: `, while with a new commit it is `satisfied`
And a required fallback push on a `worker-` branch with no extracted name is `unknown`
And a prompt with no mention, and a plain directory with the prompt `commit and push`, return no artifact and no error.

### S9: AC-WPOST-009 - The contract is validated before any provider runs
Priority: Must
Given a `handleTask` payload with the contract below and a mock provider
When the task is handled
Then the task fails with an error starting `invalid completion_contract: ` and containing the listed text, and the provider is never called

| contract | error contains |
|---|---|
| `{"version":2}` | `unsupported version 2` |
| `{"required_actions":["deploy"]}` | `unsupported action "deploy"` |
| `{"required_actions":["push"],"forbidden_actions":["push"]}` | `"push" is both required and forbidden` |
| `{"ref":"refs/tags/v1"}` | `is not under refs/heads/` |
| `{"ref":"feature/../x"}` | `invalid branch name` |
| `{"remote":"-oProxyCommand=x"}` | `invalid remote` |
| `{"expected_commit":"abc123"}` | `expected_commit must be a full` |
| `{"push_match":"prefix"}` | `unsupported push_match "prefix"` |
| `null` | `completion_contract must be an object, got null` |
| `[]` | `completion_contract must be an object` |
| `{"version":"1"}` | `version must be an integer` |
| `{"forbidden_actions":["commit",3]}` | `forbidden_actions[1] must be a string` |
| `{"ref":null}` | `ref must be a string` |
| `{"requried_actions":["push"]}` | `unknown key "requried_actions"` |
| `{"ref":"feature.lock/x"}` | `invalid branch name` |

And `{"version":"1","required_actions":"push","ref":"a..b","bogus":1,"push_match":"prefix"}` gives one error naming all five problems
And `{"remote":"https://user:secret@host/repo.git"}` gives an error, a task result, and host events without `secret` or `user:`, and with `<redacted>@`
And `{"version":1,"required_actions":["Commit"," push "],"ref":"feature/x"}` plans commit and push `required`, branch `none`, ref `refs/heads/feature/x`, remote `origin`, match `exact`, and `ref` `refs/heads/기능+fix` is accepted
And `{"required_actions":[]}` with the prompt `commit and push` plans all three actions `none`, while a payload without the key plans `commit` `required`
And for each of 35 names (among them `feature+fix`, `기능/한글`, `@`, `x/y.lock`, `a./b`, `-dash`, `HEAD`), the branch-name rule accepts exactly the names `git check-ref-format --branch` accepts.

### S10: AC-WPOST-010 - Verification never writes
Priority: Must
Given a recording runner around the verifier over the S1 to S8 fixtures
When baseline capture and verification run
Then every recorded invocation starts with `--no-replace-objects -c core.commitGraph=false` followed by `rev-parse`, `symbolic-ref`, `remote get-url`, `for-each-ref`, `ls-remote`, `merge-base --is-ancestor`, `cat-file -e`, or `rev-list`
And `commit`, `push`, `fetch`, `update-ref`, `config`, `remote add`, `merge-base` without `--is-ancestor`, and `rev-list --output=<file>` are refused before git starts
And after failing checks in three scenarios, a digest of every file (path, mode, size, content) under the work repository's `.git` and under `origin` is the same as before verification, so no ref, configuration, or object was written.

### S11: AC-WPOST-011 - Report shape and redaction
Priority: Must
Given the S2 `unsatisfied` push
When the report is decoded
Then it has `version` 2, `active` true, `source` `contract`, `outcome` `unsatisfied`, and each check has non-empty `name`, `requirement`, `status`, and `detail`
And with a pinned URL `http://user:secret-token@127.0.0.1:<port>/r.git` on a server answering 401, neither the artifact nor the error contains `secret-token` or `user:`
And `redactGitMessage("fatal: unable to access 'https://u:p@h/r.git/': x")` returns `fatal: unable to access 'https://<redacted>@h/r.git/': x`
And a required commit that did not happen together with a push whose lookup failed gives `outcome` `unsatisfied` and an error starting `postcondition failed: commit: HEAD did not advance after execution; ` and containing `push (unverified): remote lookup failed`.

### S12: AC-WPOST-012 - Scope and documentation
Priority: Must
Given the branch diff against `origin/main`
When its paths are listed
Then every path is under `pkg/worker/`, `docs/worker-completion-contract.md`, or `.autopus/specs/SPEC-WORKERPOST-001/`
And `docs/worker-completion-contract.md` names every contract key, the four states, the outcome rule, the fallback cue list, and a read-only and a commit-and-push example.

## Oracle Acceptance Notes

Every Must scenario pairs a concrete fixture with concrete expected values: check statuses, `expected` and `observed` SHAs, exact error prefixes, detail substrings, planned levels, and byte-identical digests of the git directories. None passes on a bare "no error" or file-exists check.

- S1 and S2 are the two reproductions; each records the result on `109c7ea4` (INV-P01, INV-P02). S3 to S5 fix the push target, the pinned relative URL, and the ancestry boundaries (INV-P04, INV-P09); S6 and S7 are rows of the decision tables, including failed lookups and overlapping conditions (INV-P01, INV-P09); S8 is a level table plus the ambiguous and no-report rules (INV-P02, INV-P06); S9 fixes the rejection text, the aggregation, the redaction, the branch-name rule against git, and that the provider is never called (INV-P05, INV-P07); S10 fixes the command allowlist, the unchanged git directories, and the blocked lazy fetch (INV-P03); S11 fixes the JSON keys, the mixed outcome, and the absence of a secret (INV-P07); S12 fixes the path set (INV-P08).
- The commit-graph and lazy-fetch fixtures also run plain git against the same files to show that the fixture is effective: plain git hides the gained commit and fetches the missing tip.
- No tolerance applies: every expected value is exact.

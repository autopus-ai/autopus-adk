# Worker Completion Contract

The retained worker (`auto worker`, `pkg/worker`) decides whether a task that ran successfully is also complete by checking git postconditions after the provider exits. A task payload can state those postconditions explicitly in `completion_contract`. Without it, the worker falls back to reading the prompt. Source: SPEC-WORKERPOST-001.

## Contract

`completion_contract` is an optional object in the task payload, next to `prompt`:

| Key | Type | Default | Meaning |
|---|---|---|---|
| `version` | integer | `1` | Contract version. Absent or `0` reads as `1`; any other value rejects the task. |
| `required_actions` | array | `[]` | Actions the task must leave evidence of: `commit`, `push`, `branch`. |
| `forbidden_actions` | array | `[]` | Actions the task must not perform. |
| `remote` | string | `origin` | Configured remote name, matching `^[A-Za-z0-9][A-Za-z0-9._-]*$`. |
| `ref` | string | none | Target branch, as `refs/heads/<name>` or `<name>`, valid for `git check-ref-format --branch` (non-ASCII and `+` included). Other namespaces are rejected. |
| `expected_commit` | string | HEAD | Full 40 or 64 character hex SHA the target must hold. |
| `push_match` | string | `exact` | `exact`: the remote ref's SHA equals the expected commit. `contains`: the remote tip equals it or descends from it. |

Action names are matched case-insensitively. An action in neither list is `not_required`. When the key is present, the prompt is not read at all.

The contract grants nothing. It states what the check demands; the worker never commits, pushes, or fetches to satisfy it, and the task's permissions still come from its signed security policy.

Only a payload without the `completion_contract` key uses the fallback. A present key whose value is `null` or not an object, an unknown key (a misspelt one would otherwise drop a requirement), a field of another type or `null`, an unknown version or action, an action in both lists, or a malformed `remote`, `ref`, `expected_commit`, or `push_match` makes the contract invalid. An invalid contract fails the task before any provider runs, with one error starting `invalid completion_contract: ` that names every problem; sender values in it have URL credentials replaced with `<redacted>` before they are quoted.

Workers older than this contract ignore the key and keep their prompt inference.

## States And Outcome

Every check in `postconditions.json` (report `version` 2) has a `requirement` (`required`, `forbidden`, `none`, or, in fallback mode, `ambiguous`) and a `status`:

| Status | Meaning |
|---|---|
| `satisfied` | Git evidence shows the requirement holds. |
| `unsatisfied` | Git evidence shows it does not. |
| `not_required` | The task neither requires nor forbids the action. |
| `unknown` | The worker could not decide: a lookup failed, the target is unclear, or the evidence is not trustworthy. |

Version 1 reports, written before the contract, used `passed` and `failed`. The report `outcome` is the first present of `unsatisfied`, `unknown`, `satisfied`, then `not_required`. Only `satisfied` and `not_required` complete the task. `unsatisfied` fails it with `postcondition failed: ...`; `unknown` fails it with `postcondition unverified: ...`. An unverifiable result is never reported as success.

## Checks

The baseline is captured in the task's work directory right before the provider runs: HEAD's commit, the fetch URL of the contract's remote (`git remote get-url`; a relative local path is made absolute against the work tree, as git resolves it), whether the local branch of `ref` exists, and, when push is forbidden, the remote's branch heads.

Every lookup ends present, absent, or failed. Absent is only git's own not-found answer or a directory that is not a repository; a lookup that failed (a damaged configuration, a timeout) is never read as absent, so it can only lead to `unknown`. Each check walks an ordered decision table (SPEC-WORKERPOST-001, Decision Tables) and the first matching row decides, so overlapping conditions have one result.

- `commit` required: `satisfied` when HEAD moved to a commit that descends from the baseline; `unsatisfied` when HEAD did not move or moved to a commit that does not descend from it; `unknown` without a baseline commit or with a failed lookup.
- `commit` forbidden: `satisfied` while HEAD stays at the baseline; `unsatisfied` when HEAD advanced (any forward move counts, including a fast-forward pull); `unknown` when HEAD moved sideways.
- `push` required: the worker runs `git ls-remote` against the URL recorded at the baseline and reads the line whose ref name equals `ref` exactly. The existence of the branch is never evidence; only its SHA is. Under `contains` the remote tip must already be in the local object store; it is never fetched. `unknown` covers: no `ref`, a remote without a URL at the baseline, a detached HEAD without `expected_commit`, any lookup failure (network, authentication, timeout), and a `contains` comparison that lacks trustworthy local ancestry.
- `push` forbidden: a remote that appears during execution is `unknown`; a remote absent before and after is `satisfied`; otherwise `satisfied` while HEAD stays at the baseline, and `unsatisfied` when a remote branch moved during execution to a commit HEAD gained. A branch that already pointed there at the baseline was pulled, not pushed. Without a baseline snapshot of the remote heads, or in a shallow repository, the move is `unknown`.
- `branch` required: the local branch of `ref` exists and, with `expected_commit`, points at it. `branch` forbidden: that branch did not appear during execution. Both need a `ref`, and the branch is looked up by its exact name (`for-each-ref`), never through git's revision shorthand.

Ancestry comes only from commit objects in the local store: every command runs with `--no-replace-objects` and `-c core.commitGraph=false`, so neither replace refs nor a rewritten commit-graph can invent a parent. A graft file cannot be switched off, so while one exists every ancestry answer is `unknown`; in a shallow repository a negative answer is `unknown`. Lazy fetch is off (`GIT_NO_LAZY_FETCH=1`, and `GIT_ALLOW_PROTOCOL=none` for every command except the remote lookup), so a partial clone's promisor remote is never contacted.

The lookup runs in an empty temporary directory, so repository configuration the task may have written (a new remote URL, an `insteadOf` rewrite, a credential helper, `core.sshCommand`) cannot redirect it. The report names remotes, never URLs, and git messages in it have URL credentials, with or without a scheme, replaced with `<redacted>`.

Verification runs only fixed forms of `rev-parse`, `symbolic-ref -q --short HEAD`, `remote get-url`, `for-each-ref`, `ls-remote`, `merge-base --is-ancestor`, `cat-file -e`, and `rev-list`, with `LC_ALL=C` and `GIT_OPTIONAL_LOCKS=0`, and refuses any other command, and any operand that starts with `-`, before a process starts. It writes no ref, configuration, or object. A local read times out after 30 seconds and a remote lookup after 15; a timeout is a failed lookup.

## Examples

A read-only review:

```json
{
  "prompt": "Review the change and report findings.",
  "completion_contract": {"version": 1, "forbidden_actions": ["commit", "push"]}
}
```

The task completes when HEAD is unchanged; no remote lookup is needed for that.

A change that must land on a branch:

```json
{
  "prompt": "Fix the parser and publish the branch.",
  "completion_contract": {
    "version": 1,
    "required_actions": ["commit", "push"],
    "remote": "origin",
    "ref": "refs/heads/fix/parser",
    "push_match": "exact"
  }
}
```

The task completes when HEAD has new commits on top of the baseline and `origin` holds `refs/heads/fix/parser` at exactly that HEAD. A task that ends on another branch than the one it pushed should be given `expected_commit`.

## Fallback Mode

When the payload has no `completion_contract`, the worker infers requirements from the prompt. The inference keeps the words the earlier worker matched (`commit`, `push`, `branch`) and removes the ones the prompt negates, so it never requires more than before:

- The prompt is split into clauses at `.`, `;`, `!`, `?`, and line breaks.
- A mention is negated when, in the same clause, a cue precedes it with only filler words in between, or a prohibition follows it within four words. Cues: `not`, `no`, `never`, `without`, `nor`, `don't`, `dont`, `doesn't`, `didn't`, `mustn't`, `shouldn't`, `won't`, `can't`, `cannot`, `avoid`, `avoiding`, `refrain`, `skip`, `forbid`, `prohibit`. Prohibitions: `forbidden`, `prohibited`, `disallowed`, `banned`, `not allowed`, `not permitted`, `not needed`, `not required`, `not necessary`, `금지`, `하지 마`, `하지 말`, `하지 않`. Fillers include articles, `git`, `to`, `be`, `any`, `new`, `create`, `make`, `run`, `do`, `need`, `force`, `from`, `or`, `and`, `,`, and `/`, so `do not commit or push` negates both.
- An action asked for and never negated is `required`; negated and never asked for, `not_required`; both, `ambiguous`. An ambiguous check is `satisfied` with evidence and `unknown` without it.
- `branch` counts only when the prompt names a branch (`refs/heads/<name>`, `branch [named|called] <name>`, `origin/<name>`, `origin <name>`).
- A push targets the current branch, unless it is detached or a `worker-` branch, or the single branch the prompt names; otherwise it is `unknown`. The expected commit is HEAD; the remote is `origin`.
- Negations never become forbidden checks. A prompt that mentions no action, or a work directory without a commit at the baseline, produces no report.

Limits: conditional instructions (`if tests fail, do not push`) read as negations, and noun phrases (`push notifications`) still read as requests. A prompt that names several branches while HEAD is on none of them gives an `unknown` push; before the contract it passed whenever every named branch existed on `origin`, which proved nothing about the pushed commit. Send a contract when the result matters.

# Eval Regression Gate - Required-Check Readiness Runbook

Ops runbook for promoting the eval-regression gate to a required status check on
branch `main`. The live gate is owned by the sibling **Autopus repo**
(`github.com/autopus-ai/Autopus`); this `autopus-adk` repo owns the verifier CLI,
the committed public-key allowlist, and this runbook.

Ref: SPEC-EVAL-GATE-LIVE-001 (REQ-EGL-RUNBOOK-001), extending
SPEC-EVAL-REGRESSION-PROV-001 and SPEC-EVAL-REGRESSION-CI-001.

> IMPORTANT: This document performs no privileged action. Branch protection,
> repository secrets, CODEOWNERS, and org ruleset changes are OPS-ONLY actions
> that an operator performs manually after the readiness checks below pass.

## Gate check context name

The live workflow is expected at:

```text
Autopus/.github/workflows/eval-regression-gate.yml
```

The workflow defines the gate job as `eval-regression`; GitHub renders the check
context from the job id/name. Confirm the exact rendered context on a recent
Autopus repo commit before registering it:

```bash
# Confirm the rendered check context name on a real Autopus commit SHA.
gh api repos/autopus-ai/Autopus/commits/<sha>/check-runs \
  --jq '.check_runs[].name'
```

Use the printed value for the eval-regression job as the authoritative context
string in branch protection. It is expected to be `eval-regression`, but the
rendered `check-runs` response is the source of truth.

## Trusted fetch dry-run oracle

The Autopus gate must fetch only the trusted producer workflow's successful
`pull_request_target` run for the current pull-request number and head SHA.
During workflow review, confirm the selection pins workflow identity, event,
exact display title, and success status. `--commit` is intentionally not used:
the trusted-base workflow run is associated with the base commit, not PR-head
code.

```bash
# Review oracle only; do not run from this adk test suite.
expected="Eval Regression Producer PR #<pr_number> head <head_sha>"
runs_json="$(
  gh run list \
    --workflow eval-regression-producer.yml \
    --event pull_request_target \
    --status success \
    --limit 100 \
    --json databaseId,displayTitle,createdAt
)"
selected_run_id="$(
  jq -r --arg expected "$expected" \
    '[.[] | select(.displayTitle == $expected)] | sort_by(.createdAt) | reverse | .[0].databaseId // empty' \
    <<<"$runs_json"
)"

test -n "$selected_run_id" || {
  echo "artifact_missing"
  exit 1
}

gh run download <selected_run_id> \
  --name eval-regression-report-pr-<pr_number>-<head_sha> \
  --dir .autopus/artifacts
```

The same-SHA replay defense depends on this exact selection shape:
`--workflow eval-regression-producer.yml`, `--event pull_request_target`,
`--status success`, and the exact display title
`Eval Regression Producer PR #<pr_number> head <head_sha>`. A run from another
workflow, event, PR, or head SHA must never be selected, even if it uploads a
similarly named artifact. If no exact trusted producer run exists, the gate
fails closed with `artifact_missing`.

## Operator provisioning checklist

Complete these steps outside this repository and outside untrusted PR code:

1. Generate the ed25519 signing key securely outside the repo, preferably in a
   secret manager or controlled ops workstation. Never commit the private key,
   generated seed, shell history, or derived secret material.
2. Configure the Autopus repo producer workflow with the signing secret
   `EVAL_REGRESSION_SIGNING_KEY` and protected Environment variable
   `EVAL_REGRESSION_SIGNING_KEY_ID`.
3. Configure the producer workflow's DB/network credentials in the Autopus repo
   secrets or environment protection layer. The producer needs only the minimum
   database and network access required to read the live eval-regression verdict
   source.
4. Commit only the public key to the `autopus-adk` allowlist in
   `pkg/evalregression/attestation.go`, keyed by the exact
   `EVAL_REGRESSION_SIGNING_KEY_ID` `key_id`.
5. For key rotation, use a rotation overlap: keep both old and new public keys
   in the adk allowlist while both signing keys may produce artifacts, then
   remove the old public key after the producer has fully cut over.
6. Protect Autopus repo workflow definitions before the check becomes required:
   add CODEOWNERS coverage or an org ruleset for `Autopus/.github/workflows/**`,
   including `eval-regression-producer.yml` and `eval-regression-gate.yml`.
7. Confirm the rendered `eval-regression` check context through the `check-runs`
   API on a real Autopus commit, then register that exact context in Autopus repo
   branch protection.
8. Run a known-good control PR and a blocked control PR. The good control must
   pass with reason `ok`; the blocked control must fail with reason
   `regression_blocked`.

## Branch protection registration

After the checklist passes, add the rendered `eval-regression` context to branch
`main` in the Autopus repo required status checks:

```bash
# OPS-ONLY. Requires an admin token. Not run by CI or this SPEC.
gh api repos/autopus-ai/Autopus/branches/main/protection \
  --method PUT \
  --field required_status_checks[strict]=true \
  --field required_status_checks[contexts][]='eval-regression'
```

Or via the GitHub UI in the Autopus repo: **Settings -> Branches -> Branch
protection rules -> `main` -> Require status checks to pass before merging ->
add `eval-regression`**.

## Readiness precondition

Do not promote until all of the following hold:

1. LIVE-A emits a real `eval_regression_report.v1` report and a valid
   `eval_regression_attestation.v2` sidecar from the trusted producer workflow.
2. The report is signed with the key whose public half is committed to the adk
   allowlist under the matching `key_id`.
3. The Autopus gate downloads the trusted producer's successful head-SHA run
   artifact and runs `auto check --eval-regression` unconditionally.
4. The adk workflow `.github/workflows/eval-regression-gate.yml` is absent, so
   there is no dormant duplicate gate in this verifier repository.

Until the production public key is committed and the Autopus repo secrets are
configured, the gate is expected to fail closed on `artifact_missing`,
`artifact_unsigned`, or `signature_key_unknown`. That is the intended safe
default, not an incomplete implementation.

## Harness eval lane (SPEC-HARNEVAL-003)

The ADK harness golden set has its own signing lane, separate from the Autopus
staging-to-main lane above. Its evidence is produced by
`.github/workflows/harness-eval-live.yml` on `main` and verified by the same
strict `auto check --eval-regression` path with these six expected values:

| Flag | Harness lane value |
|------|--------------------|
| `--eval-regression-expected-key-id` | `adk-harness-eval-2026-10` (`evalregression.ADKHarnessEvalKeyID`) |
| `--eval-regression-expected-trust-lane` | `adk-harness-eval` (`evalregression.ADKHarnessEvalTrustLane`) |
| `--eval-regression-expected-source-environment` | `adk-harness-live` |
| `--eval-regression-expected-target-environment` | `adk-release` |
| `--eval-regression-expected-source-revision` | the binding digest B of the release source |
| `--eval-regression-expected-workspace-scope` | `autopus-adk` |

`auto eval harness digest --binding --format json` prints B, and
`auto eval harness policy --binding B` prints the six values. The two lanes
reject each other with `attestation_policy_mismatch` because their key ids
differ; there is no unsigned-accept path in either lane. The only harness lane
entry in `evalRegressionPublicKeys` is its public key; the private key lives
only in the `adk-harness-eval-signing` Environment secret
`HARNESS_EVAL_SIGNING_KEY`, which reaches `auto eval harness export` on stdin.

Environments (OPS-ONLY): `adk-harness-eval-agent` holds only the Codex
credential and `adk-harness-eval-signing` holds only the signing key. Both
limit deployment branches to `main`; `adk-harness-eval-signing` requires at
least one reviewer and forbids self-review.

### Harness lane key rotation

The key id is part of the binding (`signing_key_id`), so a rotation changes B:
sessions signed before the rotation never enter the new binding's window and
cannot block or unblock a release. Rotate in this order:

1. Stop dispatching `harness-eval-live.yml`.
2. Cancel every run that is in progress or waiting for Environment approval:
   `gh run list --workflow harness-eval-live.yml --status in_progress` and
   `--status waiting`, then `gh run cancel <run_id>` for each.
3. Apply together, in one change: add the new public key to
   `evalRegressionPublicKeys`, replace the `ADKHarnessEvalKeyID` constant, and
   replace the `HARNESS_EVAL_SIGNING_KEY` Environment secret.
4. Resume dispatching `harness-eval-live.yml`.
5. Produce a new session under the new binding before the next release.

Every export verifies its own output against the committed allowlist before
upload; evidence signed with a key the allowlist does not hold under
`ADKHarnessEvalKeyID` stops as `self_verify_failed` and is never uploaded.

## Non-goals and safety

- This document does not run `gh api`, write secrets, alter branch protection, or
  create CODEOWNERS/org ruleset entries.
- No secret values are inlined. Secret names are listed only so an operator can
  wire the producer consistently.
- Reversal: remove `eval-regression` from
  `required_status_checks[contexts][]` in the Autopus repo branch protection if
  the gate needs to be de-listed.

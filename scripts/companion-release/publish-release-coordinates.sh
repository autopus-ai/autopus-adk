#!/usr/bin/env bash
set -euo pipefail
umask 077
fail() { printf 'release coordinate publish: %s\n' "$1" >&2; exit 1; }
usage() {
  printf '%s\n' 'usage: publish-release-coordinates.sh REPOSITORY ENVIRONMENT RELEASE_TAG SOURCE_COMMIT SOURCE_TREE STATIC_POLICY_FILE EVIDENCE_TAG_OBJECT EVIDENCE_COMMIT EVIDENCE_TREE REPORT_SHA256 ATTESTATION_SHA256 fresh:COMMIT|retained:COMMIT|reconcile TAG_SIGNING_KEY' >&2
  exit 64
}
[[ $# -eq 13 ]] || usage
readonly repository=$1 environment_name=$2 release_tag=$3 source_commit=$4 source_tree=$5
readonly static_policy_file=$6 evidence_tag_object=$7 evidence_commit=$8 evidence_tree=$9
shift 9
readonly report_sha256=$1 attestation_sha256=$2 prep_lock_argument=$3 tag_signing_key=$4
readonly release_mode='canonical' promotion_key_id='omp-context-promotion-2026-q3-k3'
prep_lock_commit='' transaction_kind=''
case "$prep_lock_argument" in
  reconcile) transaction_kind='reconcile' ;;
  fresh:*) transaction_kind='fresh'; prep_lock_commit=${prep_lock_argument#fresh:} ;;
  retained:*) transaction_kind='retained'; prep_lock_commit=${prep_lock_argument#retained:} ;;
  *) usage ;;
esac
readonly prep_lock_commit transaction_kind
readonly evidence_tag="omp-context-evidence-${release_tag}" release_ref="refs/tags/${release_tag}"
readonly evidence_ref="refs/tags/${evidence_tag}" prep_lock_ref="refs/heads/${evidence_tag}-source"
readonly hex40='^[0-9a-f]{40}$' hex64='^[0-9a-f]{64}$'
[[ "$repository" == 'autopus-ai/autopus-adk' ]] || fail 'repository is not production authority'
[[ "$environment_name" == 'adk-companion-release' ]] || fail 'environment is not protected release authority'
[[ "$release_tag" == 'v0.50.118' ]] || fail 'release tag is not exact A29'
for value in "$source_commit" "$source_tree" "$evidence_tag_object" "$evidence_commit" "$evidence_tree"; do
  [[ "$value" =~ $hex40 ]] || fail 'Git coordinate is malformed'
done
[[ "$transaction_kind" == 'reconcile' || "$prep_lock_commit" =~ $hex40 ]] || fail 'prep lock coordinate is malformed'
for value in "$report_sha256" "$attestation_sha256"; do [[ "$value" =~ $hex64 ]] || fail 'evidence digest is malformed'; done
[[ -f "$static_policy_file" && ! -L "$static_policy_file" ]] || fail 'static policy file is unsafe'
IFS= read -r static_policy_b64 <"$static_policy_file" || fail 'read static policy'
[[ "$static_policy_b64" =~ ^[A-Za-z0-9_-]+$ && ${#static_policy_b64} -le 21846 ]] || fail 'static policy is malformed'
[[ "$(wc -l <"$static_policy_file" | tr -d ' ')" == '1' ]] || fail 'static policy file is not canonical'
policy_base64=$(printf '%s' "$static_policy_b64" | tr '_-' '/+')
case $((${#policy_base64} % 4)) in 0) ;; 2) policy_base64+='==' ;; 3) policy_base64+='=' ;; *) fail 'static policy base64url length is invalid' ;; esac
policy_signing_key_id=$(jq -Rer '@base64d | fromjson | .promotion_signing_key_id' <<<"$policy_base64") || fail 'static policy cannot be decoded'
[[ "$policy_signing_key_id" == "$promotion_key_id" ]] || fail 'static policy does not own the exact K3 signer'
[[ -f "$tag_signing_key" && ! -L "$tag_signing_key" ]] || fail 'release tag signing key is unsafe'
for tool in awk cmp gh git jq mktemp shasum ssh-keygen stat tr uname wc; do command -v "$tool" >/dev/null || fail "$tool is unavailable"; done
case "$(uname -s)" in
  Darwin) key_owner_mode=$(/usr/bin/stat -f '%u:%Lp' "$tag_signing_key") ;;
  Linux) key_owner_mode=$(stat -c '%u:%a' "$tag_signing_key") ;;
  *) fail 'release tag signing platform is unsupported' ;;
esac
[[ "$key_owner_mode" == "$(id -u):600" ]] || fail 'release tag signing key ownership or mode is unsafe'
repo_root=$(git rev-parse --show-toplevel)
[[ "$(pwd -P)" == "$repo_root" ]] || fail 'publisher must run at the repository root'
readonly repo_root
[[ "$(git rev-parse --verify 'HEAD^{commit}')" == "$source_commit" ]] || fail 'HEAD differs from source commit'
[[ "$(git rev-parse --verify 'HEAD^{tree}')" == "$source_tree" ]] || fail 'HEAD differs from source tree'
[[ -z "$(git status --porcelain)" ]] || fail 'source worktree is not clean'
git fetch --no-tags origin main
[[ "$(git rev-parse --verify origin/main)" == "$source_commit" ]] || fail 'source is not exact origin/main'
remote_evidence=$(git ls-remote --refs origin "$evidence_ref") || fail 'cannot inspect remote evidence tag'
[[ "$remote_evidence" == "$evidence_tag_object"$'\t'"$evidence_ref" ]] || fail 'remote evidence tag differs'
git fetch --no-tags origin "$evidence_ref" || fail 'cannot fetch remote evidence tag'
[[ "$(git cat-file -t "$evidence_tag_object")" == 'tag' ]] || fail 'fetched evidence tag is not annotated'
[[ "$(git rev-parse --verify "${evidence_tag_object}^{commit}")" == "$evidence_commit" ]] || fail 'fetched evidence commit differs'
[[ "$(git rev-parse --verify "${evidence_tag_object}^{tree}")" == "$evidence_tree" ]] || fail 'fetched evidence tree differs'
[[ "$(git rev-list --parents -n 1 "$evidence_commit")" == "$evidence_commit" ]] || fail 'evidence commit is not orphaned'
expected_evidence_names=$'omp-context-promotion-attestation.v2.json\nomp-context-promotion-report.v1.json'
[[ "$(git ls-tree -r --name-only "$evidence_commit")" == "$expected_evidence_names" ]] || fail 'evidence tree topology differs'
[[ "$(git cat-file blob "${evidence_commit}:omp-context-promotion-report.v1.json" | shasum -a 256 | awk '{print $1}')" == "$report_sha256" ]] || fail 'evidence report digest differs'
[[ "$(git cat-file blob "${evidence_commit}:omp-context-promotion-attestation.v2.json" | shasum -a 256 | awk '{print $1}')" == "$attestation_sha256" ]] || fail 'evidence attestation digest differs'
bootstrap_cleanup() { rm -rf -- "$temp_dir"; }
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/release-coordinate-publish.XXXXXX"); readonly temp_dir
chmod 0700 "$temp_dir"; trap bootstrap_cleanup EXIT
r2_public="$repo_root/scripts/companion-release/release-tag-signing-2026-q3-r2.pub"
r2_fingerprint="$repo_root/scripts/companion-release/release-tag-signing-2026-q3-r2.fingerprint"
[[ -f "$r2_public" && ! -L "$r2_public" && -f "$r2_fingerprint" && ! -L "$r2_fingerprint" ]] || fail 'R2 release tag signer pins are missing or unsafe'
derived_public="$temp_dir/release-tag-signing.pub"; allowed_signers="$temp_dir/release-tag.allowed-signers"
ssh-keygen -y -f "$tag_signing_key" >"$derived_public"
expected_public=$(awk 'NF >= 2 { print $1 " " $2; exit }' "$r2_public")
derived_public_value=$(awk 'NF >= 2 { print $1 " " $2; exit }' "$derived_public")
expected_fingerprint=$(<"$r2_fingerprint")
[[ -n "$expected_public" && "$derived_public_value" == "$expected_public" ]] || fail 'tag private key differs from R2 public pin'
[[ "$expected_fingerprint" == 'SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ' &&
   "$(ssh-keygen -lf "$derived_public" -E sha256 | awk '{print $2}')" == "$expected_fingerprint" ]] || fail 'tag private key differs from R2 fingerprint'
printf 'autopus-adk-release-tag %s\n' "$derived_public_value" >"$allowed_signers"; chmod 0600 "$derived_public" "$allowed_signers"
tag_git_config=(GIT_CONFIG_COUNT=5 GIT_CONFIG_KEY_0=gpg.format GIT_CONFIG_VALUE_0=ssh
  GIT_CONFIG_KEY_1=user.signingkey GIT_CONFIG_VALUE_1="$tag_signing_key"
  GIT_CONFIG_KEY_2=gpg.ssh.allowedSignersFile GIT_CONFIG_VALUE_2="$allowed_signers"
  GIT_CONFIG_KEY_3=user.name GIT_CONFIG_VALUE_3='Joseph'
  GIT_CONFIG_KEY_4=user.email GIT_CONFIG_VALUE_4='joseph@Josephui-MacBookPro.local')
static_policy_sha256=$(printf '%s' "$static_policy_b64" | shasum -a 256 | awk '{print $1}')
[[ "$static_policy_sha256" =~ $hex64 ]] || fail 'static policy digest is malformed'
readonly reservation_name="$release_tag"
reservation_body=$(jq -cnS --arg schema 'autopus.adk_release_reservation.v1' --arg release_tag "$release_tag" \
  --arg source_commit "$source_commit" --arg source_tree "$source_tree" --arg evidence_tag_object "$evidence_tag_object" \
  --arg evidence_commit "$evidence_commit" --arg evidence_tree "$evidence_tree" --arg report_sha256 "$report_sha256" \
  --arg attestation_sha256 "$attestation_sha256" --arg static_policy_sha256 "$static_policy_sha256" \
  '{schema_version:$schema,release_tag:$release_tag,source_commit:$source_commit,source_tree:$source_tree,
    evidence_tag_object:$evidence_tag_object,evidence_commit:$evidence_commit,evidence_tree:$evidence_tree,
    report_sha256:$report_sha256,attestation_sha256:$attestation_sha256,static_policy_sha256:$static_policy_sha256}')
readonly static_policy_sha256 reservation_body
transaction_lib="$temp_dir/release-coordinate-transaction-lib.sh"
transaction_blob=$(git rev-parse --verify "${source_commit}:scripts/companion-release/release-coordinate-transaction-lib.sh") || fail 'release coordinate transaction helper is absent from exact source'
[[ "$(git cat-file -t "$transaction_blob")" == 'blob' ]] || fail 'release coordinate transaction helper is not a source blob'
git cat-file blob "$transaction_blob" >"$transaction_lib"; chmod 0400 "$transaction_lib"
[[ "$(git hash-object "$transaction_lib")" == "$transaction_blob" ]] || fail 'staged release coordinate transaction helper differs from exact source'
exec 9<"$transaction_lib"; rm -f -- "$transaction_lib"
# shellcheck source=/dev/null
source /dev/fd/9
exec 9<&-
seal_release_tag_ruleset() {
  local summaries ruleset_id ruleset seal_payload
  summaries=$(gh api "repos/${repository}/rulesets?includes_parents=true&targets=tag") || return 1
  ruleset_id=$(jq -er --arg name 'autopus-v0.50.118-release-authority' \
    '[.[] | select(.name == $name and .target == "tag")] |
     if length == 1 then .[0].id else error("ruleset is missing or ambiguous") end' \
    <<<"$summaries") || return 1
  ruleset=$(gh api "repos/${repository}/rulesets/${ruleset_id}") || return 1
  seal_payload="$temp_dir/sealed-release-ruleset.json"
  jq -e '{name,target,enforcement,bypass_actors:[],conditions,rules}' \
    <<<"$ruleset" >"$seal_payload" || return 1
  chmod 0600 "$seal_payload"
  gh api --method PUT "repos/${repository}/rulesets/${ruleset_id}" \
    --input "$seal_payload" >/dev/null || return 1
  scripts/companion-release/verify-release-tag-ruleset.sh --sealed
}
ensure_committed_release_tag_is_sealed() {
  verify_remote_release || return 1
  if scripts/companion-release/verify-release-tag-ruleset.sh --sealed; then
    return 0
  fi
  scripts/companion-release/verify-release-tag-ruleset.sh --armed &&
    seal_release_tag_ruleset
}
emit_receipt() {
  local receipt_mode=$1
  jq -cn --arg mode "$receipt_mode" --arg release_mode "$release_mode" --arg tag "$release_tag" \
    --arg tag_object "$release_tag_object" --arg source_commit "$source_commit" --arg source_tree "$source_tree" \
    --arg evidence_tag_object "$evidence_tag_object" --arg evidence_commit "$evidence_commit" --arg evidence_tree "$evidence_tree" \
    --arg report_sha256 "$report_sha256" --arg attestation_sha256 "$attestation_sha256" \
    --arg static_policy_sha256 "$static_policy_sha256" --arg promotion_key_id "$promotion_key_id" \
    --argjson github_release_id "$reservation_id" \
    '{mode:$mode,release_mode:$release_mode,release_tag:$tag,release_tag_object:$tag_object,
      github_release_id:$github_release_id,source_commit:$source_commit,source_tree:$source_tree,
      evidence_tag_object:$evidence_tag_object,evidence_commit:$evidence_commit,evidence_tree:$evidence_tree,
      report_sha256:$report_sha256,attestation_sha256:$attestation_sha256,
      static_policy_sha256:$static_policy_sha256,promotion_signing_key_id:$promotion_key_id}'
}
names=(ADK_COMPANION_APPROVED_SOURCE_COMMIT ADK_COMPANION_APPROVED_SOURCE_TREE OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA \
  OMP_CONTEXT_EVIDENCE_COMMIT_SHA OMP_CONTEXT_EVIDENCE_TREE_SHA OMP_CONTEXT_EVIDENCE_REPORT_SHA256 \
  OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256 OMP_CONTEXT_STATIC_POLICY_B64)
values=("$source_commit" "$source_tree" "$evidence_tag_object" "$evidence_commit" "$evidence_tree" \
  "$report_sha256" "$attestation_sha256" "$static_policy_b64")
obsolete_names=()
repository_snapshot="$temp_dir/repository-variables.json"; environment_snapshot="$temp_dir/environment-variables.json"
policy_snapshot="$temp_dir/deployment-policies.json"; snapshots_ready=0; coordinates_started=0
committed=0; rollback_enabled=1; rollback_failed=0; created_release_tag=0; created_policy_id=''; policy_creation_ambiguous=0
mode='publish'; reservation_id=''; reservation_created=0; reservation_preexisting=0; reservation_ambiguous=0
verification_ref="refs/autopus-release-prep/verify-${release_tag}"; trap cleanup EXIT
remote_release=$(git ls-remote --refs origin "$release_ref") || fail 'cannot inspect remote release tag'
if [[ -n "$remote_release" ]]; then
  [[ "$transaction_kind" == 'reconcile' ]] || {
    rollback_enabled=0
    printf '%s\n' 'release coordinate publish: committed tag requires reconciliation mode' >&2
    exit 75
  }
  mode='reconcile'; committed=1; rollback_enabled=0
  if ! ensure_committed_release_tag_is_sealed; then
    printf '%s\n' 'release coordinate publish: committed tag requires ruleset sealing reconciliation' >&2
    exit 75
  fi
  if [[ -n "$(git ls-remote --refs origin "$prep_lock_ref")" ]] ||
     ! verify_coordinates ||
     ! verify_owned_release_record; then
    printf '%s\n' 'release coordinate publish: committed release coordinates require reconciliation' >&2
    exit 75
  fi
  emit_receipt reconciled
  exit 0
fi
scripts/companion-release/verify-release-tag-ruleset.sh --armed ||
  fail 'exact armed v0.50.118 tag ruleset or environment is unavailable'
[[ "$prep_lock_commit" =~ $hex40 ]] || fail 'publishing requires exact evidence prep lock commit'
lock_report="$temp_dir/lock-report.json"
git cat-file blob "${evidence_commit}:omp-context-promotion-report.v1.json" >"$lock_report"; chmod 0600 "$lock_report"
[[ "$(scripts/companion-release/verify-release-prep-lock.sh "$prep_lock_ref" "$prep_lock_commit" "$lock_report")" == "$prep_lock_commit" ]] || fail 'release-prep compare-and-swap lock is not owned'
if verify_owned_draft_reservation; then
  [[ "$transaction_kind" == 'retained' ]] || { reservation_ambiguous=1; fail 'unexpected owned draft exists for fresh transaction'; }
  reservation_preexisting=1
else
  draft_status=$?
  [[ "$draft_status" -eq 2 ]] || { reservation_ambiguous=1; fail 'GitHub Release state is unavailable, duplicated, or unowned'; }
  jq -e 'length == 0' <<<"$draft_name_matches" >/dev/null || { reservation_ambiguous=1; fail 'another draft collides with reservation'; }
fi
gh api "repos/${repository}" | jq -e '.permissions.admin == true' >/dev/null || fail 'repository administration permission is unavailable'
environment_json=$(gh api "repos/${repository}/environments/${environment_name}")
jq -e '.deployment_branch_policy.custom_branch_policies == true and .deployment_branch_policy.protected_branches == false' <<<"$environment_json" >/dev/null || fail 'environment deployment policy mode is unsafe'
local_tag_preexisting=0
if git show-ref --verify --quiet "$release_ref"; then
  env "${tag_git_config[@]}" git verify-tag "$release_ref" >/dev/null ||
    fail 'preexisting local release tag signature is invalid'
  [[ "$(git rev-parse --verify "${release_ref}^{commit}")" == "$source_commit" ]] ||
    fail 'preexisting local release tag targets another source'
  [[ "$transaction_kind" == 'retained' ]] ||
    fail 'preexisting local release tag requires retained reconciliation'
  local_tag_preexisting=1
fi
signing_probe="$temp_dir/signing-probe"; git clone --quiet --no-checkout --shared . "$signing_probe"
env "${tag_git_config[@]}" git -C "$signing_probe" tag -s release-signing-probe "$source_commit" -m 'release signing probe'
env "${tag_git_config[@]}" git -C "$signing_probe" verify-tag refs/tags/release-signing-probe >/dev/null
scope_json --repo "$repository" >"$repository_snapshot"
scope_json --repo "$repository" --env "$environment_name" >"$environment_snapshot"
gh api "repos/${repository}/environments/${environment_name}/deployment-branch-policies" >"$policy_snapshot"
jq -e 'length <= 500 and ([.[].name] | length) == ([.[].name] | unique | length)' "$repository_snapshot" >/dev/null || fail 'repository variable snapshot is incomplete or ambiguous'
jq -e 'length <= 100 and ([.[].name] | length) == ([.[].name] | unique | length)' "$environment_snapshot" >/dev/null || fail 'environment variable snapshot is incomplete or ambiguous'
chmod 0600 "$repository_snapshot" "$environment_snapshot" "$policy_snapshot"; snapshots_ready=1; coordinates_started=1
for index in "${!names[@]}"; do
  gh variable set "${names[$index]}" --repo "$repository" --body "${values[$index]}"
  gh variable set "${names[$index]}" --repo "$repository" --env "$environment_name" --body "${values[$index]}"
done
policy_count=$(jq -r --arg tag "$release_tag" '[.branch_policies[] | select(.type == "tag" and .name == $tag)] | length' "$policy_snapshot")
case "$policy_count" in
  0)
    created_policy=$(gh api --method POST "repos/${repository}/environments/${environment_name}/deployment-branch-policies" -f name="$release_tag" -f type=tag) || { policy_creation_ambiguous=1; fail 'tag policy creation is ambiguous'; }
    created_policy_id=$(jq -r --arg tag "$release_tag" 'select(.type == "tag" and .name == $tag) | .id' <<<"$created_policy")
    [[ "$created_policy_id" =~ ^[0-9]+$ ]] || { policy_creation_ambiguous=1; fail 'tag policy response is invalid'; }
    ;;
  1) ;;
  *) fail 'release tag deployment policy is ambiguous' ;;
esac
verify_coordinates || fail 'normal release coordinates differ after write'
create_or_adopt_release_reservation || fail 'cannot reserve exact operator-owned normal release draft'
verify_owned_draft_reservation || fail 'normal release draft changed before tag commit'
[[ "$(git ls-remote --refs origin "$evidence_ref")" == "$evidence_tag_object"$'\t'"$evidence_ref" ]] || fail 'evidence tag changed before release commit'
[[ "$(git ls-remote --refs origin "$prep_lock_ref")" == "$prep_lock_commit"$'\t'"$prep_lock_ref" ]] || fail 'prep lock changed before release commit'
scripts/companion-release/verify-release-tag-ruleset.sh --armed ||
  fail 'armed release tag authority ruleset or environment changed'
git fetch --no-tags origin main
[[ "$(git rev-parse --verify origin/main)" == "$source_commit" ]] || fail 'origin/main advanced before release commit'
[[ "$(git rev-parse --verify 'HEAD^{tree}')" == "$source_tree" && -z "$(git status --porcelain)" ]] || fail 'source changed before release commit'
# The R2 annotated tag is the immutable commit point; only one-way ruleset sealing follows.
if [[ "$local_tag_preexisting" -eq 0 ]]; then
  env "${tag_git_config[@]}" git tag -s "$release_tag" "$source_commit" \
    -m "${release_tag} - A29 companion release"
  created_release_tag=1
fi
env "${tag_git_config[@]}" git verify-tag "$release_ref" >/dev/null
local_tag_object=$(git rev-parse --verify "$release_ref"); rollback_enabled=0; push_status=0
git push --atomic --force-with-lease="${prep_lock_ref}:${prep_lock_commit}" origin "$release_ref:$release_ref" ":$prep_lock_ref" || push_status=$?
remote_state=''
for attempt in 1 2 3 4 5; do if remote_state=$(git ls-remote --refs origin "$release_ref" "$prep_lock_ref"); then break; fi; remote_state=''; sleep "$attempt"; done
if [[ -n "$remote_state" || "$push_status" -eq 0 ]]; then
  observed_release=$(awk -v ref="$release_ref" '$2 == ref { print $0 }' <<<"$remote_state")
  observed_lock=$(awk -v ref="$prep_lock_ref" '$2 == ref { print $0 }' <<<"$remote_state")
  if [[ "$observed_release" == "$local_tag_object"$'\t'"$release_ref" ]]; then
    committed=1; release_tag_object=$local_tag_object
    if ! ensure_committed_release_tag_is_sealed; then
      printf '%s\n' 'release coordinate publish: committed tag requires immediate ruleset sealing reconciliation' >&2
      exit 75
    fi
    if [[ -n "$observed_lock" ]]; then
      printf '%s\n' 'release coordinate publish: sealed tag retained an inconsistent prep lock' >&2
      exit 75
    fi
  elif [[ -z "$observed_release" && "$observed_lock" == "$prep_lock_commit"$'\t'"$prep_lock_ref" ]]; then
    rollback_enabled=1; fail 'atomic R2 tag commit was rejected'
  else
    committed=1
    printf '%s\n' 'release coordinate publish: atomic R2 tag commit reached an inconsistent remote state' >&2
    exit 75
  fi
else
  committed=1
  printf '%s\n' 'release coordinate publish: atomic R2 tag commit outcome requires reconciliation' >&2
  exit 75
fi
if ! verify_coordinates ||
   ! verify_owned_release_record ||
   [[ -n "$(git ls-remote --refs origin "$prep_lock_ref")" ]]; then
  printf '%s\n' 'release coordinate publish: sealed committed tag requires coordinate reconciliation' >&2
  exit 75
fi
emit_receipt committed

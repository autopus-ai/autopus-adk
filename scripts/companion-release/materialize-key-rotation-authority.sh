#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'rotation authority materializer: %s\n' "$1" >&2; exit 1; }
usage() { fail 'usage: materialize-key-rotation-authority.sh [--public EXPECTED_COMMIT] OUTPUT_DIR'; }
public_mode=0
expected_authority_commit=''
if [[ "${1-}" == '--public' ]]; then
  [[ $# -eq 3 ]] || usage
  public_mode=1
  expected_authority_commit=$2
  shift 2
fi
[[ $# -eq 1 ]] || usage
readonly public_mode expected_authority_commit output_dir=$1
readonly repository='autopus-ai/autopus-adk'
readonly environment_name='adk-companion-release'
readonly variable_name='ADK_KEY_ROTATION_AUTHORITY_COMMIT'
readonly protected_variable_name='ADK_PROTECTED_KEY_ROTATION_AUTHORITY_COMMIT'
readonly authority_ref='refs/heads/release-key-rotation-authority-v2'
readonly ruleset_name='autopus-key-rotation-authority-v2'
readonly authority_actor_id=204883817
readonly remote_url='https://github.com/autopus-ai/autopus-adk.git'
readonly api_url='https://api.github.com/repos/autopus-ai/autopus-adk'
readonly policy_name='adk-key-rotation-authority.v1.json'
readonly verifier_name='verify-rotation.sh'
for tool in env git install jq mktemp openssl rm; do
  command -v "$tool" >/dev/null 2>&1 || fail "$tool is unavailable"
done
if [[ "$public_mode" -eq 1 ]]; then
  command -v curl >/dev/null 2>&1 || fail 'curl is unavailable'
else
  command -v gh >/dev/null 2>&1 || fail 'gh is unavailable'
fi
[[ ! -e "$output_dir" && ! -L "$output_dir" ]] || fail 'output directory already exists'
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/adk-rotation-authority.XXXXXX") || fail 'cannot create private workspace'
readonly temp_dir
anonymous_home="$temp_dir/anonymous-home"
fetch_repo="$temp_dir/fetch"
materialized="$temp_dir/materialized"
install -d -m 0700 "$anonymous_home" "$materialized"
output_created=0
cleanup() {
  status=$?
  trap - EXIT
  rm -rf -- "$temp_dir"
  if [[ "$status" -ne 0 && "$output_created" -eq 1 ]]; then rm -rf -- "$output_dir"; fi
  exit "$status"
}
trap cleanup EXIT

read_variables() {
  if [[ "$public_mode" -eq 1 ]]; then
    repository_commit=$expected_authority_commit
    [[ "$repository_commit" =~ ^[0-9a-f]{40}$ ]] ||
      fail 'public authority commit assertion is malformed'
    return
  fi
  repository_commit=$(gh variable get "$variable_name" --repo "$repository") ||
    fail 'repository authority commit variable is unavailable'
  environment_commit=$(gh variable get "$variable_name" --repo "$repository" --env "$environment_name") ||
    fail 'protected-environment authority commit variable is unavailable'
  protected_commit=$(gh variable get "$protected_variable_name" --repo "$repository" --env "$environment_name") ||
    fail 'distinct protected authority commit variable is unavailable'
  [[ "$repository_commit" =~ ^[0-9a-f]{40}$ &&
     "$repository_commit" == "$environment_commit" &&
     "$repository_commit" == "$protected_commit" ]] ||
    fail 'repository and protected-environment authority commits differ or are malformed'
}
public_api() (
  unset GH_TOKEN GITHUB_TOKEN
  curl --disable --fail --silent --show-error --proto '=https' --tlsv1.2 \
    --header 'Accept: application/vnd.github+json' \
    --header 'X-GitHub-Api-Version: 2022-11-28' "$1"
)
verify_ruleset() {
  if [[ "$public_mode" -eq 1 ]]; then
    summaries=$(public_api "${api_url}/rulesets?includes_parents=true&targets=branch&per_page=100") ||
      fail 'cannot publicly inspect authority ref rulesets'
    jq -e 'type == "array" and length < 100' <<<"$summaries" >/dev/null ||
      fail 'public authority ruleset listing is incomplete'
    ruleset_id=$(jq -er --arg name "$ruleset_name" '
      [.[] | select(.name == $name and .target == "branch")] |
      if length == 1 then .[0].id else error("authority ruleset is missing or ambiguous") end
    ' <<<"$summaries") || fail 'public authority ref ruleset is missing or ambiguous'
    [[ "$ruleset_id" =~ ^[1-9][0-9]*$ ]] || fail 'public authority ref ruleset ID is malformed'
    ruleset=$(public_api "${api_url}/rulesets/${ruleset_id}") ||
      fail 'cannot publicly read exact authority ref ruleset'
    jq -e --arg repository "$repository" --arg name "$ruleset_name" --arg ref "$authority_ref" '
      .source_type == "Repository" and .source == $repository and
      .name == $name and .target == "branch" and .enforcement == "active" and
      .conditions == {ref_name:{exclude:[],include:[$ref]}} and
      (.rules | sort_by(.type)) == [{type:"creation"},{type:"deletion"},{type:"update"}]
    ' <<<"$ruleset" >/dev/null ||
      fail 'public authority ref ruleset differs from exact immutable policy'
    return
  fi
  summaries=$(gh api --paginate --slurp \
    "repos/${repository}/rulesets?includes_parents=true&targets=branch&per_page=100") ||
    fail 'cannot inspect authority ref rulesets'
  ruleset_id=$(jq -er --arg name "$ruleset_name" '
    [(.[] | .[]) | select(.name == $name and .target == "branch")] |
    if length == 1 then .[0].id else error("authority ruleset is missing or ambiguous") end
  ' <<<"$summaries") || fail 'authority ref ruleset is missing or ambiguous'
  [[ "$ruleset_id" =~ ^[1-9][0-9]*$ ]] || fail 'authority ref ruleset ID is malformed'
  ruleset=$(gh api "repos/${repository}/rulesets/${ruleset_id}") || fail 'cannot read exact authority ref ruleset'
  jq -e --arg name "$ruleset_name" --arg ref "$authority_ref" --argjson actor "$authority_actor_id" '
    .name == $name and .target == "branch" and .enforcement == "active" and
    .conditions == {ref_name:{exclude:[],include:[$ref]}} and
    .bypass_actors == [{actor_id:$actor,actor_type:"User",bypass_mode:"always"}] and
    (.rules | sort_by(.type)) == [{type:"creation"},{type:"deletion"},{type:"update"}]
  ' <<<"$ruleset" >/dev/null || fail 'authority ref ruleset differs from exact immutable policy'
}
anonymous_git() {
  env -i PATH="$PATH" HOME="$anonymous_home" XDG_CONFIG_HOME="$anonymous_home" TMPDIR="${TMPDIR:-/tmp}" \
    GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_SYSTEM=/dev/null GIT_CONFIG_GLOBAL=/dev/null \
    GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=/usr/bin/false SSH_ASKPASS=/usr/bin/false \
    GIT_NO_REPLACE_OBJECTS=1 git -c credential.helper= -c core.askPass= -c http.extraHeader= \
    -c http.followRedirects=false "$@"
}
remote_authority() {
  remote_line=$(anonymous_git ls-remote --refs "$remote_url" "$authority_ref") ||
    fail 'cannot inspect public authority ref anonymously'
  [[ "$remote_line" == "$authority_commit"$'\t'"$authority_ref" ]] ||
    fail 'public authority ref differs from the configured commit'
}

read_variables
authority_commit=$repository_commit
readonly authority_commit
verify_ruleset
remote_authority
anonymous_git init -q "$fetch_repo" >/dev/null || fail 'cannot initialize isolated authority repository'
anonymous_git -C "$fetch_repo" fetch --no-tags --force "$remote_url" "$authority_ref" >/dev/null 2>&1 ||
  fail 'cannot fetch public authority ref anonymously'
[[ "$(anonymous_git -C "$fetch_repo" rev-parse --verify FETCH_HEAD)" == "$authority_commit" &&
   "$(anonymous_git -C "$fetch_repo" cat-file -t "$authority_commit")" == commit ]] ||
  fail 'fetched authority commit differs'
anonymous_git -C "$fetch_repo" fsck --full --strict --no-dangling "$authority_commit" >/dev/null 2>&1 ||
  fail 'authority object graph is corrupt'
[[ "$(anonymous_git -C "$fetch_repo" rev-list --parents -n 1 "$authority_commit")" == "$authority_commit" ]] ||
  fail 'authority commit is not orphaned'
names=$(anonymous_git -C "$fetch_repo" ls-tree -r --name-only "$authority_commit") ||
  fail 'cannot inspect authority tree names'
[[ "$names" == "$policy_name"$'\n'"$verifier_name" ]] ||
  fail 'authority tree does not contain exactly the canonical pair'
entries=$(anonymous_git -C "$fetch_repo" ls-tree -r "$authority_commit") || fail 'cannot inspect authority tree modes'
entry_count=0
while IFS=$' \t' read -r mode type object name; do
  entry_count=$((entry_count + 1))
  [[ "$type" == blob && "$object" =~ ^[0-9a-f]{40}$ ]] || fail 'authority tree contains a non-blob'
  case "$name" in
    "$policy_name") [[ "$mode" == 100644 ]] || fail 'authority policy mode is unsafe' ;;
    "$verifier_name") [[ "$mode" == 100755 ]] || fail 'authority verifier mode is unsafe' ;;
    *) fail 'authority tree contains an unexpected path' ;;
  esac
done <<<"$entries"
[[ "$entry_count" -eq 2 ]] || fail 'authority tree entry count differs'
anonymous_git -C "$fetch_repo" cat-file blob "$authority_commit:$policy_name" >"$materialized/$policy_name" ||
  fail 'cannot materialize authority policy blob'
anonymous_git -C "$fetch_repo" cat-file blob "$authority_commit:$verifier_name" >"$materialized/$verifier_name" ||
  fail 'cannot materialize authority verifier blob'
policy_digest=$(openssl dgst -sha256 "$materialized/$policy_name") || fail 'cannot hash authority policy'
verifier_digest=$(openssl dgst -sha256 "$materialized/$verifier_name") || fail 'cannot hash authority verifier'
policy_sha256=${policy_digest##* }
verifier_sha256=${verifier_digest##* }
[[ "$policy_sha256" =~ ^[0-9a-f]{64}$ && "$verifier_sha256" =~ ^[0-9a-f]{64}$ ]] ||
  fail 'authority digest output is malformed'
jq -en --rawfile raw "$materialized/$policy_name" --arg verifier_sha "$verifier_sha256" '
  try (($raw | fromjson) as $p |
    ($p | keys_unsorted) == ["authority_schema","rotation_schema","repository","channel",
      "signature_domain","bridge_tag","release_mode","channel_key_id","channel_public_key",
      "previous_tag_fingerprint","next_tag_public_key","next_tag_fingerprint",
      "next_promotion_key_id","next_promotion_public_key","next_promotion_public_key_sha256",
      "max_validity_seconds","verifier_sha256"] and
    ($p | to_entries | all(.[]; (.value | type) == "string")) and
    $raw == ({authority_schema:$p.authority_schema,rotation_schema:$p.rotation_schema,
      repository:$p.repository,channel:$p.channel,signature_domain:$p.signature_domain,
      bridge_tag:$p.bridge_tag,release_mode:$p.release_mode,channel_key_id:$p.channel_key_id,
      channel_public_key:$p.channel_public_key,previous_tag_fingerprint:$p.previous_tag_fingerprint,
      next_tag_public_key:$p.next_tag_public_key,next_tag_fingerprint:$p.next_tag_fingerprint,
      next_promotion_key_id:$p.next_promotion_key_id,next_promotion_public_key:$p.next_promotion_public_key,
      next_promotion_public_key_sha256:$p.next_promotion_public_key_sha256,
      max_validity_seconds:$p.max_validity_seconds,verifier_sha256:$p.verifier_sha256} | tojson) and
    $p.authority_schema == "adk-key-rotation-authority.v1" and $p.verifier_sha256 == $verifier_sha)
  catch false
' >/dev/null || fail 'external authority policy or verifier bytes are internally inconsistent'

read_variables
[[ "$repository_commit" == "$authority_commit" ]] || fail 'authority commit assertion changed during materialization'
verify_ruleset
remote_authority
install -d -m 0700 "$output_dir" || fail 'cannot create authority output directory'
output_created=1
install -m 0700 "$materialized/$verifier_name" "$output_dir/$verifier_name" || fail 'cannot install authority verifier'
install -m 0600 "$materialized/$policy_name" "$output_dir/$policy_name" || fail 'cannot install authority policy'
read_variables
[[ "$repository_commit" == "$authority_commit" ]] || fail 'authority commit assertion changed after installation'
verify_ruleset
remote_authority
assertion_mode='strict'
[[ "$public_mode" -eq 0 ]] || assertion_mode='public'
readonly assertion_mode
jq -cn --arg assertion_mode "$assertion_mode" --arg authority_ref "$authority_ref" \
  --arg authority_commit "$authority_commit" --arg verifier_sha256 "$verifier_sha256" \
  --arg policy_sha256 "$policy_sha256" \
  '{assertion_mode:$assertion_mode,authority_ref:$authority_ref,authority_commit:$authority_commit,
    verifier_sha256:$verifier_sha256,policy_sha256:$policy_sha256}'

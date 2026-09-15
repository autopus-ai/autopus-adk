#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'release tag ruleset: %s\n' "$1" >&2; exit 1; }
usage() { printf '%s\n' 'usage: verify-release-tag-ruleset.sh --armed|--sealed|--sealed-runtime' >&2; exit 64; }
[[ $# -eq 1 ]] || usage
case "$1" in --armed) mode=armed ;; --sealed) mode=sealed ;; --sealed-runtime) mode=sealed-runtime ;; *) usage ;; esac
readonly mode
readonly repository='autopus-ai/autopus-adk'
readonly ruleset_name='autopus-v0.50.118-release-authority'
readonly release_tag='v0.50.118'
readonly release_ref='refs/tags/v0.50.118'
readonly release_actor_id=204883817
readonly environment_name='adk-companion-release'
for tool in gh jq; do command -v "$tool" >/dev/null || fail "${tool} is unavailable"; done
summaries=$(gh api "repos/${repository}/rulesets?includes_parents=true&targets=tag") ||
  fail 'cannot inspect release tag rulesets'
ruleset_id=$(jq -er --arg name "$ruleset_name" '
  [.[] | select(.name == $name and .target == "tag")] |
  if length == 1 then .[0].id else error("ruleset is missing or ambiguous") end
' <<<"$summaries") || fail 'release tag ruleset is missing or ambiguous'
[[ "$ruleset_id" =~ ^[1-9][0-9]*$ ]] || fail 'release tag ruleset ID is malformed'
ruleset=$(gh api "repos/${repository}/rulesets/${ruleset_id}") || fail 'cannot read exact release tag ruleset'
jq -e --arg name "$ruleset_name" --arg ref "$release_ref" --arg mode "$mode" \
  --argjson actor "$release_actor_id" '
  .name == $name and .target == "tag" and .enforcement == "active" and
  .conditions.ref_name.include == [$ref] and .conditions.ref_name.exclude == [] and
  (if $mode == "armed" then
     .bypass_actors == [{actor_id:$actor,actor_type:"User",bypass_mode:"always"}]
   elif $mode == "sealed-runtime" then
     (.bypass_actors == [] or .bypass_actors == null)
   else .bypass_actors == [] end) and
  ([.rules[].type] | sort) == ["creation","deletion","update"] and
  ([.rules[].type] | unique | length) == 3
' <<<"$ruleset" >/dev/null || fail "release tag ruleset differs from exact ${mode} authority policy"
if [[ "$mode" != 'sealed-runtime' ]]; then
  environment=$(gh api "repos/${repository}/environments/${environment_name}") ||
    fail 'cannot inspect protected release environment'
  jq -e --argjson actor "$release_actor_id" '
    .can_admins_bypass == false and
    .deployment_branch_policy == {protected_branches:false,custom_branch_policies:true} and
    ([.protection_rules[] | select(.type == "required_reviewers")] | length) == 1 and
    ([.protection_rules[] | select(.type == "required_reviewers")][0] |
      .prevent_self_review == false and
      [.reviewers[] | {type:.type,id:.reviewer.id}] == [{type:"User",id:$actor}]) and
    ([.protection_rules[] | select(.type == "branch_policy")] | length) == 1
  ' <<<"$environment" >/dev/null ||
    fail 'protected release environment differs from exact authority policy'
  deployment_policies=$(gh api \
    "repos/${repository}/environments/${environment_name}/deployment-branch-policies?per_page=100") ||
    fail 'cannot inspect protected release deployment policies'
  jq -e --arg tag "$release_tag" '
    type == "object" and (.total_count | type) == "number" and
    (.branch_policies | type) == "array" and .total_count == (.branch_policies | length) and
    ([.branch_policies[] | select(.type == "tag" and .name == $tag)] | length) == 1
  ' <<<"$deployment_policies" >/dev/null ||
    fail 'protected release environment lacks the exact deployment tag policy'
fi

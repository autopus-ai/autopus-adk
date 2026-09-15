#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'rotation ref ruleset: %s\n' "$1" >&2; exit 1; }
public_mode=0
if [[ "${1-}" == '--public' ]]; then
  public_mode=1
  shift
fi
[[ $# -eq 0 ]] || fail 'usage: verify-rotation-ref-ruleset.sh [--public]'
readonly public_mode
readonly repository='autopus-ai/autopus-adk'
readonly ruleset_name='autopus-v0.50.109-rotation-ref-authority'
readonly rotation_ref='refs/heads/release-key-rotation-v0.50.109'
readonly publisher_actor_id=204883817
readonly api_url='https://api.github.com/repos/autopus-ai/autopus-adk'
command -v jq >/dev/null || fail 'jq is unavailable'
if [[ "$public_mode" -eq 1 ]]; then
  command -v curl >/dev/null || fail 'curl is unavailable'
  public_api() (
    unset GH_TOKEN GITHUB_TOKEN
    curl --disable --fail --silent --show-error --proto '=https' --tlsv1.2 \
      --header 'Accept: application/vnd.github+json' \
      --header 'X-GitHub-Api-Version: 2022-11-28' "$1"
  )
  summaries=$(public_api "${api_url}/rulesets?includes_parents=true&targets=branch&per_page=100") ||
    fail 'cannot publicly inspect rotation ref rulesets'
  jq -e 'type == "array" and length < 100' <<<"$summaries" >/dev/null ||
    fail 'public rotation ruleset listing is incomplete'
  ruleset_id=$(jq -er --arg name "$ruleset_name" '
    [.[] | select(.name == $name and .target == "branch")] |
    if length == 1 then .[0].id else error("ruleset is missing or ambiguous") end
  ' <<<"$summaries") || fail 'public rotation ref ruleset is missing or ambiguous'
  [[ "$ruleset_id" =~ ^[1-9][0-9]*$ ]] || fail 'public rotation ref ruleset ID is malformed'
  ruleset=$(public_api "${api_url}/rulesets/${ruleset_id}") ||
    fail 'cannot publicly read exact rotation ref ruleset'
  jq -e --arg repository "$repository" --arg name "$ruleset_name" --arg ref "$rotation_ref" '
    .source_type == "Repository" and .source == $repository and
    .name == $name and .target == "branch" and .enforcement == "active" and
    .conditions == {ref_name:{exclude:[],include:[$ref]}} and
    (.rules | sort_by(.type)) == [{type:"creation"},{type:"deletion"},{type:"update"}]
  ' <<<"$ruleset" >/dev/null ||
    fail 'public rotation ref ruleset differs from exact authority policy'
  exit 0
fi

command -v gh >/dev/null || fail 'gh is unavailable'
summaries=$(gh api "repos/${repository}/rulesets?includes_parents=true&targets=branch") ||
  fail 'cannot inspect rotation ref rulesets'
ruleset_id=$(jq -er --arg name "$ruleset_name" '
  [.[] | select(.name == $name and .target == "branch")] |
  if length == 1 then .[0].id else error("ruleset is missing or ambiguous") end
' <<<"$summaries") || fail 'rotation ref ruleset is missing or ambiguous'
[[ "$ruleset_id" =~ ^[1-9][0-9]*$ ]] || fail 'rotation ref ruleset ID is malformed'
ruleset=$(gh api "repos/${repository}/rulesets/${ruleset_id}") ||
  fail 'cannot read exact rotation ref ruleset'
jq -e --arg name "$ruleset_name" --arg ref "$rotation_ref" \
  --argjson actor "$publisher_actor_id" '
  .name == $name and .target == "branch" and .enforcement == "active" and
  .conditions.ref_name.include == [$ref] and .conditions.ref_name.exclude == [] and
  .bypass_actors == [{actor_id:$actor,actor_type:"User",bypass_mode:"always"}] and
  ([.rules[].type] | sort) == ["creation","deletion","update"] and
  ([.rules[].type] | unique | length) == 3
' <<<"$ruleset" >/dev/null || fail 'rotation ref ruleset differs from exact authority policy'

#!/usr/bin/env bash
set -euo pipefail

state=${MOCK_RELEASE_PREP_STATE:?}
log="$state/calls.log"
command=${1-}; shift || true
scope_file() {
  local environment=''
  for ((index=1; index<=$#; index++)); do
    if [[ "${!index}" == '--env' ]]; then
      index=$((index + 1)); environment=${!index}
    fi
  done
  if [[ -n "$environment" ]]; then printf '%s\n' "$state/environment-variables.json"
  else printf '%s\n' "$state/repository-variables.json"; fi
}
write_count() {
  local count
  count=$(<"$state/write-count")
  count=$((count + 1)); printf '%s\n' "$count" >"$state/write-count"
  if [[ -n "${MOCK_RELEASE_PREP_FAIL_AT:-}" && "$count" == "$MOCK_RELEASE_PREP_FAIL_AT" ]]; then
    exit 75
  fi
  if [[ -n "${MOCK_RELEASE_PREP_FAIL_FROM:-}" &&
    "$count" -ge "$MOCK_RELEASE_PREP_FAIL_FROM" ]]; then
    exit 75
  fi
}
case "$command" in
  variable)
    action=${1-}; name=${2-}; shift 2 || true
    file=$(scope_file "$@")
    case "$action" in
      list) cat "$file" ;;
      get)
        jq -er --arg name "$name" '.[] | select(.name == $name) | .value' "$file" ;;
      set)
        body=''
        while [[ $# -gt 0 ]]; do
          case "$1" in --body) body=$2; shift 2 ;; *) shift ;; esac
        done
        write_count
        jq --arg name "$name" --arg value "$body" \
          '[.[] | select(.name != $name)] + [{name:$name,value:$value}] | sort_by(.name)' \
          "$file" >"$file.next"
        mv "$file.next" "$file"
        printf 'variable-set\t%s\t%s\n' "$(basename "$file")" "$name" >>"$log"
        ;;
      delete)
        write_count
        jq --arg name "$name" '[.[] | select(.name != $name)]' "$file" >"$file.next"
        mv "$file.next" "$file"
        printf 'variable-delete\t%s\t%s\n' "$(basename "$file")" "$name" >>"$log"
        ;;
      *) exit 64 ;;
    esac
    ;;
  api)
    method=GET endpoint='' input='' field_name='' field_type='' field_tag='' field_target='' field_release_name='' field_body='' field_draft='' field_prerelease=''
    while [[ $# -gt 0 ]]; do
      case "$1" in
        --method) method=$2; shift 2 ;;
        --input) input=$2; shift 2 ;;
        -f|-F|--field|--raw-field)
          case "$2" in
            name=*) field_name=${2#name=}; field_release_name=${2#name=} ;;
            type=*) field_type=${2#type=} ;;
            tag_name=*) field_tag=${2#tag_name=} ;;
            target_commitish=*) field_target=${2#target_commitish=} ;;
            body=*) field_body=${2#body=} ;;
            draft=*) field_draft=${2#draft=} ;;
            prerelease=*) field_prerelease=${2#prerelease=} ;;
          esac
          shift 2
          ;;
        --jq) shift 2 ;;
        --silent) shift ;;
        -*) shift ;;
        *) [[ -z "$endpoint" ]] && endpoint=$1; shift ;;
      esac
    done
    case "$method:$endpoint" in
      GET:repos/autopus-ai/autopus-adk) printf '%s\n' '{"permissions":{"admin":true}}' ;;
      'GET:repos/autopus-ai/autopus-adk/rulesets?includes_parents=true&targets=tag')
        printf '%s\n' '[{"id":777,"name":"autopus-v0.50.118-release-authority","target":"tag"}]'
        ;;
      GET:repos/autopus-ai/autopus-adk/rulesets/777)
        ruleset_state=$(<"$state/ruleset-state")
        case "$ruleset_state" in
          armed) bypass='[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"}]'; rules='[{"type":"creation"},{"type":"deletion"},{"type":"update"}]' ;;
          sealed)
            if [[ "${MOCK_RELEASE_PREP_MASK_ENVIRONMENT:-0}" -eq 1 ]]; then bypass='null'; else bypass='[]'; fi
            rules='[{"type":"creation"},{"type":"deletion"},{"type":"update"}]'
            ;;
          extra) bypass='[{"actor_id":204883817,"actor_type":"User","bypass_mode":"always"},{"actor_id":42,"actor_type":"User","bypass_mode":"always"}]'; rules='[{"type":"creation"},{"type":"deletion"},{"type":"update"},{"type":"required_signatures"}]' ;;
          *) exit 65 ;;
        esac
        jq -cn --argjson bypass "$bypass" --argjson rules "$rules" \
          '{id:777,name:"autopus-v0.50.118-release-authority",target:"tag",enforcement:"active",
            bypass_actors:$bypass,conditions:{ref_name:{include:["refs/tags/v0.50.118"],exclude:[]}},rules:$rules}'
        ;;
      PUT:repos/autopus-ai/autopus-adk/rulesets/777)
        [[ -f "$input" ]] || exit 65
        jq -e '.name == "autopus-v0.50.118-release-authority" and .target == "tag" and
          .enforcement == "active" and .bypass_actors == [] and
          ([.rules[].type] | sort) == ["creation","deletion","update"]' "$input" >/dev/null || exit 65
        write_count
        printf '%s\n' sealed >"$state/ruleset-state"
        printf '%s\n' ruleset-seal >>"$log"
        printf '%s\n' '{"id":777}'
        ;;
      'GET:repos/autopus-ai/autopus-adk/releases?per_page=100')
        visibility_delay=${MOCK_RELEASE_PREP_RELEASE_VISIBILITY_DELAY:-0}
        if [[ -f "$state/release-created.json" && "$visibility_delay" -gt 0 ]]; then
          visibility_delay_file="$state/release-visibility-delay"
          if [[ ! -f "$visibility_delay_file" ]]; then
            printf '%s\n' "$visibility_delay" >"$visibility_delay_file"
          fi
          remaining_visibility_delay=$(<"$visibility_delay_file")
          if [[ "$remaining_visibility_delay" -gt 0 ]]; then
            printf '%s\n' "$((remaining_visibility_delay - 1))" >"$visibility_delay_file"
            printf '%s\n' 'release-reservation-not-yet-visible' >>"$log"
            jq -c '[[.[] | select(.id != 996)]]' "$state/releases.json"
            exit 0
          fi
        fi
        jq -c '[.]' "$state/releases.json"
        ;;
      GET:repos/autopus-ai/autopus-adk/releases/tags/v0.50.118)
        jq -ce '.[] | select(.tag_name == "v0.50.118")' "$state/releases.json"
        ;;
      POST:repos/autopus-ai/autopus-adk/releases)
        [[ "$field_tag" == 'v0.50.118' && "$field_target" =~ ^[0-9a-f]{40}$ &&
           "$field_release_name" == 'v0.50.118' &&
           "$field_draft" == 'true' && "$field_prerelease" == 'false' ]] || exit 65
        jq -e --arg tag "$field_tag" 'all(.[]; .tag_name != $tag)' \
          "$state/releases.json" >/dev/null || exit 65
        write_count
        jq -n --arg tag "$field_tag" --arg target "$field_target" \
          --arg name "$field_release_name" --arg body "$field_body" \
          '{id:996,tag_name:$tag,target_commitish:$target,name:$name,body:$body,draft:true,prerelease:false,author:{id:204883817},assets:[]}' \
          >"$state/release-created.json"
        jq -s '.[0] + [.[1]]' "$state/releases.json" "$state/release-created.json" \
          >"$state/releases.json.next"
        mv "$state/releases.json.next" "$state/releases.json"
        printf '%s\n' 'release-reservation-create' >>"$log"
        if [[ "${MOCK_RELEASE_PREP_RELEASE_RESPONSE_LOST:-0}" -eq 1 ]]; then exit 75; fi
        cat "$state/release-created.json"
        ;;
      DELETE:repos/autopus-ai/autopus-adk/releases/996)
        if [[ "${MOCK_RELEASE_PREP_RELEASE_DELETE_FAIL:-0}" -eq 1 ]]; then exit 75; fi
        write_count
        jq '[.[] | select(.id != 996)]' "$state/releases.json" >"$state/releases.json.next"
        mv "$state/releases.json.next" "$state/releases.json"
        printf '%s\n' 'release-reservation-delete' >>"$log"
        ;;
      GET:repos/autopus-ai/autopus-adk/environments/adk-companion-release)
        if [[ "${MOCK_RELEASE_PREP_MASK_ENVIRONMENT:-0}" -eq 1 ]]; then
          printf '%s\n' '{"can_admins_bypass":false,"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true},"protection_rules":[{"type":"branch_policy"}]}'
        else
          printf '%s\n' '{"can_admins_bypass":false,"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true},"protection_rules":[{"type":"required_reviewers","prevent_self_review":false,"reviewers":[{"type":"User","reviewer":{"id":204883817}}]},{"type":"branch_policy"}]}'
        fi
        ;;
      GET:repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies|GET:repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies\?per_page=100)
        jq -n --slurpfile policies "$state/deployment-policies.json" \
          '{total_count:($policies[0] | length),branch_policies:$policies[0]}'
        ;;
      POST:repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies)
        [[ "$field_name" == 'v0.50.118' && "$field_type" == 'tag' ]] || exit 65
        write_count
        jq '[.[] | select(.name != "v0.50.118")] + [{id:596,type:"tag",name:"v0.50.118"}]' \
          "$state/deployment-policies.json" >"$state/deployment-policies.json.next"
        mv "$state/deployment-policies.json.next" "$state/deployment-policies.json"
        printf '%s\n' 'policy-create' >>"$log"
        if [[ "${MOCK_RELEASE_PREP_POLICY_RESPONSE_LOST:-0}" -eq 1 ]]; then
          exit 75
        fi
        printf '%s\n' '{"id":596,"type":"tag","name":"v0.50.118"}'
        ;;
      DELETE:repos/autopus-ai/autopus-adk/environments/adk-companion-release/deployment-branch-policies/596)
        write_count
        jq '[.[] | select(.id != 596)]' "$state/deployment-policies.json" >"$state/deployment-policies.json.next"
        mv "$state/deployment-policies.json.next" "$state/deployment-policies.json"
        printf '%s\n' 'policy-delete' >>"$log"
        ;;
      *) exit 64 ;;
    esac
    ;;
  *) exit 64 ;;
esac

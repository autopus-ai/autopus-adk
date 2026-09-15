#!/usr/bin/env bash
# Internal function library sourced only by publish-release-coordinates.sh.

retry() {
  local attempt
  for attempt in 1 2 3; do
    if "$@"; then return 0; fi
    sleep "$attempt"
  done
  return 1
}
scope_json() { gh variable list "$@" --json name,value; }
load_release_for_tag() {
  local releases count
  releases=$(gh api --paginate --slurp \
    "repos/autopus-ai/autopus-adk/releases?per_page=100") || return 1
  jq -e 'type == "array" and all(.[]; type == "array")' <<<"$releases" >/dev/null ||
    return 1
  release_matches=$(jq -c --arg tag "$release_tag" \
    '[.[][] | select(.tag_name == $tag)]' <<<"$releases") ||
    return 1
  draft_name_matches=$(jq -c --arg name "$reservation_name" \
    '[.[][] | select(.draft == true and .name == $name)]' <<<"$releases") ||
    return 1
  count=$(jq 'length' <<<"$release_matches") || return 1
  case "$count" in
    0) release_json=''; return 2 ;;
    1) release_json=$(jq -c '.[0]' <<<"$release_matches") || return 1 ;;
    *) return 1 ;;
  esac
}
verify_owned_draft_reservation() {
  load_release_for_tag || return $?
  jq -e --arg name "$reservation_name" --arg body "$reservation_body" \
    --arg source "$source_commit" \
    '.id | type == "number"' <<<"$release_json" >/dev/null || return 1
  jq -e --arg name "$reservation_name" --arg body "$reservation_body" \
    --arg source "$source_commit" \
    '.tag_name == $name and .draft == true and .prerelease == false and
     .author.id == 204883817 and .name == $name and .body == $body and
     .target_commitish == $source and (.assets | type == "array" and length == 0)' \
    <<<"$release_json" >/dev/null || return 1
  reservation_id=$(jq -r '.id' <<<"$release_json")
  [[ "$reservation_id" =~ ^[0-9]+$ ]] || return 1
  jq -e --argjson id "$reservation_id" \
    'length == 1 and .[0].id == $id' <<<"$draft_name_matches" >/dev/null
}
verify_owned_release_record() {
  load_release_for_tag || return $?
  jq -e '.id | type == "number"' <<<"$release_json" >/dev/null || return 1
  jq -e --arg tag "$release_tag" '.tag_name == $tag and .author.id == 204883817' \
    <<<"$release_json" >/dev/null || return 1
  if jq -e '.draft == true' <<<"$release_json" >/dev/null; then
    jq -e --arg name "$reservation_name" --arg body "$reservation_body" \
      --arg source "$source_commit" \
      '.prerelease == false and .name == $name and .body == $body and
       .target_commitish == $source and (.assets | type == "array" and length == 0)' \
      <<<"$release_json" >/dev/null || return 1
  fi
  reservation_id=$(jq -r '.id' <<<"$release_json")
  [[ "$reservation_id" =~ ^[0-9]+$ ]]
}
create_or_adopt_release_reservation() {
  local status=0 response='' post_status=0 response_id
  if verify_owned_draft_reservation; then
    if [[ "$reservation_preexisting" -eq 1 && "$transaction_kind" == 'retained' ]]; then
      return 0
    fi
    reservation_ambiguous=1
    return 1
  else
    status=$?
    if [[ "$status" -ne 2 ]]; then
      reservation_ambiguous=1
      return 1
    fi
    if ! jq -e 'length == 0' <<<"$draft_name_matches" >/dev/null; then
      reservation_ambiguous=1
      return 1
    fi
  fi
  response=$(gh api --method POST "repos/${repository}/releases" \
    -f tag_name="$release_tag" -f target_commitish="$source_commit" \
    -f name="$reservation_name" -f body="$reservation_body" \
    -F draft=true -F prerelease=false) || post_status=$?
  if ! retry verify_owned_draft_reservation; then
    reservation_ambiguous=1
    return 1
  fi
  reservation_created=1
  if [[ "$post_status" -eq 0 ]]; then
    response_id=$(jq -r '.id' <<<"$response") || {
      reservation_ambiguous=1
      return 1
    }
    if [[ "$response_id" != "$reservation_id" ]]; then
      reservation_ambiguous=1
      return 1
    fi
  fi
  if [[ -n "$(git ls-remote --refs origin "$release_ref")" ]]; then
    reservation_ambiguous=1
    return 1
  fi
}
restore_scope() {
  local snapshot=$1 name=$2 expected=$3
  shift 3
  local current_json after_json previous='' snapshot_present=0
  current_json=$(scope_json "$@") || return 1
  if jq -e --arg name "$name" 'any(.[]; .name == $name)' "$snapshot" >/dev/null; then
    snapshot_present=1
    previous=$(jq -r --arg name "$name" '.[] | select(.name == $name) | .value' "$snapshot")
    if jq -e --arg name "$name" --arg previous "$previous" \
      'any(.[]; .name == $name and .value == $previous)' <<<"$current_json" >/dev/null; then
      return 0
    fi
  elif jq -e --arg name "$name" 'all(.[]; .name != $name)' <<<"$current_json" >/dev/null; then
    return 0
  fi
  if ! jq -e --arg name "$name" --arg expected "$expected" \
    'any(.[]; .name == $name and .value == $expected)' <<<"$current_json" >/dev/null; then
    printf 'release coordinate publish: rollback ownership conflict for %s\n' "$name" >&2
    return 1
  fi
  if [[ "$snapshot_present" -eq 1 ]]; then
    retry gh variable set "$name" "$@" --body "$previous" >/dev/null || return 1
  else
    retry gh variable delete "$name" "$@" >/dev/null 2>&1 || return 1
  fi
  after_json=$(scope_json "$@") || return 1
  if [[ "$snapshot_present" -eq 1 ]]; then
    jq -e --arg name "$name" --arg previous "$previous" \
      'any(.[]; .name == $name and .value == $previous)' <<<"$after_json" >/dev/null
  else
    jq -e --arg name "$name" 'all(.[]; .name != $name)' <<<"$after_json" >/dev/null
  fi
}
restore_deleted_scope() {
  local snapshot=$1 name=$2
  shift 2
  local current_json after_json previous=''
  current_json=$(scope_json "$@") || return 1
  if jq -e --arg name "$name" 'any(.[]; .name == $name)' "$snapshot" >/dev/null; then
    previous=$(jq -r --arg name "$name" '.[] | select(.name == $name) | .value' "$snapshot")
    if jq -e --arg name "$name" --arg previous "$previous" \
      'any(.[]; .name == $name and .value == $previous)' <<<"$current_json" >/dev/null; then
      return 0
    fi
    jq -e --arg name "$name" 'all(.[]; .name != $name)' <<<"$current_json" >/dev/null ||
      return 1
    retry gh variable set "$name" "$@" --body "$previous" >/dev/null || return 1
    after_json=$(scope_json "$@") || return 1
    jq -e --arg name "$name" --arg previous "$previous" \
      'any(.[]; .name == $name and .value == $previous)' <<<"$after_json" >/dev/null
  else
    jq -e --arg name "$name" 'all(.[]; .name != $name)' <<<"$current_json" >/dev/null
  fi
}
release_owned_lock() {
  local remote_lock status=0
  remote_lock=$(git ls-remote --exit-code --refs origin "$prep_lock_ref" 2>/dev/null) || status=$?
  [[ "$status" -eq 0 && "$remote_lock" == "$prep_lock_commit"$'\t'"$prep_lock_ref" ]] || return 1
  git push --force-with-lease="${prep_lock_ref}:${prep_lock_commit}" origin ":${prep_lock_ref}" >/dev/null 2>&1 || return 1
  status=0
  git ls-remote --exit-code --refs origin "$prep_lock_ref" >/dev/null 2>&1 || status=$?
  [[ "$status" -eq 2 ]]
}
rollback_coordinates() {
  local index name current_policies
  rollback_failed=0
  for index in "${!names[@]}"; do
    restore_scope "$repository_snapshot" "${names[$index]}" "${values[$index]}" --repo "$repository" ||
      rollback_failed=1
    restore_scope "$environment_snapshot" "${names[$index]}" "${values[$index]}" \
      --repo "$repository" --env "$environment_name" || rollback_failed=1
  done
  for name in "${obsolete_names[@]-}"; do
    [[ -n "$name" ]] || continue
    restore_deleted_scope "$repository_snapshot" "$name" --repo "$repository" || rollback_failed=1
    restore_deleted_scope "$environment_snapshot" "$name" \
      --repo "$repository" --env "$environment_name" || rollback_failed=1
  done
  if [[ -n "$created_policy_id" ]]; then
    retry gh api --method DELETE "repos/${repository}/environments/${environment_name}/deployment-branch-policies/${created_policy_id}" >/dev/null 2>&1 || rollback_failed=1
    current_policies=$(gh api "repos/${repository}/environments/${environment_name}/deployment-branch-policies") || rollback_failed=1
    if [[ -n "${current_policies:-}" ]] && ! jq -e --argjson id "$created_policy_id" '.branch_policies | all(.id != $id)' <<<"$current_policies" >/dev/null; then rollback_failed=1; fi
  fi
  if [[ "$policy_creation_ambiguous" -eq 1 ]]; then
    rollback_failed=1
  fi
  [[ "$rollback_failed" -eq 0 ]]
}
delete_created_release_reservation() {
  local status=0
  [[ "$reservation_created" -eq 1 && "$reservation_id" =~ ^[0-9]+$ ]] || return 0
  verify_owned_draft_reservation || return 1
  [[ "$(jq -r '.id' <<<"$release_json")" == "$reservation_id" ]] || return 1
  retry gh api --method DELETE "repos/${repository}/releases/${reservation_id}" \
    >/dev/null 2>&1 || return 1
  load_release_for_tag || status=$?
  [[ "$status" -eq 2 ]]
}
cleanup() {
  local status=$?
  local rollback_ok=1
  trap - EXIT
  if [[ "$status" -ne 0 && "$mode" == 'publish' && "$committed" -eq 0 ]]; then
    if [[ "$transaction_kind" == 'retained' ]]; then
      printf 'release coordinate publish: retained retry failed; coordinates, reservation, and prep lock were preserved\n' >&2
      status=75
    elif [[ "$rollback_enabled" -eq 1 ]]; then
      if [[ "$coordinates_started" -eq 1 && "$snapshots_ready" -eq 1 ]]; then
        rollback_coordinates || rollback_ok=0
      fi
      if [[ "$reservation_ambiguous" -eq 1 ]]; then rollback_ok=0; fi
      if [[ "$rollback_ok" -eq 1 ]]; then
        delete_created_release_reservation || rollback_ok=0
      fi
      if [[ "$rollback_ok" -eq 1 ]]; then release_owned_lock || rollback_ok=0; fi
      if [[ "$rollback_ok" -ne 1 ]]; then
        printf 'release coordinate publish: rollback incomplete; prep lock retained for reconciliation\n' >&2
        status=75
      fi
    else
      printf 'release coordinate publish: tag commit outcome is ambiguous; coordinates, reservation, and prep lock were preserved\n' >&2
      status=75
    fi
  fi
  if [[ "$created_release_tag" -eq 1 && "$committed" -eq 0 ]]; then git update-ref -d "$release_ref" >/dev/null 2>&1 || true; fi
  git update-ref -d "$verification_ref" >/dev/null 2>&1 || true
  rm -rf -- "$temp_dir"
  exit "$status"
}
verify_coordinates() {
  local index name policies repository_variables environment_variables environment_value
  for index in "${!names[@]}"; do
    [[ "$(gh variable get "${names[$index]}" --repo "$repository")" == "${values[$index]}" ]] ||
      return 1
    environment_value=$(gh variable get "${names[$index]}" --repo "$repository" --env "$environment_name") ||
      return 1
    [[ "$environment_value" == "${values[$index]}" ]] || return 1
  done
  repository_variables=$(scope_json --repo "$repository") || return 1
  environment_variables=$(scope_json --repo "$repository" --env "$environment_name") || return 1
  for name in "${obsolete_names[@]-}"; do
    [[ -n "$name" ]] || continue
    jq -e --arg name "$name" 'all(.[]; .name != $name)' <<<"$repository_variables" >/dev/null ||
      return 1
    jq -e --arg name "$name" 'all(.[]; .name != $name)' <<<"$environment_variables" >/dev/null ||
      return 1
  done
  policies=$(gh api "repos/${repository}/environments/${environment_name}/deployment-branch-policies") ||
    return 1
  jq -e --arg tag "$release_tag" \
    '[.branch_policies[] | select(.type == "tag" and .name == $tag)] | length == 1' \
    <<<"$policies" >/dev/null
}
verify_remote_release() {
  local remote_line remote_object
  remote_line=$(git ls-remote --refs origin "$release_ref") || return 1
  remote_object=${remote_line%%$'\t'*}
  [[ "$remote_line" == "$remote_object"$'\t'"$release_ref" && "$remote_object" =~ $hex40 ]] || return 1
  git fetch --force origin "$release_ref:$verification_ref" >/dev/null 2>&1 || return 1
  [[ "$(git cat-file -t "$verification_ref")" == 'tag' ]] || return 1
  [[ "$(git rev-parse --verify "${verification_ref}^{commit}")" == "$source_commit" ]] || return 1
  env "${tag_git_config[@]}" git verify-tag "$verification_ref" >/dev/null || return 1
  release_tag_object=$remote_object
}
emit_receipt() {
  local receipt_mode=$1
  jq -cn --arg mode "$receipt_mode" --arg release_mode "$release_mode" \
    --arg tag "$release_tag" --arg tag_object "$release_tag_object" \
    --arg source_commit "$source_commit" --arg source_tree "$source_tree" \
    --arg candidate_sha256 "$candidate_sha256" \
    --arg rotation_ref "$rotation_ref" --arg rotation_ref_commit "$rotation_ref_commit" \
    --arg rotation_document_sha256 "$rotation_document_sha256" \
    --arg promotion_key_id "$promotion_key_id" \
    --arg promotion_public_sha256 "$promotion_public_sha256" \
    --argjson github_release_id "$reservation_id" \
    '{mode:$mode,release_mode:$release_mode,release_tag:$tag,
      release_tag_object:$tag_object,github_release_id:$github_release_id,
      source_commit:$source_commit,source_tree:$source_tree,
      candidate_artifact_sha256:$candidate_sha256,rotation_ref:$rotation_ref,
      rotation_ref_commit:$rotation_ref_commit,
      rotation_document_sha256:$rotation_document_sha256,
      promotion_key_id:$promotion_key_id,
      promotion_public_sha256:$promotion_public_sha256}'
}

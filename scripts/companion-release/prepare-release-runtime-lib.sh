#!/usr/bin/env bash
# Internal function library sourced only by prepare-release.sh.

remove_isolation_root() {
  local record=$1 root expected_root_identity expected_parent_identity
  IFS='|' read -r root expected_root_identity expected_parent_identity <<<"$record"
  if [[ ! -e "$root" && ! -L "$root" ]]; then return 0; fi
  [[ ! -L "$root" &&
     "$(/usr/bin/stat -f '%u:%Lp' "$root")" == '0:755' &&
     "$(/usr/bin/stat -f '%d:%i' "$root")" == "$expected_root_identity" &&
     "$(/usr/bin/stat -f '%d:%i' /private/tmp)" == "$expected_parent_identity" ]] || return 1
  canary_helper remove-root "$dispatch_nonce" || return 1
  [[ ! -e "$root" && ! -L "$root" ]]
}
invalidate_sudo_authorization() { /usr/bin/sudo -k; }
cleanup() {
  local status=$1 remote_status=0 remote_source='' record cleanup_failed=0
  if [[ -n "${sudo_keepalive_pid:-}" ]]; then
    kill "$sudo_keepalive_pid" >/dev/null 2>&1 || true
    wait "$sudo_keepalive_pid" >/dev/null 2>&1 || true
    sudo_keepalive_pid=''
  fi
  if ! cleanup_live_canary_uid; then
    printf 'companion release prep: dedicated live-canary UID cleanup failed\n' >&2
    cleanup_failed=1
  fi
  if declare -F remove_release_canary_account >/dev/null 2>&1; then
    if ! remove_release_canary_account; then
      printf 'companion release prep: dedicated live-canary account cleanup failed\n' >&2
      cleanup_failed=1
    fi
  elif [[ -n "${release_canary_user:-}" ]]; then
    cleanup_failed=1
  fi
  for record in "${isolation_roots[@]-}"; do
    [[ -n "$record" ]] || continue
    if ! remove_isolation_root "$record"; then
      printf 'companion release prep: isolated canary cleanup failed for %s\n' "${record%%|*}" >&2
      cleanup_failed=1
    fi
  done
  if ! invalidate_sudo_authorization; then
    printf 'companion release prep: sudo authorization invalidation failed\n' >&2
    cleanup_failed=1
  fi
  # Probe records are retained only by a cleanup that dismantled the live UID,
  # the account, every isolation root and the sudo authorization, and only
  # before the staging tree is removed. The probe lane reports its own outcome.
  probe_finalize_records "$cleanup_failed" || cleanup_failed=1
  if [[ -n "$evidence_source_commit" && "$retain_prep_lock" -eq 0 ]]; then
    remote_source=$(GIT_TERMINAL_PROMPT=0 GIT_HTTP_LOW_SPEED_LIMIT=1 GIT_HTTP_LOW_SPEED_TIME=10 \
      git ls-remote --exit-code --refs origin "$evidence_source_ref" 2>/dev/null) || remote_status=$?
    if [[ "$remote_status" -eq 0 && "$remote_source" == "$evidence_source_commit"$'\t'"$evidence_source_ref" ]]; then
      GIT_TERMINAL_PROMPT=0 GIT_HTTP_LOW_SPEED_LIMIT=1 GIT_HTTP_LOW_SPEED_TIME=10 \
        git push --force-with-lease="${evidence_source_ref}:${evidence_source_commit}" \
        origin ":${evidence_source_ref}" >/dev/null 2>&1 ||
        printf 'companion release prep: warning: owned prep lock cleanup failed\n' >&2
    elif [[ "$remote_status" -ne 2 ]]; then
      printf 'companion release prep: warning: prep lock ownership could not be inspected\n' >&2
    fi
  fi
  rm -rf -- "$temp_dir"
  if [[ "$status" -eq 0 && "$cleanup_failed" -ne 0 ]]; then status=1; fi
  exit "$status"
}
matched_variable() {
  local name=$1 repository_value environment_count environment_value
  repository_value=$(gh variable get "$name" --repo "$repository") ||
    fail "repository variable ${name} is unavailable"
  environment_count=$(jq --arg name "$name" '[.[] | select(.name == $name)] | length' \
    <<<"$environment_variables")
  case "$environment_count" in
    0) ;;
    1)
      environment_value=$(jq -r --arg name "$name" '.[] | select(.name == $name) | .value' \
        <<<"$environment_variables")
      [[ "$repository_value" == "$environment_value" ]] ||
        fail "repository/environment variable ${name} differs"
      ;;
    *) fail "environment variable ${name} is ambiguous" ;;
  esac
  [[ -n "$repository_value" ]] || fail "repository variable ${name} is unavailable"
  printf '%s' "$repository_value"
}
build_candidate() {
  local policy=$1 output=$2
  env COMPANION_RELEASE_TAG="$release_tag" GITHUB_SHA="$source_commit" \
    COMPANION_SOURCE_TREE="$source_tree" OMP_CONTEXT_STATIC_POLICY_B64="$policy" \
    scripts/companion-release/build-omp-context-candidate.sh "$output"
}
extract_project() {
  local destination=$1
  install -d -m 0700 "$destination"
  git archive "$source_commit" | tar -x -C "$destination"
}
capture_canary_progress() {
  local output=$1 label=$2
  [[ "$label" == 'final' ]] || return 1
  [[ "$output" == /* && ! -e "$output" && ! -L "$output" ]] || return 1
  : >"$output" || return 1
  LC_ALL=C /usr/bin/awk -v output="$output" -v label="$label" '
    {
      print $0 >> output
      close(output)
      printf "companion release prep: %s production canary progress (%d/42 records)\n", label, NR > "/dev/stderr"
      fflush("/dev/stderr")
    }
  '
}
canary_failure_receipt() {
  local label=$1 canary_status=$2 output=$3 transcript_records failure_fields
  local failure_code failure_stage failed_sequence gate_diagnostic
  if [[ ! "$canary_status" =~ ^[1-9][0-9]*$ ]] || (( canary_status > 255 )); then canary_status=1; fi
  transcript_records=$(wc -l <"$output" | tr -d ' ')
  [[ "$transcript_records" =~ ^[0-9]+$ ]] || transcript_records=unknown
  failure_fields=$(jq -s -r \
    '[.[] | select(.type? == "error")] | if length == 0 then
       ["unclassified", "unknown", "0", ""] else
       [(.[-1].error_code? // "unclassified"), (.[-1].error_stage? // "unknown"),
        ((.[-1].failed_sequence? // 0) | tostring), (.[-1].gate_diagnostic? // "")] end | @tsv' "$output" 2>/dev/null) ||
    failure_fields=$'unparseable\tunknown\t0\t'
  IFS=$'\t' read -r failure_code failure_stage failed_sequence gate_diagnostic <<<"$failure_fields"
  [[ "$failure_code" =~ ^[A-Za-z0-9_.:-]{1,128}$ ]] || failure_code=unparseable
  [[ "$failure_stage" =~ ^[a-z]{1,32}$ ]] || failure_stage=unknown
  [[ "$failed_sequence" =~ ^[0-9]+$ ]] || failed_sequence=0
  # The gate verdict is counts and basis points by construction; anything else
  # is dropped rather than echoed. (bash 3.2 rejects {n,m} after a bracket
  # expression, hence the separate length bound.)
  [[ ${#gate_diagnostic} -le 400 && "$gate_diagnostic" =~ ^[a-z_=/0-9[:space:]]*$ ]] || gate_diagnostic=''
  printf 'companion release prep: %s production canary execution failed: exit=%s transcript_records=%s/42 error_code=%s error_stage=%s failed_sequence=%s\n' \
    "$label" "$canary_status" "$transcript_records" "$failure_code" "$failure_stage" "$failed_sequence" >&2
  [[ -z "$gate_diagnostic" ]] ||
    printf 'companion release prep: %s production canary gate verdict: %s\n' "$label" "$gate_diagnostic" >&2
  return "$canary_status"
}

cleanup_live_canary_uid() {
  [[ "${release_canary_account_created:-0}" -eq 1 ]] || {
    [[ "${live_canary_started:-0}" -eq 0 ]]
    return
  }
  terminate_release_canary_processes || return 1
  live_canary_started=0
}

run_canary() {
  local candidate=$1 project=$2 output=$3 label=$4
  local sandbox_args=() root isolated_project isolated_home isolated_tmp
  local isolated_candidate isolated_omp root_identity record
  local candidate_sha isolated_candidate_sha
  local canary_status started_at process_status canary_attempt
  root="/private/tmp/autopus-adk-release-prep-${dispatch_nonce}-final"
  [[ "$label" == 'final' &&
     "$root" =~ ^/private/tmp/autopus-adk-release-prep-[0-9a-f]{32}-final$ &&
     ! -e "$root" && ! -L "$root" ]] || fail 'isolated canary root is unsafe'
  canary_helper prepare-root "$dispatch_nonce"
  root_identity=$(/usr/bin/stat -f '%d:%i' "$root")
  record="${root}|${root_identity}|${private_tmp_identity}"
  isolation_roots+=("$record")
  isolated_project="$root/project"; isolated_home="$root/home"; isolated_tmp="$root/tmp"
  isolated_candidate="$root/auto"; isolated_omp="$root/omp-darwin-arm64"
  # The helper reads the project and both executables as this account and
  # writes them root-owned into the root; it never runs them as root.
  canary_helper stage "$dispatch_nonce" "$project"
  canary_helper install-exec "$dispatch_nonce" auto "$candidate" >/dev/null
  canary_helper install-exec "$dispatch_nonce" omp-darwin-arm64 "$omp_executable" >/dev/null
  candidate_sha=$(shasum -a 256 "$candidate" | awk '{print $1}')
  isolated_candidate_sha=$(shasum -a 256 "$isolated_candidate" | awk '{print $1}')
  [[ "$candidate_sha" == "$isolated_candidate_sha" &&
     "$(/usr/bin/stat -f '%u:%Lp' "$isolated_candidate")" == '0:555' &&
     "$(shasum -a 256 "$isolated_omp" | awk '{print $1}')" == "$expected_omp_sha256" ]] ||
    fail 'root-owned canary executable bytes differ'
  sandbox_args=(--omp "$isolated_omp")
  if [[ "$inherit_parent_sandbox" -eq 1 ]]; then sandbox_args+=(--inherit-parent-sandbox); fi
  if [[ "${probe_enabled:-0}" -eq 1 ]]; then sandbox_args+=(--probe-dir "$isolated_tmp/probe"); fi
  sudo_keepalive_alive || fail 'sudo keepalive stopped before production canary'
  while :; do
    select_release_canary_account "$isolated_home" ||
      fail 'no safe one-shot live-canary UID is available'
    canary_helper chown "$dispatch_nonce" canary "$release_canary_attempt"
    verify_release_canary_account ||
      fail 'one-shot live-canary account changed before launch'
    if release_canary_uid_is_process_free "$release_canary_uid"; then break; else process_status=$?; fi
    [[ "$process_status" -eq 1 ]] || fail 'inspect one-shot live-canary UID before launch'
    discard_release_canary_account ||
      fail 'discard one-shot live-canary UID that changed before launch'
    # The tree already belongs to that UID, and the helper never hands a tree
    # back to root, so a process appearing there after the hand-over stops
    # the run instead of trying the next UID.
    fail 'a process appeared under the one-shot live-canary UID after the hand-over'
  done
  canary_attempt=$release_canary_attempt
  live_canary_started=1
  printf 'companion release prep: %s production canary started (40 sequential provider calls, 20 task pairs)\n' "$label" >&2
  started_at=$SECONDS
  canary_status=0
  if {
    printf '%s\n' "$provider_credential"
    /bin/cat "$input_jsonl"
  } | canary_helper run-canary "$dispatch_nonce" "$release_canary_attempt" -- \
    /usr/bin/env -i PATH='/usr/bin:/bin:/usr/sbin:/sbin' \
    HOME="$isolated_home" TMPDIR="$isolated_tmp" LC_ALL=C /bin/sh -c '
      credential_locator=$1; project=$2; candidate=$3; shift 3
      IFS= read -r credential || exit 1
      export "$credential_locator=$credential"
      cd "$project" || exit 1
      exec "$candidate" "$@"
    ' sh "$credential_locator" "$isolated_project" "$isolated_candidate" \
    workflow context-runtime observe-session \
    --explicit-live --input-jsonl - --output - --format jsonl --project-dir "$isolated_project" --spec-id "$spec_id" \
    --provider "$provider" --model "$model" --model-context-window "$model_context_window" --endpoint "$endpoint" \
    --credential-locator "$credential_locator" --producer-repository "$repository" --producer-workflow-ref "$producer_workflow_ref" \
    --producer-run-id "$producer_run_id" --producer-run-attempt 1 --candidate-repository "$repository" \
    --policy-id omp-context-active-v1 --oracle-policy-digest "$oracle_policy_digest" --target-git-commit "$source_commit" \
    "${sandbox_args[@]}" | capture_canary_progress "$output" "$label"; then :; else canary_status=$?; fi
  sudo_keepalive_alive || fail 'sudo keepalive stopped during production canary'
  cleanup_live_canary_uid ||
    fail 'one-shot live-canary UID cleanup is incomplete'
  probe_export_records "$isolated_tmp" "$output" "$label" ||
    fail 'probe record export failed; nothing is staged and nothing is retained'
  if [[ "$canary_status" -ne 0 ]]; then canary_failure_receipt "$label" "$canary_status" "$output"; fi
  remove_release_canary_account ||
    fail 'one-shot live-canary account cleanup is incomplete'
  canary_helper chown "$dispatch_nonce" nobody "$canary_attempt"
  OMP_CONTEXT_RELEASE_CANARY_ROOT="$root" OMP_CONTEXT_RELEASE_CANARY_EXECUTABLE="$isolated_omp" \
    "$execsmoke" --artifact "$isolated_candidate" --expected-version "${release_tag#v}" \
    --architecture arm64 --timeout 30s
  printf 'companion release prep: %s production canary completed (42/42 records, %ss)\n' \
    "$label" "$((SECONDS - started_at))" >&2
  rm -rf -- "$project"
  # A child running as this account writes the copy; root only reads the
  # canary's tree, and skips hard links and special files in it.
  canary_helper unstage "$dispatch_nonce" "$project"
  remove_isolation_root "$record" || fail 'isolated canary root cleanup failed'
  assert_source_identity
}
validate_canary() {
  local project=$1 output=$2 candidate=$3 candidate_sha
  local report="$project/.autopus/runtime/omp-context/promotion-report-v1.json"
  [[ -f "$report" && ! -L "$report" ]] || fail 'production report is absent'
  candidate_sha=$(shasum -a 256 "$candidate" | awk '{print $1}')
  jq -s -e '([.[] | select(.type == "handshake")] | length) == 1 and
    ([.[] | select(.type == "call")] | length) == 40 and
    ([.[] | select(.type == "call") | .task_id_digest] | unique | length) == 20 and
    ([.[] | select(.type == "shutdown")] | length) == 1 and
    .[-1].cleanup_verified == true and .[-1].calls_completed == 40' "$output" >/dev/null ||
    fail 'production canary transcript is invalid'
  jq -e --arg repository "$repository" --arg revision "$source_commit" --arg tree "$source_tree" \
    --arg artifact "sha256:${candidate_sha}" --arg version "${release_tag#v}" \
    '.candidate.repository == $repository and .candidate.revision == $revision and
     .candidate.tree_sha == $tree and .candidate.artifact_sha256 == $artifact and
     .runtime.auto_version == $version and (.gates | length) == 14 and
     all(.gates[]; .status == "passed")' "$report" >/dev/null ||
    fail 'production report coordinates or gates differ'
}
derive_policy() {
  local report=$1 output=$2
  "$policy_tool" companion-manifest omp-context-static-policy --report "$report" \
    --target darwin-arm64 --release-lineage-key-id "$lineage_key_id" \
    --release-lineage-handoff "$lineage_handoff" --minimum-rollback-floor "$rollback_floor" \
    --promotion-signing-key-id "$expected_promotion_key_id" >"$output"
  chmod 0600 "$output"
}
create_prep_lock() {
  local report=$1 report_name='omp-context-promotion-report.v1.json' report_blob evidence_index evidence_tree
  report_blob=$(git hash-object -w -- "$report")
  evidence_index="$temp_dir/evidence.index"; rm -f -- "$evidence_index"
  GIT_INDEX_FILE="$evidence_index" git read-tree --empty
  GIT_INDEX_FILE="$evidence_index" git update-index --add --cacheinfo "100644,$report_blob,$report_name"
  evidence_tree=$(GIT_INDEX_FILE="$evidence_index" git write-tree)
  evidence_source_commit=$(printf 'OMP context release-prep source %s\n' "$release_tag" | \
    git -c user.name='Joseph' -c user.email='joseph@Josephui-MacBookPro.local' commit-tree "$evidence_tree")
  [[ "$(git rev-list --parents -n 1 "$evidence_source_commit")" == "$evidence_source_commit" ]] ||
    fail 'prep lock commit is not an orphan'
  git push --force-with-lease="${evidence_source_ref}:" origin "$evidence_source_commit:$evidence_source_ref"
  [[ "$(git ls-remote --refs origin "$evidence_source_ref")" == "$evidence_source_commit"$'\t'"$evidence_source_ref" ]] ||
    fail 'prep lock differs after acquisition'
}
use_retained_prep_lock() {
  local report=$1 verified_lock
  [[ -n "$retained_lock_commit" ]] || fail 'retained prep lock is unavailable'
  verified_lock=$(scripts/companion-release/verify-release-prep-lock.sh \
    "$evidence_source_ref" "$retained_lock_commit" "$report") || fail 'retained prep lock verification failed'
  [[ "$verified_lock" == "$retained_lock_commit" ]] || fail 'retained prep lock verifier returned another commit'
  evidence_source_commit=$retained_lock_commit; prep_lock_mode='retained'; retain_prep_lock=1
}
ensure_prep_lock() {
  local report=$1
  if [[ -n "$retained_lock_commit" ]]; then use_retained_prep_lock "$report"; else create_prep_lock "$report"; fi
}
publish_coordinates() {
  local lock_argument=$1 coordinates status
  if [[ "$lock_argument" != 'reconcile' ]]; then lock_argument="${prep_lock_mode}:${lock_argument}"; fi
  if coordinates=$(scripts/companion-release/publish-release-coordinates.sh \
    "$repository" "$environment_name" "$release_tag" "$source_commit" "$source_tree" "$static_policy_file" \
    "$evidence_tag_object" "$evidence_commit" "$evidence_tree" "$report_sha256" "$attestation_sha256" \
    "$lock_argument" "$tag_signing_key"); then
    evidence_source_commit=''; printf '%s\n' "$coordinates"; return 0
  else
    status=$?; if [[ "$status" -eq 75 ]]; then retain_prep_lock=1; fi; exit "$status"
  fi
}

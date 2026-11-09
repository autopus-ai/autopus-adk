#!/usr/bin/env bash
# One-shot DirectoryService identity for numeric-UID canary resolution.

readonly release_canary_uid_min=50000
readonly release_canary_uid_max=59999
readonly release_canary_uid_attempts=64
readonly release_canary_stabilize_seconds=15
readonly release_canary_fixed_gid=20

release_canary_account_value() {
  local attribute=$1
  /usr/bin/dscl . -read "/Users/${release_canary_user}" "$attribute" 2>/dev/null |
    /usr/bin/awk '
      NR == 1 { sub(/^([^:]+:)+[[:space:]]*/, ""); value = $0; next }
      {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "")
        value = value (value == "" ? "" : " ") $0
      }
      END {
        gsub(/^[[:space:]]+|[[:space:]]+$/, "", value)
        print value
      }
    '
}

release_canary_candidate_uid() {
  local attempt=$1 seed span
  [[ "$dispatch_nonce" =~ ^[0-9a-f]{32}$ &&
     "$attempt" =~ ^[1-9][0-9]*$ ]] || return 1
  (( attempt <= release_canary_uid_attempts )) || return 1
  seed=$((16#${dispatch_nonce:0:8}))
  span=$((release_canary_uid_max - release_canary_uid_min + 1))
  printf '%d\n' "$((release_canary_uid_min + (seed + attempt - 1) % span))"
}

release_canary_account_name() {
  printf '_autopus_v110_%s_%02d\n' "${dispatch_nonce:0:8}" "$1"
}

release_canary_account_marker() {
  printf 'Autopus Release Canary %s attempt %02d uid %s\n' "$dispatch_nonce" "$1" "$2"
}

release_canary_state_is_valid() {
  local expected_user expected_marker
  [[ "$dispatch_nonce" =~ ^[0-9a-f]{32}$ &&
     "${release_canary_attempt:-}" =~ ^[1-9][0-9]*$ &&
     "${release_canary_uid:-}" =~ ^[0-9]+$ ]] || return 1
  (( release_canary_attempt <= release_canary_uid_attempts &&
     release_canary_uid >= release_canary_uid_min &&
     release_canary_uid <= release_canary_uid_max )) || return 1
  expected_user=$(release_canary_account_name "$release_canary_attempt")
  expected_marker=$(release_canary_account_marker "$release_canary_attempt" "$release_canary_uid")
  [[ "$release_canary_user" == "$expected_user" &&
     "$release_canary_gid" == "$release_canary_fixed_gid" &&
     "$release_canary_home" =~ ^/private/tmp/autopus-adk-release-prep-[0-9a-f]{32}-final/home$ &&
     "$release_canary_marker" == "$expected_marker" ]]
}

verify_release_canary_account_ownership() {
  release_canary_state_is_valid &&
    [[ "${release_canary_account_created:-0}" -eq 1 &&
       "$(release_canary_account_value RealName)" == "$release_canary_marker" ]]
}

release_canary_uid_is_exclusively_owned() {
  local inventory account_name account_uid extra matches=0
  inventory=$(/usr/bin/dscl . -list /Users UniqueID) || return 1
  while read -r account_name account_uid extra; do
    [[ -n "$account_name" ]] || continue
    [[ -z "$extra" && "$account_uid" =~ ^-?[0-9]+$ ]] || return 1
    if [[ "$account_uid" == "$release_canary_uid" ]]; then
      [[ "$account_name" == "$release_canary_user" ]] || return 1
      matches=$((matches + 1))
    fi
  done <<<"$inventory"
  [[ "$matches" -eq 1 ]]
}

verify_release_canary_account() {
  verify_release_canary_account_ownership &&
    [[ "$(release_canary_account_value UniqueID)" == "$release_canary_uid" &&
       "$(release_canary_account_value PrimaryGroupID)" == "$release_canary_gid" &&
       "$(release_canary_account_value NFSHomeDirectory)" == "$release_canary_home" &&
       "$(release_canary_account_value UserShell)" == '/usr/bin/false' &&
       "$(release_canary_account_value IsHidden)" == '1' ]] &&
    release_canary_uid_is_exclusively_owned
}

clear_release_canary_account_state() {
  release_canary_user=''
  release_canary_uid=''
  release_canary_gid=''
  release_canary_home=''
  release_canary_marker=''
  release_canary_attempt=''
  release_canary_account_created=0
}

release_canary_uid_is_process_free() {
  local status
  if /usr/bin/pgrep -u "$1" >/dev/null 2>&1; then
    return 1
  else
    status=$?
  fi
  [[ "$status" -eq 1 ]] && return 0
  return 2
}

release_canary_candidate_available() {
  local uid=$1 user=$2 inventory account_name account_uid extra status
  inventory=$(/usr/bin/dscl . -list /Users UniqueID) || return 2
  while read -r account_name account_uid extra; do
    [[ -n "$account_name" ]] || continue
    [[ -z "$extra" && "$account_uid" =~ ^-?[0-9]+$ ]] || return 2
    [[ "$account_name" != "$user" && "$account_uid" != "$uid" ]] || return 1
  done <<<"$inventory"
  if release_canary_uid_is_process_free "$uid"; then
    return 0
  else
    status=$?
  fi
  [[ "$status" -eq 1 ]] && return 1
  return 2
}

create_release_canary_account() {
  local home=$1 attempt=$2 uid=$3
  [[ -z "${release_canary_user:-}" && -z "${release_canary_uid:-}" &&
     "$home" =~ ^/private/tmp/autopus-adk-release-prep-[0-9a-f]{32}-final/home$ ]] ||
    fail 'one-shot live-canary account state is invalid'
  release_canary_user=$(release_canary_account_name "$attempt")
  release_canary_uid=$uid
  release_canary_gid=$release_canary_fixed_gid
  release_canary_home=$home
  release_canary_attempt=$attempt
  release_canary_marker=$(release_canary_account_marker "$attempt" "$uid")
  release_canary_state_is_valid || fail 'one-shot live-canary identity is invalid'
  # The helper derives the same name, UID, and marker from the nonce and
  # attempt, refuses a taken name or UID, and sets the marker first.
  if ! canary_helper account-create "$dispatch_nonce" "$release_canary_attempt"; then
    [[ "$(release_canary_account_value RealName)" == "$release_canary_marker" ]] &&
      release_canary_account_created=1
    fail 'create owned one-shot live-canary account'
  fi
  release_canary_account_created=1
  verify_release_canary_account || fail 'one-shot live-canary account differs after creation'
}

terminate_release_canary_processes() {
  local attempt status account_uid
  verify_release_canary_account_ownership || return 1
  account_uid=$(release_canary_account_value UniqueID)
  if [[ "$account_uid" != "$release_canary_uid" ]]; then
    release_canary_uid_is_process_free "$release_canary_uid"
    return
  fi
  if ! release_canary_uid_is_exclusively_owned; then
    release_canary_uid_is_process_free "$release_canary_uid"
    return
  fi
  if release_canary_uid_is_process_free "$release_canary_uid"; then
    return 0
  else
    status=$?
  fi
  [[ "$status" -eq 1 ]] || return 1
  canary_helper account-kill "$dispatch_nonce" "$release_canary_attempt" TERM || return 1
  for attempt in 1 2 3 4 5; do
    /bin/sleep 1
    if release_canary_uid_is_process_free "$release_canary_uid"; then
      return 0
    else
      status=$?
    fi
    [[ "$status" -eq 1 ]] || return 1
  done
  canary_helper account-kill "$dispatch_nonce" "$release_canary_attempt" KILL || return 1
  /bin/sleep 1
  release_canary_uid_is_process_free "$release_canary_uid"
}

remove_release_canary_account() {
  local inventory account_name
  [[ -n "${release_canary_user:-}" ]] || return 0
  verify_release_canary_account_ownership || return 1
  release_canary_uid_is_process_free "$release_canary_uid" || return 1
  canary_helper account-delete "$dispatch_nonce" "$release_canary_attempt" || return 1
  inventory=$(/usr/bin/dscl . -list /Users) || return 1
  while read -r account_name; do
    [[ "$account_name" != "$release_canary_user" ]] || return 1
  done <<<"$inventory"
  clear_release_canary_account_state
}

discard_release_canary_account() {
  terminate_release_canary_processes &&
    remove_release_canary_account
}

stabilize_release_canary_account() {
  local elapsed status
  verify_release_canary_account || return 2
  for ((elapsed = 0; elapsed < release_canary_stabilize_seconds; elapsed++)); do
    if release_canary_uid_is_process_free "$release_canary_uid"; then
      :
    else
      status=$?
      [[ "$status" -eq 1 ]] && return 1
      return 2
    fi
    /bin/sleep 1
  done
  release_canary_uid_is_process_free "$release_canary_uid"
}

select_release_canary_account() {
  local home=$1 attempt uid user status
  [[ "${release_canary_next_attempt:-}" =~ ^[1-9][0-9]*$ ]] ||
    fail 'one-shot live-canary attempt state is invalid'
  for ((attempt = release_canary_next_attempt; attempt <= release_canary_uid_attempts; attempt++)); do
    release_canary_next_attempt=$((attempt + 1))
    uid=$(release_canary_candidate_uid "$attempt") ||
      fail 'derive one-shot live-canary UID'
    user=$(release_canary_account_name "$attempt")
    if release_canary_candidate_available "$uid" "$user"; then
      :
    else
      status=$?
      [[ "$status" -eq 1 ]] && continue
      fail 'inspect one-shot live-canary candidate'
    fi
    create_release_canary_account "$home" "$attempt" "$uid"
    if stabilize_release_canary_account; then
      return 0
    else
      status=$?
    fi
    [[ "$status" -eq 1 ]] || fail 'stabilize one-shot live-canary account'
    discard_release_canary_account ||
      fail 'discard stale one-shot live-canary account'
  done
  return 1
}

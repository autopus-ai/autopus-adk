#!/usr/bin/env bash
set -euo pipefail
umask 077

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_dir=$(cd -- "$tests_dir/.." && pwd)
runtime_lib="$script_dir/prepare-release-runtime-lib.sh"
prep="$script_dir/prepare-release.sh"
candidate_builder="$script_dir/build-omp-context-candidate.sh"
mock_gh="$tests_dir/testdata/mock-release-prep-gh.sh"
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/release-prep-environment.XXXXXX")
trap 'rm -rf -- "$temp_dir"' EXIT
fail() { printf 'release prep environment test: %s\n' "$1" >&2; exit 1; }

state="$temp_dir/state"
mkdir -p "$state" "$temp_dir/bin"
cp "$mock_gh" "$temp_dir/bin/gh"; chmod 0700 "$temp_dir/bin/gh"
printf '%s\n' '0' >"$state/write-count"
printf '%s\n' '[{"name":"ADK_COMPANION_KEY_ID","value":"adk-release-2026-q3-b0"}]' >"$state/repository-variables.json"
export MOCK_RELEASE_PREP_STATE="$state" PATH="$temp_dir/bin:$PATH"
repository='autopus-ai/autopus-adk'; environment_name='adk-companion-release'
# shellcheck source=../prepare-release-runtime-lib.sh
source "$runtime_lib"
# shellcheck source=../prepare-release-probe-lib.sh
source "$script_dir/prepare-release-probe-lib.sh"
grep -Fq "trap 'cleanup \$?' EXIT" "$prep" || fail 'release prep does not preserve the original cleanup status'

builder_error="$temp_dir/builder-error"
if env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}" \
  GITHUB_REF_NAME=v0.50.111 GITHUB_SHA=bad COMPANION_SOURCE_TREE=bad OMP_CONTEXT_STATIC_POLICY_B64=eA \
  /bin/bash "$candidate_builder" "$temp_dir/builder-output" >/dev/null 2>"$builder_error"; then
  fail 'candidate builder accepted the reserved GitHub tag variable'
fi
grep -Fq 'release tag is not exact A29' "$builder_error" || fail 'candidate builder still reads the reserved GitHub tag variable'
if env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}" \
  COMPANION_RELEASE_TAG=v0.50.118 GITHUB_SHA=bad COMPANION_SOURCE_TREE=bad OMP_CONTEXT_STATIC_POLICY_B64=eA \
  /bin/bash "$candidate_builder" "$temp_dir/builder-output" >/dev/null 2>"$builder_error"; then
  fail 'candidate builder accepted a malformed source commit'
fi
grep -Fq 'source commit is malformed' "$builder_error" || fail 'candidate builder rejected its dedicated release tag variable'

validation_error="$temp_dir/validation-error"
if (validate_canary "$temp_dir/missing-project" "$temp_dir/missing-output" "$temp_dir/missing-candidate") 2>"$validation_error"; then
  fail 'missing production report validation succeeded'
fi
grep -Fq 'production report is absent' "$validation_error" || fail 'canary validation failed before deriving its report path'
! grep -Fq 'unbound variable' "$validation_error" || fail 'canary validation derived its report path before binding project'

printf '%s\n' '[]' >"$state/environment-variables.json"
environment_variables=$(gh variable list --repo "$repository" --env "$environment_name" --json name,value)
actual=$(matched_variable ADK_COMPANION_KEY_ID)
[[ "$actual" == 'adk-release-2026-q3-b0' ]] || fail 'repository inheritance was rejected'
printf '%s\n' '[{"name":"ADK_COMPANION_KEY_ID","value":"adk-release-2026-q3-b0"}]' >"$state/environment-variables.json"
environment_variables=$(gh variable list --repo "$repository" --env "$environment_name" --json name,value)
[[ "$(matched_variable ADK_COMPANION_KEY_ID)" == "$actual" ]] || fail 'matching override was rejected'
printf '%s\n' '[{"name":"ADK_COMPANION_KEY_ID","value":"conflicting-key"}]' >"$state/environment-variables.json"
environment_variables=$(gh variable list --repo "$repository" --env "$environment_name" --json name,value)
if (matched_variable ADK_COMPANION_KEY_ID >/dev/null 2>&1); then fail 'conflicting environment override was accepted'; fi

empty_cleanup_dir="$temp_dir/empty-cleanup"; mkdir -p "$empty_cleanup_dir"
(
  evidence_source_commit=''; retain_prep_lock=0; isolation_roots=(); sudo_keepalive_pid=''; temp_dir="$empty_cleanup_dir"
  cleanup 0
) || fail 'empty isolation cleanup failed'
[[ ! -e "$empty_cleanup_dir" ]] || fail 'empty isolation cleanup left temporary state'
failure_cleanup_dir="$temp_dir/failure-cleanup"; mkdir -p "$failure_cleanup_dir"
if (
  evidence_source_commit=''; retain_prep_lock=0; isolation_roots=(); sudo_keepalive_pid=''; temp_dir="$failure_cleanup_dir"
  trap 'cleanup $?' EXIT; false
); then fail 'cleanup erased a failing release-prep status'; fi
[[ ! -e "$failure_cleanup_dir" ]] || fail 'failing cleanup left temporary state'
[[ "$(<"$state/write-count")" == '0' ]] || fail 'read-only checks mutated release state'
printf 'release prep environment test: PASS\n'

#!/usr/bin/env bash
set -euo pipefail
umask 077

fail() { printf 'companion release prep: %s\n' "$1" >&2; exit 1; }
usage() {
  cat >&2 <<'USAGE'
usage: prepare-release.sh --endpoint URL --credential-locator ENV --provider NAME --model NAME --model-context-window N --omp PATH --oracle-policy-digest sha256:HEX --tag-signing-key PATH --promotion-signing-key PATH [--inherit-parent-sandbox] (--preflight|--apply)
USAGE
  exit 64
}
readonly repository='autopus-ai/autopus-adk'
readonly environment_name='adk-companion-release'
readonly release_tag='v0.50.118'
readonly spec_id='SPEC-OMP-004'
readonly expected_go_toolchain='go1.26.6'
readonly expected_omp_sha256='cd2f47545cb3f8eb5e15c91bc9054d73967774652e020b432e294803d1b71ea0'
readonly expected_promotion_key_id='omp-context-promotion-2026-q3-k3'
readonly release_ref="refs/tags/${release_tag}"
readonly evidence_tag="omp-context-evidence-${release_tag}"
readonly evidence_ref="refs/tags/${evidence_tag}"
readonly evidence_source_ref="refs/heads/${evidence_tag}-source"
endpoint='' credential_locator='' provider='' model='' model_context_window=''
omp_executable='' oracle_policy_digest='' tag_signing_key='' promotion_signing_key=''
operation='' inherit_parent_sandbox=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --endpoint) [[ $# -ge 2 ]] || usage; endpoint=$2; shift 2 ;;
    --credential-locator) [[ $# -ge 2 ]] || usage; credential_locator=$2; shift 2 ;;
    --provider) [[ $# -ge 2 ]] || usage; provider=$2; shift 2 ;;
    --model) [[ $# -ge 2 ]] || usage; model=$2; shift 2 ;;
    --model-context-window) [[ $# -ge 2 ]] || usage; model_context_window=$2; shift 2 ;;
    --omp) [[ $# -ge 2 ]] || usage; omp_executable=$2; shift 2 ;;
    --oracle-policy-digest) [[ $# -ge 2 ]] || usage; oracle_policy_digest=$2; shift 2 ;;
    --tag-signing-key) [[ $# -ge 2 ]] || usage; tag_signing_key=$2; shift 2 ;;
    --promotion-signing-key) [[ $# -ge 2 ]] || usage; promotion_signing_key=$2; shift 2 ;;
    --inherit-parent-sandbox) inherit_parent_sandbox=1; shift ;;
    --preflight) [[ -z "$operation" ]] || usage; operation=preflight; shift ;;
    --apply) [[ -z "$operation" ]] || usage; operation=apply; shift ;;
    *) usage ;;
  esac
done
[[ "$operation" == 'preflight' || "$operation" == 'apply' ]] || usage
[[ "$credential_locator" =~ ^AUTOPUS_OMP_CONTEXT_PROVIDER_[A-Z0-9_]{1,96}$ ]] || fail 'credential locator is malformed'
provider_credential=${!credential_locator-}
[[ -n "$provider_credential" && "$provider_credential" != *$'\n'* ]] ||
  fail 'provider credential is unavailable or not single-line'
export -n provider_credential
unset "$credential_locator"
readonly provider_credential
[[ "$(uname -s)" == 'Darwin' && "$(uname -m)" == 'arm64' ]] || fail 'release prep requires Darwin arm64'
[[ "$endpoint" =~ ^http://127\.0\.0\.1:[1-9][0-9]{0,4}$ ]] || fail 'endpoint is not exact loopback HTTP'
[[ "$provider" =~ ^[A-Za-z0-9_.-]+$ && "$model" =~ ^[A-Za-z0-9_.:/-]+$ ]] || fail 'provider or model is malformed'
[[ "$model_context_window" =~ ^[0-9]+$ && "$model_context_window" -ge 8192 ]] || fail 'model context window is invalid'
[[ "$oracle_policy_digest" =~ ^sha256:[0-9a-f]{64}$ ]] || fail 'oracle policy digest is malformed'
for tool in awk cmp cp gh git go install jq mktemp sed shasum ssh-keygen sudo tar tr wc; do
  command -v "$tool" >/dev/null || fail "$tool is unavailable"
done
readonly runner_uid=$(/usr/bin/id -u)
readonly runner_gid=$(/usr/bin/id -g)
readonly private_tmp_identity=$(/usr/bin/stat -f '%d:%i' /private/tmp)
readonly operator_actor_id=204883817
if [[ "$operation" == 'apply' ]]; then
  /usr/bin/sudo -n -v || fail 'noninteractive root authorization is unavailable'
fi
[[ -f "$omp_executable" && ! -L "$omp_executable" && -x "$omp_executable" ]] || fail 'OMP executable is unsafe'
omp_executable=$(cd -- "$(dirname -- "$omp_executable")" && pwd)/$(basename -- "$omp_executable")
for key_name in tag_signing_key promotion_signing_key; do
  key_path=${!key_name}
  [[ -f "$key_path" && ! -L "$key_path" ]] || fail "${key_name} is unsafe"
  key_path=$(cd -- "$(dirname -- "$key_path")" && pwd)/$(basename -- "$key_path")
  [[ "$(/usr/bin/stat -f '%u:%Lp' "$key_path")" == "$(id -u):600" ]] ||
    fail "${key_name} ownership or mode is unsafe"
  printf -v "$key_name" '%s' "$key_path"
done
repo_root=$(git rev-parse --show-toplevel)
[[ "$(pwd -P)" == "$repo_root" ]] || fail 'release prep must run at the repository root'
readonly repo_root
assert_source_identity() {
  [[ -z "$(git status --porcelain)" ]] || fail 'source worktree is not clean'
  [[ "$(git rev-parse --verify 'HEAD^{commit}')" == "$source_commit" ]] || fail 'source commit changed during release prep'
  [[ "$(git rev-parse --verify 'HEAD^{tree}')" == "$source_tree" ]] || fail 'source tree changed during release prep'
  [[ "$(git remote get-url origin)" =~ ^(https://github\.com/|git@github\.com:)(autopus-ai|Autopus-AI)/autopus-adk(\.git)?$ ]] ||
    fail 'origin is not the production repository'
}
[[ -z "$(git status --porcelain)" ]] || fail 'source worktree is not clean'
source_commit=$(git rev-parse --verify 'HEAD^{commit}')
source_tree=$(git rev-parse --verify 'HEAD^{tree}')
readonly source_commit source_tree
[[ "$source_commit" =~ ^[0-9a-f]{40}$ && "$source_tree" =~ ^[0-9a-f]{40}$ ]] || fail 'source coordinates are malformed'
assert_source_identity
git fetch --no-tags origin main
[[ "$(git rev-parse --verify origin/main)" == "$source_commit" ]] || fail 'source is not exact origin/main'
[[ "$(gh api "repos/${repository}" --jq .default_branch)" == 'main' ]] || fail 'default branch differs'
assert_source_identity
readonly tag_public_key_file="$repo_root/scripts/companion-release/release-tag-signing-2026-q3-r2.pub"
readonly tag_fingerprint_file="$repo_root/scripts/companion-release/release-tag-signing-2026-q3-r2.fingerprint"
[[ -f "$tag_public_key_file" && ! -L "$tag_public_key_file" &&
   -f "$tag_fingerprint_file" && ! -L "$tag_fingerprint_file" ]] || fail 'R2 release tag signer pins are unavailable'
bootstrap_cleanup() { rm -rf -- "$temp_dir"; }
temp_dir=$(mktemp -d "${TMPDIR:-/tmp}/companion-release-prep.XXXXXX")
readonly temp_dir
chmod 0700 "$temp_dir"
trap bootstrap_cleanup EXIT
staged_tag_signing_key="$temp_dir/release-tag-signing-key"
staged_promotion_signing_key="$temp_dir/omp-context-promotion-signing-key"
/usr/bin/install -m 0600 "$tag_signing_key" "$staged_tag_signing_key"
/usr/bin/install -m 0400 "$promotion_signing_key" "$staged_promotion_signing_key"
[[ "$(/usr/bin/stat -f '%u:%Lp' "$staged_tag_signing_key")" == "$(id -u):600" &&
   "$(/usr/bin/stat -f '%u:%Lp' "$staged_promotion_signing_key")" == "$(id -u):400" ]] ||
  fail 'staged release signing key ownership or mode is unsafe'
tag_signing_key=$staged_tag_signing_key
promotion_signing_key=$staged_promotion_signing_key
evidence_source_commit=''; retain_prep_lock=0; prep_lock_mode='fresh'; isolation_roots=()
sudo_keepalive_pid=''; live_canary_started=0
release_canary_user=''; release_canary_uid=''; release_canary_gid=''; release_canary_home=''
release_canary_marker=''; release_canary_attempt=''; release_canary_next_attempt=1
release_canary_account_created=0
for runtime_lib_name in prepare-release-user-lib.sh prepare-release-runtime-lib.sh prepare-release-local-lib.sh \
  prepare-release-probe-lib.sh; do
  staged_runtime_lib="$temp_dir/$runtime_lib_name"
  runtime_lib_blob=$(git rev-parse --verify "${source_commit}:scripts/companion-release/${runtime_lib_name}") ||
    fail "release prep runtime helper ${runtime_lib_name} is absent from the exact source"
  [[ "$(git cat-file -t "$runtime_lib_blob")" == 'blob' ]] || fail "release prep runtime helper ${runtime_lib_name} is not a source blob"
  git cat-file blob "$runtime_lib_blob" >"$staged_runtime_lib"
  chmod 0400 "$staged_runtime_lib"
  [[ "$(git hash-object "$staged_runtime_lib")" == "$runtime_lib_blob" ]] || fail "staged release prep runtime helper ${runtime_lib_name} differs from the exact source"
  exec 9<"$staged_runtime_lib"; rm -f -- "$staged_runtime_lib"
  # shellcheck source=/dev/null
  source /dev/fd/9
  exec 9<&-
done
trap 'cleanup $?' EXIT
probe_configure
staged_omp="$temp_dir/omp-v17.2.7"
readonly staged_omp
cp "$omp_executable" "$staged_omp"; chmod 0500 "$staged_omp"
[[ "$(shasum -a 256 "$staged_omp" | awk '{print $1}')" == "$expected_omp_sha256" ]] || fail 'staged OMP executable digest differs'
[[ "$("$staged_omp" --version)" == 'omp/17.2.7' ]] || fail 'verified OMP version differs from v17.2.7'
omp_executable=$staged_omp
probe_verify_tag_signing_authority
environment_variables=$(gh variable list --repo "$repository" --env "$environment_name" --json name,value) ||
  fail 'protected environment variables are unavailable'
readonly environment_variables
jq -e 'type == "array" and all(.[]; (.name | type) == "string" and (.value | type) == "string") and
  (([.[].name] | length) == ([.[].name] | unique | length))' <<<"$environment_variables" >/dev/null ||
  fail 'protected environment variable inventory is malformed'
lineage_key_id=$(matched_variable ADK_COMPANION_KEY_ID)
lineage_handoff=$(matched_variable ADK_COMPANION_HANDOFF)
rollback_floor=$(matched_variable ADK_COMPANION_ROLLBACK_FLOOR)
[[ "$lineage_key_id" =~ ^[A-Za-z0-9_.-]+$ && "$lineage_handoff" =~ ^[A-Za-z0-9_.-]+$ && "$rollback_floor" =~ ^[1-9][0-9]*$ ]] ||
  fail 'release lineage policy is unavailable'
authenticated_actor_id=$(gh api user --jq .id) || fail 'cannot authenticate release operator'
[[ "$authenticated_actor_id" == "$operator_actor_id" ]] || fail 'authenticated GitHub actor is not the release operator'
release_remote=$(git ls-remote --refs origin "$release_ref") || fail 'cannot inspect release ref'
evidence_remote=$(git ls-remote --refs origin "$evidence_ref") || fail 'cannot inspect evidence ref'
lock_remote=$(git ls-remote --refs origin "$evidence_source_ref") || fail 'cannot inspect prep lock ref'
retained_lock_commit=''
if [[ -n "$lock_remote" ]]; then
  retained_lock_commit=${lock_remote%%$'\t'*}
  [[ "$retained_lock_commit" =~ ^[0-9a-f]{40}$ && "$lock_remote" == "$retained_lock_commit"$'\t'"$evidence_source_ref" ]] ||
    fail 'release-prep compare-and-swap lock is malformed'
fi
[[ -z "$release_remote" || -n "$evidence_remote" ]] || fail 'release tag exists without immutable evidence'
release_present=0; evidence_present=0
[[ -n "$release_remote" ]] && release_present=1
[[ -n "$evidence_remote" ]] && evidence_present=1
if [[ "$release_present" -eq 0 ]]; then
  scripts/companion-release/verify-release-tag-ruleset.sh --armed ||
    fail 'exact armed v0.50.118 tag ruleset or environment is unavailable'
  ruleset_state=armed
elif scripts/companion-release/verify-release-tag-ruleset.sh --sealed; then
  ruleset_state=sealed
elif scripts/companion-release/verify-release-tag-ruleset.sh --armed; then
  ruleset_state=armed-reconciliation-required
else
  fail 'committed v0.50.118 tag ruleset is neither exact sealed nor reconcilable armed state'
fi
releases=$(gh api --paginate --slurp "repos/${repository}/releases?per_page=100") || fail 'cannot inspect GitHub Release state'
release_count=$(jq '[.[][] | select(.tag_name == "v0.50.118")] | length' <<<"$releases") || fail 'GitHub Release state is malformed'
[[ "$release_count" == '0' || "$release_count" == '1' ]] || fail 'GitHub Release state is ambiguous'
[[ "$release_count" == '0' || "$release_present" -eq 1 || -n "$retained_lock_commit" ]] ||
  fail 'GitHub Release exists without its source tag or retained prep lock'
input_jsonl="$temp_dir/observe-session-input.jsonl"; policy_tool="$temp_dir/auto-policy"
verifier="$temp_dir/ompcontextverify"; execsmoke="$temp_dir/execsmoke"; uidrunner="$temp_dir/uidrunner"
final_candidate="$temp_dir/auto-final"; static_policy_file="$temp_dir/static-policy.b64"
final_static_policy_file="$temp_dir/final-static-policy.b64"
readonly input_jsonl policy_tool verifier execsmoke uidrunner final_candidate static_policy_file final_static_policy_file
producer_run_id=$(date -u '+%Y%m%d%H%M%S')
producer_workflow_ref="local-release-prep@${source_commit}"
dispatch_nonce=$(printf '%s' "${source_commit}:${producer_run_id}:$$:${temp_dir}" | shasum -a 256 | awk '{print substr($1,1,32)}')
[[ "$dispatch_nonce" =~ ^[0-9a-f]{32}$ ]] || fail 'dispatch nonce is malformed'
challenge_digest="sha256:$(printf '%s' "${release_tag}:${source_commit}:${source_tree}" | shasum -a 256 | awk '{print $1}')"
env GOENV=off GOTOOLCHAIN="$expected_go_toolchain" go mod tidy -diff
env GOENV=off GOTOOLCHAIN="$expected_go_toolchain" go build -trimpath -o "$policy_tool" ./cmd/auto
env GOENV=off GOTOOLCHAIN="$expected_go_toolchain" go build -trimpath -o "$verifier" ./scripts/companion-release/ompcontextverify
[[ "$("$policy_tool" companion-manifest omp-context-promotion-key-id <"$promotion_signing_key")" == "$expected_promotion_key_id" ]] ||
  fail 'promotion signing key differs from K3 policy authority'
scripts/companion-release/verify-homebrew-tap-pins.sh ||
  fail 'Homebrew tap predecessor pins do not match the live tap'
create_canary_plan "$static_policy_file"
IFS= read -r static_policy_b64 <"$static_policy_file"
build_candidate "$static_policy_b64" "$final_candidate"
candidate_sha256=$(shasum -a 256 "$final_candidate" | awk '{print $1}')
if [[ "$evidence_present" -eq 1 && "$probe_enabled" -eq 0 ]]; then
  load_evidence
  derive_policy "$verified_report" "$final_static_policy_file"
  cmp "$static_policy_file" "$final_static_policy_file" || fail 'verified evidence static policy differs from the current A24 plan'
  evidence_mode=active; [[ "$release_present" -eq 1 ]] && evidence_mode=historical
  verify_evidence "$evidence_mode"
fi
if [[ "$operation" == 'preflight' ]]; then
  static_policy_sha256=$(printf '%s' "$static_policy_b64" | shasum -a 256 | awk '{print $1}')
  jq -cn --arg release_tag "$release_tag" --arg source_commit "$source_commit" --arg source_tree "$source_tree" \
    --arg candidate_sha256 "$candidate_sha256" --arg static_policy_sha256 "$static_policy_sha256" \
    --arg promotion_key_id "$expected_promotion_key_id" --arg ruleset_state "$ruleset_state" \
    --argjson evidence_present "$evidence_present" --argjson release_present "$release_present" \
    --argjson github_release_count "$release_count" \
    --argjson prep_lock_present "$([[ -n "$retained_lock_commit" ]] && printf 1 || printf 0)" \
    '{mode:"preflight",release_tag:$release_tag,source_commit:$source_commit,source_tree:$source_tree,
      candidate_artifact_sha256:$candidate_sha256,static_policy_sha256:$static_policy_sha256,
      promotion_signing_key_id:$promotion_key_id,ruleset_state:$ruleset_state,
      evidence_present:($evidence_present == 1),release_tag_present:($release_present == 1),
      github_release_present:($github_release_count == 1),prep_lock_present:($prep_lock_present == 1),
      canary_records:42,provider_calls:40,task_pairs:20,remote_mutations:0}'
  exit 0
fi
if [[ "$evidence_present" -eq 1 && "$probe_enabled" -eq 0 ]]; then
  if [[ "$release_present" -eq 1 ]]; then publish_coordinates reconcile; exit 0; fi
  ensure_prep_lock "$verified_report"; publish_coordinates "$evidence_source_commit"; exit 0
fi
env GOENV=off GOTOOLCHAIN="$expected_go_toolchain" go build -trimpath -o "$execsmoke" ./scripts/companion-release/execsmoke
env GOENV=off GOTOOLCHAIN="$expected_go_toolchain" go build -trimpath -o "$uidrunner" ./scripts/companion-release/uidrunner
probe_build_export_tool
(
  while /bin/sleep 30; do /usr/bin/sudo -n -v || exit 1; done
) &
sudo_keepalive_pid=$!
final_project="$temp_dir/final-project"; final_output="$temp_dir/final-output.jsonl"
extract_project "$final_project"
run_canary "$final_candidate" "$final_project" "$final_output" final
probe_terminate
validate_canary "$final_project" "$final_output" "$final_candidate"
final_report="$final_project/.autopus/runtime/omp-context/promotion-report-v1.json"
derive_policy "$final_report" "$final_static_policy_file"
cmp "$static_policy_file" "$final_static_policy_file" || fail 'planned static policy changed after final canary'
git fetch --no-tags origin main
[[ "$(git rev-parse --verify origin/main)" == "$source_commit" ]] || fail 'origin/main advanced during release prep'
assert_source_identity
publish_local_evidence "$final_report"
publish_coordinates "$evidence_source_commit"

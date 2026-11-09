#!/usr/bin/env bash
set -euo pipefail
umask 077

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_dir=$(cd -- "$tests_dir/.." && pwd)
repo=$(cd -- "$script_dir/../.." && pwd)
fail() { printf 'release runtime hardening test: %s\n' "$1" >&2; exit 1; }
contains() { grep -Fq -- "$2" "$1" || fail "$1 missing $2"; }

source_gate="$script_dir/validate-source.sh"
environment_gate="$script_dir/validate-environment.sh"
lineage_archive="$script_dir/verify-public-key-lineage-archive.sh"

temp=$(mktemp -d "${TMPDIR:-/tmp}/release-hardening-test.XXXXXX")
trap 'rm -rf -- "$temp"' EXIT
runtime_lib="$script_dir/prepare-release-runtime-lib.sh"
user_lib="$script_dir/prepare-release-user-lib.sh"
source "$runtime_lib"
contains "$runtime_lib" 'sandbox_args=(--omp "$isolated_omp")'
contains "$runtime_lib" '/bin/cat "$input_jsonl"'
contains "$runtime_lib" 'canary_helper prepare-root "$dispatch_nonce"'
contains "$runtime_lib" 'select_release_canary_account "$isolated_home"'
contains "$runtime_lib" 'verify_release_canary_account'
contains "$runtime_lib" 'canary_helper run-canary "$dispatch_nonce" "$release_canary_attempt" --'
contains "$runtime_lib" 'cleanup_live_canary_uid'
contains "$user_lib" 'canary_helper account-kill "$dispatch_nonce" "$release_canary_attempt" TERM'
contains "$runtime_lib" 'canary_helper chown "$dispatch_nonce" nobody'
contains "$runtime_lib" 'remove_release_canary_account'
contains "$runtime_lib" '"${sandbox_args[@]}" | capture_canary_progress'
contains "$user_lib" 'release_canary_next_attempt=$((attempt + 1))'
contains "$user_lib" 'discard_release_canary_account'
contains "$user_lib" 'release_canary_stabilize_seconds=15'
! grep -Fq -- 'live_canary_uid=59999' "$runtime_lib" ||
  fail "$runtime_lib retains the fixed canary UID"
for file in "$runtime_lib" "$user_lib"; do
  ! grep -Fq -- 'sudo -n -u' "$file" || fail "$file opens a sudo user session"
done
printf '%s\n' \
  '{"schema_version":"autopus.omp_context_observe_session_response.v1","type":"handshake"}' \
  '{"schema_version":"autopus.omp_context_observe_session_response.v1","type":"error","error_code":"network_transport","error_stage":"call","failed_sequence":17}' \
  >"$temp/canary-input.jsonl"
canary_progress=$(capture_canary_progress "$temp/canary-output.jsonl" final \
  <"$temp/canary-input.jsonl" 2>&1)
cmp "$temp/canary-input.jsonl" "$temp/canary-output.jsonl" ||
  fail 'canary progress capture changed transcript bytes'
[[ "$canary_progress" == *'final production canary progress (1/42 records)'* &&
   "$canary_progress" == *'final production canary progress (2/42 records)'* ]] ||
  fail 'canary progress did not report each completed record'
failure_status=0
failure_receipt=$(canary_failure_receipt final 42 "$temp/canary-output.jsonl" 2>&1) ||
  failure_status=$?
[[ "$failure_status" -eq 42 ]] || fail 'canary failure receipt did not preserve exit status'
[[ "$failure_receipt" == *'final production canary execution failed: exit=42 transcript_records=2/42 error_code=network_transport error_stage=call failed_sequence=17'* ]] ||
  fail 'canary failure receipt omitted structured diagnostics'
[[ "$failure_receipt" != *'gate verdict'* ]] || fail 'canary failure receipt invented a gate verdict'
printf '%s\n' \
  '{"schema_version":"autopus.omp_context_observe_session_response.v1","type":"error","error_code":"cohort_gates_failed","error_stage":"evidence","gate_diagnostic":"pairs=20/20 compactions=2/2 median_reduction_bp=0/2000"}' \
  >"$temp/canary-gate.jsonl"
gate_receipt=$(canary_failure_receipt final 1 "$temp/canary-gate.jsonl" 2>&1) || true
[[ "$gate_receipt" == *'error_code=cohort_gates_failed error_stage=evidence'* &&
   "$gate_receipt" == *'final production canary gate verdict: pairs=20/20 compactions=2/2 median_reduction_bp=0/2000'* ]] ||
  fail 'canary failure receipt omitted the body-free gate verdict'
printf '%s\n' \
  '{"schema_version":"autopus.omp_context_observe_session_response.v1","type":"error","error_code":"cohort_gates_failed","error_stage":"evidence","gate_diagnostic":"assistant said: DROP TABLE"}' \
  >"$temp/canary-gate-bad.jsonl"
bad_receipt=$(canary_failure_receipt final 1 "$temp/canary-gate-bad.jsonl" 2>&1) || true
[[ "$bad_receipt" != *'gate verdict'* ]] || fail 'canary failure receipt echoed a non-numeric diagnostic'
git clone -q --no-hardlinks --no-tags "$repo" "$temp/source"
git -C "$temp/source" config user.name 'Release Test'
git -C "$temp/source" config user.email release-test@example.invalid
git -C "$temp/source" tag -am 'A23 fixture' v0.50.111
commit=$(git -C "$temp/source" rev-parse HEAD)
tree=$(git -C "$temp/source" rev-parse 'HEAD^{tree}')
if [[ "${tree: -1}" == '0' ]]; then
  wrong_tree="${tree%?}1"
else
  wrong_tree="${tree%?}0"
fi
run_source_gate() {
  local approved_commit="${1-}" approved_tree="${2-}"
  env GITHUB_REF_NAME=v0.50.111 GITHUB_REF_TYPE=tag GITHUB_SHA="$commit" \
    GITHUB_OUTPUT="$temp/source-output" COMPANION_SOURCE_PIN_REQUIRED=1 \
    COMPANION_APPROVED_SOURCE_COMMIT="$approved_commit" \
    COMPANION_APPROVED_SOURCE_TREE="$approved_tree" \
    bash "$source_gate"
}
if (cd "$temp/source" && run_source_gate '' '') >/dev/null 2>&1; then
  fail 'missing approved source pins passed'
fi
if (cd "$temp/source" && run_source_gate "$commit" "$wrong_tree") >/dev/null 2>&1; then
  fail 'wrong approved source tree passed'
fi
(cd "$temp/source" && run_source_gate "$commit" "$tree") >/dev/null

# Current-time checks must reject expired production material.
touch "$temp/key" "$temp/api-key"
chmod 0600 "$temp/key" "$temp/api-key"
printf '#!/usr/bin/env bash\nexit 0\n' >"$temp/tool"
chmod 0700 "$temp/tool"
format_time() {
  if date -u -r "$1" '+%Y-%m-%dT%H:%M:%SZ' >/dev/null 2>&1; then
    date -u -r "$1" '+%Y-%m-%dT%H:%M:%SZ'
  else
    date -u -d "@$1" '+%Y-%m-%dT%H:%M:%SZ'
  fi
}
now=$(date -u '+%s')
validation_env=(
  COMPANION_BUILD_PROVENANCE=github-actions:fixture@0123456789abcdef
  COMPANION_HANDOFF=v1 COMPANION_ROLLBACK_FLOOR=5069
  COMPANION_KEY_ID=adk-release-2026-q3-b0 COMPANION_SIGNING_KEY_FILE="$temp/key"
  COMPANION_SIGNER="$temp/tool" COMPANION_RECEIPT_VERIFIER="$temp/tool"
  COMPANION_EXEC_SMOKE_GATE="$temp/tool"
  COMPANION_MANIFEST_VERIFIER="$temp/tool" COMPANION_RELEASE_TIME_VALIDATION_REQUIRED=1
  COMPANION_PUBLIC_KEY_RECEIPT_MINIMUM_LIFETIME_SECONDS=31536000
  COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT="$(format_time "$((now - 86400))")"
  COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT="$(format_time "$((now + 31536001))")"
  APPLE_SIGNING_IDENTITY='Developer ID Application: Fixture (GP2PFA2PUV)'
  APPLE_API_KEY=FIXTURE APPLE_API_ISSUER=123e4567-e89b-42d3-a456-426614174000
  APPLE_API_KEY_PATH="$temp/api-key"
)
env "${validation_env[@]}" COMPANION_ISSUED_AT="$(format_time "$((now - 60))")" \
  COMPANION_EXPIRES_AT="$(format_time "$((now + 3600))")" bash "$environment_gate"
ln -s -- "$temp/tool" "$temp/exec-smoke-link"
if env "${validation_env[@]}" COMPANION_EXEC_SMOKE_GATE="$temp/exec-smoke-link" \
  COMPANION_ISSUED_AT="$(format_time "$((now - 60))")" \
  COMPANION_EXPIRES_AT="$(format_time "$((now + 3600))")" \
  bash "$environment_gate" >/dev/null 2>&1; then
  fail 'symlinked execution smoke gate passed'
fi
if env "${validation_env[@]}" COMPANION_EXEC_SMOKE_GATE="$temp/key" \
  COMPANION_ISSUED_AT="$(format_time "$((now - 60))")" \
  COMPANION_EXPIRES_AT="$(format_time "$((now + 3600))")" \
  bash "$environment_gate" >/dev/null 2>&1; then
  fail 'non-executable execution smoke gate passed'
fi
if env "${validation_env[@]}" COMPANION_ISSUED_AT="$(format_time "$((now - 7200))")" \
  COMPANION_EXPIRES_AT="$(format_time "$((now - 3600))")" bash "$environment_gate" >/dev/null 2>&1; then
  fail 'expired manifest window passed'
fi

# The independent verifier must bind receipt key -> manifest signature -> artifact.
verifier="$temp/manifest-verifier"
go build -trimpath -o "$verifier" "$repo/scripts/companion-release/manifestverify"
go run "$tests_dir/testdata/generate-manifest-fixture.go" "$temp/manifest"
trusted_key_sha=$(jq -er '.public_key_sha256' "$temp/manifest/public-key-receipt.json")
install -m 0600 "$temp/manifest/signing-key" "$temp/trusted-signing-key"
common_verify_args=(
  --artifact "$temp/manifest/auto" --manifest "$temp/manifest/adk-companion-manifest.json"
  --signature "$temp/manifest/adk-companion-manifest.sig"
  --receipt "$temp/manifest/public-key-receipt.json"
  --receipt-signature "$temp/manifest/public-key-receipt.sig"
  --key-id adk-release-2026-q3-b0 --version 0.50.71 --platform darwin
  --architecture arm64 --handoff v1 --minimum-rollback-floor 5069
)
signing_verify_args=("${common_verify_args[@]}" --signing-key "$temp/trusted-signing-key")
pin_verify_args=("${common_verify_args[@]}" --public-key-sha256 "$trusted_key_sha")
"$verifier" "${signing_verify_args[@]}"
"$verifier" "${pin_verify_args[@]}"
if "$verifier" "${common_verify_args[@]}" >/dev/null 2>&1; then
  fail 'unanchored self-signed receipt passed'
fi
if "$verifier" "${signing_verify_args[@]}" --public-key-sha256 "$trusted_key_sha" \
  >/dev/null 2>&1; then
  fail 'ambiguous duplicate trust anchors passed'
fi
if "$verifier" "${common_verify_args[@]}" \
  --signing-key "$temp/manifest/inconsistent-signing-key" >/dev/null 2>&1; then
  fail 'inconsistent private/public signing key passed'
fi
printf 'tamper\n' >>"$temp/manifest/auto"
if "$verifier" "${signing_verify_args[@]}" >/dev/null 2>&1; then fail 'tampered artifact passed'; fi
printf 'signed companion artifact\n' >"$temp/manifest/auto"
manifest_signature="$temp/manifest/adk-companion-manifest.sig"
signature_size_before=$(wc -c <"$manifest_signature")
(( signature_size_before == 64 )) || fail 'fixture manifest signature length drifted'
signature_byte_before=$(od -An -tu1 -N1 "$manifest_signature" | tr -d '[:space:]')
case "$signature_byte_before" in
  1) signature_byte_replacement=2 ;;
  ''|*[!0-9]*) fail 'fixture manifest signature byte could not be read' ;;
  *) signature_byte_replacement=1 ;;
esac
if (( signature_byte_replacement == 1 )); then printf '\001'; else printf '\002'; fi |
  dd of="$manifest_signature" bs=1 seek=0 conv=notrunc 2>/dev/null
signature_size_after=$(wc -c <"$manifest_signature")
(( signature_size_after == signature_size_before )) || fail 'manifest signature tamper changed length'
signature_byte_after=$(od -An -tu1 -N1 "$manifest_signature" | tr -d '[:space:]')
[[ "$signature_byte_after" == "$signature_byte_replacement" ]] ||
  fail 'manifest signature tamper did not write the replacement byte'
[[ "$signature_byte_after" != "$signature_byte_before" ]] ||
  fail 'manifest signature tamper was a no-op'
if "$verifier" "${signing_verify_args[@]}" >/dev/null 2>&1; then fail 'tampered manifest signature passed'; fi
go run "$tests_dir/testdata/generate-manifest-fixture.go" "$temp/manifest" \
  'attacker replacement artifact'
if "$verifier" "${signing_verify_args[@]}" >/dev/null 2>&1; then
  fail 'replacement receipt, manifest, and artifact pair passed the signing-key anchor'
fi
if "$verifier" "${pin_verify_args[@]}" >/dev/null 2>&1; then
  fail 'replacement receipt, manifest, and artifact pair passed the A0 key pin'
fi
contains "$lineage_archive" 'MANIFEST_SIGNATURE_NAME'
contains "$lineage_archive" 'COMPANION_MANIFEST_VERIFIER'

printf 'release runtime hardening test: PASS\n'

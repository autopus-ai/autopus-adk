#!/usr/bin/env bash

# This file is sourced by produce.sh after its fail and sha256 helpers exist.
if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  exit 64
fi

prepare_omp_context_release_lineage() {
  local artifact_dir=$1 name
  omp_context_lineage_enabled=0
  lineage_path="$artifact_dir/release-lineage-v1.json"
  lineage_signature_path="$artifact_dir/release-lineage-v1.sig"
  # produce.sh는 darwin 이외 플랫폼에서 이미 종료하므로 이 검사는 darwin/arm64 하나만 남긴다.
  # 좌표별 무장은 없다 — lineage 쌍은 모든 darwin/arm64 릴리즈가 반드시 내보내야 하는 자산이고,
  # .goreleaser.yaml의 release.extra_files가 이 두 경로를 무조건 glob하므로 누락은 곧 릴리즈 실패다.
  if [[ "$COMPANION_ARCHITECTURE" != 'arm64' ]]; then
    return 0
  fi
  omp_context_lineage_enabled=1
  for name in OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256 COMPANION_SOURCE_COMMIT COMPANION_SOURCE_TREE; do
    require_environment "$name"
  done
  [[ "$OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256" =~ ^[0-9a-f]{64}$ ]] \
    || fail 'OMP context upstream candidate digest is malformed'
  [[ "$COMPANION_SOURCE_COMMIT" =~ ^[0-9a-f]{40}$ && "$COMPANION_SOURCE_TREE" =~ ^[0-9a-f]{40}$ ]] \
    || fail 'OMP context release source coordinates are malformed'
  for name in "$lineage_path" "$lineage_signature_path"; do
    [[ ! -e "$name" && ! -L "$name" ]] \
      || fail 'OMP context release lineage output already exists'
  done
}

produce_omp_context_release_lineage() {
  local executable_digest=$1 signing_key_digest_before=$2
  [[ "$omp_context_lineage_enabled" == '1' ]] || return 0
  env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}" \
    "$COMPANION_SIGNER" companion-manifest omp-context-release-lineage \
    --key-id "$COMPANION_KEY_ID" \
    --upstream-sha256 "sha256:$OMP_CONTEXT_CANDIDATE_ARTIFACT_SHA256" \
    --executable-sha256 "$executable_digest" \
    --source-repository 'autopus-ai/autopus-adk' \
    --source-commit "$COMPANION_SOURCE_COMMIT" --source-tree "$COMPANION_SOURCE_TREE" \
    --target 'darwin-arm64' --version "$COMPANION_VERSION" \
    --lineage-output "$lineage_path" --signature-output "$lineage_signature_path" \
    <"$COMPANION_SIGNING_KEY_FILE" >/dev/null \
    || fail 'OMP context release lineage signing failed'
  [[ -f "$lineage_path" && ! -L "$lineage_path" &&
     -f "$lineage_signature_path" && ! -L "$lineage_signature_path" ]] \
    || fail 'OMP context release lineage pair is unsafe'
  [[ "$(wc -c <"$lineage_signature_path" | tr -d '[:space:]')" == '64' ]] \
    || fail 'OMP context release lineage signature is not raw Ed25519 bytes'
  [[ "$(sha256_file "$COMPANION_SIGNING_KEY_FILE")" == "$signing_key_digest_before" ]] \
    || fail 'companion signing key changed while producing OMP context lineage'
}

cleanup_omp_context_release_lineage() {
  [[ "$omp_context_lineage_enabled" == '1' ]] || return 0
  rm -f -- "$lineage_path" "$lineage_signature_path"
}

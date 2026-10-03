#!/usr/bin/env bash

[[ "${BASH_SOURCE[0]}" != "$0" ]] || {
  printf 'companion release: public key receipt helper must be sourced\n' >&2
  exit 1
}

# @AX:ANCHOR [AUTO]: Keep every shipped tag/version pair in one exact receipt-phase resolver.
# @AX:REASON [AUTO]: The producer must reject mixed or unknown release coordinates before signing artifacts.
# 릴리즈 좌표의 단일 원본이다. 각 행은 "<tag> <version> <phase>"이며, 새 릴리즈는 여기에 한 행만 더한다.
# 세 열은 한 행 안에서만 짝지어지므로, tag와 version이 섞인 조합은 어떤 행과도 일치하지 않는다.
readonly PUBLIC_KEY_RECEIPT_RELEASE_COORDINATES='
v0.50.69 0.50.69 A0
v0.50.70 0.50.70 A1
v0.50.71 0.50.71 A2
v0.50.72 0.50.72 A3
v0.50.73 0.50.73 A4
v0.50.74 0.50.74 A5
v0.50.77 0.50.77 A6
v0.50.78 0.50.78 A7
v0.50.79 0.50.79 A8
v0.50.80 0.50.80 A9
v0.50.81 0.50.81 A10
v0.50.82 0.50.82 A11
v0.50.83 0.50.83 A12
v0.50.84 0.50.84 A13
v0.50.85 0.50.85 A14
v0.50.86 0.50.86 A15
v0.50.87 0.50.87 A16
v0.50.88 0.50.88 A17
v0.50.89 0.50.89 A18
v0.50.90 0.50.90 A19
v0.50.91 0.50.91 A20
v0.50.92 0.50.92 A21
v0.50.109 0.50.109 A22
v0.50.111 0.50.111 A23
v0.50.113 0.50.113 A24
v0.50.114 0.50.114 A25
v0.50.115 0.50.115 A26
v0.50.116 0.50.116 A27
v0.50.117 0.50.117 A28
v0.50.118 0.50.118 A29
v0.50.119 0.50.119 A30
v0.50.120 0.50.120 A31
v0.50.121 0.50.121 A32
v0.50.122 0.50.122 A33
'

resolve_public_key_receipt_release_phase() {
  local coordinate_tag coordinate_version coordinate_phase
  release_phase=''
  while read -r coordinate_tag coordinate_version coordinate_phase; do
    [[ -n "$coordinate_tag" ]] || continue
    [[ "$GITHUB_REF_NAME" == "$coordinate_tag" && "$COMPANION_VERSION" == "$coordinate_version" ]] \
      || continue
    release_phase="$coordinate_phase"
    break
  done <<<"$PUBLIC_KEY_RECEIPT_RELEASE_COORDINATES"
  [[ -n "$release_phase" ]] || fail 'public_key_receipt_release_identity_mismatch'
}

produce_public_key_receipt_bundle() {
  local artifact_path=$1
  local manifest_path=$2
  local signature_path=$3
  local public_key_bundle_path=$4
  local signing_key_digest_before=$5
  local release_phase=$6
  local bundle_entry_count entry signing_key_digest_after
  local -a public_key_receipt_args

  public_key_receipt_args=(companion-manifest public-key-receipt
    --key-file "$COMPANION_SIGNING_KEY_FILE"
    --bundle-output "$public_key_bundle_path"
    --key-id "$COMPANION_KEY_ID"
    --issued-at "$COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT"
    --expires-at "$COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT"
    --handoff "$COMPANION_HANDOFF"
    --minimum-rollback-floor "$COMPANION_ROLLBACK_FLOOR")
  env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}" \
    "$COMPANION_SIGNER" "${public_key_receipt_args[@]}" >/dev/null \
    || fail 'public key receipt production failed'
  [[ -d "$public_key_bundle_path" && ! -L "$public_key_bundle_path" ]] \
    || fail 'public key receipt production failed'
  bundle_entry_count=$(find "$public_key_bundle_path" -mindepth 1 -maxdepth 1 -print | wc -l)
  [[ "${bundle_entry_count//[[:space:]]/}" == '2' ]] \
    || fail 'public key receipt production failed'
  for entry in public-key-receipt.json public-key-receipt.sig; do
    [[ -f "$public_key_bundle_path/$entry" && ! -L "$public_key_bundle_path/$entry" ]] \
      || fail 'public key receipt production failed'
  done
  [[ "$(wc -c <"$public_key_bundle_path/public-key-receipt.sig" | tr -d '[:space:]')" == '64' ]] \
    || fail 'public key receipt production failed'
  env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}" \
    "$COMPANION_RECEIPT_VERIFIER" \
    --receipt "$public_key_bundle_path/public-key-receipt.json" \
    --signature "$public_key_bundle_path/public-key-receipt.sig" \
    --signing-key "$COMPANION_SIGNING_KEY_FILE" \
    --key-id "$COMPANION_KEY_ID" \
    --issued-at "$COMPANION_PUBLIC_KEY_RECEIPT_ISSUED_AT" \
    --expires-at "$COMPANION_PUBLIC_KEY_RECEIPT_EXPIRES_AT" \
    --handoff "$COMPANION_HANDOFF" \
    --minimum-rollback-floor "$COMPANION_ROLLBACK_FLOOR" \
    || fail 'public key receipt independent verification failed'
  if [[ -n "${COMPANION_MANIFEST_VERIFIER-}" ]]; then
    env -i PATH="$PATH" HOME="${HOME-}" TMPDIR="${TMPDIR:-/tmp}" \
      "$COMPANION_MANIFEST_VERIFIER" \
      --artifact "$artifact_path" \
      --manifest "$manifest_path" \
      --signature "$signature_path" \
      --receipt "$public_key_bundle_path/public-key-receipt.json" \
      --receipt-signature "$public_key_bundle_path/public-key-receipt.sig" \
      --signing-key "$COMPANION_SIGNING_KEY_FILE" \
      --key-id "$COMPANION_KEY_ID" \
      --version "$COMPANION_VERSION" \
      --platform "$COMPANION_PLATFORM" \
      --architecture "$COMPANION_ARCHITECTURE" \
      --handoff "$COMPANION_HANDOFF" \
      --minimum-rollback-floor "$COMPANION_ROLLBACK_FLOOR" \
      || fail 'manifest and artifact independent verification failed'
  fi
  signing_key_digest_after=$(sha256_file "$COMPANION_SIGNING_KEY_FILE") \
    || fail 'manifest_public_key_digest_mismatch'
  [[ "$signing_key_digest_before" == "$signing_key_digest_after" ]] \
    || fail 'manifest_public_key_digest_mismatch'
  printf 'companion release: public key receipt phase %s produced\n' "$release_phase"
}

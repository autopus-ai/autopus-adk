#!/usr/bin/env bash
set -euo pipefail

readonly A2_A1_ANCESTOR_SHA='e25e8be02b55b9385f58919c30ad1ccf92179030'
readonly A2_MAIN_ANCESTOR_SHA='acb735cca0ef120cfed0d01863de09535310b5a3'
readonly A3_A2_ANCESTOR_SHA='7b5b52822b0cda75bf6c971f5f1c2a713881008c'
readonly A4_A3_ANCESTOR_SHA='ba5509b692a43dc8a70e0bd6173acb56166ed67f'
readonly A5_A4_ANCESTOR_SHA='334b297f05942accbecdfa15b54e38e005c82f2d'
readonly A6_A5_ANCESTOR_SHA='b27252cb1148192a8ae1a95195c50e5f221453a4'
readonly A7_A6_ANCESTOR_SHA='902f1acfa91f1d0a2ac9471d5cd79117031a2599'
readonly A8_A7_ANCESTOR_SHA='51de6030a69a8e36fcf7e5790ef157eff6fedf00'
readonly A9_A8_ANCESTOR_SHA='dd0c2759ed5435d4634011e349caad62ea3df414'
readonly A10_A9_ANCESTOR_SHA='c9c4f49d48022eb0c8d72ee7b520136a4f21f176'
readonly A11_A10_ANCESTOR_SHA='54536edc09c37a634532c2c9b51e62869d393db4'
readonly A12_A11_ANCESTOR_SHA='a8558ccc36e04125de6b8d84c7ffc9e8ddb5a2c9'
readonly A13_A12_ANCESTOR_SHA='e6367b5375cd4cdf09cb1515877bc57323521364'
readonly A14_A13_ANCESTOR_SHA='2b7aa046bdb7861113dfa57b30489c11715582e9'
readonly A15_A14_ANCESTOR_SHA='4b8eb62200d253b46e022670c482e2f716a992a3'
readonly A16_A15_ANCESTOR_SHA='0fc4f60dac8ff8afe69b680c8bf723bfbced4769'
readonly A17_A16_ANCESTOR_SHA='3e02c622af97f74873325ec65940c580e23c580a'
readonly A18_A17_ANCESTOR_SHA='2b062a5e348fbecc414abe9ba5c74c7dc79fe243'
readonly A19_A18_ANCESTOR_SHA='76f35d990e76511d169e239547d33bfedcea7948'
readonly A20_A19_ANCESTOR_SHA='5bc41dccc72f8244943fd9e862cba07a36bf09d3'
readonly A21_A20_ANCESTOR_SHA='7f44e4f143b2348c02553bab2209088c966f81ae'
readonly A22_A21_ANCESTOR_SHA='b86fab067599f457261287552c5a9dd86460d7f4'
readonly A23_A22_ANCESTOR_SHA='67f3def5d4a0a11aadd9e103389de6cc1cafc34e'
readonly A24_A23_ANCESTOR_SHA='954f60a77acb59fd4106537020693fdcadb3d640'
readonly A25_A24_ANCESTOR_SHA='bc2147a875b49e9fca75db4307455f83512837d6'
readonly A26_A25_ANCESTOR_SHA='a6d199fb5a7b27721026916fcd75dffb58a4e228'
readonly A27_A26_ANCESTOR_SHA='77ae668bf7e9eb8d0dae177d1c9b7e41a5d51ef6'
readonly A28_A27_ANCESTOR_SHA='fbe502c05f84d5eeb81b089b2344c47329ab4543'
readonly A29_A28_ANCESTOR_SHA='620e29a44d004cb199d5f1c22ae92878f9b6930e'
readonly A30_A29_ANCESTOR_SHA='4480c8d2f6c00c205ee838cd4bd20933bfff3597'
readonly A31_A30_ANCESTOR_SHA='279bc98635639a91e08285c5ffc649d8f4c7df26'
readonly A32_A31_ANCESTOR_SHA='b69a8450d6a1c916dfd6e86b254907b4b0c76b78'
readonly A33_A32_ANCESTOR_SHA='8299c8df85bc0e24bfb4d4fbbd71889dceb96986'

fail() {
  printf 'companion release source: %s\n' "$1" >&2
  exit 1
}

for name in GITHUB_REF_NAME GITHUB_REF_TYPE GITHUB_SHA GITHUB_OUTPUT; do
  [[ -n "${!name-}" ]] || fail "required environment variable ${name} is missing"
done

case "$GITHUB_REF_NAME" in
  v0.50.69) release_phase='A0' ;;
  v0.50.70) release_phase='A1' ;;
  v0.50.71) release_phase='A2' ;;
  v0.50.72) release_phase='A3' ;;
  v0.50.73) release_phase='A4' ;;
  v0.50.74) release_phase='A5' ;;
  v0.50.77) release_phase='A6' ;;
  v0.50.78) release_phase='A7' ;;
  v0.50.79) release_phase='A8' ;;
  v0.50.80) release_phase='A9' ;;
  v0.50.81) release_phase='A10' ;;
  v0.50.82) release_phase='A11' ;;
  v0.50.83) release_phase='A12' ;;
  v0.50.84) release_phase='A13' ;;
  v0.50.85) release_phase='A14' ;;
  v0.50.86) release_phase='A15' ;;
  v0.50.87) release_phase='A16' ;;
  v0.50.88) release_phase='A17' ;;
  v0.50.89) release_phase='A18' ;;
  v0.50.90) release_phase='A19' ;;
  v0.50.91) release_phase='A20' ;;
  v0.50.92) release_phase='A21' ;;
  v0.50.109) release_phase='A22' ;;
  v0.50.111) release_phase='A23' ;;
  v0.50.113) release_phase='A24' ;;
  v0.50.114) release_phase='A25' ;;
  v0.50.115) release_phase='A26' ;;
  v0.50.116) release_phase='A27' ;;
  v0.50.117) release_phase='A28' ;;
  v0.50.118) release_phase='A29' ;;
  v0.50.119) release_phase='A30' ;;
  v0.50.120) release_phase='A31' ;;
  v0.50.121) release_phase='A32' ;;
  v0.50.122) release_phase='A33' ;;
  *) fail 'release tag is outside the frozen A0/A1/A2/A3/A4/A5/A6/A7/A8/A9/A10/A11/A12/A13/A14/A15/A16/A17/A18/A19/A20/A21/A22/A23/A24/A25/A26/A27/A28/A29/A30/A31/A32/A33 policy' ;;
esac
[[ "$GITHUB_REF_TYPE" == 'tag' ]] || fail 'release ref is not a tag'
[[ "$GITHUB_SHA" =~ ^[0-9a-f]{40}$ ]] || fail 'source commit is not exact 40-hex'

head_commit=$(git rev-parse --verify 'HEAD^{commit}') \
  || fail 'cannot resolve checked-out source commit'
tag_commit=$(git rev-parse --verify "${GITHUB_REF_NAME}^{commit}") \
  || fail 'cannot resolve release tag commit'
[[ "$head_commit" == "$GITHUB_SHA" && "$tag_commit" == "$GITHUB_SHA" ]] \
  || fail 'checked-out source, tag, and release commit differ'
source_tree=$(git rev-parse --verify 'HEAD^{tree}') \
  || fail 'cannot resolve checked-out source tree'

if [[ "$release_phase" == 'A2' || "$release_phase" == 'A3' ||
      "$release_phase" == 'A4' || "$release_phase" == 'A5' ||
      "$release_phase" == 'A6' || "$release_phase" == 'A7' ||
      "$release_phase" == 'A8' || "$release_phase" == 'A9' ||
      "$release_phase" == 'A10' || "$release_phase" == 'A11' ||
      "$release_phase" == 'A12' || "$release_phase" == 'A13' ||
      "$release_phase" == 'A14' || "$release_phase" == 'A15' ||
      "$release_phase" == 'A16' || "$release_phase" == 'A17' ||
      "$release_phase" == 'A18' || "$release_phase" == 'A19' ||
      "$release_phase" == 'A20' || "$release_phase" == 'A21' ||
      "$release_phase" == 'A22' || "$release_phase" == 'A23' ||
      "$release_phase" == 'A24' || "$release_phase" == 'A25' ||
      "$release_phase" == 'A26' || "$release_phase" == 'A27' ||
      "$release_phase" == 'A28' || "$release_phase" == 'A29' ||
      "$release_phase" == 'A30' || "$release_phase" == 'A31' ||
      "$release_phase" == 'A32' || "$release_phase" == 'A33' ]]; then
  tag_object_type=$(git cat-file -t "refs/tags/$GITHUB_REF_NAME" 2>/dev/null) \
    || fail "cannot resolve exact ${release_phase} tag object"
  [[ "$tag_object_type" == 'tag' ]] \
    || fail "${release_phase} release tag must be annotated"
  tag_header_identity=$(git cat-file tag "refs/tags/$GITHUB_REF_NAME" | awk '
    NF == 0 { exit }
    $1 == "object" { objects++; object = $2 }
    $1 == "type" { types++; type = $2 }
    $1 == "tag" { tags++; tag = $2 }
    END { printf "%d:%s\t%d:%s\t%d:%s", objects, object, types, type, tags, tag }
  ') || fail "${release_phase} annotated tag headers are unavailable"
  expected_tag_headers="1:${GITHUB_SHA}"$'\t''1:commit'$'\t'"1:${GITHUB_REF_NAME}"
  [[ "$tag_header_identity" == "$expected_tag_headers" ]] ||
    fail "${release_phase} annotated tag object, type, or name headers differ"
  if [[ "$release_phase" == 'A2' ]]; then
    git merge-base --is-ancestor "$A2_A1_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A2 source does not contain the immutable A1 release'
    git merge-base --is-ancestor "$A2_MAIN_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A2 source does not contain the integrated main base'
  elif [[ "$release_phase" == 'A3' ]]; then
    git merge-base --is-ancestor "$A3_A2_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A3 source does not contain the immutable A2 release'
  elif [[ "$release_phase" == 'A4' ]]; then
    git merge-base --is-ancestor "$A4_A3_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A4 source does not contain the immutable A3 release'
  elif [[ "$release_phase" == 'A5' ]]; then
    git merge-base --is-ancestor "$A5_A4_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A5 source does not contain the immutable A4 release'
  elif [[ "$release_phase" == 'A6' ]]; then
    git merge-base --is-ancestor "$A6_A5_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A6 source does not contain the immutable A5 release'
  elif [[ "$release_phase" == 'A7' ]]; then
    git merge-base --is-ancestor "$A7_A6_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A7 source does not contain the immutable A6 release'
  elif [[ "$release_phase" == 'A8' ]]; then
    git merge-base --is-ancestor "$A8_A7_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A8 source does not contain the immutable A7 release'
  elif [[ "$release_phase" == 'A9' ]]; then
    git merge-base --is-ancestor "$A9_A8_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A9 source does not contain the immutable A8 release'
  elif [[ "$release_phase" == 'A10' ]]; then
    git merge-base --is-ancestor "$A10_A9_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A10 source does not contain the immutable A9 release'
  elif [[ "$release_phase" == 'A11' ]]; then
    git merge-base --is-ancestor "$A11_A10_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A11 source does not contain the immutable A10 release'
  elif [[ "$release_phase" == 'A12' ]]; then
    git merge-base --is-ancestor "$A12_A11_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A12 source does not contain the immutable A11 release'
  elif [[ "$release_phase" == 'A13' ]]; then
    git merge-base --is-ancestor "$A13_A12_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A13 source does not contain the immutable A12 release'
  elif [[ "$release_phase" == 'A14' ]]; then
    git merge-base --is-ancestor "$A14_A13_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A14 source does not contain the immutable A13 release'
  elif [[ "$release_phase" == 'A15' ]]; then
    git merge-base --is-ancestor "$A15_A14_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A15 source does not contain the immutable A14 release'
  elif [[ "$release_phase" == 'A16' ]]; then
    git merge-base --is-ancestor "$A16_A15_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A16 source does not contain the immutable A15 release'
  elif [[ "$release_phase" == 'A17' ]]; then
    git merge-base --is-ancestor "$A17_A16_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A17 source does not contain the immutable A16 release'
  elif [[ "$release_phase" == 'A18' ]]; then
    git merge-base --is-ancestor "$A18_A17_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A18 source does not contain the immutable A17 release'
  elif [[ "$release_phase" == 'A19' ]]; then
    git merge-base --is-ancestor "$A19_A18_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A19 source does not contain the immutable A18 release'
  elif [[ "$release_phase" == 'A20' ]]; then
    git merge-base --is-ancestor "$A20_A19_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A20 source does not contain the immutable A19 release'
  elif [[ "$release_phase" == 'A21' ]]; then
    git merge-base --is-ancestor "$A21_A20_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A21 source does not contain the immutable A20 release'
  elif [[ "$release_phase" == 'A22' ]]; then
    git merge-base --is-ancestor "$A22_A21_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A22 source does not contain the immutable A21 release'
  elif [[ "$release_phase" == 'A23' ]]; then
    git merge-base --is-ancestor "$A23_A22_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A23 source does not contain the immutable A22 release'
  elif [[ "$release_phase" == 'A24' ]]; then
    git merge-base --is-ancestor "$A24_A23_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A24 source does not contain the immutable A23 release'
  elif [[ "$release_phase" == 'A25' ]]; then
    git merge-base --is-ancestor "$A25_A24_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A25 source does not contain the immutable A24 release'
  elif [[ "$release_phase" == 'A26' ]]; then
    git merge-base --is-ancestor "$A26_A25_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A26 source does not contain the immutable A25 release'
  elif [[ "$release_phase" == 'A27' ]]; then
    git merge-base --is-ancestor "$A27_A26_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A27 source does not contain the immutable A26 release'
  elif [[ "$release_phase" == 'A28' ]]; then
    git merge-base --is-ancestor "$A28_A27_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A28 source does not contain the immutable A27 release'
  elif [[ "$release_phase" == 'A29' ]]; then
    git merge-base --is-ancestor "$A29_A28_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A29 source does not contain the immutable A28 release'
  elif [[ "$release_phase" == 'A30' ]]; then
    git merge-base --is-ancestor "$A30_A29_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A30 source does not contain the immutable A29 release'
  elif [[ "$release_phase" == 'A31' ]]; then
    git merge-base --is-ancestor "$A31_A30_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A31 source does not contain the immutable A30 release'
  elif [[ "$release_phase" == 'A32' ]]; then
    git merge-base --is-ancestor "$A32_A31_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A32 source does not contain the immutable A31 release'
  else
    git merge-base --is-ancestor "$A33_A32_ANCESTOR_SHA" "$GITHUB_SHA" \
      >/dev/null 2>&1 || fail 'A33 source does not contain the immutable A32 release'
  fi
  # A24 signs with R2 like its predecessors. An earlier commit removed it from
  # this list on the belief that R2 was destroyed; the key was found intact in
  # the operator's release-key store, so the guarantee continues unbroken.
  if [[ "$release_phase" == 'A22' || "$release_phase" == 'A23' ||
        "$release_phase" == 'A24' || "$release_phase" == 'A25' ||
        "$release_phase" == 'A26' || "$release_phase" == 'A27' ||
        "$release_phase" == 'A28' || "$release_phase" == 'A29' ||
        "$release_phase" == 'A30' || "$release_phase" == 'A31' ||
      "$release_phase" == 'A32' || "$release_phase" == 'A33' ]]; then
    if [[ "$release_phase" == 'A22' &&
          "${COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED-0}" == '1' ]]; then
      [[ "${ADK_KEY_ROTATION_VERIFIED-}" == '1' ]] ||
        fail 'A22 R2 tag verification requires an independently verified rotation sidecar'
    fi
    case "${COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED-0}" in
      0) ;;
      1)
        script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) ||
          fail 'cannot resolve release signer trust root'
        tag_public_key="$script_dir/release-tag-signing-2026-q3-r2.pub"
        tag_fingerprint="$script_dir/release-tag-signing-2026-q3-r2.fingerprint"
        [[ -f "$tag_public_key" && ! -L "$tag_public_key" &&
           -f "$tag_fingerprint" && ! -L "$tag_fingerprint" ]] ||
          fail 'R2 release signer trust root is missing or unsafe'
        expected_fingerprint=$(<"$tag_fingerprint")
        [[ "$expected_fingerprint" == 'SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ' &&
           "$(ssh-keygen -lf "$tag_public_key" -E sha256 | awk '{print $2}')" == "$expected_fingerprint" ]] ||
          fail 'R2 release signer trust root fingerprint differs'
        allowed_signers=$(mktemp "${TMPDIR:-/tmp}/adk-release-tag-signers.XXXXXX") ||
          fail 'cannot create release tag verifier state'
        trap 'rm -f -- "$allowed_signers"' EXIT
        awk 'NF >= 2 { print "autopus-adk-release-tag " $1 " " $2; exit }' \
          "$tag_public_key" >"$allowed_signers"
        chmod 0600 "$allowed_signers"
        git -c gpg.format=ssh -c gpg.ssh.allowedSignersFile="$allowed_signers" \
          verify-tag "refs/tags/$GITHUB_REF_NAME" >/dev/null ||
          fail "${release_phase} release tag signature or R2 signer differs"
        rm -f -- "$allowed_signers"
        trap - EXIT
        ;;
      *) fail 'COMPANION_RELEASE_TAG_SIGNATURE_REQUIRED must be 0 or 1' ;;
    esac
  fi
  case "${COMPANION_SOURCE_PIN_REQUIRED-0}" in
    0) ;;
    1)
      for name in COMPANION_APPROVED_SOURCE_COMMIT COMPANION_APPROVED_SOURCE_TREE; do
        [[ -n "${!name-}" ]] || fail "required approved source pin ${name} is missing"
        [[ "${!name}" =~ ^[0-9a-f]{40}$ ]] || fail "approved source pin ${name} is malformed"
      done
      [[ "$GITHUB_SHA" == "$COMPANION_APPROVED_SOURCE_COMMIT" ]] \
        || fail 'release commit differs from the approved exact source commit'
      [[ "$source_tree" == "$COMPANION_APPROVED_SOURCE_TREE" ]] \
        || fail 'release tree differs from the approved exact source tree'
      ;;
    *) fail 'COMPANION_SOURCE_PIN_REQUIRED must be 0 or 1' ;;
  esac
fi

printf 'release-phase=%s\nsource-commit=%s\nsource-tree=%s\n' \
  "$release_phase" "$GITHUB_SHA" "$source_tree" \
  >>"$GITHUB_OUTPUT"

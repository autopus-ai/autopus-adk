#!/usr/bin/env bash
set -euo pipefail
umask 077

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
script_dir=$(cd -- "$tests_dir/.." && pwd)
repo=$(cd -- "$script_dir/../.." && pwd)
fail() { printf 'release hardening test: %s\n' "$1" >&2; exit 1; }
contains() { grep -Fq -- "$2" "$1" || fail "$1 missing $2"; }
not_contains() { ! grep -Fq -- "$2" "$1" || fail "$1 unexpectedly contains $2"; }
matches() { grep -Eq -- "$2" "$1" || fail "$1 missing pattern $2"; }

config="$repo/.goreleaser.yaml"
release="$repo/.github/workflows/release.yaml"
recovery="$repo/.github/workflows/homebrew-formula-bridge-recovery.yaml"
producer_receipt="$script_dir/produce-public-key-receipt.sh"
homebrew_bridge="$script_dir/publish-homebrew-formula-bridge.sh"
homebrew_git_helper="$script_dir/publish-homebrew-formula-bridge-git.sh"
current_release_gate="$script_dir/verify-current-release.sh"
current_signature_gate="$script_dir/verify-current-release-signatures.sh"
prep="$script_dir/prepare-release.sh"
publisher="$script_dir/publish-release-coordinates.sh"
tag_ruleset_gate="$script_dir/verify-release-tag-ruleset.sh"

# GoReleaser must render, but never publish, the Cask or mutate tagged source.
contains "$config" 'skip_upload: true'
not_contains "$config" 'token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"'
not_contains "$config" 'go mod tidy'
contains "$release" 'go mod tidy -diff'

# A fresh, narrowly scoped tap token must be created after immutable release publish.
release_index=$(grep -n 'goreleaser release --clean' "$release" | cut -d: -f1)
signing_cleanup_index=$(grep -n 'name: Remove release signing credentials' "$release" | cut -d: -f1)
evidence_index=$(grep -n 'scripts/companion-release/verify-current-release.sh' "$release" | cut -d: -f1)
token_index=$(grep -n 'name: Create Homebrew tap token' "$release" | cut -d: -f1)
bridge_index=$(grep -n 'scripts/companion-release/publish-homebrew-formula-bridge.sh' "$release" | cut -d: -f1)
(( release_index < signing_cleanup_index && signing_cleanup_index < evidence_index && \
   evidence_index < token_index && token_index < bridge_index )) \
  || fail 'release evidence or tap token ordering is unsafe'
goreleaser_step=$(sed -n '/name: Run GoReleaser/,/name: Verify current immutable release evidence/p' "$release")
[[ "$goreleaser_step" != *HOMEBREW_TAP_TOKEN* ]] || fail 'GoReleaser receives tap token'
[[ "$goreleaser_step" != *APPLE_CERTIFICATE_PASSWORD* ]] || fail 'GoReleaser receives certificate password'
contains "$release" "COMPANION_CASK_PATH='dist/homebrew/Casks/auto.rb'"
contains "$release" 'COMPANION_CHECKSUMS_PATH: ${{ steps.release-evidence.outputs.checksums-path }}'
contains "$release" 'COMPANION_CHECKSUMS_PATH="$COMPANION_CHECKSUMS_PATH"'
not_contains "$release" "COMPANION_CHECKSUMS_PATH='dist/checksums.txt'"
contains "$producer_receipt" '--signing-key "$COMPANION_SIGNING_KEY_FILE"'
# Only the exact operator may create the release tag; committed tags are sealed.
[[ -x "$tag_ruleset_gate" && ! -L "$tag_ruleset_gate" ]] ||
  fail 'exact release tag ruleset verifier is missing or unsafe'
contains "$prep" 'verify-release-tag-ruleset.sh --armed'
contains "$prep" 'verify-release-tag-ruleset.sh --sealed'
contains "$publisher" 'verify-release-tag-ruleset.sh --armed'
contains "$publisher" 'verify-release-tag-ruleset.sh --sealed'
contains "$tag_ruleset_gate" "ruleset_name='autopus-v0.50.118-release-authority'"
contains "$tag_ruleset_gate" "release_ref='refs/tags/v0.50.118'"
contains "$tag_ruleset_gate" 'usage: verify-release-tag-ruleset.sh --armed|--sealed|--sealed-runtime'
contains "$tag_ruleset_gate" 'actor_type:"User"'
contains "$tag_ruleset_gate" 'else .bypass_actors == [] end'
contains "$tag_ruleset_gate" '(.bypass_actors == [] or .bypass_actors == null)'
contains "$tag_ruleset_gate" '["creation","deletion","update"]'
contains "$tag_ruleset_gate" '.can_admins_bypass == false'
contains "$tag_ruleset_gate" 'required_reviewers'
contains "$tag_ruleset_gate" 'if [[ "$mode" != '\''sealed-runtime'\'' ]]'
contains "$tag_ruleset_gate" 'deployment-branch-policies?per_page=100'
contains "$tag_ruleset_gate" 'select(.type == "tag" and .name == $tag)'
# The tap coordinates advance with every publication, so pinning their exact
# value here only forces a second edit; the publisher already enforces them
# against the live tap. Assert the shape that keeps that enforcement possible.
matches "$homebrew_bridge" "^readonly PRIOR_TAP_COMMIT='[0-9a-f]{40}'$"
matches "$homebrew_bridge" "^readonly PRIOR_CASK_BLOB='[0-9a-f]{40}'$"
contains "$homebrew_bridge" "readonly FROZEN_FORMULA_BLOB='4ebc6c38925002dec00759823d4dd847a499818a'"
contains "$homebrew_bridge" 'COMPANION_HOMEBREW_POLICY'
contains "$homebrew_bridge" "readonly FORMULA_PATH='Formula/auto.rb'"
contains "$homebrew_bridge" 'source "$git_helper"'
contains "$homebrew_git_helper" 'verify_frozen_formula'
not_contains "$homebrew_git_helper" 'reconcile_tap_file formula Formula'
contains "$homebrew_git_helper" "api_json POST 'git/blobs'"
contains "$homebrew_git_helper" "api_json POST 'git/trees'"
contains "$homebrew_git_helper" "api_json POST 'git/commits'"
contains "$homebrew_git_helper" 'api_json PATCH "git/refs/heads/${TAP_BRANCH}"'
contains "$homebrew_git_helper" '{base_tree:$base,tree:['
contains "$homebrew_git_helper" "'{sha:\$sha,force:false}'"
not_contains "$homebrew_git_helper" '--method PUT'

# Production and recovery bind the exact frozen source and sealed release tag.
for workflow in "$release" "$recovery"; do
  contains "$workflow" 'ADK_COMPANION_APPROVED_SOURCE_COMMIT'
  contains "$workflow" 'ADK_COMPANION_APPROVED_SOURCE_TREE'
  contains "$workflow" 'COMPANION_SOURCE_PIN_REQUIRED=1'
  contains "$workflow" 'verify-release-tag-ruleset.sh --sealed-runtime'
done
contains "$release" "- 'v0.50.118'"
contains "$release" "if: github.ref == 'refs/tags/v0.50.118'"
contains "$recovery" "if: github.ref == 'refs/tags/v0.50.118'"
not_contains "$release" 'canonical-full-bridge'
not_contains "$recovery" 'canonical-full bridge'
not_contains "$release" 'omp-context-bridge-release.v1.json'
not_contains "$recovery" 'omp-context-bridge-release.v1.json'
for workflow in "$release" "$recovery"; do
  contains "$workflow" 'autopus-$GITHUB_REF_NAME-checksums.txt'
  contains "$workflow" 'GITHUB_REF_NAME="$GITHUB_REF_NAME"'
  contains "$workflow" 'COMPANION_VERSION="${GITHUB_REF_NAME#v}"'
done
contains "$release" "'autopus-adk_0.50.118_darwin_amd64.tar.gz'"
contains "$release" "'autopus-adk_0.50.118_darwin_arm64.tar.gz'"
contains "$producer_receipt" 'v0.50.109 0.50.109 A22'
contains "$producer_receipt" 'v0.50.111 0.50.111 A23'
contains "$producer_receipt" 'v0.50.113 0.50.113 A24'
contains "$producer_receipt" 'v0.50.114 0.50.114 A25'
contains "$producer_receipt" 'v0.50.115 0.50.115 A26'
contains "$producer_receipt" 'v0.50.116 0.50.116 A27'
contains "$producer_receipt" 'v0.50.117 0.50.117 A28'
contains "$producer_receipt" 'v0.50.118 0.50.118 A29'
contains "$producer_receipt" "fail 'public_key_receipt_release_identity_mismatch'"
contains "$homebrew_bridge" "readonly RELEASE_TAG='v0.50.118'"
contains "$homebrew_bridge" "readonly RELEASE_VERSION='0.50.118'"
contains "$release" 'timeout-minutes: 60'
contains "$recovery" 'timeout-minutes: 20'

# Production and recovery must share one exact, fail-closed current-release gate.
for workflow in "$release" "$recovery"; do
  contains "$workflow" 'scripts/companion-release/verify-current-release.sh'
  workflow_evidence_index=$(grep -n 'scripts/companion-release/verify-current-release.sh' "$workflow" | cut -d: -f1)
  workflow_token_index=$(grep -n 'name: Create Homebrew tap token' "$workflow" | cut -d: -f1)
  (( workflow_evidence_index < workflow_token_index )) || fail 'tap token precedes release evidence'
done
contains "$current_release_gate" "readonly RELEASE_TAG='v0.50.118'"
contains "$current_release_gate" "readonly RELEASE_VERSION='0.50.118'"
contains "$current_release_gate" '.target_commitish == $commit'
contains "$current_release_gate" '.immutable == true'
contains "$current_release_gate" '(.assets | length) == ($expected | length)'
contains "$current_release_gate" '[.assets[].name] | unique | length'
contains "$current_release_gate" '.state == "uploaded"'
contains "$current_release_gate" '.size > 0'
contains "$current_release_gate" '^sha256:[0-9a-f]{64}$'
contains "$current_release_gate" 'for archive in "${EXPECTED_ARCHIVES[@]}"'
contains "$current_release_gate" 'verify-current-release-signatures.sh'
contains "$current_release_gate" 'env -i PATH="$PATH" HOME="${HOME:-/}" TMPDIR="${TMPDIR:-/tmp}"'
contains "$current_release_gate" "download_release_asset 'checksums.txt.bundle'"
contains "$current_release_gate" "download_release_asset 'checksums.txt.signatures'"
contains "$current_signature_gate" 'verify_release_checksums_v1'
contains "$current_signature_gate" 'cosign verify-blob'
contains "$current_signature_gate" 'unset GITHUB_TOKEN GH_TOKEN'
not_contains "$current_release_gate" 'omp-context-bridge-release.v1.json'
contains "$current_release_gate" 'exactly fifteen A29 normal release assets verified'
for workflow in "$release" "$recovery"; do
  contains "$workflow" 'OMP_CONTEXT_STATIC_POLICY_B64'
  contains "$workflow" 'OMP_CONTEXT_EVIDENCE_REPORT_SHA256'
  not_contains "$workflow" '--expected-signing-key-id'
  not_contains "$workflow" 'adk-key-rotation-v1.json'
done
contains "$release" 'omp-context-promotion-report.v1.json'
contains "$release" 'omp-context-promotion-attestation.v2.json'
contains "$release" '--mode active'
not_contains "$current_release_gate" '--mode active'
contains "$current_release_gate" '--mode historical'
not_contains "$current_release_gate" '--expected-signing-key-id'
for workflow in "$release" "$recovery"; do
  contains "$workflow" 'sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6'
  contains "$workflow" "cosign-release: 'v3.1.2'"
  not_contains "$workflow" 'sigstore/cosign-installer@59acb6260d9c0ba8f4a2f9d9b48431a222b68e20'
done

bash "$tests_dir/release-runtime-hardening-test.sh"
bash "$tests_dir/release-exec-smoke-hardening-test.sh"
bash "$tests_dir/release-homebrew-hardening-test.sh"
bash "$tests_dir/release-producer-helper-hardening-test.sh"
bash "$tests_dir/release-current-signature-hardening-test.sh"
bash "$tests_dir/release-current-repository-test.sh"
bash "$tests_dir/release-lineage-pins-hardening-test.sh"
bash "$tests_dir/release-omp-context-evidence-hardening-test.sh"
bash "$tests_dir/release-prep-hardening-test.sh"
bash "$tests_dir/release-prep-environment-inheritance-test.sh"

printf 'release hardening test: PASS\n'

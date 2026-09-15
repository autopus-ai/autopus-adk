#!/usr/bin/env bash
# Local-only release planning, signing, verification, and evidence publication.

create_canary_plan() {
  local output=$1
  env "$credential_locator=$provider_credential" \
    "$policy_tool" companion-manifest omp-context-canary-plan \
    --project-dir "$repo_root" --input-output "$input_jsonl" --challenge-digest "$challenge_digest" \
    --producer-repository "$repository" --producer-workflow-ref "$producer_workflow_ref" \
    --candidate-repository "$repository" --source-commit "$source_commit" --source-tree "$source_tree" \
    --target darwin-arm64 --auto-version "${release_tag#v}" --provider "$provider" --model "$model" \
    --endpoint "$endpoint" --credential-locator "$credential_locator" \
    --model-context-window "$model_context_window" \
    --policy-id omp-context-active-v1 --oracle-policy-digest "$oracle_policy_digest" \
    --omp-version 'omp/18.1.13' --omp-executable-sha256 "sha256:${expected_omp_sha256}" \
    --promotion-signing-key-id "$expected_promotion_key_id" \
    --release-lineage-key-id "$lineage_key_id" --release-lineage-handoff "$lineage_handoff" \
    --minimum-rollback-floor "$rollback_floor" >"$output"
  chmod 0600 "$output"
  [[ "$(wc -l <"$input_jsonl" | tr -d ' ')" == '42' ]] || fail 'planned canary input cardinality differs'
  jq -s -e 'length == 42 and .[0].type == "handshake" and .[-1].type == "shutdown" and
    ([.[] | select(.type == "call")] | length) == 40 and
    ([.[] | select(.type == "call") | .task_id_digest] | unique | length) == 20 and
    (group_by(.task_id_digest) | map(select(.[0].type == "call")) | all(length == 2))' \
    "$input_jsonl" >/dev/null || fail 'planned canary cohort is not exact 20-pair/40-call input'
}
sign_promotion_report() {
  local report=$1 output=$2
  "$policy_tool" companion-manifest omp-context-promotion-attestation \
    --report "$report" --valid-for 24h --output "$output" <"$promotion_signing_key"
}
verify_tag_signing_authority() {
  local derived_public expected_public derived_value expected_fingerprint signing_probe
  derived_public="$temp_dir/release-tag-signing.pub"
  ssh-keygen -y -f "$tag_signing_key" >"$derived_public"
  chmod 0600 "$derived_public"
  expected_public=$(awk 'NF >= 2 { print $1 " " $2; exit }' "$tag_public_key_file")
  derived_value=$(awk 'NF >= 2 { print $1 " " $2; exit }' "$derived_public")
  expected_fingerprint=$(<"$tag_fingerprint_file")
  [[ -n "$expected_public" && "$derived_value" == "$expected_public" ]] ||
    fail 'release tag signing key differs from R2 public pin'
  [[ "$expected_fingerprint" == 'SHA256:7FISPXCi8p7cFEdh4Fcyyp8RPQbXYZwmo3Mxi5+YjrQ' &&
     "$(ssh-keygen -lf "$derived_public" -E sha256 | awk '{print $2}')" == "$expected_fingerprint" ]] ||
    fail 'release tag signing key differs from R2 fingerprint'
  allowed_signers="$temp_dir/release-tag.allowed-signers"
  printf 'autopus-adk-release-tag %s\n' "$derived_value" >"$allowed_signers"
  chmod 0600 "$allowed_signers"
  tag_git_config=(
    GIT_CONFIG_COUNT=5
    GIT_CONFIG_KEY_0=gpg.format GIT_CONFIG_VALUE_0=ssh
    GIT_CONFIG_KEY_1=user.signingkey GIT_CONFIG_VALUE_1="$tag_signing_key"
    GIT_CONFIG_KEY_2=gpg.ssh.allowedSignersFile GIT_CONFIG_VALUE_2="$allowed_signers"
    GIT_CONFIG_KEY_3=user.name GIT_CONFIG_VALUE_3='Joseph'
    GIT_CONFIG_KEY_4=user.email GIT_CONFIG_VALUE_4='joseph@Josephui-MacBookPro.local'
  )
  signing_probe="$temp_dir/signing-probe"
  git clone --quiet --no-checkout --shared . "$signing_probe"
  env "${tag_git_config[@]}" git -C "$signing_probe" tag -s release-signing-probe HEAD -m 'release signing probe'
  env "${tag_git_config[@]}" git -C "$signing_probe" verify-tag refs/tags/release-signing-probe >/dev/null
}
credential_free_public_git() {
  local isolated_home=$1
  shift
  env -i PATH='/usr/bin:/bin:/usr/sbin:/sbin' HOME="$isolated_home" TMPDIR="$temp_dir" \
    GIT_CONFIG_NOSYSTEM=1 GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=/usr/bin/false \
    /usr/bin/git -c credential.helper= "$@"
}
load_evidence() {
  local public_root="$temp_dir/public-evidence.git" public_home="$temp_dir/public-evidence-home"
  local verified_dir="$temp_dir/verified-evidence" fetched=0 attempt
  rm -rf -- "$public_root" "$public_home" "$verified_dir"
  install -d -m 0700 "$public_root" "$public_home"
  credential_free_public_git "$public_home" init --quiet --bare "$public_root"
  for attempt in 1 2 3 4 5; do
    if credential_free_public_git "$public_home" -C "$public_root" fetch --quiet --force \
      --no-tags 'https://github.com/autopus-ai/autopus-adk.git' "$evidence_ref:$evidence_ref"; then
      fetched=1; break
    fi
    /bin/sleep 2
  done
  [[ "$fetched" -eq 1 ]] || fail 'public evidence tag is not anonymously retrievable'
  evidence_tag_object=$(/usr/bin/git -C "$public_root" rev-parse --verify "$evidence_ref")
  evidence_commit=$(/usr/bin/git -C "$public_root" rev-parse --verify "${evidence_ref}^{commit}")
  evidence_tree=$(/usr/bin/git -C "$public_root" rev-parse --verify "${evidence_ref}^{tree}")
  [[ -z "$evidence_remote" || "$evidence_remote" == "$evidence_tag_object"$'\t'"$evidence_ref" ]] ||
    fail 'authenticated and public evidence refs differ'
  report_sha256=$(/usr/bin/git -C "$public_root" cat-file blob \
    "${evidence_commit}:omp-context-promotion-report.v1.json" | shasum -a 256 | awk '{print $1}')
  attestation_sha256=$(/usr/bin/git -C "$public_root" cat-file blob \
    "${evidence_commit}:omp-context-promotion-attestation.v2.json" | shasum -a 256 | awk '{print $1}')
  (
    cd -- "$public_root"
    env -i PATH='/usr/bin:/bin:/usr/sbin:/sbin' HOME="$public_home" TMPDIR="$temp_dir" \
      OMP_CONTEXT_EVIDENCE_TAG_OBJECT_SHA="$evidence_tag_object" \
      OMP_CONTEXT_EVIDENCE_COMMIT_SHA="$evidence_commit" OMP_CONTEXT_EVIDENCE_TREE_SHA="$evidence_tree" \
      OMP_CONTEXT_EVIDENCE_REPORT_SHA256="$report_sha256" \
      OMP_CONTEXT_EVIDENCE_ATTESTATION_SHA256="$attestation_sha256" \
      "$repo_root/scripts/companion-release/verify-omp-context-evidence-tag.sh" "$verified_dir"
  )
  verified_report="$verified_dir/omp-context-promotion-report.v1.json"
  verified_attestation="$verified_dir/omp-context-promotion-attestation.v2.json"
}
verify_evidence() {
  local mode=$1 verifier_home="$temp_dir/evidence-verifier-home"
  [[ "$mode" == 'active' || "$mode" == 'historical' ]] || fail 'evidence verification mode is invalid'
  install -d -m 0700 "$verifier_home"
  env -i PATH='/usr/bin:/bin:/usr/sbin:/sbin' HOME="$verifier_home" TMPDIR="$temp_dir" \
    "$verifier" --mode "$mode" --report "$verified_report" --attestation "$verified_attestation" \
    --report-sha256 "$report_sha256" --attestation-sha256 "$attestation_sha256" \
    --candidate-repository "$repository" --candidate-revision "$source_commit" --candidate-tree "$source_tree" \
    --candidate-artifact-sha256 "$candidate_sha256" --static-policy-b64 "$static_policy_b64"
}
publish_local_evidence() {
  local report=$1 attestation publish_root evidence_index published_tree published_commit
  local report_blob attestation_blob remote_state remote_tag_status=0
  attestation="$temp_dir/omp-context-promotion-attestation.v2.json"
  sign_promotion_report "$report" "$attestation"
  report_sha256=$(shasum -a 256 "$report" | awk '{print $1}')
  attestation_sha256=$(shasum -a 256 "$attestation" | awk '{print $1}')
  verified_report=$report; verified_attestation=$attestation
  verify_evidence active
  ensure_prep_lock "$report"
  git fetch --no-tags origin main
  [[ "$(git rev-parse --verify origin/main)" == "$source_commit" ]] || fail 'origin/main advanced before local evidence publish'
  [[ "$(git ls-remote --refs origin "$evidence_source_ref")" == "$evidence_source_commit"$'\t'"$evidence_source_ref" ]] ||
    fail 'prep lock changed before local evidence publish'
  git ls-remote --exit-code --refs origin "$evidence_ref" >/dev/null 2>&1 || remote_tag_status=$?
  case "$remote_tag_status" in 0) fail 'evidence tag already exists' ;; 2) ;; *) fail 'cannot inspect evidence tag before local publish' ;; esac
  publish_root="$temp_dir/local-evidence-publish"
  git init --quiet "$publish_root"
  git -C "$publish_root" remote add origin "$(git remote get-url origin)"
  git -C "$publish_root" fetch --no-tags --depth=1 origin "$evidence_source_ref"
  [[ "$(git -C "$publish_root" rev-parse --verify FETCH_HEAD)" == "$evidence_source_commit" ]] || fail 'fetched prep lock differs during local publish'
  report_blob=$(git -C "$publish_root" hash-object -w -- "$report")
  attestation_blob=$(git -C "$publish_root" hash-object -w -- "$attestation")
  [[ "$report_blob" == "$(git rev-parse --verify "${evidence_source_commit}:omp-context-promotion-report.v1.json")" ]] ||
    fail 'signed report differs from prep lock'
  evidence_index="$publish_root/evidence.index"
  GIT_INDEX_FILE="$evidence_index" git -C "$publish_root" read-tree --empty
  GIT_INDEX_FILE="$evidence_index" git -C "$publish_root" update-index --add \
    --cacheinfo "100644,$report_blob,omp-context-promotion-report.v1.json" \
    --cacheinfo "100644,$attestation_blob,omp-context-promotion-attestation.v2.json"
  published_tree=$(GIT_INDEX_FILE="$evidence_index" git -C "$publish_root" write-tree)
  published_commit=$(printf 'OMP context promotion evidence %s\n' "$release_tag" | \
    git -C "$publish_root" -c user.name='Joseph' -c user.email='joseph@Josephui-MacBookPro.local' commit-tree "$published_tree")
  [[ "$(git -C "$publish_root" rev-list --parents -n 1 "$published_commit")" == "$published_commit" ]] ||
    fail 'locally published evidence is not an orphan'
  git -C "$publish_root" -c user.name='Joseph' -c user.email='joseph@Josephui-MacBookPro.local' \
    -c tag.gpgSign=false tag -a "$evidence_tag" "$published_commit" -m "OMP context promotion evidence ${release_tag}"
  evidence_tag_object=$(git -C "$publish_root" rev-parse --verify "$evidence_ref")
  [[ "$(git ls-remote --refs origin refs/heads/main)" == "$source_commit"$'\trefs/heads/main' ]] ||
    fail 'origin/main advanced during local evidence publish'
  git -C "$publish_root" push --atomic --force-with-lease="${evidence_source_ref}:${evidence_source_commit}" origin \
    "$evidence_ref:$evidence_ref" "$evidence_source_commit:$evidence_source_ref"
  remote_state=$(git ls-remote --refs origin "$evidence_ref" "$evidence_source_ref")
  [[ "$(awk -v ref="$evidence_ref" '$2 == ref { print $0 }' <<<"$remote_state")" == "$evidence_tag_object"$'\t'"$evidence_ref" ]] ||
    fail 'local evidence tag differs after publish'
  [[ "$(awk -v ref="$evidence_source_ref" '$2 == ref { print $0 }' <<<"$remote_state")" == "$evidence_source_commit"$'\t'"$evidence_source_ref" ]] ||
    fail 'prep lock changed during local evidence publish'
  evidence_remote="$evidence_tag_object"$'\t'"$evidence_ref"
  load_evidence
  cmp "$report" "$verified_report" || fail 'published report differs from final production canary'
  cmp "$attestation" "$verified_attestation" || fail 'published attestation differs from local signature'
  verify_evidence active
}

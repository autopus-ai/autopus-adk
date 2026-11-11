#!/usr/bin/env bash

# @AX:ANCHOR [AUTO]: Preserve the authenticated Contents API read boundary.
# @AX:REASON [AUTO]: Formula and Cask evidence must be read from the pinned tap repository and branch.
api_get() {
  local path="$1" output="$2"
  if ! GH_TOKEN="$tap_token" gh api \
    -H 'Accept: application/vnd.github+json' \
    "repos/${TAP_REPOSITORY}/contents/${path}?ref=${TAP_BRANCH}" \
    >"$output" 2>"$temp_dir/gh-error"; then
    fail "cannot read ${path} from the Homebrew tap"
  fi
}

# @AX:ANCHOR [AUTO]: Preserve the authenticated Git Data API request boundary.
# @AX:REASON [AUTO]: Commit, tree, blob, and ref evidence must share identical token and error-isolation handling.
api_json() {
  local method="$1" endpoint="$2" input="$3" output="$4" label="$5"
  local -a args=(api --method "$method" -H 'Accept: application/vnd.github+json'
    "repos/${TAP_REPOSITORY}/${endpoint}")
  [[ -z "$input" ]] || args+=(--input "$input")
  if ! GH_TOKEN="$tap_token" gh "${args[@]}" >"$output" 2>"$temp_dir/gh-error"; then
    fail "$label"
  fi
}

verify_frozen_formula() {
  local response="$temp_dir/formula-response.json" blob
  api_get "$FORMULA_PATH" "$response"
  blob=$(jq -er '.sha | select(type == "string" and test("^[0-9a-f]{40}$"))' \
    "$response") || fail 'Homebrew tap Formula response has an invalid blob SHA'
  [[ "$blob" == "$FROZEN_FORMULA_BLOB" ]] \
    || fail 'published Formula differs from the frozen v0.50.71 blob'
}

verify_prior_tap_head() {
  local response="$temp_dir/prior-tap-head.json" head_sha
  api_json GET "git/ref/heads/${TAP_BRANCH}" '' "$response" \
    'cannot read the Homebrew tap branch head'
  head_sha=$(jq -er --arg ref "refs/heads/${TAP_BRANCH}" '
    select(type == "object" and .ref == $ref and .object.type == "commit") |
    .object.sha | select(type == "string" and test("^[0-9a-f]{40}$"))
  ' "$response") || fail 'Homebrew tap branch head response is invalid'
  # A successful publication advances the tap, so the pin has to be bumped in the
  # same PR that ships the next release. Naming both SHAs turns that bump into a
  # copy-paste instead of a mid-release investigation.
  [[ "$head_sha" == "$PRIOR_TAP_COMMIT" ]] \
    || fail "Homebrew tap branch differs from the pinned predecessor commit (tap head ${head_sha}, pinned ${PRIOR_TAP_COMMIT}); update PRIOR_TAP_COMMIT to the tap head"
}

# @AX:NOTE [AUTO]: [downgraded from ANCHOR — fan_in=1 under file cap] Bind idempotent success to one stable head, tree, Cask, and Formula snapshot.
verify_idempotent_head_snapshot() {
  local remote_path="$1" cask_blob="$2" label="$3"
  local ref_response="$temp_dir/idempotent-ref.json"
  local commit_response="$temp_dir/idempotent-commit.json"
  local tree_response="$temp_dir/idempotent-tree.json"
  local final_ref_response="$temp_dir/idempotent-final-ref.json"
  local head_sha tree_sha
  api_json GET "git/ref/heads/${TAP_BRANCH}" '' "$ref_response" \
    "cannot read the Homebrew tap ${label} idempotent head"
  head_sha=$(jq -er --arg ref "refs/heads/${TAP_BRANCH}" '
    select(type == "object" and .ref == $ref and .object.type == "commit" and
      (.object.url | type) == "string" and (.object.url | length) > 0) |
    .object.sha | select(type == "string" and test("^[0-9a-f]{40}$"))
  ' "$ref_response") || fail "Homebrew tap ${label} idempotent head response is invalid"
  api_json GET "git/commits/${head_sha}" '' "$commit_response" \
    "cannot read the Homebrew tap ${label} idempotent commit"
  tree_sha=$(jq -er --arg commit "$head_sha" '
    select(type == "object" and .sha == $commit and (.tree | type) == "object" and
      (.url | type) == "string" and (.url | length) > 0) |
    .tree.sha | select(type == "string" and test("^[0-9a-f]{40}$"))
  ' "$commit_response") || fail "Homebrew tap ${label} idempotent commit response is invalid"
  api_json GET "git/trees/${tree_sha}?recursive=1" '' "$tree_response" \
    "cannot read the Homebrew tap ${label} idempotent tree"
  jq -e --arg tree "$tree_sha" --arg cask "$remote_path" --arg blob "$cask_blob" \
    --arg formula "$FORMULA_PATH" --arg frozen "$FROZEN_FORMULA_BLOB" '
    type == "object" and .sha == $tree and .truncated == false and
    (.tree | type) == "array" and
    ([.tree[] | select(.path == $cask)] | length) == 1 and
    ([.tree[] | select(.path == $cask and .mode == "100644" and
      .type == "blob" and .sha == $blob)] | length) == 1 and
    ([.tree[] | select(.path == $formula)] | length) == 1 and
    ([.tree[] | select(.path == $formula and .mode == "100644" and
      .type == "blob" and .sha == $frozen)] | length) == 1
  ' "$tree_response" >/dev/null \
    || fail "Homebrew tap ${label} idempotent tree does not preserve exact Cask/Formula blobs"
  api_json GET "git/ref/heads/${TAP_BRANCH}" '' "$final_ref_response" \
    "cannot reread the Homebrew tap ${label} idempotent head"
  jq -e --arg ref "refs/heads/${TAP_BRANCH}" --arg head "$head_sha" '
    type == "object" and .ref == $ref and .object.type == "commit" and
    .object.sha == $head and (.object.url | type) == "string" and
    (.object.url | length) > 0
  ' "$final_ref_response" >/dev/null \
    || fail "Homebrew tap ${label} head moved during idempotent verification"
}

decode_api_content() {
  local response="$1" output="$2" encoded
  encoded=$(jq -er '.content | select(type == "string")' "$response") \
    || fail 'Homebrew tap response is missing encoded content'
  printf '%s' "$encoded" | tr -d '\r\n' | "${base64_decode[@]}" >"$output" \
    || fail 'Homebrew tap content is not valid base64'
}

sha256_file() {
  local output digest
  output=$("${sha256_command[@]}" "$1") || return 1
  digest="${output%%[[:space:]]*}"
  [[ "$digest" =~ ^[0-9a-f]{64}$ ]] || return 1
  printf '%s' "$digest"
}

# @AX:ANCHOR [AUTO]: Preserve the sole compare-and-swap mutation orchestrator for the Homebrew tap.
# @AX:REASON [AUTO]: Publication must advance only the pinned predecessor through blob, tree, commit, and non-force ref updates.
# @AX:WARN [AUTO]: This function contains more than eight publication and verification branches.
# @AX:REASON [AUTO]: Predecessor, frozen Formula, Cask blob, tree, commit, ref CAS, and post-read checks must remain ordered.
publish_cask() {
  local stem="$1" label="$2" remote_path="$3" target="$4" prior_blob="$5"
  local commit_message="$6" drift_message="$7"
  local response="$temp_dir/${stem}-response.json"
  local current="$temp_dir/${stem}-current.rb"
  local target_digest current_digest blob encoded prior_tree new_blob new_tree new_commit
  local prior_commit_response="$temp_dir/prior-commit.json" blob_request="$temp_dir/${stem}-blob-request.json"
  local blob_response="$temp_dir/${stem}-blob-response.json" tree_request="$temp_dir/${stem}-tree-request.json"
  local tree_response="$temp_dir/${stem}-tree-response.json" tree_evidence_response="$temp_dir/${stem}-tree-evidence-response.json"
  local commit_request="$temp_dir/${stem}-commit-request.json" commit_response="$temp_dir/${stem}-commit-response.json"
  local ref_request="$temp_dir/${stem}-ref-request.json" ref_response="$temp_dir/${stem}-ref-response.json"
  local final_ref_response="$temp_dir/${stem}-final-ref-response.json"

  api_get "$remote_path" "$response"
  decode_api_content "$response" "$current"
  blob=$(jq -er '.sha | select(type == "string" and test("^[0-9a-f]{40}$"))' \
    "$response") || fail "Homebrew tap ${label} response has an invalid blob SHA"
  target_digest=$(sha256_file "$target") || fail "cannot digest target ${label}"
  current_digest=$(sha256_file "$current") || fail "cannot digest current ${label}"
  if [[ "$target_digest" == "$current_digest" ]] && cmp -s "$target" "$current"; then
    verify_idempotent_head_snapshot "$remote_path" "$blob" "$label"
    printf 'homebrew cask publication: %s is already current\n' "$label"
    return 0
  fi

  verify_prior_tap_head
  [[ "$blob" == "$prior_blob" ]] || fail "$drift_message"
  api_json GET "git/commits/${PRIOR_TAP_COMMIT}" '' "$prior_commit_response" \
    'cannot read the pinned Homebrew tap predecessor commit'
  prior_tree=$(jq -er --arg commit "$PRIOR_TAP_COMMIT" '
    select(type == "object" and .sha == $commit and (.parents | type) == "array" and
      (.url | type) == "string" and (.url | length) > 0) |
    .tree.sha | select(type == "string" and test("^[0-9a-f]{40}$"))
  ' "$prior_commit_response") || fail 'pinned Homebrew predecessor commit response is invalid'
  encoded=$(base64 <"$target" | tr -d '\r\n') \
    || fail "cannot encode target ${label}"
  jq -n --arg content "$encoded" '{content:$content,encoding:"base64"}' \
    >"$blob_request" || fail "cannot construct ${label} blob request"
  api_json POST 'git/blobs' "$blob_request" "$blob_response" \
    "cannot create Homebrew tap ${label} blob"
  new_blob=$(jq -er '
    select(type == "object" and (.url | type) == "string" and (.url | length) > 0) |
    .sha | select(type == "string" and test("^[0-9a-f]{40}$"))
  ' "$blob_response") || fail "Homebrew tap ${label} blob response is invalid"
  jq -n --arg base "$prior_tree" --arg path "$remote_path" --arg sha "$new_blob" \
    '{base_tree:$base,tree:[{path:$path,mode:"100644",type:"blob",sha:$sha}]}' \
    >"$tree_request" || fail "cannot construct ${label} tree request"
  api_json POST 'git/trees' "$tree_request" "$tree_response" \
    "cannot create Homebrew tap ${label} tree"
  new_tree=$(jq -er '
    select(type == "object" and .truncated == false and (.tree | type) == "array" and
      (.url | type) == "string" and (.url | length) > 0) |
    .sha | select(type == "string" and test("^[0-9a-f]{40}$"))
  ' "$tree_response") || fail "Homebrew tap ${label} tree response is invalid"
  api_json GET "git/trees/${new_tree}?recursive=1" '' "$tree_evidence_response" \
    "cannot verify Homebrew tap ${label} tree"
  jq -e --arg tree "$new_tree" --arg cask "$remote_path" --arg blob "$new_blob" \
    --arg formula "$FORMULA_PATH" --arg frozen "$FROZEN_FORMULA_BLOB" '
    type == "object" and .sha == $tree and .truncated == false and
    (.tree | type) == "array" and
    ([.tree[] | select(.path == $cask and .mode == "100644" and
      .type == "blob" and .sha == $blob)] | length) == 1 and
    ([.tree[] | select(.path == $formula and .mode == "100644" and
      .type == "blob" and .sha == $frozen)] | length) == 1
  ' "$tree_evidence_response" >/dev/null \
    || fail "Homebrew tap ${label} tree does not preserve exact Cask/Formula blobs"
  jq -n --arg message "$commit_message" --arg tree "$new_tree" \
    --arg parent "$PRIOR_TAP_COMMIT" \
    '{message:$message,tree:$tree,parents:[$parent]}' >"$commit_request" \
    || fail "cannot construct ${label} commit request"
  api_json POST 'git/commits' "$commit_request" "$commit_response" \
    "cannot create Homebrew tap ${label} commit"
  new_commit=$(jq -er --arg message "$commit_message" --arg tree "$new_tree" \
    --arg parent "$PRIOR_TAP_COMMIT" '
    select(type == "object" and (.url | type) == "string" and (.url | length) > 0 and
      .message == $message and .tree.sha == $tree and
      (.parents | type) == "array" and (.parents | length) == 1 and
      .parents[0].sha == $parent) |
    .sha | select(type == "string" and test("^[0-9a-f]{40}$"))
  ' "$commit_response") || fail "Homebrew tap ${label} commit response is invalid"
  jq -n --arg sha "$new_commit" '{sha:$sha,force:false}' >"$ref_request" \
    || fail "cannot construct ${label} ref request"
  api_json PATCH "git/refs/heads/${TAP_BRANCH}" "$ref_request" "$ref_response" \
    "Homebrew tap ${label} expected-head update failed"
  jq -e --arg ref "refs/heads/${TAP_BRANCH}" --arg commit "$new_commit" '
    type == "object" and .ref == $ref and .object.type == "commit" and
    .object.sha == $commit and (.object.url | type) == "string" and
    (.object.url | length) > 0
  ' "$ref_response" >/dev/null || fail "Homebrew tap ${label} ref response is invalid"
  # The ref read lags the PATCH that just succeeded: v0.51.4 published its Cask
  # at 08:13:08 and this read, the same second, still named the predecessor.
  # A read that names the predecessor is retried; any other commit is a real
  # concurrent move and fails at once, as before.
  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    api_json GET "git/ref/heads/${TAP_BRANCH}" '' "$final_ref_response" \
      'cannot verify the final Homebrew tap branch head'
    if jq -e --arg ref "refs/heads/${TAP_BRANCH}" --arg commit "$new_commit" '
      type == "object" and .ref == $ref and .object.type == "commit" and
      .object.sha == $commit and (.object.url | type) == "string" and
      (.object.url | length) > 0
    ' "$final_ref_response" >/dev/null; then
      break
    fi
    jq -e --arg ref "refs/heads/${TAP_BRANCH}" --arg prior "$PRIOR_TAP_COMMIT" '
      type == "object" and .ref == $ref and .object.sha == $prior
    ' "$final_ref_response" >/dev/null && [[ "$attempt" -lt 10 ]] ||
      fail "Homebrew tap ${label} head moved after publication"
    sleep 3
  done

  # The contents API is read-through and lagged behind the ref update we just
  # made: v0.50.113 published the correct Cask at 02:01:47 and this read failed
  # two seconds later against the previous blob. Verifying the published bytes
  # is the point of this block, so retry the read rather than weaken it. The
  # ref check above already proved the branch head is the commit we wrote.
  for attempt in 1 2 3 4 5 6 7 8 9 10; do
    api_get "$remote_path" "$response"
    decode_api_content "$response" "$current"
    if [[ "$(jq -er '.sha' "$response")" == "$new_blob" ]] && cmp -s "$target" "$current"; then
      break
    fi
    [[ "$attempt" -lt 10 ]] || fail "Homebrew tap ${label} differs after publication"
    sleep 3
  done
  printf 'homebrew cask publication: %s published and verified\n' "$label"
}

#!/usr/bin/env bash
# Test-only GitHub Git Data/Contents API state machine.
set -euo pipefail

# Injected, never duplicated: the publisher owns the tap coordinates and a
# release bumps them. This fixture is installed outside the repo, so the
# caller passes the pin instead of a literal that would stop matching.
readonly prior_commit="${MOCK_TAP_PRIOR_COMMIT:?mock tap gh: MOCK_TAP_PRIOR_COMMIT is required}"
readonly prior_tree='aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
readonly target_blob='1111111111111111111111111111111111111111'
readonly target_tree='2222222222222222222222222222222222222222'
readonly target_commit='3333333333333333333333333333333333333333'
readonly idempotent_race_commit='8888888888888888888888888888888888888888'
readonly idempotent_race_tree='9999999999999999999999999999999999999999'
readonly formula_drift_blob='6666666666666666666666666666666666666666'

[[ "${1-}" == 'api' ]] || exit 64
shift
method='GET'
input=''
endpoint=''
while (($#)); do
  case "$1" in
    --method) method="$2"; shift 2 ;;
    --input) input="$2"; shift 2 ;;
    -H) shift 2 ;;
    *) endpoint="$1"; shift ;;
  esac
done

if [[ "${MOCK_FAIL_WITH_TOKEN-}" == '1' ]]; then
  printf 'mock diagnostic included %s\n' "${GH_TOKEN-}" >&2
  exit 72
fi

increment() {
  local counter="$MOCK_TAP_STATE/$1.calls" count
  if [[ -f "$counter" ]]; then count=$(<"$counter"); else count=0; fi
  printf '%s\n' "$((count + 1))" >"$counter"
}

case "$endpoint" in
  *contents/Casks/auto.rb*)
    [[ "$method" == 'GET' ]] || exit 64
    if [[ -e "$MOCK_TAP_STATE/idempotent-formula-race" ]]; then
      cat "$MOCK_TAP_STATE/cask.json"
      jq -n --arg sha "$idempotent_race_commit" \
        '{ref:"refs/heads/main",object:{type:"commit",sha:$sha,url:"https://example.invalid/idempotent-racer"}}' \
        >"$MOCK_TAP_STATE/branch.json"
      content=$(jq -er '.content' "$MOCK_TAP_STATE/formula.json")
      jq -n --arg content "$content" --arg sha "$formula_drift_blob" \
        '{sha:$sha,content:$content}' >"$MOCK_TAP_STATE/formula.json"
      exit 0
    fi
    exec cat "$MOCK_TAP_STATE/cask.json"
    ;;
  *contents/Formula/auto.rb*)
    [[ "$method" == 'GET' ]] || exit 64
    increment formula-get
    exec cat "$MOCK_TAP_STATE/formula.json"
    ;;
  *git/ref/heads/main*)
    [[ "$method" == 'GET' ]] || exit 64
    increment branch-get
    if [[ -e "$MOCK_TAP_STATE/idempotent-ref-race" &&
          "$(<"$MOCK_TAP_STATE/branch-get.calls")" == '2' ]]; then
      jq -n '{ref:"refs/heads/main",object:{type:"commit",sha:"4444444444444444444444444444444444444444",url:"https://example.invalid/idempotent-ref-racer"}}' \
        >"$MOCK_TAP_STATE/branch.json"
    fi
    exec cat "$MOCK_TAP_STATE/branch.json"
    ;;
  *git/refs/heads/main*)
    [[ "$method" == 'PATCH' && -f "$input" ]] || exit 64
    [[ "$(jq -er '.sha' "$input")" == "$target_commit" ]] || exit 65
    [[ "$(jq -er '.force' "$input")" == 'false' ]] || exit 65
    if [[ -e "$MOCK_TAP_STATE/race-before-ref" ]]; then
      jq -n '{ref:"refs/heads/main",object:{type:"commit",sha:"4444444444444444444444444444444444444444",url:"https://example.invalid/racer"}}' \
        >"$MOCK_TAP_STATE/branch.json"
    elif [[ -e "$MOCK_TAP_STATE/formula-race-before-ref" ]]; then
      jq -n '{ref:"refs/heads/main",object:{type:"commit",sha:"5555555555555555555555555555555555555555",url:"https://example.invalid/formula-racer"}}' \
        >"$MOCK_TAP_STATE/branch.json"
      content=$(jq -er '.content' "$MOCK_TAP_STATE/formula.json")
      jq -n --arg content "$content" \
        '{sha:"6666666666666666666666666666666666666666",content:$content}' \
        >"$MOCK_TAP_STATE/formula.json"
    fi
    [[ "$(jq -er '.object.sha' "$MOCK_TAP_STATE/branch.json")" == "$prior_commit" ]] \
      || exit 65
    content=$(<"$MOCK_TAP_STATE/pending-cask-content")
    jq -n --arg content "$content" --arg sha "$target_blob" \
      '{sha:$sha,content:$content}' >"$MOCK_TAP_STATE/cask.json"
    jq -n --arg sha "$target_commit" \
      '{ref:"refs/heads/main",object:{type:"commit",sha:$sha,url:"https://example.invalid/target-commit"}}' \
      >"$MOCK_TAP_STATE/branch.json"
    increment ref-update
    cat "$MOCK_TAP_STATE/branch.json"
    ;;
  *git/commits/"$prior_commit"*)
    [[ "$method" == 'GET' ]] || exit 64
    jq -n --arg sha "$prior_commit" --arg tree "$prior_tree" \
      '{sha:$sha,tree:{sha:$tree},parents:[{sha:"7777777777777777777777777777777777777777"}],url:"https://example.invalid/prior-commit"}'
    ;;
  *git/commits/"$target_commit"*)
    [[ "$method" == 'GET' ]] || exit 64
    jq -n --arg sha "$target_commit" --arg tree "$target_tree" \
      --arg parent "$prior_commit" \
      '{sha:$sha,tree:{sha:$tree},parents:[{sha:$parent}],url:"https://example.invalid/target-commit"}'
    ;;
  *git/commits/"$idempotent_race_commit"*)
    [[ "$method" == 'GET' ]] || exit 64
    jq -n --arg sha "$idempotent_race_commit" --arg tree "$idempotent_race_tree" \
      --arg parent "$target_commit" \
      '{sha:$sha,tree:{sha:$tree},parents:[{sha:$parent}],url:"https://example.invalid/idempotent-racer"}'
    ;;
  *git/blobs*)
    [[ "$method" == 'POST' && -f "$input" ]] || exit 64
    [[ "$(jq -er '.encoding' "$input")" == 'base64' ]] || exit 65
    jq -er '.content | select(type == "string" and length > 0)' "$input" \
      >"$MOCK_TAP_STATE/pending-cask-content" || exit 65
    increment blob-create
    jq -n --arg sha "$target_blob" \
      '{sha:$sha,url:"https://example.invalid/target-blob"}'
    ;;
  *git/trees/"$target_tree"*)
    [[ "$method" == 'GET' ]] || exit 64
    jq -n --arg sha "$target_tree" --arg blob "$target_blob" \
      '{sha:$sha,truncated:false,tree:[
        {path:"Casks/auto.rb",mode:"100644",type:"blob",sha:$blob},
        {path:"Formula/auto.rb",mode:"100644",type:"blob",sha:"4ebc6c38925002dec00759823d4dd847a499818a"}
      ]}'
    ;;
  *git/trees/"$idempotent_race_tree"*)
    [[ "$method" == 'GET' ]] || exit 64
    jq -n --arg sha "$idempotent_race_tree" --arg blob "$target_blob" \
      --arg formula "$formula_drift_blob" \
      '{sha:$sha,truncated:false,tree:[
        {path:"Casks/auto.rb",mode:"100644",type:"blob",sha:$blob},
        {path:"Formula/auto.rb",mode:"100644",type:"blob",sha:$formula}
      ]}'
    ;;
  *git/trees*)
    [[ "$method" == 'POST' && -f "$input" ]] || exit 64
    jq -e --arg base "$prior_tree" --arg blob "$target_blob" '
      .base_tree == $base and (.tree | length) == 1 and
      .tree[0] == {path:"Casks/auto.rb",mode:"100644",type:"blob",sha:$blob}
    ' "$input" >/dev/null || exit 65
    increment tree-create
    jq -n --arg sha "$target_tree" --arg blob "$target_blob" \
      '{sha:$sha,url:"https://example.invalid/target-tree",truncated:false,
        tree:[{path:"Casks/auto.rb",mode:"100644",type:"blob",sha:$blob}]}'
    ;;
  *git/commits*)
    [[ "$method" == 'POST' && -f "$input" ]] || exit 64
    jq -e --arg tree "$target_tree" --arg parent "$prior_commit" '
      .message == "Publish signed Cask for v0.50.119" and .tree == $tree and
      .parents == [$parent]
    ' "$input" >/dev/null || exit 65
    increment commit-create
    jq -n --arg sha "$target_commit" --arg tree "$target_tree" \
      --arg parent "$prior_commit" \
      '{sha:$sha,message:"Publish signed Cask for v0.50.119",tree:{sha:$tree},
        parents:[{sha:$parent}],url:"https://example.invalid/target-commit"}'
    ;;
  *) exit 64 ;;
esac

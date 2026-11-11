#!/usr/bin/env bash
# Move the release coordinate from a published FROM using its measurement.
#
# advance-release-coordinate.sh hands over here once it has confirmed that FROM
# is published. Everything that was a hand edit before (the phase map and
# ancestor pin, the lineage declaration and branch, the accumulating phase
# lists, the predecessor pins in the canary, preflight and Homebrew publisher,
# the tests that pin the coordinate, and both runbooks) comes from:
#   - the measurement JSON of FROM (measure-published-release.sh),
#   - the tree itself (the predecessor pins being replaced),
#   - advance-release-coordinate-rules.pl (which line moves and which is history),
#   - the Published record of FROM, measured here through gh unless passed in.
#
# usage: advance-release-coordinate-measured.sh FROM_TAG FROM_PHASE TO_TAG TO_PHASE
#          --measurements FILE [--release-run ID] [--release-conclusion WORD]
#          [--canary-run ID] [--canary-conclusion WORD] [--ruleset-id ID]
#          [--ruleset-state sealed|armed] [--tag-policy-id ID] [--note TEXT]
#          [--no-runbooks] [--skip-line-check]
set -euo pipefail
umask 077

fail() { printf 'advance coordinate: %s\n' "$1" >&2; exit 1; }
usage() {
  sed -n '14,18p' "${BASH_SOURCE[0]}" | sed 's/^# //' >&2
  exit 64
}
[[ $# -ge 6 ]] || usage
readonly from_tag=$1 from_phase=$2 to_tag=$3 to_phase=$4
shift 4
measurements='' release_run='' release_conclusion='' canary_run='' canary_conclusion=''
ruleset_id='' ruleset_state='' tag_policy_id='' note='' runbooks=1 line_check=1
while (($#)); do
  case "$1" in
    --no-runbooks) runbooks=0; shift; continue ;;
    --skip-line-check) line_check=0; shift; continue ;;
  esac
  [[ $# -ge 2 ]] || usage
  case "$1" in
    --measurements) measurements=$2 ;;
    --release-run) release_run=$2 ;;
    --release-conclusion) release_conclusion=$2 ;;
    --canary-run) canary_run=$2 ;;
    --canary-conclusion) canary_conclusion=$2 ;;
    --ruleset-id) ruleset_id=$2 ;;
    --ruleset-state) ruleset_state=$2 ;;
    --tag-policy-id) tag_policy_id=$2 ;;
    --note) note=$2 ;;
    *) usage ;;
  esac
  shift 2
done
[[ -f "$measurements" && ! -L "$measurements" ]] || fail '--measurements is missing or unsafe'
measurements=$(cd -- "$(dirname -- "$measurements")" && pwd)/$(basename -- "$measurements")

repo_root=$(git rev-parse --show-toplevel) || fail 'not a git repository'
cd "$repo_root"
readonly repository='autopus-ai/autopus-adk' tools="$repo_root/scripts/release-tools"
[[ -z "$(git status --porcelain --untracked-files=no)" ]] ||
  fail 'the working tree has tracked changes; move the coordinate from a clean tree'

work=$(mktemp -d "${TMPDIR:-/tmp}/advance-coordinate.XXXXXX") || fail 'cannot create workspace'
trap 'rm -rf -- "$work"' EXIT

# 1. The Published record of FROM. Values the caller did not pass are read
# from GitHub before anything is written, so a record that cannot be measured
# refuses the move instead of leaving it half done.
api() { gh api "$@" </dev/null; }
latest_run() {
  gh run list -R "$repository" --workflow "$1" --branch "$from_tag" --limit 1 \
    --json databaseId,conclusion --jq ".[0] | select(. != null) | .$2" </dev/null
}
readonly previous_runbook="docs/runbooks/release-${from_tag}.md" next_runbook="docs/runbooks/release-${to_tag}.md"
if [[ "$runbooks" -eq 1 ]]; then
  [[ -f "$previous_runbook" ]] || fail "${previous_runbook} is missing"
  ! grep -q '^\*\*Published ' "$previous_runbook" || fail "${previous_runbook} already has a Published record"
  [[ ! -e "$next_runbook" ]] || fail "${next_runbook} already exists"
  [[ -n "$release_run" ]] || release_run=$(latest_run release.yaml databaseId) || true
  [[ -n "$release_conclusion" ]] || release_conclusion=$(latest_run release.yaml conclusion) || true
  [[ -n "$canary_run" ]] || canary_run=$(latest_run upgrade-canary.yaml databaseId) || true
  [[ -n "$canary_conclusion" ]] || canary_conclusion=$(latest_run upgrade-canary.yaml conclusion) || true
  if [[ -z "$ruleset_id" ]]; then
    ruleset_id=$(api --paginate --slurp \
      "repos/${repository}/rulesets?includes_parents=true&targets=tag&per_page=100" |
      jq -er --arg name "autopus-${from_tag}-release-authority" \
        '[(add // [])[] | select(.name == $name and .target == "tag")] |
         if length == 1 then .[0].id else empty end') || true
  fi
  if [[ -z "$ruleset_state" && -n "$ruleset_id" ]]; then
    ruleset_state=$(api "repos/${repository}/rulesets/${ruleset_id}" \
      --jq 'if (.bypass_actors // []) == [] then "sealed" else "armed" end') || true
  fi
  if [[ -z "$tag_policy_id" ]]; then
    tag_policy_id=$(api --paginate --slurp \
      "repos/${repository}/environments/adk-companion-release/deployment-branch-policies?per_page=100" |
      jq -er --arg tag "$from_tag" '[.[].branch_policies[] | select(.name == $tag and .type == "tag")] |
        if length == 1 then .[0].id else empty end') || true
  fi
  for pair in "release-run:$release_run" "release-conclusion:$release_conclusion" \
    "canary-run:$canary_run" "canary-conclusion:$canary_conclusion" \
    "ruleset-id:$ruleset_id" "ruleset-state:$ruleset_state" "tag-policy-id:$tag_policy_id"; do
    [[ -n "${pair#*:}" ]] || fail "cannot measure --${pair%%:*} for ${from_tag}; pass it, or rerun with --no-runbooks"
  done
fi

# 2. Source files, then runbooks. The engine writes nothing unless every rule
# matched.
perl "$tools/advance-release-coordinate-apply.pl" --root "$repo_root" \
  --measurements "$measurements" --rules "$tools/advance-release-coordinate-rules.pl" \
  --from-tag "$from_tag" --from-phase "$from_phase" --to-tag "$to_tag" --to-phase "$to_phase" \
  --context-out "$work/context.json"
if [[ "$runbooks" -eq 1 ]]; then
  perl "$tools/advance-release-coordinate-runbook.pl" --root "$repo_root" \
    --measurements "$measurements" --context "$work/context.json" \
    --base "$(git rev-parse HEAD)" --date "$(date +%Y-%m-%d)" \
    --release-run "$release_run" --release-conclusion "$release_conclusion" \
    --canary-run "$canary_run" --canary-conclusion "$canary_conclusion" \
    --ruleset-id "$ruleset_id" --ruleset-state "$ruleset_state" \
    --tag-policy-id "$tag_policy_id" --note "$note"
fi

# 3. Shape checks on what moved: every shell file parses, Go stays gofmt-clean,
# and no source file passes the 300 counted-line limit. The coordinates file
# grows by one declaration per move; when it would pass 300, older pin blocks
# have to be carried to verify-public-key-lineage-shipped-pins.sh first.
changed=()
while IFS= read -r path; do changed+=("$path"); done < <(git diff --name-only)
for path in ${changed[@]+"${changed[@]}"}; do
  case "$path" in
    *.sh) bash -n "$path" || fail "$path no longer parses" ;;
    *.go) [[ -z "$(gofmt -l "$path")" ]] || fail "$path is not gofmt-clean" ;;
  esac
done
if [[ "$line_check" -eq 1 ]]; then
  sources=()
  for path in ${changed[@]+"${changed[@]}"}; do
    case "$path" in *.sh|*.go) sources+=("$path") ;; esac
  done
  go run ./cmd/source-lines --max 300 "${sources[@]}" ||
    fail 'a moved file passed 300 counted lines; carry older lineage pin blocks to the shipped-pins helper first'
fi

printf '\n%s\n' "moved ${from_phase}=${from_tag} -> ${to_phase}=${to_tag} from ${measurements}"
printf '%s\n' 'Next: review git diff, fill in the new runbook notes, run the release tests, commit.'

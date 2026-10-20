#!/usr/bin/env bash
set -euo pipefail
umask 077

# The release window is bound into provider_authority_digest. v0.50.125 bound
# the Codex CLI's 1050000 while production computes the Go default, so its
# managed active grant could never admit. release-prep.sh now declares the
# production window, takes ADK_RELEASE_MODEL_CONTEXT_WINDOW over ~/.codex, and
# refuses a window the pinned OMP catalog does not offer for the model.

tests_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
repo=$(cd -- "$tests_dir/../../.." && pwd)
lib="$repo/scripts/release-tools/release-prep-window-lib.sh"
wrapper="$repo/scripts/release-tools/release-prep.sh"
go_source="$repo/internal/cli/pipeline_omp_context_active_process.go"
fail() { printf 'release prep window test: %s\n' "$1" >&2; exit 1; }
contains() { grep -Fq -- "$2" "$1" || fail "$1 missing $2"; }
temp=$(mktemp -d "${TMPDIR:-/tmp}/release-prep-window.XXXXXX")
trap 'rm -rf -- "$temp"' EXIT
# shellcheck source=../../release-tools/release-prep-window-lib.sh
source "$lib"
unset ADK_RELEASE_MODEL_CONTEXT_WINDOW

production=$(release_prep_production_window "$go_source")
[[ "$production" == '262144' ]] || fail "production window is $production, not the Go default 262144"
printf 'const otherWindow = 262144\n' >"$temp/renamed.go"
if release_prep_production_window "$temp/renamed.go" >/dev/null 2>&1; then
  fail 'a renamed production constant still produced a window'
fi

catalog='{"models":[{"id":"gpt-6.1-sol","contextWindow":500000},{"id":"small","contextWindow":128000}]}'
codex="$temp/config.toml"
printf 'model = "gpt-6.1-sol"\nmodel_context_window = 1050000\n' >"$codex"
accepts() {
  local expected=$1; shift
  local actual
  actual=$("$@" 2>"$temp/stderr") || fail "refused an admissible window ($expected): $(<"$temp/stderr")"
  [[ "$actual" == "$expected" ]] || fail "resolved '$actual', expected '$expected'"
}
refuses() {
  local reason=$1; shift
  if "$@" >/dev/null 2>"$temp/stderr"; then fail "accepted a window that must be refused ($reason)"; fi
  grep -Fq -- "$reason" "$temp/stderr" || fail "refusal does not say '$reason': $(<"$temp/stderr")"
}

# The override wins over ~/.codex, and the production window is admitted.
accepts '262144 ADK_RELEASE_MODEL_CONTEXT_WINDOW' \
  env ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144 bash -c 'source "$1"; release_prep_context_window gpt-6.1-sol "$2" "$3" 262144' \
  _ "$lib" "$catalog" "$codex"
# Without an override ~/.codex is still read, as before.
printf 'model_context_window = 262144\n' >"$temp/aligned.toml"
accepts "262144 $temp/aligned.toml" release_prep_context_window gpt-6.1-sol "$catalog" "$temp/aligned.toml" 262144
# The Codex CLI window that sank v0.50.125 is larger than the catalog offers.
refuses 'exceeds the pinned OMP catalog window 500000' \
  release_prep_context_window gpt-6.1-sol "$catalog" "$codex" 262144
grep -Fq 'Export ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144' "$temp/stderr" ||
  fail 'the ~/.codex refusal does not name the override to export'
# The catalog window itself fits the model but is not what production computes.
refuses 'Export ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144' \
  env ADK_RELEASE_MODEL_CONTEXT_WINDOW=500000 bash -c 'source "$1"; release_prep_context_window gpt-6.1-sol "$2" "$3" 262144' \
  _ "$lib" "$catalog" "$codex"
refuses 'exceeds the pinned OMP catalog window 128000' \
  env ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144 bash -c 'source "$1"; release_prep_context_window small "$2" "$3" 262144' \
  _ "$lib" "$catalog" "$codex"
refuses 'states no context window for absent' \
  env ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144 bash -c 'source "$1"; release_prep_context_window absent "$2" "$3" 262144' \
  _ "$lib" "$catalog" "$codex"
for malformed in 4096 0262144 '262144 ' -262144 26e4 abc 99999999999; do
  refuses 'is not an integer of at least 8192' \
    env ADK_RELEASE_MODEL_CONTEXT_WINDOW="$malformed" bash -c \
    'source "$1"; release_prep_context_window gpt-6.1-sol "$2" "$3" 262144' _ "$lib" "$catalog" "$codex"
done
refuses 'is not an integer of at least 8192' \
  release_prep_context_window gpt-6.1-sol "$catalog" "$temp/missing.toml" 262144
refuses 'differs from the production runtime window' \
  env ADK_RELEASE_MODEL_CONTEXT_WINDOW=262144 bash -c 'source "$1"; release_prep_context_window gpt-6.1-sol "$2" "$3" bogus' \
  _ "$lib" "$catalog" "$codex"

# The wrapper resolves the window through this policy, from the production
# source and the pinned catalog it already fetched, and says which it used.
contains "$wrapper" '. scripts/release-tools/release-prep-window-lib.sh'
contains "$wrapper" 'release_prep_production_window internal/cli/pipeline_omp_context_active_process.go'
contains "$wrapper" 'release_prep_context_window "$model" "$catalog" "${HOME}/.codex/config.toml" "$production_window"'
contains "$wrapper" '"$model_context_window" "$window_origin"'
contains "$wrapper" '--model-context-window "$model_context_window"'
! grep -Fq 'model_context_window=$(awk' "$wrapper" || fail 'the wrapper still reads the window outside the policy'

(cd -- "$repo" && go run ./cmd/source-lines --max 300 "$lib" "$wrapper" \
  "$tests_dir/release-prep-window-test.sh" >/dev/null) || fail 'window policy sources exceed 300 code lines'
printf 'release prep window test: PASS\n'

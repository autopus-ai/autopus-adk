#!/usr/bin/env bash
# Context-window policy for release-prep.sh, kept apart so it is testable
# without a staged OMP, a gateway or the signing keys.
#
# The canary binds the window it declares into provider_authority_digest, and
# production admits managed active history only when its own runtime window
# yields the same digest. Production never reads ~/.codex: the pipeline backend
# leaves the window unset and normalizePipelineOMPBackendConfig applies
# pipelineOMPActiveDefaultContextWindow. A window taken from the Codex CLI
# config (1050000 for v0.50.125) therefore produced evidence that could never
# admit, so the declared window must be the production one and must fit the
# pinned OMP catalog for the release model.

# release_prep_production_window GO_SOURCE prints the production runtime window.
release_prep_production_window() {
  local window
  window=$(awk '/^const pipelineOMPActiveDefaultContextWindow = [1-9][0-9]*$/ { print $NF; exit }' "$1" 2>/dev/null)
  if [[ ! "$window" =~ ^[1-9][0-9]*$ || ${#window} -gt 10 ]]; then
    printf 'release prep: cannot read the production context window from %s\n' "$1" >&2
    return 1
  fi
  printf '%s\n' "$window"
}

# release_prep_context_window MODEL CATALOG_JSON CODEX_CONFIG PRODUCTION_WINDOW
# prints "WINDOW ORIGIN". ADK_RELEASE_MODEL_CONTEXT_WINDOW overrides the
# model_context_window in CODEX_CONFIG, so the operator never has to edit it.
release_prep_context_window() {
  local model=$1 catalog=$2 codex_config=$3 production=$4 window origin catalog_window
  if [[ -n "${ADK_RELEASE_MODEL_CONTEXT_WINDOW:-}" ]]; then
    window=$ADK_RELEASE_MODEL_CONTEXT_WINDOW
    origin='ADK_RELEASE_MODEL_CONTEXT_WINDOW'
  else
    window=$(awk -F'=' '/^model_context_window[[:space:]]*=/ { gsub(/[^0-9]/, "", $2); print $2; exit }' \
      "$codex_config" 2>/dev/null)
    origin=$codex_config
  fi
  # Digits only, no leading zero (bash arithmetic reads 0-prefixed values as
  # octal) and at most ten digits, before any arithmetic touches the value.
  if [[ ! "$window" =~ ^[1-9][0-9]*$ || ${#window} -gt 10 ]] || (( window < 8192 )); then
    printf 'release prep: model context window %s from %s is not an integer of at least 8192\n' \
      "${window:-<unset>}" "$origin" >&2
    return 1
  fi
  catalog_window=$(jq -r --arg id "$model" \
    'first(.. | objects | select(.id? == $id) | .contextWindow | numbers) // empty' <<<"$catalog" 2>/dev/null) ||
    catalog_window=''
  if [[ ! "$catalog_window" =~ ^[1-9][0-9]*$ || ${#catalog_window} -gt 10 ]]; then
    printf 'release prep: the pinned OMP catalog states no context window for %s\n' "$model" >&2
    return 1
  fi
  if (( window > catalog_window )); then
    printf 'release prep: model context window %s from %s exceeds the pinned OMP catalog window %s for %s\n' \
      "$window" "$origin" "$catalog_window" "$model" >&2
    printf '  Export ADK_RELEASE_MODEL_CONTEXT_WINDOW=%s, the production runtime window\n' "$production" >&2
    return 1
  fi
  if [[ ! "$production" =~ ^[1-9][0-9]*$ || ${#production} -gt 10 ]] || (( window != production )); then
    printf 'release prep: model context window %s from %s differs from the production runtime window %s;\n' \
      "$window" "$origin" "$production" >&2
    printf '  the evidence would bind a window production never computes, so managed active history\n' >&2
    printf '  would never admit. Export ADK_RELEASE_MODEL_CONTEXT_WINDOW=%s\n' "$production" >&2
    return 1
  fi
  printf '%s %s\n' "$window" "$origin"
}

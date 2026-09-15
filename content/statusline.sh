#!/bin/bash
# 🐙 Autopus-ADK Statusline for Claude Code
# Displays: autopus version, model, context %, token usage, 5h/7d rate limits, cost
# Includes new version notification with GitHub API caching

set -euo pipefail

# --- Configuration ---
REPO="autopus-ai/autopus-adk"
CACHE_DIR="/tmp/autopus-statusline"
VERSION_CACHE="$CACHE_DIR/latest_version"
CACHE_MAX_AGE=3600  # 1 hour

# --- Colors ---
GREEN='\033[32m'
YELLOW='\033[33m'
RED='\033[31m'
CYAN='\033[36m'
DIM='\033[2m'
BOLD='\033[1m'
RESET='\033[0m'

# --- Read JSON from stdin ---
input=$(cat)

# --- Parse fields with jq ---
model=$(echo "$input" | jq -r '.model.display_name // "?"')
ctx_pct=$(echo "$input" | jq -r '.context_window.used_percentage // 0' | cut -d. -f1)
input_tokens=$(echo "$input" | jq -r '.context_window.total_input_tokens // 0')
output_tokens=$(echo "$input" | jq -r '.context_window.total_output_tokens // 0')
five_h=$(echo "$input" | jq -r '.rate_limits.five_hour.used_percentage // empty')
seven_d=$(echo "$input" | jq -r '.rate_limits.seven_day.used_percentage // empty')
five_h_reset=$(echo "$input" | jq -r '.rate_limits.five_hour.resets_at // empty')
seven_d_reset=$(echo "$input" | jq -r '.rate_limits.seven_day.resets_at // empty')

# --- Helper: format token count (e.g., 15234 -> 15.2K) ---
format_tokens() {
  local n=$1
  if [ "$n" -ge 1000000 ]; then
    printf "%.1fM" "$(echo "scale=1; $n / 1000000" | bc)"
  elif [ "$n" -ge 1000 ]; then
    printf "%.1fK" "$(echo "scale=1; $n / 1000" | bc)"
  else
    echo "$n"
  fi
}

# --- Helper: color by percentage (green < 50, yellow 50-80, red > 80) ---
color_pct() {
  local pct=$1
  local rounded
  rounded=$(printf '%.0f' "$pct" 2>/dev/null || echo "0")
  if [ "$rounded" -lt 50 ]; then
    printf "${GREEN}%s%%${RESET}" "$rounded"
  elif [ "$rounded" -lt 80 ]; then
    printf "${YELLOW}%s%%${RESET}" "$rounded"
  else
    printf "${RED}%s%%${RESET}" "$rounded"
  fi
}

# --- Helper: progress bar (8 chars wide) ---
progress_bar() {
  local pct=$1
  local width=8
  local filled=$((pct * width / 100))
  [ "$filled" -gt "$width" ] && filled=$width
  local empty=$((width - filled))
  local bar=""
  for ((i = 0; i < filled; i++)); do bar+="█"; done
  for ((i = 0; i < empty; i++)); do bar+="░"; done

  if [ "$pct" -lt 50 ]; then
    printf "${GREEN}%s${RESET}" "$bar"
  elif [ "$pct" -lt 80 ]; then
    printf "${YELLOW}%s${RESET}" "$bar"
  else
    printf "${RED}%s${RESET}" "$bar"
  fi
}

# --- Helper: format reset time as countdown ---
format_reset() {
  local reset_ts=$1
  if [ -z "$reset_ts" ]; then
    echo ""
    return
  fi
  local now
  now=$(date +%s)
  local diff=$((reset_ts - now))
  if [ "$diff" -le 0 ]; then
    echo "now"
    return
  fi
  local hours=$((diff / 3600))
  local mins=$(((diff % 3600) / 60))
  if [ "$hours" -gt 0 ]; then
    printf "%dh%dm" "$hours" "$mins"
  else
    printf "%dm" "$mins"
  fi
}

# --- Version check (cached, non-blocking) ---
get_current_version() {
  # --short bypasses Banner/lipgloss entirely, avoiding OSC 11 hang
  local ver
  ver=$(auto version --short 2>/dev/null)
  echo "${ver:-unknown}"
}

check_latest_version() {
  mkdir -p "$CACHE_DIR" 2>/dev/null || true

  # Check if cache is stale
  if [ -f "$VERSION_CACHE" ]; then
    local cache_age
    local cache_mtime
    cache_mtime=$(stat -f %m "$VERSION_CACHE" 2>/dev/null || stat -c %Y "$VERSION_CACHE" 2>/dev/null || echo "0")
    cache_age=$(($(date +%s) - cache_mtime))
    if [ "$cache_age" -lt "$CACHE_MAX_AGE" ]; then
      cat "$VERSION_CACHE"
      return
    fi
  fi

  # Fetch latest version from GitHub (background, non-blocking)
  (
    latest=$(curl -sf --max-time 3 \
      "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
      | jq -r '.tag_name // empty' | sed 's/^v//')
    if [ -n "$latest" ]; then
      echo "$latest" > "$VERSION_CACHE"
    fi
  ) &>/dev/null &

  # Return cached version if exists, empty otherwise
  [ -f "$VERSION_CACHE" ] && cat "$VERSION_CACHE" || echo ""
}

# --- Compare semver: returns 1 if $1 > $2 ---
version_gt() {
  local v1=$1 v2=$2
  [ "$v1" = "$v2" ] && return 1
  local IFS='.'
  read -ra a <<< "$v1"
  read -ra b <<< "$v2"
  for i in 0 1 2; do
    local x=${a[$i]:-0} y=${b[$i]:-0}
    if [ "$x" -gt "$y" ] 2>/dev/null; then return 0; fi
    if [ "$x" -lt "$y" ] 2>/dev/null; then return 1; fi
  done
  return 1
}

# --- Build output ---
current_ver=$(get_current_version)
latest_ver=$(check_latest_version)

# Version segment
ver_segment="🐙 v${current_ver}"
if [ -n "$latest_ver" ] && version_gt "$latest_ver" "$current_ver"; then
  ver_segment="${ver_segment} ${YELLOW}⬆ v${latest_ver}${RESET}"
fi

# Model segment
model_segment="${BOLD}${model}${RESET}"

# Context segment
ctx_bar=$(progress_bar "$ctx_pct")
ctx_segment="Ctx: ${ctx_bar} ${ctx_pct}%"

# Token segment
in_fmt=$(format_tokens "$input_tokens")
out_fmt=$(format_tokens "$output_tokens")
token_segment="${DIM}↓${in_fmt} ↑${out_fmt}${RESET}"

# Rate limit segments
limits=""
if [ -n "$five_h" ]; then
  five_h_color=$(color_pct "$five_h")
  five_h_countdown=$(format_reset "$five_h_reset")
  limits="5h:${five_h_color}"
  [ -n "$five_h_countdown" ] && limits="${limits}${DIM}(${five_h_countdown})${RESET}"
fi
if [ -n "$seven_d" ]; then
  seven_d_color=$(color_pct "$seven_d")
  seven_d_countdown=$(format_reset "$seven_d_reset")
  [ -n "$limits" ] && limits="${limits} "
  limits="${limits}7d:${seven_d_color}"
  [ -n "$seven_d_countdown" ] && limits="${limits}${DIM}(${seven_d_countdown})${RESET}"
fi

# --- Assemble line ---
line="${ver_segment} │ ${model_segment} │ ${ctx_segment} │ ${token_segment}"

if [ -n "$limits" ]; then
  line="${line} │ ${limits}"
fi

# --- Output ---
printf "%b\n" "$line"

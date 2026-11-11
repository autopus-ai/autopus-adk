#!/bin/sh
# Fake gh for the cireport tests: logs its argv, answers `gh auth status`
# with $FAKE_GH_AUTH_EXIT, and serves `gh api <path>` from $FAKE_GH_DIR,
# where the file name is the path with / ? & = % replaced by _.
echo "$*" >> "$FAKE_GH_LOG"
case "$1" in
auth) exit "${FAKE_GH_AUTH_EXIT:-0}" ;;
esac
for last; do :; done
key=$(printf '%s' "$last" | tr '/?&=%' '_____')
if [ -f "$FAKE_GH_DIR/$key.json" ]; then
  cat "$FAKE_GH_DIR/$key.json"
  exit 0
fi
echo "gh: Not Found (HTTP 404)" >&2
exit 1

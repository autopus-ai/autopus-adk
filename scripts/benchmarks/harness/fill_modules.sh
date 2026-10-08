#!/bin/bash
# Trusted module step of the signed live lane (SPEC-HARNEVAL-003 REQ-HR-08).
#
#   fill_modules.sh <new module cache> <40-hex commit>...
#
# The runner, its graders and the signer's baseline rebuild download modules
# offline from a file:// proxy; on a fresh runner no warm local module cache
# can serve as one. This fills a new cache, outside every sandbox and from
# the checkout in the working directory: for each commit, `go mod download`
# in its git archive with the environment's Go proxy and checksum database,
# GOFLAGS=-mod=mod as the runner's own download, and a go.sum that must not
# change. Callers then pass file://<cache>/cache/download as the proxy.
set -euo pipefail

if [ "$#" -lt 2 ]; then
  echo "fill_modules: usage: fill_modules.sh <new module cache> <commit>..." >&2
  exit 1
fi
cache=$1
shift
if [ -e "$cache" ] || [ -L "$cache" ]; then
  echo "fill_modules: $cache already exists" >&2
  exit 1
fi
for revision in "$@"; do
  if ! [[ "$revision" =~ ^[0-9a-f]{40}$ ]]; then
    echo "fill_modules: '$revision' is not a 40-hex commit" >&2
    exit 1
  fi
done
mkdir -p "$cache"
trees=$(mktemp -d)
trap 'rm -rf "$trees"' EXIT
for revision in "$@"; do
  tree="$trees/$revision"
  mkdir "$tree"
  git archive --format=tar "$revision" | tar -x -f - -C "$tree"
  cp "$tree/go.sum" "$trees/$revision.go.sum"
  (cd "$tree" && env GOMODCACHE="$cache" GOFLAGS=-mod=mod GOWORK=off GOTOOLCHAIN=local go mod download)
  if ! cmp -s "$tree/go.sum" "$trees/$revision.go.sum"; then
    echo "fill_modules: go mod download changed the go.sum of $revision" >&2
    exit 1
  fi
done

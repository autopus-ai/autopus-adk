#!/bin/bash
# Pre-release QA agent matrix: for each agent and each fixture with a planted
# bug, run the whole chain (intent -> recommended answers -> qa go --auto)
# in a scratch repository, then judge the result with a hidden oracle the
# agent never sees. A passing loop is not enough: the oracle catches fixes
# that only satisfy the agent's own scenarios.
#
# Usage: scripts/qa-agent-matrix/run.sh [-a "claude codex"] [-f "wordtop"] [-o OUTDIR] [-b AUTO_BIN]
# Runs real agent CLIs, so it costs model usage; it is never run by CI.
# Exit status: 0 when every cell passes its oracle, 1 otherwise.

agents="claude codex agy opencode"
fixtures="wordtop itemsapi"
out=""
auto_bin=""
while getopts "a:f:o:b:" opt; do
  case "$opt" in
    a) agents="$OPTARG" ;;
    f) fixtures="$OPTARG" ;;
    o) out="$OPTARG" ;;
    b) auto_bin="$OPTARG" ;;
    *) sed -n '2,11p' "$0"; exit 2 ;;
  esac
done

here=$(cd "$(dirname "$0")" && pwd)
repo=$(cd "$here/../.." && pwd)
[ -n "$out" ] || out=$(mktemp -d "${TMPDIR:-/tmp}/qa-agent-matrix.XXXXXX")
mkdir -p "$out/bin" || exit 2

# The chain shells out to `auto qa exec`, so the binary under test must be
# the one named auto on PATH.
if [ -z "$auto_bin" ]; then
  echo "building auto from $repo"
  (cd "$repo" && go build -o "$out/bin/auto" ./cmd/auto) || exit 2
else
  cp "$auto_bin" "$out/bin/auto" || exit 2
fi
export PATH="$out/bin:$PATH"
export GIT_AUTHOR_NAME="QA matrix" GIT_AUTHOR_EMAIL="qa-matrix@example.invalid"
export GIT_COMMITTER_NAME="QA matrix" GIT_COMMITTER_EMAIL="qa-matrix@example.invalid"

request_for() {
  case "$1" in
    wordtop) echo "wordtop이라는 CLI를 만들었어. 텍스트 파일을 주면 가장 많이 나온 단어를 많이 나온 순으로 N개(-n, 기본 3) 보여줘. 'The'와 'the'는 같은 단어로 쳐야 해. 횟수가 같으면 알파벳 순. 파일이 없으면 에러로 끝나야 해." ;;
    itemsapi) echo "itemsapi라는 HTTP 서버를 만들었어. PORT 환경변수의 포트에서 떠. POST /items 로 이름(본문 텍스트)을 보내면 저장하고 201을 돌려줘. GET /items 는 저장된 이름을 넣은 그대로, 넣은 순서대로 쉼표로 이어서 돌려줘. 없는 메서드는 405야." ;;
  esac
}

results="$out/results.tsv"
printf "agent\tfixture\tloop\toracle\tcases\tseconds\n" > "$results"
failed=0
for fixture in $fixtures; do
  for agent in $agents; do
    cell="$out/$fixture-$agent"
    log="$cell.log"
    rm -rf "$cell" && mkdir -p "$cell" || exit 2
    cp "$here/testdata/$fixture/"* "$cell/" || exit 2
    (cd "$cell" && git init -q -b main && git add -A && git commit -q -m "fixture: $fixture") || exit 2
    start=$(date +%s)
    id="INTENT-$fixture"
    (
      cd "$cell" || exit 2
      auto qa intent --project-dir . --agent "$agent" --id "$id" --request "$(request_for "$fixture")" &&
        auto qa intent answer "$id" --recommended --project-dir . &&
        auto qa go "$id" --project-dir . --agent "$agent" --auto
    ) > "$log" 2>&1
    loop_rc=$?
    seconds=$(( $(date +%s) - start ))
    loop="passed"
    [ "$loop_rc" -eq 0 ] || loop="failed($loop_rc)"

    branch=$(cd "$cell" && git for-each-ref --sort=-committerdate --format='%(refname:short)' refs/heads/autopus/qa-loop | head -1)
    oracle="not-run"
    cases=0
    if [ -n "$branch" ]; then
      judged="$cell.judged"
      rm -rf "$judged" && mkdir -p "$judged"
      (cd "$cell" && git archive "$branch" | tar -x -C "$judged")
      cases=$(cat "$judged"/.autopus/qa/test-scenarios/*.yaml 2>/dev/null | grep -c "type: exec")
      cp "$here/testdata/oracles/${fixture}_oracle_test.go" "$judged/"
      if (cd "$judged" && go test ./... > "$cell.oracle.log" 2>&1); then
        oracle="pass"
      else
        oracle="FAIL"
      fi
    fi
    if [ "$oracle" != "pass" ]; then
      failed=1
    fi
    printf "%s\t%s\t%s\t%s\t%s\t%s\n" "$agent" "$fixture" "$loop" "$oracle" "$cases" "$seconds" >> "$results"
    printf "%-9s %-9s loop=%-12s oracle=%-8s cases=%-3s %ss\n" "$agent" "$fixture" "$loop" "$oracle" "$cases" "$seconds"
  done
done
echo "results: $results (logs beside it)"
exit "$failed"

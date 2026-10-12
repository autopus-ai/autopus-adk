# Probe A1 (SPEC-MODELUX-002): 감지 플랫폼이 있는 update의 quality 블록

- 실행: 2026-10-12, macOS, 감지 대상 CLI(antigravity-cli, opencode, omp)가 설치된 기계. 바이너리는 worktree HEAD `44773355` 빌드(`auto 0.50.125-0.20261012003416-447733555889+dirty`).
- 경로는 커밋 전에 바꿨다: scratch 루트 → `<scratch>`, 홈 → `$HOME`, 사용자 이름 → `<user>`.
- 판정: 파일 SHA-256은 바뀌었고 `quality:` 블록 SHA-256은 같다. diff는 `platforms` 3줄 추가뿐이다. → PASS

## SHA-256

| 대상 | 실행 전 | 실행 후 |
|---|---|---|
| `autopus.yaml` | `b7f6c3aaeddc82cf96165297571b797c8a049cd85d9b55746defa474bd56bd3e` | `5af0a6e9ae5766b96887c9a301544fd2067c87c2c47fa5c0ba663d0df0baff7b` |
| `quality:` 블록(`qblock.py` 추출) | `a8ff97ba87e8c57ca6822cbd045ffc18c2c5f631a1339b0a96b1868917fb5427` | `a8ff97ba87e8c57ca6822cbd045ffc18c2c5f631a1339b0a96b1868917fb5427` |

## 파일 diff (`diff before.yaml after.yaml`)

```diff
5a6,8
>     - antigravity-cli
>     - opencode
>     - omp
```

## update 출력

```text
  + 새 플랫폼 감지: antigravity-cli, opencode, omp
Update targets (5): claude-code, codex, antigravity-cli, opencode, OMP (Oh My Pi)
  → Updating claude-code...
  ✓ claude-code updated
  → Updating codex...
  [codex] extended skills: 52 compatible, 2 platform-skipped (agent-teams, harness-workflow)
  ✓ codex updated
  → Updating antigravity-cli...
  [gemini] extended skills: 52 compatible, 2 platform-skipped (agent-teams, harness-workflow)
  ✓ antigravity-cli updated
  → Updating opencode...
  ✓ opencode updated
  → Updating OMP (Oh My Pi)...
  ✓ omp updated
  • Claude statusLine: Autopus statusLine enabled
Update complete: 5 platform(s) updated
```

## run.sh

```bash
#!/bin/bash
set -u
S=<scratch>
AUTO=$S/mux-bin/auto
P=$(mktemp -d $S/mux2-probe/proj.XXXX)
cd "$P" && git init -q
export CODEX_HOME=$S/mux2-probe/codex-home; mkdir -p "$CODEX_HOME"
echo "== init --yes"; "$AUTO" init --yes --platforms claude-code,codex </dev/null >init.log 2>&1; echo "exit=$?"
cp autopus.yaml before.yaml
python3 $S/mux2-probe/qblock.py before.yaml > qb-before.txt
echo "== update --yes"; "$AUTO" update --yes </dev/null >update.log 2>&1; echo "exit=$?"
cp autopus.yaml after.yaml
python3 $S/mux2-probe/qblock.py after.yaml > qb-after.txt
shasum -a 256 before.yaml after.yaml qb-before.txt qb-after.txt
echo "== quality block cmp"; cmp qb-before.txt qb-after.txt && echo "QUALITY BLOCK SAME" || echo "QUALITY BLOCK CHANGED"
echo "== full diff"; diff before.yaml after.yaml | head -40
echo "== update log"; sed -n 1,40p update.log
echo "PROJECT=$P"
```

## qblock.py (블록 추출: `quality:` 줄부터 다음 1열 key 직전까지)

```python
import sys,re
# Print the top-level `quality:` block: from the `quality:` line up to (excluding) the next column-1 key line.
lines=open(sys.argv[1],encoding='utf-8').read().split('\n')
out=[];inside=False
for l in lines:
    if re.match(r'^quality:',l): inside=True; out.append(l); continue
    if inside:
        if re.match(r'^[A-Za-z_][^:]*:',l): break
        out.append(l)
sys.stdout.write('\n'.join(out))
```

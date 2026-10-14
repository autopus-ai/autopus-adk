# SPEC-MODELUX-003 task.disabledAgents 다섯 이름 probe (omp 18.6.1, 17.2.7, 2026-10-14)

결정: autopus 모드의 `task.disabledAgents`는 `task`를 뺀 bundled 에이전트 전부다. 지원하는 두 omp 버전을 합쳐 `[scout, security-reviewer, sonic, designer, librarian]`이다.

경로는 치환했다. `<scratch>`는 세션 scratch이고 `<probe>`는 `<scratch>/mux3-probe/`다. 실제 model 호출은 없고, 쓰기는 모두 `<scratch>` 안에서만 일어났다.

## bundled 에이전트
- 17.2.7(CI pin, sha256 `cd2f4754…`): `omp agents unpack --dir <scratch>/omp-probe/unpack/bundled --json` → `"total": 7`. designer, librarian, reviewer, scout, security-reviewer, sonic, task.
- 18.6.1: `evidence/probe-omp-18.6.1.md` P0 → `"total": 5`. reviewer, scout, security-reviewer, sonic, task.

## 18.6.1 설정 되읽기 (`<probe>/disabled5`)
환경은 `env -i PATH HOME TMPDIR XDG_*(모두 <probe>/disabled5/sb/*) PI_CODING_AGENT_DIR LANG=C LC_ALL=C`이다.
- D1: project `.omp/config.yml`에 다섯 이름을 두고 cwd를 그 project로 한다. `omp config get task.disabledAgents --json` → exit 0, `"type": "array"`, value는 다섯 이름이 그 순서 그대로이고 stderr는 0 bytes다.
- D2: 같은 내용의 overlay를 `PI_CONFIG_FILES`로 주었다. 결과가 같다(exit 0, 다섯 이름, stderr 0 bytes).
- 정적 확인(`<probe>/omp-strings.txt`): 설정 정의 `task.disabledAgents`는 `type: "array"`, `default: []`이고 item 검증이 없다. task tool 목록은 `o.filter((a) => !e.disabledAgents.includes(a.name))`로 이름만 거른다. 그래서 번들하지 않는 이름은 아무것도 거르지 않는 항목이다.

## 18.6.1 세션 (loopback fake provider)
- D3: 위 다섯 이름을 claim한 autopus 모드 project를 `Adapter.Generate`로 만들었다. 같은 격리 env와 macOS loopback 전용 sandbox에서 `omp --mode rpc --tools task …`를 한 번 돌렸다. 18.6.1에는 `hub` tool이 없어 17.2.7 smoke의 `--tools task,hub`는 `Unknown tool in --tools: hub`로 exit 2가 된다.
- fake provider가 받은 첫 parent 요청의 task tool "# Available Agents"(`- \`name\`: …` 형식)는 17개다: Autopus 16개와 task. scout, security-reviewer, sonic, designer, librarian은 없다. 오류나 경고 없이 turn이 `stop`으로 끝났다.

## 17.2.7 세션 (CI smoke)
- D4: `TestOMPAutopusAgentSurfaceLiveSmoke`(pinned 17.2.7, fake provider 응답 한 번)의 task tool "# Available Agents"(`### name` 형식)는 정확히 17개다(Autopus 16개와 task). 다섯 이름이 모두 없다.
- 이 결정 전에는 designer와 librarian이 목록에 남아 19개였다(같은 smoke, d76f06e3).

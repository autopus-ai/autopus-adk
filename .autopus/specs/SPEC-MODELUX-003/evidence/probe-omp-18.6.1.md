# SPEC-MODELUX-003 OMP probe 기록 (omp 18.6.1, 2026-10-12)

경로는 치환했다: `<scratch>` = 세션 scratch의 `mux3-probe/`. 모델 호출과 provider 세션은 없다. 모든 쓰기는 `<scratch>` 안이다.

## 환경 격리
`env -i PATH HOME=<scratch>/proj/sb TMPDIR XDG_CONFIG_HOME XDG_DATA_HOME XDG_STATE_HOME XDG_CACHE_HOME(모두 <scratch>/proj/sb/*) LANG=C LC_ALL=C PI_CODING_AGENT_DIR=<scratch>/profile`.
`omp config get`은 `--config` 대신 `PI_CONFIG_FILES`로 overlay를 준다(`pkg/adapter/omp/omp_model_probe_process.go:145-151`).

## P0 bundled agent 정의
`omp agents unpack --dir <scratch>/bundled --json` → `"total": 5`, written: reviewer.md, scout.md, security-reviewer.md, sonic.md, task.md.
- reviewer: tools read, find, grep, glob, bash, lsp, web_search, ast_grep, yield / spawns [scout] / model ["@slow"] / output.properties: overall_correctness(enum correct, incorrect), explanation(string), confidence(number) / optionalProperties.findings.elements: title, body, priority, confidence, file_path, line_start, line_end.
- task: spawns "*", model ["@task"], thinkingLevel auto. scout: model ["@smol"], thinkingLevel medium, output(summary, files, architecture). sonic: model ["@smol"], thinkingLevel medium. security-reviewer: read-only tools, output(coverage_summary, findings, reviewed_paths, deferred).
- `omp agents --help`: ACTION은 `unpack`뿐이다. 발견 목록을 보여 주는 읽기 전용 명령이 없다.

## P3 설정 되읽기
overlay.yml:
```yaml
task:
  agentModelOverrides:
    planner: openai-codex/gpt-6-astra:max
    reviewer: anthropic/claude-opus-5-5:high
    task: openai-codex/gpt-6-astra:xhigh
  disabledAgents: [scout, security-reviewer, sonic]
```
- P3a `PI_CONFIG_FILES=<scratch>/proj/overlay.yml omp config get task.agentModelOverrides --json` → exit 0, `"type": "record"`, value = 위 세 항목 그대로(`planner`는 bundled 이름이 아니다).
- P3b 같은 overlay로 `task.disabledAgents` → exit 0, `"type": "array"`, value `["scout", "security-reviewer", "sonic"]`.
- P3c overlay 없이 → `task.disabledAgents` `[]`, `task.agentModelOverrides` `{}`(기본값).
- P3d `omp --config <overlay> config get task.disabledAgents --json` → `[]`. 18.6.1에서도 `config get`은 `--config`를 무시한다.

## 정적 확인 (`strings -n 8` 덤프, 줄 번호 S:n)
- S:524490- `AI()` 발견 순서: project `.omp/agents`(가장 가까운 것) → user → omp-plugins → claude-plugins → bundled `wR()`. 이름이 먼저 나온 것이 이긴다(Set).
- S:359871- frontmatter: `name`·`description`이 없으면 null(건너뜀), 예약 이름 거부, tools·spawns·output·thinkingLevel|thinking·model·blocking·autoloadSkills·prewalk·advisor.
- S:526773- dispatch preflight: `agent = e.agent?.trim() || defaultAgent`, `disabledAgents`에 있으면 `Agent "<x>" is disabled in settings` 오류, override는 `agentModelOverrides[<agent 이름>]`.
- S:756787- `EPi` 우선순위: per-call model > `task.agentModelOverrides[name]` > frontmatter model > 부모 세션 model.
- S:564994- task tool 목록은 `disabledAgents`를 뺀다. S:526717- outputSchema 우선순위: caller > agent `output` > session.
- S:780806-780816 OMP `/review`는 `agent: "reviewer"`로 1개 task를 보내고, S:338744-339163 renderer는 `overall_correctness`, `confidence`를 읽는다.
- S:466140-466160 각 설정 level 디렉터리의 `settings.json`과 `config.yml`을 읽는다(project `.omp/config.yml` 포함).

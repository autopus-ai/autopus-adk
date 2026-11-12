---
name: disk-triage
description: 디스크 여유 공간이 부족하거나 빠르게 줄어들 때, 무엇이 늘었는지 먼저 확인하고 내가 만든 재생성 가능한 것만 정리하는 디스크 분류 스킬
triggers:
  - disk triage
  - disk full
  - low disk
  - no space left
  - ENOSPC
  - 디스크 부족
  - 디스크 정리
  - 용량 부족
category: quality
level1_metadata: "auto disk status 증가 표로 원인 확인, 내 것만 정리, 남의 세션·백업·VM 디스크·공유 캐시는 묻고 지움, 무거운 작업 일시 중지, lease로 실행"
---

# Disk Triage Skill

여러 에이전트 세션이 한 디스크를 나눠 쓰면 남은 공간이 한 시간 안에 수십 GiB씩 오르내릴 수 있습니다.
공간이 바닥나면 빌드와 테스트가 `no space left on device`(ENOSPC)로 실패하고, 그 실패는 코드 문제처럼
보여서 엉뚱한 수정을 부릅니다. 이 스킬은 공간이 부족할 때 **원인을 먼저 확인하고, 내가 만든 것 중 다시
만들 수 있는 것만 정리하는** 순서를 정합니다.

예를 들어 한 공유 머신에서 에이전트 세션 여러 개가 동시에 빌드를 돌리자 여유 공간이 80 GiB에서 1 GiB
아래로 떨어졌다가 다시 올라갔습니다. 그 사이 테스트 실행 여러 개가 ENOSPC로 무효가 됐고, 무엇이 공간을
차지했는지는 아무도 몰랐습니다. 이 스킬의 첫 단계가 증가 표를 읽는 이유입니다.

## 언제 쓰나

- UserPromptSubmit에 `Disk alert:`가 붙었을 때
- PreToolUse 디스크 게이트가 명령을 막고 `Disk gate:` 사유를 돌려줬을 때
- 빌드나 테스트가 ENOSPC, `No space left on device`, `disk quota exceeded`로 실패했을 때
- 작업을 시작하기 전에 큰 빌드, 새 worktree, 병렬 에이전트가 필요할 때

## 1단계: 무엇이 늘었는지 먼저 읽는다

```bash
auto disk status
```

출력에서 세 부분을 봅니다.

1. 첫 줄의 여유 공간과 바닥선(`disk.min_free_gb`) 판정
2. 소비자 목록: Go 빌드 캐시, go build 작업 디렉터리, linked worktree(그 안의 node_modules와 cargo
   target), 세션 scratchpad와 그 안의 1 GiB 이상 파일, 그리고 이 머신에 있는 패키지 캐시(Go 모듈, npm,
   pnpm, yarn, bun, cargo, pip, uv, Gradle, Maven), Xcode DerivedData와 시뮬레이터(macOS), Docker
   Desktop, colima, lima, podman의 VM 디스크, `/var/lib/docker`(Linux, 읽을 수 있을 때)
3. `consumer growth since ...` 표: 지난 한 시간(또는 그보다 오래된 가장 최근 표본) 동안 많이 늘어난
   순서입니다. 예: `colima +38.0 GiB in 52m`

증가 표의 맨 위가 지금 공간을 먹는 원인입니다. 전체 크기가 큰 소비자가 아니라 **늘어나는** 소비자를
봅니다. 100 GiB짜리 캐시가 그대로라면 그것은 원인이 아닙니다.

`at least ..., walk cut at its time budget`로 표시된 소비자는 시간 예산 안에 다 세지 못한 하한값입니다.
증가 표에는 들어가지 않습니다. CI 러너 작업 디렉터리처럼 이 머신에만 있는 경로는
`disk.consumers.extra_roots`에 적으면 함께 잽니다.

```yaml
disk:
  consumers:
    extra_roots:
      - /path/to/runner/_work
```

읽을 수 없는 경로는 0이 아니라 `unavailable (unreadable)`로 나옵니다.

## 2단계: 무거운 작업을 멈추고, 이끄는 에이전트에게도 멈추라고 알린다

공간이 바닥선 아래면 빌드, 테스트, 패키지 설치, 컨테이너 빌드, `git worktree add`를 새로 시작하지
않습니다. 다른 에이전트를 이끌고 있다면 각 에이전트에게 무거운 작업을 멈추라고 직접 알립니다. Claude
Code에서는 디스크 게이트가 하위 에이전트의 Bash 호출에도 같이 적용되지만, 이미 돌고 있는 명령은 막지
못합니다.

## 3단계: 내가 만든 것 중 다시 만들 수 있는 것만 정리한다

```bash
auto disk reclaim            # 먼저 dry run으로 목록을 본다
auto disk reclaim --apply    # 목록이 맞으면 지운다
auto disk trim --apply       # Go 빌드 캐시의 오래된 항목
```

`auto disk reclaim`이 지우는 것:

- 기본 원격 브랜치(origin/HEAD 또는 origin/main)에 이미 들어간 깨끗한 linked worktree. 추적하지 않은
  파일이 없고, 실행 중인 어떤 프로세스의 작업 디렉터리도 아니며, `disk.reclaim.worktree_idle_minutes`
  넘게 쓰이지 않아야 합니다. `git worktree remove`를 `--force` 없이 쓰고 branch는 남깁니다.
- Claude Code가 `claude agent agent-<id> (pid N start T)`로 잠근 agent worktree. 깨끗하고 merge된
  경우에만 지웁니다. 잠근 프로세스가 사라졌으면(pid와 시작 시각으로 판별) 바로, 아직 살아 있으면 idle
  기준을 넘고 아무 프로세스도 그 안에 없을 때 잠금을 풀고 지웁니다. dirty하거나 merge되지 않은 것은 절대
  지우지 않습니다.
- 쉬고 있는 worktree 안의 node_modules, .next, dist(옆에 package.json이 있을 때), cargo target. git이
  무시하는 디렉터리만 지웁니다.
- 오래 쓰이지 않은 세션 scratchpad 디렉터리

Claude Code에서는 Stop hook(`auto verify stop`)이 매 턴 끝에 안전한 부분만 자동으로 정리합니다.
merge되고 깨끗한 worktree와 쉬고 있는 재생성 가능 디렉터리만 대상이고, 사용자 전체에서 30분에 한 번
(`disk.reclaim.auto_interval_minutes`) 백그라운드로 돌아서 턴을 늦추지 않습니다. 끄려면
`disk.reclaim.auto: false` 또는 `AUTOPUS_DISK_AUTO_RECLAIM=off`를 씁니다. 지운 기록은 `auto disk log`에
`auto_reclaim`으로 남습니다.

## 4단계: 남의 것과 되돌릴 수 없는 것은 묻는다

다음은 스스로 지우지 않고 사용자에게 먼저 묻습니다.

- **다른 세션의 scratchpad와 worktree.** 지금 작업 중인 다른 에이전트의 것일 수 있습니다. 내 세션
  디렉터리와 내가 만든 worktree만 내 것입니다.
- **scratchpad의 대용량 파일.** `auto disk reclaim`은 1 GiB 이상 파일을 나열만 합니다. 백업일 수 있으므로
  사용자가 확인한 뒤에만 `--include-large-files`로 지웁니다.
- **VM 디스크(colima, lima, podman machine, Docker Desktop).** 이미지와 볼륨이 모두 그 안에 있습니다.
  `docker system prune`이나 VM 삭제는 다른 프로젝트의 데이터를 지웁니다.
- **다른 세션이 쓰는 공유 캐시.** `go clean -cache`, `npm cache clean --force`, `cargo clean`은 지금 돌고
  있는 다른 빌드를 깨뜨립니다. Go 빌드 캐시는 쉬는 항목만 지우는 `auto disk trim`을 씁니다.

물을 때는 무엇이 얼마나 늘었는지(증가 표의 줄), 지우면 무엇을 잃는지, 다시 만드는 데 무엇이 드는지를
함께 적습니다.

## 5단계: 꼭 돌려야 하는 무거운 작업은 lease로 돌린다

공간이 부족해도 꼭 필요한 빌드가 있으면 머신 전체 예약을 잡고 실행합니다.

```bash
auto disk lease --need 8 -- go test ./...
auto disk lease --for release -- make release
```

lease는 여유 공간에서 다른 lease들이 잡은 예약과 내 명령의 필요량을 뺀 값이 `disk.min_free_gb` 이상일
때만 명령을 시작합니다. 모자라면 한 번 회수하고(`disk.lease.reclaim_on_shortage`) 5초마다 다시 보며
`--wait`(기본 `disk.lease.wait_minutes`)까지 기다립니다. 예약은 명령이 실행되는 동안 유지되고, 명령이
끝나거나 lease 프로세스가 죽으면 풀립니다. 디스크 게이트는 `auto disk lease -- <command>`로 시작한
명령을 막지 않습니다.

CPU와 메모리를 많이 쓰는 작업은 `auto disk lease --cpu N --mem 8G -- <command>`로 돌립니다. 기계에 여유가
있으면 여러 작업이 병렬로 돌고, 여유가 없으면(load가 논리 CPU의 85%를 넘거나 `--mem` 예약을 빼고 남는 메모리가
`disk.lease.min_free_mem_gb`보다 적으면) 우선순위대로 기다립니다. 무엇이 막고 있는지는 `auto disk status`의
`waiting:` 줄에 나옵니다. 무거운 작업을 lock 하나로 줄 세우는 셸 wrapper는 이 명령으로 바꿉니다.

세션들이 남긴 대기 루프(`until ... sleep`)가 쌓였으면 `auto proc reap`으로 목록을 보고, 내가 시작한 것만
`--session-pid <pid> --apply`로 정리합니다.

## 디스크 게이트

Claude Code에서는 PreToolUse hook `auto disk gate`가 Bash 호출마다 statfs 한 번으로 여유 공간을 확인합니다.
바닥선 아래이면 go/cargo/npm/pnpm/yarn/bun/pip/uv/poetry/Gradle/Maven의 빌드·테스트·설치, docker/podman
build와 compose up, xcodebuild, `git worktree add`를 막고 이 스킬의 순서를 사유로 돌려줍니다. `auto`
명령은 항상 통과합니다. `make`는 대부분의 대상이 가볍기 때문에 기본으로 막지 않습니다. 무거운 대상은
설정으로 더합니다.

```yaml
hooks:
  disk_gate:
    extra_commands: ["make release", "bazel build"]
```

끄려면 `hooks.disk_gate: false` 또는 `AUTOPUS_DISK_GATE=off`를 씁니다. OMP에는 PreToolUse hook이 없어서
게이트가 적용되지 않습니다. Codex, Gemini, OpenCode, Antigravity에도 아직 등록하지 않습니다. 그
플랫폼에서는 무거운 작업 전에 `auto disk check`를 직접 실행합니다.

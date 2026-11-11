---
name: ci-optimize
description: GitHub Actions CI가 느리거나 비쌀 때 jobs API로 대기·실행 시간과 runner 점유를 먼저 재고, 측정된 병목부터 고치는 CI 비용·속도 최적화 스킬 (self-hosted 포함)
triggers:
  - ci optimize
  - ci 최적화
  - ci 비용
  - ci가 느려
  - github actions cost
  - self-hosted runner
  - runner 대기
category: devops
level1_metadata: "jobs API로 queue·run 시간 분리, runner별 점유율, 낭비 패턴 점검, 측정된 병목부터 수정, 고친 뒤 같은 방법으로 다시 측정"
---

# CI Optimize Skill

모델은 캐시, paths 필터, concurrency 같은 일반 기법을 이미 압니다. 이 스킬이 주는 것은 **순서**입니다.
먼저 재고, 가장 큰 병목 하나를 고치고, 같은 방법으로 다시 잽니다. 재지 않고 고친 최적화는 대개 엉뚱한
곳을 빠르게 만들고, self-hosted에서는 오히려 느려지기도 합니다.

## 1. 측정 (항상 먼저)

`/repos/{o}/{r}/actions/runs/{id}/timing`과 workflow usage endpoint는 폐지됐습니다. jobs API의 타임스탬프로
직접 계산합니다. 먼저 `auto ci report`를 돌립니다. 아래 표의 지표, 중복·대체된 run, workflow lint를 한 번에
계산하고 finding을 심각도와 분/일 순서로 정렬합니다. 사용자의 `gh` 로그인으로 읽기만 합니다.

```bash
auto ci report                                   # origin 저장소, 최근 14일, run 200개까지 고르게 표본
auto ci report --repo owner/repo --since 30d --workflow ci.yaml --json   # 원 표 전체
```

출력의 `sample:` 줄에 표본 수와 보정 비율이, `note:` 줄에 요청 예산이나 tree 조회 상한으로 잘린 부분이
나옵니다. 보고에 그대로 옮깁니다. `auto ci report`가 없는 오래된 바이너리에서는 아래처럼 직접 잽니다.

```bash
REPO=owner/repo   # 대상 저장소
# 최근 14일 run 목록 → job별 대기(queue)와 실행(run) 시간, runner 이름
since=$(date -u -v-14d +%F 2>/dev/null || date -u -d '14 days ago' +%F)
gh api "repos/$REPO/actions/runs?per_page=100&created=>=$since" --paginate --jq '.workflow_runs[].id' |
  while read -r id; do
    gh api "repos/$REPO/actions/runs/$id/jobs?filter=all&per_page=100" --jq '.jobs[] |
      select(.started_at and .completed_at) |
      [.workflow_name, .name, .conclusion, .runner_name // "-",
       ((.started_at|fromdate) - (.created_at|fromdate)),
       ((.completed_at|fromdate) - (.started_at|fromdate))] | @tsv'
  done > jobs.tsv
```

`jobs.tsv`의 열은 workflow, job, 결과, runner, 대기 초, 실행 초입니다. 여기서 다음을 구합니다. 표본이 많으면 workflow별로 일부만 뽑고 보정 비율을 적습니다.

| 지표 | 무엇을 알려 주나 |
|---|---|
| job별 실행 시간 중앙값·p90 | 느린 job 자체 |
| job별 대기 시간 p90 | runner가 모자란지 |
| run 끝-끝 시간 중앙값·p90 (PR / push 따로) | 개발자가 실제로 기다리는 시간 |
| runner별 busy 분/일 ÷ 1440 | 어느 runner에 일이 몰렸는지 |
| conclusion 비율, 재실행(attempt>1) 분 | 결과 없이 버려진 시간 |
| 같은 SHA·트리의 push + PR 중복 | 두 번 돈 일 |

비용은 GitHub-hosted 분(`orgs/{org}/settings/billing/usage`, 구 `/settings/billing/actions`는 410)과
self-hosted의 **기계 점유**로 나눕니다. self-hosted 분은 청구되지 않지만 같은 기계의 다른 job 대기, 부하로
인한 timeout과 flaky 재실행이 실제 비용입니다. public repo의 hosted 표준 runner는 무료입니다.

## 2. 병목 분류

측정 결과를 아래 중 하나로 분류하고, 가장 많은 분을 먹는 것 하나부터 고칩니다.

- **대기가 크다 (queue p90 ≫ run):** runner 부족 또는 쏠림. runner별 점유율을 봅니다. 한 runner만 바쁘고
  나머지가 놀면 라벨 분배 문제입니다. 라벨을 넓히기 전에 그 라벨로 가는 job을 전부 확인합니다(릴리즈·서명
  job이 같이 따라가지 않는지, 고정 포트·공용 경로·장치 락처럼 runner 하나를 가정하는 곳은 없는지,
  runner별 캐시가 디스크를 얼마나 늘리는지). 무거운 lane과 가벼운 lane을 다른 라벨로 나누는 편이 안전합니다.
- **실행이 길다:** 단계별 시간(`steps[]`)으로 가장 비싼 단계를 찾습니다. 린트라면 linter별 시간
  (`golangci-lint run -v`), 테스트라면 패키지별 시간을 봅니다. 대체 가능한 비싼 도구 하나가 대부분을 차지하는
  경우가 많습니다.
- **같은 일을 두 번 한다:** main push가 PR에서 이미 통과한 트리를 다시 돌리거나, 새 push로 대체된 PR 실행이
  끝까지 돕니다.
- **결과 없이 버린다:** timeout, 부하 flaky, 취소. 실패 시간이 상한 근처에 몰려 있으면 timeout이고, 단독
  재실행에서 통과하면 부하 문제입니다. 재실행으로 덮지 말고 원인(경합, 너무 짧은 deadline, 직렬화된 공용
  자원)을 고칩니다.

## 3. 수정 기법 (분류별)

**일을 덜 하기**
- PR concurrency: `group`은 workflow별 고정 접두사 + PR 번호, `cancel-in-progress`는 `pull_request`에서만.
  push·schedule·release와 `workflow_call`로 불리는 실행은 run id로 그룹을 따로 둡니다. reusable workflow는
  호출자와 `github.workflow`·`run_id`를 공유하므로 접두사 없이 쓰면 호출자와 엉킵니다.
- push 재사용: PR 실행이 테스트한 merge 트리의 **트리 해시**를 기록하고, main push 커밋의 트리가 같고 필요한
  단계가 실제로 success였을 때만 무거운 job을 건너뜁니다. SHA나 PR 번호만으로 판정하지 않습니다. 판정이
  불확실하면 전부 실행합니다. 건너뛴 run을 근거로 쓰는 릴리즈 검증이 있으면 함께 고칩니다.
- 경로 필터: job 단위로 `if`를 씁니다. workflow 수준 `paths:`로 건너뛴 체크는 Pending에 남아 필수 체크를
  막습니다. 필수 체크는 `if: always()` 집계 job 하나에 걸고 `needs.*.result`의 failure·cancelled를 실패로 봅니다.
- 바뀐 패키지만 테스트: 역의존 패키지까지 계산하고, go.mod·공용 testdata·빌드 태그 변경과 main/merge_group에서는
  전체를 돕니다.

**self-hosted에서 빠르게**
- 캐시는 host 로컬 영구 경로(GOCACHE, golangci 캐시, cargo/sccache, pnpm store)를 씁니다. `actions/cache`와
  setup 액션의 `cache:`는 self-hosted에서도 GitHub 서버와 업로드·다운로드를 합니다.
- 한 기계의 여러 runner는 CPU를 나눠 씁니다. macOS에는 cgroup이 없어 job마다 `GOMAXPROCS`·`go test -p`가
  전체 코어 수로 잡힙니다. runner 수 × job당 병렬도가 코어 수를 크게 넘지 않게 잡고, 바꾼 뒤 p90과 flaky율을
  다시 잽니다. 한 기계 안에서 shard를 늘리면 setup 비용만 shard 수만큼 늘어납니다.
- GitHub Actions에는 job 우선순위가 없어서(라벨과 concurrency group뿐) 같은 기계의 경합은 기계 쪽의
  admission control로 나눕니다. runner는 job을 병렬로 시작하고, 무거운 단계만 우선순위대로 들여보냅니다.
  무거운 단계를 `auto disk lease --cpu N --wait 30m -- <cmd>`로 감싸면 그 기계의 CPU 슬롯 N개를 예약하고,
  명령에 `AUTOPUS_LEASE_CPUS=N`과(따로 정하지 않았으면) `GOMAXPROCS=N`을 넘깁니다. `go test -p "$AUTOPUS_LEASE_CPUS"`
  처럼 씁니다. 대기자는 `--priority interactive|ci|background` 순서로, 같은 class 안에서는 선착순으로 들어갑니다.
  Actions 안에서 기본 class는 이벤트를 따릅니다(PR·merge_group은 interactive, push·dispatch는 ci, schedule은
  background). 기다린 시간이 `disk.lease.aging_minutes`(기본 10)를 지날 때마다 class가 한 단계 오르므로
  background도 굶지 않습니다. 이미 도는 holder는 끊지 않습니다. runner 계정이 여러 개면 모두 같은
  `AUTOPUS_LEASE_DIR`(runner의 `.env`)을 가리키게 해야 서로를 셉니다. 대기열의 class, 대기 시간, 올라간
  class는 `auto disk status`로 봅니다.
- 디스크: runner 작업 디렉터리와 캐시가 같은 볼륨이면 여유 공간 부족이 컴파일 오류나 timeout으로 위장합니다.
  `disk-triage` 스킬로 소비처를 확인합니다. 캐시를 통째로 지우는 정리는 콜드 빌드를 만듭니다.

**위생**
- 모든 job에 `timeout-minutes`(측정 p90 × 약 2). 없으면 멈춘 job이 기본 상한까지 runner를 점유합니다.
  `uses:` job에는 넣을 수 없습니다.
- artifact `retention-days`를 명시합니다. 다른 run이 artifact id를 하드코딩해 참조하면 보존 만료가 곧 빌드 실패입니다.
- 재시도는 인프라 오류(다운로드 404, DNS, runner 연결 끊김)에만 하고, 실패 원인을 로그로 확인한 뒤 실패한 job만 다시 돌립니다.

## 4. 보안 경계 (최적화가 넘으면 안 되는 선)

- public repo의 fork PR은 self-hosted runner에 닿으면 안 됩니다. runner group의 public repo 허용을 끄고, public
  repo의 PR job은 hosted runner로 둡니다.
- `pull_request_target`에서 PR 코드를 checkout해 실행하지 않습니다. PR 제목·브랜치 이름 같은 이벤트 값을
  `run:`에 expression으로 넣지 않고 `env:`로 넘깁니다.
- host 로컬 영구 캐시는 GitHub의 `cache-mode` 보호 밖에 있습니다. 신뢰 수준이 다른 job이 같은 캐시 경로를 쓰지 않게 합니다.

## 5. 보고

```
🐙 ci-optimize ─────────────────────
  기간: {from}~{to}  표본: run {N} / job {M} (보정 {비율})
  병목: {분류} — {근거 수치}
  수정: {변경} → 예상 {분/일 또는 p90 변화}
```

- 수정마다 근거 수치와 위험을 함께 적습니다. 추정은 추정이라고 씁니다.
- 고친 뒤에는 같은 측정을 다시 돌려 전후를 비교합니다. 다시 재지 않은 개선은 "예상"으로만 보고합니다.
- GitHub 설정(runner 라벨, 변수, ruleset, 필수 체크)을 바꾸는 일은 사용자가 실행할 명령으로 건넵니다.

## 금지 사항

- 측정 없이 캐시·shard·병렬도를 바꾸지 않습니다.
- 실패를 재실행이나 `continue-on-error`로 덮어 통과율을 올리지 않습니다.
- 보안 경계를 넘는 최적화(fork PR을 self-hosted로, 캐시 공유로 신뢰 경계 섞기)를 하지 않습니다.

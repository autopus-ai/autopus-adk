---
name: resolving-merge-conflicts
description: 진행 중인 git merge/rebase 충돌을 hunk마다 원래 의도의 1차 출처로 추적해 해결하고 --abort 없이 끝내는 스킬
triggers:
  - merge conflict
  - rebase conflict
  - 충돌 해결
  - 머지 충돌
  - 리베이스 충돌
  - conflict resolution
category: development
level1_metadata: "양쪽 의도의 1차 출처 추적, 두 의도 보존, 새 동작 발명 금지, abort 금지"
---

# Resolving Merge Conflicts

진행 중인 merge/rebase 충돌을 끝까지 해결한다. 절대 `--abort`하지 않는다.

1. **현재 상태를 본다.** `git status`, `git log --oneline --graph` 양쪽 브랜치, 충돌 파일
   목록(`git diff --name-only --diff-filter=U`). rebase면 어느 커밋을 적용 중인지 확인한다.
2. **각 충돌의 1차 출처를 찾는다.** 양쪽 변경이 *왜* 있었는지 깊이 이해한다: 커밋 메시지와
   Lore trailer(`auto why <path>`), PR, 원래 이슈/SPEC(`.autopus/specs/`). 의도를 모르는 채
   hunk를 고르지 않는다.
3. **hunk마다 해결한다.** 가능하면 두 의도를 모두 보존한다. 양립할 수 없으면 merge의 명시된
   목표에 맞는 쪽을 고르고 트레이드오프를 적어 둔다. **새 동작을 발명하지 않는다.**
   `conflicts` 셀렉터(`read <file>:conflicts`)로 남은 블록을 확인하며 진행한다.
   생성·파생 산출물(golden baseline, digest, snapshot golden)은 hunk로 고르지 않는다. 합친
   트리에서 그 도구로 다시 만들고 diff가 의도한 행뿐인지 확인한다. 손으로 합친 digest는
   어느 쪽 입력과도 맞지 않는다.
4. **프로젝트의 자동 검사를 찾아 실행한다.** 보통 typecheck/build → 테스트 → 포맷 순서
   (Go: `go build ./... && go test ./... && gofmt -l .`). merge가 깨뜨린 것을 고친다.
5. **merge/rebase를 끝낸다.** 전부 stage하고 커밋한다. rebase면 모든 커밋이 적용될 때까지
   `git rebase --continue`를 반복한다. merge 커밋 메시지는 `lore-commit` 규약을 따르고,
   3단계의 트레이드오프를 `Rejected:`에 남긴다.

Adapted from mattpocock/skills `resolving-merge-conflicts` (MIT).

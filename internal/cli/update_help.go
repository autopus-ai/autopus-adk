package cli

// updateCmdLong is the help of `auto update`; the composition step it
// describes is SPEC-MODELUX-001 and SPEC-MODELUX-002.
const updateCmdLong = "설치된 하네스 파일을 업데이트합니다.\n\n" +
	"보존 범위는 파일 종류에 따라 다릅니다.\n" +
	"  - 마커 밖 사용자 내용(AGENTS.md, CLAUDE.md 등): 보존됩니다. AUTOPUS 마커 구간만 교체합니다.\n" +
	"  - 마커 안 내용: 보존되지 않습니다. 매 업데이트마다 재생성됩니다.\n" +
	"  - 그 외 생성 파일(대부분 overwrite 정책): 사용자 수정이 보존되지 않고 덮어쓰입니다.\n" +
	"    덮어쓰기 전 원본은 .autopus/backup/<timestamp>/transaction/<platform>/<경로>에 복사됩니다.\n" +
	"  - 사용자가 삭제한 생성 파일: 다시 만들지 않고 건너뜁니다.\n\n" +
	"--preview(--plan/--dry-run)로 변경 예정 파일과 각 파일의 처리 방식을 먼저 확인할 수 있습니다.\n\n" +
	"에이전트 모델 구성: 대화형 터미널에서는 프로젝트 설정 질문 뒤, 플랫폼 파일 생성 전에 에이전트×플랫폼\n" +
	"표와 프리셋 상태를 보여 주고 매번 \"구성을 바꿀까요?\"를 묻습니다. Enter, 입력 끝, 잘못된 답 3번은\n" +
	"autopus.yaml을 그대로 둡니다. u는 이전 버전 기본 프리셋 갱신, g는 역할 그룹 4개, a는 에이전트별 등급이고\n" +
	"(auto quality --help), 바뀐 칸에 *가 붙은 미리보기를 y로 확인해야만 쓰며 같은 실행의 생성 파일이 그 미리보기와\n" +
	"같습니다. --yes(-y), --auto, --check, CI(비어 있지 않고 0·false가 아닌 값), TTY가 아닌 입력, --workspace는\n" +
	"묻지도 바꾸지도 않고 이전 버전 기본값 안내만 한 줄 씁니다. --plan/--preview/--dry-run에는 이 단계가 없습니다.\n\n" +
	"대화형 터미널(입력이 프로세스 stdin이고 stdin·stdout이 모두 터미널, Windows는 Windows Terminal 등)에서는 이\n" +
	"단계가 init wizard와 같은 화살표와 Enter 폼입니다. 메뉴는 유지 · 최신 기본값으로 갱신 · 구성 편집이고, 구성\n" +
	"편집은 도구 탭(모든 도구 같게, 설치된 Claude Code·Codex, autopus.yaml에 저장된 platforms에 omp가 있으면 OMP)을\n" +
	"엽니다. 선택지는 그 도구의 실제 모델과 effort를 보여 줍니다. Claude 라벨은 모델 slug(opus-5-5)이고 표 칸은\n" +
	"생성될 frontmatter 값(opus)입니다. Claude Code·Codex 탭의 결과는 quality.presets.<provider>-custom과\n" +
	"quality.providers.<provider>에 저장되고, OMP 탭은 native agent별 모델을 기존 OMP writer로 씁니다. 표에는 이번\n" +
	"update가 감지한 플랫폼의 열도 나옵니다. 폼에서 Ctrl+C는 update 전체를 끝냅니다(생성 없음, 0이 아닌 종료).\n" +
	"파이프나 스크립트 입력은 위의 줄 질문 그대로입니다.\n\n" +
	"Claude Code·Codex 탭은 등급 Select마다 그 도구의 effort Select를 이어 묻습니다. 기본은 그대로이고, Claude는\n" +
	"low·medium·high·xhigh·max(haiku 에이전트 제외), Codex는 catalog가 보고하는 단계만 보이며, 자동과 같은 단계는\n" +
	"저장하지 않습니다. 고른 effort는 quality.presets.<provider>-custom.efforts에 쓰고, 이 key가 있는 파일은\n" +
	"v0.50.126보다 오래된 auto가 읽지 못합니다. efforts가 없는 파일의 생성 결과는 그대로입니다.\n\n" +
	"OMP는 v0.50.126부터 기본으로 Autopus 에이전트 16개를 .omp/agents에 만들고 task를 뺀 OMP 기본 에이전트\n" +
	"(scout·security-reviewer·sonic·designer·librarian)를 task.disabledAgents로 끕니다. 기존 프로젝트는 미리보기에\n" +
	"+ .omp/agents/<agent>.md 줄과 안내가 나오고 autopus.yaml은 쓰지 않습니다. manifest에 없는 같은 이름 파일과\n" +
	"사용자가 정한 task.disabledAgents는 그대로 둡니다. role_model_policy.agent_source: native로 v0.50.126 동안 이전\n" +
	"방식을 고를 수 있습니다(v0.50.127 제거 예정). 바이너리를 내리는 대신 이 key로 되돌리세요.\n\n" +
	"이 기본 mode의 OMP 탭은 프로필·계열을 묻지 않습니다. 지금 pin이나 프로필 route에서 시작해 역할 그룹 4개나\n" +
	"에이전트별(자기 pin이 있으면 task 포함)로 그대로 · OMP 세션 모델 그대로 · 모든 provider의 증명된 설치 model을\n" +
	"고르고, thinking은 고른 model이 보고한 단계만 보입니다. metadata 없는 catalog는 built-in 선언에 있는 model만\n" +
	"보입니다. 결과는 프로필 없는 pin(role_model_policy: {version: v1, agents: …})이고 task는 자기 pin이 없으면\n" +
	"planner를 따릅니다. 프로필 설정 위에서 적용하면 변환 줄과 확인 뒤 pin으로 옮기고, 바꾸지 않은 에이전트와 task는\n" +
	"route가 그대로이며 ultra 프로필의 quality.presets.ultra 연동은 끝납니다. native mode, 줄 OMP 편집기,\n" +
	"auto platform omp profile apply는 그대로 프로필을 씁니다."

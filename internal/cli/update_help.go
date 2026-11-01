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
	"quality.providers.<provider>에 저장되고, OMP 탭은 에이전트별 모델을 기존 OMP writer로 씁니다. 표에는 이번\n" +
	"update가 감지한 플랫폼의 열도 나옵니다. 폼에서 Ctrl+C는 update 전체를 끝냅니다(생성 없음, 0이 아닌 종료).\n" +
	"파이프나 스크립트 입력은 위의 줄 질문 그대로입니다.\n\n" +
	"Claude Code·Codex 탭은 등급 Select마다 그 도구의 effort Select를 이어 묻습니다. 기본은 그대로이고, Claude는\n" +
	"low·medium·high·xhigh·max(haiku 에이전트 제외), Codex는 catalog가 보고하는 단계만 보이며, 자동과 같은 단계는\n" +
	"저장하지 않습니다. 고른 effort는 quality.presets.<provider>-custom.efforts에 쓰고, 이 key가 있는 파일은\n" +
	"v0.50.127보다 오래된 auto가 읽지 못합니다. efforts가 없는 파일의 생성 결과는 그대로입니다.\n\n" +
	"OMP는 v0.50.127부터 기본으로 Autopus 에이전트 17개를 .omp/agents에 만들고 task를 뺀 OMP 기본 에이전트\n" +
	"(scout·security-reviewer·sonic·designer·librarian·seance)를 task.disabledAgents로 끕니다. 기존 프로젝트는 미리보기에\n" +
	"+ .omp/agents/<agent>.md 줄과 안내가 나오고 autopus.yaml은 쓰지 않습니다. manifest에 없는 같은 이름 파일과\n" +
	"사용자가 정한 task.disabledAgents는 그대로 둡니다. role_model_policy.agent_source: native는 v0.50.131에서\n" +
	"제거되어 로드에 실패합니다. 그 key를 지우고 auto update를 실행하면 Autopus 에이전트로 돌아갑니다.\n\n" +
	"OMP 탭은 프로필·계열을 묻지 않습니다. 지금 pin이나 프로필 route에서 시작해 역할 그룹 4개나\n" +
	"에이전트별(자기 pin이 있으면 task 포함)로 그대로 · OMP 세션 모델 그대로 · 모든 provider의 증명된 설치 model을\n" +
	"고르고, thinking은 고른 model이 보고한 단계만 보입니다. metadata 없는 catalog는 built-in 선언에 있는 model만\n" +
	"보입니다. 결과는 프로필 없는 pin(role_model_policy: {version: v1, agents: …})이고 task는 자기 pin이 없으면\n" +
	"planner를 따릅니다. 프로필 설정 위에서 적용하면 변환 줄과 확인 뒤 pin으로 옮기고, 바꾸지 않은 에이전트와 task는\n" +
	"route가 그대로이며 ultra 프로필의 quality.presets.ultra 연동은 끝납니다. 줄 OMP 편집기와\n" +
	"auto platform omp profile apply는 그대로 프로필을 씁니다.\n\n" +
	"도구 탭의 마지막 선택지는 spec.review_gate.enabled가 켜져 있고 orchestra.providers가 둘 이상이며 CI가 아닐 때\n" +
	"나오는 멀티프로바이더 리뷰입니다(SPEC-REVMODEL-001). 표 아래 요약 줄이 리뷰 provider마다 원문의 model·effort와\n" +
	"judge를 보입니다. 탭은 " +
	"auto spec review에 참여하는 provider(spec.review_gate.providers), provider마다 model과 effort,\n" +
	"judge(spec.review_gate.judge)를 정합니다. backend: omp provider는 그 family의 증명된 설치 model과\n" +
	"thinking(orchestra.providers.<p>.model)을, CLI backend의 claude는 Claude 탭 model과 effort(--model·--effort),\n" +
	"codex는 Codex catalog model과 effort(-m·model_reasoning_effort, model_policy: pinned), agy는 agy models가 보인\n" +
	"id(--model)를 고르고 args에서는 그 값만 바뀝니다. omp가 그 family의 model을 증명하면 모든 CLI provider를 OMP\n" +
	"backend로 전환할 수 있습니다(backend: omp와 model). 미리보기는 바뀐 CLI 값만 autopus.yaml 글자로 보이고 다른 args는 보이지\n" +
	"않으며, ${NAME}에서 오는 값은 바꾸지 않습니다. 그 값만 바꿔 써서 주석과 서식이 그대로입니다. 끈 provider도\n" +
	"orchestra 명령(brainstorm·plan·review·secure)과 auto spec review --multi에서는 계속 돕니다. 확인 호출을 고르면\n" +
	"provider n개에 요청 1회씩(재시도 포함 최대 3n회 model 호출) 보내고 결과 줄만 쓰며 아무것도 저장하지 않습니다. 탭을\n" +
	"적용하거나 취소하면 그 결과 줄이 다시 그린 표의 요약 줄 아래에 한 번 더 나옵니다. 탭이 없는 실행(--yes, TTY가 아닌\n" +
	"입력, CI 등)은 이 탭이 나올 설정이면 리뷰 model을 바꾸는 곳(터미널의 auto quality, 또는 auto quality review model\n" +
	"<provider> <model>)을 한 줄로 알립니다." +
	"\n\n" + tuiuxUpdateHelp + "\n\n" + reaUpdateHelp

// tuiuxUpdateHelp is the composition forms' navigation and frame
// (SPEC-TUIUX-001).
const tuiuxUpdateHelp = "구성 단계의 폼마다 맨 위에 `update · 모델 구성 › 구성 편집 › Codex › 역할 그룹` 같은 단계 header, 편집 중에는\n" +
	"지금까지 고른 값, 맨 아래 `방향키 이동 · Enter 선택 · Esc 뒤로 · Ctrl+C 취소` footer가 터미널 높이 안에 보입니다\n" +
	"(SPEC-TUIUX-001). Esc는 한 폼 뒤로 갑니다: 역할 그룹·에이전트별 편집은 바로 앞에 물은 폼, 방식은 도구 탭, 도구 탭은\n" +
	"메뉴이고, 다시 보인 폼은 앞 답을 고른 채입니다. 적용할까요?와 override 질문의 Esc는 계속 편집, OMP·멀티프로바이더\n" +
	"리뷰 탭은 탭 단위로 도구 탭에 돌아갑니다. 메뉴는 첫 폼이라 Esc가 `첫 단계입니다. 취소하려면 Ctrl+C를 누르세요.`를\n" +
	"보이고 머뭅니다. 이미 적용한 쓰기는 뒤로가 되돌리지 않습니다. TERM=dumb의 줄 입력에서는 `b`·`back`·`뒤로` 한 줄이\n" +
	"Esc이고 줄 질문에는 뒤로가 없습니다. auto init은 아무 파일도 쓰기 전에 `적용할 설정`(답한 값, 바뀌는 설정, REA)을\n" +
	"보이고 적용·뒤로·취소를 묻습니다(auto init --help)."

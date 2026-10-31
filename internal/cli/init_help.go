package cli

// initCmdLong is the help of `auto init`; the composition step it describes
// is SPEC-MODELUX-001 and SPEC-MODELUX-002.
const initCmdLong = "코딩 CLI에 Autopus 하네스를 설치합니다. autopus.yaml을 생성하고 플랫폼 파일을 설치합니다.\n\n" +
	"대화형 wizard의 품질 단계는 Ultra, Balanced, 역할별 직접 구성 중 하나를 고르고, 어느 쪽이든\n" +
	"플랫폼 파일을 쓰기 전에 에이전트×플랫폼 구성 표를 보여 줍니다. 역할별 직접 구성은 balanced\n" +
	"기본값에서 시작해 역할 그룹 4개(g: 설계·판단, 구현, 검증·리뷰, 탐색·보조)나 에이전트별(a)\n" +
	"등급을 고르고, 미리보기를 y로 확인하면 그 구성을 autopus.yaml에 쓰고 같은 표대로 생성합니다.\n" +
	"유지하거나 취소하면 balanced로 진행합니다. --yes, TTY가 아닌 입력, --quality <preset>은\n" +
	"이 단계를 건너뜁니다.\n\n" +
	"대화형 터미널에서는 역할별 직접 구성이 화살표와 Enter 폼입니다. 메뉴의 구성 편집은 도구 탭(모든 도구 같게,\n" +
	"Claude Code, Codex, 이번 init에서 고른 platforms에 omp가 있으면 OMP)을 열고, 선택지는 그 도구의 실제 모델과\n" +
	"effort를 보여 줍니다(Claude 라벨은 slug opus-5-5, 표 칸은 frontmatter 값 opus). Claude Code·Codex 탭은\n" +
	"quality.presets.<provider>-custom과 quality.providers.<provider>를, OMP 탭은 init 설정의 role_model_policy를\n" +
	"정하고 init이 파일 생성 때 함께 씁니다. OMP catalog를 읽지 못하면 안내 뒤 init을 계속합니다. 폼에서 Ctrl+C는\n" +
	"init cancelled.를 쓰고 아무 파일도 만들지 않습니다. 파이프나 스크립트 입력은 줄 질문 그대로입니다.\n\n" +
	"Claude Code·Codex 탭은 등급 Select마다 그 도구의 effort Select를 이어 묻습니다(기본 그대로, Claude는 haiku\n" +
	"에이전트 제외, Codex는 catalog가 보고하는 단계만). 자동과 다른 effort는 quality.presets.<provider>-custom.efforts에\n" +
	"넣고, 이 key가 있는 파일은 v0.50.127보다 오래된 auto가 읽지 못합니다. omp를 고르면 Autopus 에이전트 17개를\n" +
	".omp/agents에 만들고 task를 뺀 OMP 기본 에이전트(scout·security-reviewer·sonic·designer·librarian·seance)를\n" +
	"task.disabledAgents로 끕니다. 프로필이 없으면 세션 모델을 상속합니다. role_model_policy.agent_source: native는\n" +
	"v0.50.131에서 제거되어 로드에 실패합니다.\n\n" +
	"OMP 탭은 프로필·계열을 묻지 않고 역할 그룹 4개나 에이전트별로 그대로 · OMP 세션 모델 그대로 ·\n" +
	"모든 provider의 증명된 설치 model을 고르며, thinking은 고른 model이 보고한 단계만 보입니다. 결과는 프로필 없는\n" +
	"pin(role_model_policy: {version: v1, agents: …})이고 task는 자기 pin이 없으면 planner를 따릅니다.\n\n" +
	"wizard의 Review Gate가 예이고 orchestra.providers가 둘 이상이며 CI가 아니면 도구 탭 마지막에 멀티프로바이더\n" +
	"리뷰가 나옵니다(SPEC-REVMODEL-001). Review Gate 답과 감지한 provider는 구성 단계 전에 들어가고, 탭은 auto spec\n" +
	"review에 참여하는 provider(spec.review_gate.providers), provider마다 model과 effort, judge(spec.review_gate.judge)를\n" +
	"init 설정에 넣어 init이 함께 씁니다. backend: omp provider는 증명된 model과 thinking(orchestra.providers.<p>.model),\n" +
	"CLI backend의 claude는 --model·--effort, codex는 Codex catalog model의 -m·model_reasoning_effort와 model_policy:\n" +
	"pinned, agy는 agy models가 보인 id의 --model만 바꾸고, omp가 그 family의 model을 증명하면 OMP backend로 전환할 수\n" +
	"있습니다. 끈 provider도 orchestra 명령(brainstorm·plan·review·secure)과 auto spec review --multi에서는 계속\n" +
	"돕니다. 확인 호출을 고르면 provider n개에 요청 1회씩(재시도 포함 최대 3n회 model 호출) 보내고 결과 줄만 씁니다." +
	"\n\n" + tuiuxInitHelp + "\n\n" + reaInitHelp

// tuiuxInitHelp is init's navigation, frame, and review (SPEC-TUIUX-001).
const tuiuxInitHelp = "wizard와 구성 단계의 폼마다 맨 위에 `init · 4/6 리뷰 게이트`나 `init · 모델 구성 › 구성 편집 › Codex`\n" +
	"같은 단계 header, 그 아래 지금까지 고른 값, 맨 아래 `방향키 이동 · Enter 선택 · Esc 뒤로 · Ctrl+C 취소` footer가\n" +
	"터미널 높이 안에 보입니다(SPEC-TUIUX-001). Esc는 한 폼 뒤로 가고 다시 보인 폼은 앞 답을 고른 채입니다. 첫 단계의\n" +
	"Esc는 `첫 단계입니다. 취소하려면 Ctrl+C를 누르세요.`를 보이고 머뭅니다. 구성 메뉴의 Esc는 wizard 마지막 단계로\n" +
	"돌아가고 그 구성을 버립니다. TERM=dumb의 줄 입력에서는 `b`·`back`·`뒤로` 한 줄이 Esc입니다.\n\n" +
	"폼으로 묻는 init은 아무 파일도 쓰기 전에 `적용할 설정`을 보입니다: 답한 값, 그 값이 바꾸는 설정 key 전부\n" +
	"(새 설정과의 YAML 차이), REA 결정과 동작(등록·그대로·제거). 그 뒤 `이대로 진행할까요?`에 적용(기본값)·뒤로·취소를\n" +
	"고릅니다. 뒤로는 구성 단계가 폼을 물었으면 그 메뉴를 이어 하고 아니면 wizard 마지막 단계로 가며, 취소와 Ctrl+C는\n" +
	"init cancelled.를 쓰고 아무것도 만들거나 바꾸지 않습니다. 확인 화면에 없는 것: 그 뒤의 orchestra 정규화,\n" +
	"statusLine·parent rule 질문과 그 결과, 생성 뒤 정하는 features.rea, 생성 파일. --yes, TTY가 아닌 입력, 줄 질문(파이프\n" +
	"stdout의 구성 단계 포함)에는 뒤로와 확인 화면이 없습니다."

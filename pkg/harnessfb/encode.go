package harnessfb

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

// An approved lesson starts as a text rule. The work agent, which can read
// the codebase, turns it into a check at the strongest layer it can; the
// harness accepts the check only after running it itself. The record of
// that proof lives in the git directory, next to the approval, so a
// working-tree file cannot claim a proof nobody ran.
const encodingsGitPath = "autopus/harness-encodings"

// Layer is where a lesson is enforced, strongest first.
type Layer string

const (
	LayerAPI  Layer = "api"  // an API change makes the mistake impossible to write
	LayerLint Layer = "lint" // a lint rule, type check or compiler error catches it
	LayerTest Layer = "test" // a test catches it
	LayerHook Layer = "hook" // an agent hook or CI step catches it
	LayerText Layer = "text" // no check fits: the text rule stays, with a reason
)

// Layers in the order the work agent tries them.
var Layers = []Layer{LayerAPI, LayerLint, LayerTest, LayerHook, LayerText}

// ParseLayer accepts one of Layers.
func ParseLayer(s string) (Layer, error) {
	for _, l := range Layers {
		if string(l) == s {
			return l, nil
		}
	}
	names := make([]string, len(Layers))
	for i, l := range Layers {
		names[i] = string(l)
	}
	return "", fmt.Errorf("층은 %s 중 하나입니다: %q", strings.Join(names, ", "), s)
}

// Encoding is the proven result of turning one lesson into a check.
type Encoding struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
	Layer  Layer  `json:"layer"`
	// Check and Repro are set for every layer but text: the command the
	// harness ran and the patch that reproduces the mistake.
	Check       string   `json:"check,omitempty"`
	Repro       string   `json:"repro,omitempty"`
	ReproSHA256 string   `json:"repro_sha256,omitempty"`
	ReproPaths  []string `json:"repro_paths,omitempty"`
	// FailOutput is the end of what the check printed on the patched tree,
	// so a person can see that it failed for the lesson's reason.
	FailOutput string `json:"fail_output,omitempty"`
	Reason     string `json:"reason,omitempty"` // text layer only
	// Head and Branch are where the proof ran. The check may exist only on
	// that branch, uncommitted; the proof still applies to every worktree.
	Head   string    `json:"head,omitempty"`
	Branch string    `json:"branch,omitempty"`
	Proved time.Time `json:"proved"`
}

// Structural reports a lesson that a check now enforces, so its text rule
// can leave the rule file.
func (e Encoding) Structural() bool { return e.Layer != "" && e.Layer != LayerText }

// LoadEncoding reads the proof record of an approved candidate. A record
// written for a different rule text does not count.
func LoadEncoding(root string, c Candidate) (Encoding, bool) {
	var e Encoding
	git, err := GitDir(root)
	if err != nil {
		return e, false
	}
	data, err := readFile(git, path.Join(encodingsGitPath, c.ID+".json"))
	if err != nil || json.Unmarshal(data, &e) != nil || e.Digest != digest(c) {
		return Encoding{}, false
	}
	return e, true
}

func saveEncoding(root string, e Encoding) error {
	git, err := GitDir(root)
	if err != nil {
		return err
	}
	return writeJSON(git, path.Join(encodingsGitPath, e.ID+".json"), e)
}

func removeEncoding(root, id string) {
	if git, err := GitDir(root); err == nil {
		_ = removeFile(git, path.Join(encodingsGitPath, id+".json"))
	}
}

// ApprovedCandidate loads id and requires a person's recorded approval.
func ApprovedCandidate(root, id string) (Candidate, error) {
	c, err := LoadCandidate(root, id)
	if err != nil {
		return c, err
	}
	if len(Approved(root, []Candidate{c})) == 0 {
		return c, fmt.Errorf("%s는 승인된 교훈이 아닙니다 (%s): 승인한 교훈만 검사로 바꿉니다", id, c.State.Label())
	}
	return c, nil
}

// ErrReasonRequired means a text-layer result came without a reason.
var ErrReasonRequired = errors.New("검사로 바꿀 수 없다면 이유가 필요합니다: --reason")

// KeepAsText records that no check fits the lesson; its text rule stays.
func KeepAsText(root, id, reason string, now time.Time) (Encoding, []string, error) {
	c, err := ApprovedCandidate(root, id)
	if err != nil {
		return Encoding{}, nil, err
	}
	reason = strings.Join(strings.Fields(reason), " ")
	if reason == "" {
		return Encoding{}, nil, ErrReasonRequired
	}
	e := Encoding{ID: c.ID, Digest: digest(c), Layer: LayerText, Reason: reason, Proved: now}
	if err := saveEncoding(root, e); err != nil {
		return e, nil, err
	}
	written, err := RenderRules(root)
	return e, written, err
}

// Brief is what the work agent reads after an approval: how to turn the
// lesson into a check and how the harness will judge it.
func Brief(c Candidate) string {
	lesson := strings.Join(strings.Fields(c.Lesson), " ")
	return fmt.Sprintf(`%[1]s 교훈을 이 저장소의 검사로 바꾸세요. 그전까지는 %[2]s에 임시 글 규칙으로 남습니다.
교훈: %[3]s

1. 코드베이스를 읽고 이 실수를 막을 가장 강한 층을 고릅니다.
   api(API를 바꿔 실수 자체를 쓸 수 없게) > lint(lint·타입 검사·컴파일 오류) > test > hook(에이전트 hook·CI 단계) > text
2. 검사를 만들어 프로젝트의 기존 lint·test·CI 경로에 연결합니다. 진단 메시지에는 무엇을 왜 막는지 적습니다.
3. 실수를 재현하는 patch를 지금 트리 기준으로 만듭니다. 사용자의 커밋 안 된 작업은 건드리지 않습니다.
     base=$(git stash create); base=${base:-HEAD}   # 지금 트리를 가리키는 커밋 (stash 목록은 바뀌지 않음)
     (실수를 코드에 넣는다)
     p=$(mktemp); git diff "$base" -- <실수를 넣은 파일> > "$p"
     git apply -R "$p"                               # 넣은 실수만 되돌린다
   patch는 실수만 담아야 합니다. 검사가 실행하는 스크립트를 고치는 patch는 거부되고, 설정 파일을 고친 patch는 사람이 status에서 확인합니다.
4. auto harness encode %[1]s --layer <층> --check '<검사 명령>' --repro "$p"
   하네스가 검사를 직접 세 번 실행합니다: 지금 트리(통과), 지금 트리의 복사본(통과), 복사본에 patch를 커밋한 트리(실패).
   복사본에는 git이 무시하는 파일(node_modules, venv, 빌드 산출물)이 없으니, 필요하면 검사 명령이 그것부터 준비하게 합니다.
   지금 트리는 바꾸지 않습니다. 셋 다 맞아야 증명되고, 그때 글 규칙이 빠집니다.

어느 층으로도 막을 수 없으면: auto harness encode %[1]s --layer text --reason '<이유>'
`, c.ID, RuleFile, lesson)
}

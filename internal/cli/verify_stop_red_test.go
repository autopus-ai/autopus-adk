package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func tddProject(t *testing.T, dir string, mode string, enforce bool) {
	t.Helper()
	cfg := config.DefaultFullConfig("demo")
	cfg.Methodology.Mode, cfg.Methodology.Enforce = mode, enforce
	require.NoError(t, config.Save(dir, cfg))
}

func runStopReply(t *testing.T, dir, session, reply string) stopHookOutput {
	t.Helper()
	isolateStop(t)
	cmd := newVerifyStopCmd()
	payload, _ := json.Marshal(stopHookInput{SessionID: session, Cwd: dir, LastAssistantMessage: reply})
	cmd.SetIn(bytes.NewReader(payload))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--budget", (3 * time.Minute).String()})
	require.NoError(t, cmd.Execute())
	var got stopHookOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	return got
}

// A passing source change with no test change is sent back under
// methodology: tdd, and a TDD-exempt line lets it through.
func TestVerifyStop_TDDBlocksASourceChangeWithoutATest(t *testing.T) {
	session := "tdd-no-test"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	tddProject(t, dir, "tdd", true)

	got := runStopReply(t, dir, session, "Add의 인자 순서를 바꿨습니다.")
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "add.go")
	assert.Contains(t, got.Reason, "TDD-exempt")

	assert.Equal(t, stopHookOutput{}, runStopReply(t, dir, session, "정리했습니다.\n\nTDD-exempt: 동작이 같은 순서 변경"))
}

// A test that already passed before the change does not count as test-first.
func TestVerifyStop_TDDBlocksATestThatPassesBeforeTheChange(t *testing.T) {
	session := "tdd-weak"
	t.Cleanup(func() { resetStopState(session) })
	dir := newStopRepo(t, "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n")
	tddProject(t, dir, "tdd", true)
	weak := "package stopgate\n\nimport \"testing\"\n\nfunc TestAddZero(t *testing.T) {\n\tif Add(0, 0) != 0 {\n\t\tt.Fatal(\"Add(0, 0)\")\n\t}\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zero_test.go"), []byte(weak), 0o644))

	got := runStopReply(t, dir, session, "테스트를 더했습니다.")
	assert.Equal(t, "block", got.Decision)
	assert.Contains(t, got.Reason, "also pass against the code before this change")
}

func TestVerifyStop_TDDPassesAFixWithAFailingFirstTest(t *testing.T) {
	session := "tdd-red"
	t.Cleanup(func() { resetStopState(session) })
	// The committed base is correct; the edit introduces a new function
	// with a test that cannot compile, let alone pass, before it.
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return a + b }\n\nfunc Double(a int) int { return Add(a, a) }\n")
	tddProject(t, dir, "tdd", true)
	test := "package stopgate\n\nimport \"testing\"\n\nfunc TestDouble(t *testing.T) {\n\tif Double(2) != 4 {\n\t\tt.Fatal(\"Double(2)\")\n\t}\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "double_test.go"), []byte(test), 0o644))

	assert.Equal(t, stopHookOutput{}, runStopReply(t, dir, session, "Double을 더했습니다."))
}

func TestVerifyStop_TDDOffWithoutEnforceOrBySwitch(t *testing.T) {
	change := "package stopgate\n\n// Add sums.\nfunc Add(a, b int) int { return b + a }\n"
	for name, setup := range map[string]func(t *testing.T, dir string){
		"mode none":      func(t *testing.T, dir string) { tddProject(t, dir, "none", false) },
		"tdd unenforced": func(t *testing.T, dir string) { tddProject(t, dir, "tdd", false) },
		"switch off": func(t *testing.T, dir string) {
			tddProject(t, dir, "tdd", true)
			t.Setenv("AUTOPUS_TDD_STOP", "off")
		},
	} {
		t.Run(name, func(t *testing.T) {
			session := "tdd-off-" + strings.ReplaceAll(name, " ", "-")
			t.Cleanup(func() { resetStopState(session) })
			dir := newStopRepo(t, change)
			setup(t, dir)
			assert.Equal(t, stopHookOutput{}, runStopReply(t, dir, session, "바꿨습니다."))
		})
	}
}

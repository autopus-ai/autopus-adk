package verifygate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShiftLine(t *testing.T) {
	// Two lines inserted after line 4, line 10 rewritten as three.
	hunks := []lineHunk{{old: 4, oldN: 0, newN: 2}, {old: 10, oldN: 1, newN: 3}}
	for n, want := range map[int]int{1: 1, 4: 4, 5: 7, 9: 11, 11: 15} {
		got, ok := shiftLine(n, hunks)
		assert.True(t, ok, "line %d", n)
		assert.Equal(t, want, got, "line %d", n)
	}
	_, ok := shiftLine(10, hunks)
	assert.False(t, ok, "a rewritten line has no line now")
}

func TestHeadViewShifted(t *testing.T) {
	v := headView{module: "m", hunks: map[string][]lineHunk{
		"x_test.go":     {{old: 2, oldN: 0, newN: 5}},
		"sub/y_test.go": {{old: 3, oldN: 1, newN: 1}},
		"help.go":       {{old: 1, oldN: 0, newN: 2}},
	}}
	runs := v.runs(map[string][]testRun{
		"m.TestX":     {{Failed: true, Lines: []string{"x_test.go:6: boom", "./x_test.go:1:2: undefined", "other.go:6: z", "help.go:6: helper"}}},
		"m/sub.TestY": {{Failed: true, Lines: []string{"y_test.go:3: rewritten", "$ROOT/x_test.go:9: root"}}},
	})
	assert.Equal(t, []string{"x_test.go:11: boom", "./x_test.go:1:2: undefined", "other.go:6: z", "help.go:6: helper"}, runs["m.TestX"][0].Lines)
	assert.Equal(t, []string{"y_test.go:?: rewritten", "$ROOT/x_test.go:14: root"}, runs["m/sub.TestY"][0].Lines)
}

// The user's diff settings do not change what goHunks reads.
func TestGoHunks_IgnoresDiffConfig(t *testing.T) {
	const head = "package basegate\n\nimport \"testing\"\n\n"
	const pre = "func TestPre(t *testing.T) {\n\tt.Fatal(\"env: missing tool\")\n}\n"
	const added = "func TestAdded(t *testing.T) {}\n\n"
	base := baselineBase()
	base["pre_test.go"] = head + pre + "\nfunc a() {}\n\nfunc b() {}\n"
	root := redRepo(t, base, map[string]string{"pre_test.go": head + added + pre + "\nfunc a() {}\n\nfunc b() { _ = 1 }\n"})
	gitRun(t, root, "config", "diff.mnemonicPrefix", "true")
	gitRun(t, root, "config", "diff.noprefix", "true")
	gitRun(t, root, "config", "diff.interHunkContext", "3")

	hunks := goHunks(context.Background(), root)["pre_test.go"]
	assert.Len(t, hunks, 2, "%v", hunks)
	got, ok := shiftLine(6, hunks)
	assert.True(t, ok)
	assert.Equal(t, 8, got)
}

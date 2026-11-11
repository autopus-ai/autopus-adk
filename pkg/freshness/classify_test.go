package freshness

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseVersionAndMaxStable(t *testing.T) {
	v, ok := ParseVersion("go1.22.3")
	assert.True(t, ok)
	assert.Equal(t, Version{Major: 1, Minor: 22, Patch: 3, Parts: 3}, v)
	v, _ = ParseVersion("v2.0.0+incompatible")
	assert.Equal(t, 2, v.Major)
	v, _ = ParseVersion("3.14.0rc1")
	assert.True(t, v.IsPrerelease())
	_, ok = ParseVersion("bookworm")
	assert.False(t, ok)

	best, ok := MaxStable([]string{"18", "19rc1", "17.11", "9.6", "19.0.0-beta.1"})
	assert.True(t, ok)
	assert.Equal(t, "18", best)
}

func TestClassify(t *testing.T) {
	latest := func(v string) Latest { return Latest{Version: v} }
	for _, tc := range []struct {
		name   string
		item   Item
		latest Latest
		want   Status
		lines  int
	}{
		{"npm caret a major behind", Item{Ecosystem: EcoNPM, Name: "react", Spec: "^17.0.2"}, latest("19.3.1"), StatusMajor, 2},
		{"npm caret on the latest line", Item{Ecosystem: EcoNPM, Name: "react", Spec: "^19.0.0"}, latest("19.3.1"), StatusCurrent, 0},
		{"npm exact minor behind", Item{Ecosystem: EcoNPM, Name: "react", Spec: "19.1.0"}, latest("19.3.1"), StatusMinor, 0},
		{"npm tilde minor behind", Item{Ecosystem: EcoNPM, Name: "x", Spec: "~19.1.0"}, latest("19.3.1"), StatusMinor, 0},
		{"npm range upper bound", Item{Ecosystem: EcoNPM, Name: "x", Spec: ">=16 <18"}, latest("19.3.1"), StatusMajor, 2},
		{"npm or-range takes the newest", Item{Ecosystem: EcoNPM, Name: "x", Spec: "^18 || ^19"}, latest("19.3.1"), StatusCurrent, 0},
		{"floor allows latest", Item{Ecosystem: EcoToolchain, Name: "node", Spec: ">=18"}, Latest{Version: "26.13.0", LTS: "24.22.2"}, StatusCurrent, 0},
		{"star", Item{Ecosystem: EcoNPM, Name: "x", Spec: "*"}, latest("1.0.0"), StatusCurrent, 0},
		{"node judged on LTS", Item{Ecosystem: EcoImage, Name: "node", Spec: "20"}, Latest{Version: "27.1.0", LTS: "26.13.0"}, StatusMajor, 6},
		{"node LTS line is current", Item{Ecosystem: EcoToolchain, Name: "node", Spec: "26"}, Latest{Version: "27.1.0", LTS: "26.13.0"}, StatusCurrent, 0},
		{"go directive a line behind", Item{Ecosystem: EcoToolchain, Name: "go", Spec: "1.22"}, latest("1.27.4"), StatusMajor, 5},
		{"go toolchain patch behind", Item{Ecosystem: EcoToolchain, Name: "go", Spec: "1.27.1"}, latest("1.27.4"), StatusMinor, 0},
		{"go module minor behind", Item{Ecosystem: EcoGo, Name: "m", Spec: "v1.8.0"}, latest("v1.10.2"), StatusMinor, 0},
		{"go module 0.x minor is a line", Item{Ecosystem: EcoGo, Name: "m", Spec: "v0.3.0"}, latest("v0.5.1"), StatusMajor, 2},
		{"python pin lines", Item{Ecosystem: EcoToolchain, Name: "python", Spec: "3.11"}, latest("3.15.2"), StatusMajor, 4},
		{"python caret allows the major", Item{Ecosystem: EcoToolchain, Name: "python", Spec: "^3.11"}, latest("3.15.2"), StatusCurrent, 0},
		{"pypi exact major behind", Item{Ecosystem: EcoPyPI, Name: "django", Spec: "==4.2.1"}, latest("6.1.3"), StatusMajor, 2},
		{"pypi compatible release", Item{Ecosystem: EcoPyPI, Name: "django", Spec: "~=6.0"}, latest("6.1.3"), StatusCurrent, 0},
		{"pypi upper cap below latest", Item{Ecosystem: EcoPyPI, Name: "django", Spec: ">=4,<5"}, latest("6.1.3"), StatusMajor, 2},
		{"cargo bare is caret", Item{Ecosystem: EcoCrates, Name: "serde", Spec: "1.0"}, latest("1.0.229"), StatusCurrent, 0},
		{"cargo exact", Item{Ecosystem: EcoCrates, Name: "serde", Spec: "=0.9.1"}, latest("1.0.229"), StatusMajor, 1},
		{"image major", Item{Ecosystem: EcoImage, Name: "postgres", Spec: "13"}, latest("18"), StatusMajor, 5},
		{"image same major", Item{Ecosystem: EcoImage, Name: "postgres", Spec: "18"}, latest("18"), StatusCurrent, 0},
		{"mysql 8.0", Item{Ecosystem: EcoImage, Name: "mysql", Spec: "8.0"}, latest("9.5"), StatusMajor, 1},
		{"pre-release pin newer than latest", Item{Ecosystem: EcoNPM, Name: "x", Spec: "20.0.0-rc.1"}, latest("19.3.1"), StatusCurrent, 0},
		{"unparseable spec", Item{Ecosystem: EcoNPM, Name: "x", Spec: "next"}, latest("1.0.0"), StatusSkipped, 0},
		{"unknown latest", Item{Ecosystem: EcoNPM, Name: "x", Spec: "1"}, Latest{Err: "offline"}, StatusUnknown, 0},
	} {
		got := Classify(tc.item, tc.latest)
		assert.Equal(t, tc.want, got.Status, tc.name)
		assert.Equal(t, tc.lines, got.LinesBehind, tc.name)
	}
}

func TestReplyPins(t *testing.T) {
	pins := ReplyPins("done.\nversion-pin: postgres 13 matches the managed RDS fleet\n- version-pin: npm:React legacy plugin\nversion-pin: nogood\n")
	assert.True(t, PinnedBy(Item{Ecosystem: EcoImage, Name: "postgres", Spec: "13"}, pins))
	assert.True(t, PinnedBy(Item{Ecosystem: EcoNPM, Name: "react", Spec: "^17"}, pins))
	assert.False(t, PinnedBy(Item{Ecosystem: EcoNPM, Name: "nogood", Spec: "1"}, pins), "a pin needs a reason")
	assert.False(t, PinnedBy(Item{Ecosystem: EcoNPM, Name: "vue", Spec: "2"}, pins))
	assert.True(t, PinnedBy(Item{Ecosystem: EcoNPM, Name: "vue", Spec: "2", Pinned: true}, pins))
}

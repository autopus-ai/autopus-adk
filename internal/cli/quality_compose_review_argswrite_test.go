package cli

// SPEC-REVMODEL-001 T12 (REQ-RVM-25, 26; Line Splice Writer Contract for
// CLI args): every args node shape takes only its target values, a missing
// or empty args takes the migrated args with the target values changed, and
// only a target value placeholder or an aliased, anchored, or block node
// refuses.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

func reviewClaudeTo(value reviewCLIValue) func(*reviewComposition) {
	return func(w *reviewComposition) { w.CLI["claude"] = value }
}

func TestSaveReviewComposition_ClaudeArgsShapes(t *testing.T) {
	opusHigh := reviewCLIValue{"claude-opus-5-5", "high"}
	block := "            args:\n                - --print\n                - --model\n                - claude-fable-5-1\n" +
		"                - --effort\n                - max\n"
	cases := []struct {
		name  string
		args  string // replaces the RL claude args line
		value reviewCLIValue
		want  string
	}{
		{"block list", block, opusHigh, strings.Replace(strings.Replace(block, "claude-fable-5-1", "claude-opus-5-5", 1), "- max", "- high", 1)},
		{"block list drops the effort pair", block, reviewCLIValue{"claude-haiku-4-5", ""},
			"            args:\n                - --print\n                - --model\n                - claude-haiku-4-5\n"},
		{"block list appends", "            args:\n                - --print # print\n", opusHigh,
			"            args:\n                - --print # print\n                - --model\n                - claude-opus-5-5\n" +
				"                - --effort\n                - high\n"},
		{"empty flow list", "            args: []\n", opusHigh, "            args: [--print, --model, claude-opus-5-5, --effort, high, --output-format, json]\n"},
		{"null value", "            args:\n", opusHigh, "            args: [--print, --model, claude-opus-5-5, --effort, high, --output-format, json]\n"},
		{"flow list keeps other tokens", "            args: [--print,  --verbose, --model, 'claude-fable-5-1', --effort, max] # mine\n", opusHigh,
			"            args: [--print,  --verbose, --model, claude-opus-5-5, --effort, high] # mine\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeReviewRL(t)
			replaceReviewFile(t, dir, reviewRLClaudeArgs+"\n", tc.args)
			before := readReviewFile(t, dir)

			cfg, err := saveReviewEdit(t, dir, reviewClaudeTo(tc.value))

			require.NoError(t, err)
			assert.Equal(t, strings.Replace(before, tc.args, tc.want, 1), readReviewFile(t, dir))
			assert.Equal(t, tc.value, reviewCLIValueOf(reviewClaudeRow, cfg.Orchestra.Providers["claude"].Args))
		})
	}
}

// A claude mapping without args takes the migrated default args, with the
// target values changed, at the end of the mapping.
func TestSaveReviewComposition_MissingArgsTakeTheMigratedArgs(t *testing.T) {
	dir := writeReviewRL(t)
	replaceReviewFile(t, dir, reviewRLClaudeArgs+"\n", "")
	before := readReviewFile(t, dir)

	_, err := saveReviewEdit(t, dir, reviewClaudeTo(reviewCLIValue{"claude-sonnet-5-5", "low"}))

	require.NoError(t, err)
	assert.Equal(t, strings.Replace(before, "                timeout: 480\n        codex:\n",
		"                timeout: 480\n            args: [--print, --model, claude-sonnet-5-5, --effort, low, --output-format, json]\n        codex:\n", 1),
		readReviewFile(t, dir))
}

func TestSaveReviewComposition_ArgsRefusals(t *testing.T) {
	t.Setenv("REVIEW_MODEL", "claude-fable-5-1")
	cases := map[string]string{
		"placeholder target": "            args: [--print, --model, \"${REVIEW_MODEL}\", --effort, max]",
		"anchored args":      "            args: &a [--print, --model, claude-fable-5-1, --effort, max]",
		"two-line flow list": "            args: [--print, --model, claude-fable-5-1,\n                --effort, max]",
		"block scalar":       "            args: |-\n                --print",
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			dir := writeReviewRL(t)
			path := filepath.Join(dir, "autopus.yaml")
			cfg, err := config.Load(dir)
			require.NoError(t, err)
			replaceReviewFile(t, dir, reviewRLClaudeArgs, args)
			before := readReviewFile(t, dir)
			baseline := newReviewBaseline(cfg, readReviewRaw(dir))
			working := baseline.reviewComposition.clone()
			working.CLI["claude"] = reviewCLIValue{"claude-opus-5-5", "max"}

			err = saveReviewComposition(dir, cfg, baseline, working)

			var refusal reviewWriteRefusal
			require.True(t, errors.As(err, &refusal), "%v", err)
			assert.Equal(t, "orchestra.providers.claude.args", refusal.key)
			data, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, before, string(data))
		})
	}
}

// A placeholder in another element blocks nothing and stays as it is.
func TestSaveReviewComposition_OtherPlaceholdersStay(t *testing.T) {
	t.Setenv("REVIEW_TOKEN", "rvm-sentinel-7f3a9c")
	dir := writeReviewRL(t)
	args := `            args: [--print, --api-key, "${REVIEW_TOKEN}", --model, claude-fable-5-1, --effort, max]`
	replaceReviewFile(t, dir, reviewRLClaudeArgs, args)
	before := readReviewFile(t, dir)

	_, err := saveReviewEdit(t, dir, reviewClaudeTo(reviewCLIValue{"claude-opus-5-5", "max"}))

	require.NoError(t, err)
	assert.Equal(t, strings.Replace(before, "claude-fable-5-1, --effort", "claude-opus-5-5, --effort", 1), readReviewFile(t, dir))
}

// An args element the edit leaves keeps any scalar tag: a number or a bool
// next to the target values blocks nothing and stays as it is, in a flow or
// a block list, around a set, a removal, or an append (REQ-RVM-25).
func TestSaveReviewComposition_UntouchedNonStringElementsStay(t *testing.T) {
	block := func(items ...string) string {
		text := "            args:\n"
		for _, item := range items {
			text += "                - " + item + "\n"
		}
		return text
	}
	opusXHigh := reviewCLIValue{"claude-opus-5-5", "xhigh"}
	cases := []struct {
		name, args, want string
		value            reviewCLIValue
	}{
		{"flow list", "            args: [--print, --max-turns, 30, --model, claude-fable-5-1, --effort, max, --fork, true]\n",
			"            args: [--print, --max-turns, 30, --model, claude-opus-5-5, --effort, xhigh, --fork, true]\n", opusXHigh},
		{"block list", block("--print", "--max-turns", "30", "--model", "claude-fable-5-1", "--effort", "max", "--fork", "true"),
			block("--print", "--max-turns", "30", "--model", "claude-opus-5-5", "--effort", "xhigh", "--fork", "true"), opusXHigh},
		{"flow list drops the effort pair before a number", "            args: [--print, --effort, max, --max-turns, 30, --model, claude-fable-5-1]\n",
			"            args: [--print, --max-turns, 30, --model, claude-haiku-4-5]\n", reviewCLIValue{Model: "claude-haiku-4-5"}},
		{"block list appends after a number", block("--print", "--max-turns", "30"),
			block("--print", "--max-turns", "30", "--model", "claude-opus-5-5", "--effort", "xhigh"), opusXHigh},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeReviewRL(t)
			replaceReviewFile(t, dir, reviewRLClaudeArgs+"\n", tc.args)
			before := readReviewFile(t, dir)

			cfg, err := saveReviewEdit(t, dir, reviewClaudeTo(tc.value))

			require.NoError(t, err)
			assert.Equal(t, strings.Replace(before, tc.args, tc.want, 1), readReviewFile(t, dir))
			assert.Equal(t, tc.value, reviewCLIValueOf(reviewClaudeRow, cfg.Orchestra.Providers["claude"].Args))
		})
	}
}

// heldBy holds an element the splice writes to a plain string and lets an
// element it leaves keep its tag, never its value.
func TestReviewTargetHeldBy_OnlyWrittenElementsMustBeStrings(t *testing.T) {
	var document yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("[--max-turns, 30, --model, 31]"), &document))
	node := document.Content[0]
	target := reviewTarget{list: []string{"--max-turns", "30", "--model", "31"}, args: &reviewArgsTarget{}}

	target.written = []bool{false, false, false, false}
	assert.True(t, target.heldBy(node))
	target.written = []bool{false, false, false, true}
	assert.False(t, target.heldBy(node), "a written 31 is a number")
	target.written = nil
	assert.False(t, target.heldBy(node), "a whole written list holds strings only")
	target.written, target.list[1] = []bool{false, false, false, false}, "40"
	assert.False(t, target.heldBy(node), "a left element keeps its raw value")
}

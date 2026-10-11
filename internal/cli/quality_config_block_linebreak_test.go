package cli

// SPEC-MODELUX-001 REQ-MUX-08, security review F2: yaml.v3 counts a lone CR,
// U+0085, U+2028, and U+2029 as line breaks, while the block splice and the
// preview's comment count split the bytes at \n only. Their line numbers
// then disagree, and a file holding such a character before quality made
// both index past the end of the lines and crash `auto update` before it
// generated anything. Such a file is refused, the preview says so, and no
// path panics.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// lineBreakSkews spells each character yaml.v3 reads as a line break that a
// split at \n does not see.
var lineBreakSkews = map[string]string{
	"lone CR": "\r",
	"U+0085":  "\u0085",
	"U+2028":  "\u2028",
	"U+2029":  "\u2029",
}

// withoutPanic runs write and turns a panic into an error, so one red case
// does not end the whole test binary.
func withoutPanic(write func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return write()
}

func TestQualityCompositionWrites_RefuseALineBreakTheSpliceCannotSee(t *testing.T) {
	for name, skew := range lineBreakSkews {
		content := "project_name: demo\nmode: full\nplatforms: [claude-code]\n" +
			strings.Repeat("# note"+skew, 40) + "\nquality:\n  default: balanced\n  presets:\n" +
			"    balanced:\n      description: b\n    ultra:\n      description: u\ndesign:\n  enabled: false\n"
		writers := map[string]func(dir string, cfg *config.HarnessConfig) error{
			"block": func(dir string, cfg *config.HarnessConfig) error {
				return saveQualityBlock(dir, cfg, compositionCustomEdit)
			},
			"line": func(dir string, cfg *config.HarnessConfig) error {
				return persistComposeBinding(dir, cfg, composeBindingPlan{Writer: composeWriterLine, Default: "ultra"})
			},
		}
		for writer, write := range writers {
			t.Run(name+"/"+writer, func(t *testing.T) {
				dir := writeBlockFixture(t, content)
				cfg, err := config.LoadPreview(dir)
				require.NoError(t, err)

				err = withoutPanic(func() error { return write(dir, cfg) })

				require.ErrorIs(t, err, errQualityLineBreakRefused)
				assert.Equal(t, content, readBlockFixture(t, dir))
				assert.Equal(t, "balanced", cfg.Quality.Default)
			})
		}
	}
}

// yaml.v3 writes U+2028 and U+2029 unescaped, so a value that holds one, here
// through the \L escape, would leave the re-encoded block with the same skew.
func TestSaveQualityBlock_RefusesARewriteThatWouldHoldALineBreakTheSpliceCannotSee(t *testing.T) {
	content := "project_name: demo\nmode: full\nplatforms: [claude-code]\nquality:\n  default: balanced\n" +
		"  presets:\n    balanced:\n      description: \"a\\Lb\"\n    ultra:\n      description: u\n" +
		"design:\n  enabled: false\n"
	dir := writeBlockFixture(t, content)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	err = withoutPanic(func() error { return saveQualityBlock(dir, cfg, compositionCustomEdit) })

	require.ErrorIs(t, err, errQualityLineBreakRefused)
	assert.Equal(t, content, readBlockFixture(t, dir))
}

// The block range refuses line numbers outside the lines it was handed
// instead of indexing past them.
func TestQualityBlockRange_RefusesLinesOutsideTheText(t *testing.T) {
	t.Parallel()
	var doc yaml.Node
	require.NoError(t, yaml.Unmarshal([]byte("quality:\n  default: balanced\ndesign:\n  enabled: false\n"), &doc))
	root := doc.Content[0]
	lines := []string{"quality:\n", "  default: balanced\n"}
	for name, move := range map[string]func(){
		"next key past the end": func() { root.Content[2].Line = 9 },
		"key before the start":  func() { root.Content[0].Line = 0 },
		"key after the next":    func() { root.Content[0].Line = 3; root.Content[2].Line = 2 },
	} {
		move()
		_, _, err := qualityBlockRange(lines, root, 0)
		assert.Error(t, err, name)
		root.Content[0].Line, root.Content[2].Line = 1, 3
	}
}

// The probe-A2 shape of the review: one comment line before quality hides
// 400 U+2028 characters. Through the real `auto update`, `u` and a group
// edit preview the change with a line saying it cannot be written, the
// confirmed write is refused with one warning line, autopus.yaml stays
// byte-identical, and the update still generates.
func TestUpdateCompose_LineBreakTheSpliceCannotSee_WarnsAndUpdateContinues(t *testing.T) {
	base := newComposeUpdateRun(t, true, "")
	settled := base.settled(t, "R")
	for name, input := range map[string]string{"u": "u\ny\n", "g": "g\n1\n\n\n\ny\n"} {
		t.Run(name, func(t *testing.T) {
			run := base.rerun(t, true, "")
			dir := cloneComposeProject(t, settled)
			path := filepath.Join(dir, "autopus.yaml")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			skewed := "# note" + strings.Repeat(" \u2028#", 400) + "\n" + string(data)
			require.NoError(t, os.WriteFile(path, []byte(skewed), 0o644))

			var out string
			err = withoutPanic(func() error {
				var runErr error
				out, runErr = runComposeUpdate(t, strings.NewReader(input), "--dir", dir)
				return runErr
			})

			require.NoError(t, err, out)
			assert.Equal(t, skewed, readBlockFixture(t, dir))
			assert.Contains(t, out, "  "+composeBindTextFor("ko").blockUnreadable+"\n")
			assert.Equal(t, 1, strings.Count(out, "  경고: "+errQualityLineBreakRefused.Error()+"\n"), out)
			assert.NotContains(t, out, "구성을 적용했습니다.")
			assert.Contains(t, out, "Update complete: 5 platform(s) updated")
			assert.Len(t, *run.probes, 1)
		})
	}
}

package cli

// SPEC-REA-001 T12 (acceptance S6, S7): the Update Prompt Contract inside
// the real `auto update`, over projects built by the real `auto init`.

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/reamcp"
)

// reaReadCounter counts the Read calls an update makes on its input.
type reaReadCounter struct {
	r     io.Reader
	reads int
}

func (c *reaReadCounter) Read(p []byte) (int, error) {
	c.reads++
	return c.r.Read(p)
}

// reaOLD returns a project as v0.50.125 leaves it: platforms, no
// features.rea, and no rea entry.
func reaOLD(t *testing.T, platforms string) string {
	t.Helper()
	dir := t.TempDir()
	out, err := reaInit(t, dir, platforms, "--yes", "--no-rea")
	require.NoError(t, err, out)
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	cfg.Features.Rea.Enabled = nil
	require.NoError(t, config.Save(dir, cfg))
	return dir
}

// reaUpdate runs the real `auto update` in dir over in.
func reaUpdate(t *testing.T, in io.Reader, dir string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(in)
	root.SetArgs(append([]string{"update", "--dir", dir, "--local"}, args...))
	err := root.Execute()
	return out.String(), err
}

// reaSums returns the SHA-256 of the three target files and autopus.yaml.
func reaSums(t *testing.T, dir string) map[string]string {
	t.Helper()
	sums := map[string]string{}
	for _, rel := range []string{".mcp.json", ".codex/config.toml", ".agents/plugins/autopus/mcp_config.json", "autopus.yaml"} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			sums[rel] = "missing"
			continue
		}
		sum := sha256.Sum256(data)
		sums[rel] = hex.EncodeToString(sum[:])
	}
	return sums
}

// reaCounts returns how often the run printed the question, the hint, and
// the old-binary line.
func reaCounts(out string) [3]int {
	return [3]int{
		strings.Count(out, reaTextKo.Question),
		strings.Count(out, reaTextKo.Hint),
		strings.Count(out, reaTextKo.OldBinary),
	}
}

// reaKey returns the stored features.rea.enabled of dir.
func reaKey(t *testing.T, dir string) *bool {
	t.Helper()
	cfg, err := config.Load(dir)
	require.NoError(t, err)
	return cfg.Features.Rea.Enabled
}

// S7 (1), (2): an interactive undecided update asks once; Enter registers
// and keeps the key absent, n stores false with the old-binary line; the
// next interactive run asks nothing and changes nothing.
func TestUpdateRea_InteractiveQuestion(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	old := reaOLD(t, reaAllTargets)

	// Enter takes the default, No, and leaves REA undecided: nothing is
	// registered and autopus.yaml keeps no key, so the next run asks again.
	enter := cloneComposeProject(t, old)
	stubDriverProbes(t, true, "")
	before := reaSums(t, enter)
	out, err := reaUpdate(t, strings.NewReader("\n\n"), enter)
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{1, 1, 0}, reaCounts(out), out)
	assertReaAbsent(t, enter)
	assert.Nil(t, reaKey(t, enter))
	assert.Equal(t, before, reaSums(t, enter))

	yes := cloneComposeProject(t, old)
	out, err = reaUpdate(t, strings.NewReader("y\n\n"), yes)
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{1, 0, 0}, reaCounts(out), out)
	assertReaManagedOnce(t, yes)
	assert.Nil(t, reaKey(t, yes))
	settled := reaSums(t, yes)
	out, err = reaUpdate(t, strings.NewReader("\n\n"), yes)
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{0, 0, 0}, reaCounts(out), out)
	assert.Equal(t, settled, reaSums(t, yes))

	no := cloneComposeProject(t, old)
	out, err = reaUpdate(t, strings.NewReader("n\n\n"), no)
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{1, 0, 1}, reaCounts(out), out)
	assertReaAbsent(t, no)
	assert.Equal(t, reaBool(false), reaKey(t, no))
	settled = reaSums(t, no)
	out, err = reaUpdate(t, strings.NewReader("\n\n"), no)
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{0, 0, 0}, reaCounts(out), out)
	assert.Equal(t, settled, reaSums(t, no))
}

// S7 (3), (4), (5), (16), (6): every non-interactive undecided run reads no
// stdin, writes no rea entry and no key, and prints the hint once; a preview
// also leaves every file as it was.
func TestUpdateRea_NonInteractiveHint(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	old := reaOLD(t, reaAllTargets)
	runs := []struct {
		name string
		tty  bool
		ci   string
		args []string
	}{
		{"(3) non-TTY", false, "", nil},
		{"(4) --yes", true, "", []string{"--yes"}},
		{"(5) --auto", true, "", []string{"--auto"}},
		{"(16) TTY with CI=true", true, "true", nil},
		{"(6) --plan", true, "", []string{"--plan"}},
	}
	for _, run := range runs {
		dir := cloneComposeProject(t, old)
		before := reaSums(t, dir)
		stubDriverProbes(t, run.tty, run.ci)
		in := &reaReadCounter{r: strings.NewReader("")}

		out, err := reaUpdate(t, in, dir, run.args...)

		require.NoError(t, err, run.name+"\n"+out)
		assert.Equal(t, [3]int{0, 1, 0}, reaCounts(out), run.name+"\n"+out)
		assertReaAbsent(t, dir)
		assert.Nil(t, reaKey(t, dir), run.name)
		if !run.tty || run.ci != "" {
			assert.Zero(t, in.reads, run.name)
		}
		if run.name == "(6) --plan" {
			assert.Equal(t, before, reaSums(t, dir), run.name)
		}
	}
}

// S7 (8), (9), (10), (14): flags decide without asking; both together, or
// either with --workspace, fail before any write; a preview with --rea
// plans the three files and writes nothing.
func TestUpdateRea_Flags(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	stubDriverProbes(t, false, "")
	old := reaOLD(t, reaAllTargets)

	dir := cloneComposeProject(t, old)
	out, err := reaUpdate(t, strings.NewReader(""), dir, "--rea")
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{0, 0, 0}, reaCounts(out), out)
	assertReaManagedOnce(t, dir)
	assert.Nil(t, reaKey(t, dir))
	out, err = reaUpdate(t, strings.NewReader(""), dir, "--no-rea")
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{0, 0, 1}, reaCounts(out), out)
	assertReaAbsent(t, dir)
	assert.Equal(t, reaBool(false), reaKey(t, dir))

	both := cloneComposeProject(t, old)
	before := reaSums(t, both)
	_, err = reaUpdate(t, strings.NewReader(""), both, "--rea", "--no-rea")
	require.EqualError(t, err, "--rea와 --no-rea는 함께 쓸 수 없습니다")
	assert.Equal(t, before, reaSums(t, both))

	root := NewRootCmd()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"update", "--dir", both, "--workspace", "--yes", "--rea"})
	require.EqualError(t, root.Execute(), "--rea·--no-rea는 --workspace와 함께 쓸 수 없습니다. repo마다 --local로 실행하세요")
	assert.Equal(t, before, reaSums(t, both))

	out, err = reaUpdate(t, strings.NewReader(""), both, "--plan", "--rea")
	require.NoError(t, err, out)
	assert.Equal(t, before, reaSums(t, both))
	assert.Equal(t, [3]int{0, 0, 0}, reaCounts(out), out)
	for _, rel := range []string{".mcp.json", ".codex/config.toml"} {
		assert.Contains(t, out, "emit "+rel+" ", "the preview plans the rea change")
	}
	// The Antigravity plugin MCP file is written outside the adapter manifest,
	// so the preview names it through the REA plan instead of a mapping.
	assert.Contains(t, out, "create .agents/plugins/autopus/mcp_config.json (antigravity-cli) — REA would register")
	assert.NotContains(t, out, "features.rea.enabled", "the planned entries carry the decision")

	out, err = reaUpdate(t, strings.NewReader(""), both, "--plan", "--no-rea")
	require.NoError(t, err, out)
	assert.Equal(t, before, reaSums(t, both))
	assert.Contains(t, out, "update autopus.yaml — REA decision would be persisted in autopus.yaml (features.rea.enabled: false)")
}

// S7 (13): a manifest-tracked .mcp.json the operator deleted is not
// recreated by --rea, so no managed entry carries the decision and the key
// stores true with the old-binary line.
func TestUpdateRea_DeletedTargetIsNotRecreated(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	stubDriverProbes(t, false, "")
	dir := reaOLD(t, "claude-code")
	require.NoError(t, os.Remove(filepath.Join(dir, ".mcp.json")))

	out, err := reaUpdate(t, strings.NewReader(""), dir, "--yes", "--rea")

	require.NoError(t, err, out)
	assert.NoFileExists(t, filepath.Join(dir, ".mcp.json"))
	assert.Equal(t, reaBool(true), reaKey(t, dir))
	assert.Equal(t, [3]int{0, 0, 1}, reaCounts(out), out)
}

// S6: over Q a failing Node gate keeps every file and autopus.yaml, prints
// one Node line, and probes once; a stored off decision never probes.
func TestUpdateRea_NodeGateKeepsEntries(t *testing.T) {
	recorder := reaCLIEnv(t, "v24.11.0\n")
	stubDriverProbes(t, false, "")
	q := reaOLD(t, reaAllTargets)
	out, err := reaUpdate(t, strings.NewReader(""), q, "--yes", "--rea")
	require.NoError(t, err, out)
	assertReaManagedOnce(t, q)

	gates := []struct {
		stdout string
		exit   int
		line   string
	}{
		{"v22.18.0\n", 0, "REA MCP를 등록하지 않았습니다: node v22.18.0은 ^22.19.0 || ^24.11.0 || >=26.0.0 범위가 아닙니다"},
		{"", 127, "REA MCP를 등록하지 않았습니다: node를 찾지 못했습니다 (^22.19.0 || ^24.11.0 || >=26.0.0 필요)"},
		{"garbage\n", 0, "REA MCP를 등록하지 않았습니다: node 버전을 확인하지 못했습니다"},
	}
	for _, gate := range gates {
		before := reaSums(t, q)
		recorder.stdout, recorder.exit, recorder.commands = gate.stdout, gate.exit, nil

		out, err := reaUpdate(t, strings.NewReader(""), q, "--yes")

		require.NoError(t, err, out)
		assert.Equal(t, before, reaSums(t, q), gate.line)
		assert.Equal(t, 1, strings.Count(out, gate.line), out)
		assert.Len(t, recorder.commands, 1, gate.line)
	}

	off := reaOLD(t, reaAllTargets)
	cfg, err := config.Load(off)
	require.NoError(t, err)
	cfg.Features.Rea.Enabled = reaBool(false)
	require.NoError(t, config.Save(off, cfg))
	recorder.commands = nil
	out, err = reaUpdate(t, strings.NewReader(""), off, "--yes")
	require.NoError(t, err, out)
	assert.Empty(t, recorder.commands)
	assert.Equal(t, reamcp.ClassAbsent, reaFileClass(off, reamcp.Claude))
}

// reaEnter puts one Enter before in: an interactive update over an
// undecided project now asks the REA question before the composition step
// (spec.md Existing Test Changes rule 8).
func reaEnter(in io.Reader) io.Reader {
	return io.MultiReader(strings.NewReader("\n"), in)
}

package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/reamcp"
)

// testReaRun starts a run over dir and cfg whose probe answers stdout with
// exit 0 and records its commands.
func testReaRun(t *testing.T, dir string, cfg *config.HarnessConfig, stdout string) (*reaRun, *reaNodeExecRecorder) {
	t.Helper()
	recorder := &reaNodeExecRecorder{stdout: stdout}
	run := newReaRun(dir, cfg, cfg.Features.Rea.Enabled != nil)
	run.text = reaTextFor("ko")
	run.probe = &reaNodeProbe{exec: recorder.exec, goos: "linux"}
	run.snapshot(cfg.Platforms) // what init and update do before their writes
	return run, recorder
}

// The Form Text, exactly, in both locales (spec.md Form Text, acceptance S20).
func TestReaText_FormText(t *testing.T) {
	t.Parallel()
	ko, en := reaTextFor("ko"), reaTextFor("en")
	assert.Equal(t, reaTextFor("ja"), en)
	assert.Equal(t, "REA 역공학 MCP(rea-agents@6.5.0)를 이 프로젝트의 Claude Code·Codex·Antigravity에 등록할까요? [y/N]", ko.Question)
	assert.Equal(t, "Register the REA reverse-engineering MCP (rea-agents@6.5.0) for Claude Code, Codex, and Antigravity in this project? [y/N]", en.Question)
	assert.Equal(t, "REA 역공학 MCP를 아직 정하지 않았습니다. 켜려면 auto update --rea, 끄려면 auto update --no-rea", ko.Hint)
	assert.Equal(t, "REA reverse-engineering MCP is undecided; run auto update --rea to enable or auto update --no-rea to decline", en.Hint)
	assert.Equal(t, "REA MCP not registered: node v20.11.0 is outside ^22.19.0 || ^24.11.0 || >=26.0.0", strings.Replace(en.NodeOutside, "%s", "v20.11.0", 1))
	assert.Equal(t, "REA MCP를 등록하지 않았습니다: node v20.11.0은 ^22.19.0 || ^24.11.0 || >=26.0.0 범위가 아닙니다", strings.Replace(ko.NodeOutside, "%s", "v20.11.0", 1))
	assert.Equal(t, "REA MCP를 등록하지 않았습니다: node를 찾지 못했습니다 (^22.19.0 || ^24.11.0 || >=26.0.0 필요)", ko.NodeAbsent)
	assert.Equal(t, "REA MCP not registered: node not found (needs ^22.19.0 || ^24.11.0 || >=26.0.0)", en.NodeAbsent)
	assert.Equal(t, "REA MCP를 등록하지 않았습니다: node 버전을 확인하지 못했습니다", ko.NodeUnknown)
	assert.Equal(t, "REA MCP not registered: node version unreadable", en.NodeUnknown)
	assert.Equal(t, "features.rea는 v0.50.127보다 오래된 auto가 읽지 못합니다", ko.OldBinary)
	assert.Equal(t, "auto older than v0.50.127 cannot read features.rea", en.OldBinary)
	assert.Equal(t, "REA 역공학 MCP를 등록했습니다: .mcp.json (rea-agents@6.5.0)", strings.Replace(ko.Registered, "%s", ".mcp.json", 1))
	assert.Equal(t, "Registered the REA reverse-engineering MCP: .mcp.json (rea-agents@6.5.0)", strings.Replace(en.Registered, "%s", ".mcp.json", 1))
	assert.Equal(t, "OMP·OpenCode에는 Autopus가 REA MCP를 등록하지 않습니다. /auto reverse 분석은 Claude Code·Codex·Antigravity에서 실행하세요. npx -y rea-agents@6.5.0 setup은 REA를 사용자 범위의 메인 세션에만 등록합니다", ko.Guidance)
	assert.Equal(t, "Autopus does not register REA MCP for OMP or OpenCode; run /auto reverse analysis in Claude Code, Codex, or Antigravity. npx -y rea-agents@6.5.0 setup registers REA for the main session only (user scope)", en.Guidance)
	assert.Equal(t, "--rea와 --no-rea는 함께 쓸 수 없습니다", ko.FlagBoth)
	assert.Equal(t, "--rea and --no-rea cannot be combined with --workspace; run them per repository with --local", en.FlagWorkspace)
	assert.Equal(t, []string{"REA: 켜짐 (managed 항목)", "REA: 켜짐 (autopus.yaml)", "REA: 꺼짐 (autopus.yaml)", "REA: 미결정"},
		[]string{ko.DoctorOnEntry, ko.DoctorOnKey, ko.DoctorOffKey, ko.DoctorUnset})
	assert.Equal(t, []string{"REA: on (managed entry)", "REA: on (autopus.yaml)", "REA: off (autopus.yaml)", "REA: undecided"},
		[]string{en.DoctorOnEntry, en.DoctorOnKey, en.DoctorOffKey, en.DoctorUnset})
}

// Both flags, or either with a workspace run, fail before any write.
func TestCheckReaFlags(t *testing.T) {
	original := composeLanguage
	t.Cleanup(func() { composeLanguage = original })
	composeLanguage = func() string { return "ko" }

	assert.NoError(t, checkReaFlags(true, false, false))
	assert.NoError(t, checkReaFlags(false, false, true))
	assert.EqualError(t, checkReaFlags(true, true, false), "--rea와 --no-rea는 함께 쓸 수 없습니다")
	assert.EqualError(t, checkReaFlags(false, true, true), "--rea·--no-rea는 --workspace와 함께 쓸 수 없습니다. repo마다 --local로 실행하세요")
	assert.Equal(t, reaOn, reaFlagState(true, false))
	assert.Equal(t, reaOff, reaFlagState(false, true))
	assert.Equal(t, reaUndecided, reaFlagState(false, false))
}

// decide maps each decision and Node verdict to the action, prints one Node
// line per failing gate, and probes only an on decision (acceptance S6).
func TestReaRunDecide_ActionsAndNodeLines(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		state  reaState
		stdout string
		action config.ReaMCPAction
		line   string
		probes int
	}{
		{"on, N24", reaOn, "v24.11.0\n", config.ReaMCPAdd, "", 1},
		{"on, N2218", reaOn, "v22.18.0\n", config.ReaMCPKeep, "REA MCP를 등록하지 않았습니다: node v22.18.0은 ^22.19.0 || ^24.11.0 || >=26.0.0 범위가 아닙니다", 1},
		{"on, garbage", reaOn, "garbage\n", config.ReaMCPKeep, "REA MCP를 등록하지 않았습니다: node 버전을 확인하지 못했습니다", 1},
		{"off", reaOff, "v24.11.0\n", config.ReaMCPRemove, "", 0},
		{"undecided", reaUndecided, "v24.11.0\n", config.ReaMCPKeep, "", 0},
	}
	for _, tc := range cases {
		dir, cfg := reaFixture(t, "claude-code")
		run, recorder := testReaRun(t, dir, cfg, tc.stdout)
		var out bytes.Buffer

		run.decide(context.Background(), &out, cfg, tc.state)

		assert.Equal(t, tc.action, cfg.Runtime.ReaMCP, tc.name)
		assert.Len(t, recorder.commands, tc.probes, tc.name)
		if tc.line == "" {
			assert.Empty(t, out.String(), tc.name)
		} else {
			assert.Equal(t, "  "+tc.line+"\n", out.String(), tc.name)
		}
	}

	absentDir, absentCfg := reaFixture(t, "codex")
	absent, recorder := testReaRun(t, absentDir, absentCfg, "")
	recorder.exit = 127
	var out bytes.Buffer
	absent.decide(context.Background(), &out, absentCfg, reaOn)
	assert.Equal(t, "  REA MCP를 등록하지 않았습니다: node를 찾지 못했습니다 (^22.19.0 || ^24.11.0 || >=26.0.0 필요)\n", out.String())
}

// A run without a REA target platform neither probes nor acts, and its only
// line is the OMP·OpenCode guidance (acceptance S3 (7)).
func TestReaRun_InactiveWithoutTarget(t *testing.T) {
	t.Parallel()
	dir, cfg := reaFixture(t, "opencode", "omp")
	run, recorder := testReaRun(t, dir, cfg, "v24.11.0\n")
	var out bytes.Buffer

	run.decide(context.Background(), &out, cfg, reaOn)
	require.NoError(t, run.finish(&out, dir, cfg))

	assert.Empty(t, recorder.commands)
	assert.Equal(t, config.ReaMCPKeep, cfg.Runtime.ReaMCP)
	assert.Equal(t, "  "+reaTextKo.Guidance+"\n", out.String())
	assert.Nil(t, cfg.Features.Rea.Enabled)
}

// finish after an add run: the files this run registered are named once, a
// managed entry keeps the key absent, and no old-binary line is printed.
func TestReaRunFinish_RegistersAndLeavesKeyAbsent(t *testing.T) {
	t.Parallel()
	dir, cfg := reaFixture(t, "claude-code", "codex", "antigravity-cli", "omp")
	writeReaTarget(t, dir, reamcp.Codex, []byte(reaCodexBase))
	run, _ := testReaRun(t, dir, cfg, "v24.11.0\n")
	var out bytes.Buffer
	run.decide(context.Background(), &out, cfg, reaOn)
	writeManagedRea(t, dir, reamcp.Platforms()...) // what the adapters write for add

	require.NoError(t, run.finish(&out, dir, cfg))

	assert.Equal(t, "  REA 역공학 MCP를 등록했습니다: .mcp.json, .codex/config.toml, .agents/plugins/autopus/mcp_config.json (rea-agents@6.5.0)\n"+
		"  "+reaTextKo.Guidance+"\n", out.String())
	reloaded, err := config.Load(dir)
	require.NoError(t, err)
	assert.Nil(t, reloaded.Features.Rea.Enabled)

	rerun, _ := testReaRun(t, dir, cfg, "v24.11.0\n")
	out.Reset()
	rerun.decide(context.Background(), &out, cfg, reaOn)
	require.NoError(t, rerun.finish(&out, dir, cfg))
	assert.NotContains(t, out.String(), "등록했습니다", "a rerun over current entries registers nothing")
}

// finish after a failed gate: no entry, so the key stores true, and the
// old-binary line appears once because autopus.yaml did not hold the key.
func TestReaRunFinish_GateFailureStoresTrue(t *testing.T) {
	t.Parallel()
	dir, cfg := reaFixture(t, "claude-code")
	run, _ := testReaRun(t, dir, cfg, "v20.11.0\n")
	var out bytes.Buffer
	run.decide(context.Background(), &out, cfg, reaOn)

	require.NoError(t, run.finish(&out, dir, cfg))

	assert.Equal(t, 1, strings.Count(out.String(), reaTextKo.OldBinary))
	assert.Equal(t, 1, strings.Count(out.String(), "node v20.11.0은"))
	reloaded, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, reaBool(true), reloaded.Features.Rea.Enabled)
}

// finish after an off run: the key stores false once with the old-binary
// line, the unselected file is swept, and the guidance line is gone; a second
// off run writes nothing and prints nothing.
func TestReaRunFinish_OffStoresFalseAndSweeps(t *testing.T) {
	t.Parallel()
	dir, cfg := reaFixture(t, "claude-code", "omp")
	writeManagedRea(t, dir, reamcp.Codex)
	run, _ := testReaRun(t, dir, cfg, "v24.11.0\n")
	var out bytes.Buffer
	run.decide(context.Background(), &out, cfg, reaOff)

	require.NoError(t, run.finish(&out, dir, cfg))

	assert.Equal(t, "  "+reaTextKo.OldBinary+"\n", out.String())
	assert.Equal(t, reaCodexBase, string(readReaFile(t, dir, reamcp.Codex)))
	reloaded, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, reaBool(false), reloaded.Features.Rea.Enabled)

	again, _ := testReaRun(t, dir, reloaded, "v24.11.0\n")
	out.Reset()
	again.decide(context.Background(), &out, reloaded, reaOff)
	require.NoError(t, again.finish(&out, dir, reloaded))
	assert.Empty(t, out.String())
}

// An unselected target whose directory is a symlink out of the project is
// never followed: the off sweep prints one skip line, writes nothing, does
// not fail the run, and still stores the off decision (acceptance S16 (d)).
func TestReaRunFinish_SweepSkipsASymlinkedTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture")
	}
	t.Parallel()
	dir, cfg := reaFixture(t, "claude-code")
	outside := t.TempDir()
	managed := reaApplied(t, reamcp.Codex, []byte(reaCodexBase), config.ReaMCPAdd)
	require.NoError(t, os.WriteFile(filepath.Join(outside, "config.toml"), managed, 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(dir, ".codex")))
	run, _ := testReaRun(t, dir, cfg, "v24.11.0\n")
	var out bytes.Buffer
	run.decide(context.Background(), &out, cfg, reaOff)

	require.NoError(t, run.finish(&out, dir, cfg))

	assert.Equal(t, 1, strings.Count(out.String(), "  REA MCP 대상 파일을 건너뜁니다: .codex/config.toml (symlink를 따라가지 않습니다)\n"), out.String())
	reloaded, loadErr := config.Load(dir)
	require.NoError(t, loadErr)
	assert.Equal(t, reaBool(false), reloaded.Features.Rea.Enabled)
	data, readErr := os.ReadFile(filepath.Join(outside, "config.toml"))
	require.NoError(t, readErr)
	assert.Equal(t, managed, data)
}

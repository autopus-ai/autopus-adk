package cli

// Antigravity CLI reads plugin MCP servers from <plugin>/mcp_config.json and
// never from a project .gemini/settings.json, so REA's Antigravity target is
// the workspace plugin's file (SPEC-REA-001, agy 1.4 smoke of 2026-10-17).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/reamcp"
)

// reaPluginMCPRel is the Antigravity target file.
const reaPluginMCPRel = ".agents/plugins/autopus/mcp_config.json"

// A real init registers Antigravity in the plugin file, leaves the settings
// file without a rea entry, and doctor names the file each class was read
// from.
func TestReaPluginMCP_InitRegistersInThePluginAndDoctorNamesTheFile(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	dir := t.TempDir()
	out, err := reaInit(t, dir, reaAllTargets, "--yes", "--rea")
	require.NoError(t, err, out)

	assert.Equal(t, reaPluginMCPRel, reamcp.TargetRelPath(reamcp.Antigravity))
	assertReaManagedOnce(t, dir)
	settings, err := os.ReadFile(filepath.Join(dir, ".gemini", "settings.json"))
	require.NoError(t, err)
	assert.NotContains(t, string(settings), "rea-agents")

	text, _ := runReaCLI(t, "", "doctor", "--dir", dir)
	for _, line := range []string{
		"claude-code (.mcp.json): managed",
		"codex (.codex/config.toml): managed",
		"antigravity-cli (" + reaPluginMCPRel + "): managed",
	} {
		assert.Equal(t, 1, strings.Count(text, line), line+"\n"+text)
	}
}

// The off sweep deletes the plugin file when the managed entry was all it
// held, keeps a server the user added there, and leaves a file it cannot
// decode alone.
func TestReaPluginMCP_SweepRemovesOnlyTheManagedEntry(t *testing.T) {
	t.Parallel()
	alone := t.TempDir()
	writeManagedRea(t, alone, reamcp.Antigravity)
	require.NoError(t, removeReaEntry(alone, reamcp.Antigravity))
	assert.NoFileExists(t, filepath.Join(alone, filepath.FromSlash(reaPluginMCPRel)))

	shared := t.TempDir()
	mine := []byte(`{"mcpServers": {"mine": {"command": "mine-mcp"}}}`)
	writeReaTarget(t, shared, reamcp.Antigravity, reaApplied(t, reamcp.Antigravity, mine, config.ReaMCPAdd))
	require.NoError(t, removeReaEntry(shared, reamcp.Antigravity))
	assert.Equal(t, string(reaApplied(t, reamcp.Antigravity, reaApplied(t, reamcp.Antigravity, mine, config.ReaMCPAdd), config.ReaMCPRemove)),
		string(readReaFile(t, shared, reamcp.Antigravity)))
	assert.NotContains(t, string(readReaFile(t, shared, reamcp.Antigravity)), "rea-agents")

	jsonc := t.TempDir()
	commented := []byte("{\n  // mine\n  \"mcpServers\": {},\n}\n")
	writeReaTarget(t, jsonc, reamcp.Antigravity, commented)
	require.NoError(t, removeReaEntry(jsonc, reamcp.Antigravity))
	assert.Equal(t, commented, readReaFile(t, jsonc, reamcp.Antigravity))
	assert.Equal(t, reamcp.ClassUserOwned, reaFileClass(jsonc, reamcp.Antigravity))
}

// A preview names the plugin file the run would create or delete, which no
// planned mapping shows, and a run that keeps the entry names nothing.
func TestReaPluginMCP_PreviewItems(t *testing.T) {
	t.Parallel()
	dir, cfg := reaFixture(t, "antigravity-cli")
	cfg.Runtime.ReaMCP = config.ReaMCPAdd
	items := reaPluginPreviewItems(dir, cfg)
	require.Len(t, items, 1)
	assert.Equal(t, previewItem{Path: reaPluginMCPRel, Kind: "create", Scope: "antigravity-cli",
		Reason: "REA would register its managed rea entry in this plugin MCP file"}, items[0])

	writeManagedRea(t, dir, reamcp.Antigravity)
	assert.Empty(t, reaPluginPreviewItems(dir, cfg), "the current entry needs no write")
	cfg.Runtime.ReaMCP = config.ReaMCPRemove
	items = reaPluginPreviewItems(dir, cfg)
	require.Len(t, items, 1)
	assert.Equal(t, "remove", items[0].Kind)
	cfg.Runtime.ReaMCP = config.ReaMCPKeep
	assert.Empty(t, reaPluginPreviewItems(dir, cfg))
	cfg.Runtime.ReaMCP = config.ReaMCPRemove
	cfg.Platforms = []string{"claude-code"}
	assert.Empty(t, reaPluginPreviewItems(dir, cfg), "an unselected platform belongs to the off sweep")
}

// reaPluginJSONC is a plugin MCP file agy reads but strict JSON refuses: a
// comment and trailing commas, with or without the managed rea entry.
func reaPluginJSONC(withRea bool) []byte {
	servers := `"mine": {"command": "mine-mcp"},`
	if withRea {
		servers += ` "rea": {"command": "/bin/bash", "args": ["-l", "-c", "exec npx -y --prefix=/ rea-agents@6.5.0 mcp"]},`
	}
	return []byte("{\n  // mine\n  \"mcpServers\": {" + servers + "},\n}\n")
}

// Reviewer F2: a plugin file that is not strict JSON is user-owned. --rea
// leaves it, registers the other files, and prints one line with the remedy;
// --no-rea prints one line when the file still names rea-agents and none when
// it does not; no run fails.
func TestReaPluginMCP_NotStrictJSONSkipsWithARemedy(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	stubDriverProbes(t, false, "")
	dir := reaOLD(t, reaAllTargets)
	jsonc := reaPluginJSONC(false)
	writeReaTarget(t, dir, reamcp.Antigravity, jsonc)
	notRegistered := fmt.Sprintf(reaTextKo.PluginNotRegistered, reaPluginMCPRel)
	notRemoved := fmt.Sprintf(reaTextKo.PluginNotRemoved, reaPluginMCPRel)

	out, err := reaUpdate(t, strings.NewReader(""), dir, "--yes", "--rea")
	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, notRegistered), out)
	assert.Contains(t, out, "REA 역공학 MCP를 등록했습니다: .mcp.json, .codex/config.toml (rea-agents@6.5.0)")
	assert.Equal(t, jsonc, readReaFile(t, dir, reamcp.Antigravity))

	withRea := reaPluginJSONC(true)
	writeReaTarget(t, dir, reamcp.Antigravity, withRea)
	out, err = reaUpdate(t, strings.NewReader(""), dir, "--yes", "--no-rea")
	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, notRemoved), out)
	assert.NotContains(t, out, notRegistered)
	assert.Equal(t, withRea, readReaFile(t, dir, reamcp.Antigravity))

	writeReaTarget(t, dir, reamcp.Antigravity, jsonc)
	out, err = reaUpdate(t, strings.NewReader(""), dir, "--yes", "--no-rea")
	require.NoError(t, err, out)
	assert.NotContains(t, out, notRemoved, "a file that names no rea entry needs no removal")

	writeReaTarget(t, dir, reamcp.Antigravity, withRea)
	out, err = runReaCLI(t, "", "platform", "remove", "antigravity-cli", "--dir", dir)
	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, notRemoved), out)
	assert.Equal(t, withRea, readReaFile(t, dir, reamcp.Antigravity))
}

// Doctor warns on that state: an on decision cannot register in the file,
// and an off decision cannot remove the rea entry it still names.
func TestReaPluginMCP_DoctorWarnsOnANotStrictPluginFile(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	dir, cfg := reaFixture(t, "claude-code", "codex", "antigravity-cli")
	cfg.Features.Rea.Enabled = reaBool(false)
	require.NoError(t, config.Save(dir, cfg))
	writeReaTarget(t, dir, reamcp.Antigravity, reaPluginJSONC(true))

	_, checks, status := reaDoctorJSON(t, dir)
	row := checks["doctor.rea.platform.antigravity-cli"]
	assert.Equal(t, jsonCheck{ID: row.ID, Severity: "warning", Status: "warn", Detail: "user_owned"}, row)
	assert.Equal(t, "warn", status)
	raw, _ := runReaCLI(t, "", "doctor", "--dir", dir, "--json")
	assert.Contains(t, raw, `"rea_target_not_strict_json"`)
	text, _ := runReaCLI(t, "", "doctor", "--dir", dir)
	assert.Contains(t, text, fmt.Sprintf(reaTextKo.PluginNotRemoved, reaPluginMCPRel))

	writeReaTarget(t, dir, reamcp.Antigravity, reaPluginJSONC(false))
	_, checks, _ = reaDoctorJSON(t, dir)
	assert.Equal(t, "info", checks["doctor.rea.platform.antigravity-cli"].Severity, "off with no rea entry left")
	cfg.Features.Rea.Enabled = reaBool(true)
	require.NoError(t, config.Save(dir, cfg))
	_, checks, _ = reaDoctorJSON(t, dir)
	assert.Equal(t, "warning", checks["doctor.rea.platform.antigravity-cli"].Severity, "on cannot register")
	writeReaTarget(t, dir, reamcp.Antigravity, reaPluginJSONC(true))
	_, checks, _ = reaDoctorJSON(t, dir)
	assert.Equal(t, "info", checks["doctor.rea.platform.antigravity-cli"].Severity, "on over the pinned entry agy starts")
}

// Reviewer L1: the likeliest non-strict file is the one Autopus wrote with a
// user comment added. agy starts its pinned rea entry, so --rea and a plain
// update say nothing about it and leave its bytes.
func TestReaPluginMCP_PinnedEntryInANotStrictFileIsQuiet(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	stubDriverProbes(t, false, "")
	dir := reaOLD(t, reaAllTargets)
	withRea := reaPluginJSONC(true)
	writeReaTarget(t, dir, reamcp.Antigravity, withRea)
	notRegistered := fmt.Sprintf(reaTextKo.PluginNotRegistered, reaPluginMCPRel)
	for _, args := range [][]string{{"--yes", "--rea"}, {"--yes"}} {
		out, err := reaUpdate(t, strings.NewReader(""), dir, args...)
		require.NoError(t, err, out)
		assert.NotContains(t, out, notRegistered, args)
		assert.Equal(t, withRea, readReaFile(t, dir, reamcp.Antigravity), args)
	}
}

// The preview lists the skip instead of a change.
func TestReaPluginMCP_PreviewListsTheSkip(t *testing.T) {
	t.Parallel()
	dir, cfg := reaFixture(t, "antigravity-cli")
	for action, withRea := range map[config.ReaMCPAction]bool{config.ReaMCPAdd: false, config.ReaMCPRemove: true} {
		writeReaTarget(t, dir, reamcp.Antigravity, reaPluginJSONC(withRea))
		cfg.Runtime.ReaMCP = action
		items := reaPluginPreviewItems(dir, cfg)
		require.Len(t, items, 1, action)
		assert.Equal(t, "skip", items[0].Kind, action)
		assert.Contains(t, items[0].Reason, "not strict JSON", action)
	}
	writeReaTarget(t, dir, reamcp.Antigravity, reaPluginJSONC(false))
	cfg.Runtime.ReaMCP = config.ReaMCPRemove
	assert.Empty(t, reaPluginPreviewItems(dir, cfg), "nothing to remove")
	writeReaTarget(t, dir, reamcp.Antigravity, reaPluginJSONC(true))
	cfg.Runtime.ReaMCP = config.ReaMCPAdd
	assert.Empty(t, reaPluginPreviewItems(dir, cfg), "the pinned entry is already there")
	cfg.Runtime.ReaMCP = config.ReaMCPRemove
	cfg.Platforms = []string{"claude-code"}
	writeReaTarget(t, dir, reamcp.Antigravity, reaPluginJSONC(true))
	sweep := reaSweepPreviewItems(dir, cfg)
	require.Len(t, sweep, 1)
	assert.Equal(t, "skip", sweep[0].Kind)
}

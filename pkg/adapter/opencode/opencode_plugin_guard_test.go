package opencode

// SPEC-EDITGUARD-001 T10 and S9: both generated OpenCode plugins send every
// file-editing call of the A2 host-native events through `auto guard edit`
// with the synthesized payload, throw only for a deny from a guard that exited
// 0, and keep the shell-tool hooks to shell calls.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

var pluginVersions = []string{"v1", "v2"}

// The recording stub receives exactly the payloads the T0 spike plugin
// synthesized from the same A2 events, in event order, with the displaced
// paths of the move the spike did not list, and the shell-tool hooks run for
// the two shell calls only.
func TestOpenCodeGuardPlugin_SendsTheSynthesizedPayloadOfEveryA2Event(t *testing.T) {
	t.Parallel()
	for _, version := range pluginVersions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			h := newGuardHarness(t)
			results := h.run(renderGuardPlugin(t, version, 0), version, autoStub, h.a2Calls(version))
			for i, result := range results {
				assert.Equal(t, "resolved", result.Outcome, "call %d: %+v", i+1, result)
			}
			raw, err := os.ReadFile(filepath.Join("..", "..", "editguard", "testdata", "hostpayload",
				"opencode-2.0.10-synthesized.jsonl"))
			require.NoError(t, err)
			want := strings.Split(strings.TrimSpace(h.withRoot(string(raw))), "\n")
			assert.Equal(t, want, h.stubLog("stdin.log"))
			assert.Equal(t, strings.Split(strings.Repeat("guard edit --platform opencode\n", len(want)-1)+
				"guard edit --platform opencode", "\n"), h.stubLog("args.log"))
			log, err := os.ReadFile(filepath.Join(h.root, "hook.log"))
			require.NoError(t, err)
			assert.Equal(t, "beforeafterbeforeafter", string(log), "shell-tool hooks run for shell calls only")
		})
	}
}

// With the real guard, the protected edit, write, second patch target, and
// move destination reject with the GS-CON reason, and the rest resolves.
func TestOpenCodeGuardPlugin_RealGuardDeniesProtectedTargets(t *testing.T) {
	t.Parallel()
	for _, version := range pluginVersions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			h := newGuardHarness(t)
			results := h.run(renderGuardPlugin(t, version, 0), version, autoReal, h.a2Calls(version))
			// edit protected, write protected, write pkg/foo.go, shell, two-file
			// patch, move onto protected, patch pkg/a.go only, shell.
			want := []string{guardGSCon, guardGSCon, "", "", guardGSCon, guardGSCon, "", ""}
			for i, result := range results {
				if want[i] == "" {
					assert.Equal(t, "resolved", result.Outcome, "call %d: %+v", i+1, result)
					continue
				}
				assert.Equal(t, pluginResult{Outcome: "rejected", Message: want[i]}, pluginResult{
					Outcome: result.Outcome, Message: result.Message}, "call %d", i+1)
			}
		})
	}
}

// S9 (a) to (e): only a deny printed by a guard that exited 0 throws, with its
// reason as the message; a deny before exit 2, silence, a hung guard, output
// that is no deny decision, and a missing guard all resolve.
func TestOpenCodeGuardPlugin_FailsOpenUnlessACleanExitDenies(t *testing.T) {
	t.Parallel()
	for _, version := range pluginVersions {
		t.Run(version, func(t *testing.T) {
			t.Parallel()
			h := newGuardHarness(t)
			protected := h.a2Calls(version)[0]
			var calls []pluginCall
			modes := []string{"deny0", "deny2", "", "text", "noreason"}
			for _, mode := range modes {
				calls = append(calls, pluginCall{Event: protected.Event, Mode: modeOf(mode)})
			}
			// The generated 5 second timeout leaves room for the stub's first
			// exec on a loaded macOS runner.
			results := h.run(renderGuardPlugin(t, version, 0), version, autoStub, calls)
			assert.Equal(t, pluginResult{Outcome: "rejected", Message: "R1"},
				pluginResult{Outcome: results[0].Outcome, Message: results[0].Message}, "deny on exit 0: %+v", results[0])
			for i, mode := range modes[1:] {
				assert.Equal(t, "resolved", results[i+1].Outcome, "mode %q: %+v", mode, results[i+1])
			}

			hung := h.run(renderGuardPlugin(t, version, 1), version, autoStub,
				[]pluginCall{{Event: protected.Event, Mode: modeOf("sleep")}})
			assert.Equal(t, "resolved", hung[0].Outcome, "a hung guard: %+v", hung[0])
			assert.Less(t, hung[0].MS, 4000, "a hung guard is cut off at the hook timeout")

			absent := h.run(renderGuardPlugin(t, version, 0), version, autoAbsent, []pluginCall{protected})
			assert.Equal(t, "resolved", absent[0].Outcome, "a missing auto: %+v", absent[0])
		})
	}
}

// The patch targets reach the guard at the header positions the OpenCode
// 2.0.10 patch parser reads (its function bq in the 2.0.10 binary): headers
// are trimmed and padded headers count where the parser trims, inner padding
// is trimmed, an Update hunk ends only at an unpadded header, and a Move to
// counts right below its Update header, after End of File lines. A Delete
// File and both ends of a Move to are also listed as displaced.
func TestOpenCodeGuardPlugin_PatchTargetsFollowTheOpenCodeParser(t *testing.T) {
	t.Parallel()
	patch := func(body ...string) string {
		return "*** Begin Patch\n" + strings.Join(body, "\n") + "\n*** End Patch"
	}
	edit := func(head ...string) string { return patch(append(head, "@@", "-old", "+new")...) }
	cases := []struct {
		tool, patch     string
		want, displaced []string
	}{
		{"patch", edit("  *** Update File: a.txt"), []string{"a.txt"}, nil},
		{"patch", edit("*** Add File: c.txt", "+hi", "  *** Update File: a.txt"), []string{"c.txt", "a.txt"}, nil},
		{"patch", edit("*** Delete File: b.txt", "\t*** Update File: a.txt"), []string{"b.txt", "a.txt"}, []string{"b.txt"}},
		{"patch", patch("*** Add File:  f.txt ", "+hi"), []string{"f.txt"}, nil},
		{"patch", edit("*** Update File: a.txt", "*** Move to:  h.txt \t"), []string{"a.txt", "h.txt"}, []string{"a.txt", "h.txt"}},
		{"patch", edit("*** Update File: a.txt", "*** End of File", "*** Move to: d.txt"), []string{"a.txt", "d.txt"},
			[]string{"a.txt", "d.txt"}},
		{"patch", edit("*** Update File: a.txt", " *** Move to: e.txt"), []string{"a.txt"}, nil},
		{"patch", edit("*** Update File: a.txt", "*** Move to:"), []string{"a.txt"}, []string{"a.txt"}},
		{"patch", patch("*** Add File: c.txt", "+*** Update File: a.txt"), []string{"c.txt"}, nil},
		{"patch", edit("*** Update File: b.txt", "@@", " *** Update File: a.txt", "-keep"), []string{"b.txt"}, nil},
		{"patch", edit("*** update file: a.txt"), nil, nil},
		{"patch", strings.ReplaceAll(edit("*** Update File: a.txt"), "\n", "\r\n"), []string{"a.txt"}, nil},
		{"patch", "<<'EOF'\n" + patch("*** Add File: x.txt", "+x") + "\nEOF", []string{"x.txt"}, nil},
	}
	h := newGuardHarness(t)
	var calls []pluginCall
	for i, c := range cases {
		calls = append(calls, pluginCall{Event: map[string]any{"tool": c.tool, "sessionID": "ses", "id": i,
			"input": map[string]any{"patchText": c.patch}}})
	}
	h.run(renderGuardPlugin(t, "v2", 0), "v2", autoStub, calls)
	payloads := h.stubLog("stdin.log")
	require.Len(t, payloads, len(cases))
	for i, c := range cases {
		assert.Equal(t, synthesized(h.root, c.tool, c.want, c.displaced...), payloads[i], "case %d: %q", i+1, c.patch)
	}
}

// The V1-only spellings (host-unverified, CD-1): apply_patch carries a patch
// and multiedit names filePath at the top and in each edit.
func TestOpenCodeGuardPlugin_V1Spellings(t *testing.T) {
	t.Parallel()
	h := newGuardHarness(t)
	calls := []pluginCall{
		{Event: map[string]any{"tool": "apply_patch", "sessionID": "ses", "id": "1",
			"input": map[string]any{"patchText": "*** Begin Patch\n*** Add File: x.txt\n+x\n*** End Patch"}}},
		{Event: map[string]any{"tool": "multiedit", "sessionID": "ses", "id": "2",
			"input": map[string]any{"filePath": "pkg/a.go", "edits": []any{map[string]any{"filePath": guardSkill}}}}},
		{Event: map[string]any{"tool": "read", "sessionID": "ses", "id": "3",
			"input": map[string]any{"filePath": guardSkill}}},
	}
	h.run(renderGuardPlugin(t, "v1", 0), "v1", autoStub, calls)
	assert.Equal(t, []string{
		synthesized(h.root, "apply_patch", []string{"x.txt"}),
		synthesized(h.root, "multiedit", []string{"pkg/a.go", guardSkill}),
	}, h.stubLog("stdin.log"), "a read runs no guard")
}

// synthesized is the payload of the Decision Output Contract as the plugin
// writes it; displaced appears only when a patch displaces a path.
func synthesized(root, tool string, targets []string, displaced ...string) string {
	quote := func(paths []string) string {
		quoted := make([]string, len(paths))
		for i, p := range paths {
			quoted[i] = `"` + p + `"`
		}
		return "[" + strings.Join(quoted, ",") + "]"
	}
	payload := `{"platform":"opencode","cwd":"` + root + `","tool_name":"` + tool + `","targets":` + quote(targets)
	if len(displaced) > 0 {
		payload += `,"displaced":` + quote(displaced)
	}
	return payload + "}"
}

// The generated plugin of each version carries the guard of the enforced
// OpenCode lane with that version's tools, and edit_guard: false renders null.
func TestOpenCodePluginMapping_CarriesTheVersionGuard(t *testing.T) {
	t.Parallel()
	v1 := `const EDIT_GUARD = {"command":"auto","args":["guard","edit","--platform","opencode"],"timeout":5,` +
		`"tools":["edit","write","multiedit","patch","apply_patch"],"patchTools":["patch","apply_patch"]}`
	v2 := `const EDIT_GUARD = {"command":"auto","args":["guard","edit","--platform","opencode"],"timeout":5,` +
		`"tools":["edit","write","patch"],"patchTools":["patch"]}`
	for cliVersion, want := range map[string]string{"1.18.7": v1, "2.0.10": v2} {
		cfg := config.DefaultFullConfig("guard")
		a := NewWithRoot(t.TempDir(), WithCLIVersion(cliVersion))
		mappings, err := a.preparePluginMappings(cfg)
		require.NoError(t, err)
		require.Len(t, mappings, 1)
		plugin := string(mappings[0].Content)
		assert.Contains(t, plugin, want+"\n", cliVersion)
		assert.NotContains(t, plugin, "auto guard edit --platform", "%s: the guard is no shell-tool hook", cliVersion)

		cfg.Hooks.EditGuard = new(false)
		mappings, err = a.preparePluginMappings(cfg)
		require.NoError(t, err)
		assert.Contains(t, string(mappings[0].Content), "const EDIT_GUARD = null\n", cliVersion)
	}
	_, err := NewWithRoot(t.TempDir(), WithCLIVersion("2.0.10")).Generate(context.Background(), config.DefaultFullConfig("x"))
	require.NoError(t, err)
}

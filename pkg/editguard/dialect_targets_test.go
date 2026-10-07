package editguard

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// openCodeDeny is the exact OpenCode deny of the Decision Output Contract.
func openCodeDeny(reason string) string {
	return `{"decision":"deny","reason":"` + reason + `"}`
}

func openCodePayload(cwd string, targets ...any) string {
	data, _ := json.Marshal(map[string]any{"platform": "opencode", "cwd": cwd, "tool_name": "patch", "targets": targets})
	return string(data)
}

// S9: the payloads the OpenCode 2.0.10 spike plugin synthesized in probe A2
// deny a protected first, second, or move-destination target and allow the
// rest.
func TestOpenCodeDialect_SynthesizedProbePayloads(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformOpenCode)
	lines := strings.Split(strings.TrimSpace(hostPayload(t, "opencode-2.0.10-synthesized.jsonl", root)), "\n")
	deny := openCodeDeny(gsConReason)
	// edit, write protected, write pkg/foo.go, two-file patch, move onto
	// protected, patch pkg/a.go only.
	want := []string{deny, deny, "", deny, deny, ""}
	if len(lines) != len(want) {
		t.Fatalf("fixture has %d payloads, want %d", len(lines), len(want))
	}
	for i, line := range lines {
		got := runGuard(strings.NewReader(line), dialect, Options{})
		if got.code != 0 || got.stdout != want[i] || got.stderr != "" || want[i] != "" && got.stdoutWrites != 1 {
			t.Errorf("payload %d %s: got %+v, want stdout %q", i+1, line, got, want[i])
		}
	}
}

// S16: a malformed target is dropped and every other target still decides;
// the payload's own platform field never picks the encoding.
func TestOpenCodeDialect_MalformedTargetsAreDropped(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformOpenCode)
	deny := openCodeDeny(gsConReason)
	rows := []struct {
		payload, want string
	}{
		{openCodePayload(root, skillRel, 42), deny},
		{openCodePayload(root, 42, skillRel), deny},
		{openCodePayload(root, nil, "", map[string]any{}, skillRel), deny},
		{strings.Replace(openCodePayload(root, skillRel), `"platform":"opencode"`, `"platform":"claude-code"`, 1), deny},
		{openCodePayload(root, 42), ""},
		{openCodePayload(root), ""},
		{`{"cwd":"` + root + `","targets":"` + skillRel + `"}`, ""},
	}
	for _, row := range rows {
		got := runGuard(strings.NewReader(row.payload), dialect, Options{})
		if row.want == "" {
			got.allowed(t, row.payload)
			continue
		}
		if got.code != 0 || got.stdout != row.want || got.stdoutWrites != 1 || got.stderr != "" {
			t.Errorf("%s: got %+v", row.payload, got)
		}
	}
	call, err := dialect.Decode([]byte(openCodePayload(root, 42, skillRel, "")))
	if err != nil || call.Cwd != root || call.Dropped != 2 || !slices.Equal(call.Targets, []string{skillRel}) {
		t.Fatalf("Decode = %+v, %v", call, err)
	}
}

func codexPayload(cwd, tool string, input map[string]any) string {
	data, _ := json.Marshal(map[string]any{"cwd": cwd, "hook_event_name": "PreToolUse", "tool_name": tool,
		"tool_input": input})
	return string(data)
}

// A3: the apply_patch payload Codex 0.160.0 sent names every target, the move
// destination included, and a protected one gets the PreToolUse deny.
func TestCodexDialect_HostApplyPatchPayload(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformCodex)
	payload := hostPayload(t, "codex-0.160.0-apply-patch.json", root)
	call, err := dialect.Decode([]byte(payload))
	if err != nil || call.Cwd != root || !slices.Equal(call.Targets, []string{"notes.txt", "old.txt", "moved/new.txt"}) {
		t.Fatalf("Decode = %+v, %v", call, err)
	}
	if got := runGuard(strings.NewReader(payload), dialect, Options{}); got.code != 0 || got.stdout != "" || got.stderr != "" {
		t.Errorf("unprotected patch: %+v", got)
	}
	protected := strings.Replace(payload, "moved/new.txt", skillRel, 1)
	got := runGuard(strings.NewReader(protected), dialect, Options{})
	if got.code != 0 || got.stdout != claudeDeny(gsConReason) || got.stdoutWrites != 1 || got.stderr != "" {
		t.Errorf("move onto a generated file: %+v", got)
	}
}

// Every Codex tool other than apply_patch is a shell path the guard does not
// cover, and a patch that is not a string has no target.
func TestCodexDialect_OtherToolsHaveNoTarget(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformCodex)
	for _, payload := range []string{
		codexPayload(root, "exec_command", map[string]any{"cmd": "*** Update File: " + skillRel}),
		codexPayload(root, "Edit", map[string]any{"file_path": skillRel}),
		codexPayload(root, "apply_patch", map[string]any{"command": 42}),
		codexPayload(root, "apply_patch", nil),
	} {
		call, err := dialect.Decode([]byte(payload))
		if err != nil || len(call.Targets) != 0 {
			t.Errorf("Decode(%s) = %+v, %v", payload, call, err)
		}
		runGuard(strings.NewReader(payload), dialect, Options{}).allowed(t, payload)
	}
	if _, err := dialect.Decode([]byte(`{"tool_name":"apply_patch","tool_input":"*** Begin Patch"}`)); err == nil {
		t.Error("a tool_input that is not an object must be a decode error")
	}
}

// The header positions of the apply_patch grammar. Each want is what the real
// Codex 0.160.0 parser (`codex --codex-run-as-apply-patch`) read from the same
// patch in a differential run: padded headers count after Begin Patch, an Add
// body, or a Delete; inside an Update hunk a padded line is context; a Move to
// line counts only unpadded right below its Update header.
func TestCodexDialect_PatchTargetsFollowTheApplyPatchGrammar(t *testing.T) {
	t.Parallel()
	dialect := dialectOf(t, PlatformCodex)
	patch := func(body ...string) string {
		return "*** Begin Patch\n" + strings.Join(body, "\n") + "\n*** End Patch\n"
	}
	edit := func(head ...string) string { return patch(append(head, "@@", "-old", "+new")...) }
	cases := []struct {
		name  string
		patch string
		want  []string
	}{
		{"padded first header", edit("  *** Update File: a.txt"), []string{"a.txt"}},
		{"padded header after an add body", edit("*** Add File: c.txt", "+hi", "  *** Update File: a.txt"),
			[]string{"c.txt", "a.txt"}},
		{"tab-padded header after an add body", edit("*** Add File: g.txt", "+hi", "\t*** Update File: a.txt"),
			[]string{"g.txt", "a.txt"}},
		{"padded header after a delete", edit("*** Delete File: b.txt", "   *** Update File: a.txt"),
			[]string{"b.txt", "a.txt"}},
		{"move destination right-trimmed", edit("*** Update File: a.txt", "*** Move to: d.txt \t"),
			[]string{"a.txt", "d.txt"}},
		{"move keeps inner padding", edit("*** Update File: a.txt", "*** Move to:  h.txt"),
			[]string{"a.txt", " h.txt"}},
		{"padded move line is context", edit("*** Update File: a.txt", " *** Move to: e.txt"), []string{"a.txt"}},
		{"add keeps inner padding", patch("*** Add File:  f.txt", "+hi"), []string{" f.txt"}},
		{"add header right-trimmed", patch("*** Add File: l.txt  ", "+hi"), []string{"l.txt"}},
		{"added line naming a file", patch("*** Add File: c.txt", "+*** Update File: a.txt"), []string{"c.txt"}},
		{"context line naming a file", patch("*** Update File: a.txt", "@@", " *** Update File: b.txt", "-old"),
			[]string{"a.txt"}},
		{"padded header after an update is context", edit("*** Update File: b.txt", "@@", "-keep", "+kept",
			" *** Update File: a.txt"), []string{"b.txt"}},
		{"end of file marker", patch("*** Update File: a.txt", "@@", "-old", "+new", "*** End of File",
			"*** Update File: b.txt", "@@", "-keep", "+kept"), []string{"a.txt", "b.txt"}},
		{"markers are case sensitive", edit("*** update file: a.txt"), nil},
		// 0.160.0 rejects a CRLF patch whole; reading it as LF stays a superset.
		{"crlf line ends", strings.ReplaceAll(edit("*** Update File: a.txt"), "\n", "\r\n"), []string{"a.txt"}},
	}
	for _, c := range cases {
		call, err := dialect.Decode([]byte(codexPayload("/w", "apply_patch", map[string]any{"command": c.patch})))
		if err != nil || !slices.Equal(call.Targets, c.want) {
			t.Errorf("%s: targets = %q, %v; want %q", c.name, call.Targets, err, c.want)
		}
	}
	empty := codexPayload("/w", "apply_patch", map[string]any{"command": edit("*** Update File: a.txt", "*** Move to: ")})
	if call, err := dialect.Decode([]byte(empty)); err != nil || call.Dropped != 1 || !slices.Equal(call.Targets, []string{"a.txt"}) {
		t.Errorf("empty move destination: %+v, %v", call, err)
	}
}

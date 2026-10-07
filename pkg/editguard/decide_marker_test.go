package editguard

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// A call that deletes a project root's autopus.yaml, moves it away, or moves
// a file onto it would leave that tree without the root whose locks and
// manifests protect it, so the guard denies it as guard state. Editing the
// marker in place keeps the root and stays allowed.
func TestDecide_DisplacingARootMarker_IsGuardState(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	writeFile(t, root, "nested/"+projectMarker, "x")
	writeFile(t, root, "notes.txt", "x")
	symlink(t, filepath.Join(root, "nested"), filepath.Join(root, "nested-alias"))
	displaced := func(paths ...string) Call {
		return Call{Cwd: root, Targets: paths, Displaced: paths}
	}
	cases := map[string]struct {
		call Call
		want Decision
	}{
		"delete":              {displaced(projectMarker), denyOf(ClassGuardState, gstReason(projectMarker))},
		"absolute":            {displaced(filepath.Join(root, projectMarker)), denyOf(ClassGuardState, gstReason(projectMarker))},
		"nested root":         {displaced("nested/" + projectMarker), denyOf(ClassGuardState, gstReason("nested/"+projectMarker))},
		"through a link":      {displaced("nested-alias/" + projectMarker), denyOf(ClassGuardState, gstReason("nested/"+projectMarker))},
		"lexical dot-dot":     {displaced("nested-alias/../" + projectMarker), denyOf(ClassGuardState, gstReason(projectMarker))},
		"move source":         {displaced(projectMarker, "config.yaml"), denyOf(ClassGuardState, gstReason(projectMarker))},
		"move destination":    {displaced("notes.txt", projectMarker), denyOf(ClassGuardState, gstReason(projectMarker))},
		"edit in place":       {Call{Cwd: root, Targets: []string{projectMarker}}, Decision{}},
		"delete another file": {displaced("notes.txt"), Decision{}},
		"not a root":          {displaced("pkg/" + projectMarker), Decision{}},
	}
	for name, tc := range cases {
		if got := Decide(tc.call, Options{}); got != tc.want {
			t.Errorf("%s: Decide(%+v) = %+v, want %+v", name, tc.call, got, tc.want)
		}
	}
	if volumeFoldsCase(t, root) {
		if got := Decide(displaced("AUTOPUS.YAML"), Options{}); got != denyOf(ClassGuardState, gstReason("AUTOPUS.YAML")) {
			t.Errorf("case variant = %+v, want the GST deny", got)
		}
	}
}

// Codex apply_patch: a Delete File, and either end of an Update with Move to,
// displace their paths; an Add or an Update in place does not.
func TestCodexDialect_PatchDisplacements(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformCodex)
	patch := func(body ...string) string {
		return codexPayload(root, "apply_patch", map[string]any{"command": "*** Begin Patch\n" +
			strings.Join(body, "\n") + "\n*** End Patch\n"})
	}
	gst := claudeDeny(gstReason(projectMarker))
	for _, tc := range []struct {
		payload   string
		displaced []string
		stdout    string
	}{
		{patch("*** Delete File: " + projectMarker), []string{projectMarker}, gst},
		{patch("*** Update File: "+projectMarker, "*** Move to: config.yaml", "@@", "-a", "+b"),
			[]string{projectMarker, "config.yaml"}, gst},
		{patch("*** Update File: notes.txt", "*** Move to: "+projectMarker, "@@", "-a", "+b"),
			[]string{"notes.txt", projectMarker}, gst},
		{patch("*** Update File: "+projectMarker, "@@", "-a", "+b", "*** Add File: x.txt", "+x"), nil, ""},
		{patch("*** Add File: "+projectMarker, "+project: {}"), nil, ""},
	} {
		call, err := dialect.Decode([]byte(tc.payload))
		if err != nil || !slices.Equal(call.Displaced, tc.displaced) {
			t.Errorf("Decode(%s) displaced = %q, %v; want %q", tc.payload, call.Displaced, err, tc.displaced)
		}
		if got := runGuard(strings.NewReader(tc.payload), dialect, Options{}); got.stdout != tc.stdout {
			t.Errorf("guard(%s) stdout = %q, want %q", tc.payload, got.stdout, tc.stdout)
		}
	}
}

// OpenCode: the plugin lists a patch's displaced paths in "displaced"; a
// non-string entry is skipped.
func TestOpenCodeDialect_DisplacedPaths(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	dialect := dialectOf(t, PlatformOpenCode)
	payload := `{"platform":"opencode","cwd":"` + root + `","tool_name":"patch","targets":["` + projectMarker +
		`"],"displaced":[42,"` + projectMarker + `"]}`
	call, err := dialect.Decode([]byte(payload))
	if err != nil || !slices.Equal(call.Displaced, []string{projectMarker}) {
		t.Fatalf("Decode = %+v, %v", call, err)
	}
	if got := runGuard(strings.NewReader(payload), dialect, Options{}); got.stdout != openCodeDeny(gstReason(projectMarker)) {
		t.Errorf("guard = %+v, want the GST deny", got)
	}
	inPlace := strings.Replace(payload, `,"displaced":[42,"`+projectMarker+`"]`, "", 1)
	runGuard(strings.NewReader(inPlace), dialect, Options{}).allowed(t, "an edit in place")
}

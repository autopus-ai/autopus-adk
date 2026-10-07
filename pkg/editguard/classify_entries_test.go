package editguard

import (
	"os"
	"path/filepath"
	"testing"
)

// H2: an entry named like a manifest that is not a regular file is no
// manifest the generator wrote. It is skipped instead of dropping the whole
// manifest stage, which would leave every generated file of the root open.
func TestManifestStage_NonRegularEntryNamedLikeAManifest_IsSkipped(t *testing.T) {
	t.Parallel()
	plants := map[string]func(t *testing.T, name string){
		"directory": func(t *testing.T, name string) {
			if err := os.Mkdir(name, 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"symlink to a directory": func(t *testing.T, name string) {
			symlink(t, filepath.Dir(name), name)
		},
	}
	for kind, plant := range plants {
		root := fixtureR(t)
		plant(t, filepath.Join(root, ".autopus", "zzz-manifest.json"))
		stage := loadManifestStage(root, false)
		if stage.fault != "" {
			t.Errorf("%s: fault = %q, want the entry skipped", kind, stage.fault)
		}
		if _, ok := stage.generated(skillRel); !ok {
			t.Errorf("%s: the generated skill is no longer protected", kind)
		}
		if got := decideOne(root, skillRel, Options{}); got != denyOf(ClassGeneratedSurface, gsConReason) {
			t.Errorf("%s: Decide(%s) = %+v, want the GS-CON deny", kind, skillRel, got)
		}
	}
}

// H2: everything below `.autopus/<x>-manifest.json` is guard state, so a
// file-editing tool cannot create the directory that would shadow a manifest.
func TestDecide_PathsBelowAManifestName_AreGuardState(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	for _, raw := range []string{".autopus/zzz-manifest.json/x", ".autopus/claude-code-manifest.json/a/b.md"} {
		if got := decideOne(root, raw, Options{}); got != denyOf(ClassGuardState, gstReason(raw)) {
			t.Errorf("Decide(%q) = %+v, want the GST deny", raw, got)
		}
	}
	for _, raw := range []string{".autopus/nested/x-manifest.json", ".autopus/manifests/x.json", ".autopus/x-manifest.jsonl"} {
		if got := decideOne(root, raw, Options{}); got != (Decision{}) {
			t.Errorf("Decide(%q) = %+v, want an allow", raw, got)
		}
	}
}

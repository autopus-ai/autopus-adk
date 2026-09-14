package content

import (
	"reflect"
	"testing"
)

// TestSkillResourceRelPaths_MatchesRenderedKeys guards the cheap path listing
// against drifting from the expensive render: an ownership allowlist built from
// the paths must cover exactly what RenderSkillResources writes.
func TestSkillResourceRelPaths_MatchesRenderedKeys(t *testing.T) {
	rendered, err := RenderSkillResources("agent-pipeline", "claude", cfgDefault("claude"))
	if err != nil {
		t.Fatalf("RenderSkillResources: %v", err)
	}
	if len(rendered) == 0 {
		t.Fatal("precondition failed: agent-pipeline ships no reference bodies")
	}

	paths := SkillResourceRelPaths("agent-pipeline")
	if !reflect.DeepEqual(paths, SkillResourceNames(rendered)) {
		t.Errorf("SkillResourceRelPaths = %v, want the rendered keys %v", paths, SkillResourceNames(rendered))
	}
	for _, rel := range paths {
		if got := rel[:len(SkillResourceDirName)]; got != SkillResourceDirName {
			t.Errorf("resource path %q is not under %q", rel, SkillResourceDirName)
		}
	}
}

// TestSkillResourceNames_IsSortedAndComplete pins deterministic ordering,
// which is what makes generated file mappings and checksums stable.
func TestSkillResourceNames_IsSortedAndComplete(t *testing.T) {
	resources := map[string][]byte{
		"references/zeta.md":  []byte("z"),
		"references/alpha.md": []byte("a"),
		"references/mid.md":   []byte("m"),
	}
	want := []string{"references/alpha.md", "references/mid.md", "references/zeta.md"}
	if got := SkillResourceNames(resources); !reflect.DeepEqual(got, want) {
		t.Errorf("SkillResourceNames = %v, want %v", got, want)
	}
	if got := SkillResourceNames(nil); len(got) != 0 {
		t.Errorf("SkillResourceNames(nil) = %v, want empty", got)
	}
}

// TestSkillResourceRelPaths_RejectsNonSegmentNames keeps a malformed catalog
// entry from listing paths outside the embedded resource tree.
func TestSkillResourceRelPaths_RejectsNonSegmentNames(t *testing.T) {
	for _, name := range []string{"", "..", "../skills", "a/b", `a\b`, "no-such-skill"} {
		if got := SkillResourceRelPaths(name); got != nil {
			t.Errorf("SkillResourceRelPaths(%q) = %v, want nil", name, got)
		}
	}
}

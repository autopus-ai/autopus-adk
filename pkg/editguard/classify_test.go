package editguard

import (
	"encoding/json"
	"strings"
	"testing"
)

type manifestFiles map[string]string

func writeManifest(t *testing.T, root, platform string, files manifestFiles) {
	t.Helper()
	doc := map[string]any{"version": "1.0.0", "platform": platform, "files": map[string]any{}}
	for path, policy := range files {
		doc["files"].(map[string]any)[path] = map[string]string{"checksum": "x", "policy": policy}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ".autopus/"+platform+"-manifest.json", string(data))
}

// fixtureR builds the acceptance fixture R: forged and overriding entries next
// to one real generated file.
func fixtureR(t *testing.T) string {
	t.Helper()
	root := newProject(t)
	writeManifest(t, root, "claude-code", manifestFiles{
		skillRel:                         "always",
		".claude/settings.json":          "merge",
		"CLAUDE.md":                      "marker",
		"pkg/main.go":                    "always",
		".autopus/brainstorms/BS-001.md": "always",
		".agents/skills/x/SKILL.md":      "always",
		".git/hooks/pre-commit":          "always",
	})
	writeManifest(t, root, "opencode", manifestFiles{".agents/skills/x/SKILL.md": "merge"})
	for _, rel := range []string{skillRel, ".claude/settings.json", "CLAUDE.md", "pkg/main.go",
		".autopus/brainstorms/BS-001.md", ".agents/skills/x/SKILL.md"} {
		writeFile(t, root, rel, "x")
	}
	return root
}

// S1: only an always entry inside the namespace with no merge or marker
// listing is generated.
func TestManifestStage_FixtureR_OnlyUnoverriddenNamespaceAlwaysEntries(t *testing.T) {
	t.Parallel()
	stage := loadManifestStage(fixtureR(t), false)
	if stage.fault != "" {
		t.Fatalf("fault = %q", stage.fault)
	}
	hit, ok := stage.generated(skillRel)
	if !ok || hit.display != skillRel || hit.manifest != ".autopus/claude-code-manifest.json" {
		t.Fatalf("generated(%s) = %+v, %v", skillRel, hit, ok)
	}
	for _, key := range []string{".claude/settings.json", "CLAUDE.md", "pkg/main.go",
		".autopus/brainstorms/BS-001.md", ".agents/skills/x/SKILL.md", ".git/hooks/pre-commit",
		".claude/commands/my-cmd.md", "config.toml"} {
		if _, ok := stage.generated(key); ok {
			t.Errorf("generated(%q) = true, want false", key)
		}
	}
}

func TestManifestStage_FirstAlwaysManifestInLexicalOrderIsNamed(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeManifest(t, root, "codex", manifestFiles{".codex/skills/a.md": "always"})
	writeManifest(t, root, "antigravity-cli", manifestFiles{".codex/skills/a.md": "always"})
	writeFile(t, root, ".autopus/claude-code-permissions.json", "not a manifest")
	hit, ok := loadManifestStage(root, false).generated(".codex/skills/a.md")
	if !ok || hit.manifest != ".autopus/antigravity-cli-manifest.json" {
		t.Fatalf("hit = %+v, %v", hit, ok)
	}
}

func TestManifestStage_OverrideInEitherOrderWins(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeManifest(t, root, "a", manifestFiles{".claude/x.md": "merge", ".claude/y.md": "always"})
	writeManifest(t, root, "b", manifestFiles{".claude/x.md": "always", ".claude/y.md": "marker"})
	stage := loadManifestStage(root, false)
	for _, key := range []string{".claude/x.md", ".claude/y.md"} {
		if _, ok := stage.generated(key); ok {
			t.Errorf("generated(%q) = true, want the override to win", key)
		}
	}
}

// S16 / REQ-EG-18: any unreadable or corrupt manifest skips the whole stage.
// An entry that is not a regular file is no manifest at all (H2,
// TestManifestStage_NonRegularEntryNamedLikeAManifest_IsSkipped).
func TestManifestStage_FaultyManifest_SkipsTheStage(t *testing.T) {
	t.Parallel()
	faults := map[string]func(root string){
		"truncated json": func(root string) { writeFile(t, root, ".autopus/opencode-manifest.json", "{") },
		"files not an object": func(root string) {
			writeFile(t, root, ".autopus/opencode-manifest.json", `{"files":42}`)
		},
		"policy not a string": func(root string) {
			writeFile(t, root, ".autopus/opencode-manifest.json", `{"files":{".claude/a":{"policy":7}}}`)
		},
		"oversized": func(root string) {
			writeFile(t, root, ".autopus/opencode-manifest.json", strings.Repeat(" ", maxManifestBytes+1))
		},
	}
	for name, plant := range faults {
		root := fixtureR(t)
		plant(root)
		stage := loadManifestStage(root, false)
		if stage.fault != ".autopus/opencode-manifest.json" {
			t.Errorf("%s: fault = %q", name, stage.fault)
		}
		if _, ok := stage.generated(skillRel); ok {
			t.Errorf("%s: a faulted stage reported a generated file", name)
		}
	}
}

func TestManifestStage_NoManifestDirectory_IsEmptyWithoutFault(t *testing.T) {
	t.Parallel()
	stage := loadManifestStage(newProject(t), false)
	if stage.fault != "" {
		t.Fatalf("fault = %q", stage.fault)
	}
	if _, ok := stage.generated(skillRel); ok {
		t.Fatal("no manifest means nothing is generated")
	}
}

func TestManifestStage_UnreadableManifestDirectory_Faults(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeFile(t, root, ".autopus", "a file where the directory belongs")
	if stage := loadManifestStage(root, false); stage.fault != ".autopus" {
		t.Fatalf("fault = %q", stage.fault)
	}
}

func TestManifestStage_FoldedKeysMatchCaseVariantsAndKeepManifestSpelling(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeManifest(t, root, "claude-code", manifestFiles{skillRel: "always", "./.claude//Other.md": "always"})
	stage := loadManifestStage(root, true)
	hit, ok := stage.generated(strings.ToLower(skillRel))
	if !ok || hit.display != skillRel {
		t.Fatalf("folded hit = %+v, %v", hit, ok)
	}
	if hit, ok := stage.generated(".claude/other.md"); !ok || hit.display != ".claude/Other.md" {
		t.Fatalf("cleaned key hit = %+v, %v", hit, ok)
	}
}

func TestIsSourceRepo_RequiresAllThreeMarkers(t *testing.T) {
	t.Parallel()
	root := newProject(t)
	writeFile(t, root, "content/x.md", "x")
	writeFile(t, root, "templates/x.tmpl", "x")
	if isSourceRepo(root) {
		t.Fatal("two of three markers is not the source repo")
	}
	writeFile(t, root, "cmd/generate-templates/main.go", "package main\n")
	if !isSourceRepo(root) {
		t.Fatal("all three markers identify the source repo")
	}
}

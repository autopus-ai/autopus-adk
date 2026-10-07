package qualityloop

import "testing"

// The generated-surface check reads its members from the unified surface table
// (SPEC-EDITGUARD-001 REQ-EG-01) and keeps every member-specific rule, so the
// S10 probe verdicts and the edge paths below stay what they were.
func TestIsGeneratedSurfacePath_KeepsCurrentMatchingRules(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		// S10 probe paths and their pinned qualityloop verdicts.
		".claude/x":                          true,
		".agents/skills/x":                   true,
		".agents/plugins/marketplace.json":   true,
		"a/.codex/x":                         true,
		".autopus/runtime/x":                 true,
		".omp/x":                             false,
		".autopus/specs/x":                   false,
		"sub/config.toml":                    true,
		"x/plugins/cache/y":                  true,
		".autopus/claude-code-manifest.json": true,
		".autopus/backup/x":                  false,
		".mcp.json":                          false,
		".agents/hooks.json":                 true,
		// Member-specific rules.
		"foo.agents/plugins/marketplace.json": true,
		"x.autopus/context/signatures.md.bak": true,
		"config.toml.bak":                     false,
		"plugins/cache/z":                     false,
		"docs/.autopus/x/y-manifest.json":     true,
		`a\.claude\x`:                         true,
		"  .gemini/settings.json  ":           true,
		"":                                    false,
		"   ":                                 false,
		"pkg/main.go":                         false,
	}
	for path, want := range cases {
		if got := isGeneratedSurfacePath(path); got != want {
			t.Errorf("isGeneratedSurfacePath(%q) = %v, want %v", path, got, want)
		}
	}
}
